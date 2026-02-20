package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"dooh/internal/auth"
	"dooh/internal/config"
	"dooh/internal/db"
	"dooh/internal/demo"
	"dooh/internal/exporter"
	"dooh/internal/idgen"
	"dooh/internal/tui"
)

type principal struct {
	UserID     string
	UserName   string
	KeyID      string
	KeyPrefix  string
	Actor      string
	ClientType string
	Scopes     map[string]bool
}

var errNoAuthContext = errors.New("No authenticated user context. Provide a valid key (via --api-key, stored login key, or env key).")

type globalOpts struct {
	Profile    string
	ProfileSet bool
	ConfigPath string
}

type contextState struct {
	Profile   string `json:"profile"`
	DB        string `json:"db"`
	Theme     string `json:"theme"`
	UpdatedAt string `json:"updated_at"`
}

type runtime struct {
	opts              globalOpts
	config            config.Config
	profile           config.Profile
	context           contextState
	aiProfileEnforced bool
}

var palette = []string{"#FF7A59", "#FFD166", "#2EC4B6", "#4D96FF", "#FF6B6B", "#70E000", "#00E5FF"}

// Run executes dooh CLI commands using stdlib and sqlite3 CLI.
func Run(args []string, stdout io.Writer) error {
	opts, rest, err := parseGlobal(args)
	if err != nil {
		return err
	}
	ctx, _ := readContextState()

	envProfile := strings.TrimSpace(os.Getenv("DOOH_PROFILE"))
	effectiveProfile := "default"
	switch {
	case opts.ProfileSet && strings.TrimSpace(opts.Profile) != "":
		effectiveProfile = strings.TrimSpace(opts.Profile)
	case hasAIEnvKey():
		effectiveProfile = "ai"
	case envProfile != "":
		effectiveProfile = envProfile
	case strings.TrimSpace(ctx.Profile) != "":
		effectiveProfile = strings.TrimSpace(ctx.Profile)
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	opts.Profile = effectiveProfile
	rt := runtime{opts: opts, config: cfg, profile: config.Resolve(cfg, opts.Profile), context: ctx, aiProfileEnforced: hasAIEnvKey() && !opts.ProfileSet}

	if len(rest) == 0 {
		printUsage(stdout)
		return nil
	}

	switch rest[0] {
	case "version", "--version", "-v":
		_, _ = fmt.Fprintln(stdout, "0.3.0")
		return nil
	case "config":
		return runConfig(rt, rest[1:], stdout)
	case "db":
		return runDB(rt, rest[1:], stdout)
	case "setup":
		return runSetup(rt, rest[1:], stdout)
	case "demo":
		return runDemo(rt, rest[1:], stdout)
	case "login":
		return runLogin(rt, rest[1:], stdout)
	case "env":
		return runEnv(rt, rest[1:], stdout)
	case "context":
		return runContext(rt, rest[1:], stdout)
	case "user":
		return runUser(rt, rest[1:], stdout)
	case "key":
		return runKey(rt, rest[1:], stdout)
	case "task":
		return runTask(rt, rest[1:], stdout)
	case "collection":
		return runCollection(rt, rest[1:], stdout)
	case "export":
		return runExport(rt, rest[1:], stdout)
	case "tui":
		return runTUI(rt, rest[1:], stdout)
	case "whoami":
		return runWhoAmI(rt, rest[1:], stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", rest[0])
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "dooh (pronounced duo)")
	_, _ = fmt.Fprintln(w, "global flags: --profile <name> --config <path>")
	_, _ = fmt.Fprintln(w, "commands: config, db, setup, demo, login, env, context, user, key, task, collection, export, tui, whoami, version")
}

func parseGlobal(args []string) (globalOpts, []string, error) {
	opts := globalOpts{}
	if len(args) == 0 {
		return opts, nil, nil
	}

	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		a := args[i]
		switch {
		case a == "--profile":
			if i+1 >= len(args) {
				return opts, nil, errors.New("--profile requires a value")
			}
			opts.Profile = args[i+1]
			opts.ProfileSet = true
			i += 2
		case strings.HasPrefix(a, "--profile="):
			opts.Profile = strings.TrimPrefix(a, "--profile=")
			opts.ProfileSet = true
			i++
		case a == "--config":
			if i+1 >= len(args) {
				return opts, nil, errors.New("--config requires a value")
			}
			opts.ConfigPath = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--config="):
			opts.ConfigPath = strings.TrimPrefix(a, "--config=")
			i++
		default:
			return opts, nil, fmt.Errorf("unknown global flag %q", a)
		}
	}
	return opts, args[i:], nil
}

func runConfig(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("config subcommand required")
	}
	switch args[0] {
	case "show":
		p := rt.profile
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
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
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
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	_, _ = fmt.Fprintf(out, "client_type=%s\n", p.ClientType)
	return nil
}

func runDB(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("db subcommand required")
	}
	if args[0] != "init" {
		return fmt.Errorf("unknown db command %q", args[0])
	}
	fs := flag.NewFlagSet("db init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", "", "sqlite database path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	dbResolved := resolveDB(rt, *dbPath)

	sqlite := db.New(dbResolved)
	if err := initDatabase(sqlite); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "initialized database: %s\n", dbResolved)
	return nil
}

func runSetup(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("setup subcommand required")
	}
	switch args[0] {
	case "demo":
		fs := flag.NewFlagSet("setup demo", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		dbPath := fs.String("db", "", "sqlite database path")
		humanProfile := fs.String("human-profile", "human", "profile to store demo human key")
		agentProfile := fs.String("agent-profile", "ai", "profile to store demo ai key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		dbResolved := resolveDB(rt, *dbPath)
		sqlite := db.New(dbResolved)
		if err := initDatabase(sqlite); err != nil {
			return err
		}
		res, err := demo.Seed(sqlite)
		if err != nil {
			return err
		}
		humanID, err := userIDByName(sqlite, "Human Demo")
		if err != nil {
			return err
		}
		agentID, err := userIDByName(sqlite, "Agent Demo")
		if err != nil {
			return err
		}
		humanScopes := "tasks:read,tasks:write,tasks:delete,collections:read,collections:write,export:run,users:admin,keys:admin,system:rollback"
		agentScopes := "tasks:read,tasks:write,tasks:delete,collections:read,collections:write,export:run"
		humanKey, humanPrefix, err := createAPIKey(sqlite, humanID, "human_cli", humanScopes)
		if err != nil {
			return err
		}
		agentKey, agentPrefix, err := createAPIKey(sqlite, agentID, "agent_cli", agentScopes)
		if err != nil {
			return err
		}
		humanPath, err := writeStoredKey(*humanProfile, "human", humanKey)
		if err != nil {
			return err
		}
		agentPath, err := writeStoredKey(*agentProfile, "agent", agentKey)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "setup complete: db=%s users=%d collections=%d tasks=%d\n", dbResolved, res.Users, res.Collections, res.Tasks)
		_, _ = fmt.Fprintf(out, "stored human key %s in %s (profile=%s)\n", humanPrefix, humanPath, *humanProfile)
		_, _ = fmt.Fprintf(out, "stored ai key %s in %s (profile=%s)\n", agentPrefix, agentPath, *agentProfile)
		_, _ = fmt.Fprintf(out, "next: dooh --profile %s whoami && dooh --profile %s tui\n", *humanProfile, *humanProfile)
		return nil
	default:
		return fmt.Errorf("unknown setup command %q", args[0])
	}
}

func runLogin(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key to store for profile")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("usage: login --api-key <key> [--db <path>]")
	}
	if strings.TrimSpace(*apiKey) == "" {
		return errors.New("--api-key is required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := principalFromKey(sqlite, strings.TrimSpace(*apiKey))
	if err != nil {
		return errNoAuthContext
	}
	path, err := writeStoredKey(rt.opts.Profile, p.Actor, strings.TrimSpace(*apiKey))
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "stored %s key for profile %s in %s\n", displayActor(p.Actor), rt.opts.Profile, path)
	_, _ = fmt.Fprintf(out, "user=%s key=%s\n", p.UserID, p.KeyPrefix)
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

func runContext(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("context subcommand required: show|set|clear")
	}
	switch args[0] {
	case "show":
		path, err := contextFilePath()
		if err != nil {
			return err
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
	default:
		return fmt.Errorf("unknown context command %q", args[0])
	}
}

func runDemo(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("demo subcommand required")
	}
	switch args[0] {
	case "seed":
		fs := flag.NewFlagSet("demo seed", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		dbPath := fs.String("db", "", "sqlite database path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		res, err := demo.Seed(sqlite)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "seeded demo data: users=%d collections=%d tasks=%d\n", res.Users, res.Collections, res.Tasks)
		return nil
	default:
		return fmt.Errorf("unknown demo command %q", args[0])
	}
}

func runUser(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("user subcommand required")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("user create", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		name := fs.String("name", "", "user name")
		dbPath := fs.String("db", "", "sqlite database path")
		apiKey := fs.String("api-key", "", "api key")
		bootstrap := fs.Bool("bootstrap", false, "allow first admin user/key bootstrap when no keys exist")
		allowSystemAdmin := fs.Bool("allow-system-admin", false, "allow non-human system key for lifecycle admin actions")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *name == "" {
			return errors.New("--name is required")
		}
		sqlite := db.New(resolveDB(rt, *dbPath))

		count, err := countRows(sqlite, "SELECT COUNT(*) FROM api_keys;")
		if err != nil {
			return err
		}
		if count > 0 {
			p, err := mustAuth(rt, sqlite, *apiKey, true, "users:admin")
			if err != nil {
				return err
			}
			if err := requireHumanLifecycleAdmin(p, *allowSystemAdmin); err != nil {
				return err
			}
		} else if !*bootstrap {
			return errors.New("no keys exist; rerun with --bootstrap for first user")
		}

		id, err := idgen.ULIDLike()
		if err != nil {
			return err
		}
		sql := fmt.Sprintf("INSERT INTO users(id,name,status) VALUES(%s,%s,'active');", db.Quote(id), db.Quote(*name))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "created user %s (%s)\n", id, *name)
		return nil
	case "list":
		fs := flag.NewFlagSet("user list", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		dbPath := fs.String("db", "", "sqlite database path")
		apiKey := fs.String("api-key", "", "api key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		if _, err := mustReadAuth(rt, sqlite, *apiKey, "users:admin"); err != nil {
			return err
		}
		rows, err := sqlite.QueryTSV("SELECT id,name,status,created_at FROM users ORDER BY created_at;")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, style("NAME                 STATUS    CREATED                  USER_ID", "1"))
		_, _ = fmt.Fprintln(out, strings.Repeat("-", 92))
		for _, r := range rows {
			if len(r) >= 4 {
				_, _ = fmt.Fprintf(out, "%-20s %s %-24s %s\n", truncate(r[1], 20), statusCell(r[2], 9), truncate(r[3], 24), r[0])
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown user command %q", args[0])
	}
}

func runKey(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("key subcommand required")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("key create", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		user := fs.String("user", "", "user ID")
		scopes := fs.String("scopes", "", "comma-separated scopes")
		clientType := fs.String("client-type", "agent_cli", "human_cli|agent_cli|system")
		dbPath := fs.String("db", "", "sqlite database path")
		apiKey := fs.String("api-key", "", "admin api key")
		bootstrap := fs.Bool("bootstrap", false, "allow first key creation when no keys exist")
		allowSystemAdmin := fs.Bool("allow-system-admin", false, "allow non-human system key for lifecycle admin actions")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *user == "" {
			return errors.New("--user is required")
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		count, err := countRows(sqlite, "SELECT COUNT(*) FROM api_keys;")
		if err != nil {
			return err
		}
		if count > 0 {
			p, err := mustAuth(rt, sqlite, *apiKey, true, "keys:admin")
			if err != nil {
				return err
			}
			if err := requireHumanLifecycleAdmin(p, *allowSystemAdmin); err != nil {
				return err
			}
		} else if !*bootstrap {
			return errors.New("no keys exist; rerun with --bootstrap for first key")
		}

		plain, prefix, hash, err := auth.NewAPIKey()
		if err != nil {
			return err
		}
		id, err := idgen.ULIDLike()
		if err != nil {
			return err
		}
		sql := fmt.Sprintf("INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type) VALUES(%s,%s,%s,%s,%s,%s);",
			db.Quote(id), db.Quote(*user), db.Quote(prefix), db.Quote(hash), db.Quote(*scopes), db.Quote(*clientType))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "created key %s for user %s\n", prefix, *user)
		_, _ = fmt.Fprintf(out, "api_key=%s\n", plain)
		return nil
	case "revoke":
		fs := flag.NewFlagSet("key revoke", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		prefix := fs.String("prefix", "", "key prefix")
		dbPath := fs.String("db", "", "sqlite database path")
		apiKey := fs.String("api-key", "", "admin api key")
		allowSystemAdmin := fs.Bool("allow-system-admin", false, "allow non-human system key for lifecycle admin actions")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *prefix == "" {
			return errors.New("--prefix is required")
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		p, err := mustAuth(rt, sqlite, *apiKey, true, "keys:admin")
		if err != nil {
			return err
		}
		if err := requireHumanLifecycleAdmin(p, *allowSystemAdmin); err != nil {
			return err
		}
		sql := fmt.Sprintf("UPDATE api_keys SET revoked_at = strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ','now') WHERE key_prefix=%s AND revoked_at IS NULL;", db.Quote(*prefix))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "revoked key %s\n", *prefix)
		return nil
	default:
		return fmt.Errorf("unknown key command %q", args[0])
	}
}

func runTask(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("task subcommand required")
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("task add", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		title := fs.String("title", "", "title")
		priority := fs.String("priority", "later", "priority")
		dbPath := fs.String("db", "", "sqlite database path")
		apiKey := fs.String("api-key", "", "api key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *title == "" {
			return errors.New("--title is required")
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write")
		if err != nil {
			return err
		}
		printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
		id, err := idgen.ULIDLike()
		if err != nil {
			return err
		}
		shortID, err := idgen.Short("t")
		if err != nil {
			return err
		}
		sql := fmt.Sprintf("INSERT INTO tasks(id,short_id,title,priority,created_by,updated_by) VALUES(%s,%s,%s,%s,%s,%s);",
			db.Quote(id), db.Quote(shortID), db.Quote(*title), db.Quote(*priority), db.Quote(p.UserID), db.Quote(p.UserID))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "task.created", "task", id, map[string]string{"short_id": shortID, "title": *title}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "created task %s (%s)\n", shortID, *title)
		return nil
	case "list":
		fs := flag.NewFlagSet("task list", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		dbPath := fs.String("db", "", "sqlite database path")
		apiKey := fs.String("api-key", "", "api key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		if _, err := mustReadAuth(rt, sqlite, *apiKey, "tasks:read"); err != nil {
			return err
		}
		rows, err := sqlite.QueryTSV("SELECT title,status,priority,updated_at,short_id FROM tasks WHERE deleted_at IS NULL ORDER BY updated_at DESC;")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, style("TITLE                                     STATUS     PRIORITY UPDATED                  TASK_ID", "1"))
		_, _ = fmt.Fprintln(out, strings.Repeat("-", 100))
		for _, r := range rows {
			if len(r) >= 5 {
				_, _ = fmt.Fprintf(out, "%-40s %s %s %-24s %s\n",
					truncate(r[0], 40),
					statusCell(r[1], 10),
					priorityCell(r[2], 8),
					truncate(r[3], 24),
					r[4],
				)
			}
		}
		return nil
	case "complete":
		return runTaskStatus(rt, args[1:], out, "completed", "task.completed")
	case "reopen":
		return runTaskStatus(rt, args[1:], out, "open", "task.reopened")
	case "archive":
		return runTaskStatus(rt, args[1:], out, "archived", "task.archived")
	case "block":
		return runTaskBlock(rt, args[1:], out, true)
	case "unblock":
		return runTaskBlock(rt, args[1:], out, false)
	case "subtask":
		return runTaskSubtask(rt, args[1:], out)
	case "assign":
		return runTaskAssign(rt, args[1:], out)
	case "delete":
		fs := flag.NewFlagSet("task delete", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		target := fs.String("id", "", "task id or short id")
		dbPath := fs.String("db", "", "sqlite database path")
		apiKey := fs.String("api-key", "", "api key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *target == "" {
			return errors.New("--id is required")
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:delete")
		if err != nil {
			return err
		}
		printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
		sql := fmt.Sprintf("UPDATE tasks SET deleted_at = strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ','now'), updated_by=%s, version=version+1 WHERE (id=%s OR short_id=%s) AND deleted_at IS NULL;",
			db.Quote(p.UserID), db.Quote(*target), db.Quote(*target))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "task.deleted", "task", *target, map[string]string{"target": *target}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "deleted task %s\n", *target)
		return nil
	default:
		return fmt.Errorf("unknown task command %q", args[0])
	}
}

func runTaskStatus(rt runtime, args []string, out io.Writer, status string, eventName string) error {
	fs := flag.NewFlagSet("task status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "task id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return errors.New("--id is required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	taskID, shortID, _, err := resolveTask(sqlite, *target)
	if err != nil {
		return err
	}
	if status == "completed" {
		blocked, err := hasOpenBlockers(sqlite, taskID)
		if err != nil {
			return err
		}
		if blocked {
			return errors.New("cannot complete task while blockers are open")
		}
	}
	extra := ""
	if status == "completed" {
		extra = ", completed_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')"
	}
	if status == "archived" {
		extra = ", archived_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')"
	}
	if status == "open" {
		extra = ", completed_at = NULL, archived_at = NULL"
	}
	sql := fmt.Sprintf("UPDATE tasks SET status=%s, updated_by=%s, version=version+1 %s WHERE id=%s AND deleted_at IS NULL;",
		db.Quote(status), db.Quote(p.UserID), extra, db.Quote(taskID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, eventName, "task", taskID, map[string]string{"task_id": taskID, "short_id": shortID, "status": status}); err != nil {
		return err
	}
	if err := syncParentsForChild(sqlite, taskID, p.UserID); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "%s task %s\n", status, shortID)
	return nil
}

func runTaskBlock(rt runtime, args []string, out io.Writer, add bool) error {
	fs := flag.NewFlagSet("task block", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "task id or short id")
	by := fs.String("by", "", "blocking task id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" || *by == "" {
		return errors.New("--id and --by are required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	taskID, _, _, err := resolveTask(sqlite, *target)
	if err != nil {
		return err
	}
	blockerID, _, _, err := resolveTask(sqlite, *by)
	if err != nil {
		return err
	}
	if taskID == blockerID {
		return errors.New("task cannot block itself")
	}
	if add {
		hasPath, err := hasDependencyPath(sqlite, blockerID, taskID)
		if err != nil {
			return err
		}
		if hasPath {
			return errors.New("dependency cycle detected")
		}
		sql := fmt.Sprintf("INSERT OR IGNORE INTO task_dependencies(task_id,blocked_by_task_id) VALUES(%s,%s);", db.Quote(taskID), db.Quote(blockerID))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "task.blocked", "task", taskID, map[string]string{"task_id": taskID, "blocked_by_task_id": blockerID}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "task %s now blocked by %s\n", *target, *by)
		return nil
	}
	sql := fmt.Sprintf("DELETE FROM task_dependencies WHERE task_id=%s AND blocked_by_task_id=%s;", db.Quote(taskID), db.Quote(blockerID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "task.unblocked", "task", taskID, map[string]string{"task_id": taskID, "blocked_by_task_id": blockerID}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "task %s unblocked by %s\n", *target, *by)
	return nil
}

func runTaskSubtask(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("task subtask subcommand required: add|remove")
	}
	add := false
	switch args[0] {
	case "add":
		add = true
	case "remove":
		add = false
	default:
		return fmt.Errorf("unknown task subtask command %q", args[0])
	}
	fs := flag.NewFlagSet("task subtask", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	parent := fs.String("parent", "", "parent task id or short id")
	child := fs.String("child", "", "child task id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *parent == "" || *child == "" {
		return errors.New("--parent and --child are required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	parentID, _, _, err := resolveTask(sqlite, *parent)
	if err != nil {
		return err
	}
	childID, _, _, err := resolveTask(sqlite, *child)
	if err != nil {
		return err
	}
	if parentID == childID {
		return errors.New("task cannot be a subtask of itself")
	}
	if add {
		hasPath, err := hasSubtaskPath(sqlite, childID, parentID)
		if err != nil {
			return err
		}
		if hasPath {
			return errors.New("subtask cycle detected")
		}
		sql := fmt.Sprintf("INSERT OR IGNORE INTO task_subtasks(parent_task_id,child_task_id) VALUES(%s,%s);", db.Quote(parentID), db.Quote(childID))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "task.subtask_added", "task", parentID, map[string]string{"parent_task_id": parentID, "child_task_id": childID}); err != nil {
			return err
		}
		if err := syncParentStatus(sqlite, parentID, p.UserID); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "added subtask %s -> %s\n", *parent, *child)
		return nil
	}
	sql := fmt.Sprintf("DELETE FROM task_subtasks WHERE parent_task_id=%s AND child_task_id=%s;", db.Quote(parentID), db.Quote(childID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "task.subtask_removed", "task", parentID, map[string]string{"parent_task_id": parentID, "child_task_id": childID}); err != nil {
		return err
	}
	if err := syncParentStatus(sqlite, parentID, p.UserID); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "removed subtask %s -> %s\n", *parent, *child)
	return nil
}

func runTaskAssign(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("task assign subcommand required: add|remove")
	}
	add := false
	switch args[0] {
	case "add":
		add = true
	case "remove":
		add = false
	default:
		return fmt.Errorf("unknown task assign command %q", args[0])
	}
	fs := flag.NewFlagSet("task assign", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "task id or short id")
	user := fs.String("user", "", "user id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *target == "" || *user == "" {
		return errors.New("--id and --user are required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "tasks:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	taskID, _, _, err := resolveTask(sqlite, *target)
	if err != nil {
		return err
	}
	ok, err := userExists(sqlite, *user)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("unknown user %s", *user)
	}
	if add {
		sql := fmt.Sprintf("INSERT OR IGNORE INTO task_assignees(task_id,user_id) VALUES(%s,%s);", db.Quote(taskID), db.Quote(*user))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "task.assignee_added", "task", taskID, map[string]string{"task_id": taskID, "user_id": *user}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "assigned task %s to %s\n", *target, *user)
		return nil
	}
	sql := fmt.Sprintf("DELETE FROM task_assignees WHERE task_id=%s AND user_id=%s;", db.Quote(taskID), db.Quote(*user))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "task.assignee_removed", "task", taskID, map[string]string{"task_id": taskID, "user_id": *user}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "unassigned task %s from %s\n", *target, *user)
	return nil
}

func runCollection(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("collection subcommand required")
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("collection add", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		name := fs.String("name", "", "name")
		kind := fs.String("kind", "project", "kind")
		color := fs.String("color", "", "hex color")
		dbPath := fs.String("db", "", "sqlite database path")
		apiKey := fs.String("api-key", "", "api key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *name == "" {
			return errors.New("--name is required")
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		p, err := mustAuth(rt, sqlite, *apiKey, false, "collections:write")
		if err != nil {
			return err
		}
		printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
		id, err := idgen.ULIDLike()
		if err != nil {
			return err
		}
		shortID, err := idgen.Short("c")
		if err != nil {
			return err
		}
		col := strings.TrimSpace(*color)
		if col == "" {
			col = palette[int(time.Now().UnixNano())%len(palette)]
		}
		sql := fmt.Sprintf("INSERT INTO collections(id,short_id,name,kind,color_hex,created_by,updated_by) VALUES(%s,%s,%s,%s,%s,%s,%s);",
			db.Quote(id), db.Quote(shortID), db.Quote(*name), db.Quote(*kind), db.Quote(col), db.Quote(p.UserID), db.Quote(p.UserID))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "collection.created", "collection", id, map[string]string{"short_id": shortID, "name": *name}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "created collection %s (%s) color=%s\n", shortID, *name, col)
		return nil
	case "list":
		fs := flag.NewFlagSet("collection list", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		dbPath := fs.String("db", "", "sqlite database path")
		apiKey := fs.String("api-key", "", "api key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		if _, err := mustReadAuth(rt, sqlite, *apiKey, "collections:read"); err != nil {
			return err
		}
		rows, err := sqlite.QueryTSV("SELECT short_id,name,kind,color_hex,updated_at FROM collections WHERE deleted_at IS NULL ORDER BY updated_at DESC;")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, style("NAME                 KIND       COLOR      UPDATED                  COLLECTION_ID", "1"))
		_, _ = fmt.Fprintln(out, strings.Repeat("-", 92))
		for _, r := range rows {
			if len(r) >= 5 {
				_, _ = fmt.Fprintf(out, "%-20s %-10s %-10s %-24s %s\n",
					truncate(r[1], 20),
					kindCell(r[2], 10),
					style(truncate(r[3], 10), "38;5;45"),
					truncate(r[4], 24),
					r[0],
				)
			}
		}
		return nil
	case "link":
		return runCollectionLink(rt, args[1:], out, true)
	case "unlink":
		return runCollectionLink(rt, args[1:], out, false)
	default:
		return fmt.Errorf("unknown collection command %q", args[0])
	}
}

func runCollectionLink(rt runtime, args []string, out io.Writer, add bool) error {
	fs := flag.NewFlagSet("collection link", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	parent := fs.String("parent", "", "parent collection id or short id")
	child := fs.String("child", "", "child collection id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *parent == "" || *child == "" {
		return errors.New("--parent and --child are required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, *apiKey, false, "collections:write")
	if err != nil {
		return err
	}
	printWriteContext(out, rt, resolveDB(rt, *dbPath), p)
	parentID, _, err := resolveCollection(sqlite, *parent)
	if err != nil {
		return err
	}
	childID, _, err := resolveCollection(sqlite, *child)
	if err != nil {
		return err
	}
	if parentID == childID {
		return errors.New("collection cannot link to itself")
	}
	if add {
		hasPath, err := hasCollectionPath(sqlite, childID, parentID)
		if err != nil {
			return err
		}
		if hasPath {
			return errors.New("collection hierarchy cycle detected")
		}
		sql := fmt.Sprintf("INSERT OR IGNORE INTO collection_links(parent_collection_id,child_collection_id) VALUES(%s,%s);", db.Quote(parentID), db.Quote(childID))
		if err := sqlite.Exec(sql); err != nil {
			return err
		}
		if err := rebuildCollectionClosure(sqlite); err != nil {
			return err
		}
		if err := writeEvent(sqlite, p, "collection.linked", "collection", parentID, map[string]string{"parent_collection_id": parentID, "child_collection_id": childID}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "linked collection %s -> %s\n", *parent, *child)
		return nil
	}
	sql := fmt.Sprintf("DELETE FROM collection_links WHERE parent_collection_id=%s AND child_collection_id=%s;", db.Quote(parentID), db.Quote(childID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := rebuildCollectionClosure(sqlite); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "collection.unlinked", "collection", parentID, map[string]string{"parent_collection_id": parentID, "child_collection_id": childID}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "unlinked collection %s -> %s\n", *parent, *child)
	return nil
}

func runExport(rt runtime, args []string, out io.Writer) error {
	if len(args) < 1 || args[0] != "site" {
		return errors.New("usage: export site --out <dir>")
	}
	fs := flag.NewFlagSet("export site", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outDir := fs.String("out", "", "output directory")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	resolvedOut := strings.TrimSpace(*outDir)
	if resolvedOut == "" {
		resolvedOut = rt.profile.ExportDir
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	if _, err := mustReadAuth(rt, sqlite, *apiKey, "export:run"); err != nil {
		return err
	}
	if err := exporter.ExportSite(sqlite, resolvedOut); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "exported site data to %s\n", resolvedOut)
	return nil
}

func runTUI(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	theme := fs.String("theme", "", "theme")
	listThemes := fs.Bool("list-themes", false, "list theme presets")
	filter := fs.String("filter", "", "filter tasks by text")
	limit := fs.Int("limit", 12, "max tasks to display")
	static := fs.Bool("static", false, "render once and exit")
	plain := fs.Bool("plain", false, "disable ANSI and render plain table")
	renderer := fs.String("renderer", "auto", "renderer: auto|legacy|tea")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	catalog, err := tui.LoadThemes("internal/tui/themes/presets.json")
	if err != nil {
		return err
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustReadAuth(rt, sqlite, *apiKey, "tasks:read", "collections:read")
	if err != nil {
		return err
	}
	identity := tui.Identity{Actor: displayActor(p.Actor), UserID: p.UserID, UserName: p.UserName}
	if *listThemes {
		for _, item := range catalog.Themes {
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", item.ID, item.Name, item.Description)
		}
		return nil
	}

	selected := strings.TrimSpace(*theme)
	selected = resolveTheme(rt, selected)
	if selected == "" {
		selected = catalog.Default
	}
	chosen, ok := catalog.ThemeByID(selected)
	if !ok {
		ids := make([]string, 0, len(catalog.Themes))
		for _, item := range catalog.Themes {
			ids = append(ids, item.ID)
		}
		return fmt.Errorf("unknown theme %q (available: %s)", selected, strings.Join(ids, ", "))
	}
	loc, err := time.LoadLocation(rt.profile.Timezone)
	if err != nil {
		loc = time.Local
	}
	if *static {
		panel, err := tui.RenderDashboard(sqlite, chosen, *filter, *limit, loc, identity, *plain)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, panel)
		return nil
	}
	if *plain {
		panel, err := tui.RenderDashboard(sqlite, chosen, *filter, *limit, loc, identity, true)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, panel)
		return nil
	}
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
		panel, err := tui.RenderDashboard(sqlite, chosen, *filter, *limit, loc, identity, true)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(out, panel)
		return nil
	}
	r := strings.ToLower(strings.TrimSpace(*renderer))
	switch r {
	case "", "auto", "tea":
		if err := tui.RunInteractiveTea(os.Stdin, out, sqlite, catalog, chosen.ID, *filter, *limit, loc, identity, false); err != nil {
			if r == "tea" {
				return err
			}
		} else {
			return nil
		}
		return tui.RunInteractive(os.Stdin, out, sqlite, catalog, chosen.ID, *filter, *limit, loc, identity, false)
	case "legacy":
		return tui.RunInteractive(os.Stdin, out, sqlite, catalog, chosen.ID, *filter, *limit, loc, identity, false)
	default:
		return errors.New("--renderer must be auto, legacy, or tea")
	}
}

func resolveDB(rt runtime, flagVal string) string {
	if strings.TrimSpace(flagVal) != "" {
		return flagVal
	}
	if v := strings.TrimSpace(os.Getenv("DOOH_DB")); v != "" {
		return v
	}
	if strings.TrimSpace(rt.context.DB) != "" {
		return strings.TrimSpace(rt.context.DB)
	}
	return rt.profile.DB
}

func resolveTheme(rt runtime, flagVal string) string {
	if strings.TrimSpace(flagVal) != "" {
		return strings.TrimSpace(flagVal)
	}
	if strings.TrimSpace(rt.context.Theme) != "" {
		return strings.TrimSpace(rt.context.Theme)
	}
	return strings.TrimSpace(rt.profile.Theme)
}

func printWriteContext(out io.Writer, rt runtime, dbPath string, p principal) {
	_, _ = fmt.Fprintf(out, "%s profile=%s mode=%s user=%s key=%s db=%s\n",
		style("context", "2"),
		rt.opts.Profile,
		displayActor(p.Actor),
		p.UserID,
		p.KeyPrefix,
		dbPath,
	)
	if rt.aiProfileEnforced && p.Actor == "agent" {
		_, _ = fmt.Fprintln(out, "ai context active (profile auto-set to ai)")
	}
}

func mustReadAuth(rt runtime, sqlite db.SQLite, keyFromFlag string, neededScopes ...string) (principal, error) {
	return mustAuth(rt, sqlite, keyFromFlag, false, neededScopes...)
}

func initDatabase(sqlite db.SQLite) error {
	_, migration, err := readInitMigration()
	if err != nil {
		return err
	}
	if err := sqlite.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return err
	}
	return sqlite.Exec(string(migration))
}

func readInitMigration() (string, []byte, error) {
	candidates := []string{
		filepath.Join("migrations", "0001_init.sql"),
		filepath.Join("..", "migrations", "0001_init.sql"),
		filepath.Join("..", "..", "migrations", "0001_init.sql"),
	}
	if _, file, _, ok := goruntime.Caller(0); ok {
		base := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
		candidates = append([]string{filepath.Join(base, "migrations", "0001_init.sql")}, candidates...)
	}
	var lastErr error
	for _, path := range candidates {
		b, err := os.ReadFile(path)
		if err == nil {
			return path, b, nil
		}
		lastErr = err
	}
	return "", nil, fmt.Errorf("read migration migrations/0001_init.sql: %w", lastErr)
}

func userIDByName(sqlite db.SQLite, name string) (string, error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT id FROM users WHERE name=%s AND status='active' LIMIT 1;", db.Quote(name)))
	if err != nil {
		return "", err
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return "", fmt.Errorf("missing active user %q", name)
	}
	return rows[0][0], nil
}

func createAPIKey(sqlite db.SQLite, userID string, clientType string, scopes string) (plain string, prefix string, err error) {
	plain, prefix, hash, err := auth.NewAPIKey()
	if err != nil {
		return "", "", err
	}
	id, err := idgen.ULIDLike()
	if err != nil {
		return "", "", err
	}
	sql := fmt.Sprintf("INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type) VALUES(%s,%s,%s,%s,%s,%s);",
		db.Quote(id), db.Quote(userID), db.Quote(prefix), db.Quote(hash), db.Quote(scopes), db.Quote(clientType))
	if err := sqlite.Exec(sql); err != nil {
		return "", "", err
	}
	return plain, prefix, nil
}

func authStoreDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dooh", "auth"), nil
}

func contextFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dooh", "context.json"), nil
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

func keyFilePath(profile string, actor string) (string, error) {
	actor = normalizeActor(actor)
	if actor != "human" && actor != "agent" {
		return "", fmt.Errorf("invalid actor %q", actor)
	}
	p := strings.TrimSpace(profile)
	if p == "" {
		p = "default"
	}
	dir, err := authStoreDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%s.%s.key", p, actor)), nil
}

func writeStoredKey(profile string, actor string, key string) (string, error) {
	path, err := keyFilePath(profile, actor)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(key)+"\n"), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func readStoredKey(profile string, actor string) (key string, path string, err error) {
	path, err = keyFilePath(profile, actor)
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", path, nil
		}
		return "", path, err
	}
	return strings.TrimSpace(string(b)), path, nil
}

func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func principalFromKey(sqlite db.SQLite, key string) (principal, error) {
	var p principal
	hash := auth.HashAPIKey(key)
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT k.id,k.user_id,k.key_prefix,k.scopes,k.client_type,u.name FROM api_keys k JOIN users u ON u.id=k.user_id WHERE k.key_hash=%s AND k.revoked_at IS NULL AND u.status='active' LIMIT 1;", db.Quote(hash)))
	if err != nil {
		return p, err
	}
	if len(rows) == 0 || len(rows[0]) < 6 {
		return p, errNoAuthContext
	}
	clientType := strings.TrimSpace(rows[0][4])
	actor, ok := actorFromClientType(clientType)
	if !ok {
		return principal{}, fmt.Errorf("key client_type %s is not interactive", clientType)
	}
	p = principal{UserID: rows[0][1], UserName: rows[0][5], KeyID: rows[0][0], KeyPrefix: rows[0][2], Actor: actor, ClientType: clientType, Scopes: parseScopes(rows[0][3])}
	return p, nil
}

func mustAuth(rt runtime, sqlite db.SQLite, keyFromFlag string, requireHumanTTY bool, neededScopes ...string) (principal, error) {
	var p principal
	key := strings.TrimSpace(keyFromFlag)
	if key != "" {
		p, err := principalFromKey(sqlite, key)
		if err != nil {
			return principal{}, err
		}
		for _, need := range neededScopes {
			if !p.Scopes[need] {
				return principal{}, fmt.Errorf("missing required scope %q", need)
			}
		}
		return p, nil
	}

	mode := normalizeActor(strings.TrimSpace(os.Getenv("DOOH_MODE")))
	if mode == "agent" {
		key = firstNonEmpty(strings.TrimSpace(os.Getenv("DOOH_AI_KEY")), envKeyValue(rt.profile.APIKeyEnv))
		if key == "" {
			stored, _, err := readStoredKey(rt.opts.Profile, "agent")
			if err != nil {
				return p, err
			}
			key = stored
		}
		if key == "" {
			return p, errNoAuthContext
		}
	} else if mode == "human" {
		if key == "" {
			stored, _, err := readStoredKey(rt.opts.Profile, "human")
			if err != nil {
				return p, err
			}
			key = stored
		}
		if key == "" {
			return p, errNoAuthContext
		}
	} else {
		key = firstNonEmpty(strings.TrimSpace(os.Getenv("DOOH_AI_KEY")), envKeyValue(rt.profile.APIKeyEnv))
		if key == "" {
			if stored, _, err := readStoredKey(rt.opts.Profile, "human"); err != nil {
				return p, err
			} else if stored != "" {
				key = stored
			}
		}
		if key == "" {
			if stored, _, err := readStoredKey(rt.opts.Profile, "agent"); err != nil {
				return p, err
			} else if stored != "" {
				key = stored
			}
		}
		if key == "" {
			return p, errNoAuthContext
		}
	}
	p, err := principalFromKey(sqlite, key)
	if err != nil {
		return principal{}, err
	}
	if requireHumanTTY && p.Actor == "human" {
		if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
			return p, errors.New("human actor requires interactive terminal")
		}
	}
	for _, need := range neededScopes {
		if !p.Scopes[need] {
			return principal{}, fmt.Errorf("missing required scope %q", need)
		}
	}
	return p, nil
}

func normalizeActor(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "ai":
		return "agent"
	case "human":
		return "human"
	case "agent":
		return "agent"
	default:
		return ""
	}
}

func displayActor(v string) string {
	if normalizeActor(v) == "agent" {
		return "ai"
	}
	return "human"
}

func requireHumanLifecycleAdmin(p principal, allowSystemAdmin bool) error {
	if p.Actor == "human" {
		return nil
	}
	if p.ClientType == "system" && allowSystemAdmin {
		return nil
	}
	return errors.New("lifecycle admin actions require human actor (or --allow-system-admin with system key)")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func envKeyValue(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		n = "DOOH_API_KEY"
	}
	return strings.TrimSpace(os.Getenv(n))
}

func hasAIEnvKey() bool {
	return strings.TrimSpace(os.Getenv("DOOH_AI_KEY")) != "" || strings.TrimSpace(os.Getenv("DOOH_API_KEY")) != ""
}

func resolvePrincipalForShow(rt runtime, sqlite db.SQLite) (principal, string, bool) {
	if k := strings.TrimSpace(os.Getenv("DOOH_AI_KEY")); k != "" {
		if p, err := principalFromKey(sqlite, k); err == nil {
			return p, "env:DOOH_AI_KEY", true
		}
	}
	if envName := strings.TrimSpace(rt.profile.APIKeyEnv); envName != "" {
		if k := strings.TrimSpace(os.Getenv(envName)); k != "" {
			if p, err := principalFromKey(sqlite, k); err == nil {
				return p, "env:" + envName, true
			}
		}
	}
	if k := strings.TrimSpace(os.Getenv("DOOH_API_KEY")); k != "" {
		if p, err := principalFromKey(sqlite, k); err == nil {
			return p, "env:DOOH_API_KEY", true
		}
	}
	if k, _, err := readStoredKey(rt.opts.Profile, "human"); err == nil && k != "" {
		if p, err := principalFromKey(sqlite, k); err == nil {
			return p, "stored:human", true
		}
	}
	if k, _, err := readStoredKey(rt.opts.Profile, "agent"); err == nil && k != "" {
		if p, err := principalFromKey(sqlite, k); err == nil {
			return p, "stored:ai", true
		}
	}
	return principal{}, "", false
}

func actorFromClientType(clientType string) (string, bool) {
	switch strings.TrimSpace(clientType) {
	case "human_cli":
		return "human", true
	case "agent_cli":
		return "agent", true
	default:
		return "", false
	}
}

func parseScopes(v string) map[string]bool {
	out := map[string]bool{}
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out[s] = true
		}
	}
	return out
}

func countRows(sqlite db.SQLite, query string) (int, error) {
	rows, err := sqlite.QueryTSV(query)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return 0, nil
	}
	var n int
	_, err = fmt.Sscanf(rows[0][0], "%d", &n)
	if err != nil {
		return 0, fmt.Errorf("parse count: %w", err)
	}
	return n, nil
}

func resolveTask(sqlite db.SQLite, target string) (id string, shortID string, title string, err error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT id,short_id,title FROM tasks WHERE (id=%s OR short_id=%s) AND deleted_at IS NULL LIMIT 1;", db.Quote(target), db.Quote(target)))
	if err != nil {
		return "", "", "", err
	}
	if len(rows) == 0 || len(rows[0]) < 3 {
		return "", "", "", fmt.Errorf("unknown task %s", target)
	}
	return rows[0][0], rows[0][1], rows[0][2], nil
}

func resolveCollection(sqlite db.SQLite, target string) (id string, shortID string, err error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT id,short_id FROM collections WHERE (id=%s OR short_id=%s) AND deleted_at IS NULL LIMIT 1;", db.Quote(target), db.Quote(target)))
	if err != nil {
		return "", "", err
	}
	if len(rows) == 0 || len(rows[0]) < 2 {
		return "", "", fmt.Errorf("unknown collection %s", target)
	}
	return rows[0][0], rows[0][1], nil
}

func userExists(sqlite db.SQLite, userID string) (bool, error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT 1 FROM users WHERE id=%s AND status='active' LIMIT 1;", db.Quote(userID)))
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func hasOpenBlockers(sqlite db.SQLite, taskID string) (bool, error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT 1 FROM task_dependencies td JOIN tasks b ON b.id=td.blocked_by_task_id WHERE td.task_id=%s AND b.deleted_at IS NULL AND b.status!='completed' LIMIT 1;", db.Quote(taskID)))
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func hasDependencyPath(sqlite db.SQLite, startTaskID string, targetTaskID string) (bool, error) {
	query := fmt.Sprintf(`
WITH RECURSIVE walk(id) AS (
  SELECT blocked_by_task_id FROM task_dependencies WHERE task_id=%s
  UNION
  SELECT td.blocked_by_task_id FROM task_dependencies td JOIN walk w ON td.task_id=w.id
)
SELECT 1 FROM walk WHERE id=%s LIMIT 1;`, db.Quote(startTaskID), db.Quote(targetTaskID))
	rows, err := sqlite.QueryTSV(query)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func hasSubtaskPath(sqlite db.SQLite, startTaskID string, targetTaskID string) (bool, error) {
	query := fmt.Sprintf(`
WITH RECURSIVE walk(id) AS (
  SELECT child_task_id FROM task_subtasks WHERE parent_task_id=%s
  UNION
  SELECT ts.child_task_id FROM task_subtasks ts JOIN walk w ON ts.parent_task_id=w.id
)
SELECT 1 FROM walk WHERE id=%s LIMIT 1;`, db.Quote(startTaskID), db.Quote(targetTaskID))
	rows, err := sqlite.QueryTSV(query)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func hasCollectionPath(sqlite db.SQLite, startCollectionID string, targetCollectionID string) (bool, error) {
	query := fmt.Sprintf(`
WITH RECURSIVE walk(id) AS (
  SELECT child_collection_id FROM collection_links WHERE parent_collection_id=%s
  UNION
  SELECT cl.child_collection_id FROM collection_links cl JOIN walk w ON cl.parent_collection_id=w.id
)
SELECT 1 FROM walk WHERE id=%s LIMIT 1;`, db.Quote(startCollectionID), db.Quote(targetCollectionID))
	rows, err := sqlite.QueryTSV(query)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func rebuildCollectionClosure(sqlite db.SQLite) error {
	sql := `
BEGIN;
DELETE FROM collection_closure;
INSERT INTO collection_closure(ancestor_collection_id, descendant_collection_id, depth)
SELECT id, id, 0 FROM collections WHERE deleted_at IS NULL;
WITH RECURSIVE paths(ancestor_id, descendant_id, depth) AS (
  SELECT parent_collection_id, child_collection_id, 1 FROM collection_links
  UNION ALL
  SELECT p.ancestor_id, cl.child_collection_id, p.depth + 1
  FROM paths p
  JOIN collection_links cl ON cl.parent_collection_id = p.descendant_id
)
INSERT OR REPLACE INTO collection_closure(ancestor_collection_id, descendant_collection_id, depth)
SELECT ancestor_id, descendant_id, MIN(depth)
FROM paths
GROUP BY ancestor_id, descendant_id;
COMMIT;`
	return sqlite.Exec(sql)
}

func syncParentsForChild(sqlite db.SQLite, childTaskID string, actorUserID string) error {
	parents, err := sqlite.QueryTSV(fmt.Sprintf("SELECT parent_task_id FROM task_subtasks WHERE child_task_id=%s;", db.Quote(childTaskID)))
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range parents {
		if len(r) == 0 {
			continue
		}
		if err := syncParentStatusRecursive(sqlite, r[0], actorUserID, seen); err != nil {
			return err
		}
	}
	return nil
}

func syncParentStatus(sqlite db.SQLite, parentTaskID string, actorUserID string) error {
	return syncParentStatusRecursive(sqlite, parentTaskID, actorUserID, map[string]bool{})
}

func syncParentStatusRecursive(sqlite db.SQLite, parentTaskID string, actorUserID string, seen map[string]bool) error {
	if seen[parentTaskID] {
		return nil
	}
	seen[parentTaskID] = true

	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT COUNT(*), SUM(CASE WHEN c.status='completed' THEN 1 ELSE 0 END) FROM task_subtasks ts JOIN tasks c ON c.id=ts.child_task_id WHERE ts.parent_task_id=%s AND c.deleted_at IS NULL;", db.Quote(parentTaskID)))
	if err != nil {
		return err
	}
	if len(rows) > 0 && len(rows[0]) >= 2 {
		total := parseIntDefault(rows[0][0], 0)
		done := parseIntDefault(rows[0][1], 0)
		if total > 0 {
			if done == total {
				sql := fmt.Sprintf("UPDATE tasks SET status='completed', completed_at=COALESCE(completed_at,strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ','now')), updated_by=%s, version=version+1 WHERE id=%s AND deleted_at IS NULL;", db.Quote(actorUserID), db.Quote(parentTaskID))
				if err := sqlite.Exec(sql); err != nil {
					return err
				}
			} else {
				sql := fmt.Sprintf("UPDATE tasks SET status='open', completed_at=NULL, updated_by=%s, version=version+1 WHERE id=%s AND deleted_at IS NULL AND status='completed';", db.Quote(actorUserID), db.Quote(parentTaskID))
				if err := sqlite.Exec(sql); err != nil {
					return err
				}
			}
		}
	}
	parents, err := sqlite.QueryTSV(fmt.Sprintf("SELECT parent_task_id FROM task_subtasks WHERE child_task_id=%s;", db.Quote(parentTaskID)))
	if err != nil {
		return err
	}
	for _, r := range parents {
		if len(r) == 0 {
			continue
		}
		if err := syncParentStatusRecursive(sqlite, r[0], actorUserID, seen); err != nil {
			return err
		}
	}
	return nil
}

func parseIntDefault(v string, def int) int {
	if strings.TrimSpace(v) == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func style(text string, code string) string {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func colorizeStatus(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "completed":
		return style("completed", "38;5;78")
	case "archived":
		return style("archived", "38;5;179")
	case "open":
		return style("open", "38;5;81")
	default:
		return v
	}
}

func colorizePriority(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "now":
		return style("now", "38;5;203")
	case "soon":
		return style("soon", "38;5;221")
	case "later":
		return style("later", "38;5;111")
	default:
		return v
	}
}

func colorizeKind(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "project":
		return style("project", "38;5;75")
	case "goal":
		return style("goal", "38;5;220")
	case "tag":
		return style("tag", "38;5;43")
	case "class":
		return style("class", "38;5;119")
	case "area":
		return style("area", "38;5;209")
	default:
		return v
	}
}

func statusCell(v string, width int) string {
	raw := fmt.Sprintf("%-*s", width, strings.ToLower(strings.TrimSpace(v)))
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "completed":
		return style(raw, "38;5;78")
	case "archived":
		return style(raw, "38;5;179")
	case "open":
		return style(raw, "38;5;81")
	default:
		return raw
	}
}

func priorityCell(v string, width int) string {
	raw := fmt.Sprintf("%-*s", width, strings.ToLower(strings.TrimSpace(v)))
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "now":
		return style(raw, "38;5;203")
	case "soon":
		return style(raw, "38;5;221")
	case "later":
		return style(raw, "38;5;111")
	default:
		return raw
	}
}

func kindCell(v string, width int) string {
	raw := fmt.Sprintf("%-*s", width, strings.ToLower(strings.TrimSpace(v)))
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "project":
		return style(raw, "38;5;75")
	case "goal":
		return style(raw, "38;5;220")
	case "tag":
		return style(raw, "38;5;43")
	case "class":
		return style(raw, "38;5;119")
	case "area":
		return style(raw, "38;5;209")
	default:
		return raw
	}
}

func writeEvent(sqlite db.SQLite, p principal, eventType string, aggregateType string, aggregateID string, payload any) error {
	if strings.TrimSpace(p.UserID) == "" {
		return errors.New("missing actor user for event")
	}
	eventID, err := idgen.ULIDLike()
	if err != nil {
		return err
	}
	outboxID, err := idgen.ULIDLike()
	if err != nil {
		return err
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	payloadStr := string(b)
	topic := "website." + eventType
	sql := fmt.Sprintf("BEGIN; INSERT INTO events(id,event_type,aggregate_type,aggregate_id,actor_user_id,key_id,client_type,payload_json) VALUES(%s,%s,%s,%s,%s,%s,%s,%s); INSERT INTO outbox(id,event_id,topic,payload_json,status,available_at) VALUES(%s,%s,%s,%s,'pending',strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ','now')); COMMIT;",
		db.Quote(eventID), db.Quote(eventType), db.Quote(aggregateType), db.Quote(aggregateID), db.Quote(p.UserID), db.Quote(p.KeyID), db.Quote(p.ClientType), db.Quote(payloadStr),
		db.Quote(outboxID), db.Quote(eventID), db.Quote(topic), db.Quote(payloadStr),
	)
	return sqlite.Exec(sql)
}
