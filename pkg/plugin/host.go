package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/debug"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
)

// Timers, as variables so tests can shorten them. NewHost copies them.
var (
	initTimeout    = 5 * time.Second
	invokeTimeout  = 60 * time.Second
	restartBackoff = []time.Duration{time.Second, 5 * time.Second, 25 * time.Second}
	shutdownGrace  = 2 * time.Second
	flushInterval  = 100 * time.Millisecond
	toastInterval  = time.Second
	// healthyReset is how long a plugin must stay active for its failure
	// count to start over.
	healthyReset = 60 * time.Second
)

const (
	// maxBadLines malformed lines or params get a plugin killed.
	maxBadLines = 10
	// stderrLines is how many stderr lines are kept per plugin.
	stderrLines = 50
	// outboxSize bounds the messages waiting for the sender.
	outboxSize = 256
	// stderrSettle bounds the wait for a dead plugin's last stderr lines.
	stderrSettle = 100 * time.Millisecond
	// shutdownFlush bounds the wait for the shutdown notification to be
	// written before stdin is closed.
	shutdownFlush = 200 * time.Millisecond
)

// newCommand builds a plugin process; a seam for tests.
var newCommand = exec.CommandContext

// Plugin states reported in Status.State.
const (
	stateDisabled = "disabled"
	stateStarting = "starting"
	stateActive   = "active"
	stateFailed   = "failed"
)

// Options configure a Host.
type Options struct {
	Configs   []Config
	BTVersion string
	Scope     Scope
	Popup     bool
	// DB returns a bead's database name. It must be cheap: it is called on
	// the render path.
	DB func(issue *model.Issue) string
	// Repo returns the checkout path of a bead's database, or "" when
	// unknown. It may do IO and is called only when syncing (in the
	// background, at most once per database per sync, with any one bead of
	// that database) or invoking.
	Repo func(issue *model.Issue) string
	// NoRestart marks a plugin failed on its first failure instead of
	// restarting it, for a one-shot probe.
	NoRestart bool
}

// Status describes one configured plugin. State is disabled, starting,
// active or failed. Version and Actions come from the last accepted
// manifest and are kept after the plugin fails; Actions carry their keys
// after config overrides.
type Status struct {
	Name, State, Version, LastError string
	Restarts                        int
	Actions                         []Action
}

// Action is a plugin action offered on a bead. Key is empty when the config
// set it to "none".
type Action struct {
	Plugin, ID, Label, Key string
}

// ActionResult is the outcome of Invoke. Unknown means the plugin timed out
// or went away; the action is never replayed. NotRunning means the plugin
// was not active, so the action was never sent.
type ActionResult struct {
	Toast      *Toast
	Dismiss    bool
	Unknown    bool
	NotRunning bool
}

// StateChangedMsg reports beads whose plugin state changed. It is sent at
// most once per flush interval.
type StateChangedMsg struct{ Beads []BeadKey }

// ToastMsg asks the UI to show a plugin's toast.
type ToastMsg struct {
	Plugin string
	Toast  Toast
}

// StatusMsg reports a plugin becoming active or failing for good.
type StatusMsg struct{ Status Status }

// PromptMsg asks the UI to show a confirm or select prompt for a running
// action. Exactly one of Confirm and Select is set.
type PromptMsg struct {
	Plugin  string
	Confirm *ConfirmParams
	Select  *SelectParams
	// Reply delivers the answer; nil means dismissed. Only the first call
	// counts, and calls after the action ended do nothing.
	Reply func(answer any)
	// Done closes once the prompt is no longer awaited: answered, its
	// action ended, or the plugin went away. The UI should then close a
	// prompt it still shows.
	Done <-chan struct{}
}

// Host runs the configured plugins and holds the state they push. All
// plugin IO happens on its own goroutines; results reach the UI through
// the sender.
type Host struct {
	opts    Options
	store   *store
	plugins []*proc

	initTimeout, invokeTimeout, shutdownGrace, flushInterval, toastInterval, healthyReset time.Duration
	restartBackoff                                                                        []time.Duration

	sendMu sync.Mutex
	send   func(tea.Msg)
	held   []tea.Msg
	outbox chan tea.Msg

	latestMu  sync.Mutex
	latest    []syncIssue
	latestGen uint64

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once
}

// syncIssue is a bead copied out of the model for beads.sync.
type syncIssue struct {
	issue   model.Issue
	db      string
	blocked bool
}

// NewHost returns a host for opts.Configs; nothing runs until Start.
func NewHost(opts Options) *Host {
	h := &Host{
		opts:           opts,
		store:          newStore(opts.Configs, opts.DB),
		outbox:         make(chan tea.Msg, outboxSize),
		initTimeout:    initTimeout,
		invokeTimeout:  invokeTimeout,
		shutdownGrace:  shutdownGrace,
		flushInterval:  flushInterval,
		toastInterval:  toastInterval,
		healthyReset:   healthyReset,
		restartBackoff: append([]time.Duration(nil), restartBackoff...),
	}
	h.ctx, h.cancel = context.WithCancel(context.Background())
	for _, cfg := range opts.Configs {
		p := &proc{h: h, cfg: cfg, kick: make(chan struct{}, 1), state: stateStarting}
		if !cfg.IsEnabled() {
			p.state = stateDisabled
		}
		h.plugins = append(h.plugins, p)
	}
	return h
}

// SetSender sets the function messages are delivered through, typically
// tea.Program.Send. Call it before Start: status messages produced before
// it is set are held and delivered then, possibly after later messages, and
// all other messages are dropped.
func (h *Host) SetSender(send func(tea.Msg)) {
	h.sendMu.Lock()
	h.send = send
	held := h.held
	h.held = nil
	h.sendMu.Unlock()
	for _, m := range held {
		h.emit(m)
	}
}

// emit queues m for the sender without ever blocking.
func (h *Host) emit(m tea.Msg) {
	select {
	case h.outbox <- m:
	default:
		debug.Log("plugin host: outbox full, dropping %T", m)
	}
}

// pump delivers queued messages in order, so a slow sender never blocks a
// plugin's reader.
func (h *Host) pump() {
	for {
		select {
		case <-h.ctx.Done():
			return
		case m := <-h.outbox:
			h.sendMu.Lock()
			send := h.send
			if send == nil {
				if _, ok := m.(StatusMsg); ok {
					h.held = append(h.held, m)
				}
			}
			h.sendMu.Unlock()
			if send != nil {
				send(m)
			}
		}
	}
}

// Start starts every enabled plugin. Later calls, and calls after Stop, do
// nothing.
func (h *Host) Start() {
	h.startOnce.Do(func() {
		if h.ctx.Err() != nil {
			return
		}
		go h.pump()
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			h.store.flushLoop(h.ctx.Done(), h.flushInterval, h.emit)
		}()
		for _, p := range h.plugins {
			if p.cfg.IsEnabled() {
				h.wg.Add(1)
				go p.supervise(h.ctx)
			}
		}
	})
}

// Stop shuts every plugin down: shutdown notification, stdin closed, a grace
// period, then kill. It is idempotent.
func (h *Host) Stop() {
	h.stopOnce.Do(func() {
		h.cancel()
		h.wg.Wait()
	})
}

// Register adds the host's badge, section and field providers to reg.
func (h *Host) Register(reg *slots.Registry) {
	reg.AddBadges(slots.BadgeFunc(h.store.badges))
	reg.AddSections(slots.SectionFunc(h.store.sections))
	reg.AddFields(fieldProvider{h.store})
}

// SyncIssues copies issues and sends them to the active plugins in the
// background. A plugin gets beads.sync only when its subscribed set changed.
func (h *Host) SyncIssues(issues []model.Issue) {
	statuses := make(map[string]model.Status, len(issues))
	for i := range issues {
		statuses[issues[i].ID] = issues[i].Status
	}
	latest := make([]syncIssue, len(issues))
	for i := range issues {
		src := &issues[i]
		cp := model.Issue{
			ID: src.ID, Title: src.Title, Status: src.Status, Priority: src.Priority,
			IssueType: src.IssueType, Assignee: src.Assignee, SourceRepo: src.SourceRepo,
			UpdatedAt: src.UpdatedAt,
		}
		if src.Metadata != nil {
			cp.Metadata = make(map[string]json.RawMessage, len(src.Metadata))
			for k, v := range src.Metadata {
				cp.Metadata[k] = append(json.RawMessage(nil), v...)
			}
		}
		latest[i] = syncIssue{issue: cp, db: h.store.db(src), blocked: isBlocked(src, statuses)}
	}
	h.latestMu.Lock()
	h.latest = latest
	h.latestGen++
	h.latestMu.Unlock()
	for _, p := range h.plugins {
		select {
		case p.kick <- struct{}{}:
		default:
		}
	}
}

// isBlocked reports whether issue has status blocked or an open blocking
// dependency on a loaded bead, the rule of bt's ready filter. Blockers that
// are not loaded count as resolved.
func isBlocked(issue *model.Issue, statuses map[string]model.Status) bool {
	if issue.Status == model.StatusBlocked {
		return true
	}
	for _, dep := range issue.Dependencies {
		if dep == nil || !dep.Type.IsBlocking() {
			continue
		}
		if st, ok := statuses[dep.DependsOnID]; ok && st != model.StatusClosed && st != model.StatusTombstone {
			return true
		}
	}
	return false
}

// latestIssues returns the last synced issues and their generation, which
// changes on every SyncIssues; gen 0 means none yet.
func (h *Host) latestIssues() (issues []syncIssue, gen uint64) {
	h.latestMu.Lock()
	defer h.latestMu.Unlock()
	return h.latest, h.latestGen
}

// Actions lists the plugin actions offered on issue, in config order.
func (h *Host) Actions(issue *model.Issue) []Action { return h.store.actions(issue) }

// FieldPrefixes returns "<plugin>." for every active plugin.
func (h *Host) FieldPrefixes() []string {
	var out []string
	for _, name := range h.store.activeNames() {
		out = append(out, name+".")
	}
	return out
}

// Statuses reports every configured plugin, in config order.
func (h *Host) Statuses() []Status {
	out := make([]Status, 0, len(h.plugins))
	for _, p := range h.plugins {
		out = append(out, p.status())
	}
	return out
}

// Invoke runs action a on issue and waits for the plugin's answer, at most
// the invoke timeout. It may do IO: call it off the UI goroutine.
func (h *Host) Invoke(ctx context.Context, a Action, issue *model.Issue, view string) ActionResult {
	if issue == nil {
		return ActionResult{Unknown: true}
	}
	p := h.proc(a.Plugin)
	if p == nil {
		return ActionResult{NotRunning: true}
	}
	s := p.activeSession()
	if s == nil {
		return ActionResult{NotRunning: true}
	}
	ref := BeadRef{DB: h.store.db(issue), ID: issue.ID}
	if h.opts.Repo != nil {
		if repo := h.opts.Repo(issue); repo != "" {
			ref.Repo = &repo
		}
	}
	end := p.beginAction()
	defer end()
	ctx, cancel := context.WithTimeout(ctx, h.invokeTimeout)
	defer cancel()
	var res InvokeResult
	err := s.conn.Call(ctx, "action.invoke", InvokeParams{Action: a.ID, Bead: ref, View: view}, &res)
	var rpcErr *RPCError
	switch {
	case err == nil:
		return ActionResult{Toast: res.Toast, Dismiss: res.Dismiss && h.opts.Popup}
	case errors.As(err, &rpcErr):
		return ActionResult{Toast: &Toast{Message: rpcErr.Message, Tone: "error"}}
	default:
		debug.Log("plugin %s: action %s on %s: %v", a.Plugin, a.ID, issue.ID, err)
		return ActionResult{Unknown: true}
	}
}

func (h *Host) proc(name string) *proc {
	for _, p := range h.plugins {
		if p.cfg.Name == name {
			return p
		}
	}
	return nil
}
