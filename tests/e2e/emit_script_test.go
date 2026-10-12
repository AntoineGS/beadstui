package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmitScript_BashAndFish(t *testing.T) {
	t.Parallel()
	bt := buildBtBinary(t)
	env := t.TempDir()

	// Ensure at least one actionable recommendation.
	writeBeads(t, env, `{"id":"A","title":"Unblocker","status":"open","priority":1,"issue_type":"task"}
{"id":"B","title":"Blocked","status":"open","priority":2,"issue_type":"task","dependencies":[{"issue_id":"B","depends_on_id":"A","type":"blocks"}]}`)

	tests := []struct {
		name        string
		formatFlag  string
		wantShebang string
		wantExtra   string
	}{
		{name: "bash", formatFlag: "bash", wantShebang: "#!/usr/bin/env bash", wantExtra: "set -euo pipefail"},
		{name: "fish", formatFlag: "fish", wantShebang: "#!/usr/bin/env fish", wantExtra: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(bt, "emit-script", "--limit=1", "--format="+tt.formatFlag)
			cmd.Dir = env
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run failed: %v\n%s", err, out)
			}
			s := string(out)
			if !strings.Contains(s, tt.wantShebang) {
				t.Fatalf("missing shebang %q:\n%s", tt.wantShebang, s)
			}
			if tt.wantExtra != "" && !strings.Contains(s, tt.wantExtra) {
				t.Fatalf("missing %q:\n%s", tt.wantExtra, s)
			}
			if tt.formatFlag == "fish" {
				if !strings.Contains(s, "begin; if pushd '") || strings.Contains(s, "pushd --") || !strings.Contains(s, "command bd 'show' 'A'") {
					t.Fatalf("missing fish-compatible route-checked bd show command for A:\n%s", s)
				}
			} else if !strings.Contains(s, "'bd' 'show' 'A'") || !strings.Contains(s, "(cd ") {
				t.Fatalf("missing POSIX route-checked bd show command for A:\n%s", s)
			}
			if !strings.Contains(s, "# Data hash:") {
				t.Fatalf("missing data hash header:\n%s", s)
			}
		})
	}
}

func TestEmitScriptFishRendersAndExecutesFishSyntax(t *testing.T) {
	bt := buildBtBinary(t)
	env := filepath.Join(t.TempDir(), "project with ' quote")
	if err := os.MkdirAll(env, 0o755); err != nil {
		t.Fatal(err)
	}
	writeBeads(t, env, `{"id":"bt-a","title":"Fish route","status":"open","priority":1,"issue_type":"task"}`)
	cmd := exec.Command(bt, "emit-script", "--limit=1", "--format=fish")
	cmd.Dir = env
	cmd.Env = append(os.Environ(), "BT_TEST_MODE=1", "BT_NO_BROWSER=1")
	script, err := cmd.Output()
	if err != nil {
		t.Fatalf("emit fish script: %v\n%s", err, script)
	}
	if strings.Contains(string(script), "(cd ") || strings.Contains(string(script), "pushd --") || !strings.Contains(string(script), "begin; if pushd '") {
		t.Fatalf("fish output does not use a fish command block and scoped directory stack:\n%s", script)
	}

	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish is unavailable; syntax/execution skipped after renderer assertion")
	}
	stubDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "bd.log")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$PWD\" \"$@\" >> \"$BT_BD_LOG\"\n"
	if err := os.WriteFile(filepath.Join(stubDir, "bd"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(t.TempDir(), "emit.fish")
	if err := os.WriteFile(scriptPath, script, 0o644); err != nil {
		t.Fatal(err)
	}
	callerDir := t.TempDir()
	run := exec.Command(fish, "-c", `set -l original_dir $PWD; source "$argv[1]"; test "$PWD" = "$original_dir"`, scriptPath)
	run.Dir = callerDir
	run.Env = append(os.Environ(), "PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"), "BT_BD_LOG="+logPath)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("fish script syntax/execution: %v\n%s", err, out)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read stub log: %v", err)
	}
	want := env + "\nshow\nbt-a\n"
	if string(logged) != want {
		t.Fatalf("fish routed command log = %q, want %q", logged, want)
	}
}
