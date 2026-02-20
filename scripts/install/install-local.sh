#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BIN_DIR="${HOME}/.local/bin"
DOOH_HOME_DEV="${HOME}/.config/dooh-dev"
DOOH_DEV_DB="${HOME}/.local/share/dooh-dev/dooh-dev.db"

mkdir -p "${BIN_DIR}" "${HOME}/.local/share/dooh-dev" "${DOOH_HOME_DEV}"

cd "${ROOT_DIR}"
go build -o "${BIN_DIR}/dooh" ./cmd/dooh
go build -o "${BIN_DIR}/dooh-dev-bin" ./cmd/dooh

cat > "${BIN_DIR}/dooh-dev" <<WRAP
#!/usr/bin/env bash
set -euo pipefail
SELF_DIR="\$(cd "\$(dirname "\$0")" && pwd)"
export DOOH_HOME="${DOOH_HOME:-\$HOME/.config/dooh-dev}"
export DOOH_DB="${DOOH_DB:-\$HOME/.local/share/dooh-dev/dooh-dev.db}"
export DOOH_PROFILE="${DOOH_PROFILE:-dev}"
exec "\${SELF_DIR}/dooh-dev-bin" "\$@"
WRAP

chmod +x "${BIN_DIR}/dooh" "${BIN_DIR}/dooh-dev-bin" "${BIN_DIR}/dooh-dev"

echo "installed ${BIN_DIR}/dooh"
echo "installed ${BIN_DIR}/dooh-dev (wrapper with isolated DOOH_HOME/DOOH_DB/DOOH_PROFILE)"
"${BIN_DIR}/dooh" version
"${BIN_DIR}/dooh-dev" version
