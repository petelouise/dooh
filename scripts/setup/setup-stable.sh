#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DOOH_BIN="${DOOH_BIN:-dooh}"
DB_PATH="${DOOH_DB:-${HOME}/.local/share/dooh/dooh.db}"
PROFILE_HUMAN="human"
PROFILE_AI="ai"
THEME="paper-fruit"
HUMAN_NAME="Human Operator"
AI_NAME="AI Partner"

mkdir -p "$(dirname "${DB_PATH}")"

if [ -f "${DB_PATH}" ]; then
  demo_users=$(sqlite3 "${DB_PATH}" "SELECT COUNT(*) FROM users WHERE name IN ('Human Demo','Agent Demo');" 2>/dev/null || echo "0")
  demo_cols=$(sqlite3 "${DB_PATH}" "SELECT COUNT(*) FROM collections WHERE name IN ('Moon Garden','Pollinator Patio','Forest Sketchbook');" 2>/dev/null || echo "0")
  if [ "${demo_users}" != "0" ] || [ "${demo_cols}" != "0" ]; then
    echo "refusing to run: target db appears to contain demo seed data (${DB_PATH})" >&2
    exit 1
  fi
fi

"${DOOH_BIN}" db init --db "${DB_PATH}"

if ! sqlite3 "${DB_PATH}" "SELECT id FROM users WHERE name='${HUMAN_NAME}' LIMIT 1;" | grep -q .; then
  "${DOOH_BIN}" user create --db "${DB_PATH}" --name "${HUMAN_NAME}" --bootstrap
fi
HUMAN_ID=$(sqlite3 "${DB_PATH}" "SELECT id FROM users WHERE name='${HUMAN_NAME}' LIMIT 1;")

if ! sqlite3 "${DB_PATH}" "SELECT id FROM users WHERE name='${AI_NAME}' LIMIT 1;" | grep -q .; then
  "${DOOH_BIN}" user create --db "${DB_PATH}" --name "${AI_NAME}" --bootstrap
fi
AI_ID=$(sqlite3 "${DB_PATH}" "SELECT id FROM users WHERE name='${AI_NAME}' LIMIT 1;")

HUMAN_SCOPES="tasks:read,tasks:write,tasks:delete,collections:read,collections:write,export:run,users:admin,keys:admin,system:rollback"
AI_SCOPES="tasks:read,tasks:write,tasks:delete,collections:read,collections:write,export:run"

if ! sqlite3 "${DB_PATH}" "SELECT id FROM api_keys WHERE user_id='${HUMAN_ID}' AND client_type='human_cli' AND revoked_at IS NULL LIMIT 1;" | grep -q .; then
  HUMAN_KEY_OUT=$("${DOOH_BIN}" key create --db "${DB_PATH}" --user "${HUMAN_ID}" --client-type human_cli --scopes "${HUMAN_SCOPES}" --bootstrap)
  HUMAN_KEY=$(printf "%s\n" "${HUMAN_KEY_OUT}" | sed -n 's/^api_key=//p' | head -n1)
  HUMAN_PREFIX=$(printf "%s\n" "${HUMAN_KEY_OUT}" | sed -n 's/^created key \([^ ]*\).*/\1/p' | head -n1)
  "${DOOH_BIN}" --profile "${PROFILE_HUMAN}" login --db "${DB_PATH}" --api-key "${HUMAN_KEY}"
else
  HUMAN_PREFIX=$(sqlite3 "${DB_PATH}" "SELECT key_prefix FROM api_keys WHERE user_id='${HUMAN_ID}' AND client_type='human_cli' AND revoked_at IS NULL LIMIT 1;")
  HUMAN_KEY=""
fi

if ! sqlite3 "${DB_PATH}" "SELECT id FROM api_keys WHERE user_id='${AI_ID}' AND client_type='agent_cli' AND revoked_at IS NULL LIMIT 1;" | grep -q .; then
  ADMIN_KEY="${HUMAN_KEY:-${HUMAN_API_KEY:-}}"
  if [ -z "${ADMIN_KEY}" ]; then
    echo "missing admin key for AI key creation: set HUMAN_API_KEY for reruns where ai key does not yet exist" >&2
    exit 1
  fi
  AI_KEY_OUT=$("${DOOH_BIN}" key create --db "${DB_PATH}" --api-key "${ADMIN_KEY}" --user "${AI_ID}" --client-type agent_cli --scopes "${AI_SCOPES}")
  AI_KEY=$(printf "%s\n" "${AI_KEY_OUT}" | sed -n 's/^api_key=//p' | head -n1)
  AI_PREFIX=$(printf "%s\n" "${AI_KEY_OUT}" | sed -n 's/^created key \([^ ]*\).*/\1/p' | head -n1)
else
  AI_PREFIX=$(sqlite3 "${DB_PATH}" "SELECT key_prefix FROM api_keys WHERE user_id='${AI_ID}' AND client_type='agent_cli' AND revoked_at IS NULL LIMIT 1;")
  AI_KEY=""
fi

"${DOOH_BIN}" context set --profile "${PROFILE_HUMAN}" --db "${DB_PATH}" --theme "${THEME}"

cat <<OUT
stable setup complete
- db: ${DB_PATH}
- human profile: ${PROFILE_HUMAN}
- ai profile: ${PROFILE_AI}
- human key prefix: ${HUMAN_PREFIX}
- ai key prefix: ${AI_PREFIX}

next:
  ${DOOH_BIN} whoami
  ${DOOH_BIN} task list
OUT

if [ -n "${AI_KEY}" ]; then
  cat <<OUT

for AI runtime .env:
  DOOH_AI_KEY=${AI_KEY}
OUT
fi
