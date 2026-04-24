#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./webserver/scripts/build-install.sh [options]

Options:
  --output PATH       Install destination (default: ~/.local/bin/codex-buddy)
  --no-restart        Do not restart `codex-buddy.service` after install
  --dry-run           Print the resolved build/install plan without writing files
  -h, --help          Show this help

Examples:
  ./webserver/scripts/build-install.sh
  ./webserver/scripts/build-install.sh --no-restart
EOF
}

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"

RESTART_SERVICE=1
DRY_RUN=0
INSTALL_PATH="${HOME}/.local/bin/codex-buddy"

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
    --no-restart)
      RESTART_SERVICE=0
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
      echo >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is not installed or not in PATH" >&2
  exit 1
fi

if [[ -z "${GOPATH:-}" || "${GOPATH:-}" == /Users/* ]]; then
  export GOPATH="${HOME}/.go"
fi
if [[ -z "${GOMODCACHE:-}" || "${GOMODCACHE:-}" == /Users/* ]]; then
  export GOMODCACHE="${GOPATH}/pkg/mod"
fi
if [[ -z "${GOCACHE:-}" || "${GOCACHE:-}" == /Users/* ]]; then
  export GOCACHE="${HOME}/.cache/go-build"
fi

mkdir -p "${GOPATH}" "${GOMODCACHE}" "${GOCACHE}"

SERVICE_PATH="${HOME}/.config/systemd/user/codex-buddy.service"
SHOULD_RESTART=0
if [[ "${RESTART_SERVICE}" -eq 1 && -f "${SERVICE_PATH}" ]] && command -v systemctl >/dev/null 2>&1; then
  SHOULD_RESTART=1
fi

echo "==> codex-buddy webserver build + install"
echo "repo: ${REPO_ROOT}"
echo "install: ${INSTALL_PATH}"
echo "tags: <none>"
if [[ "${SHOULD_RESTART}" -eq 1 ]]; then
  echo "service: codex-buddy.service will be restarted"
else
  echo "service: no restart"
fi

if [[ "${DRY_RUN}" -eq 1 ]]; then
  exit 0
fi

mkdir -p "$(dirname -- "${INSTALL_PATH}")"
TMP_BIN="$(mktemp "${TMPDIR:-/tmp}/codex-buddy.webserver.XXXXXX")"
INSTALL_DIR="$(dirname -- "${INSTALL_PATH}")"
TMP_INSTALL="$(mktemp "${INSTALL_DIR}/.codex-buddy.install.XXXXXX")"
trap 'rm -f -- "${TMP_BIN}" "${TMP_INSTALL}"' EXIT

echo "==> building"
(
  cd "${REPO_ROOT}"
  go build -o "${TMP_BIN}" ./cmd/codex-buddy
)

echo "==> installing"
cp "${TMP_BIN}" "${TMP_INSTALL}"
chmod +x "${TMP_INSTALL}"
mv -f "${TMP_INSTALL}" "${INSTALL_PATH}"

if [[ "${SHOULD_RESTART}" -eq 1 ]]; then
  echo "==> restarting service"
  systemctl --user daemon-reload
  systemctl --user restart codex-buddy.service
fi

echo "==> done"
echo "binary: ${INSTALL_PATH}"
if [[ "${SHOULD_RESTART}" -eq 1 ]]; then
  echo "status page: http://127.0.0.1:8787/status"
fi
