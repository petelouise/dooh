package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Profile struct {
	DB        string
	Actor     string
	Timezone  string
	Theme     string
	ExportDir string
	APIKeyEnv string
}

type Config struct {
	Profiles map[string]Profile
	Sources  []string
}

func Load(explicitPath string) (Config, error) {
	cfg := Config{Profiles: map[string]Profile{}}

	paths, err := candidatePaths(explicitPath)
	if err != nil {
		return cfg, err
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		parsed, err := parseFile(p)
		if err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", p, err)
		}
		merge(&cfg, parsed)
		cfg.Sources = append(cfg.Sources, p)
	}

	return cfg, nil
}

func Resolve(cfg Config, profile string) Profile {
	base := Profile{
		DB:        "./dooh.db",
		Actor:     "agent",
		Timezone:  "America/Los_Angeles",
		Theme:     "sunset-pop",
		ExportDir: "./site-data",
		APIKeyEnv: "DOOH_API_KEY",
	}
	if p, ok := cfg.Profiles["default"]; ok {
		base = overlay(base, p)
	}
	if profile != "" {
		if p, ok := cfg.Profiles[profile]; ok {
			base = overlay(base, p)
		}
	}
	return base
}

func candidatePaths(explicitPath string) ([]string, error) {
	if strings.TrimSpace(explicitPath) != "" {
		return []string{explicitPath}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve cwd: %w", err)
	}
	return []string{
		filepath.Join(home, ".config", "dooh", "config.toml"),
		filepath.Join(cwd, ".dooh", "config.toml"),
	}, nil
}

func parseFile(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	cfg := Config{Profiles: map[string]Profile{}}
	current := ""
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			if strings.HasPrefix(section, "profile.") {
				current = strings.TrimSpace(strings.TrimPrefix(section, "profile."))
				if _, ok := cfg.Profiles[current]; !ok {
					cfg.Profiles[current] = Profile{}
				}
			} else {
				current = ""
			}
			continue
		}
		if current == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		v = strings.Trim(v, `"`)

		p := cfg.Profiles[current]
		switch k {
		case "db":
			p.DB = v
		case "actor":
			p.Actor = v
		case "timezone":
			p.Timezone = v
		case "theme":
			p.Theme = v
		case "export_dir":
			p.ExportDir = v
		case "api_key_env":
			p.APIKeyEnv = v
		}
		cfg.Profiles[current] = p
	}
	if err := s.Err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func merge(dst *Config, src Config) {
	for name, p := range src.Profiles {
		existing, ok := dst.Profiles[name]
		if !ok {
			dst.Profiles[name] = p
			continue
		}
		dst.Profiles[name] = overlay(existing, p)
	}
}

func overlay(base Profile, v Profile) Profile {
	if v.DB != "" {
		base.DB = v.DB
	}
	if v.Actor != "" {
		base.Actor = v.Actor
	}
	if v.Timezone != "" {
		base.Timezone = v.Timezone
	}
	if v.Theme != "" {
		base.Theme = v.Theme
	}
	if v.ExportDir != "" {
		base.ExportDir = v.ExportDir
	}
	if v.APIKeyEnv != "" {
		base.APIKeyEnv = v.APIKeyEnv
	}
	return base
}
