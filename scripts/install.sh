#!/usr/bin/env bash
# DeployMonster installer — curl-pipe entry point (GitHub-first)
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- --version=v0.0.1
#   curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- --force
#   curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- uninstall
#
# Design notes:
# - Downloads a goreleaser archive from a tagged GitHub release, verifies
#   it against the SHA256 in the release's `checksums.txt`, and installs
#   the binary to /usr/local/bin.
# - No `go install` fallback — that path silently ships a binary without
#   the embedded React SPA and is a worse user experience than failing
#   loudly. Users without a release binary for their platform should file
#   an issue.
# - No GPG/cosign verification yet (Phase 8 — requires a release key).
#   Checksum verification alone protects against a corrupted download and
#   against a tampered CDN serving wrong bytes for one specific file;
#   a full MITM replacing both the archive and checksums.txt requires
#   signature verification on top, which is intentionally deferred.

set -euo pipefail

REPO="deploy-monster/deploy-monster"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="deploymonster"
DATA_DIR="/var/lib/deploymonster"
SERVICE_FILE="/etc/systemd/system/deploymonster.service"

MODE="install"
VERSION=""
FORCE=0

# Populated by generate_config() for the systemd unit
GENERATED_ADMIN_EMAIL=""
GENERATED_ADMIN_PASSWORD=""

# ─── TTY-aware color helpers ────────────────────────────────────────────────
if [ -t 1 ] && [ -t 2 ]; then
    RED=$'\033[0;31m'
    GREEN=$'\033[0;32m'
    YELLOW=$'\033[1;33m'
    BLUE=$'\033[0;34m'
    NC=$'\033[0m'
else
    RED=""
    GREEN=""
    YELLOW=""
    BLUE=""
    NC=""
fi

info()  { printf '%s[INFO]%s %s\n'  "${GREEN}"  "${NC}" "$*"; }
warn()  { printf '%s[WARN]%s %s\n'  "${YELLOW}" "${NC}" "$*"; }
error() { printf '%s[ERROR]%s %s\n' "${RED}"    "${NC}" "$*" >&2; exit 1; }
step()  { printf '%s[ %s ]%s %s\n'  "${BLUE}"   "$1" "${NC}" "$2"; }

# ─── Arg parsing ────────────────────────────────────────────────────────────
parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            install|uninstall) MODE="$1" ;;
            --version=*) VERSION="${1#--version=}" ;;
            --version) shift; VERSION="${1:-}" ;;
            --force) FORCE=1 ;;
            --help|-h)
                sed -n '2,14p' "$0"
                exit 0
                ;;
            *) error "Unknown argument: $1 (try --help)" ;;
        esac
        shift
    done
}

# ─── Preflight ──────────────────────────────────────────────────────────────
require_cmd() {
    command -v "$1" >/dev/null 2>&1 || error "Missing required command: $1"
}

preflight() {
    require_cmd uname
    require_cmd curl
    require_cmd tar
    require_cmd mktemp
    # sha256sum (Linux) or shasum (Darwin) — we pick whichever is present
    # inside verify_checksum, so no hard require here.
}

# ─── Platform detection ─────────────────────────────────────────────────────
detect_platform() {
    local raw_os raw_arch
    raw_os=$(uname -s | tr '[:upper:]' '[:lower:]')
    raw_arch=$(uname -m)

    case "$raw_arch" in
        x86_64|amd64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) error "Unsupported architecture: $raw_arch" ;;
    esac

    case "$raw_os" in
        linux|darwin) OS="$raw_os" ;;
        *) error "Unsupported OS: $raw_os (Linux and macOS only)" ;;
    esac

    info "Detected platform: ${OS}/${ARCH}"
}

# ─── Version resolution ─────────────────────────────────────────────────────
get_latest_version() {
    if [ -n "$VERSION" ]; then
        info "Pinned version: ${VERSION}"
        return
    fi

    local api_url response
    api_url="https://api.github.com/repos/${REPO}/releases/latest"
    info "Resolving latest release from ${api_url}..."

    response=$(curl -fsSL "$api_url") || error "Failed to query GitHub API (rate limited or offline?)"

    VERSION=$(printf '%s\n' "$response" \
        | grep -E '^\s*"tag_name"\s*:' \
        | head -1 \
        | sed -E 's/.*"tag_name"\s*:\s*"([^"]+)".*/\1/')

    if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$ ]]; then
        error "Could not parse a vX.Y.Z[-suffix] tag out of the GitHub API response (got: '${VERSION}')"
    fi

    info "Latest version: ${VERSION}"
}

# ─── Checksum verification ──────────────────────────────────────────────────
verify_checksum() {
    local archive_path="$1"
    local archive_name="$2"
    local checksums_path="$3"

    local expected actual
    expected=$(awk -v name="$archive_name" '$2 == name { print $1 }' "$checksums_path")
    if [ -z "$expected" ]; then
        error "No SHA256 for ${archive_name} in checksums.txt (release broken?)"
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$archive_path" | awk '{ print $1 }')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$archive_path" | awk '{ print $1 }')
    else
        error "Missing sha256sum or shasum — cannot verify checksum"
    fi

    if [ "$expected" != "$actual" ]; then
        error "Checksum mismatch for ${archive_name}: expected ${expected}, got ${actual}"
    fi

    info "Checksum verified (${expected:0:16}…)"
}

# ─── Remove service + binary (shared by force-install and uninstall) ────────
remove_service_and_binary() {
    local reason="${1:-}"

    if [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
        if systemctl list-unit-files deploymonster.service >/dev/null 2>&1; then
            [ -n "$reason" ] && info "Stopping existing service ($reason)..."
            sudo systemctl stop    deploymonster.service 2>/dev/null || true
            sudo systemctl disable deploymonster.service 2>/dev/null || true
            info "Stopped and disabled deploymonster.service"
        fi
        if [ -f "$SERVICE_FILE" ]; then
            sudo rm -f "$SERVICE_FILE"
            sudo systemctl daemon-reload
            info "Removed ${SERVICE_FILE}"
        fi
    fi

    if [ -x "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        if [ -w "$INSTALL_DIR" ]; then
            rm -f "${INSTALL_DIR}/${BINARY_NAME}"
        else
            sudo rm -f "${INSTALL_DIR}/${BINARY_NAME}"
        fi
        info "Removed ${INSTALL_DIR}/${BINARY_NAME}"
    fi
}

# ─── Download + install binary ──────────────────────────────────────────────
install_binary() {
    if [ -x "${INSTALL_DIR}/${BINARY_NAME}" ] && [ "$FORCE" -ne 1 ]; then
        local existing
        existing=$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null | head -1 || echo "unknown")
        warn "${INSTALL_DIR}/${BINARY_NAME} already present (${existing})."
        warn "Pass --force to overwrite, or run: $0 uninstall"
        return
    fi

    # Force mode: clean slate before downloading
    if [ "$FORCE" -eq 1 ] && { [ -x "${INSTALL_DIR}/${BINARY_NAME}" ] || [ -f "$SERVICE_FILE" ]; }; then
        remove_service_and_binary "force reinstall"
    fi

    local version_stripped archive_name download_url checksums_url
    version_stripped="${VERSION#v}"
    archive_name="deploymonster_${version_stripped}_${OS}_${ARCH}.tar.gz"
    download_url="https://github.com/${REPO}/releases/download/${VERSION}/${archive_name}"
    checksums_url="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

    tmp_dir=$(mktemp -d)
    trap 'rm -rf "${tmp_dir}"' EXIT

    info "Downloading ${archive_name}..."
    curl -fsSL "$download_url" -o "${tmp_dir}/${archive_name}" \
        || error "Failed to download ${download_url}"

    info "Downloading checksums.txt..."
    curl -fsSL "$checksums_url" -o "${tmp_dir}/checksums.txt" \
        || error "Failed to download ${checksums_url}"

    verify_checksum "${tmp_dir}/${archive_name}" "$archive_name" "${tmp_dir}/checksums.txt"

    info "Extracting archive..."
    tar -xzf "${tmp_dir}/${archive_name}" -C "${tmp_dir}" --no-same-owner

    if [ ! -f "${tmp_dir}/${BINARY_NAME}" ]; then
        error "Archive did not contain a '${BINARY_NAME}' binary at the root"
    fi

    chmod +x "${tmp_dir}/${BINARY_NAME}"

    if [ -w "$INSTALL_DIR" ]; then
        mv "${tmp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        require_cmd sudo
        sudo mv "${tmp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    info "Installed to ${INSTALL_DIR}/${BINARY_NAME}"
}

# ─── Interactive configuration ──────────────────────────────────────────────
generate_config() {
    local config_path="${DATA_DIR}/monster.yaml"

    if [ -f "$config_path" ]; then
        if [ "$FORCE" -ne 1 ]; then
            warn "Configuration already exists at ${config_path}"
            warn "Pass --force to regenerate it during install"
            return
        fi
        local backup_suffix=".bak.$(date +%s)"
        local sudo_cmd=""
        [ ! -w "$DATA_DIR" ] && sudo_cmd="sudo"
        $sudo_cmd cp "$config_path" "${config_path}${backup_suffix}"
        info "Backed up existing config to ${config_path}${backup_suffix}"
    fi

    # Global scope variables (not local) - used later in main()
    domain="${MONSTER_DOMAIN:-}"
    acme_email="${MONSTER_ACME_EMAIL:-}"
    admin_email="${MONSTER_ADMIN_EMAIL:-}"
    admin_password="${MONSTER_ADMIN_PASSWORD:-}"
    GENERATED_ADMIN_EMAIL=""
    GENERATED_ADMIN_PASSWORD=""

    if [ -t 0 ]; then
        echo
        step "SETUP" "Answer a few questions (press Enter to accept defaults)"
        echo
        read -rp "Platform domain (optional, e.g., deploy.example.com): " input_domain
        domain="${input_domain:-$domain}"

        read -rp "ACME / Let's Encrypt email (optional): " input_acme
        acme_email="${input_acme:-$acme_email}"

        if [ -z "$admin_email" ]; then
            read -rp "Admin email [admin@local.host]: " admin_email
            admin_email="${admin_email:-admin@local.host}"
        else
            read -rp "Admin email [${admin_email}]: " input_admin_email
            admin_email="${input_admin_email:-$admin_email}"
        fi

        if [ -z "$admin_password" ]; then
            local generated_pass
            generated_pass=$(tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 16 || echo "changeme")
            read -rsp "Admin password (press Enter to use auto-generated): " admin_password
            echo >&2
            admin_password="${admin_password:-$generated_pass}"
            if [ "$admin_password" = "$generated_pass" ]; then
                info "Auto-generated admin password: ${generated_pass}"
            fi
        fi
        echo
    else
        # Non-interactive: use defaults if not provided via env
        admin_email="${admin_email:-admin@local.host}"
        if [ -z "$admin_password" ]; then
            admin_password=$(tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 16 || echo "changeme")
            info "Non-interactive mode: auto-generated admin password"
        fi
    fi

    local server_port=8443
    if [ -n "$domain" ]; then
        server_port=443
    fi

    # Generate secret key if not provided
    local secret_key="${MONSTER_SECRET_KEY:-}"
    if [ -z "$secret_key" ]; then
        secret_key=$(openssl rand -hex 32 2>/dev/null || tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 64 || echo "")
    fi

    local sudo_cmd=""
    [ ! -w "$DATA_DIR" ] && sudo_cmd="sudo"

    $sudo_cmd tee "$config_path" > /dev/null <<EOF
server:
  host: 0.0.0.0
  port: ${server_port}
  domain: "${domain}"
  secret_key: "${secret_key}"

database:
  driver: sqlite
  path: deploymonster.db

ingress:
  http_port: 80
  https_port: 443
  enable_https: true
  force_https: true

acme:
  email: "${acme_email}"
  staging: false
  provider: http-01

docker:
  host: unix:///var/run/docker.sock

backup:
  schedule: "02:00"
  retention_days: 30
  storage_path: /var/lib/deploymonster/backups
  encryption: true

registration:
  mode: open

limits:
  max_apps_per_tenant: 100
  max_build_minutes: 30
  max_concurrent_builds: 5
EOF

    $sudo_cmd chmod 640 "$config_path"
    info "Config written to ${config_path}"

    GENERATED_ADMIN_EMAIL="$admin_email"
    GENERATED_ADMIN_PASSWORD="$admin_password"
}

# ─── Data dir + systemd unit ────────────────────────────────────────────────
setup_service() {
    local sudo_cmd=""
    if [ ! -w "$(dirname "$DATA_DIR")" ]; then
        require_cmd sudo
        sudo_cmd="sudo"
    fi

    if [ ! -d "$DATA_DIR" ]; then
        $sudo_cmd mkdir -p "$DATA_DIR"
        info "Created ${DATA_DIR}"
    fi

    if [ "$OS" != "linux" ] || ! command -v systemctl >/dev/null 2>&1; then
        return
    fi

    require_cmd sudo

    # Build unit file dynamically so we can inject admin credentials
    local unit_content=""
    unit_content="[Unit]
Description=DeployMonster PaaS
Documentation=https://github.com/deploy-monster/deploy-monster
After=network-online.target docker.service
Wants=network-online.target docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/deploymonster serve
Restart=always
RestartSec=5
WorkingDirectory=/var/lib/deploymonster
LimitNOFILE=65536"

    if [ -n "${GENERATED_ADMIN_EMAIL:-}" ]; then
        unit_content="${unit_content}
Environment=MONSTER_ADMIN_EMAIL=${GENERATED_ADMIN_EMAIL}"
    fi
    if [ -n "${GENERATED_ADMIN_PASSWORD:-}" ]; then
        unit_content="${unit_content}
Environment=MONSTER_ADMIN_PASSWORD=${GENERATED_ADMIN_PASSWORD}"
    fi

    unit_content="${unit_content}
# Do not set User= — the daemon needs access to /var/run/docker.sock and
# to bind :80/:443 on first-run ACME. Users who want a dedicated system
# account should adjust this unit and add that account to the \`docker\`
# group manually.

[Install]
WantedBy=multi-user.target"

    printf '%s\n' "$unit_content" | sudo tee "$SERVICE_FILE" > /dev/null
    sudo systemctl daemon-reload
    sudo systemctl enable deploymonster.service >/dev/null
    info "Systemd unit installed and enabled"
}

# ─── Uninstall path ─────────────────────────────────────────────────────────
do_uninstall() {
    info "Uninstalling DeployMonster..."
    remove_service_and_binary "uninstall"

    if [ -d "$DATA_DIR" ]; then
        warn "Data directory preserved: ${DATA_DIR}"
        warn "Remove manually if you're sure: sudo rm -rf ${DATA_DIR}"
    fi

    info "DeployMonster uninstalled."
}

# ─── Entry banner ───────────────────────────────────────────────────────────
banner() {
    cat <<'ASCII'

  ____             _             __  __                 _
 |  _ \  ___ _ __ | | ___  _   _|  \/  | ___  _ __  ___| |_ ___ _ __
 | | | |/ _ \ '_ \| |/ _ \| | | | |\/| |/ _ \| '_ \/ __| __/ _ \ '__|
 | |_| |  __/ |_) | | (_) | |_| | |  | | (_) | | | \__ \ ||  __/ |
 |____/ \___| .__/|_|\___/ \__, |_|  |_|\___/|_| |_|___/\__\___|_|
            |_|            |___/

  Tame Your Deployments

ASCII
}

main() {
    parse_args "$@"
    preflight
    banner

    if [ "$MODE" = "uninstall" ]; then
        detect_platform
        do_uninstall
        return
    fi

    detect_platform

    if command -v docker >/dev/null 2>&1; then
        info "Docker found: $(docker --version | head -1)"
    else
        warn "Docker not found — DeployMonster needs it to deploy applications."
        warn "Install Docker first: https://docs.docker.com/engine/install/"
    fi

    get_latest_version
    install_binary
    setup_service
    generate_config

    # Reload systemd unit with the freshly generated config if service exists
    if [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
        if [ -f "$SERVICE_FILE" ]; then
            sudo systemctl daemon-reload
        fi
    fi

    local access_url
    if [ -n "${domain:-}" ]; then
        access_url="https://${domain}"
    else
        access_url="http://$(hostname -I | awk '{print $1}' 2>/dev/null || echo 'localhost'):8443"
    fi

    cat <<OUTRO

${GREEN}[INFO]${NC} DeployMonster ${VERSION} installed successfully.

  Config file:    ${DATA_DIR}/monster.yaml
  Admin email:    ${GENERATED_ADMIN_EMAIL:-admin@local.host}
  Admin password: ${GENERATED_ADMIN_PASSWORD:-<not set>}

  Access URL:     ${access_url}

OUTRO

    if [ -n "${domain:-}" ]; then
        cat <<SSLTIP
  SSL:            Automatic via Let's Encrypt (HTTP-01)
  Make sure:
    1. Domain A/AAAA record points to this server
    2. Ports 80 and 443 are open in your firewall

SSLTIP
    fi

    cat <<OUTRO
  Start the server:
    sudo systemctl start deploymonster

  Stop the server:
    sudo systemctl stop deploymonster

  View logs:
    sudo journalctl -u deploymonster -f

  Run setup wizard later:
    deploymonster setup

  Uninstall:
    curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh | bash -s -- uninstall

OUTRO

    if [ -t 0 ] && [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
        local start_now
        read -rp "Start DeployMonster now? [Y/n]: " start_now
        if [[ "${start_now:-Y}" =~ ^[Yy]$ ]]; then
            sudo systemctl start deploymonster
            info "DeployMonster is starting..."
            info "Open ${access_url} in your browser when ready."
        fi
    fi
}

main "$@"
