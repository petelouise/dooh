# Design: `in-progress` Task Status

_2026-02-21_

## What we're building

Add `in_progress` as a task status between `open` and `completed`. A new `task start
--id <id>` CLI command moves a task to this state, recording a `started_at` timestamp
and emitting a `task.started` event. This makes the pair's division of labor
machine-visible: the AI can signal it is actively working on a task, and the human
can see which tasks are in flight at a glance.

## Approach chosen

Extend the existing status model (Option A). Add `in_progress` to the status `CHECK`
constraint via a schema migration, add a `started_at` nullable timestamp column, and
wire up the new command using the existing `runTaskStatus` helper pattern. TUI icon
and float-to-top ordering are deferred until TUI stability work lands.

## Schema migration (`0002_in_progress_status.sql`)

SQLite does not support `ALTER TABLE ... MODIFY COLUMN` for CHECK constraints, so the
migration recreates the `tasks` table with the updated constraint and copies data.

New column: `started_at TIMESTAMP` (nullable; NULL until `task start` is called).
Updated constraint: `status IN ('open','in_progress','completed','archived')`.

## CLI changes (`internal/cli/task.go`)

- New `task start --id <id>` command via `runTaskStatus` with status `"in_progress"`,
  event type `"task.started"`, and an extra SET clause `started_at = datetime('now')`.
- Validation: only `open` tasks can be started (error if already `in_progress`,
  `completed`, or `archived`).
- `task list --status` gains `in_progress` as a valid filter value.
- `statusCell` and `statusIcon` updated to handle `"in_progress"` (icon: `◎`).
- Help text updated across `task list`, `task show`, and the new `task start`.

## Events

New event type `task.started`. Payload: `{task_id, short_id, status: "in_progress"}`.

## Export

`started_at` included in exported task fields. `in_progress` added to status filter
options in export.

## Out of scope (deferred)

- TUI `◎` icon in task rows
- Float-to-top ordering for in-progress tasks in the TUI
- Both depend on TUI stability work completing first (separate P0 item)
