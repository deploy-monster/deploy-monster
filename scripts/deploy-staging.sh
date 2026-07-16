#!/usr/bin/env bash
# DeployMonster staging deploy script
# Usage:
#   export STAGING_SSH_HOST=deploymonster@staging.example.com
#   export STAGING_DOMAIN=staging.example.com
#   export STAGING_ADMIN_EMAIL=admin@example.com
#   ./scripts/deploy-staging.sh
#
# Optional:
#   STAGING_LETSENCRYPT_EMAIL=admin@example.com   # for real TLS cert
#   STAGING_ACME_STAGING=true                        # use LE staging CA (default: true)
#   MONSTER_SECRET=<secret>                          # server secret key
set -euo pipefail

SSH_HOST="${STAGING_SSH_HOST:-}"
DOMAIN="${STAGING_DOMAIN:-}"
ADMIN_EMAIL="${STAGING_ADMIN_EMAIL:-}"
LETSENCRYPT_EMAIL="${STAGING_LETSENCRYPT_EMAIL:-$ADMIN_EMAIL}"
ACME_STAGING="${STAGING_ACME_STAGING:-true}"
SECRET="${MONSTER_SECRET:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
VERSION="$(cat "$ROOT_DIR/VERSION" 2>/dev/null || echo "dev")"
COMMIT="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo "none")"

RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
pass() { printf "${GREEN}[PASS]${NC} %s\n" "$1"; }
fail() { printf "${RED}[FAIL]${NC} %s\n" "$1" >&2; exit 1; }

# ─── Input validation ────────────────────────────────────────────────────────
[ -n "$SSH_HOST" ]  || fail "STAGING_SSH_HOST is required (user@host)"
[ -n "$DOMAIN" ]    || fail "STAGING_DOMAIN is required"
[ -n "$ADMIN_EMAIL" ] || fail "STAGING_ADMIN_EMAIL is required"

echo "============================================"
echo "  DeployMonster Staging Deploy v$VERSION"
echo "  Target:  $SSH_HOST"
echo "  Domain:  $DOMAIN"
echo "============================================"
echo ""

# ─── Step 1: Pre-flight checks ──────────────────────────────────────────────
echo "==> Step 1/6: Pre-flight checks"

command -v ssh >/dev/null 2>&1  || fail "ssh is required"
command -v dig >/dev/null 2>&1  || fail "dig is required"
command -v go >/dev/null 2>&1   || fail "go is required"

# DNS check
if dig +short "$DOMAIN" | grep -q .; then
  STAGING_IP="$(dig +short "$DOMAIN" | head -1)"
  pass "DNS resolves: $DOMAIN → $STAGING_IP"
else
  fail "DNS does not resolve for $DOMAIN — add an A record first"
fi

# SSH check
if ssh -o ConnectTimeout=5 -o BatchMode=yes "$SSH_HOST" 'echo ok' 2>/dev/null; then
  pass "SSH reachable: $SSH_HOST"
else
  fail "SSH not reachable — check your SSH key and host"
fi

# Docker check
ssh "$SSH_HOST" 'docker --version' >/dev/null 2>&1 || fail "Docker not installed on staging host"
pass "Docker installed on staging host"

# ─── Step 2: Build the binary ────────────────────────────────────────────────
echo "==> Step 2/6: Building DeployMonster binary"
cd "$ROOT_DIR"
scripts/build.sh
BINARY="$ROOT_DIR/bin/deploymonster"
[ -f "$BINARY" ] || fail "Build failed: $BINARY not found"
pass "Binary built: $($BINARY version 2>&1 | head -1)"

# ─── Step 3: Copy binary and config to staging host ──────────────────────────
echo "==> Step 3/6: Deploying to staging host"

ssh "$SSH_HOST" 'sudo mkdir -p /usr/local/bin /etc/deploymonster /var/lib/deploymonster'
scp "$BINARY" "$SSH_HOST:/tmp/deploymonster"
ssh "$SSH_HOST" "sudo mv /tmp/deploymonster /usr/local/bin/deploymonster && sudo chmod 755 /usr/local/bin/deploymonster"
pass "Binary installed on staging host"

# ─── Step 4: Generate config ─────────────────────────────────────────────────
echo "==> Step 4/6: Generating configuration"

# Generate a secret key if not provided
if [ -z "$SECRET" ]; then
  SECRET="$(openssl rand -hex 32)"
fi

# Write config file via SSH
ssh "$SSH_HOST" "sudo tee /etc/deploymonster/monster.yaml > /dev/null" <<CONFIG
server:
  host: 0.0.0.0
  port: 8443
  domain: "${DOMAIN}"
  secret_key: "${SECRET}"
  rate_limit_per_minute: 120

database:
  driver: sqlite
  path: /var/lib/deploymonster/deploymonster.db

ingress:
  http_port: 80
  https_port: 443
  enable_https: true

acme:
  email: "${LETSENCRYPT_EMAIL}"
  staging: ${ACME_STAGING}
  provider: http-01

dns:
  provider: manual

docker:
  host: unix:///var/run/docker.sock

registration:
  mode: open

backup:
  encryption: true
  storage_path: /var/lib/deploymonster/backups
CONFIG
pass "Configuration generated"

# ─── Step 5: Setup systemd service ────────────────────────────────────────────
echo "==> Step 5/6: Setting up systemd service"

scp "$ROOT_DIR/deployments/deploymonster.service" "$SSH_HOST:/tmp/deploymonster.service"
ssh "$SSH_HOST" <<'SERVICE'
  sudo mv /tmp/deploymonster.service /etc/systemd/system/deploymonster.service
  sudo systemctl daemon-reload
  sudo systemctl enable deploymonster
SERVICE
pass "systemd service installed"

# ─── Step 6: First run (setup) ───────────────────────────────────────────────
echo "==> Step 6/6: First run setup"

# Run setup non-interactively by setting env vars
ssh "$SSH_HOST" <<SETUP
  sudo -E MONSTER_SECRET="${SECRET}" /usr/local/bin/deploymonster setup <<<EOF
${ADMIN_EMAIL}
${ADMIN_EMAIL}
y
EOF
SETUP

# Start the service
ssh "$SSH_HOST" 'sudo systemctl restart deploymonster'
sleep 3

# Health check
HEALTH=$(ssh "$SSH_HOST" 'curl -fsS -o /dev/null -w "%{http_code}" http://localhost:8443/health 2>/dev/null || echo "fail"')
if [ "$HEALTH" = "200" ]; then
  pass "DeployMonster is running! HTTP 200 on /health"
else
  ssh "$SSH_HOST" 'sudo journalctl -u deploymonster -n 50 --no-pager' || true
  fail "Health check failed with HTTP $HEALTH — check logs above"
fi

# ─── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo "============================================"
echo "  ✅ Staging deployment complete!"
echo "============================================"
echo ""
echo "  URL:        https://${DOMAIN}"
echo "  Health:     https://${DOMAIN}/health"
echo "  API:        https://${DOMAIN}/api/v1/health"
echo "  Admin:      https://${DOMAIN}/login"
echo ""
echo "  Next: run smoke tests:"
echo "    STAGING_BASE_URL=https://${DOMAIN} \\"
echo "    DM_SMOKE_EMAIL=<admin-email> \\"
echo "    DM_SMOKE_PASSWORD=<admin-password> \\"
echo "    DM_SMOKE_INSECURE_TLS=1 \\"
echo "    ./scripts/staging-smoke.sh"
echo ""
