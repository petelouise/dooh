#!/usr/bin/env bash
set -euo pipefail

ALLOW_DIRTY=0
VERSION=""

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --allow-dirty) ALLOW_DIRTY=1; shift ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

if [ -z "${VERSION}" ]; then
  echo "usage: $0 --version vX.Y.Z [--allow-dirty]" >&2
  exit 1
fi

if [ "${ALLOW_DIRTY}" -ne 1 ]; then
  if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "git worktree is dirty; commit or use --allow-dirty" >&2
    exit 1
  fi
fi

if git rev-parse "${VERSION}" >/dev/null 2>&1; then
  echo "tag already exists: ${VERSION}" >&2
  exit 1
fi

if ! grep -q "^## ${VERSION} " CHANGELOG.md; then
  echo "missing changelog entry for ${VERSION}" >&2
  exit 1
fi

if command -v dooh >/dev/null 2>&1; then
  dooh version
else
  echo "warning: dooh not found on PATH, skipping installed binary version check"
fi

GOCACHE="${PWD}/.cache/go-build" go test ./...

echo "preflight passed for ${VERSION}"
