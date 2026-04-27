#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./uconsole/scripts/build-install.sh [options]

Options:
  --ws281x            Build with the WS2812 hardware driver (`ws281x`)
  --output PATH       Install destination (default: ~/.local/bin/codex-buddy)
  --restart-service   Restart `codex-buddy.service` after install
  --dry-run           Print the resolved build/install plan without writing files
  -h, --help          Show this help

Examples:
  ./uconsole/scripts/build-install.sh
  ./uconsole/scripts/build-install.sh --ws281x
  ./uconsole/scripts/build-install.sh --ws281x --restart-service
EOF
}

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"

GO_HOME_BASE="${HOME}"
if [[ ! -d "${GO_HOME_BASE}" || ! -w "${GO_HOME_BASE}" ]]; then
  GO_HOME_BASE="${TMPDIR:-/tmp}/codex-buddy-go"
fi

ENABLE_WS281X=0
RESTART_SERVICE=0
DRY_RUN=0
INSTALL_PATH="${HOME}/.local/bin/codex-buddy"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --ws281x)
      ENABLE_WS281X=1
      shift
      ;;
    --output)
      if [[ $# -lt 2 ]]; then
        echo "error: --output requires a path" >&2
        exit 2
      fi
      INSTALL_PATH="$2"
      shift 2
      ;;
    --restart-service)
      RESTART_SERVICE=1
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
  export GOPATH="${GO_HOME_BASE}/.go"
fi
if [[ -z "${GOMODCACHE:-}" || "${GOMODCACHE:-}" == /Users/* ]]; then
  export GOMODCACHE="${GOPATH}/pkg/mod"
fi
if [[ -z "${GOCACHE:-}" || "${GOCACHE:-}" == /Users/* ]]; then
  export GOCACHE="${GO_HOME_BASE}/.cache/go-build"
fi

mkdir -p "${GOPATH}" "${GOMODCACHE}" "${GOCACHE}"

REAL_CC_VALUE="${CC:-$(go env CC 2>/dev/null || true)}"
if [[ -z "${REAL_CC_VALUE}" ]]; then
  REAL_CC_VALUE="$(command -v aarch64-linux-gnu-gcc 2>/dev/null || command -v gcc)"
fi
export REAL_CC="${REAL_CC_VALUE}"
export CC="${SCRIPT_DIR}/gcc-no-base64.sh"

TAGS=("uconsole_gui")
if [[ "${ENABLE_WS281X}" -eq 1 ]]; then
  TAGS+=("ws281x")
fi

SERVICE_PATH="${HOME}/.config/systemd/user/codex-buddy.service"
SHOULD_RESTART=0
if [[ "${RESTART_SERVICE}" -eq 1 && -f "${SERVICE_PATH}" ]] && command -v systemctl >/dev/null 2>&1; then
  SHOULD_RESTART=1
fi

echo "==> codex-buddy uconsole build + install"
echo "repo: ${REPO_ROOT}"
echo "install: ${INSTALL_PATH}"
echo "tags: ${TAGS[*]}"
if [[ "${SHOULD_RESTART}" -eq 1 ]]; then
  echo "service: codex-buddy.service will be restarted"
else
  echo "service: no restart"
fi

if [[ "${DRY_RUN}" -eq 1 ]]; then
  exit 0
fi

INSTALL_DIR="$(dirname -- "${INSTALL_PATH}")"
mkdir -p "${INSTALL_DIR}"
TMP_BIN="$(mktemp "${TMPDIR:-/tmp}/codex-buddy.uconsole.XXXXXX")"
TMP_INSTALL="${INSTALL_DIR}/.codex-buddy.install.$$"
trap 'rm -f -- "${TMP_BIN}" "${TMP_INSTALL}"' EXIT

echo "==> building"
(
  cd "${REPO_ROOT}"
  go build -tags "$(IFS=' '; printf '%s' "${TAGS[*]}")" -o "${TMP_BIN}" ./cmd/codex-buddy
)

echo "==> installing"
install -m 0755 "${TMP_BIN}" "${TMP_INSTALL}"
mv "${TMP_INSTALL}" "${INSTALL_PATH}"

if [[ "${SHOULD_RESTART}" -eq 1 ]]; then
  echo "==> restarting service"
  systemctl --user daemon-reload
  systemctl --user restart codex-buddy.service
fi

echo "==> done"
echo "binary: ${INSTALL_PATH}"
