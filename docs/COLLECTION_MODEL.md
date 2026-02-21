# Collection Hierarchy Model

_2026-02-21 — design decision record_

This document records the collection taxonomy design: what kinds exist, what can contain
what, and why. It is intended to be authoritative for schema changes, application-layer
validation, TUI navigation design, and AI agent instructions.

---

## The vocabulary

Four collection kinds. No others.

| Kind | What it represents | Typical examples |
|---|---|---|
| `area` | A life domain — where and when you operate | home, work, school, health |
| `goal` | An outcome you're working toward — why | "launch Q2 product", "run a marathon" |
| `project` | A coherent body of work with a completion state — what | "redesign checkout", "build greenhouse" |
| `tag` | A lightweight label for grouping and filtering | "billing", "waiting", "someday" |

The current schema also includes `class` and `custom` kinds. Both are undefined catch-alls
with no specified semantics or navigation behavior. They should be removed from the
`kind` CHECK constraint and from all documentation. Any existing collections of those
kinds should be migrated to `tag` or reclassified manually before the constraint is
tightened.

---

## Containment rules

```text
goal   ←— can be top-level, or nested under another goal
  └── goal      (optional nesting: year > quarter > sprint)
  └── project
  └── task      (directly, without a project intermediary)

area   ←— top-level only; cannot belong to any other collection
  └── project
  └── task      (directly, without a project intermediary)

project  ←— must have at least one parent (area, goal, or both)
  └── task

task   ←— can belong to any of: project, goal, area
  └── checklist item   (ordered text + checkbox; not a full task)

tag    ←— can be applied to any collection kind, or directly to a task
           cascades down: tagging a project implicitly tags its tasks
           tagging a goal implicitly tags its projects and their tasks
```

In plain language:

- Areas are the ceiling. Nothing contains an area.
- Goals are cross-cutting. They sit alongside areas as a second top-level organizing
  axis. They can span multiple life domains and are not required to belong to any area.
- Goals can nest. A year-level goal can contain quarter-level goals or projects directly.
  Nesting depth is unconstrained but more than two levels deep is likely a smell.
- Projects bridge the two axes. A project can have a goal parent (expressing purpose),
  an area parent (expressing context), or both. Having both is the normal case for
  substantive work: "Product redesign" belongs to goal "reduce churn" and area "work."
- Direct task membership is allowed at every level. A task can live directly in an area,
  a goal, or a project without needing intermediate containers. "Pick up dry cleaning"
  belongs to area "home" without needing a project.
- Checklist items are lightweight steps within a task — plain text plus a checked state.
  They are not tasks. They have no priority, due date, assignee, or collection membership.
  If a step within a task needs any of those things, it should be a task in a project,
  not a checklist item.
- Tags cascade. A tag applied to a goal applies implicitly to all its projects and all
  their tasks. A tag applied to a project applies to all its tasks. Tags on tasks are
  local only. Inherited tags are displayed distinctly (e.g. `billing (via Q1 Project)`)
  rather than appearing as directly applied.

---

## Why these rules, not others

Three models were considered:

**Model A — Goals inside areas (goals and projects as peers under areas)**

The relationship "this project implements this goal" would have been expressed as a
special link type rather than containment. The problem: the most important structural
relationship in the system becomes a second-class citizen. Goals would also be bounded
by area, which is false — "get healthy" is not a home goal or a work goal.

Rejected because it forces a link where containment is more natural and correct.

**Model B — Goals outside areas, can contain projects (chosen)**

Goals and areas are both top-level but serve different organizational purposes. A project
can be inside both — appearing in area navigation by context and in goal navigation by
purpose. The goal-project relationship is containment, not a link type.

The cost is topological complexity: a project has two parents of different kinds. The
`collection_closure` table already supports this; the application layer needs to enforce
kind-specific constraints rather than relying on generic graph rules.

Chosen because it maps accurately to how work actually distributes across a life, and
because it lets the AI answer "what goal does this task serve?" by walking up through
containment rather than querying a separate link table.

**Model C — Goals as the primary spine, areas as contextual labels (not containers)**

Everything organizes under goals. Areas become strong tags rather than containers.
Clean hierarchy, one top-level kind.

Rejected because it makes context-based navigation ("I'm sitting down at my desk, show
me work tasks") into a filter operation rather than a navigation gesture. Areas-as-
containers give you that affordance without cost.

---

## Constraints to enforce

These rules need to be enforced at the application layer (not just in the TUI or docs):

| Constraint | Where to enforce |
|---|---|
| `area` may not have any parent collection | `collection link` command; schema trigger |
| `goal` parent, if any, must be another `goal` | `collection link` command validation |
| `project` must have ≥1 parent of kind `area` or `goal` | enforced on `collection link` when first parent is added; `project` creation should prompt or require `--parent` |
| `tag` may have any parent or none | no constraint |
| `class` and `custom` kinds: reject on create | `collection add` command validation |

The `collection_links` table currently has no kind-aware constraints. All of the above
must be enforced in the CLI command layer and validated in tests. A database trigger
would be belt-and-suspenders but is optional.

---

## Schema changes required

```sql
-- 1. Tighten the kind constraint (after migrating existing class/custom rows)
ALTER TABLE collections RENAME TO collections_old;
CREATE TABLE collections (
  ...
  kind TEXT NOT NULL CHECK(kind IN ('area','goal','project','tag')),
  ...
);
-- migrate rows, drop old table

-- 2. No structural schema changes needed for the collection hierarchy itself.
--    collection_links and collection_closure already support multiple parents.
--    The rules are enforced at the application layer.

-- 3. Replace task_subtasks with task_checklist (see section below)
```

---

## Task checklist model

Subtasks in the current schema are full tasks linked via a `task_subtasks` join table.
This creates a philosophical incoherence: a subtask with its own priority, due date,
assignees, and collection membership is not meaningfully different from a task. The
distinction collapses. The data model has tasks all the way down with no clear boundary.

**The decision:** subtasks are replaced by lightweight checklist items — ordered text
plus a checked state, nothing more. If a step within a task needs its own scheduling,
assignment, or organizational context, it is a task in a project, not a checklist item.
The hierarchy handles this; the checklist does not need to.

This also clarifies the four-level structure of the whole system:

```text
area / goal
  └── project   (a coordinated body of work)
        └── task   (a unit of work owned by one actor)
              └── checklist item   (a step; just text + ☐/☑)
```

Each level is meaningfully distinct. Nothing bleeds upward.

**Schema migration:**

```sql
-- Drop the task-to-task relationship table
DROP TABLE task_subtasks;

-- Add the checklist table
CREATE TABLE task_checklist (
  id          TEXT PRIMARY KEY,
  task_id     TEXT NOT NULL REFERENCES tasks(id),
  text        TEXT NOT NULL,
  checked     INTEGER NOT NULL DEFAULT 0 CHECK(checked IN (0,1)),
  position    INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX idx_task_checklist_task ON task_checklist(task_id, position);
```

**Auto-complete behavior:** when all checklist items on a task are checked, the task
may auto-complete (matching the existing behavior for the subtask model). This is
simpler to query — count unchecked rows rather than querying child task statuses.

**CLI changes:**
- Remove: `task subtask add`, `task subtask remove`
- Add: `task checklist add --id <task> --text "..."`
- Add: `task checklist check --id <task> --item <item_id>`
- Add: `task checklist uncheck --id <task> --item <item_id>`
- Add: `task checklist remove --id <task> --item <item_id>`
- `task show` should include checklist items in its output

**TUI changes:** expanded task card shows checklist items as `☐ step text` / `☑ step
text` lines after the description. Checking/unchecking from the TUI is a natural write
operation to add alongside quick-add and complete.

**Blocking relationships** (`task_dependencies`) remain at the task level. Checklist
items do not block each other — they are steps, not dependencies.

---

## Navigation implications for the TUI

The TUI currently has five flat views (1–5: tasks, projects, goals, today, assignees).
The collection model above calls for a different navigation model:

- **Two root axes**: areas and goals. Both accessible from the top level.
- **Entering an area** scopes to that area's projects and tasks.
- **Entering a goal** scopes to that goal's sub-goals, projects, and tasks.
- **Entering a project** scopes to that project's tasks.
- **Breadcrumb in the filter bar** shows the path taken: `Work > Product redesign` or
  `Reduce churn > Product redesign`. Both are valid paths to the same project.
- **`Left` key** goes up one level in the breadcrumb.
- **Numbered view modes** (1–5) apply _within_ the current scope, not globally.

The TUI scope state (`ScopeKind`, `ScopeID`, `ScopeName`) already exists. It needs to
be extended with a `ScopePath` (the sequence of IDs from root to current node) to
support breadcrumb rendering and `Left` navigation.

Tag filtering remains a filter chip, not a navigation axis. Tags are orthogonal to the
hierarchy.

---

## Implications for the AI agent

The AI should use the hierarchy to reason about task placement:

- When creating a task, prefer assigning it to a project over a bare area or goal —
  projects are the natural home for actionable work.
- When creating a project, always assign it at least one parent (area, goal, or both).
  A parentless project is an orphan that won't appear in any scoped view.
- "This task serves goal X" is expressed by the task's project being inside goal X, not
  by any tag or metadata field.
- To find all tasks serving a goal, query the closure: descendants of the goal that are
  tasks, either directly or through projects.
- Tag cascading is implicit: if the agent tags a project `waiting`, all its tasks are
  also effectively tagged `waiting` for filter purposes. The agent does not need to
  re-tag individual tasks.

The startup contract in `AI_CLI_PLAYBOOK.md` should be updated to include:
```bash
dooh --json collection list           # learn the collection landscape
dooh --json collection show --id <id> # drill into a specific collection
```

---

## Resolved design decisions

**Can a project belong to multiple goals?** Yes, with no restriction or warnings. A
project that genuinely serves two distinct outcomes should express that. The system does
not try to enforce that projects have a single purpose; the pair is responsible for
noticing when a project is overloaded. Show all goal parents clearly when displaying a
project so the dual membership is visible.

**Can goals belong to areas?** No. Goals are cross-cutting by definition — "get healthy"
is not a home goal or a work goal. If a user wants to categorize goals by domain (work
goals vs. personal goals), use goal nesting: create a permanent top-level goal "Work"
or "Personal" and nest domain-specific goals under it. Do not allow areas to contain
goals or goals to be tagged with areas as a containment shortcut — that creates two
channels between the axes and lets them contradict each other.

**Should tag cascading be explicit or implicit?** Compute implicitly at query time via
the closure table; display inherited tags distinctly in the UI. Do not materialize
cascade as additional tag records. "Billing (via Q1 Project)" in the expanded task card
is more honest than showing `billing` as if it were directly applied, and requires no
extra storage or event noise. Revisit only if tag query performance becomes a real issue
at scale (unlikely for a single-user SQLite tool).
