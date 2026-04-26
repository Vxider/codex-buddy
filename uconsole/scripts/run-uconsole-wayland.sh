#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
UCONSOLE_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

SERVER_URL="${1:-http://127.0.0.1:8787}"
BIN_PATH="${UCONSOLE_DIR}/bin/codex-buddy-wayland"
HOME_BASE="${TMPDIR:-/tmp}/codex-buddy-home"

mkdir -p "${HOME_BASE}/.config" "${HOME_BASE}/.cache" "${HOME_BASE}/.local/share"

exec env \
  -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY \
  -u http_proxy -u https_proxy -u all_proxy \
  HOME="${HOME_BASE}" \
  XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/1000}" \
  WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-0}" \
  "${BIN_PATH}" uconsole --server-url "${SERVER_URL}" --no-led
