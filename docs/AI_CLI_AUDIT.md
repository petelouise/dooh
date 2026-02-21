# AI CLI Interface Audit

Original audit date: 2026-02-21
Resolution date: 2026-02-21 (implemented in v0.3.0, help text improved in v0.3.1)
Scope: CLI interface usability from an AI coding agent's perspective

> **Status: All P0-P2 items resolved.** This document records the original audit
> findings and their resolution status. For current usage see `docs/AI_CLI_PLAYBOOK.md`.

---

## Summary of Original Audit vs Current State

| Priority | Issue | Status |
|---|---|---|
| P0 | Add `--json` output mode to all commands | **Resolved** |
| P0 | Add `task show --id` | **Resolved** |
| P0 | Add `task update` command | **Resolved** |
| P1 | Add `task list` filtering (`--status`, `--priority`, `--assignee`, `--limit`) | **Resolved** |
| P1 | Add `event list` CLI command | **Resolved** |
| P1 | Add task-to-collection membership commands | **Resolved** |
| P2 | Add `collection show` | **Resolved** |
| P2 | Suppress context banner with `--quiet` | **Resolved** |
| P2 | Improve help text and per-command `--help` | **Resolved** |
| P3 | Distinct exit codes by error category | **Resolved** |
| P3 | Batch operations | Not implemented (low priority) |
| P3 | Lower-privilege user lookup | **Resolved** (`dooh user lookup`) |

---

## Current Command Inventory

### All commands available:
```
dooh version
dooh config show|init
dooh db init
dooh setup demo
dooh demo seed
dooh login
dooh env
dooh context show|set|clear
dooh whoami
dooh user create|list|lookup
dooh key create|revoke
dooh task add|list|show|update|complete|reopen|archive|delete
dooh task block|unblock
dooh task subtask add|remove
dooh task assign add|remove
dooh task collection add|remove
dooh collection add|list|show|link|unlink
dooh event list
dooh export site
dooh tui
```

### Global flags (apply to all commands):
```
--profile <name>   select config profile (default, human, ai)
--config <path>    override config file path
--json             output machine-readable JSON
--quiet, -q        suppress context banner on write commands
```

### Exit codes:
```
0  success
1  general error
2  usage/validation error (bad flags, missing required flag)
3  auth failure
4  not found
5  permission denied
6  conflict/precondition failure (e.g. dependency cycle, open blockers)
```

---

## Current Capabilities

### Machine-readable output
All data commands support `--json` for structured output. The context banner is
suppressed automatically when `--json` is active:
```bash
dooh --json task list
dooh --json task show --id t_abc123
dooh --json whoami
dooh --json event list
```

### Per-command help
Every subcommand responds to `--help` with flag descriptions and examples:
```bash
dooh task --help
dooh task add --help
dooh task list --help
dooh task show --help
dooh task update --help
dooh collection --help
dooh collection add --help
dooh event list --help
dooh user lookup --help
```

### Task list filters
```bash
dooh --json task list --status open --priority now
dooh --json task list --assignee <user_id>
dooh --json task list --collection c_XXXXXX --limit 10 --sort priority --order asc
dooh --json task list --status all --sort scheduled
```

### Task create/read/update/delete
```bash
dooh --json task add --title "Water mint patch" --priority now
dooh --json task add --title "Count finches" --priority soon --due 2026-03-01
dooh --json task show --id t_XXXXXX
dooh --json task update --id t_XXXXXX --priority now --due 2026-03-15
dooh --json task update --id t_XXXXXX --due clear
dooh --json task complete --id t_XXXXXX
dooh --json task reopen --id t_XXXXXX
dooh --json task archive --id t_XXXXXX
dooh task delete --id t_XXXXXX
```

### Relationships
```bash
dooh task block --id t_XXXXXX --by t_YYYYYY
dooh task unblock --id t_XXXXXX --by t_YYYYYY
dooh task subtask add --parent t_XXXXXX --child t_YYYYYY
dooh task assign add --id t_XXXXXX --user <user_id>
dooh task collection add --id t_XXXXXX --collection c_YYYYYY
```

### Collections
```bash
dooh --json collection add --name "Pollinator Patrol" --kind project
dooh --json collection list
dooh --json collection show --id c_XXXXXX
dooh collection link --parent c_PARENT --child c_CHILD
```

### Event audit trail
```bash
dooh --json event list
dooh --json event list --limit 50 --client-type agent_cli
dooh --json event list --type task.created --actor <user_id>
dooh --json event list --since 2026-02-01T00:00:00Z
```

### User lookup (low-privilege, available to AI agents)
```bash
dooh --json user lookup
```

---

## Remaining Gap: Batch Operations (P3)

There is no way to complete/update/assign multiple tasks in a single command.
Each mutation requires a separate invocation. This is acceptable for current
use but could be a productivity improvement for bulk project operations.

---

## Attribution/Audit Trail Assessment

**Confirmed working well:**
- Every mutation writes to `events` with `actor_user_id`, `key_id`, and `client_type`
- `client_type` is `human_cli` or `agent_cli` derived from API key — not user-settable
- `dooh --json event list` provides full CLI access to audit trail
- `created_by` and `updated_by` on tasks/collections track the last actor
- Tests verify attribution correctness (`TestMutationsWriteEventAttributionForHumanAndAI`)
- `dooh --json whoami` lets agent confirm its own identity programmatically
