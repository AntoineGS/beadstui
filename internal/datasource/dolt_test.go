package datasource

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

var registerDeferSQLDriver sync.Once

type deferSQLDriver struct{}

func (deferSQLDriver) Open(string) (driver.Conn, error) { return deferSQLConn{}, nil }

type deferSQLConn struct{}

func (deferSQLConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare unsupported")
}
func (deferSQLConn) Close() error              { return nil }
func (deferSQLConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("transactions unsupported") }
func (deferSQLConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "information_schema.columns") {
		values := make([][]driver.Value, 0, len(IssuesColumnList))
		for _, column := range IssuesColumnList {
			values = append(values, []driver.Value{"beads_project", column})
		}
		return &deferSQLRows{columns: []string{"TABLE_SCHEMA", "COLUMN_NAME"}, values: values}, nil
	}
	if strings.Contains(query, "FROM issues") || strings.Contains(query, "_global_source") {
		values := deferIssueRow()
		if strings.Contains(query, "_global_source") {
			values = append(values, "beads_project")
		}
		columns := append([]string(nil), IssuesColumnList...)
		if strings.Contains(query, "_global_source") {
			columns = append(columns, "_global_source")
		}
		return &deferSQLRows{columns: columns, values: [][]driver.Value{values}}, nil
	}
	return &deferSQLRows{columns: []string{"empty"}}, nil
}

func deferIssueRow() []driver.Value {
	values := make([]driver.Value, len(IssuesColumnList))
	values[0], values[1], values[3], values[4], values[5] = "bt-sql", "deferred SQL issue", "open", int64(1), "task"
	values[11] = time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC)
	return values
}

type deferSQLRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *deferSQLRows) Columns() []string { return r.columns }
func (r *deferSQLRows) Close() error      { return nil }
func (r *deferSQLRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func openDeferSQLTestDB(t *testing.T) *sql.DB {
	t.Helper()
	const driverName = "beadstui_defer_sql_test"
	// sql.Register panics on duplicate names, so register once per process.
	registerDeferSQLDriver.Do(func() { sql.Register(driverName, deferSQLDriver{}) })
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// bt-edi: schema-drift tolerance. When a column on the issues table is
// absent (e.g. upstream beads drops closed_by_session for the events-table
// redesign), the single-DB scan path must NULL-substitute it instead of
// silently degrading to the 8-column fallback.
func TestBuildSingleDBIssuesQuery_NullSubstitutesMissingColumns(t *testing.T) {
	available := map[string]bool{}
	for _, c := range IssuesColumnList {
		available[c] = true
	}
	delete(available, "closed_by_session")

	query := buildSingleDBIssuesQuery(available)

	if !strings.Contains(query, "NULL AS closed_by_session") {
		t.Errorf("expected NULL substitution for missing closed_by_session column.\nquery:\n%s", query)
	}
	if strings.Contains(query, "NULL AS id") {
		t.Errorf("present columns must not be NULL-substituted.\nquery:\n%s", query)
	}
	if !strings.Contains(query, "FROM issues") {
		t.Errorf("query must read from issues table.\nquery:\n%s", query)
	}
	if !strings.Contains(query, "status != 'tombstone'") {
		t.Errorf("query must filter tombstones.\nquery:\n%s", query)
	}
}

func TestBuildSingleDBIssuesQuery_AllPresent(t *testing.T) {
	available := map[string]bool{}
	for _, c := range IssuesColumnList {
		available[c] = true
	}

	query := buildSingleDBIssuesQuery(available)

	if strings.Contains(query, "NULL AS") {
		t.Errorf("no NULL substitutions expected when all columns present.\nquery:\n%s", query)
	}
	for _, col := range IssuesColumnList {
		if !strings.Contains(query, col) {
			t.Errorf("expected column %q in SELECT list.\nquery:\n%s", col, query)
		}
	}
}

func TestSQLReadersScanNonNullDeferUntil(t *testing.T) {
	want := time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC)
	db := openDeferSQLTestDB(t)
	available := make(map[string]bool, len(IssuesColumnList))
	for _, column := range IssuesColumnList {
		available[column] = true
	}
	projectReader := &DoltReader{db: db, availableCols: available}
	projectIssues, err := projectReader.LoadIssues()
	if err != nil {
		t.Fatalf("project reader LoadIssues: %v", err)
	}
	globalReader := &GlobalDoltReader{db: db, databases: []string{"beads_project"}}
	globalIssues, err := globalReader.LoadIssues()
	if err != nil {
		t.Fatalf("global reader LoadIssues: %v", err)
	}
	for name, issues := range map[string][]model.Issue{"project": projectIssues, "global": globalIssues} {
		if len(issues) != 1 || issues[0].DeferUntil == nil || !issues[0].DeferUntil.Equal(want) {
			t.Errorf("%s reader defer_until = %+v, want %s", name, issues, want)
		}
	}
}

func TestReadDoltConfig_ValidMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	metadata := `{"backend":"dolt","dolt_mode":"server","dolt_database":"beads_dotfiles"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, ok := ReadDoltConfig(tmpDir)
	if !ok {
		t.Fatal("Expected ReadDoltConfig to return true for dolt backend")
	}

	if cfg.Host != "127.0.0.1" {
		t.Errorf("Expected host 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.Port != 3307 {
		t.Errorf("Expected default port 3307, got %d", cfg.Port)
	}
	if cfg.Database != "beads_dotfiles" {
		t.Errorf("Expected database beads_dotfiles, got %s", cfg.Database)
	}
	if cfg.User != "root" {
		t.Errorf("Expected user root, got %s", cfg.User)
	}
}

func TestReadDoltConfig_CustomPort(t *testing.T) {
	tmpDir := t.TempDir()

	metadata := `{"backend":"dolt","dolt_database":"mydb"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	doltDir := filepath.Join(tmpDir, "dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}

	doltCfg := "listener:\n  port: 3309\n"
	if err := os.WriteFile(filepath.Join(doltDir, "config.yaml"), []byte(doltCfg), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, ok := ReadDoltConfig(tmpDir)
	if !ok {
		t.Fatal("Expected ReadDoltConfig to return true")
	}
	if cfg.Port != 3309 {
		t.Errorf("Expected custom port 3309, got %d", cfg.Port)
	}
	if cfg.Database != "mydb" {
		t.Errorf("Expected database mydb, got %s", cfg.Database)
	}
}

func TestReadDoltConfig_PortFileOverridesConfigYaml(t *testing.T) {
	tmpDir := t.TempDir()

	metadata := `{"backend":"dolt","dolt_database":"bv"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	// config.yaml says 13729 (stale hash-derived port)
	doltDir := filepath.Join(tmpDir, "dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(doltDir, "config.yaml"), []byte("listener:\n  port: 13729\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// dolt-server.port says 3307 (actual running server)
	if err := os.WriteFile(filepath.Join(tmpDir, "dolt-server.port"), []byte("3307\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, ok := ReadDoltConfig(tmpDir)
	if !ok {
		t.Fatal("Expected ReadDoltConfig to return true")
	}
	if cfg.Port != 3307 {
		t.Errorf("Port file should override config.yaml: want 3307, got %d", cfg.Port)
	}
}

func TestReadDoltConfig_DefaultDatabase(t *testing.T) {
	tmpDir := t.TempDir()

	// No dolt_database field - should default to "beads"
	metadata := `{"backend":"dolt","dolt_mode":"server"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, ok := ReadDoltConfig(tmpDir)
	if !ok {
		t.Fatal("Expected ReadDoltConfig to return true")
	}
	if cfg.Database != "beads" {
		t.Errorf("Expected default database 'beads', got %s", cfg.Database)
	}
}

func TestReadDoltConfig_NotDoltBackend(t *testing.T) {
	tmpDir := t.TempDir()

	metadata := `{"database":"beads.db","backend":"sqlite"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	_, ok := ReadDoltConfig(tmpDir)
	if ok {
		t.Error("Expected ReadDoltConfig to return false for non-dolt backend")
	}
}

func TestReadDoltConfig_NoMetadataFile(t *testing.T) {
	tmpDir := t.TempDir()

	_, ok := ReadDoltConfig(tmpDir)
	if ok {
		t.Error("Expected ReadDoltConfig to return false when metadata.json is missing")
	}
}

func TestReadDoltConfig_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, ok := ReadDoltConfig(tmpDir)
	if ok {
		t.Error("Expected ReadDoltConfig to return false for invalid JSON")
	}
}

func TestDoltConfig_DSN(t *testing.T) {
	cfg := DoltConfig{
		Host:     "127.0.0.1",
		Port:     3307,
		Database: "beads_dotfiles",
		User:     "root",
	}

	expected := "root@tcp(127.0.0.1:3307)/beads_dotfiles?parseTime=true&timeout=2s"
	if got := cfg.DSN(); got != expected {
		t.Errorf("DSN mismatch:\n  got:  %s\n  want: %s", got, expected)
	}
}

func TestDoltConfig_DSN_CustomValues(t *testing.T) {
	cfg := DoltConfig{
		Host:     "192.168.1.100",
		Port:     3309,
		Database: "myproject",
		User:     "admin",
	}

	expected := "admin@tcp(192.168.1.100:3309)/myproject?parseTime=true&timeout=2s"
	if got := cfg.DSN(); got != expected {
		t.Errorf("DSN mismatch:\n  got:  %s\n  want: %s", got, expected)
	}
}

// Note: pre-ADR-003 tests for discoverDoltSources, PriorityDolt vs JSONL,
// SelectBestSource_DoltWinsTiebreak, and BuildSelectionReason_Dolt were
// removed when their symbols were collapsed in Phase 2 (bt-okpn). The
// per-project Dolt resolution path is now exercised by TestDiscoverSource_*
// in source_test.go.
