# dooh: Goals, Priorities, and Interface Assessment

_Last updated: 2026-02-21_

This document synthesizes what the project is for, what should be built next, what
needs improvement, and where the current interface is confusing or inelegant. Items are
prioritized throughout.

---

## 1. Project Goals

**dooh** is a local-first task and goal management system designed for one human and one
AI agent sharing a machine. The name ("do?, oh!") reflects the pair dynamic: the human
asks "do?", the AI replies "oh!" (or vice versa).

### Core goals

1. **Clear attribution for every action.** Every mutation is recorded with the actor's
   identity (human or AI), key, and timestamp. This makes it auditable and trustworthy
   even when the AI is running autonomously for extended periods.

2. **Ergonomic for both actors.** Humans get a keyboard-first TUI with beautiful themes
   and natural-language timestamps. AI agents get machine-readable output, precise scopes,
   and stable flag interfaces. Neither interface should be an afterthought.

3. **Local-first, no cloud dependency.** A single SQLite database with WAL mode and
   event sourcing. No sync server, no API to call, no credentials to rotate externally.
   The pair works on the same machine and shares the same database.

4. **Sufficient for real project management.** Tasks with dependencies, subtasks,
   priorities, scheduled/due dates, estimates, and assignments. Collections for projects,
   goals, areas, and tags, with hierarchical links. Not a toy.

5. **Readable audit trail.** The append-only events table is the canonical history.
   System rollback is possible. The pair should be able to answer "who changed this and
   when?" for any task without raw SQL.

6. **Static website integration.** A read-only JSON export (`export site`) lets task
   data appear on external dashboards or sites without running any server.

### What it is not (MVP constraints)

- Not a multi-user, multi-machine, or cloud-synced system.
- Not a mobile or web-first tool.
- Not an AI assistant itself—dooh is the shared workspace the pair manages together.
- Not a general project management SaaS replacement.

---

## 2. Features to Implement Next

Ranked by impact and dependency order. P0 items are blockers for reliable AI-agent use.
P1–P2 items are high-value and tractable. P3+ items are worth doing but not urgent.

### P0 — Critical for AI-agent usability

**2.1 `--json` output on all data-returning commands**

Every command outputs fixed-width ANSI tables or embedded-sentence results. There is no
machine-readable mode. An AI agent must strip ANSI codes, parse variable-width columns,
and use different regex patterns for every command. This is fragile and will break as
output format evolves.

Add `--json` to: `task add`, `task list`, `task show`, `task update`, `task complete`,
`task reopen`, `task archive`, `task delete`, `collection add`, `collection list`,
`collection show`, `event list`, `whoami`, `context show`, `user list`.

When `--json` is set: output single JSON object or array (never mixed), suppress the
`printWriteContext` banner or nest it under a `"context"` key, and include all fields
untruncated.

**2.2 `task show --id <id>`**

No command retrieves the full detail of a single task by ID. To inspect a task an AI
agent must parse the entire `task list` table or drop to raw SQL. Add `task show` returning
all task fields, assignees, blockers (with their statuses), subtasks (with statuses),
collection memberships, and created/updated actors. With `--json`, this becomes the
primary read path for agents.

**2.3 `task update --id <id> [flags]`**

After creation, a task's title, priority, due date, scheduled date, description, and
estimate cannot be changed from the CLI. Only status transitions (complete/reopen/archive)
and relationship mutations exist. Add `task update` accepting any subset of `--title`,
`--priority`, `--due`, `--scheduled`, `--description`, `--estimate`, `--rollover`,
`--skip-weekends`.

**2.4 TUI stability baseline (P0 from TUI_ROADMAP)**

Before adding TUI scope, the existing rendering needs to be trustworthy:
- Footer hotkeys line must be visible at all times (currently inconsistent).
- Title/filter/footer background must fill the exact terminal width.
- Column headers must align with row columns at widths 80/100/140.
- Idle flicker must be eliminated.

Implementation approach: move all line composition to plain-text segments first, apply
style last; use ANSI-aware width helpers (`visibleWidth`, `truncateVisible`, `padVisible`);
reserve fixed layout regions for header, footer, and viewport body.

---

### P1 — High value, moderate effort

**2.5 `task list` filtering flags**

`task list` has no filter options. The SQL is hardcoded and returns all non-deleted tasks
ordered by `updated_at`. Add: `--status open|completed|archived|all` (default `open`),
`--priority now|soon|later|all`, `--assignee <user_id>`, `--collection <collection_id>`,
`--limit N`, `--offset N`, `--sort updated|priority|scheduled|created`,
`--order asc|desc`.

**2.6 `event list` CLI command**

The events table is the audit core, but it is only queryable via raw `sqlite3` commands.
Add `event list` with `--limit N` (default 20), `--type <event_type>`, `--actor <user_id>`,
`--client-type human_cli|agent_cli`, `--since <timestamp>`, and `--json`. Output columns:
seq, event_type, actor_user_id, client_type, aggregate_type, aggregate_id, occurred_at.

**2.7 Task-to-collection membership commands**

The `task_collections` join table exists and the schema fully supports M2M membership,
but no CLI commands expose it. Add `task collection add --id <task> --collection <id>`
and `task collection remove --id <task> --collection <id>`. Without these, an AI agent
cannot organize tasks into collections without raw SQL.

**2.8 TUI filtering and sorting UX (P1 from TUI_ROADMAP)**

The tokenized quick-filter syntax (`#tag`, `~area`, `^goal`, `@assignee`, `!overdue`)
is partially implemented but not fully consistent. Formalize a filter AST with AND
semantics across token types, add top-bar chips reflecting parsed tokens, and add a sort
chip + `o` keybinding for cycling sort modes. Support quoted multi-word tokens
(`#"Deep Work"`).

---

### P2 — Important, lower urgency

**2.9 `description` and `urls` fields on tasks**

Tasks have no description or URL fields. Add both as schema migrations:
`tasks.description TEXT NOT NULL DEFAULT ''` and `tasks.urls TEXT NOT NULL DEFAULT ''`
(newline-delimited for MVP). Expose via `task add --description --url` (repeatable)
and `task update`. Show in TUI expanded card view. Remove `groups` from TUI at the same
time.

**2.10 `collection show --id <id>`**

No command shows a collection's details plus member tasks. Add `collection show` returning
name, kind, color, parent/child collections, and member tasks with their statuses.
Essential for an AI agent building project status summaries.

**2.11 `--quiet` / banner suppression**

Every write command prepends a context banner (`context profile=ai mode=ai user=... db=...`)
before the result. This is useful for human verification but is the first line of output
for agents, requiring every caller to skip it. With `--json`, suppress the banner (or
nest under `"context"`). With `--quiet`, suppress it regardless of output format.

**2.12 Per-command help text**

`dooh task` (no subcommand) returns a generic error. `dooh task add --help` fails
silently (flags output is discarded). Remove `fs.SetOutput(io.Discard)` and add a brief
flag summary and usage examples to each subcommand. The help text doesn't need to be
exhaustive, but it should name all flags and their defaults.

**2.13 Remove `groups` from codebase**

The `groups` collection kind appears in the TUI expanded task card but is ambiguous and
unwanted. It is already flagged for removal in TUI_ROADMAP. Remove from TUI output,
help text, and docs. The data model can retain it for backward compatibility but no new
tasks should be added to group-kind collections via normal workflows.

---

### P3 — Useful but not urgent

**2.14 Area navigation in TUI (P3 from TUI_ROADMAP)**

No dedicated areas view exists (unlike projects and goals which have progress views).
Add a `6` key shortcut for areas, a dedicated areas view with completion and counts, and
drill-in scoping by entering on an area row.

**2.15 Theme system redesign (P4 from TUI_ROADMAP)**

Current themes use ad-hoc palette values with hardcoded 256-color fallbacks that can
look inconsistent across terminals. Replace with semantic theme tokens (text/muted/accent/
success/warn/danger/chart1-4). Add contrast validation tests. Add at least 2 more light
themes; the current set is dark-heavy.

**2.16 Distinct exit codes by error category**

All errors exit with code 1. Callers (including AI agents) cannot distinguish auth
failures from not-found from validation errors without parsing stderr. Add: 2 for
usage/validation, 3 for auth failure, 4 for not found, 5 for permission denied,
6 for conflict/precondition failure.

**2.17 Batch operations**

All mutations are single-item. An AI agent completing 10 tasks makes 10 sequential calls.
Accept comma-separated or repeated `--id` flags on `task complete`, `task archive`,
`task assign add`. Lower priority than filtering and structured output, but useful for
multi-step workflows.

**2.18 Lower-privilege user lookup**

`user list` requires `users:admin` scope, which AI keys do not have. But an AI agent
needs user IDs to run `task assign add`. Add `user lookup` (or a read-only user list)
requiring only `tasks:read` scope. Return only `id` and `name` for active users.

---

### P4 — Optional, high effort

**2.19 Bubble Tea primitives for layout/viewport (P5 from TUI_ROADMAP)**

Current Bubble Tea use is mostly event loop and key handling. Adopting `bubbles/viewport`
for the task body, a command palette (`:`) for discoverable actions, and lightweight
expand/collapse animations would make the TUI feel more polished. Defer until P0–P2 are
stable.

**2.20 TUI write operations**

The TUI is fully read-only. Power users who want to create, edit, or complete tasks
without leaving the TUI must switch to the CLI. A minimal write surface (quick-add via
`n`, inline complete via `x`, inline priority toggle) would close this gap. This requires
careful modal design to avoid clashing with existing filter interactions.

---

## 3. What Should Be Improved

### 3.1 Setup ceremony

The manual first-time setup is 6 steps (db init, user create, key create, login, context
set, verify). `setup-stable.sh` hides this, but the script is not the documented default
path. A single `dooh init` command should do all of this interactively, prompting for
a user name and outputting the final context state. The script can remain as a
non-interactive CI option.

### 3.2 Auth model clarity

The relationship between `DOOH_MODE`, `DOOH_AI_KEY`, and key `client_type` is subtle.
Mode is technically auto-derived from the key type, but `DOOH_MODE` is still referenced
in some error messages and the README. An AI agent reading the docs has to understand all
three concepts before knowing what environment variables to set. Simplify: if `DOOH_AI_KEY`
is set, that is sufficient. Document that `DOOH_MODE` is deprecated for auto-derived keys.

### 3.3 Overlapping diagnostic commands

`dooh whoami`, `dooh context show`, `dooh config show`, and `dooh env` all print
overlapping identity/config information. Their relationship is not obvious. Consolidate:
`whoami` should be the single "who am I and what am I connected to" command. `context show`
and `config show` can remain for config debugging, but their output should not repeat
identity information that `whoami` already shows.

### 3.4 Event payload is not queryable from CLI

`events.payload_json` stores the full delta for each mutation, but `event list` (once
added) should also expose the payload—or at least the changed fields—in `--json` mode.
Currently the payload is useful only to developers reading the DB directly.

### 3.5 Error messages are minimal

Most error paths print a single lowercase sentence without structured context. An AI
agent receiving `task not found` has no field telling it _which_ ID was not found.
Error output should include the relevant IDs and operation, especially when `--json` is
set (output a JSON error object with `{"error": "...", "code": "not_found", "id": "..."}`).

### 3.6 Two binaries create cognitive overhead

The stable/dev channel split (`dooh` vs `dooh-dev`) is a sound isolation strategy,
but it means users and AI agents must track which binary they're running. A `--channel`
flag or a single binary that reads `DOOH_HOME` for isolation would be simpler. The
current approach works but adds a surface for mistakes (running `dooh` when you meant
`dooh-dev` against the wrong DB).

### 3.7 `task list` does not show collection membership

The CLI task list shows title, status, priority, updated, and task ID. There is no
column for which project or area a task belongs to. For a human or AI reviewing a full
task list, it is impossible to understand task context without drilling into each task.
Add an optional `--show-collections` flag or a narrower `--show-project` / `--show-area`
column.

---

## 4. What Is Confusing or Inelegant About the Current Interface

### 4.1 Context banner on every write command (highest friction)

Every mutation prints a banner before the result:
```
context profile=ai mode=ai user=01ABCDEF key=aaaaaaaa db=./dooh.db
created task t_abc123 (Water mint patch)
```

The banner was added for human verification of identity but creates two problems: (1) an
AI agent must skip the first line of every command's output, and (2) the truncated key
prefix is exposed in terminal history for every write. This is the single most
friction-generating behavior for scripted use.

### 4.2 IDs are inconsistent across commands

| Command | Output | ID format |
|---|---|---|
| `task add` | `created task t_abc123 (title)` | embedded in sentence |
| `task complete` | `completed task t_abc123` | embedded in sentence |
| `task list` | table, last column | right-padded column |
| `collection add` | `created collection c_abc123 (name)` | embedded in sentence |
| `user create` | `created user 01ULID (name)` | full ULID, not short_id |
| `whoami` | `user=01ULID` | key=value pair |

An AI agent needs a different extraction strategy for every command. Adding `--json`
fixes this for structured callers; a consistent `id=<value>` suffix would help for
non-JSON callers.

### 4.3 TUI filter syntax is undiscoverable

The quick-token syntax in the text filter (`#tag`, `~area`, `^goal`, `@assignee`, `!due`,
`!overdue`, `!nodue`) is powerful but completely invisible until you read the README or
press `f`. There is no tooltip, no example shown in the filter input placeholder, and no
help text inside the TUI itself. The footer line (when visible) does not mention filter
tokens. New users will use only fuzzy text matching and never discover scoped filtering.

### 4.4 `groups` appears in task detail but has no defined purpose

The expanded task card in the TUI shows `groups:` as a collection category alongside
`projects`, `goals`, `areas`, and `tags`. Nothing in the docs defines what a group is
or how it differs from an area. The TUI_ROADMAP already calls for its removal. Until
removed, it creates confusion about the collection taxonomy.

### 4.5 TUI is read-only with no indication of how to write

A user launching the TUI for the first time sees a beautiful task list but there is no
affordance pointing toward how to create or edit tasks. The TUI shows `q` to quit but
nothing explains that all mutations happen in the CLI. At minimum, the footer or a
splash line should note "use `dooh task add` to create tasks."

### 4.6 `dooh env` output purpose is unclear

`dooh env` outputs the resolved environment variables. The distinction between `env`,
`context show`, and `config show` is not documented in the command output itself.
A user who runs all three will see overlapping information and won't know which is
canonical. Adding a one-line description at the top of each command's output (or making
`--help` explain the difference) would resolve this.

### 4.7 `--bootstrap` flag has no discoverable documentation

The `--bootstrap` flag on `user create` and `key create` bypasses the auth requirement
during initial setup. It is essential for first-time setup and in the quickstart scripts,
but it doesn't appear in the command-level help (which is minimal or missing). A user
trying to set up dooh from scratch without the setup scripts will not know to use it.

### 4.8 Priority `now/soon/later` has no displayed semantics

The three-level priority system (`now`, `soon`, `later`) is clean, but there is no
explanation anywhere in the CLI or TUI of what each level means operationally (is `now`
for today? this sprint? blocking?). The TUI shows the priority label but doesn't help
users decide which level to choose when adding a task. A brief definition in help text
or the TUI would make the system feel more intentional.

### 4.9 `export site` has no bundled viewer

`dooh export site --out ./site-data` produces well-structured JSON
(`tasks.json`, `collections.json`, `metrics.json`). But the README contains no pointer
to a viewer, template, or example website. The output is useful only to users who already
have a site to consume it. Providing a minimal HTML file or linking to an example would
make the export feature immediately usable for more users.

### 4.10 `collection list` output does not show kind or task count

The collection list shows name and short ID but not the kind (`project`, `goal`, `area`,
`tag`) or how many tasks belong to each collection. A user looking at a collection list
cannot distinguish a project from a goal from an area at a glance. Adding kind and
member count columns would make the list immediately useful.

---

## Priority Summary

| Priority | Item | Category |
|---|---|---|
| P0 | `--json` output mode on all commands | Next feature |
| P0 | `task show --id` | Next feature |
| P0 | `task update` command | Next feature |
| P0 | TUI stability (footer, width, alignment, flicker) | Improvement |
| P1 | `task list` filtering flags | Next feature |
| P1 | `event list` CLI command | Next feature |
| P1 | Task-to-collection membership commands | Next feature |
| P1 | Context banner suppression (`--quiet`, `--json`) | Interface inelegance |
| P1 | Consistent ID format in command output | Interface inelegance |
| P2 | `description` and `urls` task fields | Next feature |
| P2 | `collection show` command | Next feature |
| P2 | Per-command help text | Improvement |
| P2 | Remove `groups` from UX | Interface confusion |
| P2 | TUI filter token discoverability | Interface confusion |
| P2 | `collection list` show kind + task count | Interface inelegance |
| P3 | Simplified first-time setup (`dooh init`) | Improvement |
| P3 | Auth model documentation clarity | Improvement |
| P3 | Area navigation view in TUI | Next feature |
| P3 | Distinct exit codes by error category | Improvement |
| P3 | Batch operations | Next feature |
| P3 | Lower-privilege user lookup | Next feature |
| P3 | `task list` show collection column | Improvement |
| P3 | TUI note on how to write tasks | Interface confusion |
| P4 | Theme system redesign | Improvement |
| P4 | Bubble Tea viewport + command palette | Next feature |
| P4 | TUI write operations | Next feature |
| P4 | `export site` bundled viewer / example | Interface confusion |
