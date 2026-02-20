#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BIN_DIR="${HOME}/.local/bin"
DOOH_HOME_DEV="${HOME}/.config/dooh-dev"
DOOH_DEV_DB="${HOME}/.local/share/dooh-dev/dooh-dev.db"

INSTALL_STABLE=0
INSTALL_DEV=0
STABLE_SRC_DIR="${ROOT_DIR}"
DEV_SRC_DIR="${ROOT_DIR}"

usage() {
  cat <<USAGE
Usage: $0 [options]

Safe default:
  - installs dev channel only (dooh-dev)
  - does not touch stable dooh binary unless requested

Options:
  --stable                install stable binary (dooh)
  --dev                   install dev channel (dooh-dev wrapper + dooh-dev-bin)
  --all                   install both stable and dev channels
  --stable-src <path>     source repo path for stable build (default: current repo)
  --dev-src <path>        source repo path for dev build (default: current repo)
  -h, --help              show help
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --stable)
      INSTALL_STABLE=1
      shift
      ;;
    --dev)
      INSTALL_DEV=1
      shift
      ;;
    --all)
      INSTALL_STABLE=1
      INSTALL_DEV=1
      shift
      ;;
    --stable-src)
      STABLE_SRC_DIR="$2"
      shift 2
      ;;
    --dev-src)
      DEV_SRC_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown arg: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

# Safe default: dev-only installation.
if [ "${INSTALL_STABLE}" -eq 0 ] && [ "${INSTALL_DEV}" -eq 0 ]; then
  INSTALL_DEV=1
fi

mkdir -p "${BIN_DIR}" "${HOME}/.local/share/dooh-dev" "${DOOH_HOME_DEV}"

if [ "${INSTALL_STABLE}" -eq 1 ]; then
  (
    cd "${STABLE_SRC_DIR}"
    go build -o "${BIN_DIR}/dooh" ./cmd/dooh
  )
  chmod +x "${BIN_DIR}/dooh"
  echo "installed stable: ${BIN_DIR}/dooh (from ${STABLE_SRC_DIR})"
  "${BIN_DIR}/dooh" version
fi

if [ "${INSTALL_DEV}" -eq 1 ]; then
  (
    cd "${DEV_SRC_DIR}"
    go build -o "${BIN_DIR}/dooh-dev-bin" ./cmd/dooh
  )
  cat > "${BIN_DIR}/dooh-dev" <<WRAP
#!/usr/bin/env bash
set -euo pipefail
SELF_DIR="\$(cd "\$(dirname "\$0")" && pwd)"
export DOOH_HOME="\${DOOH_HOME:-\$HOME/.config/dooh-dev}"
export DOOH_DB="\${DOOH_DB:-\$HOME/.local/share/dooh-dev/dooh-dev.db}"
export DOOH_PROFILE="\${DOOH_PROFILE:-dev}"
exec "\${SELF_DIR}/dooh-dev-bin" "\$@"
WRAP
  chmod +x "${BIN_DIR}/dooh-dev-bin" "${BIN_DIR}/dooh-dev"
  echo "installed dev: ${BIN_DIR}/dooh-dev (wrapper) + dooh-dev-bin (from ${DEV_SRC_DIR})"
  "${BIN_DIR}/dooh-dev" version
fi
