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

func TestMustAuthInfersActorFromExplicitKeyAndRespectsEnvSourceRules(t *testing.T) {
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
	p, err := mustAuth(rt, sqlite, "dooh_human_key", false, "tasks:write")
	if err != nil {
		t.Fatalf("expected explicit key auth without DOOH_MODE, got %v", err)
	}
	if p.Actor != "human" {
		t.Fatalf("expected human actor from key, got %s", p.Actor)
	}

	_ = os.Setenv("DOOH_MODE", "human")
	_, err = mustAuth(rt, sqlite, "", false, "tasks:write")
	if err == nil {
		t.Fatalf("expected human key flag requirement error")
	}

	_ = os.Setenv("DOOH_MODE", "ai")
	_ = os.Setenv("DOOH_API_KEY", "dooh_agent_key")
	p, err = mustAuth(rt, sqlite, "", false, "tasks:write")
	if err != nil {
		t.Fatalf("expected ai env key auth, got %v", err)
	}
	if p.Actor != "agent" {
		t.Fatalf("expected agent actor from env key, got %s", p.Actor)
	}
	p, err = mustAuth(rt, sqlite, "dooh_human_key", false, "tasks:write")
	if err != nil {
		t.Fatalf("expected explicit key to override mode hint, got %v", err)
	}
	if p.Actor != "human" {
		t.Fatalf("expected human actor from explicit key, got %s", p.Actor)
	}
}

func TestRequireHumanLifecycleAdmin(t *testing.T) {
	if err := requireHumanLifecycleAdmin(principal{Actor: "human", ClientType: "human_cli"}, false); err != nil {
		t.Fatalf("human should be allowed: %v", err)
	}
	if err := requireHumanLifecycleAdmin(principal{Actor: "agent", ClientType: "agent_cli"}, false); err == nil {
		t.Fatalf("ai actor should be denied without override")
	}
	if err := requireHumanLifecycleAdmin(principal{Actor: "agent", ClientType: "system"}, false); err == nil {
		t.Fatalf("system key should be denied without override flag")
	}
	if err := requireHumanLifecycleAdmin(principal{Actor: "agent", ClientType: "system"}, true); err != nil {
		t.Fatalf("system key should be allowed with override flag: %v", err)
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
