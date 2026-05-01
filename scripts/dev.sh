#!/usr/bin/env bash
set -euo pipefail

# dev.sh — Start the full development stack (React HMR + Go backend).
# Usage:
#   ./scripts/dev.sh
#
# Prerequisites:
#   - Go 1.26+
#   - Node.js 22+ with pnpm
#   - Docker (for deployments)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$ROOT_DIR"

# Detect whether pnpm is available
if ! command -v pnpm >/dev/null 2>&1; then
    echo "ERROR: pnpm is required (see CLAUDE.md — web/ uses pnpm, not npm)" >&2
    exit 1
fi

cd "$ROOT_DIR/web"

# Install dependencies if needed
if [ ! -d "node_modules" ]; then
    echo "==> Installing web dependencies..."
    pnpm install --frozen-lockfile --silent
fi

# Build a one-time embed so Go serve has a UI fallback
echo "==> Embedding placeholder UI for Go server..."
EMBED_DIR="$ROOT_DIR/internal/api/static"
rm -rf "$EMBED_DIR"
mkdir -p "$EMBED_DIR"
if [ -d "$ROOT_DIR/web/dist" ]; then
    cp -r "$ROOT_DIR/web/dist/"* "$EMBED_DIR/"
else
    echo "<html><body><h1>DeployMonster</h1><p>Run pnpm run build to update UI.</p></body></html>" > "$EMBED_DIR/index.html"
fi

# Use concurrently if available (from web/node_modules/.bin or globally)
CONCURRENTLY=""
if [ -x "$ROOT_DIR/web/node_modules/.bin/concurrently" ]; then
    CONCURRENTLY="$ROOT_DIR/web/node_modules/.bin/concurrently"
elif command -v concurrently >/dev/null 2>&1; then
    CONCURRENTLY="concurrently"
fi

cd "$ROOT_DIR"

if [ -n "$CONCURRENTLY" ]; then
    echo "==> Starting dev stack with concurrently..."
    "$CONCURRENTLY" --names "WEB,GO" --prefix-colors "cyan,green" \
        "cd web && pnpm run dev" \
        "go run ./cmd/deploymonster"
else
    echo "==> Starting dev stack (no concurrently found)..."
    # Start React dev server in background
    (cd web && pnpm run dev) &
    WEB_PID=$!

    finish() {
        echo ""
        echo "==> Stopping dev servers..."
        kill "$WEB_PID" 2>/dev/null || true
        wait "$WEB_PID" 2>/dev/null || true
        exit 0
    }
    trap finish INT TERM EXIT

    # Start Go dev server in foreground
    go run ./cmd/deploymonster
fi
