# Changelog

All notable changes to `dooh` are tracked in this file.

## v0.3.1 - 2026-02-20
- Added `DOOH_HOME` support for config/auth/context path isolation.
- Added stable/dev operational scripts:
  - `scripts/install/install-local.sh`
  - `scripts/setup/setup-stable.sh`
  - `scripts/setup/setup-dev.sh`
  - `scripts/release/release-preflight.sh`
  - `scripts/release/release-build.sh`
  - `scripts/release/release-tag.sh`
- Expanded AI/operator onboarding docs with stable-vs-dev channel guidance.

## v0.3.0 - 2026-02-20
- Bubble Tea-first TUI path with legacy fallback support.
- Filter/sort UX stabilization:
  - `Tab` focus model,
  - `Enter` to edit focused filter,
  - sort direction toggle (`Shift+O`),
  - unified chip style and token hydration.
- Mandatory authenticated identity for runtime commands.
- Pair-mode auth/profile simplification for shared human+ai shell.
- Improved TUI usability:
  - stronger focus hints,
  - filter guidance,
  - sorting and dropdown consistency fixes.
