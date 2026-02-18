package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndResolveProfiles(t *testing.T) {
	d := t.TempDir()
	cfgPath := filepath.Join(d, "config.toml")
	content := `[profile.default]
db = "./x.db"
theme = "sunset-pop"

[profile.human]
theme = "paper-fruit"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	p := Resolve(cfg, "human")
	if p.DB != "./x.db" {
		t.Fatalf("expected db override, got %s", p.DB)
	}
	if p.Theme != "paper-fruit" {
		t.Fatalf("expected profile theme, got %s", p.Theme)
	}
}
