package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := DefaultConfigPath()
	if err != nil || got != filepath.Join(home, ".config", "bt", "config.yaml") {
		t.Fatalf("DefaultConfigPath = %q, %v", got, err)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	cfgs, err := LoadConfig(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil || cfgs != nil {
		t.Fatalf("missing file: got %v, %v; want nil, nil", cfgs, err)
	}
}

func TestLoadConfigParsesPlugins(t *testing.T) {
	path := writeConfig(t, `
experimental:
  background_mode: true
plugins:
  - name: example
    command: ["example", "plugin"]
    env: {EXAMPLE_MODE: test}
    options:
      roots: ["~/src"]
    keys:
      dispatch: X
      jump: none
  - name: other
    command: [other]
    enabled: false
`)
	cfgs, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfgs) != 2 {
		t.Fatalf("got %d plugins, want 2", len(cfgs))
	}
	ex := cfgs[0]
	if ex.Name != "example" || strings.Join(ex.Command, " ") != "example plugin" {
		t.Errorf("example: %+v", ex)
	}
	if ex.Env["EXAMPLE_MODE"] != "test" || ex.Keys["dispatch"] != "X" || ex.Keys["jump"] != "none" {
		t.Errorf("example env/keys: %+v", ex)
	}
	if roots, ok := ex.Options["roots"].([]any); !ok || len(roots) != 1 || roots[0] != "~/src" {
		t.Errorf("example options: %#v", ex.Options)
	}
	if !ex.IsEnabled() {
		t.Error("enabled unset must mean enabled")
	}
	if cfgs[1].Name != "other" || cfgs[1].IsEnabled() {
		t.Errorf("other: want disabled, got %+v", cfgs[1])
	}
}

func TestLoadConfigInvalidName(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, "plugins:\n  - name: Bad-Name\n    command: [x]\n"))
	if err == nil || !strings.Contains(err.Error(), "Bad-Name") {
		t.Fatalf("want error naming Bad-Name, got %v", err)
	}
}

func TestLoadConfigEmptyCommand(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, "plugins:\n  - name: example\n"))
	if err == nil || !strings.Contains(err.Error(), "command") {
		t.Fatalf("want empty command error, got %v", err)
	}
}

func TestLoadConfigDuplicateName(t *testing.T) {
	cfgs, err := LoadConfig(writeConfig(t, `
plugins:
  - name: example
    command: [first]
  - name: example
    command: [second]
`))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("want duplicate error, got %v", err)
	}
	if len(cfgs) != 1 || cfgs[0].Command[0] != "first" {
		t.Fatalf("want only the first entry, got %+v", cfgs)
	}
}
