# dooh

`dooh` (pronounced "duo") is a local-first task/project/goal manager for a human + ai pair.

## Current status
Working local MVP includes:
- sqlite-backed CLI commands (`db`, `user`, `key`, `task`, `collection`, `export`),
- append-only events + outbox writes for mutating task/collection actions,
- colorful TUI theme catalog with selection,
- staged renderer architecture (`legacy` active, `tea` migration path scaffolded),
- static JSON website export.

## Build
```bash
GOCACHE=$(pwd)/.cache/go-build go build ./cmd/dooh
```

`GOCACHE=...` prefixed this way is per-command (temporary). If you want it for your current shell session:
```bash
export GOCACHE="$(pwd)/.cache/go-build"
```

## First-time setup (real-world)
```bash
# install once (recommended for normal use)
go build -o ~/.local/bin/dooh ./cmd/dooh

# bootstrap local demo workspace (db + users + keys + sample tasks)
dooh setup demo --db ./dooh.db

# set persistent local defaults (no repeated flags needed)
dooh context set --profile human --db ./dooh.db --theme paper-fruit

# verify identity + open TUI
dooh whoami
dooh tui
```

What setup demo does:
- initializes schema in your database,
- seeds one human user and one ai user,
- creates profile-scoped stored keys under `~/.config/dooh/auth`,
- creates sample collections/tasks for immediate CLI/TUI testing.

## Manual first-time setup (without `setup demo`)
```bash
# 1) initialize database
dooh db init --db ./dooh.db

# 2) bootstrap first human user
dooh user create --db ./dooh.db --name Human --bootstrap

# 3) create first human admin key
HUMAN_ID=$(sqlite3 -noheader ./dooh.db "select id from users limit 1;")
dooh key create --db ./dooh.db --user "$HUMAN_ID" --client-type human_cli --scopes "tasks:read,tasks:write,tasks:delete,collections:read,collections:write,export:run,users:admin,keys:admin,system:rollback" --bootstrap

# 4) login once to store the key for your profile
dooh --profile human login --db ./dooh.db --api-key "<HUMAN_KEY>"

# 5) set defaults so commands are short
dooh context set --profile human --db ./dooh.db --theme paper-fruit

# 6) use normal commands
dooh task add --title "Ship MVP" --priority now
dooh collection add --name "Project Alpha" --kind project
dooh task list
dooh collection list
dooh export site --out ./site-data
```

## Real-world pair workflow

### Human shell
```bash
dooh context set --profile human --db ./dooh.db --theme paper-fruit
dooh whoami
dooh task list
dooh tui
```

### AI assistant shell (inherits human shell + loads ai `.env`)
Example `.env` for the ai runtime:
```bash
DOOH_AI_KEY=<AI_KEY>
```

Expected behavior with ai `.env` loaded:
- actor resolves to ai from key type,
- profile auto-forces to `ai` unless `--profile` is explicitly set,
- no repeated `DOOH_MODE` needed,
- all writes are attributed to ai user/key in events.

AI usage:
```bash
dooh whoami
dooh task add --title "Draft release checklist" --priority soon
dooh task assign add --id t_XXXXXX --user <human_user_id>
dooh task list
```

Relationship commands:
```bash
dooh task block --id <task> --by <task>
dooh task unblock --id <task> --by <task>
dooh task subtask add --parent <task> --child <task>
dooh task subtask remove --parent <task> --child <task>
dooh task assign add --id <task> --user <user_id>
dooh task assign remove --id <task> --user <user_id>
dooh task reopen --id <task>
dooh collection link --parent <collection> --child <collection>
dooh collection unlink --parent <collection> --child <collection>
```

Useful identity/context commands:
```bash
dooh context show
dooh context set --profile ai
dooh context clear
dooh whoami
```

## Testing checklist

### CLI smoke test
```bash
dooh setup demo --db ./dooh.db
dooh context set --profile human --db ./dooh.db --theme paper-fruit
dooh whoami
dooh task add --title "CLI smoke task" --priority now
dooh task list
dooh export site --out ./site-data
```

### AI smoke test
```bash
export DOOH_AI_KEY="<AI_KEY>"
dooh whoami
dooh task add --title "AI smoke task" --priority soon
dooh task list
```

### TUI smoke test
```bash
dooh tui
dooh tui --static --plain
dooh tui --renderer legacy
dooh tui --renderer tea
```

### Audit verification
```bash
sqlite3 ./dooh.db "select seq,event_type,actor_user_id,key_id,client_type,occurred_at from events order by seq desc limit 20;"
```
Check that human and ai commands produce expected attribution rows.

### Automated tests
```bash
GOCACHE=$(pwd)/.cache/go-build go test ./...
GOCACHE=$(pwd)/.cache/go-build go test ./internal/cli -run Test
GOCACHE=$(pwd)/.cache/go-build go test ./internal/tui -run Test
GOCACHE=$(pwd)/.cache/go-build go test ./... -cover
```

## Fast demo seed + colorful dashboard
```bash
dooh db init --db ./dooh.db
dooh demo seed --db ./dooh.db
export DOOH_MODE=human
dooh tui --db ./dooh.db --api-key "<PASTE_KEY>" --theme midnight-arcade --limit 12
dooh tui --db ./dooh.db --api-key "<PASTE_KEY>" --theme midnight-arcade --limit 12 --static
dooh tui --db ./dooh.db --api-key "<PASTE_KEY>" --theme midnight-arcade --limit 12 --plain
```

TUI controls:
- `up/down`: move selection (no Enter required)
- `Enter`: expand task detail, or drill into selected project/goal/assignee
- `right`: expand selected task inline
- `left`: collapse inline detail
- `/`: edit fuzzy filter live (`Enter`/`Esc` close input)
- `Tab` / `Shift+Tab`: move focus between filter fields
- `f`: edit text filter
- `g`: edit tags filter (typeahead + counts, multi-tag AND)
- `a`: edit assignee filter (typeahead + counts)
- `s`: cycle status filter
- `p`: cycle priority filter
- `m`: toggle Today mode (`mine` / `all`) in Today view
- `t`: randomize theme
- `1`: task list view
- `2`: project progress view
- `3`: goal progress view
- `4`: today view (tasks scheduled today)
- `5`: assignee progress view
- `c`: clear all filters back to defaults (`status=open`, `priority=all`, empty text)
- `q`: quit

View headers are context-aware:
- big section headers for `ALL TASKS`, `PROJECTS`, `GOALS`, `TODAY`, `ASSIGNEES`
- when scoped from a project/goal row via `Enter`, header shows that specific project/goal name

TUI is currently read-focused:
- no command bar workflow is required
- filters are always visible at the top: text/status/priority/tag/assignee/scope
- `today` view defaults to tasks assigned to the authenticated user
- toggle `mine/all` in Today view with `m`

Task rows use status icons instead of a status text column:
- `○` open
- `✓` completed
- `✕` archived
- `⚑` due date exists
- `!` overdue open task (highlighted)

Task table columns:
- `selection`, `status icon`, `assignee initials`, `title (with due/overdue marker)`, `priority`, `scheduled`
- `updated` is shown in the bottom status/detail bar

Expanded task details are split by collection type:
- `projects`, `goals`, `areas`, `groups`, `tags`, `assignees`

Timestamps in TUI use natural format:
- `today`
- `tomorrow`
- `yesterday`
- weekday names inside a 7-day window (e.g. `monday`)
- `03 Feb 2026` for dates outside that window

Fallback behavior:
- interactive mode uses conservative cbreak (`stty -icanon -echo min 1 time 0`)
- if terminal capability checks fail (non-TTY, tiny width, mode switch failure), TUI falls back to plain static rendering
- `--plain` forces plain rendering

## Theme presets
```bash
dooh tui --list-themes
dooh tui --theme sunset-pop
dooh tui --theme sunset-pop --filter rollback
```

## TUI renderer architecture
The TUI now supports renderer selection:
- `--renderer auto` (default): prefer `tea` path, fallback to legacy as needed.
- `--renderer legacy`: force current ANSI renderer.
- `--renderer tea`: explicit migration path for task-view-focused renderer.

Examples:
```bash
dooh tui --renderer auto
dooh tui --renderer legacy
dooh tui --renderer tea
```

Notes:
- In this build environment, Bubble Tea dependencies are not fetched from the network.
- The `tea` path is scaffolded and currently delegates to the stable legacy interaction loop.
- Once Bubble Tea deps are available, task view migration can be switched on without CLI breakage.

## Config profiles
Profiles are named blocks in config files:
- global: `~/.config/dooh/config.toml`
- project override: `./.dooh/config.toml`

Precedence:
- command flags
- env vars (`DOOH_DB`, `DOOH_PROFILE`, profile-selected key env var)
- persisted context overrides (`dooh context set`)
- selected profile (`--profile` or env/context resolved)
- `[profile.default]`
- built-in defaults

Generate a starter file:
```bash
dooh config init
```

Inspect resolved config:
```bash
dooh config show
dooh --profile human config show
```

## Auth safety behavior
- all runtime data commands require authenticated user context (no anonymous mode).
- explicit `--api-key` takes priority and determines actor from key `client_type`.
- if no explicit key is provided, auth resolves from `DOOH_MODE` + stored/env keys.
- `DOOH_MODE` supports `human`, `ai`, and legacy `agent`, but is optional.
- ai env key (`DOOH_AI_KEY`, or `DOOH_API_KEY` for compatibility) enables zero-touch ai operation.
- when ai env key is present, profile is auto-forced to `ai` unless `--profile` is explicitly provided.
- Key `client_type` must be interactive (`human_cli` or `agent_cli`) for runtime commands.
- profile-scoped keys are written to `~/.config/dooh/auth/<profile>.<actor>.key` with `0600` permissions.
- user/key lifecycle admin actions are human-only by default; non-human requires system key + `--allow-system-admin`.
- there is intentionally no user-delete command.

## Developer-only run mode
Using `go run` is still supported for development:
```bash
go run ./cmd/dooh version
go run ./cmd/dooh context show
```

## TUI troubleshooting
- If visuals look wrong in your terminal, try:
  - `dooh tui --renderer legacy`
  - `dooh tui --plain`
- If non-interactive (pipe/CI), TUI auto-falls back to plain static output.
- If theme contrast looks poor, switch theme quickly with:
  - `dooh tui --theme paper-fruit`
  - `dooh tui --theme mint-circuit`
