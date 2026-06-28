#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
MACOS_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

if ! command -v xcodegen >/dev/null 2>&1; then
  echo "error: xcodegen is required" >&2
  exit 1
fi

if ! command -v xcodebuild >/dev/null 2>&1; then
  echo "error: xcodebuild is required" >&2
  exit 1
fi

cd "${MACOS_DIR}"
xcodegen generate
xcodebuild -scheme AgentBuddyMenuBar -configuration Release build
