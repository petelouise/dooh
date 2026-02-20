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
		if strings.Contains(line, "Title") && strings.Contains(line, "Priority") && strings.Contains(line, "Scheduled") {
			header = line
		}
	}
	if header == "" {
		t.Fatalf("missing table header in frame")
	}
	if !(strings.Index(header, "Title") < strings.Index(header, "Priority") &&
		strings.Index(header, "Priority") < strings.Index(header, "Scheduled")) {
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
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,updated_at) VALUES('3','t_CCCCCC','Completed row','completed','later',"+db.Quote(now)+");")
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

func TestTagFilterMultiAnd(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	m.filters.Tags = []string{"Bugs", "Deep Work"}
	rows, err := m.filteredRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no rows for strict multi-tag AND, got %d", len(rows))
	}
	m.filters.Tags = []string{"Bugs"}
	rows, err = m.filteredRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected rows for single tag")
	}
}

func TestTodayModeMineVsAll(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	m.view = "today"
	m.currentUserHint = "human"
	m.filters.TodayMode = "mine"
	panelMine, err := m.render(120, 24)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(panelMine, "Write deep focus memo") {
		t.Fatalf("mine mode should hide non-matching assignee tasks")
	}
	m.filters.TodayMode = "all"
	panelAll, err := m.render(120, 24)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(panelAll, "Write deep focus memo") {
		t.Fatalf("all mode should show all scheduled today tasks")
	}
}

func TestFilterFocusTabMoves(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	start := m.filterFocus
	m.handleKey("tab")
	if m.filterFocus == start {
		t.Fatalf("expected tab to advance filter focus")
	}
	m.handleKey("shift_tab")
	if m.filterFocus != start {
		t.Fatalf("expected shift_tab to restore filter focus")
	}
}

func TestFooterHotkeysAlwaysRendered(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	rendered, err := m.render(80, 18)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "keys: arrows") {
		t.Fatalf("expected footer hotkey hint line in rendered output")
	}
}

func TestEnterOnProjectViewDrillsToScopedTasks(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	m.view = "projects"
	m.handleKey("enter")
	if m.view != "tasks" {
		t.Fatalf("expected enter to drill into tasks view, got %s", m.view)
	}
	if m.filters.ScopeKind != "project" {
		t.Fatalf("expected project scope after enter, got %s", m.filters.ScopeKind)
	}
	if m.filters.ScopeID == "" {
		t.Fatalf("expected non-empty project scope id")
	}
}

func TestQuickFilterTokens(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	m.filters.Text = "#Bugs @Human"
	rows, err := m.filteredRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for #Bugs @Human, got %d", len(rows))
	}
	if rows[0].Title != "Critical fix title" {
		t.Fatalf("unexpected row: %s", rows[0].Title)
	}
}

func TestQuickFilterOverdueToken(t *testing.T) {
	sqlite := newTUIDB(t)
	now := time.Now().UTC()
	overdueDue := now.Add(-48 * time.Hour).Format(time.RFC3339)
	mustExec(t, sqlite, "UPDATE tasks SET due_at="+db.Quote(overdueDue)+" WHERE id='1';")
	m := testModel(sqlite)
	if !parseQuickFilter("!overdue").Overdue {
		t.Fatalf("expected !overdue token to parse as overdue")
	}
	m.filters.Text = "!overdue"
	rows, err := m.filteredRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected overdue rows")
	}
	for _, r := range rows {
		if !isOverdue(r, time.Now(), time.UTC) {
			t.Fatalf("unexpected non-overdue row in !overdue result: %s", r.Title)
		}
	}
}

func TestSortPriorityAndScheduled(t *testing.T) {
	sqlite := newTUIDB(t)
	m := testModel(sqlite)
	m.filters.Sort = "priority"
	rows, err := m.filteredRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 rows")
	}
	if rows[0].Priority != "now" {
		t.Fatalf("expected priority sort to place now first, got %s", rows[0].Priority)
	}

	m.filters.Sort = "scheduled"
	rows, err = m.filteredRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 rows")
	}
	firstScheduled, _ := parseTime(rows[0].Scheduled)
	secondScheduled, _ := parseTime(rows[1].Scheduled)
	if firstScheduled.After(secondScheduled) {
		t.Fatalf("expected scheduled sort ascending, got %s then %s", rows[0].Scheduled, rows[1].Scheduled)
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
  deleted_at TEXT,
  updated_at TEXT
);
`)
	now := time.Now().UTC()
	due := now.Add(24 * time.Hour).Format(time.RFC3339)
	scheduled := now.Add(-2 * time.Hour).Format(time.RFC3339)
	updated := now.Format(time.RFC3339)
	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u1','Human Demo','active');")
	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u2','Agent Demo','active');")
	mustExec(t, sqlite, "INSERT INTO collections(id,short_id,name,kind,color_hex,updated_at) VALUES('c1','c_P1','Project Atlas','project','#7AB8FF',"+db.Quote(updated)+");")
	mustExec(t, sqlite, "INSERT INTO collections(id,short_id,name,kind,color_hex,updated_at) VALUES('c2','c_T1','Bugs','tag','#FF9AA2',"+db.Quote(updated)+");")
	mustExec(t, sqlite, "INSERT INTO collections(id,short_id,name,kind,color_hex,updated_at) VALUES('c3','c_T2','Deep Work','tag','#A2D2FF',"+db.Quote(updated)+");")
	mustExec(t, sqlite, "INSERT INTO collections(id,short_id,name,kind,color_hex,updated_at) VALUES('c4','c_G1','Weekly Goal','goal','#98B6FF',"+db.Quote(updated)+");")
	mustExec(t, sqlite, "INSERT INTO collections(id,short_id,name,kind,color_hex,updated_at) VALUES('c5','c_A1','Home Ops','area','#A3D9A5',"+db.Quote(updated)+");")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,due_at,scheduled_at,updated_at) VALUES('1','t_AAAAAA','Critical fix title','open','now',"+db.Quote(due)+","+db.Quote(scheduled)+","+db.Quote(updated)+");")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,due_at,scheduled_at,updated_at) VALUES('2','t_BBBBBB','Write deep focus memo','open','soon',"+db.Quote(due)+","+db.Quote(now.Format(time.RFC3339))+","+db.Quote(updated)+");")
	mustExec(t, sqlite, "INSERT INTO task_collections(task_id,collection_id) VALUES('1','c1');")
	mustExec(t, sqlite, "INSERT INTO task_collections(task_id,collection_id) VALUES('1','c2');")
	mustExec(t, sqlite, "INSERT INTO task_collections(task_id,collection_id) VALUES('1','c4');")
	mustExec(t, sqlite, "INSERT INTO task_collections(task_id,collection_id) VALUES('1','c5');")
	mustExec(t, sqlite, "INSERT INTO task_collections(task_id,collection_id) VALUES('2','c3');")
	mustExec(t, sqlite, "INSERT INTO task_assignees(task_id,user_id) VALUES('1','u1');")
	mustExec(t, sqlite, "INSERT INTO task_assignees(task_id,user_id) VALUES('2','u2');")
	return sqlite
}

func testModel(sqlite db.SQLite) model {
	catalog := ThemeCatalog{
		Default: "sunset-pop",
		Themes: []Theme{
			{ID: "sunset-pop", Name: "Sunset Pop"},
		},
	}
	return newModel(sqlite, catalog, "sunset-pop", "", 12, time.UTC, Identity{Actor: "human", UserID: "u1", UserName: "Human Demo"}, true)
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
