# dooh AI CLI Playbook

This guide is written for an AI coding agent that manages tasks through CLI only.

## Operating rules
- Always run as an authenticated user. Anonymous mode is unsupported.
- Prefer CLI over TUI.
- AI can manage tasks, task links, assignments, collections, exports.
- AI must not perform human lifecycle admin by default.
- There is no `user delete` command.
- User/key lifecycle actions are human-only unless explicitly using a system key with `--allow-system-admin`.

## One-time setup (human)
```bash
dooh setup demo --db ./dooh.db
dooh context set --profile human --db ./dooh.db --theme paper-fruit
dooh whoami
```

## AI runtime setup (shared shell + `.env`)
Put this in the AI runtime `.env`:
```bash
DOOH_AI_KEY=<AI_KEY>
```

Then run:
```bash
dooh whoami
dooh context show
```

Expected:
- actor resolves to `ai`,
- profile auto-forces to `ai` when `DOOH_AI_KEY` is present,
- writes are attributed to AI user/key in events.

## Core commands the AI should use often

### Task creation and listing
```bash
dooh task add --title "Water mint patch" --priority now
dooh task add --title "Count visiting finches" --priority soon
dooh task list
```

### Status changes
```bash
dooh task complete --id t_XXXXXX
dooh task reopen --id t_XXXXXX
dooh task archive --id t_XXXXXX
dooh task delete --id t_XXXXXX
```

### Assignments
```bash
dooh task assign add --id t_XXXXXX --user <user_id>
dooh task assign remove --id t_XXXXXX --user <user_id>
```

### Dependencies and subtasks
```bash
dooh task block --id t_XXXXXX --by t_YYYYYY
dooh task unblock --id t_XXXXXX --by t_YYYYYY
dooh task subtask add --parent t_XXXXXX --child t_YYYYYY
dooh task subtask remove --parent t_XXXXXX --child t_YYYYYY
```

### Collections
```bash
dooh collection add --name "Pollinator Patrol" --kind project
dooh collection add --name "Backyard Pond" --kind area
dooh collection add --name "Bees" --kind tag
dooh collection list
dooh collection link --parent c_PARENT --child c_CHILD
dooh collection unlink --parent c_PARENT --child c_CHILD
```

### Export for website/archive
```bash
dooh export site --out ./site-data
```

## Identity and audit checks

### Verify current actor
```bash
dooh whoami
```

### Verify event attribution
```bash
sqlite3 ./dooh.db "select seq,event_type,actor_user_id,key_id,client_type,occurred_at from events order by seq desc limit 20;"
```

## Non-goals for AI by default
- Creating/revoking user keys.
- Creating users.
- Any lifecycle admin requiring `users:admin` / `keys:admin`.

These are intentionally guarded to human actor unless a system override is explicitly and deliberately used.

## Safe loop for automation
1. `dooh whoami`
2. Read current task state (`dooh task list`, `dooh collection list`)
3. Apply changes
4. Re-read task state
5. Export if needed (`dooh export site --out ./site-data`)
6. Optionally check last events for attribution

