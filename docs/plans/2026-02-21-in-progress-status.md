# `in-progress` Task Status Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `in_progress` as a task status with a `task start` CLI command and `started_at` timestamp, making the AI/human pair's division of labor machine-visible.

**Architecture:** Three independent pieces land in order: schema (migration SQL + `dooh db migrate` command), CLI (`task start`, updated `task list`/`task show`), and export (`started_at` in site export). TUI icon deferred.

**Tech Stack:** Go, SQLite, `dooh/internal/cli`, `dooh/internal/exporter`, `dooh/internal/db`

---

### Task 1: Update `0001_init.sql` for new database creation

This is the schema for fresh databases. Existing databases get the migration in Task 2.

**Files:**
- Modify: `migrations/0001_init.sql:53`

**Step 1: Edit the tasks table definition**

Find this line in `migrations/0001_init.sql`:
```sql
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','completed','archived')),
```

Change it to:
```sql
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','in_progress','completed','archived')),
```

Also add `started_at TEXT,` after the `estimated_minutes INTEGER,` line (currently line 59):
```sql
  estimated_minutes INTEGER,
  started_at TEXT,
  completed_at TEXT,
```

**Step 2: Verify the schema looks correct**

Run: `grep -A2 -B2 'in_progress\|started_at' migrations/0001_init.sql`
Expected: both new values appear in context.

**Step 3: Commit**

```bash
git add migrations/0001_init.sql
git commit -m "feat: add in_progress status and started_at to tasks schema"
```

---

### Task 2: Add `0002_in_progress_status.sql` migration and `dooh db migrate` command

SQLite cannot `ALTER TABLE ... MODIFY COLUMN` to update a CHECK constraint. The migration
uses the standard SQLite table-rebuild pattern. This is for existing databases only.

**Files:**
- Create: `migrations/0002_in_progress_status.sql`
- Modify: `internal/cli/admin.go`

**Step 1: Write the failing test**

Add to `internal/cli/streamlined_ux_test.go`:

```go
func TestDBMigrateAppliesInProgressStatus(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	// Create a DB using the OLD schema (without in_progress / started_at)
	path := filepath.Join(t.TempDir(), "migrate_test.db")
	sqlite := db.New(path)
	mustExec(t, sqlite, "PRAGMA journal_mode=WAL;")
	mustExec(t, sqlite, `
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','disabled'))
);
CREATE TABLE tasks (
  id TEXT PRIMARY KEY,
  short_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','completed','archived')),
  priority TEXT NOT NULL DEFAULT 'later' CHECK(priority IN ('now','soon','later')),
  due_at TEXT,
  scheduled_at TEXT,
  rollover_enabled INTEGER NOT NULL DEFAULT 0,
  skip_weekends INTEGER NOT NULL DEFAULT 0,
  estimated_minutes INTEGER,
  completed_at TEXT,
  archived_at TEXT,
  deleted_at TEXT,
  created_by TEXT NOT NULL,
  updated_by TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  version INTEGER NOT NULL DEFAULT 1
);
INSERT INTO users(id,name) VALUES('u','Test User');
INSERT INTO tasks(id,short_id,title,status,priority,created_by,updated_by)
  VALUES('t1','t_AAA','Alpha','open','now','u','u');
`)

	var out bytes.Buffer
	if err := Run([]string{"db", "migrate", "--db", path}, &out); err != nil {
		t.Fatalf("db migrate failed: %v", err)
	}

	// After migration, task can be set to in_progress
	if err := sqlite.Exec("UPDATE tasks SET status='in_progress' WHERE id='t1';"); err != nil {
		t.Fatalf("expected in_progress to be valid after migration: %v", err)
	}
	// started_at column exists
	rows, err := sqlite.QueryTSV("SELECT started_at FROM tasks WHERE id='t1';")
	if err != nil {
		t.Fatalf("started_at column should exist after migration: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected row from tasks")
	}
}
```

**Step 2: Run to verify it fails**

Run: `go test ./internal/cli/... -run TestDBMigrateAppliesInProgressStatus -v`
Expected: FAIL — `db migrate` unknown command

**Step 3: Create `migrations/0002_in_progress_status.sql`**

```sql
-- Migration 002: add in_progress to task status, add started_at column
-- Uses SQLite table-rebuild pattern since CHECK constraints cannot be altered.

PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS tasks_new (
  id TEXT PRIMARY KEY,
  short_id TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','in_progress','completed','archived')),
  priority TEXT NOT NULL DEFAULT 'later' CHECK(priority IN ('now','soon','later')),
  due_at TEXT,
  scheduled_at TEXT,
  rollover_enabled INTEGER NOT NULL DEFAULT 0 CHECK(rollover_enabled IN (0,1)),
  skip_weekends INTEGER NOT NULL DEFAULT 0 CHECK(skip_weekends IN (0,1)),
  estimated_minutes INTEGER,
  started_at TEXT,
  completed_at TEXT,
  archived_at TEXT,
  deleted_at TEXT,
  created_by TEXT NOT NULL REFERENCES users(id),
  updated_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  version INTEGER NOT NULL DEFAULT 1
);

INSERT INTO tasks_new
  SELECT id, short_id, title, description, status, priority,
         due_at, scheduled_at, rollover_enabled, skip_weekends,
         estimated_minutes, NULL, completed_at, archived_at,
         deleted_at, created_by, updated_by, created_at, updated_at, version
  FROM tasks;

DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;

PRAGMA foreign_keys = ON;
PRAGMA user_version = 2;
```

**Step 4: Add `runDBMigrate` to `internal/cli/admin.go`**

After `readInitMigration` (around line 462), add:

```go
func readMigration(name string) ([]byte, error) {
	candidates := []string{
		filepath.Join("migrations", name),
		filepath.Join("..", "migrations", name),
		filepath.Join("..", "..", "migrations", name),
	}
	if _, file, _, ok := goruntime.Caller(0); ok {
		base := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
		candidates = append([]string{filepath.Join(base, "migrations", name)}, candidates...)
	}
	var lastErr error
	for _, path := range candidates {
		b, err := os.ReadFile(path)
		if err == nil {
			return b, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("read migration %s: %w", name, lastErr)
}

func runDBMigrate(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("db migrate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", "", "sqlite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dbResolved := resolveDB(rt, *dbPath)
	sqlite := db.New(dbResolved)

	rows, err := sqlite.QueryTSV("PRAGMA user_version;")
	if err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	current := 0
	if len(rows) > 0 && len(rows[0]) > 0 {
		current = parseIntDefault(rows[0][0], 0)
	}

	type pending struct {
		version int
		name    string
	}
	migrations := []pending{
		{2, "0002_in_progress_status.sql"},
	}

	applied := 0
	for _, m := range migrations {
		if current >= m.version {
			continue
		}
		sql, err := readMigration(m.name)
		if err != nil {
			return err
		}
		if err := sqlite.Exec(string(sql)); err != nil {
			return fmt.Errorf("apply %s: %w", m.name, err)
		}
		applied++
		_, _ = fmt.Fprintf(out, "applied migration %s\n", m.name)
	}
	if applied == 0 {
		_, _ = fmt.Fprintln(out, "database is up to date")
	}
	return nil
}
```

**Step 5: Wire `db migrate` into `runDB` and `printDBHelp`**

In `printDBHelp` (line 19), add:
```go
	_, _ = fmt.Fprintln(out, "  migrate  apply pending schema migrations to an existing database")
```

In `runDB` (line 37), change the unknown-command error and add the new case before the `init` check:
```go
func runDB(rt runtime, args []string, out io.Writer) error {
	if len(args) == 0 {
		return printDBHelp(out)
	}
	switch args[0] {
	case "help", "--help", "-h":
		return printDBHelp(out)
	case "init":
		// existing init logic (lines 40-56) stays as is
		...
	case "migrate":
		return runDBMigrate(rt, args[1:], out)
	default:
		return fmt.Errorf("unknown db command %q (available: init, migrate)", args[0])
	}
}
```

**Step 6: Run test to verify it passes**

Run: `go test ./internal/cli/... -run TestDBMigrateAppliesInProgressStatus -v`
Expected: PASS

**Step 7: Run all tests**

Run: `go test ./...`
Expected: all pass

**Step 8: Commit**

```bash
git add migrations/0002_in_progress_status.sql internal/cli/admin.go internal/cli/streamlined_ux_test.go
git commit -m "feat: add dooh db migrate command and 0002 in_progress migration"
```

---

### Task 3: Add `task start` CLI command

**Files:**
- Modify: `internal/cli/task.go`
- Modify: `internal/cli/output.go`

**Step 1: Write the failing test**

Add to `internal/cli/audit_integration_test.go`:

```go
func TestTaskStartTransitionsToInProgress(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	path := filepath.Join(t.TempDir(), "start_test.db")
	sqlite := db.New(path)
	if err := initDatabase(sqlite); err != nil {
		t.Fatal(err)
	}

	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u_h','Human','active');")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k_h','u_h','hhhhhhhh','"+auth.HashAPIKey("dooh_test_key")+"','tasks:write,tasks:read','human_cli',NULL);")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,created_by,updated_by) VALUES('t1','t_AAA','Test task','open','now','u_h','u_h');")

	var out bytes.Buffer
	if err := Run([]string{"task", "start", "--db", path, "--api-key", "dooh_test_key", "--id", "t_AAA"}, &out); err != nil {
		t.Fatalf("task start failed: %v", err)
	}

	rows, err := sqlite.QueryTSV("SELECT status, started_at FROM tasks WHERE id='t1';")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || rows[0][0] != "in_progress" {
		t.Fatalf("expected in_progress status, got %v", rows)
	}
	if rows[0][1] == "" {
		t.Fatal("expected started_at to be set")
	}
}

func TestTaskStartRejectsCompletedTask(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	path := filepath.Join(t.TempDir(), "start_reject.db")
	sqlite := db.New(path)
	if err := initDatabase(sqlite); err != nil {
		t.Fatal(err)
	}

	mustExec(t, sqlite, "INSERT INTO users(id,name,status) VALUES('u_h','Human','active');")
	mustExec(t, sqlite, "INSERT INTO api_keys(id,user_id,key_prefix,key_hash,scopes,client_type,revoked_at) VALUES('k_h','u_h','hhhhhhhh','"+auth.HashAPIKey("dooh_test_key2")+"','tasks:write,tasks:read','human_cli',NULL);")
	mustExec(t, sqlite, "INSERT INTO tasks(id,short_id,title,status,priority,created_by,updated_by) VALUES('t2','t_BBB','Done task','completed','now','u_h','u_h');")

	var out bytes.Buffer
	err := Run([]string{"task", "start", "--db", path, "--api-key", "dooh_test_key2", "--id", "t_BBB"}, &out)
	if err == nil {
		t.Fatal("expected error starting a completed task")
	}
	if !strings.Contains(err.Error(), "cannot start") {
		t.Fatalf("expected 'cannot start' error, got: %v", err)
	}
}
```

**Step 2: Run to verify it fails**

Run: `go test ./internal/cli/... -run 'TestTaskStart' -v`
Expected: FAIL — `task start` unknown command

**Step 3: Add `task start` to `runTask` switch in `task.go`**

In the `switch args[0]` block (around line 27), add before `case "block"`:
```go
	case "start":
		return runTaskStart(rt, args[1:], out)
```

**Step 4: Add `runTaskStart` function in `task.go`**

Add after `runTaskStatus` (after line 837):

```go
func runTaskStart(rt runtime, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("task start", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("id", "", "task id or short id")
	dbPath := fs.String("db", "", "sqlite database path")
	apiKey := fs.String("api-key", "", "api key")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return printTaskStartHelp(out)
		}
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
	taskID, shortID, currentStatus, err := resolveTask(sqlite, *target)
	if err != nil {
		return err
	}
	if currentStatus != "open" {
		return fmt.Errorf("cannot start task with status %q (only open tasks can be started)", currentStatus)
	}
	sql := fmt.Sprintf(
		"UPDATE tasks SET status='in_progress', started_at=strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ','now'), updated_by=%s, version=version+1 WHERE id=%s AND deleted_at IS NULL;",
		db.Quote(p.UserID), db.Quote(taskID))
	if err := sqlite.Exec(sql); err != nil {
		return err
	}
	if err := writeEvent(sqlite, p, "task.started", "task", taskID, map[string]string{"task_id": taskID, "short_id": shortID, "status": "in_progress"}); err != nil {
		return err
	}
	if rt.opts.JSON {
		return writeJSON(out, map[string]string{"short_id": shortID, "id": taskID, "status": "in_progress"})
	}
	_, _ = fmt.Fprintf(out, "started task %s\n", shortID)
	return nil
}
```

**Step 5: Add `printTaskStartHelp` in `task.go`**

Add after `printTaskStatusHelp` (after line 172):

```go
func printTaskStartHelp(out io.Writer) error {
	_, _ = fmt.Fprintln(out, "usage: dooh task start --id <id>")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "mark a task as in progress (status: open → in_progress)")
	_, _ = fmt.Fprintln(out, "sets started_at timestamp and emits a task.started event")
	_, _ = fmt.Fprintln(out, "note: only open tasks can be started")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "required:")
	_, _ = fmt.Fprintln(out, "  --id <id>   task short_id or full ID")
	_, _ = fmt.Fprintln(out, "")
	_, _ = fmt.Fprintln(out, "example:")
	_, _ = fmt.Fprintln(out, "  dooh --json task start --id t_abc123")
	return nil
}
```

**Step 6: Check what `resolveTask` returns — does it return status?**

Run: `grep -n 'func resolveTask' internal/cli/task.go`

If `resolveTask` does not return status, update it. Currently it likely returns `(id, shortID, err)`.
Check the signature and if needed change to return `(id, shortID, status, err)` by adding
`t.status` to the SELECT query inside `resolveTask`.

Current `resolveTask` (find it with grep above). If it returns only 2 values, change it to:

```go
func resolveTask(sqlite db.SQLite, target string) (id, shortID, status string, err error) {
	rows, err := sqlite.QueryTSV(fmt.Sprintf(
		"SELECT id, short_id, status FROM tasks WHERE (id=%s OR short_id=%s) AND deleted_at IS NULL LIMIT 1;",
		db.Quote(target), db.Quote(target)))
	if err != nil {
		return "", "", "", err
	}
	if len(rows) == 0 || len(rows[0]) < 3 {
		return "", "", "", fmt.Errorf("unknown task %s", target)
	}
	return rows[0][0], rows[0][1], rows[0][2], nil
}
```

Then update all callers of `resolveTask` throughout `task.go` to accept the new return
signature (add `_,` or use the status where relevant — other callers can discard it with `_`).

Run: `grep -n 'resolveTask' internal/cli/task.go`
Update each call site.

**Step 7: Add `in_progress` to `statusCell` in `output.go`**

In `statusCell` (line 57 of `internal/cli/output.go`), add a case:
```go
	case "in_progress":
		return style(raw, "38;5;214")  // orange — visually distinct from open/completed
```

**Step 8: Run tests**

Run: `go test ./internal/cli/... -run 'TestTaskStart' -v`
Expected: PASS

Run: `go test ./...`
Expected: all pass

**Step 9: Commit**

```bash
git add internal/cli/task.go internal/cli/output.go internal/cli/audit_integration_test.go
git commit -m "feat: add task start command with in_progress status and started_at timestamp"
```

---

### Task 4: Update `task list` and `task show` to handle `in_progress`

**Files:**
- Modify: `internal/cli/task.go`

**Step 1: Update `task list` help text and status filter**

In `printTaskListHelp` (line 100), change the status description:
```go
	_, _ = fmt.Fprintln(out, "  --status <string>      open|in_progress|completed|archived|all (default: open)")
```

No code change needed in `runTaskList` — the filter already passes `*status` as a quoted string to the WHERE clause, so `--status in_progress` will just work.

**Step 2: Update `task show` query to include `started_at`**

In `runTaskShow`, the SELECT query at line 477 needs `started_at` added. Find this query:
```go
"SELECT id,short_id,title,description,status,priority,due_at,scheduled_at,estimated_minutes,rollover_enabled,skip_weekends,completed_at,archived_at,created_by,updated_by,created_at,updated_at,version FROM tasks WHERE ..."
```

Add `started_at` between `estimated_minutes` and `completed_at`:
```go
"SELECT id,short_id,title,description,status,priority,due_at,scheduled_at,estimated_minutes,rollover_enabled,skip_weekends,started_at,completed_at,archived_at,created_by,updated_by,created_at,updated_at,version FROM tasks WHERE ..."
```

Then the column indices shift by 1 for everything after index 10. Update all `r[N]` references
after index 10 in `runTaskShow` accordingly (bump each by 1), and add `started_at` output:
- `r[11]` → `started_at`
- All previous `r[11]` (was `completed_at`) → `r[12]`
- All previous `r[12]` (was `archived_at`) → `r[13]`
- etc.

In the JSON output block, add:
```go
		if r[11] != "" {
			task["started_at"] = r[11]
		}
```

In the human-readable output, after the `status:` line, add:
```go
	if r[11] != "" {
		_, _ = fmt.Fprintf(out, "started:     %s\n", r[11])
	}
```

**Step 3: Update `task reopen` to also clear `started_at`**

In `runTaskStatus` (line 818), the `open` case clears `completed_at` and `archived_at`.
Also clear `started_at`:
```go
	if status == "open" {
		extra = ", completed_at = NULL, archived_at = NULL, started_at = NULL"
	}
```

**Step 4: Run all tests**

Run: `go test ./...`
Expected: all pass (note: `r[N]` index fixes may surface panics if any were missed — fix them)

**Step 5: Commit**

```bash
git add internal/cli/task.go
git commit -m "feat: update task list/show/reopen to handle in_progress status and started_at"
```

---

### Task 5: Update exporter to include `started_at`

**Files:**
- Modify: `internal/exporter/site.go`

**Step 1: Read the current export query**

Run: `grep -n 'SELECT.*tasks' internal/exporter/site.go`

Currently:
```go
tasks, err := sqlite.QueryTSV("SELECT short_id,title,status,priority,COALESCE(due_at,''),COALESCE(scheduled_at,''),updated_at FROM tasks WHERE deleted_at IS NULL ORDER BY updated_at DESC;")
```

**Step 2: Add `started_at` to the export query and output map**

Change the query to include `COALESCE(started_at,'')`:
```go
tasks, err := sqlite.QueryTSV("SELECT short_id,title,status,priority,COALESCE(due_at,''),COALESCE(scheduled_at,''),updated_at,COALESCE(started_at,'') FROM tasks WHERE deleted_at IS NULL ORDER BY updated_at DESC;")
```

In the loop that builds the task map (around line 39), add:
```go
			if r[7] != "" {
				task["started_at"] = r[7]
			}
```

(Adjust index if the existing code uses a different position.)

**Step 3: Run all tests**

Run: `go test ./...`
Expected: all pass

**Step 4: Commit**

```bash
git add internal/exporter/site.go
git commit -m "feat: include started_at in site export output"
```

---

### Task 6: Update `printTaskHelp` to list `start` command

**Files:**
- Modify: `internal/cli/task.go`

**Step 1: Find `printTaskHelp` and add `start` to the command list**

Run: `grep -n 'printTaskHelp\|complete\|archive' internal/cli/task.go | head -20`

In the help output, add `start` alongside `complete`, `reopen`, `archive`:
```go
	_, _ = fmt.Fprintln(out, "  start      mark a task as in progress")
```

**Step 2: Run all tests**

Run: `go test ./...`
Expected: all pass

**Step 3: Update PRIORITIES.md to mark `in-progress` status as done**

In `docs/PRIORITIES.md`, in the Priority Index table, mark the P0 `in-progress` row as implemented.
Add a note at the top of the P0 section noting the CLI work is complete and the TUI work is deferred.

**Step 4: Commit**

```bash
git add internal/cli/task.go docs/PRIORITIES.md
git commit -m "docs: mark in-progress CLI status as implemented in PRIORITIES.md"
```

---

### Task 7: Final verification

**Step 1: Build the binary**

Run: `go build ./cmd/dooh`
Expected: no errors

**Step 2: Run all tests**

Run: `go test ./...`
Expected: all pass

**Step 3: Smoke test with real DB (if available)**

```bash
./dooh task list --db dooh.db               # verify existing tasks still show
./dooh task start --id <some_open_id> --db dooh.db  # if a real DB exists
./dooh task list --status in_progress --db dooh.db
./dooh db migrate --db dooh.db              # should say "database is up to date" if already migrated
```
