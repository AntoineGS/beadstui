package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePluginsConfig(t *testing.T, yaml string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "bt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

const pluginsTestConfig = `plugins:
  - name: example
    command: ["sh", "-c", "true"]
    keys:
      dispatch: D
      jump: none
  - name: other
    enabled: false
    command: ["bt-no-such-plugin-binary", "run"]
`

// probePluginConfig is a plugin that answers initialize with a manifest
// declaring action dispatch on key j (bound by bt), then waits for stdin to
// close.
const probePluginConfig = `plugins:
  - name: example
    command: ["sh", "-c", "read -r line; id=$(printf '%s' \"$line\" | sed 's/.*\"id\":\\([0-9]*\\).*/\\1/'); printf '{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"protocolVersion\":1,\"name\":\"example\",\"version\":\"1.2.3\",\"actions\":[{\"id\":\"dispatch\",\"label\":\"Dispatch\",\"key\":\"j\"}]}}\\n' \"$id\"; cat >/dev/null"]
  - name: broken
    command: ["sh", "-c", "echo boom >&2; exit 3"]
  - name: other
    enabled: false
    command: ["bt-no-such-plugin-binary", "run"]
`

func runPluginsForTest(t *testing.T, asJSON bool) (string, error) {
	return runPluginsWith(t, asJSON, false)
}

func runPluginsWith(t *testing.T, asJSON, noStart bool) (string, error) {
	t.Helper()
	var out bytes.Buffer
	prevJSON, prevNoStart := pluginsJSON, pluginsNoStart
	t.Cleanup(func() {
		pluginsJSON, pluginsNoStart = prevJSON, prevNoStart
		pluginsCmd.SetOut(nil)
	})
	pluginsCmd.SetOut(&out)
	pluginsJSON, pluginsNoStart = asJSON, noStart
	err := runPlugins(pluginsCmd, nil)
	return out.String(), err
}

func TestPluginsCmd_ProbeJSON(t *testing.T) {
	writePluginsConfig(t, probePluginConfig)
	out, err := runPluginsForTest(t, true)
	if err != nil {
		t.Fatal(err)
	}
	var got []pluginInfo
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 3 {
		t.Fatalf("got %d plugins, want 3: %s", len(got), out)
	}
	ex := got[0]
	if ex.State != "active" || ex.Version != "1.2.3" || ex.Restarts == nil || *ex.Restarts != 0 || ex.LastError != "" {
		t.Errorf("example = %+v", ex)
	}
	if len(ex.Conflicts) != 1 || ex.Conflicts[0].Key != "j" || ex.Conflicts[0].Action != "dispatch" ||
		!strings.HasPrefix(ex.Conflicts[0].Reason, "bt binds it in list") {
		t.Errorf("example conflicts = %+v", ex.Conflicts)
	}
	if b := got[1]; b.State != "failed" || !strings.Contains(b.LastError, "boom") || *b.Restarts != 0 {
		t.Errorf("broken = %+v", b)
	}
	if o := got[2]; o.State != "disabled" {
		t.Errorf("other = %+v", o)
	}
}

func TestPluginsCmd_ProbeText(t *testing.T) {
	writePluginsConfig(t, probePluginConfig)
	out, err := runPluginsForTest(t, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"state: active  version: 1.2.3",
		"key j (dispatch): bt binds it in list",
		"state: failed",
		"error: exited",
		"boom",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPluginsCmd_NoStart(t *testing.T) {
	writePluginsConfig(t, probePluginConfig)
	out, err := runPluginsWith(t, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"state"`) || strings.Contains(out, `"restarts"`) {
		t.Errorf("--no-start reported runtime status:\n%s", out)
	}
}

func TestPluginsCmd_Text(t *testing.T) {
	writePluginsConfig(t, pluginsTestConfig)
	out, err := runPluginsForTest(t, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"example", "enabled", "sh -c true", "dispatch=D", "jump=none",
		"other", "disabled", "not found",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPluginsCmd_JSON(t *testing.T) {
	writePluginsConfig(t, pluginsTestConfig)
	out, err := runPluginsForTest(t, true)
	if err != nil {
		t.Fatal(err)
	}
	var got []pluginInfo
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("got %d plugins, want 2: %s", len(got), out)
	}
	if got[0].Name != "example" || !got[0].Enabled || got[0].Binary == "" || got[0].Keys["jump"] != "none" {
		t.Errorf("unexpected first plugin: %+v", got[0])
	}
	if got[1].Enabled || got[1].Binary != "" {
		t.Errorf("unexpected second plugin: %+v", got[1])
	}
}

func TestPluginsCmd_NoConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	out, err := runPluginsForTest(t, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no plugins configured") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestPluginsCmd_ConfigErrorFails(t *testing.T) {
	writePluginsConfig(t, `plugins:
  - name: good
    command: ["sh"]
  - name: Bad Name
    command: ["sh"]
`)
	out, err := runPluginsWith(t, false, true)
	if err == nil {
		t.Fatal("expected an error for the invalid entry")
	}
	if !strings.Contains(out, "good") {
		t.Errorf("valid entries should still print:\n%s", out)
	}
}
