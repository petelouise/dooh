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

func TestLifecycleCommandsDenyAIAccessEvenWithAdminScopes(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	path := filepath.Join(t.TempDir(), "guard.db")
	sqlite := db.New(path)
	if err := initDatabase(sqlite); err != nil {
		t.Fatal(err)
	}

	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u_h','Human Demo','active');")
	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u_a','Agent Demo','active');")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k_h','u_h','hhhhhhhh','"+auth.HashAPIKey("dooh_human_admin")+"','users:admin,keys:admin','human_cli',NULL);")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k_a','u_a','aaaaaaaa','"+auth.HashAPIKey("dooh_ai_admin")+"','users:admin,keys:admin','agent_cli',NULL);")

	oldMode := os.Getenv("DOOH_MODE")
	defer func() { _ = os.Setenv("DOOH_MODE", oldMode) }()
	_ = os.Setenv("DOOH_MODE", "ai")

	var out bytes.Buffer
	err := Run([]string{"user", "create", "--db", path, "--api-key", "dooh_ai_admin", "--name", "Blocked"}, &out)
	if err == nil || !strings.Contains(err.Error(), "lifecycle admin actions require human actor") {
		t.Fatalf("expected ai lifecycle denial, got err=%v out=%s", err, out.String())
	}

	out.Reset()
	err = Run([]string{"key", "create", "--db", path, "--api-key", "dooh_ai_admin", "--user", "u_h", "--scopes", "tasks:read"}, &out)
	if err == nil || !strings.Contains(err.Error(), "lifecycle admin actions require human actor") {
		t.Fatalf("expected ai lifecycle denial for key create, got err=%v out=%s", err, out.String())
	}
}

func TestMutationsWriteEventAttributionForHumanAndAI(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	path := filepath.Join(t.TempDir(), "events.db")
	sqlite := db.New(path)
	if err := initDatabase(sqlite); err != nil {
		t.Fatal(err)
	}

	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u_h','Human Demo','active');")
	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u_a','Agent Demo','active');")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k_h','u_h','hhhhhhhh','"+auth.HashAPIKey("dooh_human_write")+"','tasks:write,tasks:delete,tasks:read','human_cli',NULL);")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k_a','u_a','aaaaaaaa','"+auth.HashAPIKey("dooh_ai_write")+"','tasks:write,tasks:delete,tasks:read','agent_cli',NULL);")

	oldMode := os.Getenv("DOOH_MODE")
	oldAIKey := os.Getenv("DOOH_AI_KEY")
	defer func() {
		_ = os.Setenv("DOOH_MODE", oldMode)
		_ = os.Setenv("DOOH_AI_KEY", oldAIKey)
	}()

	var out bytes.Buffer
	if err := Run([]string{"task", "add", "--db", path, "--api-key", "dooh_human_write", "--title", "Human mutation", "--priority", "now"}, &out); err != nil {
		t.Fatalf("human task add failed: %v", err)
	}

	_ = os.Setenv("DOOH_MODE", "ai")
	_ = os.Setenv("DOOH_AI_KEY", "dooh_ai_write")
	out.Reset()
	if err := Run([]string{"task", "add", "--db", path, "--title", "AI mutation", "--priority", "soon"}, &out); err != nil {
		t.Fatalf("ai task add failed: %v", err)
	}

	rows, err := sqlite.QueryTSV("SELECT event_type,actor_user_id,key_id,client_type FROM events WHERE event_type='task.created' ORDER BY seq;")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 task.created events, got %d", len(rows))
	}
	if rows[0][1] != "u_h" || rows[0][2] != "k_h" || rows[0][3] != "human_cli" {
		t.Fatalf("unexpected human attribution row: %v", rows[0])
	}
	if rows[1][1] != "u_a" || rows[1][2] != "k_a" || rows[1][3] != "agent_cli" {
		t.Fatalf("unexpected ai attribution row: %v", rows[1])
	}
}

func TestContextPrecedenceFlagsThenEnvThenContext(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	home := t.TempDir()
	db1 := filepath.Join(t.TempDir(), "db1.db")
	db2 := filepath.Join(t.TempDir(), "db2.db")
	db3 := filepath.Join(t.TempDir(), "db3.db")
	for _, path := range []string{db1, db2, db3} {
		sqlite := db.New(path)
		if err := initDatabase(sqlite); err != nil {
			t.Fatal(err)
		}
		mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u_h','Human Demo','active');")
		mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k_h','u_h','hhhhhhhh','"+auth.HashAPIKey("dooh_human_ctx")+"','tasks:read','human_cli',NULL);")
	}

	oldHome := os.Getenv("HOME")
	oldDB := os.Getenv("DOOH_DB")
	oldMode := os.Getenv("DOOH_MODE")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("DOOH_DB", oldDB)
		_ = os.Setenv("DOOH_MODE", oldMode)
	}()
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("DOOH_MODE", "human")

	var out bytes.Buffer
	if err := Run([]string{"context", "set", "--db", db1}, &out); err != nil {
		t.Fatalf("context set failed: %v", err)
	}
	out.Reset()
	if err := Run([]string{"whoami", "--api-key", "dooh_human_ctx"}, &out); err != nil {
		t.Fatalf("whoami with context db failed: %v", err)
	}
	if !strings.Contains(out.String(), "db="+db1) {
		t.Fatalf("expected context db in output, got: %s", out.String())
	}

	_ = os.Setenv("DOOH_DB", db2)
	out.Reset()
	if err := Run([]string{"whoami", "--api-key", "dooh_human_ctx"}, &out); err != nil {
		t.Fatalf("whoami with env db failed: %v", err)
	}
	if !strings.Contains(out.String(), "db="+db2) {
		t.Fatalf("expected env db override in output, got: %s", out.String())
	}

	out.Reset()
	if err := Run([]string{"whoami", "--db", db3, "--api-key", "dooh_human_ctx"}, &out); err != nil {
		t.Fatalf("whoami with flag db failed: %v", err)
	}
	if !strings.Contains(out.String(), "db="+db3) {
		t.Fatalf("expected flag db override in output, got: %s", out.String())
	}
}

func TestUserDeleteCommandIsNotSupported(t *testing.T) {
	var out bytes.Buffer
	err := Run([]string{"user", "delete", "--id", "u_h"}, &out)
	if err == nil || !strings.Contains(err.Error(), "unknown user command") {
		t.Fatalf("expected user delete to be unsupported, got err=%v out=%s", err, out.String())
	}
}
