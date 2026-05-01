#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
UCONSOLE_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

SERVER_URL="${1:-http://127.0.0.1:8787}"
BIN_SOURCE="${UCONSOLE_DIR}/bin/codex-buddy-wayland"
ICON_SOURCE="${UCONSOLE_DIR}/assets/codex-buddy-uconsole.png"
DESKTOP_TEMPLATE="${SCRIPT_DIR}/codex-buddy-uconsole.desktop.template"
BIN_TARGET="${HOME}/.local/bin/codex-buddy-uconsole"
APP_DIR="${HOME}/.local/share/applications"
ICON_DIR="${HOME}/.local/share/icons/hicolor/1024x1024/apps"
ICON_TARGET="${ICON_DIR}/codex-buddy-uconsole.png"
DESKTOP_ID="github.com.vxider.codex-buddy.uconsole"
DESKTOP_FILE="${APP_DIR}/${DESKTOP_ID}.desktop"
LEGACY_DESKTOP_FILE="${APP_DIR}/codex-buddy-uconsole.desktop"
DESKTOP_COPY="${HOME}/Desktop/codex-buddy-uconsole.desktop"

if [[ ! -f "${ICON_SOURCE}" ]]; then
  echo "error: icon not found: ${ICON_SOURCE}" >&2
  exit 1
fi
if [[ ! -f "${DESKTOP_TEMPLATE}" ]]; then
  echo "error: desktop template not found: ${DESKTOP_TEMPLATE}" >&2
  exit 1
fi

mkdir -p "${HOME}/.local/bin" "${APP_DIR}" "${ICON_DIR}"

if [[ -x "${BIN_TARGET}" ]]; then
  BIN_INSTALL_SOURCE="${BIN_TARGET}"
elif [[ -x "${BIN_SOURCE}" ]]; then
  BIN_INSTALL_SOURCE="${BIN_SOURCE}"
  cp "${BIN_SOURCE}" "${BIN_TARGET}"
  chmod +x "${BIN_TARGET}"
else
  echo "error: binary not found: ${BIN_TARGET} or ${BIN_SOURCE}" >&2
  exit 1
fi

cp "${ICON_SOURCE}" "${ICON_TARGET}"

escape_sed_replacement() {
  printf '%s' "$1" | sed -e 's/[&|]/\\&/g'
}

BIN_TARGET_ESCAPED="$(escape_sed_replacement "${BIN_TARGET}")"
SERVER_URL_ESCAPED="$(escape_sed_replacement "${SERVER_URL}")"

sed \
	-e "s|__BIN_TARGET__|${BIN_TARGET_ESCAPED}|g" \
	-e "s|__SERVER_URL__|${SERVER_URL_ESCAPED}|g" \
	"${DESKTOP_TEMPLATE}" > "${DESKTOP_FILE}"

chmod +x "${DESKTOP_FILE}"
cp "${DESKTOP_FILE}" "${LEGACY_DESKTOP_FILE}"
chmod +x "${LEGACY_DESKTOP_FILE}"

if [[ -d "${HOME}/Desktop" ]]; then
	cp "${DESKTOP_FILE}" "${DESKTOP_COPY}"
  chmod +x "${DESKTOP_COPY}"
fi

echo "Installed binary: ${BIN_TARGET}"
echo "Binary source: ${BIN_INSTALL_SOURCE}"
echo "Installed icon: ${ICON_TARGET}"
echo "Installed launcher: ${DESKTOP_FILE}"
echo "Legacy launcher: ${LEGACY_DESKTOP_FILE}"
if [[ -f "${DESKTOP_COPY}" ]]; then
	echo "Desktop shortcut: ${DESKTOP_COPY}"
fi
