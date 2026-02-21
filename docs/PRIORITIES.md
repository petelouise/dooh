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

## Design Shifts

These are not feature requests — they are design decisions that shape everything below.

---

### 1. The TUI stays read-only (for now)

The TUI is the human's reading surface. The CLI is the action layer — for both humans
and agents. This is a deliberate decision, not a gap. TUI write features are a real
future direction but are not being implemented now. See the Deferred section.

The orientation principle still applies: every design decision in the TUI should ask
"what would a human want to do from here?" not just "what information is useful to show?"
The most important improvements to the read experience are making the pair dynamic
visible (actor glyph, log view) and making navigation feel like drilling into a hierarchy
rather than switching between flat numbered views.

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

### 4. Redesign the collection taxonomy and let the hierarchy drive navigation

The collection system has two problems that need to be solved together: the taxonomy
is underspecified (four meaningful kinds plus two undefined catch-alls), and the
navigation doesn't use the hierarchy that's already in the data.

**The taxonomy decision** (see `docs/COLLECTION_MODEL.md` for full reasoning and
implementation detail):

Four kinds, no others: `area`, `goal`, `project`, `tag`. Remove `class` and `custom`.

The containment rules reflect that areas and goals are fundamentally different things —
areas are _where and when_ (life domains: home, work, school), goals are _why_
(outcomes: "launch Q2 product", "run a marathon"). They are orthogonal organizing axes,
both top-level, not one nested inside the other.

```text
area    top-level only; cannot belong to any other collection
          └── project, or task directly
goal    top-level, or nested under another goal (year > quarter)
          └── goal, project, or task directly
project must have ≥1 parent (area, goal, or both)
          └── task
task    can directly belong to: project, goal, or area
          └── ☐ checklist item   (text + checked; not a full task)
tag     applies to any collection or task; cascades down to member tasks
```

A project can have both a goal parent ("serves this outcome") and an area parent ("lives
in this domain"). This is the normal case for substantive work. The goal-project
relationship is containment, not a special link type — drilling into a goal shows its
projects.

**The navigation shift:** the TUI's primary axis is the collection tree, not numbered
flat views.

- Two root axes in the TUI: areas and goals. Both accessible from the top level.
- Entering an area or goal scopes the view; entering a project scopes further.
- The breadcrumb in the filter bar shows the path taken (`Work > Product redesign` or
  `Reduce churn > Product redesign`). Both are valid paths to the same project.
- `Left` goes up one level in the breadcrumb.
- Numbered view modes (`1`–`5`) apply _within_ the current scope, not globally.
- The TUI scope state (`ScopeKind`, `ScopeID`, `ScopeName`) needs a `ScopePath` field
  to support breadcrumb rendering and `Left` navigation.

The `collection_closure` table already supports multiple parents; the application layer
needs kind-aware constraint enforcement added to `collection link`.

---

### 5. One binary, configured by `DOOH_HOME`

`dooh` and `dooh-dev` are separate binaries produced from the same codebase. The
isolation they provide (separate config dirs, separate databases) is real and useful.
The mechanism (two binaries) is unnecessary.

**The target:** one binary, configured by `DOOH_HOME` or `--home`.

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

```text
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

### 7. Replace subtasks with a task checklist

The current subtask model — full tasks linked via a `task_subtasks` join table — creates
a hierarchy that collapses on itself. A subtask with its own priority, due date, assignee,
and collection membership is not meaningfully different from a task. The word "subtask"
implies simplicity but the implementation delivers full task complexity. There is no clear
line between a task and its subtasks, and the four-level structure of the system
(area/goal → project → task → step) loses its bottom level to ambiguity.

**The shift:** subtasks become a lightweight checklist on a task. Text plus a checked
state. Nothing more.

```text
area / goal
  └── project   (a body of work)
        └── task   (a unit of work)
              └── ☐ checklist item   (a step; just text + checked state)
```

If a step needs its own due date, assignee, or collection membership, it is not a
checklist item — it is a task in a project. The hierarchy already handles complex
work decomposition. The checklist handles simple ordered steps.

**Schema:** drop `task_subtasks`, add `task_checklist` (`id`, `task_id`, `text`,
`checked`, `position`). See `docs/COLLECTION_MODEL.md` for the full migration.

**CLI:** replace `task subtask add/remove` with `task checklist add/check/uncheck/remove`.

**TUI:** expanded task card shows `☐ step` / `☑ step` lines (read-only display).

**Auto-complete:** when all checklist items are checked, the parent task may
auto-complete — simpler to query than the current child-task-status approach.

---

### 8. Build the scheduling intelligence the schema promises

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

## Implementation Plan

Work through these in priority order. Within P0 and P1, some items are parallel — noted
explicitly below. The single hard constraint is **TUI stability gates all TUI feature
work**. Schema and CLI changes are independent and can land while TUI stabilization is
in progress; keep them in separate PRs.

---

### P0 — Do first; nothing else starts until these are done

**TUI stability baseline**

Footer visibility, full-width background fill, column alignment, flicker elimination.
The TUI is nearly there but not yet trustworthy at all terminal widths.

Implementation: move all line composition to plain-text segments first, apply style
last; add ANSI-aware width helpers (`visibleWidth`, `truncateVisible`, `padVisible`);
reserve fixed layout regions (header / body / footer); render only after `WindowSizeMsg`,
reflow on resize only.

Acceptance: columns align at 80/100/140 widths, footer always visible, top bar spans
full width, no idle flicker.

**`in-progress` task status** _(parallel with TUI stability — CLI lands first)_

Schema migration: add `in_progress` to the status CHECK constraint, add `started_at`
timestamp. New command: `task start --id <id>`. Event type: `task.started`. Export
updated. TUI icon (◎ or ▶) and row ordering (in-progress floats to top) land once
TUI stability is complete.

_CLI complete: `task start`, schema migration, `task.started` event, and exporter updated. TUI icon (◎/▶) and float-to-top ordering remain deferred until TUI stability is done._

---

### P1 — After P0; CLI-first items can begin during P0

**Checklist migration** _(schema + CLI; no TUI dependency)_

Drop `task_subtasks`. Add `task_checklist` table (see `docs/COLLECTION_MODEL.md` for
schema). Replace `task subtask add/remove` CLI commands with `task checklist
add/check/uncheck/remove`. Update `task show` output to include checklist items.
TUI expanded card display (read-only `☐/☑` rows) follows once TUI stability holds.

**`description` and `urls` fields on tasks** _(schema + CLI; TUI display after P0)_

Schema: `tasks.description TEXT NOT NULL DEFAULT ''` and `tasks.urls TEXT NOT NULL
DEFAULT ''`. CLI: `task add --description --url` (repeatable), `task update
--description --url --clear-urls`. TUI expanded card shows both fields; remove `groups`
row from the card in the same pass.

**`dooh log` command** _(pure CLI; no TUI dependency)_

A colorized event stream formatted like `git log --oneline`. Pulls from `event list`
data; applies color by actor type (`client_type`); shows event kind and aggregate name.
Default: last 40 events. Flags: `--actor human|ai`, `--type <event_type>`,
`--since <timestamp>`. The hard part is making it feel fast and readable, not like a
database dump.

**Actor glyph in task rows** _(TUI; after P0 stable)_

Surface `client_type` from the most recent event per task in the task list query. Add a
one-character column after the status icon: `H` or `A`, styled in the respective accent
color. No new data — new presentation of data that already exists.

---

### P2 — After P1 TUI work is solid

**Filter token syntax** _(small, low-risk TUI change; good first P2 item)_

Add a placeholder to the filter input (`#tag ~area @user …`) and a token syntax hint
to the TUI help screen or expanded footer row. No new functionality — just surface what
already works. Quoted multi-word tokens (`#"Deep Work"`) can land in the same pass.

**TUI sort controls**

`o` key cycles sort mode: default → priority → scheduled. Active sort shows as a chip
in the top bar. Sort changes row order without breaking expand/selection state.

**Area view in TUI**

Dedicated areas view accessible via the `6` shortcut. Shows areas with task completion
counts. Reuses the progress-row loader used by the existing project/goal views. Entering
an area row scopes the task list to that area.

**Collection hierarchy navigation**

Breadcrumb path in the filter bar showing the current scope path (`Work > Product
redesign`). `Left` key goes up one level. The drill-in already works — this adds the
return path. Requires extending `ScopeState` with a `ScopePath` slice.

**"Since you were away" view** _(depends on actor glyph work from P1)_

A TUI view showing tasks the non-current actor touched since the viewer's last event.
"Last human event" timestamp drives what counts as "while you were away" for the human
viewer. Key: `7` (or reassign once area view slot is confirmed).

**`dooh init`**

Interactive first-time setup: prompts for name, creates DB, creates human user and key,
stores context. Detects `DOOH_AI_KEY` and offers to register the AI user in the same
pass. See Design Shift 6 for the full flow.

**Priority semantics**

Define `now`/`soon`/`later` in `task add --help`, the TUI expanded task card, and the
README:
- `now`: actively blocking something or in flight today
- `soon`: committed this week, not today
- `later`: on the radar; revisit during weekly planning

---

### P3 — Cleanup and polish; no hard sequencing dependencies

- **Single binary**: remove `dooh-dev` build target, add `--home` flag, update install
  script to set up `DOOH_HOME`-based dev alias.
- **Scheduling intelligence**: `dooh today` CLI command, rollover markers in TUI and CLI
  output, estimate summation in today view, skip-weekends forwarding.
- **Theme redesign**: replace ad-hoc palette values with semantic tokens; add contrast
  validation; ship at least 2 light themes and 4 dark themes.
- **Batch operations**: accept repeated `--id` flags on `task complete`, `task archive`,
  `task assign add`.
- **Outbox**: add `dooh outbox status` command, implement a consumer, or remove the table
  and its accumulating undelivered rows.
- **README**: lead with the setup script; move the manual 6-step sequence to an appendix.
- **`whoami`/`context show`/`env` boundary**: make each command the authoritative source
  for exactly one thing; add a one-line description to each command's output or `--help`.
- **`dooh init` non-interactive mode**: `--name`, `--ai-key`, `--db` flags for
  script-driven setup.

---

### P4 — Nice to have; no timeline

- **`export site` bundled HTML viewer**: a single `index.html` that reads `tasks.json`
  and `collections.json` with no build step and no external dependencies.
- **Bubble Tea viewport + command palette**: full viewport-managed body rendering and a
  command-palette overlay (`:`) for discoverability. See TUI Implementation Notes.

---

## Interface Critique

An honest assessment of what is currently confusing, inelegant, or in tension with the
tool's own goals. Ordered by severity.

---

### The TUI is a bystander

The TUI currently watches the pair work and reports on it. This is fine for a dashboard
but the wrong orientation for a workspace. The TUI is not becoming writable now — that
is deliberately deferred — but the orientation principle matters even for read-only
design: every decision should ask "what would a human want to do from here?" not just
"what is useful to show?"

The most immediate improvements are making the pair dynamic visible (actor glyph, log
view) and making navigation feel like drilling into a hierarchy rather than switching
numbered flat views. Both are achievable without write interactions.

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
task card, and in the README. _(See Implementation Plan: P2)_

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
remove the kind from the schema. _(Handled in the P1 `description`/`urls` pass.)_

---

### The filter token syntax is invisible

The TUI's text filter (`f`) supports a powerful scoped token syntax: `#tag`, `~area`,
`^goal`, `@assignee`, `!overdue`, `!nodue`, `!todaydue`. This is the fastest way to
scope a task list — faster than the dropdown filters for tags and assignees. But nothing
in the TUI surface indicates it exists. The filter input shows a blank cursor. The footer
shows hotkeys for view modes but not for filter tokens.

At minimum: show a one-line placeholder in the filter input field (`#tag ~area @user …`),
and add a token syntax hint to the TUI help screen (or the expanded footer row).
_(See Implementation Plan: P2)_

---

### `rollover_enabled` and `skip_weekends` are schema ghosts

These fields exist on every task record. They represent real, useful scheduling behavior.
But there is no CLI flag to set them during `task add`, no TUI indicator that a task has
them enabled, and no visible effect. They are promises the interface has not kept.

Either implement the scheduling behavior these fields imply (see Design Shift 8), or
remove them from the schema. Invisible features are worse than missing features.
_(See Implementation Plan: P3)_

---

### The outbox has no consumer

The `outbox` table (status: pending/delivered/failed, with retry logic) is the
infrastructure for delivering events to external consumers. But no consumer exists. The
table accumulates rows that are never delivered and never cleaned up. Users who run
`sqlite3 dooh.db "select count(*) from outbox"` will find a growing list of pending
records with no explanation.

If the outbox is infrastructure for a future integration, document it clearly and add a
`dooh outbox status` command that shows pending/delivered/failed counts. If no integration
is planned for the foreseeable future, remove it. _(See Implementation Plan: P3)_

---

### Setup ceremony is hostile for newcomers

The manual first-time setup takes six steps, requires knowing ULID-format user IDs, and
involves running `sqlite3` directly. The setup scripts hide this, but the documentation
leads with the manual steps before the scripts. A newcomer following the README will hit
database concepts before they have created a single task.

The fix is `dooh init` (see Design Shift 6), but even before that, the README should
lead with the setup script, not the manual steps. Manual steps belong in an appendix.
_(See Implementation Plan: P2 / `dooh init`)_

---

### `dooh whoami` vs `dooh context show` vs `dooh env`

Three commands return overlapping identity and configuration information. Their
differences are not documented at the command level:

- `whoami`: who am I, what key am I using, what DB am I connected to
- `context show`: what context overrides are persisted locally
- `env`: what environment variables are currently resolved

Clarify by making each command the canonical source for exactly one thing. `whoami`
should be the identity oracle. `context show` should focus on user-set overrides. `env`
should focus on the raw environment resolution chain. _(See Implementation Plan: P3)_

---

### The `export site` output has no bundled viewer

`dooh export site` produces clean, well-structured JSON. But there is no example HTML,
no template, no reference viewer. Users who run the command get a directory of JSON files
and no indication of how to use them.

A minimal bundled viewer — a single `index.html` that reads `tasks.json` and
`collections.json` with no build step — would make the export feature immediately useful.
_(See Implementation Plan: P4)_

---

## Priority Index

| Priority | Item | Category |
|---|---|---|
| P0 | TUI stability: footer, width, alignment, flicker | Polish |
| P0 | `in-progress` task status + `task start` CLI command | Design shift | ✅ CLI done; TUI icon deferred |
| P1 | Checklist migration (drop `task_subtasks`, add `task_checklist`) | Design shift |
| P1 | `description` + `urls` fields on tasks; remove `groups` from TUI | Feature + cleanup |
| P1 | `dooh log`: colorized event stream viewer | Design shift |
| P1 | Actor glyph (H/A) in task rows | Design shift |
| P2 | Filter token syntax: placeholder + help hint in TUI | Discoverability |
| P2 | TUI sort controls: `o` key, sort chip, quoted filter tokens | UX improvement |
| P2 | Area view in TUI (`6` key) | Feature |
| P2 | Collection hierarchy navigation (breadcrumb + Left key) | Design shift |
| P2 | "Since you were away" TUI view | Design shift |
| P2 | `dooh init` interactive setup command | UX improvement |
| P2 | Priority semantics: define and document `now/soon/later` | Clarity |
| P3 | Single binary: remove `dooh-dev` target, add `--home` flag | Simplification |
| P3 | Scheduling intelligence: rollover, `dooh today` CLI, estimates | Design shift |
| P3 | Theme redesign: semantic tokens + contrast tests | Polish |
| P3 | Batch operations on status-change commands | Feature |
| P3 | Outbox: consumer, status command, or removal | Cleanup |
| P3 | README: lead with setup script, move manual steps to appendix | Clarity |
| P3 | `whoami`/`context show`/`env` boundary clarification | Clarity |
| P3 | `dooh init` non-interactive mode | Feature |
| P4 | `export site` bundled HTML viewer | Feature |
| P4 | Bubble Tea viewport + command palette | Polish |

---

## Deferred: TUI Write Features

The TUI will remain read-only until the core pair visibility and navigation work (P0–P2)
is complete. The write surface is a real future direction — the design below is preserved
for when it becomes a priority. Do not implement any of this now.

### Design proposal

**The problem:** The TUI enforces a workflow split — humans look here, then switch to the
CLI to act. This context-switch is small but persistent, and it makes the TUI feel like a
dashboard rather than a workspace.

**Minimum writable surface for a first pass:**
- `n` to quick-add a task (title + priority; opens a two-field inline form, not a modal)
- `x` or `Enter` (when on a task row) to complete
- `e` to open an inline editor for the selected task's title and priority
- `d` to archive
- Checklist items checkable directly from the expanded task card

This is not a full TUI editor — it is enough to close the loop without context-switching.
Description, URLs, and relationships remain CLI territory.

**Implementation note:** write interactions in a TUI with rich keybindings require careful
modal design. Follow the pattern established by the filter interactions (`f`, `g`, `a`,
etc.): a key opens an input mode, `Enter` commits, `Esc` cancels.

---

## TUI Implementation Notes

Key architectural decisions for anyone working on TUI items above.

### Rendering approach (applies to all TUI work)

- All line composition: build plain-text segments first, apply Lip Gloss styles last.
  Never measure the width of an already-styled string — use ANSI-aware helpers.
- Reserved layout regions: **header** (title + filter bar + tabs + column headers),
  **body** (viewport-managed rows), **footer** (selected summary + hotkey hints).
  Each region is allocated before rendering; body gets what's left.
- Re-render only on `WindowSizeMsg` or model mutations — never on a timer.

### Filtering and sorting (P2)

- Filter input parses a token AST: free-text fuzzy terms + typed tokens (`#tag`,
  `~area`, `^goal`, `@assignee`, `!overdue`, `!nodue`, `!todaydue`). Tokens combine
  with implicit AND. Quoted multi-word: `#"Deep Work"`.
- Parsed tokens render as chips in the top bar. Free text shows as-is.
- Sort mode (`o` key) cycles: default → priority → scheduled. Active sort shows as a
  chip. Sort changes order without breaking expand/selection state.

### Theme system (P3)

- Replace ad-hoc palette values with semantic tokens only: `text`, `muted`, `accent`,
  `success`, `warn`, `danger`, `chart1–4`.
- All shipped themes must pass minimum contrast ratio checks. Add a `theme lint`
  internal helper. No hardcoded 256-color fallbacks where semantic tokens exist.

### Bubble Tea upgrade path (P4)

- Current Bubble Tea use is mostly event loop + key handling. Bigger gains come from
  adopting Bubbles primitives for viewport and interactive controls (text inputs,
  list navigation). Do that only after P0 baseline is stable.
- Keep `--renderer legacy` as a fallback throughout. Feature-flag experimental UI
  behind `--ui-experimental` initially.
