#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

if ! command -v xcodegen >/dev/null 2>&1; then
  echo "xcodegen is required. Install it on macOS with: brew install xcodegen" >&2
  exit 1
fi

specs="project.yml"

if [[ -f project.local.env ]]; then
  echo "Using local signing overrides from project.local.env"
  # shellcheck disable=SC1091
  source project.local.env
fi

xcodegen generate --spec "$specs"

echo "Generated ios/CodexBuddy.xcodeproj"
