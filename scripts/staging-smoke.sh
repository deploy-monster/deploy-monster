#!/usr/bin/env bash
set -euo pipefail

# Lightweight staging smoke test for an already-running DeployMonster instance.
#
# Required unless DM_SMOKE_PUBLIC_ONLY=1:
#   DM_SMOKE_EMAIL / STAGING_SMOKE_EMAIL
#   DM_SMOKE_PASSWORD / STAGING_SMOKE_PASSWORD
#
# Optional:
#   STAGING_BASE_URL=http://staging.example.com
#   DM_SMOKE_INSECURE_TLS=1

BASE_URL="${STAGING_BASE_URL:-${1:-http://localhost:8443}}"
BASE_URL="${BASE_URL%/}"
PUBLIC_ONLY="${DM_SMOKE_PUBLIC_ONLY:-0}"
INSECURE_TLS="${DM_SMOKE_INSECURE_TLS:-0}"
EMAIL="${DM_SMOKE_EMAIL:-${STAGING_SMOKE_EMAIL:-}}"
PASSWORD="${DM_SMOKE_PASSWORD:-${STAGING_SMOKE_PASSWORD:-}}"

curl_args=(-fsS --connect-timeout 5 --max-time 20)
if [ "$INSECURE_TLS" = "1" ]; then
  curl_args+=(-k)
fi

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

total=0
failed=0

pass() {
  total=$((total + 1))
  printf '[PASS] %s\n' "$1"
}

fail() {
  total=$((total + 1))
  failed=$((failed + 1))
  printf '[FAIL] %s\n' "$1" >&2
}

request() {
  local method="$1"
  local path="$2"
  local out="$3"
  shift 3

  curl "${curl_args[@]}" \
    -X "$method" \
    -o "$out" \
    -w '%{http_code}' \
    "$@" \
    "$BASE_URL$path" || true
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

expect_status() {
  local method="$1"
  local path="$2"
  local expected="$3"
  local label="$4"
  shift 4

  local body="$tmp_dir/${label//[^a-zA-Z0-9]/_}.json"
  local code
  code="$(request "$method" "$path" "$body" "$@")"
  if [ "$code" = "$expected" ]; then
    pass "$label"
  else
    fail "$label returned HTTP ${code:-<empty>}: $(head -c 300 "$body" 2>/dev/null || true)"
  fi
}

echo "Staging smoke target: $BASE_URL"

expect_status GET /health 200 "public health"
expect_status GET /api/v1/health 200 "api health"
expect_status GET /api/v1/openapi.json 200 "openapi spec"
expect_status GET /api/v1/marketplace 200 "marketplace list"

if [ "$PUBLIC_ONLY" = "1" ]; then
  echo "Public-only mode enabled; skipping authenticated checks."
else
  if [ -z "$EMAIL" ] || [ -z "$PASSWORD" ]; then
    fail "missing DM_SMOKE_EMAIL/DM_SMOKE_PASSWORD for authenticated checks"
  else
    email_json="$(json_escape "$EMAIL")"
    password_json="$(json_escape "$PASSWORD")"
    login_body="$tmp_dir/login.json"
    login_code="$(
      curl "${curl_args[@]}" \
        -X POST \
        -H 'Content-Type: application/json' \
        -d "{\"email\":\"$email_json\",\"password\":\"$password_json\"}" \
        -o "$login_body" \
        -w '%{http_code}' \
        "$BASE_URL/api/v1/auth/login" || true
    )"

    if [ "$login_code" = "200" ]; then
      token="$(sed -n 's/.*"access_token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$login_body" | head -n1)"
      if [ -n "$token" ]; then
        pass "auth login"
        expect_status GET /api/v1/auth/me 200 "current user" -H "Authorization: Bearer $token"
        expect_status GET /api/v1/apps 200 "apps list" -H "Authorization: Bearer $token"
      else
        fail "auth login response missing access_token"
      fi
    else
      fail "auth login returned HTTP ${login_code:-<empty>}: $(head -c 300 "$login_body" 2>/dev/null || true)"
    fi
  fi
fi

echo "Smoke summary: $((total - failed))/${total} passed"
if [ "$failed" -gt 0 ]; then
  exit 1
fi
