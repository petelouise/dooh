package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"dooh/internal/auth"
	"dooh/internal/db"
	"dooh/internal/demo"
	"dooh/internal/idgen"
)

func runDB(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(out, "db subcommands:")
		_, _ = fmt.Fprintln(out, "  init   initialize a new database")
		return nil
	}
	if args[0] != "init" {
		return fmt.Errorf("unknown db command %q (available: init)", args[0])
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
		_, _ = fmt.Fprintln(out, "setup subcommands:")
		_, _ = fmt.Fprintln(out, "  demo   bootstrap demo users, keys, and seed data")
		return nil
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
		return fmt.Errorf("unknown setup command %q (available: demo)", args[0])
	}
}

func runDemo(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(out, "demo subcommands:")
		_, _ = fmt.Fprintln(out, "  seed   insert demo data into database")
		return nil
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
		return fmt.Errorf("unknown demo command %q (available: seed)", args[0])
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

func runUser(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return printUserHelp(out)
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
		if rt.opts.JSON {
			return writeJSON(out, map[string]string{"id": id, "name": *name, "status": "active"})
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
		if rt.opts.JSON {
			users := make([]map[string]string, 0, len(rows))
			for _, r := range rows {
				if len(r) >= 4 {
					users = append(users, map[string]string{
						"id":         r[0],
						"name":       r[1],
						"status":     r[2],
						"created_at": r[3],
					})
				}
			}
			return writeJSON(out, users)
		}
		_, _ = fmt.Fprintln(out, style("NAME                 STATUS    CREATED                  USER_ID", "1"))
		_, _ = fmt.Fprintln(out, strings.Repeat("-", 92))
		for _, r := range rows {
			if len(r) >= 4 {
				_, _ = fmt.Fprintf(out, "%-20s %s %-24s %s\n", truncate(r[1], 20), statusCell(r[2], 9), truncate(r[3], 24), r[0])
			}
		}
		return nil
	case "lookup":
		return runUserLookup(rt, args[1:], out)
	case "help", "--help", "-h":
		return printUserHelp(out)
	default:
		return fmt.Errorf("unknown user command %q (available: create, list, lookup)", args[0])
	}
}

func printUserHelp(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "user subcommands:")
	_, _ = fmt.Fprintln(out, "  create   create a new user (admin only)")
	_, _ = fmt.Fprintln(out, "  list     list all users (admin only)")
	_, _ = fmt.Fprintln(out, "  lookup   list active user IDs and names (any authenticated user)")
	return nil
}

func runUserLookup(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("user lookup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sqlite := db.New(resolveDB(rt, *dbPath))
	if _, err := mustReadAuth(rt, sqlite, *apiKey, "tasks:read"); err != nil {
		return err
	}
	rows, err := sqlite.QueryTSV("SELECT id,name FROM users WHERE status='active' ORDER BY name;")
	if err != nil {
		return err
	}
	if rt.opts.JSON {
		users := make([]map[string]string, 0, len(rows))
		for _, r := range rows {
			if len(r) >= 2 {
				users = append(users, map[string]string{"id": r[0], "name": r[1]})
			}
		}
		return writeJSON(out, users)
	}
	_, _ = fmt.Fprintln(out, style("NAME                 USER_ID", "1"))
	_, _ = fmt.Fprintln(out, strings.Repeat("-", 50))
	for _, r := range rows {
		if len(r) >= 2 {
			_, _ = fmt.Fprintf(out, "%-20s %s\n", truncate(r[1], 20), r[0])
		}
	}
	return nil
}

func runKey(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(out, "key subcommands:")
		_, _ = fmt.Fprintln(out, "  create   create a new API key (admin only)")
		_, _ = fmt.Fprintln(out, "  revoke   revoke an API key (admin only)")
		return nil
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
		if rt.opts.JSON {
			return writeJSON(out, map[string]string{"key_prefix": prefix, "api_key": plain, "user_id": *user})
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
		return fmt.Errorf("unknown key command %q (available: create, revoke)", args[0])
	}
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

func userExists(sqlite db.SQLite, userID string) (bool, error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT 1 FROM users WHERE id=%s AND status='active' LIMIT 1;", db.Quote(userID)))
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}
