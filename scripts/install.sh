#!/usr/bin/env bash
# DeployMonster installer — curl-pipe entry point (GitHub-first)
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.2.0/scripts/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.2.0/scripts/install.sh | bash -s -- --version=v0.2.0
#   curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.2.0/scripts/install.sh | bash -s -- --force
#   curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.2.0/scripts/install.sh | MONSTER_JOIN_TOKEN=join-token bash -s -- --version=v0.2.0
#   curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.2.0/scripts/install.sh | bash -s -- --agent --master=http://master:8443 --token=join-token
#   curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.2.0/scripts/install.sh | bash -s -- uninstall
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
ENV_DIR="/etc/deploymonster"
ENV_FILE="${ENV_DIR}/deploymonster.env"

MODE="install"
ROLE="server"
VERSION=""
FORCE=0
AGENT_MASTER_URL="${MONSTER_MASTER_URL:-}"
AGENT_JOIN_TOKEN="${MONSTER_JOIN_TOKEN:-}"
AGENT_SERVER_ID="${MONSTER_SERVER_ID:-}"
AGENT_MASTER_PORT="${MONSTER_MASTER_PORT:-}"

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

systemd_quote() {
    printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

# ─── Arg parsing ────────────────────────────────────────────────────────────
parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            install|uninstall) MODE="$1" ;;
            --agent) ROLE="agent" ;;
            --server) ROLE="server" ;;
            --master=*) AGENT_MASTER_URL="${1#--master=}" ;;
            --master) [ $# -gt 1 ] || error "--master requires a value"; shift; AGENT_MASTER_URL="$1" ;;
            --token=*) AGENT_JOIN_TOKEN="${1#--token=}" ;;
            --token) [ $# -gt 1 ] || error "--token requires a value"; shift; AGENT_JOIN_TOKEN="$1" ;;
            --server-id=*) AGENT_SERVER_ID="${1#--server-id=}" ;;
            --server-id) [ $# -gt 1 ] || error "--server-id requires a value"; shift; AGENT_SERVER_ID="$1" ;;
            --master-port=*) AGENT_MASTER_PORT="${1#--master-port=}" ;;
            --master-port) [ $# -gt 1 ] || error "--master-port requires a value"; shift; AGENT_MASTER_PORT="$1" ;;
            --version=*) VERSION="${1#--version=}" ;;
            --version) [ $# -gt 1 ] || error "--version requires a value"; shift; VERSION="$1" ;;
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

    # The platform API/UI listener is plain HTTP on :8443. The ingress
    # listeners on :80/:443 are for deployed applications and ACME.
    local server_port=8443

    # Generate secret key if not provided
    local secret_key="${MONSTER_SECRET:-${MONSTER_SECRET_KEY:-}}"
    if [ -z "$secret_key" ]; then
        secret_key=$(openssl rand -hex 32 2>/dev/null || tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 64 || echo "")
    fi

    # Detect IP for CORS
    local server_ip=$(hostname -I | awk '{print $1}' 2>/dev/null || echo "localhost")
    local cors_origin
    if [ -n "$domain" ]; then
        cors_origin="http://${domain}:${server_port}"
    else
        cors_origin="http://${server_ip}:${server_port}"
    fi

    local sudo_cmd=""
    if [ ! -d "$DATA_DIR" ]; then
        if [ ! -w "$(dirname "$DATA_DIR")" ]; then
            require_cmd sudo
            sudo_cmd="sudo"
        fi
        $sudo_cmd mkdir -p "$DATA_DIR"
        $sudo_cmd chmod 750 "$DATA_DIR"
    elif [ ! -w "$DATA_DIR" ]; then
        require_cmd sudo
        sudo_cmd="sudo"
    fi

    $sudo_cmd tee "$config_path" > /dev/null <<EOF
server:
  host: 0.0.0.0
  port: ${server_port}
  domain: "${domain}"
  secret_key: "${secret_key}"
  cors_origins: "${cors_origin}"

database:
  driver: sqlite
  path: deploymonster.db

ingress:
  http_port: 80
  https_port: 443
  # Application ingress. The platform UI itself is served on server.port.
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

    $sudo_cmd chmod 600 "$config_path"
    info "Config written to ${config_path}"

    GENERATED_ADMIN_EMAIL="$admin_email"
    GENERATED_ADMIN_PASSWORD="$admin_password"
}

write_admin_env_file() {
    local sudo_cmd="sudo"
    require_cmd sudo
    local join_token="${MONSTER_JOIN_TOKEN:-$AGENT_JOIN_TOKEN}"

    $sudo_cmd mkdir -p "$ENV_DIR"
    $sudo_cmd chmod 700 "$ENV_DIR"
    {
        printf 'MONSTER_ADMIN_EMAIL=%s\n' "$(systemd_quote "${GENERATED_ADMIN_EMAIL:-admin@local.host}")"
        printf 'MONSTER_ADMIN_PASSWORD=%s\n' "$(systemd_quote "${GENERATED_ADMIN_PASSWORD:-}")"
        if [ -n "${MONSTER_BUILD_IMAGE_REGISTRY:-}" ]; then
            printf 'MONSTER_BUILD_IMAGE_REGISTRY=%s\n' "$(systemd_quote "${MONSTER_BUILD_IMAGE_REGISTRY}")"
        fi
        if [ -n "${MONSTER_BUILD_IMAGE_PUSH:-}" ]; then
            printf 'MONSTER_BUILD_IMAGE_PUSH=%s\n' "$(systemd_quote "${MONSTER_BUILD_IMAGE_PUSH}")"
        fi
        if [ -n "${MONSTER_BUILD_REGISTRY_USERNAME:-}" ]; then
            printf 'MONSTER_BUILD_REGISTRY_USERNAME=%s\n' "$(systemd_quote "${MONSTER_BUILD_REGISTRY_USERNAME}")"
        fi
        if [ -n "${MONSTER_BUILD_REGISTRY_PASSWORD:-}" ]; then
            printf 'MONSTER_BUILD_REGISTRY_PASSWORD=%s\n' "$(systemd_quote "${MONSTER_BUILD_REGISTRY_PASSWORD}")"
        fi
        if [ -n "$join_token" ]; then
            printf 'MONSTER_JOIN_TOKEN=%s\n' "$(systemd_quote "${join_token}")"
        fi
    } | $sudo_cmd tee "$ENV_FILE" > /dev/null
    $sudo_cmd chmod 600 "$ENV_FILE"
    info "Admin bootstrap credentials written to ${ENV_FILE}"
}

generate_agent_config() {
    local config_path="${DATA_DIR}/monster.yaml"
    local sudo_cmd=""

    if [ -z "$AGENT_MASTER_URL" ]; then
        if [ -t 0 ]; then
            read -rp "Master URL (e.g., http://master.example.com:8443): " AGENT_MASTER_URL
        fi
    fi
    if [ -z "$AGENT_JOIN_TOKEN" ]; then
        if [ -t 0 ]; then
            read -rsp "Agent join token: " AGENT_JOIN_TOKEN
            echo >&2
        fi
    fi
    if [ -z "$AGENT_SERVER_ID" ] && [ -t 0 ]; then
        local default_id
        default_id="$(hostname 2>/dev/null || echo "")"
        read -rp "Agent server ID [${default_id}]: " AGENT_SERVER_ID
        AGENT_SERVER_ID="${AGENT_SERVER_ID:-$default_id}"
    fi

    [ -n "$AGENT_MASTER_URL" ] || error "Agent install requires --master or MONSTER_MASTER_URL"
    [ -n "$AGENT_JOIN_TOKEN" ] || error "Agent install requires --token or MONSTER_JOIN_TOKEN"

    if [ -f "$config_path" ] && [ "$FORCE" -ne 1 ]; then
        warn "Configuration already exists at ${config_path}"
        warn "Pass --force to regenerate it during install"
        return
    fi

    if [ ! -d "$DATA_DIR" ]; then
        if [ ! -w "$(dirname "$DATA_DIR")" ]; then
            require_cmd sudo
            sudo_cmd="sudo"
        fi
        $sudo_cmd mkdir -p "$DATA_DIR"
        $sudo_cmd chmod 750 "$DATA_DIR"
    elif [ ! -w "$DATA_DIR" ]; then
        require_cmd sudo
        sudo_cmd="sudo"
    fi

    $sudo_cmd tee "$config_path" > /dev/null <<EOF
server:
  host: 127.0.0.1
  port: 8443

docker:
  host: unix:///var/run/docker.sock
EOF

    $sudo_cmd chmod 600 "$config_path"
    info "Agent config written to ${config_path}"
}

write_agent_env_file() {
    local sudo_cmd="sudo"
    require_cmd sudo

    $sudo_cmd mkdir -p "$ENV_DIR"
    $sudo_cmd chmod 700 "$ENV_DIR"
    {
        printf 'MONSTER_MASTER_URL=%s\n' "$(systemd_quote "${AGENT_MASTER_URL}")"
        printf 'MONSTER_JOIN_TOKEN=%s\n' "$(systemd_quote "${AGENT_JOIN_TOKEN}")"
        if [ -n "$AGENT_SERVER_ID" ]; then
            printf 'MONSTER_SERVER_ID=%s\n' "$(systemd_quote "${AGENT_SERVER_ID}")"
        fi
        if [ -n "$AGENT_MASTER_PORT" ]; then
            printf 'MONSTER_MASTER_PORT=%s\n' "$(systemd_quote "${AGENT_MASTER_PORT}")"
        fi
        if [ -n "${MONSTER_BUILD_REGISTRY_USERNAME:-}" ]; then
            printf 'MONSTER_BUILD_REGISTRY_USERNAME=%s\n' "$(systemd_quote "${MONSTER_BUILD_REGISTRY_USERNAME}")"
        fi
        if [ -n "${MONSTER_BUILD_REGISTRY_PASSWORD:-}" ]; then
            printf 'MONSTER_BUILD_REGISTRY_PASSWORD=%s\n' "$(systemd_quote "${MONSTER_BUILD_REGISTRY_PASSWORD}")"
        fi
    } | $sudo_cmd tee "$ENV_FILE" > /dev/null
    $sudo_cmd chmod 600 "$ENV_FILE"
    info "Agent connection settings written to ${ENV_FILE}"
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

    local exec_start="/usr/local/bin/deploymonster serve"
    local service_description="DeployMonster PaaS"
    if [ "$ROLE" = "agent" ]; then
        write_agent_env_file
        exec_start="/usr/local/bin/deploymonster serve --agent"
        service_description="DeployMonster Agent"
    else
        write_admin_env_file
    fi

    # Build unit file dynamically.
    local unit_content=""
    unit_content="[Unit]
Description=${service_description}
Documentation=https://github.com/deploy-monster/deploy-monster
After=network-online.target docker.service
Wants=network-online.target docker.service

[Service]
Type=simple
ExecStart=${exec_start}
EnvironmentFile=-${ENV_FILE}
Restart=always
RestartSec=5
WorkingDirectory=/var/lib/deploymonster
LimitNOFILE=65536"

    unit_content="${unit_content}
# Do not set User= — the daemon needs access to /var/run/docker.sock and
# to bind :80/:443 on first-run ACME. Users who want a dedicated system
# account should adjust this unit and add that account to the \`docker\`
# group manually.

[Install]
WantedBy=multi-user.target"

    printf '%s\n' "$unit_content" | sudo tee "$SERVICE_FILE" > /dev/null
    sudo chmod 644 "$SERVICE_FILE"
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
    if [ "$ROLE" = "agent" ]; then
        generate_agent_config
    else
        generate_config
    fi
    setup_service

    # Reload systemd unit with the freshly generated config if service exists
    if [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
        if [ -f "$SERVICE_FILE" ]; then
            sudo systemctl daemon-reload
        fi
    fi

    local access_url
    if [ -n "${domain:-}" ]; then
        access_url="http://${domain}:8443"
    else
        access_url="http://$(hostname -I | awk '{print $1}' 2>/dev/null || echo 'localhost'):8443"
    fi

    cat <<OUTRO

${GREEN}[INFO]${NC} DeployMonster ${VERSION} installed successfully.

  Config file:    ${DATA_DIR}/monster.yaml
OUTRO

    if [ "$ROLE" = "agent" ]; then
        cat <<OUTRO
  Role:           agent
  Master URL:     ${AGENT_MASTER_URL}
  Server ID:      ${AGENT_SERVER_ID:-<hostname>}

OUTRO
    else
        cat <<OUTRO
  Admin email:    ${GENERATED_ADMIN_EMAIL:-admin@local.host}
  Admin password: ${GENERATED_ADMIN_PASSWORD:-<not set>}

  Access URL:     ${access_url}

OUTRO

    fi

    if [ "$ROLE" != "agent" ] && [ -n "${domain:-}" ]; then
        cat <<SSLTIP
  SSL:            Automatic via Let's Encrypt (HTTP-01)
  Make sure:
    1. Domain A/AAAA record points to this server
    2. Ports 80 and 443 are open in your firewall

SSLTIP
    fi

    local service_label="server"
    if [ "$ROLE" = "agent" ]; then
        service_label="agent"
    fi

    cat <<OUTRO
  Start the ${service_label}:
    sudo systemctl start deploymonster

  Stop the ${service_label}:
    sudo systemctl stop deploymonster

  View logs:
    sudo journalctl -u deploymonster -f
OUTRO

    if [ "$ROLE" != "agent" ]; then
        cat <<OUTRO
  Run setup wizard later:
    deploymonster setup

OUTRO
    fi

    cat <<OUTRO
  Uninstall:
    curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.2.0/scripts/install.sh | bash -s -- uninstall

OUTRO

    if [ -t 0 ] && [ "$OS" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
        local start_now
        read -rp "Start DeployMonster now? [Y/n]: " start_now
        if [[ "${start_now:-Y}" =~ ^[Yy]$ ]]; then
            sudo systemctl start deploymonster
            info "DeployMonster ${service_label} is starting..."
            if [ "$ROLE" != "agent" ]; then
                info "Open ${access_url} in your browser when ready."
            fi
        fi
    fi
}

main "$@"
