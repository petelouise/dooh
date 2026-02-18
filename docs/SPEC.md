# dooh Product + Technical Spec

## Product intent
`dooh` (pronounced "duo") is a local-first task and goal management system for one human and one AI agent sharing a machine.

## MVP constraints
- Single local SQLite database.
- AI uses `.env` key.
- Human uses OS keychain key (not in `.env`).
- Read-only website integration via static JSON export.
- Colorful, keyboard-first TUI with selectable themes.
- Config profiles supported via `--profile` / `DOOH_PROFILE`.

## Identity and auth model
- Every mutating action requires API key auth.
- Actor identity is derived from key, never from role string alone.
- Required write flag: `--actor human|agent`.
- `--actor human` requires interactive TTY and keychain retrieval.
- `--actor agent` uses `DOOH_API_KEY` from environment.

## Config profile separation
- Files:
- `~/.config/dooh/config.toml` (global baseline)
- `./.dooh/config.toml` (project override)
- Sections: `[profile.default]`, `[profile.human]`, `[profile.agent]`, etc.
- Resolution order:
- flags > env > selected profile > default profile > built-in defaults

## Scope model
- Agent scopes: `tasks:read`, `tasks:write`, `tasks:delete`, `collections:read`, `collections:write`, `export:run`.
- Human scopes: all agent scopes + `users:admin`, `keys:admin`, `system:rollback`.

## Data rules
- ULID for canonical IDs.
- Short alphanumeric IDs for task and collection display aliases.
- Tasks belong to many collections.
- Collections belong to many parent collections.
- Collection membership is inherited upward.
- Tasks support assignees, subtasks, due/scheduled dates, rollover, skip-weekends, estimate, dependencies, and priority (`now|soon|later`).
- Completion blocked if dependency blockers are incomplete.
- Parent auto-completes when all subtasks are complete; parent reopens when a new incomplete subtask is added.
- Archive is reversible.
- Hard purge is disabled (never).

## Time behavior
- Single configured app timezone (`app_config.timezone`) used for scheduling and export.
- System timezone changes do not alter behavior unless app timezone is explicitly updated.

## Consistency and rollback
- SQLite WAL mode.
- Optimistic concurrency via `version` columns.
- Lease-based advisory locks for sequential edits.
- Append-only `events` table + snapshots.
- Full timeline rewind allowed for admins, with automatic restore-point snapshot.

## Website integration (MVP)
- Command: `dooh export site --out <dir>`.
- Outputs `index.json`, `tasks.json`, `collections.json`, `metrics.json`, `manifest.json`.
- Deterministic sorted JSON for stable diffs.
- No always-on server required.

## TUI theme presets
`internal/tui/themes/presets.json` includes:
- `sunset-pop`
- `mint-circuit`
- `paper-fruit`
- `midnight-arcade`

Theme selection:
- `dooh tui --list-themes`
- `dooh tui --theme <id>`
