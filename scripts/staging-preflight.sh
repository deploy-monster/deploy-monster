#!/usr/bin/env bash
set -euo pipefail

# Pre-flight checks for docs/staging-validation.md.
#
# Required:
#   STAGING_SSH_HOST=user@host
#   STAGING_BASE_URL=https://staging.example.com
#
# Optional:
#   STAGING_TEST_APP_DOMAIN=test-123.staging.example.com
#   STAGING_DATA_DIR=/var/lib/deploymonster
#   STAGING_MIN_FREE_GB=10

SSH_HOST="${STAGING_SSH_HOST:-}"
BASE_URL="${STAGING_BASE_URL:-}"
TEST_APP_DOMAIN="${STAGING_TEST_APP_DOMAIN:-}"
DATA_DIR="${STAGING_DATA_DIR:-/var/lib/deploymonster}"
MIN_FREE_GB="${STAGING_MIN_FREE_GB:-10}"

fail() {
  printf '[FAIL] %s\n' "$1" >&2
  exit 1
}

pass() {
  printf '[PASS] %s\n' "$1"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

host_from_url() {
  printf '%s\n' "$1" |
    sed -E 's#^[a-zA-Z][a-zA-Z0-9+.-]*://##; s#/.*$##; s#:[0-9]+$##'
}

require_cmd ssh
require_cmd awk
require_cmd sed
require_cmd dig

[ -n "$SSH_HOST" ] || fail "STAGING_SSH_HOST is required"
[ -n "$BASE_URL" ] || fail "STAGING_BASE_URL is required"
case "$MIN_FREE_GB" in
  ''|*[!0-9]*) fail "STAGING_MIN_FREE_GB must be a positive integer" ;;
esac
[ "$MIN_FREE_GB" -gt 0 ] || fail "STAGING_MIN_FREE_GB must be greater than zero"

BASE_HOST="$(host_from_url "$BASE_URL")"
[ -n "$BASE_HOST" ] || fail "could not parse host from STAGING_BASE_URL=$BASE_URL"

printf 'Staging pre-flight target:\n'
printf '  ssh:       %s\n' "$SSH_HOST"
printf '  base url:  %s\n' "$BASE_URL"
printf '  base host: %s\n' "$BASE_HOST"
printf '  data dir:  %s\n' "$DATA_DIR"

if dig +short "$BASE_HOST" | grep -q .; then
  pass "DNS resolves for $BASE_HOST"
else
  fail "DNS does not resolve for $BASE_HOST"
fi

if [ -n "$TEST_APP_DOMAIN" ]; then
  if dig +short "$TEST_APP_DOMAIN" | grep -q .; then
    pass "DNS resolves for $TEST_APP_DOMAIN"
  else
    fail "DNS does not resolve for $TEST_APP_DOMAIN"
  fi
else
  printf '[WARN] STAGING_TEST_APP_DOMAIN not set; skipping test app DNS check\n'
fi

ssh "$SSH_HOST" \
  DATA_DIR="$DATA_DIR" \
  MIN_FREE_GB="$MIN_FREE_GB" \
  'sh -s' <<'REMOTE'
set -eu

data_dir="${DATA_DIR:-/var/lib/deploymonster}"
min_free_gb="${MIN_FREE_GB:-10}"

if [ -d "$data_dir" ]; then
  check_path="$data_dir"
  used_kb="$(du -sk "$data_dir" 2>/dev/null | awk '{print $1}')"
else
  check_path="/"
  used_kb=0
fi

avail_kb="$(df -Pk "$check_path" | awk 'NR == 2 {print $4}')"
min_free_kb=$((min_free_gb * 1024 * 1024))
restore_required_kb=$((used_kb * 3))
if [ "$restore_required_kb" -lt "$min_free_kb" ]; then
  required_kb="$min_free_kb"
else
  required_kb="$restore_required_kb"
fi

printf 'Remote disk check:\n'
printf '  check path:      %s\n' "$check_path"
printf '  data used KiB:   %s\n' "$used_kb"
printf '  available KiB:   %s\n' "$avail_kb"
printf '  required KiB:    %s\n' "$required_kb"

if [ "$avail_kb" -lt "$required_kb" ]; then
  printf '[FAIL] available disk is below restore-drill requirement\n' >&2
  exit 1
fi

if command -v docker >/dev/null 2>&1; then
  docker --version
else
  printf '[FAIL] docker is not installed on staging host\n' >&2
  exit 1
fi

printf '[PASS] remote staging host disk and Docker checks passed\n'
REMOTE

pass "staging pre-flight checks passed"
