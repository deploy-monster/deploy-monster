#!/usr/bin/env bash
# DeployMonster v0.0.1 headless smoke-test runner
# Usage (on a clean Ubuntu 24.04 VM):
#   bash -c "$(curl -fsSL https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/smoke-test.sh)"
#
# Or if running locally from a cloned repo:
#   sudo bash scripts/smoke-test.sh

set -euo pipefail

INSTALLER_URL="${INSTALLER_URL:-https://raw.githubusercontent.com/deploy-monster/deploy-monster/v0.0.1/scripts/install.sh}"
MASTER_IP="${MASTER_IP:-}"

total=0
passed=0
failed=0

pass() { printf '  [PASS] %s\n' "$1"; ((passed++)); ((total++)); }
fail() { printf '  [FAIL] %s\n' "$1" >&2; ((failed++)); ((total++)); }

section() { printf '\n== %s ==\n' "$1"; }

# Retry a command up to N times with a sleep interval.
# Usage: retry <max_tries> <sleep_sec> <description> <command...>
retry() {
  local max_tries="$1"
  local sleep_sec="$2"
  local desc="$3"
  shift 3
  local i=1
  while [ "$i" -le "$max_tries" ]; do
    if "$@" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$sleep_sec"
    i=$((i + 1))
  done
  fail "$desc (timed out after $((max_tries * sleep_sec))s)"
  return 1
}

# ---------------------------------------------------------------------------
# 1. Preflight
# ---------------------------------------------------------------------------
section "Preflight"

for cmd in curl tar uname docker; do
  if command -v "$cmd" >/dev/null 2>&1; then
    pass "command exists: $cmd"
  else
    fail "missing command: $cmd"
  fi
done

if [ "${SMOKE_NO_SYSTEMD:-}" != "1" ]; then
  if command -v systemctl >/dev/null 2>&1; then
    pass "command exists: systemctl"
  else
    fail "missing command: systemctl (set SMOKE_NO_SYSTEMD=1 to skip systemd tests)"
  fi
fi

# ---------------------------------------------------------------------------
# 2. Installer (via raw GitHub staging URL)
# ---------------------------------------------------------------------------
section "Installer from $INSTALLER_URL"

# Download installer to temp and run it
tmp_dir=$(mktemp -d)
cleanup() {
  rm -rf "$tmp_dir"
  if [ -n "${DM_PID:-}" ] && kill -0 "$DM_PID" 2>/dev/null; then
    kill "$DM_PID" 2>/dev/null || true
    wait "$DM_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if [ -n "${INSTALLER_PATH:-}" ]; then
  cp "$INSTALLER_PATH" "$tmp_dir/install.sh"
else
  curl -fsSL "$INSTALLER_URL" -o "$tmp_dir/install.sh"
fi

# In non-interactive CI-like runs, pipe empty answers to accept defaults
if [ -t 0 ]; then
  bash "$tmp_dir/install.sh" --version=v0.0.1
else
  yes '' | bash "$tmp_dir/install.sh" --version=v0.0.1
fi

if [ "${SMOKE_NO_SYSTEMD:-}" != "1" ]; then
  if systemctl is-enabled deploymonster >/dev/null 2>&1; then
    pass "service enabled after install"
  else
    fail "service not enabled after install"
  fi
fi

version_output=$(/usr/local/bin/deploymonster version 2>/dev/null || true)
if [[ "$version_output" == *"0.0.1"* ]]; then
  pass "binary reports version 0.0.1"
else
  fail "binary version mismatch: $version_output"
fi

# Reinstall guard
if bash "$tmp_dir/install.sh" 2>&1 | grep -q "pass --force to overwrite"; then
  pass "reinstall guard fires without --force"
else
  fail "reinstall guard did not fire"
fi

bash "$tmp_dir/install.sh" --force
if [ "${SMOKE_NO_SYSTEMD:-}" != "1" ]; then
  if systemctl is-enabled deploymonster >/dev/null 2>&1; then
    pass "reinstall with --force succeeds"
  else
    fail "reinstall with --force failed"
  fi
fi

# Uninstall
bash "$tmp_dir/install.sh" uninstall
if [[ ! -x /usr/local/bin/deploymonster ]]; then
  pass "binary removed after uninstall"
else
  fail "binary still exists after uninstall"
fi

if [[ -d /var/lib/deploymonster ]]; then
  pass "data dir preserved after uninstall"
else
  fail "data dir missing after uninstall"
fi

# Re-install for further tests
if [ -t 0 ]; then
  bash "$tmp_dir/install.sh" --force
else
  yes '' | bash "$tmp_dir/install.sh" --force
fi

# ---------------------------------------------------------------------------
# 3. Service boots and health endpoint responds
# ---------------------------------------------------------------------------
section "Service boot"

DM_PID=""
if [ "${SMOKE_NO_SYSTEMD:-}" = "1" ]; then
  pass "Running deploymonster directly (no systemd)"
  cd /var/lib/deploymonster
  nohup /usr/local/bin/deploymonster serve >/var/lib/deploymonster/deploymonster.log 2>&1 &
  DM_PID=$!
  sleep 1
else
  systemctl start deploymonster
fi

# Wait for the health endpoint with retries
health_code=""
for i in {1..30}; do
  health_code=$(curl -sk -o /dev/null -w "%{http_code}" https://localhost:8443/api/v1/health || true)
  if [[ "$health_code" == "200" ]]; then
    break
  fi
  sleep 2
done

if [[ "$health_code" == "200" ]]; then
  pass "health endpoint returns 200"
else
  fail "health endpoint returned ${health_code:-<empty>}"
fi

# ---------------------------------------------------------------------------
# 4. Auth register + login
# ---------------------------------------------------------------------------
section "Auth"

register_resp=$(curl -sk -X POST https://localhost:8443/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"smoke@test.local","password":"SmokeTest-2026!","tenant_name":"smoke-tenant"}' || true)

if [[ "$register_resp" == *"email"* ]]; then
  pass "auth register succeeds"
else
  fail "auth register failed: $register_resp"
fi

login_resp=$(curl -sk -X POST https://localhost:8443/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"smoke@test.local","password":"SmokeTest-2026!"}' || true)

TOKEN=$(echo "$login_resp" | grep -oP '"access_token":\s*"\K[^"]+' || true)
if [[ -n "$TOKEN" ]]; then
  pass "auth login returns access token"
else
  fail "auth login failed: $login_resp"
fi

# ---------------------------------------------------------------------------
# 5. Deploy a minimal Node app (via example repo)
# ---------------------------------------------------------------------------
section "App deploy"

if [[ -n "$TOKEN" ]]; then
  app_resp=$(curl -sk -X POST https://localhost:8443/api/v1/apps \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d '{"name":"smoke-node","git_url":"https://github.com/deploy-monster/example-node.git","branch":"main"}' || true)

  APP_ID=$(echo "$app_resp" | grep -oP '"id":\s*"\K[^"]+' | head -n1 || true)
  if [[ -n "$APP_ID" ]]; then
    pass "app created"
  else
    fail "app creation failed: $app_resp"
  fi

  if [[ -n "$APP_ID" ]]; then
    deploy_resp=$(curl -sk -X POST "https://localhost:8443/api/v1/apps/$APP_ID/deploy" \
      -H "Authorization: Bearer $TOKEN" || true)

    # Wait up to 120s for container to appear (build + start may be slow on small VMs)
    containers=0
    for i in {1..60}; do
      containers=$(docker ps --format "{{.Names}}" | grep -c "^smoke-node$" || true)
      if [[ "$containers" -gt 0 ]]; then
        break
      fi
      sleep 2
    done

    if [[ "$containers" -gt 0 ]]; then
      pass "container running after deploy"
    else
      fail "container did not start within 120s"
    fi

    # Teardown
    curl -sk -X DELETE "https://localhost:8443/api/v1/apps/$APP_ID" \
      -H "Authorization: Bearer $TOKEN" >/dev/null 2>&1 || true
    sleep 2
    remaining=$(docker ps --format "{{.Names}}" | grep -c "^smoke-node$" || true)
    if [[ "$remaining" -eq 0 ]]; then
      pass "app teardown removes container"
    else
      fail "container still running after teardown"
    fi
  fi
else
  fail "skipping app deploy — no auth token"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
section "Summary"
printf "Total:  %d\nPassed: %d\nFailed: %d\n" "$total" "$passed" "$failed"

if [[ "$failed" -gt 0 ]]; then
  exit 1
fi
