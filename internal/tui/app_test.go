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
	rendered, err := m.render(120, 20)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(rendered, "\r\n"), "\r\n")
	header := ""
	body := ""
	for _, line := range lines {
		if strings.Contains(line, "Title") && strings.Contains(line, "Priority") && strings.Contains(line, "Updated") && strings.Contains(line, "ID") {
			header = line
		}
		if strings.Contains(line, "Critical fix title") && strings.Contains(line, "t_AAAAAA") {
			body = line
		}
	}
	if header == "" {
		t.Fatalf("missing table header in frame")
	}
	if body == "" {
		t.Fatalf("missing task body row in frame")
	}
	if !(strings.Index(header, "Title") < strings.Index(header, "Priority") &&
		strings.Index(header, "Priority") < strings.Index(header, "Updated") &&
		strings.Index(header, "Updated") < strings.Index(header, "ID")) {
		t.Fatalf("unexpected column order: %q", header)
	}
	if !(strings.Index(body, "Critical fix title") < strings.Index(body, "now") &&
		strings.Index(body, "now") < strings.Index(body, "today") &&
		strings.Index(body, "today") < strings.Index(body, "t_AAAAAA")) {
		t.Fatalf("unexpected body column order: %q", body)
	}
	if !strings.Contains(body, "○") {
		t.Fatalf("expected open status icon in body row: %q", body)
	}
}

func TestDetailExpandCollapse(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	m.handleKey("right")
	expanded, err := m.render(120, 24)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(expanded, "title: Critical fix title") {
		t.Fatalf("expected expanded detail in frame")
	}
	m.handleKey("left")
	collapsed, err := m.render(120, 24)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(collapsed, "title: Critical fix title") {
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
	rendered, err := m.render(120, 22)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "Critical fix title") {
		t.Fatalf("expected fuzzy live filter to match critical fix title")
	}
	m.handleKey("z")
	rendered, err = m.render(120, 22)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "Critical fix title") {
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
CREATE TABLE task_collections(
  task_id TEXT NOT NULL,
  collection_id TEXT NOT NULL
);
CREATE TABLE collections(
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL
);
`)
	now := time.Now().UTC()
	due := now.Add(24 * time.Hour).Format(time.RFC3339)
	scheduled := now.Add(-2 * time.Hour).Format(time.RFC3339)
	updated := now.Format(time.RFC3339)
	mustExec(t, sqlite, "INSERT INTO collections(id,name) VALUES('c1','Project Atlas');")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,due_at,scheduled_at,updated_at) VALUES('1','t_AAAAAA','Critical fix title','open','now',"+db.Quote(due)+","+db.Quote(scheduled)+","+db.Quote(updated)+");")
	mustExec(t, sqlite, "INSERT INTO task_collections(task_id,collection_id) VALUES('1','c1');")
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
