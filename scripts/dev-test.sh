#!/usr/bin/env bash
set -euo pipefail

# dev-test.sh — Run a focused subset of Go tests during development.
# Usage:
#   ./scripts/dev-test.sh              # Run common packages quickly
#   ./scripts/dev-test.sh ./internal/build/...
#   ./scripts/dev-test.sh TestName ./internal/deploy/...

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ $# -eq 0 ]; then
  echo "==> Running dev test suite (core, api, ingress, deploy, auth, build)..."
  go test -count=1 ./internal/core/... ./internal/api/... ./internal/ingress/... ./internal/deploy/... ./internal/auth/... ./internal/build/...
else
  echo "==> Running: go test -count=1 -v $*"
  go test -count=1 -v "$@"
fi

echo "==> Dev tests passed"
