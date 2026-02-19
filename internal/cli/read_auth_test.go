package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dooh/internal/auth"
	"dooh/internal/db"
)

func TestReadCommandsRequireAuthContext(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	sqlite, dbPath := newReadAuthDB(t)

	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u1','Human Demo','active');")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k1','u1','hhhhhhhh','"+auth.HashAPIKey("dooh_human_key")+"','tasks:read,collections:read,export:run,users:admin','human_cli',NULL);")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,updated_at,created_by,updated_by) VALUES('t1','t_AAAAAA','Alpha','open','now',strftime('%Y-%m-%dT%H:%M:%fZ','now'),'u1','u1');")
	mustExec(t, sqlite, "INSERT INTO collections(id,short_id,name,kind,color_hex,created_by,updated_by,updated_at) VALUES('c1','c_AAAAAA','Atlas','project','#7AB8FF','u1','u1',strftime('%Y-%m-%dT%H:%M:%fZ','now'));")

	oldMode := os.Getenv("DOOH_MODE")
	defer func() { _ = os.Setenv("DOOH_MODE", oldMode) }()
	_ = os.Unsetenv("DOOH_MODE")

	var out bytes.Buffer
	err := Run([]string{"task", "list", "--db", dbPath}, &out)
	if err == nil {
		t.Fatalf("expected auth context error")
	}
	if !strings.Contains(err.Error(), "No authenticated user context") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadCommandsPassWithHumanAuth(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	sqlite, dbPath := newReadAuthDB(t)

	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u1','Human Demo','active');")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k1','u1','hhhhhhhh','"+auth.HashAPIKey("dooh_human_key")+"','tasks:read,collections:read,export:run,users:admin','human_cli',NULL);")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,updated_at,created_by,updated_by) VALUES('t1','t_AAAAAA','Alpha','open','now',strftime('%Y-%m-%dT%H:%M:%fZ','now'),'u1','u1');")
	mustExec(t, sqlite, "INSERT INTO collections(id,short_id,name,kind,color_hex,created_by,updated_by,updated_at) VALUES('c1','c_AAAAAA','Atlas','project','#7AB8FF','u1','u1',strftime('%Y-%m-%dT%H:%M:%fZ','now'));")

	oldMode := os.Getenv("DOOH_MODE")
	defer func() { _ = os.Setenv("DOOH_MODE", oldMode) }()
	_ = os.Setenv("DOOH_MODE", "human")

	var out bytes.Buffer
	if err := Run([]string{"task", "list", "--db", dbPath, "--api-key", "dooh_human_key"}, &out); err != nil {
		t.Fatalf("task list should pass: %v", err)
	}
	if !strings.Contains(out.String(), "TITLE") {
		t.Fatalf("expected task list output, got: %s", out.String())
	}

	out.Reset()
	if err := Run([]string{"tui", "--db", dbPath, "--api-key", "dooh_human_key", "--static", "--plain"}, &out); err != nil {
		t.Fatalf("tui static should pass: %v", err)
	}
	if !strings.Contains(out.String(), "identity=human | Human Demo") {
		t.Fatalf("expected identity badge in tui output, got: %s", out.String())
	}
}

func newReadAuthDB(t *testing.T) (db.SQLite, string) {
	t.Helper()
	d := t.TempDir()
	path := filepath.Join(d, "read-auth.db")
	sqlite := db.New(path)
	mustExec(t, sqlite, "PRAGMA journal_mode=WAL;")
	mustExec(t, sqlite, `
CREATE TABLE users(id TEXT PRIMARY KEY,name TEXT,status TEXT);
CREATE TABLE api_keys(id TEXT PRIMARY KEY,user_id TEXT,key_prefix TEXT,key_hash TEXT,scopes TEXT,client_type TEXT,revoked_at TEXT);
CREATE TABLE tasks(
  id TEXT PRIMARY KEY,
  short_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  priority TEXT NOT NULL,
  due_at TEXT,
  scheduled_at TEXT,
  updated_at TEXT,
  deleted_at TEXT,
  created_by TEXT,
  updated_by TEXT
);
CREATE TABLE collections(
  id TEXT PRIMARY KEY,
  short_id TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  color_hex TEXT,
  deleted_at TEXT,
  created_by TEXT,
  updated_by TEXT,
  updated_at TEXT
);
CREATE TABLE task_collections(task_id TEXT NOT NULL,collection_id TEXT NOT NULL);
CREATE TABLE task_assignees(task_id TEXT NOT NULL,user_id TEXT NOT NULL);
`)
	return sqlite, path
}
