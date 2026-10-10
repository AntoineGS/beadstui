package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
)

type recorder struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (r *recorder) send(m tea.Msg) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, m)
}

func (r *recorder) all() []tea.Msg {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]tea.Msg(nil), r.msgs...)
}

func (r *recorder) count(match func(tea.Msg) bool) int {
	n := 0
	for _, m := range r.all() {
		if match(m) {
			n++
		}
	}
	return n
}

func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// shortTimers shortens the package timers for one test.
func shortTimers(t *testing.T) {
	t.Helper()
	saved := []any{initTimeout, invokeTimeout, restartBackoff, shutdownGrace, flushInterval, healthyReset}
	initTimeout = 500 * time.Millisecond
	invokeTimeout = time.Second
	restartBackoff = []time.Duration{50 * time.Millisecond, 50 * time.Millisecond, 50 * time.Millisecond}
	shutdownGrace = 200 * time.Millisecond
	t.Cleanup(func() {
		initTimeout = saved[0].(time.Duration)
		invokeTimeout = saved[1].(time.Duration)
		restartBackoff = saved[2].([]time.Duration)
		shutdownGrace = saved[3].(time.Duration)
		flushInterval = saved[4].(time.Duration)
		healthyReset = saved[5].(time.Duration)
	})
}

type testHost struct {
	*Host
	rec *recorder
	reg *slots.Registry
}

func exampleConfig(behaviour string) Config {
	return Config{Name: "example", Command: []string{os.Args[0]}, Env: map[string]string{
		"BT_TEST_PLUGIN": behaviour,
		// Under -race the helper would otherwise sleep 1s on exit.
		"GORACE": "atexit_sleep_ms=0",
	}}
}

// startHost starts a host for cfg after shortTimers; Stop runs at cleanup.
func startHost(t *testing.T, cfg Config) *testHost {
	t.Helper()
	return startHostWith(t, Options{
		Configs: []Config{cfg},
		DB:      func(*model.Issue) string { return "proj" },
		Repo:    func(*model.Issue) string { return "" },
	})
}

func startHostWith(t *testing.T, opts Options) *testHost {
	t.Helper()
	opts.BTVersion = "test"
	opts.Scope = Scope{Mode: "project"}
	h := NewHost(opts)
	th := &testHost{Host: h, rec: &recorder{}, reg: slots.NewRegistry()}
	h.SetSender(th.rec.send)
	h.Register(th.reg)
	h.Start()
	t.Cleanup(h.Stop)
	return th
}

func (th *testHost) status() Status { return th.Statuses()[0] }

func (th *testHost) waitActive(t *testing.T) {
	t.Helper()
	waitFor(t, 5*time.Second, "plugin active", func() bool { return th.status().State == "active" })
}

func (th *testHost) stateChanges() int {
	return th.rec.count(func(m tea.Msg) bool { _, ok := m.(StateChangedMsg); return ok })
}

func (th *testHost) sectionTexts(issue *model.Issue) []string {
	var out []string
	for _, s := range th.reg.Sections(issue, slots.Context{}) {
		out = append(out, s.Markdown)
	}
	return out
}

func (th *testHost) waitSection(t *testing.T, issue *model.Issue, want string) {
	t.Helper()
	waitFor(t, 5*time.Second, "section "+want, func() bool {
		return reflect.DeepEqual(th.sectionTexts(issue), []string{want})
	})
}

func openIssue(id string) model.Issue {
	return model.Issue{ID: id, Title: "Issue " + id, Status: model.StatusOpen, IssueType: model.TypeTask, Priority: 2}
}

func TestHostHandshakeSyncAndState(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("ok"))
	issues := []model.Issue{openIssue("example-1"), openIssue("example-2")}
	th.SyncIssues(issues)
	waitFor(t, 5*time.Second, "StateChangedMsg", func() bool { return th.stateChanges() > 0 })
	issue := &issues[0]
	th.waitSection(t, issue, "sync rev 1")

	badges := th.reg.Badges(issue)
	if len(badges) != 1 || badges[0].Text != "WAIT" || badges[0].Tone != slots.ToneWarn {
		t.Errorf("badges = %+v, want [WAIT warn]", badges)
	}
	if v, ok := th.reg.FieldValue(issue, "example.state"); !ok || v != "waiting" {
		t.Errorf("example.state = %q, %v", v, ok)
	}
	want := []Action{{Plugin: "example", ID: "dispatch", Label: "Dispatch", Key: "D"}}
	if got := th.Actions(issue); !reflect.DeepEqual(got, want) {
		t.Errorf("Actions = %+v, want %+v", got, want)
	}
	if got := th.FieldPrefixes(); !reflect.DeepEqual(got, []string{"example."}) {
		t.Errorf("FieldPrefixes = %v", got)
	}
	if st := th.status(); st.Version != "0.1.0" {
		t.Errorf("status = %+v", st)
	}
	if th.rec.count(func(m tea.Msg) bool { s, ok := m.(StatusMsg); return ok && s.Status.State == "active" }) != 1 {
		t.Error("want one StatusMsg active")
	}
}

func TestHostSubscribeFilters(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("ok-subscribed"))
	open := openIssue("example-1")
	open.Metadata = map[string]json.RawMessage{"example.session": json.RawMessage(`"s"`), "other": json.RawMessage(`"x"`)}
	closed := openIssue("example-2")
	closed.Status = model.StatusClosed
	th.SyncIssues([]model.Issue{open, closed})
	th.waitSection(t, &open, "n=1 meta=example.session")
	if got := th.sectionTexts(&closed); len(got) != 0 {
		t.Errorf("closed bead got state: %v", got)
	}
}

func TestHostSyncSkipsUnchangedPayload(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("ok"))
	issues := []model.Issue{openIssue("example-1")}
	th.SyncIssues(issues)
	th.waitSection(t, &issues[0], "sync rev 1")
	th.SyncIssues([]model.Issue{openIssue("example-1")})
	// A changed payload must arrive as revision 2; revision 3 would mean
	// the identical sync was sent too.
	changed := []model.Issue{openIssue("example-1"), openIssue("example-2")}
	th.SyncIssues(changed)
	th.waitSection(t, &changed[1], "sync rev 2")
	if got := th.sectionTexts(&changed[0]); !reflect.DeepEqual(got, []string{"sync rev 2"}) {
		t.Errorf("sections = %v, want [sync rev 2]", got)
	}
}

func TestHostSyncSendsBlockedAndUpdatedAt(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("echo-bead"))
	updated := time.Date(2026, 10, 8, 12, 30, 0, 0, time.UTC)
	blocker := openIssue("example-1")
	blocked := openIssue("example-2")
	blocked.UpdatedAt = updated
	blocked.Dependencies = []*model.Dependency{{IssueID: "example-2", DependsOnID: "example-1", Type: model.DepBlocks}}
	related := openIssue("example-3")
	related.Dependencies = []*model.Dependency{{IssueID: "example-3", DependsOnID: "example-1", Type: model.DepRelated}}
	external := openIssue("example-4")
	external.Dependencies = []*model.Dependency{{IssueID: "example-4", DependsOnID: "other-9", Type: model.DepBlocks}}
	status := openIssue("example-5")
	status.Status = model.StatusBlocked

	th.SyncIssues([]model.Issue{blocker, blocked, related, external, status})
	zero := time.Time{}.Format(time.RFC3339)
	th.waitSection(t, &blocked, "blocked=true updated="+updated.Format(time.RFC3339))
	for _, c := range []struct {
		issue model.Issue
		want  string
	}{
		{blocker, "blocked=false updated=" + zero},
		{related, "blocked=false updated=" + zero},
		{external, "blocked=false updated=" + zero},
		{status, "blocked=true updated=" + zero},
	} {
		if got := th.sectionTexts(&c.issue); !reflect.DeepEqual(got, []string{c.want}) {
			t.Errorf("%s sections = %v, want [%s]", c.issue.ID, got, c.want)
		}
	}

	blocker.Status = model.StatusClosed
	th.SyncIssues([]model.Issue{blocker, blocked})
	th.waitSection(t, &blocked, "blocked=false updated="+updated.Format(time.RFC3339))
}

func TestHostInvoke(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("ok"))
	th.waitActive(t)
	issue := openIssue("example-1")
	res := th.Invoke(context.Background(), Action{Plugin: "example", ID: "dispatch"}, &issue, "list")
	if res.Unknown || res.Toast == nil || res.Toast.Message != "did dispatch example-1" {
		t.Fatalf("Invoke = %+v (toast %+v)", res, res.Toast)
	}
}

func TestHostKeyOverride(t *testing.T) {
	for override, want := range map[string]string{"X": "X", "none": ""} {
		t.Run(override, func(t *testing.T) {
			shortTimers(t)
			cfg := exampleConfig("ok")
			cfg.Keys = map[string]string{"dispatch": override}
			th := startHost(t, cfg)
			issues := []model.Issue{openIssue("example-1")}
			th.SyncIssues(issues)
			th.waitSection(t, &issues[0], "sync rev 1")
			got := th.Actions(&issues[0])
			if len(got) != 1 || got[0].ID != "dispatch" || got[0].Key != want {
				t.Fatalf("Actions = %+v, want key %q", got, want)
			}
		})
	}
}

func TestHostStatusActionsKeptAfterFailure(t *testing.T) {
	shortTimers(t)
	cfg := exampleConfig("exit-after-init")
	cfg.Keys = map[string]string{"dispatch": "X"}
	th := startHostWith(t, Options{Configs: []Config{cfg}, NoRestart: true})
	th.waitActive(t)
	want := []Action{{Plugin: "example", ID: "dispatch", Label: "Dispatch", Key: "X"}}
	if got := th.status().Actions; !reflect.DeepEqual(got, want) {
		t.Fatalf("active Actions = %+v, want %+v", got, want)
	}
	waitFor(t, 5*time.Second, "plugin failed", func() bool { return th.status().State == "failed" })
	st := th.status()
	if st.Restarts != 0 || st.Version != "0.1.0" || !reflect.DeepEqual(st.Actions, want) {
		t.Errorf("failed status = %+v, want no restarts and the manifest kept", st)
	}
}

func TestHostNoRestartFailsAtOnce(t *testing.T) {
	shortTimers(t)
	restartBackoff = []time.Duration{time.Minute}
	th := startHostWith(t, Options{Configs: []Config{exampleConfig("bad-manifest")}, NoRestart: true})
	waitFor(t, 5*time.Second, "plugin failed", func() bool { return th.status().State == "failed" })
	if st := th.status(); st.Restarts != 0 || !strings.Contains(st.LastError, "protocol version") {
		t.Errorf("status = %+v", st)
	}
}

func TestHostHangingInitFails(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("hang-init"))
	waitFor(t, 10*time.Second, "plugin failed", func() bool { return th.status().State == "failed" })
	if st := th.status(); st.Restarts != len(restartBackoff) || !strings.Contains(st.LastError, "initialize") {
		t.Errorf("status = %+v", st)
	}
	if th.rec.count(func(m tea.Msg) bool { s, ok := m.(StatusMsg); return ok && s.Status.State == "failed" }) != 1 {
		t.Error("want one StatusMsg failed")
	}
	issue := openIssue("example-1")
	if res := th.Invoke(context.Background(), Action{Plugin: "example", ID: "dispatch"}, &issue, "list"); !res.NotRunning || res.Unknown {
		t.Errorf("Invoke on failed plugin = %+v, want NotRunning", res)
	}
	if res := th.Invoke(context.Background(), Action{Plugin: "other", ID: "dispatch"}, &issue, "list"); !res.NotRunning || res.Unknown {
		t.Errorf("Invoke on unknown plugin = %+v, want NotRunning", res)
	}
}

func TestHostFailureCountResetsAfterHealthyRun(t *testing.T) {
	shortTimers(t)
	healthyReset = 150 * time.Millisecond
	th := startHost(t, exampleConfig("exit-after-init"))
	// Every run stays active ~300ms, past the reset, so no crash counts
	// towards the 3 backoff steps.
	waitFor(t, 10*time.Second, "5 restarts", func() bool {
		st := th.status()
		if st.State == "failed" {
			t.Fatalf("plugin failed after %d restarts: %+v", st.Restarts, st)
		}
		return st.Restarts >= 5
	})
}

func TestHostMissingCommandFailsImmediately(t *testing.T) {
	shortTimers(t)
	restartBackoff = []time.Duration{time.Second, time.Second, time.Second}
	for _, cmd := range []string{"bt-example-plugin-does-not-exist", filepath.Join(t.TempDir(), "missing")} {
		t.Run(filepath.Base(cmd), func(t *testing.T) {
			th := startHost(t, Config{Name: "example", Command: []string{cmd, "--flag"}})
			waitFor(t, 500*time.Millisecond, "plugin failed", func() bool { return th.status().State == "failed" })
			if st := th.status(); st.LastError != "command not found: "+cmd || st.Restarts != 0 {
				t.Errorf("status = %+v, want LastError %q and no restarts", st, "command not found: "+cmd)
			}
		})
	}
}

func TestHostBadManifest(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("bad-manifest"))
	waitFor(t, 10*time.Second, "plugin failed", func() bool { return th.status().State == "failed" })
	if st := th.status(); !strings.Contains(st.LastError, "protocol version") {
		t.Errorf("LastError = %q", st.LastError)
	}
}

func TestHostCrashDuringInvoke(t *testing.T) {
	shortTimers(t)
	// Long enough to observe the dropped state before the restart re-syncs.
	restartBackoff = []time.Duration{300 * time.Millisecond, 300 * time.Millisecond, 300 * time.Millisecond}
	th := startHost(t, exampleConfig("crash-on-invoke"))
	issues := []model.Issue{openIssue("example-1")}
	issue := &issues[0]
	th.SyncIssues(issues)
	th.waitSection(t, issue, "sync rev 1")

	start := time.Now()
	res := th.Invoke(context.Background(), Action{Plugin: "example", ID: "dispatch"}, issue, "list")
	if !res.Unknown || time.Since(start) > time.Second {
		t.Fatalf("Invoke = %+v after %v, want Unknown within 1s", res, time.Since(start))
	}
	before := th.stateChanges()
	waitFor(t, 2*time.Second, "state dropped", func() bool {
		return th.stateChanges() > before && len(th.reg.Badges(issue)) == 0
	})
	waitFor(t, 5*time.Second, "restart", func() bool {
		st := th.status()
		return st.Restarts == 1 && st.State == "active" && len(th.reg.Badges(issue)) == 1
	})
	if st := th.status(); st.LastError != "" {
		t.Errorf("LastError after a successful restart = %q, want empty", st.LastError)
	}
}

func TestHostSlowInvokeTimesOut(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("slow-invoke"))
	issues := []model.Issue{openIssue("example-1")}
	issue := &issues[0]
	th.SyncIssues(issues)
	th.waitSection(t, issue, "sync rev 1")

	done := make(chan ActionResult, 1)
	start := time.Now()
	go func() {
		done <- th.Invoke(context.Background(), Action{Plugin: "example", ID: "dispatch"}, issue, "list")
	}()
	for {
		select {
		case res := <-done:
			if elapsed := time.Since(start); !res.Unknown || elapsed < 900*time.Millisecond || elapsed > 3*time.Second {
				t.Fatalf("Invoke = %+v after %v, want Unknown after ~1s", res, elapsed)
			}
			return
		default:
		}
		t0 := time.Now()
		th.reg.Badges(issue)
		if d := time.Since(t0); d > 100*time.Millisecond {
			t.Fatalf("Badges blocked for %v during invoke", d)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHostConfirmPrompt(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("confirm"))
	th.waitActive(t)
	issue := openIssue("example-1")
	done := make(chan ActionResult, 1)
	go func() {
		done <- th.Invoke(context.Background(), Action{Plugin: "example", ID: "dispatch"}, &issue, "list")
	}()

	var prompt PromptMsg
	waitFor(t, 5*time.Second, "PromptMsg", func() bool {
		for _, m := range th.rec.all() {
			if p, ok := m.(PromptMsg); ok {
				prompt = p
				return true
			}
		}
		return false
	})
	if prompt.Plugin != "example" || prompt.Confirm == nil || prompt.Confirm.Message != "example-1" || prompt.Select != nil {
		t.Fatalf("prompt = %+v", prompt)
	}
	prompt.Reply(true)
	prompt.Reply(false) // only the first answer counts
	select {
	case res := <-done:
		if res.Toast == nil || res.Toast.Message != "confirmed=true" {
			t.Fatalf("Invoke = %+v (toast %+v)", res, res.Toast)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Invoke did not return after Reply")
	}
	prompt.Reply(nil) // after the action ended: no-op
	select {
	case <-prompt.Done:
	case <-time.After(2 * time.Second):
		t.Fatal("Done not closed after the prompt was answered")
	}
}

func TestHostPromptDoneWhenActionEnds(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("confirm"))
	th.waitActive(t)
	issue := openIssue("example-1")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan ActionResult, 1)
	go func() {
		done <- th.Invoke(ctx, Action{Plugin: "example", ID: "dispatch"}, &issue, "list")
	}()
	var prompt PromptMsg
	waitFor(t, 5*time.Second, "PromptMsg", func() bool {
		for _, m := range th.rec.all() {
			if p, ok := m.(PromptMsg); ok {
				prompt = p
				return true
			}
		}
		return false
	})
	select {
	case <-prompt.Done:
		t.Fatal("Done closed while the prompt is awaited")
	default:
	}
	cancel()
	if res := <-done; !res.Unknown {
		t.Fatalf("cancelled Invoke = %+v, want Unknown", res)
	}
	select {
	case <-prompt.Done:
	case <-time.After(2 * time.Second):
		t.Fatal("Done not closed after the action ended")
	}
	prompt.Reply(true) // stale: no-op
}

func TestFilterResolvesRepoPerDatabase(t *testing.T) {
	var calls int
	h := NewHost(Options{Repo: func(i *model.Issue) string {
		calls++
		if i.SourceRepo == "unknown" {
			return ""
		}
		return "/src/" + i.SourceRepo
	}})
	s := &session{p: &proc{h: h}, manifest: validManifest()}
	var issues []syncIssue
	for i, db := range []string{"proj", "other", "proj", "unknown", "other", "unknown"} {
		issues = append(issues, syncIssue{issue: model.Issue{ID: fmt.Sprintf("example-%d", i), SourceRepo: db}, db: db})
	}
	beads, ok := s.filter(context.Background(), issues)
	if !ok || len(beads) != len(issues) {
		t.Fatalf("filter = %d beads, %v", len(beads), ok)
	}
	for _, b := range beads {
		want := "/src/" + b.DB
		if b.DB == "unknown" {
			if b.Repo != nil {
				t.Errorf("%s: repo %q, want null", b.ID, *b.Repo)
			}
		} else if b.Repo == nil || *b.Repo != want {
			t.Errorf("%s: repo %v, want %s", b.ID, b.Repo, want)
		}
	}
	if calls != 3 {
		t.Errorf("Repo called %d times, want 3", calls)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := s.filter(cancelled, issues); ok {
		t.Error("filter must give up when ctx is done")
	}
}

func TestHostPromptOutsideActionRejected(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("confirm-unsolicited"))
	th.SyncIssues([]model.Issue{openIssue("example-1")})
	waitFor(t, 5*time.Second, "ToastMsg", func() bool {
		return th.rec.count(func(m tea.Msg) bool {
			tm, ok := m.(ToastMsg)
			return ok && tm.Plugin == "example" && strings.Contains(tm.Toast.Message, "no_action")
		}) == 1
	})
	if n := th.rec.count(func(m tea.Msg) bool { _, ok := m.(PromptMsg); return ok }); n != 0 {
		t.Errorf("got %d PromptMsg, want none", n)
	}
}

func TestHostToastRateLimit(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("toast-spam"))
	th.SyncIssues([]model.Issue{openIssue("example-1")})
	isToast := func(m tea.Msg) bool { _, ok := m.(ToastMsg); return ok }
	waitFor(t, 5*time.Second, "first ToastMsg", func() bool { return th.rec.count(isToast) > 0 })
	time.Sleep(500 * time.Millisecond)
	if n := th.rec.count(isToast); n != 1 {
		t.Fatalf("got %d ToastMsg, want 1", n)
	}
}

func TestHostGarbageKills(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("garbage"))
	th.SyncIssues([]model.Issue{openIssue("example-1")})
	waitFor(t, 5*time.Second, "restart", func() bool { return th.status().Restarts >= 1 })
}

func TestHostStopIsBounded(t *testing.T) {
	shortTimers(t)
	shutdownGrace = 2 * time.Second
	th := startHost(t, exampleConfig("ok"))
	th.waitActive(t)
	start := time.Now()
	th.Stop()
	if d := time.Since(start); d > 2500*time.Millisecond {
		t.Fatalf("Stop took %v", d)
	}
	start = time.Now()
	th.Stop()
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("second Stop took %v", d)
	}
}

func TestHostUnknownMethodsIgnored(t *testing.T) {
	shortTimers(t)
	th := startHost(t, exampleConfig("unknown-spam"))
	issues := []model.Issue{openIssue("example-1")}
	th.SyncIssues(issues)
	// The toast follows the 11 unknown notifications on the same reader, so
	// once it arrives all of them were handled.
	waitFor(t, 5*time.Second, "toast after unknown notifications", func() bool {
		return th.rec.count(func(m tea.Msg) bool { tm, ok := m.(ToastMsg); return ok && tm.Toast.Message == "unknown done" }) == 1
	})
	res := th.Invoke(context.Background(), Action{Plugin: "example", ID: "dispatch"}, &issues[0], "list")
	if res.Unknown {
		t.Fatal("plugin was killed for unknown notifications")
	}
	if st := th.status(); st.Restarts != 0 || st.State != "active" {
		t.Fatalf("status = %+v, want active with no restarts", st)
	}
}

func TestHostShutdownDeliveredBeforeStdinCloses(t *testing.T) {
	shortTimers(t)
	marker := filepath.Join(t.TempDir(), "shutdown")
	cfg := exampleConfig("ignore-eof-until-shutdown")
	cfg.Env["BT_TEST_PLUGIN_MARKER"] = marker
	th := startHost(t, cfg)
	th.waitActive(t)
	th.Stop()
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("plugin never received shutdown: %v", err)
	}
}

func TestHostStopKillsPluginIgnoringShutdown(t *testing.T) {
	shortTimers(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	cfg := exampleConfig("ignore-shutdown")
	cfg.Env["BT_TEST_PLUGIN_PIDFILE"] = pidFile
	th := startHost(t, cfg)
	th.waitActive(t)
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(b))
	start := time.Now()
	th.Stop()
	if d, bound := time.Since(start), shutdownGrace+500*time.Millisecond; d > bound {
		t.Fatalf("Stop took %v, want ≤ %v", d, bound)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("plugin process %d still exists after Stop (kill 0: %v)", pid, err)
	}
}

func TestHostRepoOncePerDatabase(t *testing.T) {
	shortTimers(t)
	var calls atomic.Int32
	th := startHostWith(t, Options{
		Configs: []Config{exampleConfig("ok")},
		DB:      func(i *model.Issue) string { return i.SourceRepo },
		Repo: func(i *model.Issue) string {
			calls.Add(1)
			return "/src/" + i.SourceRepo
		},
	})
	var issues []model.Issue
	for i := 0; i < 100; i++ {
		issue := openIssue(fmt.Sprintf("example-%d", i))
		issue.SourceRepo = []string{"proj", "other"}[i%2]
		issues = append(issues, issue)
	}
	th.SyncIssues(issues)
	th.waitSection(t, &issues[99], "sync rev 1")
	if n := calls.Load(); n != 2 {
		t.Fatalf("Repo called %d times, want 2 (once per database)", n)
	}
}
