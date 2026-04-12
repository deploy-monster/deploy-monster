#!/usr/bin/env bash
# DeployMonster standalone uninstaller
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/uninstall.sh | bash
# Or locally:
#   sudo bash scripts/uninstall.sh

set -euo pipefail

INSTALLER_URL="${INSTALLER_URL:-https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh}"

# Re-use the installer's uninstall path
exec bash -c "$(curl -fsSL "$INSTALLER_URL" 2>/dev/null || cat "$(dirname "$0")/install.sh")" -- uninstall
