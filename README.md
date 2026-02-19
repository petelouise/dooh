# dooh

`dooh` (pronounced "duo") is a local-first task/project/goal manager for a human + AI agent pair.

## Current status
Working local MVP includes:
- sqlite-backed CLI commands (`db`, `user`, `key`, `task`, `collection`, `export`),
- append-only events + outbox writes for mutating task/collection actions,
- colorful TUI theme catalog with selection,
- static JSON website export.

## Build
```bash
GOCACHE=$(pwd)/.cache/go-build go build ./cmd/dooh
```

`GOCACHE=...` prefixed this way is per-command (temporary). If you want it for your current shell session:
```bash
export GOCACHE="$(pwd)/.cache/go-build"
```

## Streamlined quick start
```bash
# one command bootstrap: db + seed + human/ai demo keys in profile auth store
go run ./cmd/dooh setup demo --db ./dooh.db

# run directly (stored login keys and explicit --api-key both work)
DOOH_MODE=human go run ./cmd/dooh --profile human whoami
go run ./cmd/dooh --profile human tui --theme midnight-arcade
```

## Manual quick start
```bash
# 1) initialize database
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh db init --db ./dooh.db

# 2) bootstrap first human user
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh user create --db ./dooh.db --name Human --bootstrap

# 3) create first human admin key
HUMAN_ID=$(sqlite3 -noheader ./dooh.db "select id from users limit 1;")
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh key create --db ./dooh.db --user "$HUMAN_ID" --client-type human_cli --scopes "tasks:read,tasks:write,tasks:delete,collections:read,collections:write,export:run,users:admin,keys:admin,system:rollback" --bootstrap

# 4) use printed api_key for writes (or store once via login command)
export DOOH_MODE=human
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh task add --db ./dooh.db --api-key "<PASTE_KEY>" --title "Ship MVP" --priority now
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh collection add --db ./dooh.db --api-key "<PASTE_KEY>" --name "Project Alpha" --kind project

# 5) list + export
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh task list --db ./dooh.db --api-key "<PASTE_KEY>"
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh collection list --db ./dooh.db --api-key "<PASTE_KEY>"
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh export site --db ./dooh.db --out ./site-data --api-key "<PASTE_KEY>"
```

Relationship commands:
```bash
go run ./cmd/dooh task block --id <task> --by <task>
go run ./cmd/dooh task unblock --id <task> --by <task>
go run ./cmd/dooh task subtask add --parent <task> --child <task>
go run ./cmd/dooh task subtask remove --parent <task> --child <task>
go run ./cmd/dooh task assign add --id <task> --user <user_id>
go run ./cmd/dooh task assign remove --id <task> --user <user_id>
go run ./cmd/dooh task reopen --id <task>
go run ./cmd/dooh collection link --parent <collection> --child <collection>
go run ./cmd/dooh collection unlink --parent <collection> --child <collection>
```

For all data commands (read and write):
- explicit `--api-key` is enough; actor is inferred from key type.
- without `--api-key`, mode/key source is resolved from `DOOH_MODE` + stored/env keys.
- `DOOH_MODE` accepts `human`, `ai`, or legacy `agent`.

Store keys once per profile:
```bash
go run ./cmd/dooh --profile human login --db ./dooh.db --api-key "<HUMAN_KEY>"
go run ./cmd/dooh --profile agent login --db ./dooh.db --api-key "<AGENT_KEY>"
```

Emit shell exports for profile context:
```bash
eval "$(go run ./cmd/dooh --profile human env --mode human)"
eval "$(go run ./cmd/dooh --profile agent env --mode ai)"
```

Inspect current execution identity:
```bash
DOOH_MODE=human go run ./cmd/dooh whoami --api-key "<HUMAN_KEY>"
DOOH_MODE=ai DOOH_API_KEY="<AGENT_KEY>" go run ./cmd/dooh whoami
```

## Fast demo seed + colorful dashboard
```bash
export GOCACHE="$(pwd)/.cache/go-build"
go run ./cmd/dooh db init --db ./dooh.db
go run ./cmd/dooh demo seed --db ./dooh.db
export DOOH_MODE=human
go run ./cmd/dooh tui --db ./dooh.db --api-key "<PASTE_KEY>" --theme midnight-arcade --limit 12
go run ./cmd/dooh tui --db ./dooh.db --api-key "<PASTE_KEY>" --theme midnight-arcade --limit 12 --static
go run ./cmd/dooh tui --db ./dooh.db --api-key "<PASTE_KEY>" --theme midnight-arcade --limit 12 --plain
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
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh tui --list-themes
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh tui --theme sunset-pop
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh tui --theme sunset-pop --filter rollback
```

## Config profiles
Profiles are named blocks in config files:
- global: `~/.config/dooh/config.toml`
- project override: `./.dooh/config.toml`

Precedence:
- command flags
- env vars (`DOOH_DB`, profile-selected key env var)
- selected profile (`--profile` or `DOOH_PROFILE`)
- `[profile.default]`
- built-in defaults

Generate a starter file:
```bash
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh config init
```

Inspect resolved config:
```bash
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh config show
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh --profile human config show
```

## Auth safety behavior
- all runtime data commands require authenticated user context (no anonymous mode).
- explicit `--api-key` takes priority and determines actor from key `client_type`.
- if no explicit key is provided, auth resolves from `DOOH_MODE` + stored/env keys.
- `DOOH_MODE` supports `human`, `ai`, and legacy `agent`.
- Key `client_type` must be interactive (`human_cli` or `agent_cli`) for runtime commands.
- profile-scoped keys are written to `~/.config/dooh/auth/<profile>.<actor>.key` with `0600` permissions.
