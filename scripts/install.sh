#!/usr/bin/env bash
# DeployMonster Installer
# Usage: curl -fsSL https://get.deploy.monster | bash
set -euo pipefail

REPO="deploy-monster/deploy-monster"
INSTALL_DIR="/usr/local/bin"
DATA_DIR="/var/lib/deploymonster"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) error "Unsupported architecture: $ARCH" ;;
    esac

    case "$OS" in
        linux|darwin) ;;
        *) error "Unsupported OS: $OS" ;;
    esac

    info "Detected platform: ${OS}/${ARCH}"
}

# Get latest release version
get_latest_version() {
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$VERSION" ]; then
        VERSION="latest"
    fi
    info "Latest version: ${VERSION}"
}

# Download and install binary
install_binary() {
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/deploymonster_${VERSION#v}_${OS}_${ARCH}.tar.gz"

    info "Downloading DeployMonster ${VERSION}..."
    TMP_DIR=$(mktemp -d)
    trap "rm -rf ${TMP_DIR}" EXIT

    curl -fsSL "$DOWNLOAD_URL" -o "${TMP_DIR}/deploymonster.tar.gz" || {
        warn "Release download failed, trying dev build..."
        # Fallback: build from source
        if command -v go &>/dev/null; then
            info "Building from source..."
            go install "github.com/${REPO}/cmd/deploymonster@latest"
            return
        fi
        error "Download failed and Go is not installed for source build"
    }

    tar -xzf "${TMP_DIR}/deploymonster.tar.gz" -C "${TMP_DIR}"

    # Install with sudo if needed
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMP_DIR}/deploymonster" "${INSTALL_DIR}/deploymonster"
    else
        sudo mv "${TMP_DIR}/deploymonster" "${INSTALL_DIR}/deploymonster"
    fi

    chmod +x "${INSTALL_DIR}/deploymonster"
    info "Installed to ${INSTALL_DIR}/deploymonster"
}

# Create data directory and systemd service
setup_service() {
    # Create data directory
    if [ ! -d "$DATA_DIR" ]; then
        sudo mkdir -p "$DATA_DIR"
        info "Created ${DATA_DIR}"
    fi

    # Create systemd service (Linux only)
    if [ "$OS" = "linux" ] && command -v systemctl &>/dev/null; then
        sudo tee /etc/systemd/system/deploymonster.service > /dev/null << 'EOF'
[Unit]
Description=DeployMonster PaaS
After=docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/deploymonster serve
Restart=always
RestartSec=5
WorkingDirectory=/var/lib/deploymonster

[Install]
WantedBy=multi-user.target
EOF
        sudo systemctl daemon-reload
        info "Systemd service created"
    fi
}

# Main
main() {
    echo ""
    echo "  ____             _             __  __                 _            "
    echo " |  _ \\  ___ _ __ | | ___  _   _|  \\/  | ___  _ __  ___| |_ ___ _ __ "
    echo " | | | |/ _ \\ '_ \\| |/ _ \\| | | | |\\/| |/ _ \\| '_ \\/ __| __/ _ \\ '__|"
    echo " | |_| |  __/ |_) | | (_) | |_| | |  | | (_) | | | \\__ \\ ||  __/ |   "
    echo " |____/ \\___| .__/|_|\\___/ \\__, |_|  |_|\\___/|_| |_|___/\\__\\___|_|   "
    echo "            |_|            |___/                                      "
    echo ""
    echo "  Tame Your Deployments"
    echo ""

    detect_platform
    get_latest_version
    install_binary
    setup_service

    echo ""
    info "DeployMonster installed successfully!"
    echo ""
    echo "  Start the server:"
    echo "    deploymonster"
    echo ""
    echo "  Or with systemd:"
    echo "    sudo systemctl start deploymonster"
    echo ""
    echo "  Then open: https://localhost:8443"
    echo ""
}

main "$@"
