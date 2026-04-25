#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./tailscale-tray/scripts/build-install.sh [options]

Options:
  --output PATH     install destination (default: ~/.local/bin/tailscale-tray)
  --no-autostart    do not install an autostart desktop entry
  --dry-run         print the resolved install plan without writing files
  -h, --help        show this help
EOF
}

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

INSTALL_PATH="${HOME}/.local/bin/tailscale-tray"
AUTOSTART=1
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      if [[ $# -lt 2 ]]; then
        echo "error: --output requires a path" >&2
        exit 2
      fi
      INSTALL_PATH="$2"
      shift 2
      ;;
    --no-autostart)
      AUTOSTART=0
      shift
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is not installed or not in PATH" >&2
  exit 1
fi

BIN_DIR="$(dirname -- "${INSTALL_PATH}")"
APP_DIR="${HOME}/.local/share/applications"
AUTOSTART_DIR="${HOME}/.config/autostart"
DESKTOP_FILE="${APP_DIR}/tailscale-tray.desktop"
AUTOSTART_FILE="${AUTOSTART_DIR}/tailscale-tray.desktop"

echo "==> tailscale tray build + install"
echo "module: ${MODULE_DIR}"
echo "install: ${INSTALL_PATH}"
if [[ "${AUTOSTART}" -eq 1 ]]; then
  echo "autostart: enabled"
else
  echo "autostart: disabled"
fi

if [[ "${DRY_RUN}" -eq 1 ]]; then
  exit 0
fi

mkdir -p "${BIN_DIR}" "${APP_DIR}"
if [[ "${AUTOSTART}" -eq 1 ]]; then
  mkdir -p "${AUTOSTART_DIR}"
fi

TMP_BIN="$(mktemp "${TMPDIR:-/tmp}/tailscale-tray.XXXXXX")"
trap 'rm -f -- "${TMP_BIN}"' EXIT

echo "==> building"
(
  cd "${MODULE_DIR}"
  GOPROXY="${GOPROXY:-off}" GOSUMDB="${GOSUMDB:-off}" go build -o "${TMP_BIN}" .
)

echo "==> installing"
cp "${TMP_BIN}" "${INSTALL_PATH}"
chmod +x "${INSTALL_PATH}"

cat > "${DESKTOP_FILE}" <<EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=Tailscale Tray
Comment=Standalone Tailscale taskbar plugin
Exec=${INSTALL_PATH}
Icon=network-vpn
Terminal=false
Categories=Network;Utility;
StartupNotify=false
EOF

chmod +x "${DESKTOP_FILE}"

if [[ "${AUTOSTART}" -eq 1 ]]; then
  cp "${DESKTOP_FILE}" "${AUTOSTART_FILE}"
  chmod +x "${AUTOSTART_FILE}"
else
  rm -f "${AUTOSTART_FILE}"
fi

echo "==> done"
echo "binary: ${INSTALL_PATH}"
echo "launcher: ${DESKTOP_FILE}"
if [[ "${AUTOSTART}" -eq 1 ]]; then
  echo "autostart: ${AUTOSTART_FILE}"
fi
