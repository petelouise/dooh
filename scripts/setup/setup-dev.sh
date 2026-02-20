#!/usr/bin/env bash
set -euo pipefail

DOOH_DEV_BIN="${DOOH_DEV_BIN:-dooh-dev}"
DEV_HOME="${DOOH_HOME:-${HOME}/.config/dooh-dev}"
DEV_DB="${DOOH_DB:-${HOME}/.local/share/dooh-dev/dooh-dev.db}"
STABLE_DB_DEFAULT="${HOME}/.local/share/dooh/dooh.db"

if [ "${DEV_DB}" = "${STABLE_DB_DEFAULT}" ]; then
  echo "refusing to run: dev db matches stable db path (${DEV_DB})" >&2
  exit 1
fi

mkdir -p "$(dirname "${DEV_DB}")" "${DEV_HOME}"

export DOOH_HOME="${DEV_HOME}"
export DOOH_DB="${DEV_DB}"
export DOOH_PROFILE="dev"

"${DOOH_DEV_BIN}" setup demo --db "${DEV_DB}"
"${DOOH_DEV_BIN}" context set --profile dev --db "${DEV_DB}" --theme sunset-pop

cat <<OUT
dev setup complete (dev only data)
- dooh_home: ${DOOH_HOME}
- db: ${DOOH_DB}
- profile: ${DOOH_PROFILE}

next:
  ${DOOH_DEV_BIN} whoami
  ${DOOH_DEV_BIN} task list
OUT
