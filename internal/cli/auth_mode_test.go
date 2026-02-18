package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"dooh/internal/auth"
	"dooh/internal/config"
	"dooh/internal/db"
)

func TestMustAuthRequiresModeAndSourceRules(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	sqlite := newAuthDB(t)
	rt := runtime{profile: config.Profile{APIKeyEnv: "DOOH_API_KEY"}}

	oldMode := os.Getenv("DOOH_MODE")
	oldKey := os.Getenv("DOOH_API_KEY")
	defer func() {
		_ = os.Setenv("DOOH_MODE", oldMode)
		_ = os.Setenv("DOOH_API_KEY", oldKey)
	}()

	_ = os.Unsetenv("DOOH_MODE")
	_, err := mustAuth(rt, sqlite, "dooh_human_key", false, "tasks:write")
	if err == nil {
		t.Fatalf("expected DOOH_MODE requirement error")
	}

	_ = os.Setenv("DOOH_MODE", "human")
	_, err = mustAuth(rt, sqlite, "", false, "tasks:write")
	if err == nil {
		t.Fatalf("expected human key flag requirement error")
	}

	_ = os.Setenv("DOOH_MODE", "agent")
	_ = os.Setenv("DOOH_API_KEY", "dooh_agent_key")
	_, err = mustAuth(rt, sqlite, "manual_key_not_allowed", false, "tasks:write")
	if err == nil {
		t.Fatalf("expected agent key source restriction")
	}
}

func newAuthDB(t *testing.T) db.SQLite {
	t.Helper()
	d := t.TempDir()
	sqlite := db.New(filepath.Join(d, "auth.db"))
	mustExec(t, sqlite, "PRAGMA journal_mode=WAL;")
	mustExec(t, sqlite, `
CREATE TABLE users(id TEXT PRIMARY KEY,name TEXT,status TEXT);
CREATE TABLE api_keys(id TEXT PRIMARY KEY,user_id TEXT,key_prefix TEXT,key_hash TEXT,scopes TEXT,client_type TEXT,revoked_at TEXT);
`)
	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u','User','active');")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k_h','u','hhhhhhhh','"+auth.HashAPIKey("dooh_human_key")+"','tasks:write','human_cli',NULL);")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k_a','u','aaaaaaaa','"+auth.HashAPIKey("dooh_agent_key")+"','tasks:write','agent_cli',NULL);")
	return sqlite
}
