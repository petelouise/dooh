# AI CLI Interface Audit

Audit date: 2026-02-21
Auditor: AI agent (Claude)
Scope: CLI interface usability from an AI coding agent's perspective
Codebase version: v0.3.0 (as reported by `dooh version`)

## Executive Summary

dooh has a solid foundation for human+AI pair use. The auth model correctly attributes actions by actor type, the event system records `client_type` per mutation, and the `DOOH_AI_KEY` env-based auth is low-friction for agents. However, the CLI currently outputs **human-formatted text only** with no machine-readable mode, several important commands are missing, and there is no way to query the audit trail from the CLI. These gaps force an AI agent to do fragile text-parsing and fall back to raw SQLite queries for basic operations.

This audit identifies issues in priority order and proposes concrete fixes.

---

## 1. No Machine-Readable Output Mode

**Severity: Critical**

Every command outputs fixed-width columnar text with ANSI color codes. There is no `--json`, `--format`, or `--output` flag anywhere. An AI agent must:

- Parse positional columns with variable-width truncation
- Strip ANSI escape sequences (or set `NO_COLOR=1`)
- Extract IDs from the rightmost column of table output
- Parse `key=value` lines from `whoami` / `context show`

**Current output examples:**

```
# task add
context profile=ai mode=ai user=01ABCDEF key=aaaaaaaa db=./dooh.db
created task t_abc123 (Water mint patch)

# task list
TITLE                                     STATUS     PRIORITY UPDATED                  TASK_ID
----------------------------------------------------------------------------------------------------
Water mint patch                          open       now      2026-02-20T...           t_abc123

# whoami
context profile=ai mode=ai user=01ABCDEF key=aaaaaaaa db=./dooh.db
client_type=agent_cli
```

**Problems for AI agents:**
- Title column is truncated at 40 chars -- IDs or data can be lost
- Columns are not delimited; they rely on fixed widths that shift with content
- ANSI codes break naive string matching
- `task add` output embeds the short_id inside a sentence, not as a standalone field
- No way to get structured data about a single task

**Recommendation:**
Add `--json` flag to all data-returning commands. When set:
- Output a single JSON object (for single-entity commands) or JSON array (for list commands)
- Suppress `printWriteContext` banner or nest it under a `"context"` key
- Include all fields, not truncated
- Example for `task add --json`:
  ```json
  {"short_id":"t_abc123","id":"01ABCDEF...","title":"Water mint patch","priority":"now","status":"open","created_by":"01USER..."}
  ```
- Example for `task list --json`:
  ```json
  [{"short_id":"t_abc123","title":"Water mint patch","status":"open","priority":"now","updated_at":"2026-02-20T..."}]
  ```

**Implementation note:** The `--json` flag can be added to the `flag.NewFlagSet` in each command. When set, output `json.Marshal` to stdout and skip the table formatting. This is a pure addition -- no existing behavior changes.

---

## 2. Missing `task show` / `task get` Command

**Severity: High**

There is no way to retrieve details of a single task by ID. An AI agent that creates a task and later needs to check its status, priority, assignees, blockers, subtasks, or collections must:
- Run `task list` and parse the entire table
- Or run raw SQL: `sqlite3 dooh.db "SELECT ... FROM tasks WHERE short_id='t_xxx'"`

**Recommendation:**
Add `task show --id <short_id|id>` that returns:
- All task fields (title, status, priority, due_at, scheduled_at, description, etc.)
- Assignee list
- Blocker list (with their statuses)
- Subtask list (with their statuses)
- Collection memberships
- Created/updated timestamps and actors

With `--json`, this becomes the primary way an agent reads task state.

---

## 3. Missing `task update` Command

**Severity: High**

There is no way to update a task's title, priority, due date, scheduled date, or description after creation. The only mutations are status changes (complete/reopen/archive) and delete. The schema supports `description`, `due_at`, `scheduled_at`, `estimated_minutes`, `rollover_enabled`, and `skip_weekends`, but none of these are settable from the CLI.

**Recommendation:**
Add `task update --id <id> [--title ...] [--priority ...] [--due ...] [--scheduled ...] [--description ...] [--estimate ...]` with event attribution.

---

## 4. No `task list` Filtering

**Severity: High**

`task list` has zero filter flags. The SQL is hardcoded:
```sql
SELECT title,status,priority,updated_at,short_id FROM tasks
WHERE deleted_at IS NULL ORDER BY updated_at DESC;
```

An AI agent cannot:
- Filter by status (show only open tasks)
- Filter by priority
- Filter by assignee
- Filter by collection/tag
- Limit results
- Paginate

The TUI has rich filtering (status/priority/assignee/tag/text), but none of this is exposed to CLI list commands.

**Recommendation:**
Add flags to `task list`:
- `--status open|completed|archived|all` (default: `open`)
- `--priority now|soon|later|all`
- `--assignee <user_id>`
- `--collection <collection_id>`
- `--limit N` / `--offset N`
- `--sort updated|priority|scheduled|created`
- `--order asc|desc`

---

## 5. No CLI Command to Query Audit Events

**Severity: High**

The `events` table is the core audit trail with proper `actor_user_id`, `key_id`, and `client_type` attribution. But there is no CLI command to query it. The README instructs users to run raw SQLite:

```bash
sqlite3 ./dooh.db "select seq,event_type,actor_user_id,key_id,client_type,occurred_at from events order by seq desc limit 20;"
```

**Recommendation:**
Add `event list` (or `audit list`) command:
- `--limit N` (default: 20)
- `--type <event_type>` filter
- `--actor <user_id>` filter
- `--client-type human_cli|agent_cli` filter
- `--since <timestamp>`
- `--json` support
- Always show: seq, event_type, actor_user_id, client_type, aggregate_type, aggregate_id, occurred_at

This directly supports the project's goal of "clear record of whether human or AI is making each change."

---

## 6. Missing `collection show` and Collection-Task Management

**Severity: Medium**

- No `collection show --id <id>` to see collection details and member tasks
- No `task collection add --id <task> --collection <collection>` to add a task to a collection
- No `task collection remove --id <task> --collection <collection>`
- The `task_collections` join table exists in the schema but has no CLI commands

An AI agent cannot organize tasks into collections without raw SQL.

**Recommendation:**
Add:
- `collection show --id <id>` -- returns collection details + member tasks
- `task tag --id <task> --collection <collection>` (or `task collection add`)
- `task untag --id <task> --collection <collection>` (or `task collection remove`)

---

## 7. `printWriteContext` Banner Pollutes Output

**Severity: Medium**

Every write command prints a context banner before the result:
```
context profile=ai mode=ai user=01ABCDEF key=aaaaaaaa db=./dooh.db
created task t_abc123 (Water mint patch)
```

This banner is useful for humans verifying identity but problematic for agents:
- The first line is not the command result
- Parsing must skip it
- The key prefix is exposed in every output (minor security consideration)

**Recommendation:**
- With `--json`: suppress the banner entirely (or nest under `"context"` key)
- With `--quiet` or `--no-context`: suppress the banner
- Keep current behavior as default for human use

---

## 8. Inconsistent ID Exposure in Command Output

**Severity: Medium**

IDs appear differently across commands:

| Command | Output pattern | ID location |
|---|---|---|
| `task add` | `created task t_abc123 (title)` | Embedded in sentence |
| `task complete` | `completed task t_abc123` | Embedded in sentence |
| `task list` | `TITLE ... TASK_ID` table | Last column |
| `collection add` | `created collection c_abc123 (name) color=#FF7A59` | Embedded in sentence |
| `user create` | `created user 01ULID (name)` | Full ULID, not short_id |
| `whoami` | `user=01ULID` | key=value line |

An agent must use different regex patterns for every command. With `--json` this becomes moot, but even without it, a consistent `id=<value>` suffix would help.

---

## 9. Exit Code Is Always 0 or 1

**Severity: Low**

`main.go` exits 0 on success and 1 on any error. There is no distinction between:
- Auth failure (could retry with different key)
- Not found (task ID doesn't exist)
- Validation error (missing required flag)
- Permission denied (scope violation)
- Conflict (dependency cycle, already completed)

**Recommendation:**
Consider distinct exit codes for categories:
- 0: success
- 1: general error
- 2: usage/validation error (bad flags)
- 3: auth failure
- 4: not found
- 5: permission denied
- 6: conflict/precondition failure

---

## 10. Help Text Is Minimal

**Severity: Low**

The top-level help is a single line:
```
dooh (pronounced duo)
global flags: --profile <name> --config <path>
commands: config, db, setup, demo, login, env, context, user, key, task, collection, export, tui, whoami, version
```

There is no:
- Per-command help (`dooh task --help` returns `task subcommand required` error)
- Flag documentation (flag defaults are in code but not displayed)
- Usage examples
- Scope requirement hints

`flag.NewFlagSet` with `ContinueOnError` + `fs.SetOutput(io.Discard)` means `--help` on subcommands fails silently and returns a generic error.

**Recommendation:**
- Make `dooh task` (no subcommand) print task subcommand list and flag summary
- Make `dooh task add --help` print flag descriptions
- Remove `fs.SetOutput(io.Discard)` and let flag package render help
- Add brief examples in help text

---

## 11. No Batch Operations

**Severity: Low**

Every mutation is single-item. There is no way to:
- Complete multiple tasks: `task complete --id t_1 --id t_2`
- Add multiple tasks from a list
- Bulk-assign a user to several tasks

For an AI agent managing a project, this means N sequential commands for N operations.

**Recommendation:**
Consider accepting comma-separated or repeated `--id` flags for status-change commands. Lower priority than `--json` and filtering, but useful for productivity.

---

## 12. No `user list` Without Admin Scope

**Severity: Low**

`user list` requires `users:admin` scope, which AI keys typically don't have. But an AI agent needs user IDs to run `task assign add --user <id>`. Currently the agent must already know user IDs out-of-band.

**Recommendation:**
Add a `user lookup` or lower-privilege user list that returns only `id` + `name` for active users, requiring only `tasks:read` scope (since you need user IDs to understand task assignments).

---

## Attribution/Audit Trail Assessment

The existing attribution model is well-designed:

**What works well:**
- Every mutation writes to the `events` table with `actor_user_id`, `key_id`, and `client_type`
- `client_type` is `human_cli` or `agent_cli`, directly derived from the API key -- not from a user-settable mode flag
- The `outbox` table records events for downstream consumption
- `created_by` and `updated_by` on tasks/collections track the last actor
- Tests verify attribution correctness (`TestMutationsWriteEventAttributionForHumanAndAI`)

**What needs improvement:**
- No CLI command to query events (issue #5 above)
- No `--json` on `whoami` to let agent confirm its own identity programmatically
- The `events.payload_json` stores relevant context but is not queryable from CLI
- No `last_modified_by_actor_type` in task list output -- you can't see at a glance whether human or AI last touched a task without querying events

---

## Summary: Priority Recommendations

| Priority | Issue | Impact |
|---|---|---|
| P0 | Add `--json` output mode to all commands | Unblocks reliable AI agent automation |
| P0 | Add `task show --id` | Agents need single-task detail retrieval |
| P0 | Add `task update` command | Agents can't modify task metadata after creation |
| P1 | Add `task list` filtering (`--status`, `--priority`, `--assignee`, `--limit`) | Agents process too much data without filters |
| P1 | Add `event list` / `audit list` CLI command | Core audit trail is CLI-inaccessible |
| P1 | Add task-to-collection membership commands | Schema supports it, CLI doesn't |
| P2 | Add `collection show` | Agents can't inspect collection members |
| P2 | Suppress context banner with `--quiet` | Cleaner output parsing |
| P2 | Improve help text and per-command `--help` | Discoverability |
| P3 | Distinct exit codes by error category | Better error handling in scripts |
| P3 | Batch operations | Productivity for multi-item workflows |
| P3 | Lower-privilege user lookup | Agent can discover user IDs for assignment |

---

## Appendix: Current Command Inventory

### Commands available:
```
dooh version
dooh config show|init
dooh db init
dooh setup demo
dooh demo seed
dooh login
dooh env
dooh context show|set|clear
dooh user create|list
dooh key create|revoke
dooh task add|list|complete|reopen|archive|delete|block|unblock|subtask|assign
dooh collection add|list|link|unlink
dooh export site
dooh tui
dooh whoami
```

### Commands missing for AI-complete workflow:
```
dooh task show --id <id>           # get single task detail
dooh task update --id <id> ...     # modify title/priority/due/description
dooh task collection add ...       # add task to collection
dooh task collection remove ...    # remove task from collection
dooh collection show --id <id>     # get collection detail + members
dooh event list                    # query audit trail
dooh user lookup                   # list users without admin scope
```

### Flags missing on existing commands:
```
--json                             # on all data commands
--status, --priority, --assignee   # on task list
--collection                       # on task list
--limit, --offset                  # on task list, collection list, event list
--quiet / --no-context             # on write commands
```
