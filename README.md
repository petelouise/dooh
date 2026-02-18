# dooh

`dooh` (pronounced "duo") is a local-first task/project/goal manager for a human + AI agent pair.

## Current status
Initial scaffold includes:
- offline-friendly Go CLI command layout,
- first-pass SQLite schema migration,
- colorful TUI theme preset catalog,
- product spec and operating rules.

## Build
```bash
go build ./cmd/dooh
```

## Examples
```bash
./dooh version
./dooh tui --list-themes
./dooh tui --theme mint-circuit
./dooh task add --title "Ship migration" --priority now
```

## Notes
- Network was unavailable during bootstrap, so the CLI currently uses stdlib parsing only.
- See `docs/SPEC.md` for full architecture and behavior decisions.
