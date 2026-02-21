package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dooh/internal/db"
)

type contextState struct {
	Profile   string `json:"profile"`
	DB        string `json:"db"`
	Theme     string `json:"theme"`
	UpdatedAt string `json:"updated_at"`
}

func runConfig(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return printConfigHelp(out)
	}
	switch args[0] {
	case "show":
		p := rt.profile
		if rt.opts.JSON {
			return writeJSON(out, map[string]any{
				"profile":     rt.opts.Profile,
				"db":          p.DB,
				"timezone":    p.Timezone,
				"theme":       p.Theme,
				"export_dir":  p.ExportDir,
				"api_key_env": p.APIKeyEnv,
				"sources":     rt.config.Sources,
			})
		}
		_, _ = fmt.Fprintf(out, "profile=%s\n", rt.opts.Profile)
		_, _ = fmt.Fprintf(out, "db=%s\n", p.DB)
		_, _ = fmt.Fprintf(out, "timezone=%s\n", p.Timezone)
		_, _ = fmt.Fprintf(out, "theme=%s\n", p.Theme)
		_, _ = fmt.Fprintf(out, "export_dir=%s\n", p.ExportDir)
		_, _ = fmt.Fprintf(out, "api_key_env=%s\n", p.APIKeyEnv)
		if len(rt.config.Sources) > 0 {
			_, _ = fmt.Fprintln(out, "sources:")
			for _, s := range rt.config.Sources {
				_, _ = fmt.Fprintf(out, "- %s\n", s)
			}
		}
		return nil
	case "init":
		fs := flag.NewFlagSet("config init", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		path := fs.String("path", filepath.Join(".dooh", "config.toml"), "path to write")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(*path), 0o755); err != nil {
			return err
		}
		tpl := `# dooh config
# precedence: flags > env > selected profile > default profile > built-in defaults

[profile.default]
db = "./dooh.db"
timezone = "America/Los_Angeles"
theme = "sunset-pop"
export_dir = "./site-data"
api_key_env = "DOOH_API_KEY"

[profile.human]
theme = "paper-fruit"

[profile.ai]
theme = "midnight-arcade"
api_key_env = "DOOH_AI_KEY"

[profile.agent]
theme = "midnight-arcade"
`
		if err := os.WriteFile(*path, []byte(tpl), 0o644); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "wrote config template to %s\n", *path)
		return nil
	case "help", "--help", "-h":
		return printConfigHelp(out)
	default:
		return fmt.Errorf("unknown config command %q (available: show, init)", args[0])
	}
}

func printConfigHelp(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "config subcommands:")
	_, _ = fmt.Fprintln(out, "  show   display resolved config for current profile")
	_, _ = fmt.Fprintln(out, "  init   create starter config.toml file")
	return nil
}

func runWhoAmI(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("whoami", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false)
	if err != nil {
		return err
	}
	if rt.opts.JSON {
		return writeJSON(out, map[string]any{
			"profile":     rt.opts.Profile,
			"mode":        displayActor(p.Actor),
			"user_id":     p.UserID,
			"user_name":   p.UserName,
			"key_prefix":  p.KeyPrefix,
			"client_type": p.ClientType,
			"db":          resolveDB(rt, *dbPath),
		})
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	_, _ = fmt.Fprintf(out, "client_type=%s\n", p.ClientType)
	return nil
}

func runContext(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return printContextHelp(out)
	}
	switch args[0] {
	case "show":
		path, err := contextFilePath()
		if err != nil {
			return err
		}
		if rt.opts.JSON {
			result := map[string]any{
				"profile":      rt.opts.Profile,
				"db":           resolveDB(rt, ""),
				"theme":        resolveTheme(rt, ""),
				"context_file": path,
			}
			if strings.TrimSpace(rt.context.Profile) != "" || strings.TrimSpace(rt.context.DB) != "" || strings.TrimSpace(rt.context.Theme) != "" {
				result["context_overrides"] = map[string]string{
					"profile": rt.context.Profile,
					"db":      rt.context.DB,
					"theme":   rt.context.Theme,
				}
			}
			result["ai_profile_enforced"] = rt.aiProfileEnforced
			sqlite := db.New(resolveDB(rt, ""))
			if p, source, ok := resolvePrincipalForShow(rt, sqlite); ok {
				result["actor"] = displayActor(p.Actor)
				result["user_id"] = p.UserID
				result["auth_source"] = source
			} else {
				result["auth_source"] = "none"
			}
			return writeJSON(out, result)
		}
		_, _ = fmt.Fprintf(out, "profile=%s\n", rt.opts.Profile)
		_, _ = fmt.Fprintf(out, "db=%s\n", resolveDB(rt, ""))
		_, _ = fmt.Fprintf(out, "theme=%s\n", resolveTheme(rt, ""))
		_, _ = fmt.Fprintf(out, "context_file=%s\n", path)
		if strings.TrimSpace(rt.context.Profile) != "" || strings.TrimSpace(rt.context.DB) != "" || strings.TrimSpace(rt.context.Theme) != "" {
			_, _ = fmt.Fprintf(out, "context.profile=%s\n", rt.context.Profile)
			_, _ = fmt.Fprintf(out, "context.db=%s\n", rt.context.DB)
			_, _ = fmt.Fprintf(out, "context.theme=%s\n", rt.context.Theme)
		}
		if rt.aiProfileEnforced {
			_, _ = fmt.Fprintln(out, "ai context active (profile auto-set to ai)")
		}
		sqlite := db.New(resolveDB(rt, ""))
		if p, source, ok := resolvePrincipalForShow(rt, sqlite); ok {
			_, _ = fmt.Fprintf(out, "actor=%s\n", displayActor(p.Actor))
			_, _ = fmt.Fprintf(out, "user=%s\n", p.UserID)
			_, _ = fmt.Fprintf(out, "auth_source=%s\n", source)
		} else {
			_, _ = fmt.Fprintln(out, "auth_source=none")
		}
		return nil
	case "set":
		fs := flag.NewFlagSet("context set", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		profile := fs.String("profile", "", "profile name override")
		dbPath := fs.String("db", "", "sqlite database path override")
		theme := fs.String("theme", "", "theme override")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*profile) == "" && strings.TrimSpace(*dbPath) == "" && strings.TrimSpace(*theme) == "" {
			return errors.New("context set requires at least one of --profile, --db, --theme")
		}
		next := rt.context
		if strings.TrimSpace(*profile) != "" {
			next.Profile = strings.TrimSpace(*profile)
		}
		if strings.TrimSpace(*dbPath) != "" {
			next.DB = strings.TrimSpace(*dbPath)
		}
		if strings.TrimSpace(*theme) != "" {
			next.Theme = strings.TrimSpace(*theme)
		}
		next.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := writeContextState(next); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, "updated context overrides")
		return nil
	case "clear":
		if err := clearContextState(); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, "cleared context overrides")
		return nil
	case "help", "--help", "-h":
		return printContextHelp(out)
	default:
		return fmt.Errorf("unknown context command %q (available: show, set, clear)", args[0])
	}
}

func printContextHelp(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "context subcommands:")
	_, _ = fmt.Fprintln(out, "  show    display current profile, db, theme, and auth state")
	_, _ = fmt.Fprintln(out, "  set     persist local overrides (--profile, --db, --theme)")
	_, _ = fmt.Fprintln(out, "  clear   remove all local overrides")
	return nil
}

func runEnv(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	mode := fs.String("mode", "", "actor mode (human|ai)")
	dbPath := fs.String("db", "", "sqlite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	selectedMode := normalizeActor(strings.TrimSpace(*mode))
	if selectedMode == "" {
		selectedMode = normalizeActor(strings.TrimSpace(os.Getenv("DOOH_MODE")))
	}
	if selectedMode == "" {
		if rt.opts.Profile == "agent" || rt.opts.Profile == "ai" {
			selectedMode = "agent"
		} else {
			selectedMode = "human"
		}
	}
	if selectedMode != "human" && selectedMode != "agent" {
		return errors.New("--mode must be human or ai")
	}
	_, _ = fmt.Fprintf(out, "export DOOH_PROFILE=%s\n", shellQuote(rt.opts.Profile))
	_, _ = fmt.Fprintf(out, "export DOOH_DB=%s\n", shellQuote(resolveDB(rt, *dbPath)))
	_, _ = fmt.Fprintf(out, "export DOOH_MODE=%s\n", shellQuote(displayActor(selectedMode)))
	apiEnv := rt.profile.APIKeyEnv
	if strings.TrimSpace(apiEnv) == "" {
		apiEnv = "DOOH_API_KEY"
	}
	if selectedMode == "agent" {
		k, _, err := readStoredKey(rt.opts.Profile, "agent")
		if err != nil {
			return err
		}
		if k == "" {
			_, _ = fmt.Fprintf(out, "# no stored ai key for profile %s (run: dooh --profile %s login --api-key <key>)\n", rt.opts.Profile, rt.opts.Profile)
		} else {
			_, _ = fmt.Fprintf(out, "export %s=%s\n", apiEnv, shellQuote(k))
		}
	} else {
		_, _ = fmt.Fprintf(out, "unset %s\n", apiEnv)
	}
	return nil
}

func contextFilePath() (string, error) {
	base, err := appHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "context.json"), nil
}

func readContextState() (contextState, error) {
	var s contextState
	path, err := contextFilePath()
	if err != nil {
		return s, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return contextState{}, err
	}
	return s, nil
}

func writeContextState(s contextState) error {
	path, err := contextFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func clearContextState() error {
	path, err := contextFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
