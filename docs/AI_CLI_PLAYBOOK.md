# dooh AI CLI Playbook

This guide is for an AI coding agent operating `dooh` through CLI only.

## Auth model

Every mutating command requires API key auth. Actor identity is derived from the key,
never from a role string alone.

- **Agent mode** (`DOOH_MODE=agent`): requires `DOOH_AI_KEY` env var; rejects
  `--api-key` flag. This is the correct mode for AI CLI use.
- **Human mode** (`DOOH_MODE=human`): requires explicit `--api-key` flag; does not fall
  back to env vars.

## Scope model

| Scope | Agent | Human |
|---|---|---|
| `tasks:read` | yes | yes |
| `tasks:write` | yes | yes |
| `tasks:delete` | yes | yes |
| `collections:read` | yes | yes |
| `collections:write` | yes | yes |
| `export:run` | yes | yes |
| `users:admin` | no | yes |
| `keys:admin` | no | yes |
| `system:rollback` | no | yes |

Lifecycle admin (creating/revoking keys, creating users) is human-only by default.
Do not attempt these unless explicitly instructed with a system override.

## Exit codes

```text
0  success
1  general error
2  usage/validation error (bad flags, missing required flag)
3  auth failure
4  not found
5  permission denied
6  conflict/precondition failure (dependency cycle, open blockers, etc.)
```

## Machine-readable output

All data commands support `--json` as a global flag for structured output:
```bash
dooh --json task list
dooh --json task show --id t_XXXXXX
dooh --json whoami
dooh --json event list
```

Use `--quiet` (or `-q`) to suppress the context banner on write commands:
```bash
dooh --quiet task add --title "Water mint patch" --priority now
```

Combine both for clean programmatic use:
```bash
dooh --json --quiet task add --title "Water mint patch" --priority now
```

## Allowed vs disallowed

| action | allowed for AI by default | notes |
| --- | --- | --- |
| task create/update/show/status/delete | yes | use `task` subcommands |
| task assign/block/subtask/collection | yes | supported in CLI |
| collection create/list/show/link | yes | supported in CLI |
| query audit events | yes | `dooh event list` |
| user lookup (id + name) | yes | `dooh user lookup` |
| export site data | yes | `dooh export site --out ...` |
| create/revoke keys | no | human lifecycle admin by default |
| create users | no (default) | human lifecycle admin by default |
| delete users | no | command does not exist |

## Startup contract (every run)
1. Confirm identity:
```bash
dooh --json whoami
```
2. Discover user IDs (needed for assignment):
```bash
dooh --json user lookup
```
3. Read current state:
```bash
dooh --json task list
dooh --json collection list
```
4. Understand the collection landscape before creating tasks or projects:
```bash
dooh --json collection list
dooh --json collection show --id c_XXXXXX
```

## Environment contract
Required:
```bash
DOOH_AI_KEY=<AI_KEY>
```
Optional — override home dir for channel isolation:
```bash
DOOH_HOME=~/.config/dooh-dev
```

## Copy/paste runbook

### Create, read, update tasks
```bash
dooh --json task add --title "Water mint patch" --priority now
dooh --json task add --title "Count finches" --priority soon --due 2026-03-01 --description "Tally at feeder"
dooh --json task show --id t_XXXXXX
dooh --json task update --id t_XXXXXX --priority now --due 2026-03-15
dooh --json task update --id t_XXXXXX --due clear
dooh --json task complete --id t_XXXXXX
dooh --json task reopen --id t_XXXXXX
dooh --json task archive --id t_XXXXXX
dooh task delete --id t_XXXXXX
```

### Filter and sort tasks
```bash
dooh --json task list --status open --priority now
dooh --json task list --assignee <user_id> --sort priority --order asc
dooh --json task list --collection c_XXXXXX --limit 10
dooh --json task list --status all --sort scheduled
```

### Relationships and assignees
```bash
dooh task block --id t_XXXXXX --by t_YYYYYY
dooh task unblock --id t_XXXXXX --by t_YYYYYY
dooh task subtask add --parent t_XXXXXX --child t_YYYYYY   # pending: becomes task checklist (P1)
dooh task subtask remove --parent t_XXXXXX --child t_YYYYYY # pending: becomes task checklist (P1)
dooh task assign add --id t_XXXXXX --user <user_id>
dooh task assign remove --id t_XXXXXX --user <user_id>
```

> **Upcoming (P1):** `task subtask` commands will be replaced by `task checklist add/check/uncheck/remove`.
> Until then, use `task subtask` for step tracking. See `docs/PRIORITIES.md` and `docs/COLLECTION_MODEL.md`.

### Collection membership
```bash
dooh task collection add --id t_XXXXXX --collection c_YYYYYY
dooh task collection remove --id t_XXXXXX --collection c_YYYYYY
```

### Collections
```bash
dooh --json collection add --name "Pollinator Patrol" --kind project
dooh --json collection list
dooh --json collection show --id c_XXXXXX
dooh collection link --parent c_PARENT --child c_CHILD
dooh collection unlink --parent c_PARENT --child c_CHILD
```

### Audit trail
```bash
dooh --json event list
dooh --json event list --limit 50 --client-type agent_cli
dooh --json event list --type task.created --actor <user_id>
dooh --json event list --since 2026-02-20T00:00:00Z
```

### Verify and export
```bash
dooh --json task list
dooh export site --out ./site-data
```

## Safety reminders
- Always authenticated; anonymous mode is unsupported.
- AI should not run lifecycle admin commands unless explicitly instructed with system override.
- Re-read state after every mutation.
- Use `--json` for all reads to avoid brittle text parsing.
