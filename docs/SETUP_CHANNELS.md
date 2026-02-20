# Stable + Dev Channels

Use two channels side-by-side:
- `dooh` for real data.
- `dooh-dev` for experiments/demo data.

## Install
```bash
./scripts/install/install-local.sh
```

## Stable channel (real data)
```bash
./scripts/setup/setup-stable.sh
dooh whoami
dooh task list
```

Defaults:
- binary: `dooh`
- DOOH_HOME: `~/.config/dooh`
- db: `~/.local/share/dooh/dooh.db`

## Dev channel (demo data)
```bash
./scripts/setup/setup-dev.sh
dooh-dev whoami
dooh-dev task list
```

Defaults:
- binary: `dooh-dev`
- DOOH_HOME: `~/.config/dooh-dev`
- db: `~/.local/share/dooh-dev/dooh-dev.db`

## Safety rules
- Never run `setup demo` against stable db.
- Keep stable and dev db paths different.
- Keep stable and dev DOOH_HOME different.

## Quick checks
```bash
dooh context show
dooh-dev context show
```
These should show different profile/db/context paths.
