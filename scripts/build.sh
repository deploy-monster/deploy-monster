#!/usr/bin/env bash
set -euo pipefail

# Build script for DeployMonster
# Builds React UI (if web/ exists and has package.json) then Go binary

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

VERSION="${VERSION:-$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo "dev")}"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo "none")}"
DATE="${DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
OUTPUT="${OUTPUT:-$ROOT_DIR/bin/deploymonster}"

echo "==> Building DeployMonster $VERSION ($COMMIT)"

# Build React UI if package.json exists
if [ -f "$ROOT_DIR/web/package.json" ]; then
    echo "==> Building React UI..."
    cd "$ROOT_DIR/web"
    npm ci --silent
    npm run build
    cd "$ROOT_DIR"
    echo "==> React UI built successfully"
else
    echo "==> Skipping React UI build (no web/package.json)"
    mkdir -p "$ROOT_DIR/web/dist"
    echo "<html><body><h1>DeployMonster</h1><p>UI not built yet.</p></body></html>" > "$ROOT_DIR/web/dist/index.html"
fi

# Build Go binary
echo "==> Building Go binary..."
mkdir -p "$(dirname "$OUTPUT")"
CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE" \
    -o "$OUTPUT" \
    "$ROOT_DIR/cmd/deploymonster"

echo "==> Built: $OUTPUT ($(du -h "$OUTPUT" | cut -f1))"
