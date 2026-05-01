#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./uconsole/scripts/build-install.sh [options]

Options:
  --output PATH       Install destination (default: ~/.local/bin/codex-buddy-uconsole)
  --dry-run           Print the resolved build/install plan without writing files
  -h, --help          Show this help

Examples:
  ./uconsole/scripts/build-install.sh
EOF
}

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"
DESKTOP_INSTALL_SCRIPT="${SCRIPT_DIR}/install-user-desktop.sh"

GO_HOME_BASE="${HOME}"
if [[ ! -d "${GO_HOME_BASE}" || ! -w "${GO_HOME_BASE}" ]]; then
  GO_HOME_BASE="${TMPDIR:-/tmp}/codex-buddy-go"
fi

DRY_RUN=0
INSTALL_PATH="${HOME}/.local/bin/codex-buddy-uconsole"

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

echo "==> codex-buddy uconsole build + install"
echo "repo: ${REPO_ROOT}"
echo "install: ${INSTALL_PATH}"
echo "tags: ${TAGS[*]}"
echo "service: n/a (GUI app)"

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
  go build -tags "$(IFS=' '; printf '%s' "${TAGS[*]}")" -o "${TMP_BIN}" ./cmd/codex-buddy-uconsole
)

echo "==> installing"
install -m 0755 "${TMP_BIN}" "${TMP_INSTALL}"
mv "${TMP_INSTALL}" "${INSTALL_PATH}"

if [[ -x "${DESKTOP_INSTALL_SCRIPT}" ]]; then
  echo "==> refreshing desktop launcher"
  "${DESKTOP_INSTALL_SCRIPT}"
fi

echo "==> done"
echo "binary: ${INSTALL_PATH}"
