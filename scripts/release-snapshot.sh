#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

missing=0
for tool in goreleaser syft; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "ERROR: $tool is required for release snapshots" >&2
        missing=1
    fi
done

if [ "$missing" -ne 0 ]; then
    echo "Install goreleaser and syft before running this script." >&2
    exit 1
fi

cd "$ROOT_DIR"
exec goreleaser release --snapshot --clean --skip=before,docker
