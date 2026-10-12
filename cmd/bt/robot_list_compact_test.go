package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// listFixtureBeads exercises every slot CompactIssue cares about: core fields,
// populated session columns, a parent-child edge, a blocking edge whose target
// is closed, and a relates-to edge.
const listFixtureBeads = `{"id":"epic-1","title":"Epic parent","status":"open","priority":0,"issue_type":"epic","labels":["area:cli"],"description":"epic description that must never appear in compact output","created_at":"2026-04-15T10:00:00Z","updated_at":"2026-04-20T10:00:00Z"}
{"id":"blk-1","title":"Closed blocker","status":"closed","priority":1,"issue_type":"task","description":"resolved","created_at":"2026-04-10T10:00:00Z","updated_at":"2026-04-12T10:00:00Z","closed_at":"2026-04-12T10:00:00Z"}
{"id":"child-1","title":"Child with sessions","description":"child description body","design":"child design body","acceptance_criteria":"child criteria","notes":"child notes","status":"open","priority":1,"issue_type":"task","labels":["area:cli"],"created_by_session":"cc-sess-a","claimed_by_session":"cc-sess-a","dependencies":[{"issue_id":"child-1","depends_on_id":"epic-1","type":"parent-child"},{"issue_id":"child-1","depends_on_id":"blk-1","type":"blocks"},{"issue_id":"child-1","depends_on_id":"epic-1","type":"related"}],"created_at":"2026-04-16T10:00:00Z","updated_at":"2026-04-18T10:00:00Z"}
`

// setupListFixture writes a tiny project under t.TempDir() for the robot
// list bellwether contract tests and returns the project directory.
func setupListFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.jsonl"), []byte(listFixtureBeads), 0o644); err != nil {
		t.Fatalf("write beads: %v", err)
	}
	return dir
}

// runListFixture invokes the built bt binary in the fixture project.
func runListFixture(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	exe := buildTestBinary(t)
	cmd := exec.Command(exe, append([]string{"robot", "list"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BT_TEST_MODE=1", "BT_NO_BROWSER=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bt robot list %v failed: %v\nout=%s", args, err, string(out))
	}
	return out
}

// TestRobotListCompactDefault verifies the compact shape is the default and
// that its wire payload contains none of the fat body fields.
func TestRobotListCompactDefault(t *testing.T) {
	dir := setupListFixture(t)
	out := runListFixture(t, dir)

	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("parse: %v\nraw=%s", err, string(out))
	}

	if schema, _ := payload["schema"].(string); schema != "compact.v1" {
		t.Errorf("envelope.schema = %q, want compact.v1", schema)
	}

	issues, ok := payload["issues"].([]any)
	if !ok {
		t.Fatalf("issues is not an array: %T", payload["issues"])
	}
	if len(issues) == 0 {
		t.Fatalf("expected at least one issue in compact output")
	}

	forbidden := []string{"description", "design", "acceptance_criteria", "notes", "comments", "close_reason"}
	required := []string{"id", "title", "status", "priority", "issue_type",
		"blockers_count", "unblocks_count", "children_count", "relates_count", "is_blocked",
		"created_at", "updated_at"}
	for i, raw := range issues {
		obj, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("issues[%d] is not an object: %T", i, raw)
		}
		for _, k := range forbidden {
			if _, has := obj[k]; has {
				t.Errorf("issues[%d].%s leaked into compact output", i, k)
			}
		}
		for _, k := range required {
			if _, has := obj[k]; !has {
				t.Errorf("issues[%d] missing required compact field %s", i, k)
			}
		}
	}
}

func TestRobotLabelScopeKeepsCandidatesNarrowAndEmpty(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `{"id":"bt-a","title":"Labeled candidate","status":"open","priority":1,"issue_type":"task","labels":["area:cli"],"dependencies":[{"depends_on_id":"bt-b","type":"blocks"}]}
{"id":"bt-b","title":"Unlabeled blocker","status":"open","priority":1,"issue_type":"task"}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.jsonl"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := buildTestBinary(t)
	cmd := exec.Command(exe, "robot", "next", "--label", "area:cli")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BT_TEST_MODE=1", "BT_NO_BROWSER=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("robot next label scope failed: %v\n%s", err, out)
	}
	var next struct {
		Message string `json:"message"`
		ID      string `json:"id"`
	}
	if err := json.Unmarshal(out, &next); err != nil {
		t.Fatalf("decode robot next: %v\n%s", err, out)
	}
	if next.Message != "No actionable items available" {
		t.Fatalf("label scope expanded to hidden blocker candidate: %s", out)
	}
	list := exec.Command(exe, "robot", "list", "--label", "area:cli")
	list.Dir = dir
	list.Env = cmd.Env
	listOut, err := list.Output()
	if err != nil {
		t.Fatalf("robot list label scope failed: %v\n%s", err, listOut)
	}
	var scoped struct {
		Issues []struct {
			ID string `json:"id"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(listOut, &scoped); err != nil {
		t.Fatalf("decode scoped list: %v\n%s", err, listOut)
	}
	if len(scoped.Issues) != 1 || scoped.Issues[0].ID != "bt-a" {
		t.Fatalf("list label scope expanded candidates: %+v", scoped.Issues)
	}
	bqlList := exec.Command(exe, "robot", "list", "--bql", "blocked = false")
	bqlList.Dir = dir
	bqlList.Env = cmd.Env
	bqlOut, err := bqlList.Output()
	if err != nil {
		t.Fatalf("robot list computed BQL failed: %v\n%s", err, bqlOut)
	}
	var bqlPayload struct {
		Issues []struct {
			ID string `json:"id"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(bqlOut, &bqlPayload); err != nil {
		t.Fatalf("decode computed BQL list: %v\n%s", err, bqlOut)
	}
	if len(bqlPayload.Issues) != 1 || bqlPayload.Issues[0].ID != "bt-b" {
		t.Fatalf("computed blocked=false selected %+v; want only ready hidden dependency", bqlPayload.Issues)
	}
	recipeNext := exec.Command(exe, "robot", "next", "--recipe", "actionable")
	recipeNext.Dir = dir
	recipeNext.Env = cmd.Env
	recipeOut, err := recipeNext.Output()
	if err != nil {
		t.Fatalf("robot next actionable recipe failed: %v\n%s", err, recipeOut)
	}
	if err := json.Unmarshal(recipeOut, &next); err != nil {
		t.Fatalf("decode actionable recipe next: %v\n%s", err, recipeOut)
	}
	if next.ID != "bt-b" {
		t.Fatalf("actionable recipe pick = %q, want bt-b: %s", next.ID, recipeOut)
	}

	empty := exec.Command(exe, "robot", "next", "--label", "nonexistent")
	empty.Dir = dir
	empty.Env = cmd.Env
	emptyOut, err := empty.Output()
	if err != nil {
		t.Fatalf("robot next empty label scope failed: %v\n%s", err, emptyOut)
	}
	if err := json.Unmarshal(emptyOut, &next); err != nil {
		t.Fatalf("decode empty scoped next: %v\n%s", err, emptyOut)
	}
	if next.Message != "No actionable items available" {
		t.Fatalf("empty label scope fell back to full corpus: %s", emptyOut)
	}
	emptyList := exec.Command(exe, "robot", "list", "--label", "nonexistent")
	emptyList.Dir = dir
	emptyList.Env = cmd.Env
	listOut, err = emptyList.Output()
	if err != nil {
		t.Fatalf("robot list empty label scope failed: %v\n%s", err, listOut)
	}
	var listPayload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(listOut, &listPayload); err != nil {
		t.Fatalf("decode empty scoped list: %v\n%s", err, listOut)
	}
	if listPayload.Count != 0 {
		t.Fatalf("robot list empty label scope returned %d candidates", listPayload.Count)
	}
}

func TestEmitScriptDoesNotSuggestClaimsForNonclaimableIssues(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `{"id":"bt-flight","title":"Already assigned","status":"in_progress","assignee":"worker","priority":1,"issue_type":"task"}
{"id":"bt-unknown","title":"Unknown gate","status":"open","priority":1,"issue_type":"task","dependencies":[{"depends_on_id":"bt-missing","type":"blocks"}]}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.jsonl"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(buildTestBinary(t), "emit-script")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BT_TEST_MODE=1", "BT_NO_BROWSER=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("emit-script failed: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "bd update") || strings.Contains(string(out), "# To claim:") {
		t.Fatalf("emit-script suggested an unclaimable item:\n%s", out)
	}
	if !strings.Contains(string(out), "No actionable recommendations available") {
		t.Fatalf("emit-script omitted no-pick output:\n%s", out)
	}
}

func TestRobotListActionableRecipeUsesFullReadinessAndScopesCandidates(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `{"id":"bt-open","title":"Open blocker","status":"open","issue_type":"task"}
{"id":"bt-closed-gate","title":"Closed gate","status":"closed","issue_type":"task"}
{"id":"bt-tomb-gate","title":"Tombstone gate","status":"tombstone","issue_type":"task"}
{"id":"cass-x","title":"Closed alias gate","status":"closed","issue_type":"task"}
{"id":"cass-y","title":"Open alias gate","status":"open","issue_type":"task"}
{"id":"bt-parent","title":"Parent","status":"open","issue_type":"epic","dependencies":[{"depends_on_id":"bt-parent-gate","type":"blocks"}]}
{"id":"bt-parent-gate","title":"Parent gate","status":"open","issue_type":"task"}
{"id":"bt-open-dep","title":"Open dependency candidate","status":"open","issue_type":"task","labels":["area:cli"],"dependencies":[{"depends_on_id":"bt-open","type":"blocks"}]}
{"id":"bt-missing-dep","title":"Missing dependency candidate","status":"open","issue_type":"task","labels":["area:cli"],"dependencies":[{"depends_on_id":"bt-absent","type":"blocks"}]}
{"id":"bt-closed-dep","title":"Closed dependency candidate","status":"open","issue_type":"task","labels":["area:cli"],"dependencies":[{"depends_on_id":"bt-closed-gate","type":"blocks"}]}
{"id":"bt-tomb-dep","title":"Tombstone dependency candidate","status":"open","issue_type":"task","labels":["area:cli"],"dependencies":[{"depends_on_id":"bt-tomb-gate","type":"blocks"}]}
{"id":"bt-alias-closed","title":"Closed external alias candidate","status":"open","issue_type":"task","labels":["area:cli"],"dependencies":[{"depends_on_id":"external:cass:x","type":"blocks"}]}
{"id":"bt-alias-open","title":"Open external alias candidate","status":"open","issue_type":"task","labels":["area:cli"],"dependencies":[{"depends_on_id":"external:cass:y","type":"blocks"}]}
{"id":"bt-child","title":"Parent gate candidate","status":"open","issue_type":"task","labels":["area:cli"],"dependencies":[{"depends_on_id":"bt-parent","type":"parent-child"}]}
{"id":"bt-deferred","title":"Deferred candidate","status":"open","issue_type":"task","labels":["area:cli"],"defer_until":"2099-01-01T00:00:00Z"}
{"id":"bt-ready","title":"Ready candidate","status":"open","issue_type":"task","labels":["area:cli"]}
{"id":"other-ready","title":"Other source candidate","status":"open","issue_type":"task","labels":["area:cli"]}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "beads.jsonl"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := buildTestBinary(t)
	cmd := exec.Command(exe, "robot", "list", "--source", "bt", "--label", "area:cli", "--bql", "status=open", "--recipe", "actionable")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BT_TEST_MODE=1", "BT_NO_BROWSER=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("robot list actionable recipe failed: %v\n%s", err, out)
	}
	var payload struct {
		Count  int `json:"count"`
		Issues []struct {
			ID string `json:"id"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("decode actionable recipe list: %v\n%s", err, out)
	}
	got := make([]string, 0, len(payload.Issues))
	for _, issue := range payload.Issues {
		got = append(got, issue.ID)
	}
	sort.Strings(got)
	want := []string{"bt-alias-closed", "bt-closed-dep", "bt-ready", "bt-tomb-dep"}
	if !reflect.DeepEqual(got, want) || payload.Count != len(want) {
		t.Fatalf("actionable recipe list IDs=%v count=%d, want %v", got, payload.Count, want)
	}
	repoScoped := exec.Command(exe, "robot", "list", "--repo", "bt", "--recipe", "actionable")
	repoScoped.Dir = dir
	repoScoped.Env = cmd.Env
	repoOut, err := repoScoped.Output()
	if err != nil {
		t.Fatalf("repo-scoped actionable list failed: %v\n%s", err, repoOut)
	}
	var repoPayload struct {
		Issues []struct {
			ID string `json:"id"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(repoOut, &repoPayload); err != nil {
		t.Fatalf("decode repo-scoped list: %v\n%s", err, repoOut)
	}
	for _, issue := range repoPayload.Issues {
		if strings.HasPrefix(issue.ID, "cass-") || strings.HasPrefix(issue.ID, "other-") {
			t.Fatalf("--repo bt leaked candidate %q: %s", issue.ID, repoOut)
		}
	}
	if len(repoPayload.Issues) == 0 {
		t.Fatalf("--repo bt removed every actionable candidate: %s", repoOut)
	}
	triageCmd := exec.Command(exe, "robot", "triage", "--source", "bt", "--label", "area:cli")
	triageCmd.Dir = dir
	triageCmd.Env = cmd.Env
	triageOut, err := triageCmd.Output()
	if err != nil {
		t.Fatalf("scoped triage failed: %v\n%s", err, triageOut)
	}
	var triagePayload struct {
		Triage struct {
			Meta struct {
				IssueCount int `json:"issue_count"`
			} `json:"meta"`
		} `json:"triage"`
	}
	if err := json.Unmarshal(triageOut, &triagePayload); err != nil {
		t.Fatalf("decode scoped triage: %v\n%s", err, triageOut)
	}
	if triagePayload.Triage.Meta.IssueCount != 9 {
		t.Fatalf("triage --source bt --label area:cli issue_count = %d, want 9", triagePayload.Triage.Meta.IssueCount)
	}
	emptyTriage := exec.Command(exe, "robot", "triage", "--source", "bt", "--label", "not-present")
	emptyTriage.Dir = dir
	emptyTriage.Env = cmd.Env
	emptyOut, err := emptyTriage.Output()
	if err != nil {
		t.Fatalf("empty scoped triage failed: %v\n%s", err, emptyOut)
	}
	if err := json.Unmarshal(emptyOut, &triagePayload); err != nil {
		t.Fatalf("decode empty scoped triage: %v\n%s", err, emptyOut)
	}
	if triagePayload.Triage.Meta.IssueCount != 0 {
		t.Fatalf("empty triage label scope issue_count = %d, want 0", triagePayload.Triage.Meta.IssueCount)
	}
	unknown := exec.Command(exe, "robot", "list", "--recipe", "not-a-recipe")
	unknown.Dir = dir
	unknown.Env = cmd.Env
	if out, err := unknown.CombinedOutput(); err == nil || !strings.Contains(string(out), "unknown recipe") {
		t.Fatalf("unknown recipe was not rejected: err=%v output=%s", err, out)
	}
}

// TestRobotListFullPreservesBodies verifies --full restores the pre-compact
// shape: body fields present, envelope.schema omitted (omitempty).
func TestRobotListFullPreservesBodies(t *testing.T) {
	dir := setupListFixture(t)
	out := runListFixture(t, dir, "--full")

	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("parse: %v\nraw=%s", err, string(out))
	}
	if _, has := payload["schema"]; has {
		t.Errorf("envelope.schema must be omitted in full mode, got %v", payload["schema"])
	}
	issues, _ := payload["issues"].([]any)

	sawBody := false
	for _, raw := range issues {
		obj, _ := raw.(map[string]any)
		if id, _ := obj["id"].(string); id == "child-1" {
			if d, ok := obj["description"].(string); ok && strings.Contains(d, "child description body") {
				sawBody = true
			}
		}
	}
	if !sawBody {
		t.Errorf("--full output missing expected description body; full payload = %s", string(out))
	}
}

// TestRobotListShapeEnvAndAliases — env var and --compact/--full aliases
// pick the same shape as explicit --shape.
func TestRobotListShapeEnvAndAliases(t *testing.T) {
	dir := setupListFixture(t)

	cases := []struct {
		name       string
		env        map[string]string
		args       []string
		wantSchema string
	}{
		{"env compact", map[string]string{"BT_OUTPUT_SHAPE": "compact"}, nil, "compact.v1"},
		{"env full", map[string]string{"BT_OUTPUT_SHAPE": "full"}, nil, ""},
		{"--compact alias", nil, []string{"--compact"}, "compact.v1"},
		{"--full alias", nil, []string{"--full"}, ""},
		{"--shape compact", nil, []string{"--shape=compact"}, "compact.v1"},
		{"--shape full", nil, []string{"--shape=full"}, ""},
		{"cli overrides env", map[string]string{"BT_OUTPUT_SHAPE": "full"}, []string{"--compact"}, "compact.v1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exe := buildTestBinary(t)
			cmd := exec.Command(exe, append([]string{"robot", "list"}, tc.args...)...)
			cmd.Dir = dir
			env := append(os.Environ(), "BT_TEST_MODE=1", "BT_NO_BROWSER=1")
			for k, v := range tc.env {
				env = append(env, k+"="+v)
			}
			cmd.Env = env
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("run failed: %v out=%s", err, out)
			}
			var payload map[string]any
			if err := json.Unmarshal(out, &payload); err != nil {
				t.Fatalf("parse: %v", err)
			}
			got, _ := payload["schema"].(string)
			if got != tc.wantSchema {
				t.Errorf("schema = %q, want %q", got, tc.wantSchema)
			}
		})
	}
}

// TestRobotListCompactFields validates the semantic counts (is_blocked,
// parent_id, children_count, blockers_count, relates_count, session bridge)
// on the fixture beads.
func TestRobotListCompactFields(t *testing.T) {
	dir := setupListFixture(t)
	out := runListFixture(t, dir)

	var payload struct {
		Issues []map[string]any `json:"issues"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("parse: %v", err)
	}
	byID := map[string]map[string]any{}
	for _, iss := range payload.Issues {
		id, _ := iss["id"].(string)
		byID[id] = iss
	}

	epic := byID["epic-1"]
	if epic == nil {
		t.Fatalf("epic-1 missing from compact output")
	}
	if c, _ := epic["children_count"].(float64); c != 1 {
		t.Errorf("epic-1.children_count = %v, want 1", c)
	}

	child := byID["child-1"]
	if child == nil {
		t.Fatalf("child-1 missing from compact output")
	}
	if pid, _ := child["parent_id"].(string); pid != "epic-1" {
		t.Errorf("child-1.parent_id = %q, want epic-1", pid)
	}
	if bc, _ := child["blockers_count"].(float64); bc != 1 {
		t.Errorf("child-1.blockers_count = %v, want 1", bc)
	}
	if rc, _ := child["relates_count"].(float64); rc != 1 {
		t.Errorf("child-1.relates_count = %v, want 1", rc)
	}
	// blk-1 is closed, so child-1.is_blocked must be false.
	if blocked, _ := child["is_blocked"].(bool); blocked {
		t.Errorf("child-1.is_blocked = true, want false (blocker is closed)")
	}
	if cs, _ := child["created_by_session"].(string); cs != "cc-sess-a" {
		t.Errorf("child-1.created_by_session = %q, want cc-sess-a", cs)
	}
}

// TestRobotListFullIssuesKeys captures the keys present in a --full compact
// regression: when this test starts asserting different keys, it tells us
// the full-mode wire shape has drifted.
func TestRobotListFullIssuesKeys(t *testing.T) {
	dir := setupListFixture(t)
	out := runListFixture(t, dir, "--full")

	var payload struct {
		Issues []map[string]any `json:"issues"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(payload.Issues) == 0 {
		t.Fatalf("expected issues in --full output")
	}

	// Collect union of keys across all issues.
	keys := map[string]struct{}{}
	for _, iss := range payload.Issues {
		for k := range iss {
			keys[k] = struct{}{}
		}
	}

	required := []string{"id", "title", "status", "priority", "issue_type", "description", "created_at", "updated_at"}
	for _, k := range required {
		if _, has := keys[k]; !has {
			var sorted []string
			for k := range keys {
				sorted = append(sorted, k)
			}
			sort.Strings(sorted)
			t.Errorf("--full issue missing key %q; saw keys=%v", k, sorted)
		}
	}
}
