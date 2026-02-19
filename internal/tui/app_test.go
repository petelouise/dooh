package tui

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dooh/internal/db"
)

func TestRenderClampToWidth(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	rendered, err := m.render(80, 20)
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(strings.TrimSuffix(rendered, "\r\n"), "\r\n") {
		if got := visibleLen(line); got > 80 {
			t.Fatalf("line %d width=%d > 80: %q", i+1, got, line)
		}
	}
}

func TestColumnOrderTitleFirstIDLast(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	rows, err := m.filteredRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected at least one filtered row")
	}
	rendered, err := m.render(120, 20)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(rendered, "\r\n"), "\r\n")
	header := ""
	for _, line := range lines {
		if strings.Contains(line, "Title") && strings.Contains(line, "Priority") && strings.Contains(line, "Scheduled") && strings.Contains(line, "ID") {
			header = line
		}
	}
	if header == "" {
		t.Fatalf("missing table header in frame")
	}
	if !(strings.Index(header, "Title") < strings.Index(header, "Priority") &&
		strings.Index(header, "Priority") < strings.Index(header, "Scheduled") &&
		strings.Index(header, "Scheduled") < strings.Index(header, "ID")) {
		t.Fatalf("unexpected column order: %q", header)
	}
	if !strings.Contains(rendered, "○") {
		t.Fatalf("expected open status icon in frame")
	}
}

func TestDetailExpandCollapse(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	m.handleKey("right")
	if m.expandedID == "" {
		t.Fatalf("expected expanded id after right key")
	}
	expanded, err := m.render(120, 24)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(expanded, "projects:") {
		t.Fatalf("expected expanded detail in frame")
	}
	m.handleKey("left")
	if m.expandedID != "" {
		t.Fatalf("expected expanded id cleared after left key")
	}
	collapsed, err := m.render(120, 24)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(collapsed, "projects:") {
		t.Fatalf("expected detail to be collapsed")
	}
}

func TestDefaultStatusFilterOpen(t *testing.T) {
	sqlite := newTUIDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,updated_at) VALUES('2','t_BBBBBB','Completed row','completed','later',"+db.Quote(now)+");")
	m := testModel(sqlite)
	rendered, err := m.render(120, 22)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "Completed row") {
		t.Fatalf("completed task should be hidden by default open filter")
	}
}

func TestFuzzyFilterLiveTyping(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	m.handleKey("/")
	m.handleKey("c")
	m.handleKey("f")
	m.handleKey("t")
	rows, err := m.filteredRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected fuzzy live filter to match critical fix title")
	}
	m.handleKey("z")
	rows, err = m.filteredRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected row filtered out after non-matching key")
	}
}

func newTUIDB(t *testing.T) db.SQLite {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	sqlite := db.New(filepath.Join(t.TempDir(), "tui.db"))
	mustExec(t, sqlite, `
CREATE TABLE tasks(
  id TEXT PRIMARY KEY,
  short_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  priority TEXT NOT NULL,
  due_at TEXT,
  scheduled_at TEXT,
  updated_at TEXT,
  deleted_at TEXT
);
CREATE TABLE users(
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL
);
CREATE TABLE task_collections(
  task_id TEXT NOT NULL,
  collection_id TEXT NOT NULL
);
CREATE TABLE task_assignees(
  task_id TEXT NOT NULL,
  user_id TEXT NOT NULL
);
CREATE TABLE collections(
  id TEXT PRIMARY KEY,
  short_id TEXT NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  color_hex TEXT,
  updated_at TEXT
);
`)
	now := time.Now().UTC()
	due := now.Add(24 * time.Hour).Format(time.RFC3339)
	scheduled := now.Add(-2 * time.Hour).Format(time.RFC3339)
	updated := now.Format(time.RFC3339)
	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u1','Human Demo','active');")
	mustExec(t, sqlite, "INSERT INTO collections(id,short_id,name,kind,color_hex,updated_at) VALUES('c1','c_P1','Project Atlas','project','#7AB8FF',"+db.Quote(updated)+");")
	mustExec(t, sqlite, "INSERT INTO collections(id,short_id,name,kind,color_hex,updated_at) VALUES('c2','c_T1','Bugs','tag','#FF9AA2',"+db.Quote(updated)+");")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,due_at,scheduled_at,updated_at) VALUES('1','t_AAAAAA','Critical fix title','open','now',"+db.Quote(due)+","+db.Quote(scheduled)+","+db.Quote(updated)+");")
	mustExec(t, sqlite, "INSERT INTO task_collections(task_id,collection_id) VALUES('1','c1');")
	mustExec(t, sqlite, "INSERT INTO task_collections(task_id,collection_id) VALUES('1','c2');")
	mustExec(t, sqlite, "INSERT INTO task_assignees(task_id,user_id) VALUES('1','u1');")
	return sqlite
}

func testModel(sqlite db.SQLite) model {
	catalog := ThemeCatalog{
		Default: "sunset-pop",
		Themes: []Theme{
			{ID: "sunset-pop", Name: "Sunset Pop"},
		},
	}
	return newModel(sqlite, catalog, "sunset-pop", "", 12, time.UTC, true)
}

func visibleLen(s string) int {
	var b strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inEsc {
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				inEsc = false
			}
			continue
		}
		if ch == 0x1b {
			inEsc = true
			continue
		}
		b.WriteByte(ch)
	}
	return len(b.String())
}

func mustExec(t *testing.T, sqlite db.SQLite, sql string) {
	t.Helper()
	if err := sqlite.Exec(sql); err != nil {
		t.Fatal(err)
	}
}
