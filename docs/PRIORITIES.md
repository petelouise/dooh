# dooh: Vision, Priorities, and Interface Critique

_2026-02-21 — supersedes all prior versions of this document_

---

## The Premise

dooh is a shared workspace for a human and an AI working together on the same machine.
Not a task manager with an AI bolt-on. Not an AI tool with a human approval screen. A
genuine pair: two actors, one database, every action attributed, full history retained.

The system's deepest value proposition is accountability over time — not just "what tasks
exist" but "who did what, when, and in which order." The event log is the product.
Everything else is interface to it.

---

## What Success Looks Like

When dooh is working well:

- The human opens the TUI and immediately sees what the AI has been doing in their
  absence — which tasks the AI touched, created, or completed, without needing to read
  the audit log.
- The AI opens its shell, runs `dooh whoami`, and has everything it needs — no brittle
  text parsing, no manual ID extraction, no raw SQL.
- A new task moves from `open` to `in progress` to `done`, and the event log tells a
  coherent story of who pushed it forward at each step.
- The collection hierarchy drives where you look, not just how things are tagged: drilling
  from a goal into an area into a project into a task is one gesture at each level.
- First-time setup takes one command.
- There is one binary.

Most of these are not true yet. This document describes how to get there.

---

## Proposed Big Changes

These are not feature requests — they are design shifts. Some require rethinking existing
interfaces. All are worth the disruption.

---

### 1. Make the TUI writable

The TUI is currently read-only. It is beautiful and well-designed for browsing, but it
enforces a workflow split that cuts against the tool's own premise: humans look here,
then switch to a different interface (the CLI) to act. This context-switch is small but
persistent, and it makes the TUI feel like a dashboard rather than a workspace.

**The shift:** treat the TUI as the primary interface for humans. The CLI becomes the
scripting layer — the interface for agents and for automation. Humans who want to work
in the terminal should not need to leave the TUI to manage tasks.

**Minimum writable surface for a first pass:**
- `n` to quick-add a task (title + priority; opens a two-field inline form, not a modal)
- `x` or `Enter` (when on a task row) to complete
- `e` to open an inline editor for the selected task's title and priority
- `d` to archive

This is not a full TUI editor — it is enough to close the loop without context-switching.
Description, URLs, and relationships remain CLI territory for now.

**Risk:** write interactions in a TUI that already has rich keybindings require careful
modal design. The existing filter interactions (`f`, `g`, `a`, etc.) set a good precedent.
Follow the same pattern: a key opens an input mode, `Enter` commits, `Esc` cancels.

---

### 2. Surface the pair dynamic in every view

The central differentiator of dooh — human+AI attribution on every action — is almost
invisible in the UI. Task rows do not indicate who last touched them. There is no "what
happened while I was away" view. The pair is a first-class concept in the data model but
a second-class citizen in the interface.

**The shift:** make actor type visible at a glance, everywhere.

**Concrete changes:**

- **Actor glyph in task rows.** Add a one-character column after the status icon:
  `H` (human, styled in the human accent color) or `A` (AI, styled in the AI accent
  color) showing who last modified the task. This is already in the events table — it
  just needs to be surfaced.

- **A "since you were away" view.** A new TUI view (key `6`) that shows tasks the AI
  touched since the human's last interaction. "Last interaction" can be the timestamp of
  the last `human_cli` event. This view answers the most common pair question — "what did
  the AI do?" — without requiring the user to read the audit log.

- **`dooh log`** as a first-class command. A beautiful, colorized event stream, formatted
  like `git log --oneline`, with actor type, event kind, and task/collection name. The
  key is not the data (that already exists via `dooh event list --json`) but the
  formatting: fast, readable, useful at a glance. Human events in one color, AI events
  in another. Default: last 40 events. Filterable by actor, type, or aggregate.

---

### 3. Add `in-progress` as a task status

The current lifecycle is `open → completed | archived`. There is no way to mark a task
as "being worked on right now." This gap is small for humans, who can hold context
mentally, but significant for the pair — if the AI is executing a multi-step task, there
is no machine-readable signal that the task is actively in flight versus waiting.

**The shift:** add `in-progress` between `open` and `completed`.

- An AI agent should call `task start --id <id>` when beginning work, and `task complete`
  when done. This makes the pair's division of labor machine-visible.
- The TUI should distinguish `in-progress` tasks with a distinct status icon (◎ or ▶)
  and show them prominently in the today view.
- `in-progress` tasks should appear at the top of the task list regardless of sort mode,
  since they are the most urgent context.
- The event type becomes `task.started`, joining `task.created`, `task.completed`, etc.

This requires a schema migration (add `in_progress` to the status CHECK constraint and
a `started_at` timestamp column) and updates to the TUI, CLI, and export.

---

### 4. Let the collection hierarchy drive navigation

The `collection_closure` table encodes a full ancestor/descendant tree across the
collection hierarchy. This is a significant piece of infrastructure that is completely
invisible in the navigation. The TUI has five flat views numbered 1–5. The collection
hierarchy should be the navigation, not a filter facet.

**The shift:** the TUI's primary navigation axis is the collection tree, not the view
tabs.

**Concrete design:**

- Entering a project from the project view (current behavior) is the right primitive.
  Extend it: entering a goal shows that goal's projects; entering a project shows its
  areas; entering an area shows its tasks. Each drill-in scopes the view, and the scope
  chip in the filter bar shows the path (`Goal > Project > Area`).
- Add breadcrumb navigation: `Left` goes up one level in the hierarchy.
- The numbered views (`1`–`5`) become view-mode shortcuts within the current scope,
  not global scope-changers. From inside a project, `1` shows that project's tasks,
  `2` shows its progress, etc.
- Tab order in the header should reflect this: scope is the outermost dimension,
  view mode is the inner dimension.

This does not require new data — the collection_closure query already supports it.
It is a navigation redesign, not a data redesign.

---

### 5. Kill the dual binary

`dooh` and `dooh-dev` are separate binaries produced from the same codebase. The
isolation they provide (separate config dirs, separate databases) is real and useful.
The mechanism (two binaries) is unnecessary.

**The shift:** one binary, configured by `DOOH_HOME` or `--home`.

```bash
# stable (current default)
dooh task list

# dev channel, explicit
DOOH_HOME=~/.config/dooh-dev dooh task list

# or via alias in shell profile
alias dooh-dev='DOOH_HOME=~/.config/dooh-dev dooh'
```

The install script can set up both aliases. Channel identity (which home dir you're in)
can be shown in the `whoami` output and the TUI context banner. The binary itself does
not need to encode the channel.

This eliminates: two build targets, two install paths, two config dirs to document, and
the cognitive load of remembering which binary to invoke for which purpose.

---

### 6. Replace setup scripts with `dooh init`

First-time setup currently takes six manual steps (or a shell script that most users
will read skeptically). This is the worst possible first impression for a tool that
should feel elegant.

**The shift:** `dooh init` is an interactive first-time setup command.

```
$ dooh init
Welcome to dooh.
Your name: Pete
Creating database at ~/.local/share/dooh/dooh.db...
Creating user "Pete"...
Creating human admin key...
Key stored at ~/.config/dooh/auth/default.human.key
Context saved.

Ready. Run 'dooh task add --title "First task" --priority now' to begin.
```

If `DOOH_AI_KEY` is set, `dooh init` detects it and offers to register the AI user
and key in the same pass.

The existing setup scripts remain for non-interactive CI/bootstrap use, but `dooh init`
is what the documentation leads with.

---

### 7. Build the scheduling intelligence the schema promises

The schema has `rollover_enabled`, `skip_weekends`, `scheduled_at`, and
`estimated_minutes`. These are real scheduling concepts. But none of them drive any
behavior visible to the user — they are stored and forgotten.

**The shift:** make scheduling meaningful.

- **`dooh today`** as a CLI command (not just a TUI view): shows tasks scheduled for
  today, tasks that have rolled over from past days (if `rollover_enabled`), and tasks
  overdue. Output is ordered: overdue first, then today's tasks by priority, then
  tomorrow's candidates.
- **`rollover`** should visibly mark tasks that rolled over from a previous day with a
  distinct marker in both TUI and CLI output. Currently there is no indication that a
  task's scheduled date is stale.
- **`estimated_minutes`** should be summed in the today view to show total estimated
  time for the day, giving the pair a capacity signal.
- **`skip_weekends`** should be surfaced in the TUI: if a task is scheduled for a
  weekend, it should be automatically forwarded to Monday and marked as such.

---

## Immediate Priorities

What to work on now, given the above direction. The big changes above are the north
star; these are the immediate next steps ordered by impact.

**P0 — Do before anything else**

- **TUI stability baseline** (see TUI_ROADMAP.md P0): footer visibility, full-width
  background fill, column alignment, flicker elimination. The TUI is nearly there but
  not yet trustworthy at all terminal widths. Fix this before extending the TUI.

- **`in-progress` task status**: schema migration, `task start` CLI command, TUI icon,
  event attribution. Small scope, high leverage for pair visibility.

**P1 — High value, moderate effort**

- **Actor glyph in task rows**: surface `client_type` from the most recent event per
  task in the task list query. Add one-character column. No new data, new presentation.

- **`dooh log` command**: format the event stream beautifully. Pull from `event list`
  data, apply colors by actor type, show event kind and aggregate name. The hard part
  is making it feel like `git log`, not like a database dump.

- **TUI quick-add** (`n` key): inline two-field form (title + priority cycle). Commit on
  `Enter`. The pattern is already established by the filter input mode.

- **`urls` field on tasks**: schema migration, `task add --url`, `task update --url`,
  TUI expanded card display. Remove `groups` from TUI in the same pass.

**P2 — Important, not urgent**

- **"Since you were away" view** in TUI: tasks touched by the other actor since the
  viewer's last event. Requires computing "last human event" and "last AI event"
  timestamps.

- **Collection hierarchy navigation**: breadcrumb path in filter bar, `Left` to go up
  one scope level. The drill-in already works; add the return path.

- **`dooh init`**: interactive first-time setup. Replace the documentation's manual
  6-step sequence.

- **Area view in TUI**: dedicated areas view with completion counts, `6` shortcut. Reuse
  the progress-row loader used by project/goal views.

**P3 — Polish and completeness**

- **Dual binary → single binary**: update install script, update documentation, add
  `--home` flag. Low risk, meaningful simplification.

- **Theme redesign** (TUI_ROADMAP P4): semantic tokens, contrast validation, 2 more
  light themes.

- **Scheduling intelligence**: `dooh today` CLI command, rollover markers, estimate
  summation in today view, skip-weekends forwarding.

- **Batch operations** on status-change commands: accept repeated `--id` flags on
  `task complete`, `task archive`, `task assign add`.

- **`dooh init`** non-interactive mode: `--name`, `--ai-key`, `--db` flags for
  script-driven setup.

---

## Interface Critique

An honest assessment of what is currently confusing, inelegant, or in tension with the
tool's own goals. Ordered by severity.

---

### The TUI is a bystander

The most important interface problem is not a bug or a missing feature. It is a design
orientation. The TUI currently watches the pair work and reports on it. This is fine for
a dashboard but wrong for a workspace. A human using dooh day-to-day will spend most of
their time in the TUI — browsing tasks, checking project progress, planning the day —
and then leave it every time they want to act. That break in flow is the interface's
biggest friction point.

The fix is making the TUI writable (see Proposed Big Change 1 above), but the
orientation shift matters independently: every design decision in the TUI should ask
"what would a human want to do from here?" not just "what information is useful to show?"

---

### The pair is invisible

dooh's differentiator — two actors working together, every action attributed — barely
shows up in the UI. A human looking at a task list sees titles, priorities, and statuses.
They do not see whether a task was last touched by them or by the AI. They cannot tell,
at a glance, that the AI has been active. The "pair" is a data model concept that stops
at the database boundary.

Fixing this requires no new data, only new presentation: an actor glyph per task row,
a "since you were away" view, and `dooh log` as a daily-use command. These three things
together would make the pair dynamic visible and would give dooh a distinctive interface
identity.

---

### Priority semantics are undefined

The three-level priority system (`now`, `soon`, `later`) is clean and easy to use. But
what each level means is never stated. The README describes them as options; the TUI shows
them as labels; the CLI accepts them as strings. There is no guidance on when to choose
one over another.

This matters because the AI will be assigning priorities too. Without defined semantics,
the pair cannot maintain consistent priority discipline over time. A suggested definition:

- **`now`**: actively in progress or blocking something in progress. Work on this today.
- **`soon`**: committed for the current week. Not today, but not deferrable beyond the
  week.
- **`later`**: on the radar, not scheduled. Review during weekly planning.

This definition should appear in `dooh task add --help`, in the TUI footer or expanded
task card, and in the README.

---

### `groups` in the TUI detail card has no referent

The expanded task card shows `groups:` as a collection category alongside `projects`,
`goals`, `areas`, and `tags`. The collection schema has no `group` kind — the valid kinds
are `project`, `goal`, `tag`, `class`, `area`, and `custom`. The `groups` field in the
`row` struct appears to be populated from somewhere (possibly `class`-kind collections
or an earlier schema state), but its semantic meaning is undefined. Users seeing `groups:`
in the detail card do not know what it represents or how to populate it.

Remove `groups` from the TUI detail card. If `class`-kind collections are genuinely
useful, show them as `class:` with their own semantics defined. If they are not useful,
remove the kind from the schema.

---

### The filter token syntax is invisible

The TUI's text filter (`f`) supports a powerful scoped token syntax: `#tag`, `~area`,
`^goal`, `@assignee`, `!overdue`, `!nodue`, `!todaydue`. This is the fastest way to
scope a task list — faster than the dropdown filters for tags and assignees. But nothing
in the TUI surface indicates it exists. The filter input shows a blank cursor. The footer
shows hotkeys for view modes but not for filter tokens.

At minimum: show a one-line placeholder in the filter input field (`#tag ~area @user …`),
and add a token syntax hint to the TUI help screen (or the expanded footer row).

---

### `rollover_enabled` and `skip_weekends` are schema ghosts

These fields exist on every task record. They represent real, useful scheduling behavior.
But there is no CLI flag to set them during `task add` (only `task update` exposes them,
if it does at all), no TUI indicator that a task has them enabled, and no visible effect
— a task with `rollover_enabled=1` looks identical to one without it. They are promises
the interface has not kept.

Either implement the scheduling behavior these fields imply (see Proposed Big Change 7),
or remove them from the schema and the documentation. Invisible features are worse than
missing features: they create false confidence and cluttered data.

---

### The outbox has no consumer

The `outbox` table (status: pending/delivered/failed, with retry logic) is the
infrastructure for delivering events to external consumers. But no consumer exists. The
table accumulates rows that are never delivered and never cleaned up. Users who run
`sqlite3 dooh.db "select count(*) from outbox"` will find a growing list of pending
records with no explanation.

If the outbox is infrastructure for a future integration, document it clearly and add a
`dooh outbox status` command that shows pending/delivered/failed counts. If no integration
is planned for the foreseeable future, remove it — or at least add a periodic cleanup
that marks old pending rows as failed with a clear reason.

---

### Setup ceremony is hostile for newcomers

The manual first-time setup takes six steps, requires knowing ULID-format user IDs, and
involves running `sqlite3` directly. The setup scripts hide this, but the documentation
leads with the manual steps before the scripts. A newcomer following the README will hit
database concepts before they have created a single task.

The fix is `dooh init` (see Proposed Big Change 6), but even before that, the README
should lead with the setup script, not the manual steps. Manual steps belong in an
appendix.

---

### `dooh whoami` vs `dooh context show` vs `dooh env`

Three commands return overlapping identity and configuration information. Their
differences are not documented at the command level:

- `whoami`: who am I, what key am I using, what DB am I connected to
- `context show`: what context overrides are persisted locally
- `env`: what environment variables are currently resolved

A user who runs all three will see repetition without understanding what each is
authoritative for. With `--json`, this becomes especially confusing: which command's
output is the right one to parse in a script?

Clarify by making each command the canonical source for exactly one thing. `whoami`
should be the identity oracle — the one command to run when you need to confirm who you
are and what you are connected to. `context show` should focus on user-set overrides.
`env` should focus on the raw environment resolution chain. Brief descriptions at the
start of each command's output (or in `--help`) would resolve most of the confusion.

---

### The `export site` output has no bundled viewer

`dooh export site` produces clean, well-structured JSON. But there is no example HTML,
no template, no reference viewer. The feature is complete from the data side and empty
from the presentation side. Users who run the command get a directory of JSON files and
no indication of how to use them.

A minimal bundled viewer — a single `index.html` that reads `tasks.json` and
`collections.json` with no build step and no external dependencies — would make the
export feature immediately useful and demonstrate what the data model looks like to an
external consumer. This is a small amount of work for a large improvement in
discoverability.

---

## Priority Index

| Priority | Item | Category |
|---|---|---|
| P0 | TUI stability: footer, width, alignment, flicker | Polish |
| P0 | `in-progress` task status + `task start` CLI command | Design shift |
| P1 | Actor glyph (H/A) in task rows | Design shift |
| P1 | `dooh log`: beautiful event stream viewer | Design shift |
| P1 | TUI quick-add (`n` key) | Design shift |
| P1 | `urls` field on tasks; remove `groups` from TUI | Feature + cleanup |
| P2 | "Since you were away" TUI view | Design shift |
| P2 | Collection hierarchy navigation (breadcrumb + Left key) | Design shift |
| P2 | `dooh init` interactive setup command | UX improvement |
| P2 | Area view in TUI (`6` key) | Feature |
| P2 | Priority semantics: define and document `now/soon/later` | Clarity |
| P2 | Remove `groups` from TUI or give it a definition | Cleanup |
| P2 | Filter token syntax: placeholder + help hint in TUI | Discoverability |
| P3 | Dual binary → single binary with `--home` flag | Simplification |
| P3 | Theme redesign: semantic tokens + contrast tests | Polish |
| P3 | Scheduling intelligence: rollover, today CLI, estimates | Design shift |
| P3 | Batch operations on status-change commands | Feature |
| P3 | Outbox: consumer, status command, or removal | Cleanup |
| P3 | README: lead with setup script, move manual steps to appendix | Clarity |
| P3 | `whoami`/`context show`/`env` boundary clarification | Clarity |
| P4 | `export site` bundled HTML viewer | Feature |
| P4 | Bubble Tea viewport + command palette | Polish |
