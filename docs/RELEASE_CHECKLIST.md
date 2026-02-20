# Release Readiness Checklist

Use this checklist before cutting each release.

## 1) Code health
- `go test ./...` passes.
- TUI smoke check completed (`dooh tui`, `dooh tui --plain`, `dooh tui --renderer legacy`).
- CLI smoke check completed with both human and ai keys.

## 2) Versioning and changelog
- Bump version in `internal/cli/cli.go` (`dooh version` output).
- Add release notes entry in `CHANGELOG.md` (if present).
- Tag release with semantic version (`vX.Y.Z`).

## 3) Build artifacts
- Build binaries for target platforms.
- Produce checksums for artifacts.
- Attach binaries + checksums to release.

## 4) Install path clarity
- Stable binary name: `dooh`.
- Dev binary name: `dooh-dev` (or separate install path).
- Stable and dev config should not collide:
  - stable: `~/.config/dooh`
  - dev: `~/.config/dooh-dev` (or `DOOH_CONFIG_HOME` override)

## 5) Data safety
- Verify rollback path is documented and tested.
- Keep release-to-release migration notes.
- Recommend snapshot before upgrade:
  - `cp dooh.db dooh.db.bak.<date>`

## 6) Auth and audit safety
- Confirm no anonymous runtime access.
- Confirm ai profile auto-enforcement with `DOOH_AI_KEY`.
- Confirm lifecycle admin remains human-only by default.
- Confirm there is still no `user delete` command.
- Verify events include actor attribution.

## 7) Docs and onboarding
- README quick start validated copy/paste.
- AI guide updated (`docs/AI_CLI_PLAYBOOK.md`).
- Example commands match current flags and behavior.

