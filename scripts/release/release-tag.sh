#!/usr/bin/env bash
set -euo pipefail

VERSION=""
PUSH=0

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --push) PUSH=1; shift ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

if [ -z "${VERSION}" ]; then
  echo "usage: $0 --version vX.Y.Z [--push]" >&2
  exit 1
fi

if git rev-parse "${VERSION}" >/dev/null 2>&1; then
  echo "tag already exists: ${VERSION}" >&2
  exit 1
fi

git tag -a "${VERSION}" -m "release ${VERSION}"

echo "created tag ${VERSION}"
echo "next: attach dist/${VERSION} artifacts to GitHub release"

if [ "${PUSH}" -eq 1 ]; then
  git push origin "${VERSION}"
  echo "pushed ${VERSION}"
fi
