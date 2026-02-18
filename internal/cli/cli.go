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
	KeyID      string
	ClientType string
	Scopes     map[string]bool
}

type globalOpts struct {
	Profile    string
	ConfigPath string
}

type runtime struct {
	opts    globalOpts
	config  config.Config
	profile config.Profile
}

var palette = []string{"#FF7A59", "#FFD166", "#2EC4B6", "#4D96FF", "#FF6B6B", "#70E000", "#00E5FF"}

// Run executes dooh CLI commands using stdlib and sqlite3 CLI.
func Run(args []string, stdout io.Writer) error {
	opts, rest, err := parseGlobal(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	rt := runtime{opts: opts, config: cfg, profile: config.Resolve(cfg, opts.Profile)}

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
	case "demo":
		return runDemo(rt, rest[1:], stdout)
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
	_, _ = fmt.Fprintln(w, "commands: config, db, demo, user, key, task, collection, export, tui, version")
}

func parseGlobal(args []string) (globalOpts, []string, error) {
	opts := globalOpts{Profile: strings.TrimSpace(os.Getenv("DOOH_PROFILE"))}
	if opts.Profile == "" {
		opts.Profile = "default"
	}
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
			i += 2
		case strings.HasPrefix(a, "--profile="):
			opts.Profile = strings.TrimPrefix(a, "--profile=")
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
		_, _ = fmt.Fprintf(out, "actor=%s\n", p.Actor)
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
actor = "agent"
timezone = "America/Los_Angeles"
theme = "sunset-pop"
export_dir = "./site-data"
api_key_env = "DOOH_API_KEY"

[profile.human]
actor = "human"
theme = "paper-fruit"

[profile.agent]
actor = "agent"
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
	migrationPath := filepath.Join("migrations", "0001_init.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", migrationPath, err)
	}
	if err := sqlite.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return err
	}
	if err := sqlite.Exec(string(migration)); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "initialized database: %s\n", dbResolved)
	return nil
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
			if _, err := mustAuth(rt, sqlite, "human", *apiKey, true, "users:admin"); err != nil {
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
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		rows, err := db.New(resolveDB(rt, *dbPath)).QueryTSV("SELECT id,name,status,created_at FROM users ORDER BY created_at;")
		if err != nil {
			return err
		}
		for _, r := range rows {
			if len(r) >= 4 {
				_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", r[0], r[1], r[2], r[3])
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
			if _, err := mustAuth(rt, sqlite, "human", *apiKey, true, "keys:admin"); err != nil {
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
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *prefix == "" {
			return errors.New("--prefix is required")
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		if _, err := mustAuth(rt, sqlite, "human", *apiKey, true, "keys:admin"); err != nil {
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
		actor := fs.String("actor", "", "human|agent")
		apiKey := fs.String("api-key", "", "api key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *title == "" {
			return errors.New("--title is required")
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		p, err := mustAuth(rt, sqlite, resolveActor(rt, *actor), *apiKey, false, "tasks:write")
		if err != nil {
			return err
		}
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
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		rows, err := db.New(resolveDB(rt, *dbPath)).QueryTSV("SELECT short_id,title,status,priority,updated_at FROM tasks WHERE deleted_at IS NULL ORDER BY updated_at DESC;")
		if err != nil {
			return err
		}
		for _, r := range rows {
			if len(r) >= 5 {
				_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n", r[0], r[1], r[2], r[3], r[4])
			}
		}
		return nil
	case "complete":
		return runTaskStatus(rt, args[1:], out, "completed", "task.completed")
	case "archive":
		return runTaskStatus(rt, args[1:], out, "archived", "task.archived")
	case "delete":
		fs := flag.NewFlagSet("task delete", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		target := fs.String("id", "", "task id or short id")
		dbPath := fs.String("db", "", "sqlite database path")
		actor := fs.String("actor", "", "human|agent")
		apiKey := fs.String("api-key", "", "api key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *target == "" {
			return errors.New("--id is required")
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		p, err := mustAuth(rt, sqlite, resolveActor(rt, *actor), *apiKey, false, "tasks:delete")
		if err != nil {
			return err
		}
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
	actor := fs.String("actor", "", "human|agent")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return errors.New("--id is required")
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	p, err := mustAuth(rt, sqlite, resolveActor(rt, *actor), *apiKey, false, "tasks:write")
	if err != nil {
		return err
	}
	extra := ""
	if status == "completed" {
		extra = ", completed_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')"
	}
	if status == "archived" {
		extra = ", archived_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')"
	}
	sql := fmt.Sprintf("UPDATE tasks SET status=%s, updated_by=%s, version=version+1 %s WHERE (id=%s OR short_id=%s) AND deleted_at IS NULL;",
		db.Quote(status), db.Quote(p.UserID), extra, db.Quote(*target), db.Quote(*target))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, eventName, "task", *target, map[string]string{"target": *target, "status": status}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "%s task %s\n", status, *target)
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
		actor := fs.String("actor", "", "human|agent")
		apiKey := fs.String("api-key", "", "api key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *name == "" {
			return errors.New("--name is required")
		}
		sqlite := db.New(resolveDB(rt, *dbPath))
		p, err := mustAuth(rt, sqlite, resolveActor(rt, *actor), *apiKey, false, "collections:write")
		if err != nil {
			return err
		}
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
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		rows, err := db.New(resolveDB(rt, *dbPath)).QueryTSV("SELECT short_id,name,kind,color_hex,updated_at FROM collections WHERE deleted_at IS NULL ORDER BY updated_at DESC;")
		if err != nil {
			return err
		}
		for _, r := range rows {
			if len(r) >= 5 {
				_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n", r[0], r[1], r[2], r[3], r[4])
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown collection command %q", args[0])
	}
}

func runExport(rt runtime, args []string, out io.Writer) error {
	if len(args) < 1 || args[0] != "site" {
		return errors.New("usage: export site --out <dir>")
	}
	fs := flag.NewFlagSet("export site", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	outDir := fs.String("out", "", "output directory")
	dbPath := fs.String("db", "", "sqlite database path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	resolvedOut := strings.TrimSpace(*outDir)
	if resolvedOut == "" {
		resolvedOut = rt.profile.ExportDir
	}
	if err := exporter.ExportSite(db.New(resolveDB(rt, *dbPath)), resolvedOut); err != nil {
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
	dbPath := fs.String("db", "", "sqlite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	catalog, err := tui.LoadThemes("internal/tui/themes/presets.json")
	if err != nil {
		return err
	}
	if *listThemes {
		for _, item := range catalog.Themes {
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", item.ID, item.Name, item.Description)
		}
		return nil
	}

	selected := strings.TrimSpace(*theme)
	if selected == "" {
		selected = rt.profile.Theme
	}
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
	panel, err := tui.RenderDashboard(db.New(resolveDB(rt, *dbPath)), chosen, *filter, *limit)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(out, panel)
	return nil
}

func resolveDB(rt runtime, flagVal string) string {
	if strings.TrimSpace(flagVal) != "" {
		return flagVal
	}
	if v := strings.TrimSpace(os.Getenv("DOOH_DB")); v != "" {
		return v
	}
	return rt.profile.DB
}

func resolveActor(rt runtime, flagVal string) string {
	if strings.TrimSpace(flagVal) != "" {
		return flagVal
	}
	if v := strings.TrimSpace(os.Getenv("DOOH_ACTOR")); v != "" {
		return v
	}
	return rt.profile.Actor
}

func mustAuth(rt runtime, sqlite db.SQLite, actor string, keyFromFlag string, requireHumanTTY bool, neededScopes ...string) (principal, error) {
	var p principal
	if actor != "human" && actor != "agent" {
		return p, errors.New("--actor must be human or agent")
	}
	key := strings.TrimSpace(keyFromFlag)
	if key == "" {
		if actor == "human" {
			return p, errors.New("human actor requires --api-key (env fallback disabled to avoid accidental agent impersonation)")
		}
		envKey := rt.profile.APIKeyEnv
		if strings.TrimSpace(envKey) == "" {
			envKey = "DOOH_API_KEY"
		}
		key = strings.TrimSpace(os.Getenv(envKey))
	}
	if key == "" {
		return p, errors.New("missing api key")
	}
	if requireHumanTTY && actor == "human" {
		if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
			return p, errors.New("human actor requires interactive terminal")
		}
	}
	hash := auth.HashAPIKey(key)
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT k.id,k.user_id,k.scopes,k.client_type FROM api_keys k JOIN users u ON u.id=k.user_id WHERE k.key_hash=%s AND k.revoked_at IS NULL AND u.status='active' LIMIT 1;", db.Quote(hash)))
	if err != nil {
		return p, err
	}
	if len(rows) == 0 || len(rows[0]) < 4 {
		return p, errors.New("invalid api key")
	}
	p = principal{UserID: rows[0][1], KeyID: rows[0][0], ClientType: rows[0][3], Scopes: parseScopes(rows[0][2])}

	expectedClient := actor + "_cli"
	if p.ClientType != expectedClient && p.ClientType != "system" {
		return principal{}, fmt.Errorf("key client_type %s cannot be used as %s", p.ClientType, actor)
	}
	for _, need := range neededScopes {
		if !p.Scopes[need] {
			return principal{}, fmt.Errorf("missing required scope %q", need)
		}
	}
	return p, nil
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

func writeEvent(sqlite db.SQLite, p principal, eventType string, aggregateType string, aggregateID string, payload any) error {
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
