package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCandidatePathsUsesDOOHHome(t *testing.T) {
	t.Setenv("DOOH_HOME", filepath.Join(t.TempDir(), "dooh-home"))
	paths, err := candidatePaths("")
	if err != nil {
		t.Fatalf("candidatePaths: %v", err)
	}
	if len(paths) < 1 {
		t.Fatalf("expected at least one candidate path")
	}
	want := filepath.Join(os.Getenv("DOOH_HOME"), "config.toml")
	if paths[0] != want {
		t.Fatalf("expected first path %s, got %s", want, paths[0])
	}
}

func TestCandidatePathsExplicitPathWins(t *testing.T) {
	t.Setenv("DOOH_HOME", filepath.Join(t.TempDir(), "dooh-home"))
	explicit := "/tmp/custom-config.toml"
	paths, err := candidatePaths(explicit)
	if err != nil {
		t.Fatalf("candidatePaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != explicit {
		t.Fatalf("expected explicit path only, got %#v", paths)
	}
}
