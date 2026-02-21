# Release Readiness Checklist

Use this checklist before cutting each release.

## 1) Code health
- `GOCACHE=$(pwd)/.cache/go-build go test ./...` passes.
- TUI smoke check completed (`dooh tui`, `dooh tui --plain`, `dooh tui --renderer legacy`).
- CLI smoke check completed with both human and ai keys.

## 2) Versioning and changelog
- Bump version in `internal/cli/cli.go` (`dooh version` output).
- Add release notes entry in `CHANGELOG.md`.
- Tag release with semantic version (`vX.Y.Z`).

## 3) Use release scripts
- Preflight:
```bash
./scripts/release/release-preflight.sh --version vX.Y.Z
```
- Build artifacts:
```bash
./scripts/release/release-build.sh --version vX.Y.Z
```
- Create tag:
```bash
./scripts/release/release-tag.sh --version vX.Y.Z
```

## 4) Build artifacts
- Confirm binaries exist under `dist/vX.Y.Z/`.
- Confirm `SHA256SUMS` exists.
- Attach binaries + checksums to release.

## 5) Install path clarity
- Single binary: `dooh`.
- Channel isolation via `DOOH_HOME` (default: `~/.config/dooh`).
- Dev channel: `DOOH_HOME=~/.config/dooh-dev dooh ...` or shell alias.
- Stable and dev config/db paths must not collide — verify with `dooh context show`.

## 6) Data safety
- Stable channel must not use demo seed data.
- Dev channel may use demo seed data only in isolated db.
- Snapshot before upgrade:
  - `cp ~/.local/share/dooh/dooh.db ~/.local/share/dooh/dooh.db.bak.$(date +%Y%m%d-%H%M%S)`

## 7) Auth and audit safety
- Confirm no anonymous runtime access.
- Confirm ai profile auto-enforcement with `DOOH_AI_KEY`.
- Confirm lifecycle admin remains human-only by default.
- Confirm there is still no `user delete` command.
- Verify events include actor attribution.

## 8) Docs and onboarding
- README quickstart validated copy-paste.
- AI guide updated (`docs/AI_CLI_PLAYBOOK.md`).
- Priorities doc current (`docs/PRIORITIES.md`).
