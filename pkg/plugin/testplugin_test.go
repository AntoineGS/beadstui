package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMain runs the scripted test plugin instead of the tests when the host
// starts this binary with BT_TEST_PLUGIN set.
func TestMain(m *testing.M) {
	if spec := os.Getenv("BT_TEST_PLUGIN"); spec != "" {
		runTestPlugin(spec)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runTestPlugin speaks the protocol on stdin/stdout. spec is a
// comma-separated list of behaviours; every behaviour also does what "ok"
// does unless it overrides it. Behaviours that need an active host
// (toast-spam, unknown-spam, garbage, confirm-unsolicited) start on the
// first beads.sync, which bt only sends once the manifest is accepted.
// ignore-eof-until-shutdown keeps running after stdin closes and writes
// $BT_TEST_PLUGIN_MARKER on shutdown; ignore-shutdown ignores both.
// exit-after-init exits 300ms after the handshake. echo-bead puts each
// bead's blocked and updated_at in its section.
func runTestPlugin(spec string) {
	has := map[string]bool{}
	for _, b := range strings.Split(spec, ",") {
		has[b] = true
	}
	name := os.Getenv("BT_TEST_PLUGIN_NAME")
	if name == "" {
		name = "example"
	}
	var conn *Conn
	var afterSync sync.Once
	handler := func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		switch method {
		case "initialize":
			if has["hang-init"] {
				select {}
			}
			if has["exit-after-init"] {
				go func() {
					time.Sleep(300 * time.Millisecond)
					os.Exit(3)
				}()
			}
			m := validManifest()
			m.Name = name
			if has["bad-manifest"] {
				m.ProtocolVersion = 2
			}
			if has["ok-subscribed"] {
				m.Subscribe = Subscribe{Statuses: []string{"open"}, MetadataPrefixes: []string{"example."}}
			}
			return m, nil
		case "beads.sync":
			var p SyncParams
			_ = json.Unmarshal(params, &p)
			text := fmt.Sprintf("sync rev %d", p.Revision)
			if has["ok-subscribed"] {
				var keys []string
				for _, b := range p.Beads {
					for k := range b.Metadata {
						keys = append(keys, k)
					}
				}
				sort.Strings(keys)
				text = fmt.Sprintf("n=%d meta=%s", len(p.Beads), strings.Join(keys, ","))
			}
			set := StateSetParams{}
			for _, b := range p.Beads {
				section := text
				if has["echo-bead"] {
					section = fmt.Sprintf("blocked=%v updated=%s", b.Blocked, b.UpdatedAt.UTC().Format(time.RFC3339))
				}
				set.Beads = append(set.Beads, BeadState{
					DB: b.DB, ID: b.ID,
					Fields:   map[string]FieldState{"state": {Value: "waiting"}},
					Sections: map[string]string{"agent": section},
					Actions:  []string{"dispatch"},
				})
			}
			_ = conn.Notify("state.set", set)
			afterSync.Do(func() { go afterActive(conn, has) })
			return nil, nil
		case "action.invoke":
			var p InvokeParams
			_ = json.Unmarshal(params, &p)
			switch {
			case has["crash-on-invoke"]:
				os.Exit(3)
			case has["slow-invoke"]:
				time.Sleep(2 * time.Minute)
			case has["confirm"]:
				var answer any
				if err := conn.Call(ctx, "ui.confirm", ConfirmParams{Title: "Sure?", Message: p.Bead.ID}, &answer); err != nil {
					return nil, err
				}
				return InvokeResult{Toast: &Toast{Message: fmt.Sprintf("confirmed=%v", answer)}}, nil
			}
			return InvokeResult{Toast: &Toast{Message: fmt.Sprintf("did %s %s", p.Action, p.Bead.ID)}}, nil
		case "shutdown":
			if has["ignore-shutdown"] {
				return nil, nil
			}
			if marker := os.Getenv("BT_TEST_PLUGIN_MARKER"); marker != "" {
				_ = os.WriteFile(marker, []byte("shutdown"), 0o600)
			}
			os.Exit(0)
		}
		return nil, &RPCError{Code: -32601, Message: "method not found: " + method}
	}
	if pidFile := os.Getenv("BT_TEST_PLUGIN_PIDFILE"); pidFile != "" {
		_ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600)
	}
	conn = NewConn(os.Stdin, os.Stdout, handler, nil)
	_ = conn.Run(context.Background())
	if has["ignore-eof-until-shutdown"] || has["ignore-shutdown"] {
		select {}
	}
}

func afterActive(conn *Conn, has map[string]bool) {
	if has["toast-spam"] {
		for i := 0; i < 20; i++ {
			_ = conn.Notify("ui.toast", Toast{Message: fmt.Sprintf("spam %d", i)})
		}
	}
	if has["unknown-spam"] {
		for i := 0; i < 11; i++ {
			_ = conn.Notify("example.unknown", map[string]int{"n": i})
		}
		_ = conn.Notify("ui.toast", Toast{Message: "unknown done"})
	}
	if has["garbage"] {
		for i := 0; i < 11; i++ {
			_, _ = os.Stdout.Write([]byte("this is not json\n"))
		}
	}
	if has["confirm-unsolicited"] {
		err := conn.Call(context.Background(), "ui.confirm", ConfirmParams{Title: "Unsolicited"}, nil)
		var rpcErr *RPCError
		msg := fmt.Sprintf("unexpected: %v", err)
		if errors.As(err, &rpcErr) {
			msg = "error " + rpcErr.Type()
		}
		_ = conn.Notify("ui.toast", Toast{Message: msg})
	}
}
