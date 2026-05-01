#!/usr/bin/env bash
set -euo pipefail

# smoke-docker.sh — Run the fresh-VM smoke test locally inside Docker.
# This simulates a clean Ubuntu 24.04 VM without needing cloud infra.
#
# Requirements:
#   - Docker running locally
#   - Docker socket accessible at /var/run/docker.sock
#
# Usage:
#   ./scripts/smoke-docker.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
IMAGE="ubuntu:24.04"

if ! command -v docker >/dev/null 2>&1; then
    echo "ERROR: docker is required" >&2
    exit 1
fi

if [ ! -S /var/run/docker.sock ]; then
    echo "ERROR: /var/run/docker.sock not found" >&2
    exit 1
fi

echo "==> Pulling ${IMAGE}..."
docker pull -q "$IMAGE"

echo "==> Starting smoke-test container..."
docker run --rm \
  --privileged \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$ROOT_DIR:/repo:ro" \
  -e SMOKE_NO_SYSTEMD=1 \
  -e INSTALLER_PATH="/repo/scripts/install.sh" \
  "$IMAGE" bash -c '
    set -euo pipefail
    export DEBIAN_FRONTEND=noninteractive

    echo "==> Installing prerequisites..."
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates tar docker.io >/dev/null 2>&1

    echo "==> Running smoke-test.sh..."
    bash /repo/scripts/smoke-test.sh
  '
