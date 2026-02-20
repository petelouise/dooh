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

func TestLoginHumanEnablesStoredKeyFallback(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	sqlite, dbPath := newReadAuthDB(t)
	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u1','Human Demo','active');")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k1','u1','hhhhhhhh','"+auth.HashAPIKey("dooh_human_key")+"','tasks:read,collections:read,export:run,users:admin','human_cli',NULL);")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,updated_at,created_by,updated_by) VALUES('t1','t_AAAAAA','Alpha','open','now',strftime('%Y-%m-%dT%H:%M:%fZ','now'),'u1','u1');")

	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldMode := os.Getenv("DOOH_MODE")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("DOOH_MODE", oldMode)
	}()
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("DOOH_MODE", "human")

	var out bytes.Buffer
	if err := Run([]string{"--profile", "human", "login", "--db", dbPath, "--api-key", "dooh_human_key"}, &out); err != nil {
		t.Fatalf("login should pass: %v", err)
	}
	out.Reset()
	if err := Run([]string{"--profile", "human", "task", "list", "--db", dbPath}, &out); err != nil {
		t.Fatalf("task list should use stored human key: %v", err)
	}
	if !strings.Contains(out.String(), "TITLE") {
		t.Fatalf("expected task list output, got: %s", out.String())
	}
}

func TestEnvAgentPrintsStoredAgentKey(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	sqlite, dbPath := newReadAuthDB(t)
	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u2','Agent Demo','active');")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k2','u2','aaaaaaaa','"+auth.HashAPIKey("dooh_agent_key")+"','tasks:read,collections:read','agent_cli',NULL);")

	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer func() { _ = os.Setenv("HOME", oldHome) }()
	_ = os.Setenv("HOME", home)

	var out bytes.Buffer
	if err := Run([]string{"--profile", "agent", "login", "--db", dbPath, "--api-key", "dooh_agent_key"}, &out); err != nil {
		t.Fatalf("agent login should pass: %v", err)
	}
	out.Reset()
	if err := Run([]string{"--profile", "agent", "env", "--mode", "ai", "--db", dbPath}, &out); err != nil {
		t.Fatalf("env should pass: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "export DOOH_MODE='ai'") {
		t.Fatalf("expected agent mode export, got: %s", got)
	}
	if !strings.Contains(got, "export DOOH_API_KEY='dooh_agent_key'") {
		t.Fatalf("expected agent api key export, got: %s", got)
	}
}

func TestSetupDemoCreatesProfilesAndWhoAmIWorksWithoutFlagKey(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	home := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "setup-demo.db")

	oldHome := os.Getenv("HOME")
	oldMode := os.Getenv("DOOH_MODE")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("DOOH_MODE", oldMode)
	}()
	_ = os.Setenv("HOME", home)
	_ = os.Unsetenv("DOOH_MODE")

	var out bytes.Buffer
	if err := Run([]string{"setup", "demo", "--db", dbPath}, &out); err != nil {
		t.Fatalf("setup demo should pass: %v", err)
	}
	if !strings.Contains(out.String(), "setup complete:") {
		t.Fatalf("expected setup output, got: %s", out.String())
	}
	humanKeyPath := filepath.Join(home, ".config", "dooh", "auth", "human.human.key")
	agentKeyPath := filepath.Join(home, ".config", "dooh", "auth", "ai.agent.key")
	if _, err := os.Stat(humanKeyPath); err != nil {
		t.Fatalf("expected human key file: %v", err)
	}
	if _, err := os.Stat(agentKeyPath); err != nil {
		t.Fatalf("expected agent key file: %v", err)
	}

	_ = os.Setenv("DOOH_MODE", "human")
	out.Reset()
	if err := Run([]string{"--profile", "human", "whoami", "--db", dbPath}, &out); err != nil {
		t.Fatalf("whoami should pass with stored human key: %v", err)
	}
	if !strings.Contains(out.String(), "mode=human") || !strings.Contains(out.String(), "user=") {
		t.Fatalf("expected whoami context output, got: %s", out.String())
	}
}

func TestContextSetShowAndClear(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer func() { _ = os.Setenv("HOME", oldHome) }()
	_ = os.Setenv("HOME", home)

	var out bytes.Buffer
	if err := Run([]string{"context", "set", "--profile", "human", "--db", "/tmp/demo.db", "--theme", "sunset-pop"}, &out); err != nil {
		t.Fatalf("context set should pass: %v", err)
	}
	out.Reset()
	if err := Run([]string{"context", "show"}, &out); err != nil {
		t.Fatalf("context show should pass: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "profile=human") || !strings.Contains(s, "db=/tmp/demo.db") || !strings.Contains(s, "theme=sunset-pop") {
		t.Fatalf("unexpected context show output: %s", s)
	}
	out.Reset()
	if err := Run([]string{"context", "clear"}, &out); err != nil {
		t.Fatalf("context clear should pass: %v", err)
	}
}

func TestAIEnvForcesAIProfile(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	sqlite, dbPath := newReadAuthDB(t)
	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u2','Agent Demo','active');")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k2','u2','aaaaaaaa','"+auth.HashAPIKey("dooh_agent_key")+"','tasks:read,collections:read','agent_cli',NULL);")

	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldProfile := os.Getenv("DOOH_PROFILE")
	oldAIKey := os.Getenv("DOOH_AI_KEY")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("DOOH_PROFILE", oldProfile)
		_ = os.Setenv("DOOH_AI_KEY", oldAIKey)
	}()
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("DOOH_PROFILE", "human")
	_ = os.Setenv("DOOH_AI_KEY", "dooh_agent_key")

	var out bytes.Buffer
	if err := Run([]string{"whoami", "--db", dbPath}, &out); err != nil {
		t.Fatalf("whoami should pass in ai env: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "profile=ai") || !strings.Contains(got, "mode=ai") {
		t.Fatalf("expected ai profile and mode, got: %s", got)
	}
}

func TestReadStoredKeyMissingFile(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer func() { _ = os.Setenv("HOME", oldHome) }()
	_ = os.Setenv("HOME", home)

	key, path, err := readStoredKey("missing", "human")
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if key != "" {
		t.Fatalf("expected empty key for missing file")
	}
	if !strings.HasSuffix(path, "missing.human.key") {
		t.Fatalf("unexpected key path: %s", path)
	}
}

func TestDOOHHomeOverridesAuthAndContextPaths(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	sqlite, dbPath := newReadAuthDB(t)
	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u1','Human Demo','active');")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k1','u1','hhhhhhhh','"+auth.HashAPIKey("dooh_human_key")+"','tasks:read,collections:read,export:run,users:admin','human_cli',NULL);")

	home := t.TempDir()
	doohHome := filepath.Join(t.TempDir(), "dooh-alt")
	oldHome := os.Getenv("HOME")
	oldDoohHome := os.Getenv("DOOH_HOME")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("DOOH_HOME", oldDoohHome)
	}()
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("DOOH_HOME", doohHome)

	var out bytes.Buffer
	if err := Run([]string{"--profile", "human", "login", "--db", dbPath, "--api-key", "dooh_human_key"}, &out); err != nil {
		t.Fatalf("login should pass: %v", err)
	}
	keyPath := filepath.Join(doohHome, "auth", "human.human.key")
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("expected key under DOOH_HOME path: %v", err)
	}

	out.Reset()
	if err := Run([]string{"context", "set", "--profile", "human", "--db", dbPath, "--theme", "paper-fruit"}, &out); err != nil {
		t.Fatalf("context set should pass: %v", err)
	}
	contextPath := filepath.Join(doohHome, "context.json")
	if _, err := os.Stat(contextPath); err != nil {
		t.Fatalf("expected context under DOOH_HOME path: %v", err)
	}
}

func TestInitDatabaseReusable(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	path := filepath.Join(t.TempDir(), "init.db")
	sqlite := db.New(path)
	if err := initDatabase(sqlite); err != nil {
		t.Fatalf("first init failed: %v", err)
	}
	if err := initDatabase(sqlite); err != nil {
		t.Fatalf("second init failed: %v", err)
	}
}
