#!/usr/bin/env bash
set -euo pipefail

VERSION=""
while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

if [ -z "${VERSION}" ]; then
  echo "usage: $0 --version vX.Y.Z" >&2
  exit 1
fi

OUT_DIR="dist/${VERSION}"
mkdir -p "${OUT_DIR}"

TARGETS=(
  "darwin amd64"
  "darwin arm64"
  "linux amd64"
  "linux arm64"
)

for target in "${TARGETS[@]}"; do
  GOOS="${target%% *}"
  GOARCH="${target##* }"
  BIN="dooh_${VERSION}_${GOOS}_${GOARCH}"
  GOOS="${GOOS}" GOARCH="${GOARCH}" CGO_ENABLED=0 go build -o "${OUT_DIR}/${BIN}" ./cmd/dooh
  chmod +x "${OUT_DIR}/${BIN}"
  echo "built ${OUT_DIR}/${BIN}"
done

(
  cd "${OUT_DIR}"
  shasum -a 256 dooh_* > SHA256SUMS
)

echo "release artifacts written to ${OUT_DIR}"
