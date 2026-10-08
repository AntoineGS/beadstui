package plugin

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
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
	saved := []any{initTimeout, invokeTimeout, restartBackoff, shutdownGrace, flushInterval}
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
	h := NewHost(Options{
		Configs:   []Config{cfg},
		BTVersion: "test",
		Scope:     Scope{Mode: "project"},
		DB:        func(*model.Issue) string { return "proj" },
		Repo:      func(*model.Issue) string { return "" },
	})
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
	if res := th.Invoke(context.Background(), Action{Plugin: "example", ID: "dispatch"}, &issue, "list"); !res.Unknown {
		t.Errorf("Invoke on failed plugin = %+v, want Unknown", res)
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
