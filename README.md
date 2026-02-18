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

## Quick start
```bash
# 1) initialize database
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh db init --db ./dooh.db

# 2) bootstrap first human user
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh user create --db ./dooh.db --name Human --bootstrap

# 3) create first human admin key
HUMAN_ID=$(sqlite3 -noheader ./dooh.db "select id from users limit 1;")
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh key create --db ./dooh.db --user "$HUMAN_ID" --client-type human_cli --scopes "tasks:read,tasks:write,tasks:delete,collections:read,collections:write,export:run,users:admin,keys:admin,system:rollback" --bootstrap

# 4) use printed api_key for writes
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh task add --db ./dooh.db --actor human --api-key "<PASTE_KEY>" --title "Ship MVP" --priority now
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh collection add --db ./dooh.db --actor human --api-key "<PASTE_KEY>" --name "Project Alpha" --kind project

# 5) list + export
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh task list --db ./dooh.db
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh collection list --db ./dooh.db
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh export site --db ./dooh.db --out ./site-data
```

## Fast demo seed + colorful dashboard
```bash
export GOCACHE="$(pwd)/.cache/go-build"
go run ./cmd/dooh db init --db ./dooh.db
go run ./cmd/dooh demo seed --db ./dooh.db
go run ./cmd/dooh tui --db ./dooh.db --theme midnight-arcade --limit 12
```

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
- env vars (`DOOH_DB`, `DOOH_ACTOR`, profile-selected key env var)
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
- `--actor human` requires explicit `--api-key` (no env fallback) to reduce accidental AI impersonation.
- `--actor agent` can use `--api-key` or `DOOH_API_KEY`.
- Key `client_type` must match actor (`human_cli` or `agent_cli`) unless key type is `system`.
