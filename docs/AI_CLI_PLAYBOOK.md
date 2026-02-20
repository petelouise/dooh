# dooh AI CLI Playbook

This guide is for an AI coding agent operating `dooh` through CLI only.

## Allowed vs disallowed

| action | allowed for AI by default | notes |
| --- | --- | --- |
| task create/update/status/delete | yes | use `task` subcommands |
| task assign/block/subtask | yes | supported in CLI |
| collection create/list/link | yes | supported in CLI |
| export site data | yes | `dooh export site --out ...` |
| create/revoke keys | no | human lifecycle admin by default |
| create users | no (default) | human lifecycle admin by default |
| delete users | no | command does not exist |

## Startup contract (every run)
1. Confirm identity:
```bash
dooh whoami
```
2. Read current state:
```bash
dooh task list
dooh collection list
```

## Environment contract
Required:
```bash
DOOH_AI_KEY=<AI_KEY>
```
Optional for dev channel only:
```bash
DOOH_HOME=~/.config/dooh-dev
```

## Copy/paste runbook

### Create and update tasks
```bash
dooh task add --title "Water mint patch" --priority now
dooh task add --title "Count visiting finches" --priority soon
dooh task complete --id t_XXXXXX
dooh task reopen --id t_XXXXXX
dooh task archive --id t_XXXXXX
dooh task delete --id t_XXXXXX
```

### Relationships and assignees
```bash
dooh task block --id t_XXXXXX --by t_YYYYYY
dooh task unblock --id t_XXXXXX --by t_YYYYYY
dooh task subtask add --parent t_XXXXXX --child t_YYYYYY
dooh task subtask remove --parent t_XXXXXX --child t_YYYYYY
dooh task assign add --id t_XXXXXX --user <user_id>
dooh task assign remove --id t_XXXXXX --user <user_id>
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

### Verify and export
```bash
dooh task list
dooh export site --out ./site-data
sqlite3 ./dooh.db "select seq,event_type,actor_user_id,key_id,client_type,occurred_at from events order by seq desc limit 20;"
```

## Safety reminders
- Always authenticated; anonymous mode is unsupported.
- AI should not run lifecycle admin commands unless explicitly instructed with system override.
- Re-read state after every mutation.
