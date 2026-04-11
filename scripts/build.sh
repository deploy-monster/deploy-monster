#!/usr/bin/env bash
set -euo pipefail

# Build script for DeployMonster
# 1. Build React UI
# 2. Copy dist to internal/api/static/ for embed.FS
# 3. Build Go binary with ldflags

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

VERSION="${VERSION:-$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo "dev")}"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo "none")}"
DATE="${DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
OUTPUT="${OUTPUT:-$ROOT_DIR/bin/deploymonster}"

echo "==> Building DeployMonster $VERSION ($COMMIT)"

# Step 1: Build React UI
if [ -f "$ROOT_DIR/web/package.json" ]; then
    echo "==> Building React UI..."
    cd "$ROOT_DIR/web"
    if ! command -v pnpm >/dev/null 2>&1; then
        echo "ERROR: pnpm is required (see CLAUDE.md — web/ uses pnpm, not npm)" >&2
        exit 1
    fi
    pnpm install --frozen-lockfile --silent
    pnpm run build
    cd "$ROOT_DIR"
    echo "==> React UI built"
else
    echo "==> Skipping React UI (no web/package.json)"
fi

# Step 2: Copy UI to embed directory
EMBED_DIR="$ROOT_DIR/internal/api/static"
rm -rf "$EMBED_DIR"
mkdir -p "$EMBED_DIR"

if [ -d "$ROOT_DIR/web/dist" ]; then
    cp -r "$ROOT_DIR/web/dist/"* "$EMBED_DIR/"
    echo "==> UI embedded ($(du -sh "$EMBED_DIR" | cut -f1))"
else
    echo "<html><body><h1>DeployMonster</h1></body></html>" > "$EMBED_DIR/index.html"
    echo "==> Placeholder UI embedded"
fi

# Step 3: Build Go binary
echo "==> Building Go binary..."
mkdir -p "$(dirname "$OUTPUT")"
CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE" \
    -o "$OUTPUT" \
    "$ROOT_DIR/cmd/deploymonster"

echo "==> Done: $OUTPUT ($(du -h "$OUTPUT" | cut -f1))"
