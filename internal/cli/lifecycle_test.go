package cli

import (
	"os/exec"
	"path/filepath"
	"testing"

	"dooh/internal/db"
)

func TestHasDependencyPathDetectsReachability(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	sqlite := newLifecycleDB(t)

	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,created_by,updated_by) VALUES('a','t_a','A','open','now','u','u');")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,created_by,updated_by) VALUES('b','t_b','B','open','now','u','u');")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,created_by,updated_by) VALUES('c','t_c','C','open','now','u','u');")
	mustExec(t, sqlite, "INSERT INTO task_dependencies(task_id,blocked_by_task_id) VALUES('a','b');")
	mustExec(t, sqlite, "INSERT INTO task_dependencies(task_id,blocked_by_task_id) VALUES('b','c');")

	has, err := hasDependencyPath(sqlite, "a", "c")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatalf("expected path a -> c")
	}
}

func TestSyncParentsForChildReopensAndCompletesParent(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	sqlite := newLifecycleDB(t)

	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,created_by,updated_by) VALUES('parent','t_p','Parent','completed','now','u','u');")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,created_by,updated_by) VALUES('child','t_c','Child','open','now','u','u');")
	mustExec(t, sqlite, "INSERT INTO task_subtasks(parent_task_id,child_task_id) VALUES('parent','child');")

	if err := syncParentsForChild(sqlite, "child", "u"); err != nil {
		t.Fatal(err)
	}
	rows, err := sqlite.QueryTSV("SELECT status FROM tasks WHERE id='parent';")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || len(rows[0]) == 0 || rows[0][0] != "open" {
		t.Fatalf("expected parent reopened, got %v", rows)
	}

	mustExec(t, sqlite, "UPDATE tasks SET status='completed' WHERE id='child';")
	if err := syncParentsForChild(sqlite, "child", "u"); err != nil {
		t.Fatal(err)
	}
	rows, err = sqlite.QueryTSV("SELECT status FROM tasks WHERE id='parent';")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || len(rows[0]) == 0 || rows[0][0] != "completed" {
		t.Fatalf("expected parent completed, got %v", rows)
	}
}

func newLifecycleDB(t *testing.T) db.SQLite {
	t.Helper()
	d := t.TempDir()
	sqlite := db.New(filepath.Join(d, "lifecycle.db"))
	mustExec(t, sqlite, "PRAGMA journal_mode=WAL;")
	mustExec(t, sqlite, `
CREATE TABLE tasks(
  id TEXT PRIMARY KEY,
  short_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  priority TEXT NOT NULL,
  deleted_at TEXT,
  completed_at TEXT,
  archived_at TEXT,
  created_by TEXT NOT NULL,
  updated_by TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE task_dependencies(
  task_id TEXT NOT NULL,
  blocked_by_task_id TEXT NOT NULL,
  PRIMARY KEY(task_id, blocked_by_task_id)
);
CREATE TABLE task_subtasks(
  parent_task_id TEXT NOT NULL,
  child_task_id TEXT NOT NULL,
  PRIMARY KEY(parent_task_id, child_task_id)
);
`)
	return sqlite
}

func mustExec(t *testing.T, sqlite db.SQLite, sql string) {
	t.Helper()
	if err := sqlite.Exec(sql); err != nil {
		t.Fatal(err)
	}
}
