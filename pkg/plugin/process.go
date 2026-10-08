package plugin

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/debug"
)

// proc is one configured plugin across restarts.
type proc struct {
	h    *Host
	cfg  Config
	kick chan struct{}

	mu          sync.Mutex
	state       string
	version     string
	failure     string
	stderr      []string
	restarts    int
	session     *session // set while active
	revision    int
	lastToast   time.Time
	inflight    int
	actionsDone chan struct{} // closed when inflight drops to zero
}

// session is one run of a plugin process.
type session struct {
	p        *proc
	conn     *Conn
	manifest Manifest
	active   atomic.Bool
	badLines atomic.Int32
	kill     context.CancelFunc

	mu         sync.Mutex
	killReason string
	lastStderr string

	// Used only by the supervisor goroutine.
	synced   bool
	lastHash [sha256.Size]byte
}

func (p *proc) status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := Status{Name: p.cfg.Name, State: p.state, Version: p.version, LastError: p.failure, Restarts: p.restarts}
	if st.LastError == "" && len(p.stderr) > 0 {
		st.LastError = p.stderr[len(p.stderr)-1]
	}
	return st
}

func (p *proc) setState(state string) {
	p.mu.Lock()
	p.state = state
	p.mu.Unlock()
	if state == stateActive || state == stateFailed {
		p.h.emit(StatusMsg{Status: p.status()})
	}
}

func (p *proc) activeSession() *session {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.session
}

// beginAction marks an action in flight; the returned func ends it.
func (p *proc) beginAction() func() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inflight == 0 {
		p.actionsDone = make(chan struct{})
	}
	p.inflight++
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.inflight--
		if p.inflight == 0 {
			close(p.actionsDone)
		}
	}
}

// supervise runs the plugin, restarting it after each backoff step, until
// ctx is done or it fails once more than there are steps.
func (p *proc) supervise(ctx context.Context) {
	defer p.h.wg.Done()
	for failures := 0; ; {
		reason := p.run(ctx)
		if ctx.Err() != nil {
			return
		}
		failures++
		debug.Log("plugin %s: %s", p.cfg.Name, reason)
		p.mu.Lock()
		p.failure = reason
		p.mu.Unlock()
		if failures > len(p.h.restartBackoff) {
			p.setState(stateFailed)
			return
		}
		p.setState(stateStarting)
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.h.restartBackoff[failures-1]):
		}
		p.mu.Lock()
		p.restarts++
		p.mu.Unlock()
	}
}

// run starts the process once and returns why it stopped, or "" when ctx
// ended it.
func (p *proc) run(ctx context.Context) string {
	procCtx, kill := context.WithCancel(context.Background())
	defer kill()
	cmd := newCommand(procCtx, p.cfg.Command[0], p.cfg.Command[1:]...)
	cmd.Env = os.Environ()
	for k, v := range p.cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// cmd.Dir stays empty: the plugin runs in bt's working directory.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Sprintf("start: %v", err)
	}
	// Plain pipes, so Wait never waits on a grandchild holding them open.
	outR, outW, err := os.Pipe()
	if err != nil {
		return fmt.Sprintf("start: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_ = outR.Close()
		_ = outW.Close()
		return fmt.Sprintf("start: %v", err)
	}
	cmd.Stdout, cmd.Stderr = outW, errW
	err = cmd.Start()
	_ = outW.Close()
	_ = errW.Close()
	if err != nil {
		_ = outR.Close()
		_ = errR.Close()
		return fmt.Sprintf("start: %v", err)
	}

	s := &session{p: p, kill: kill}
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		s.drainStderr(errR)
	}()
	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()
	s.conn = NewConn(outR, stdin, s.handle, s.badLine)
	connCtx, stopConn := context.WithCancel(context.Background())
	connDone := make(chan struct{})
	go func() {
		defer close(connDone)
		_ = s.conn.Run(connCtx)
	}()

	reason, stopping := s.serve(ctx, exited, connDone)

	s.active.Store(false)
	p.mu.Lock()
	p.session = nil
	p.mu.Unlock()
	p.h.store.deactivate(p.cfg.Name)
	if stopping {
		s.shutdown(stdin, exited)
	} else {
		kill()
		<-exited
	}
	stopConn()
	<-connDone
	select {
	case <-stderrDone:
	case <-time.After(stderrSettle):
	}
	_ = errR.Close()

	switch {
	case stopping:
		return ""
	case s.killedFor() != "":
		return s.killedFor()
	case reason != "":
		return reason
	}
	reason = "exited"
	if waitErr != nil {
		reason += ": " + waitErr.Error()
	}
	if line := s.stderrLine(); line != "" {
		reason += ": " + line
	}
	return reason
}

// serve does the handshake, then syncs until the process ends or ctx is
// done. reason is empty when the process exited or closed its stdout.
func (s *session) serve(ctx context.Context, exited, connDone <-chan struct{}) (reason string, stopping bool) {
	p := s.p
	params := InitializeParams{ProtocolVersion: ProtocolVersion, Scope: p.h.opts.Scope, Popup: p.h.opts.Popup, Options: p.cfg.Options}
	params.BT.Version = p.h.opts.BTVersion
	ictx, cancel := context.WithTimeout(ctx, p.h.initTimeout)
	err := s.conn.Call(ictx, "initialize", params, &s.manifest)
	cancel()
	switch {
	case ctx.Err() != nil:
		return "", true
	case errors.Is(err, ErrClosed):
		return "", false
	case err != nil:
		return fmt.Sprintf("initialize: %v", err), false
	}
	if err := s.manifest.Validate(p.cfg.Name); err != nil {
		return fmt.Sprintf("invalid manifest: %v", err), false
	}

	p.h.store.activate(p.cfg.Name, s.manifest)
	s.active.Store(true)
	p.mu.Lock()
	p.session = s
	p.version = s.manifest.Version
	p.mu.Unlock()
	p.setState(stateActive)

	s.sync(ctx)
	for {
		select {
		case <-ctx.Done():
			return "", true
		case <-exited:
			return "", false
		case <-connDone:
			return "", false
		case <-p.kick:
			s.sync(ctx)
		}
	}
}

// sync sends beads.sync when the subscribed beads differ from the last set
// this process received.
func (s *session) sync(ctx context.Context) {
	issues, ok := s.p.h.latestIssues()
	if !ok {
		return
	}
	beads := s.filter(issues)
	b, err := json.Marshal(beads)
	if err != nil {
		debug.Log("plugin %s: encode beads.sync: %v", s.p.cfg.Name, err)
		return
	}
	sum := sha256.Sum256(b)
	if s.synced && sum == s.lastHash {
		return
	}
	s.p.mu.Lock()
	s.p.revision++
	rev := s.p.revision
	s.p.mu.Unlock()
	if err := notify(ctx, s.conn, "beads.sync", SyncParams{Revision: rev, Beads: beads}); err != nil {
		debug.Log("plugin %s: beads.sync lost: %v", s.p.cfg.Name, err)
		return
	}
	s.synced, s.lastHash = true, sum
}

func (s *session) filter(issues []syncIssue) []Bead {
	sub := s.manifest.Subscribe
	repo := s.p.h.opts.Repo
	beads := make([]Bead, 0, len(issues))
	for _, in := range issues {
		if len(sub.Statuses) > 0 && !contains(sub.Statuses, string(in.issue.Status)) {
			continue
		}
		b := Bead{
			DB: in.db, ID: in.issue.ID, Status: string(in.issue.Status), Title: in.issue.Title,
			Type: string(in.issue.IssueType), Priority: in.issue.Priority, Assignee: in.issue.Assignee,
		}
		for k, v := range in.issue.Metadata {
			if hasAnyPrefix(k, sub.MetadataPrefixes) {
				if b.Metadata == nil {
					b.Metadata = map[string]json.RawMessage{}
				}
				b.Metadata[k] = v
			}
		}
		if repo != nil {
			issue := in.issue
			if r := repo(&issue); r != "" {
				b.Repo = &r
			}
		}
		beads = append(beads, b)
	}
	return beads
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// notify queues a notification, giving up when ctx is done. params may be
// nil for a notification without params.
func notify(ctx context.Context, c *Conn, method string, params any) error {
	m := message{JSONRPC: "2.0", Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
		m.Params = b
	}
	if err := c.send(ctx, m); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	return nil
}

// shutdown asks the process to exit, closes its stdin, and kills it if it
// is still running after the grace period.
func (s *session) shutdown(stdin io.Closer, exited <-chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), stderrSettle)
	_ = notify(ctx, s.conn, "shutdown", nil)
	cancel()
	_ = stdin.Close()
	select {
	case <-exited:
	case <-time.After(s.p.h.shutdownGrace):
		s.kill()
		<-exited
	}
}

// killFor kills the process, recording the first reason given.
func (s *session) killFor(reason string) {
	s.mu.Lock()
	if s.killReason == "" {
		s.killReason = reason
	}
	s.mu.Unlock()
	s.kill()
}

func (s *session) killedFor() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.killReason
}

func (s *session) stderrLine() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastStderr
}

// drainStderr keeps the plugin's stderr in the ring and the debug log, and
// keeps reading past over-long lines so the plugin never blocks on it.
func (s *session) drainStderr(r io.Reader) {
	p := s.p
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if debug.Enabled() {
			debug.Log("plugin %s: %s", p.cfg.Name, line)
		}
		s.mu.Lock()
		s.lastStderr = line
		s.mu.Unlock()
		p.mu.Lock()
		p.stderr = append(p.stderr, line)
		if len(p.stderr) > stderrLines {
			p.stderr = p.stderr[len(p.stderr)-stderrLines:]
		}
		p.mu.Unlock()
	}
	_, _ = io.Copy(io.Discard, r)
}
