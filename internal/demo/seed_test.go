package demo

import (
	"os/exec"
	"path/filepath"
	"testing"

	"dooh/internal/db"
)

func TestSeedCreatesTasksAndCollections(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	d := t.TempDir()
	dbPath := filepath.Join(d, "t.db")
	sqlite := db.New(dbPath)

	if err := sqlite.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Exec(`
CREATE TABLE users(id TEXT PRIMARY KEY,name TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'active',created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')));
CREATE TABLE collections(id TEXT PRIMARY KEY,short_id TEXT NOT NULL UNIQUE,name TEXT NOT NULL,kind TEXT NOT NULL,color_hex TEXT NOT NULL,description TEXT NOT NULL DEFAULT '',archived_at TEXT,deleted_at TEXT,created_by TEXT NOT NULL,updated_by TEXT NOT NULL,created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),version INTEGER NOT NULL DEFAULT 1);
CREATE TABLE tasks(id TEXT PRIMARY KEY,short_id TEXT NOT NULL UNIQUE,title TEXT NOT NULL,description TEXT NOT NULL DEFAULT '',status TEXT NOT NULL,priority TEXT NOT NULL,due_at TEXT,scheduled_at TEXT,rollover_enabled INTEGER NOT NULL DEFAULT 0,skip_weekends INTEGER NOT NULL DEFAULT 0,estimated_minutes INTEGER,completed_at TEXT,archived_at TEXT,deleted_at TEXT,created_by TEXT NOT NULL,updated_by TEXT NOT NULL,created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),version INTEGER NOT NULL DEFAULT 1);
CREATE TABLE task_collections(task_id TEXT NOT NULL,collection_id TEXT NOT NULL,created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),PRIMARY KEY(task_id,collection_id));
CREATE TABLE task_assignees(task_id TEXT NOT NULL,user_id TEXT NOT NULL,created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),PRIMARY KEY(task_id,user_id));
`); err != nil {
		t.Fatal(err)
	}

	res, err := Seed(sqlite)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tasks == 0 {
		t.Fatalf("expected tasks seeded")
	}
	rows, err := sqlite.QueryTSV("SELECT COUNT(*) FROM tasks;")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || len(rows[0]) == 0 || rows[0][0] == "0" {
		t.Fatalf("expected rows in tasks")
	}
}
