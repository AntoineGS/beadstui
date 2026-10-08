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

func runPluginsForTest(t *testing.T, asJSON bool) (string, error) {
	t.Helper()
	var out bytes.Buffer
	prevJSON := pluginsJSON
	t.Cleanup(func() {
		pluginsJSON = prevJSON
		pluginsCmd.SetOut(nil)
	})
	pluginsCmd.SetOut(&out)
	pluginsJSON = asJSON
	err := runPlugins(pluginsCmd, nil)
	return out.String(), err
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
	out, err := runPluginsForTest(t, false)
	if err == nil {
		t.Fatal("expected an error for the invalid entry")
	}
	if !strings.Contains(out, "good") {
		t.Errorf("valid entries should still print:\n%s", out)
	}
}
