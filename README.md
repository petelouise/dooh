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

## Theme presets
```bash
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh tui --list-themes
GOCACHE=$(pwd)/.cache/go-build go run ./cmd/dooh tui --theme sunset-pop
```

## Auth safety behavior
- `--actor human` requires explicit `--api-key` (no env fallback) to reduce accidental AI impersonation.
- `--actor agent` can use `--api-key` or `DOOH_API_KEY`.
- Key `client_type` must match actor (`human_cli` or `agent_cli`) unless key type is `system`.
