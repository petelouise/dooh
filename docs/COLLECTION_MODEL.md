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

```
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
  └── subtask   (task-to-task relationship; not a collection)

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
- Tags cascade. A tag applied to a goal applies implicitly to all its projects and all
  their tasks. A tag applied to a project applies to all its tasks. Tags on tasks are
  local only.

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

-- 2. No structural schema changes needed for the hierarchy itself.
--    collection_links and collection_closure already support multiple parents.
--    The rules are enforced at the application layer.
```

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
- **Numbered view modes** (1–5) apply *within* the current scope, not globally.

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

## Open questions (not yet resolved)

**Can a project belong to multiple goals?** The model allows it (multiple parents of
kind `goal`). Useful if a single project serves two distinct outcomes. Probably fine to
allow; validation should warn if a project has more than two or three goal parents, as
that often signals the project is too broad.

**Can goals belong to areas?** The model says no — goals are cross-cutting. But some
teams/people want to say "my work goals" vs. "my personal goals." A compromise: allow
tagging goals with areas (`tag` semantics) without making it a containment relationship.
This keeps the hierarchy clean while allowing the label.

**Should `tag` cascading be explicit or implicit?** Current proposal: implicit (the
query layer computes it). Alternative: store tag inheritance explicitly in a separate
table (like `collection_closure`). Implicit is simpler to implement; explicit is faster
to query and easier to audit. Defer until tag query performance becomes an issue.
