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
#   DM_SMOKE_REQUIRE_BASE_URL=1
#   DM_SMOKE_REQUIRE_AGENT=1
#   DM_SMOKE_AGENT_ID=agent-1
#   DM_SMOKE_REMOTE_SERVER_ID=agent-1
#   DM_SMOKE_REMOTE_IMAGE=nginx:alpine

if [ "${DM_SMOKE_REQUIRE_BASE_URL:-0}" = "1" ] && [ -z "${STAGING_BASE_URL:-}" ] && [ -z "${1:-}" ]; then
  echo "STAGING_BASE_URL or first argument is required when DM_SMOKE_REQUIRE_BASE_URL=1" >&2
  exit 2
fi

BASE_URL="${STAGING_BASE_URL:-${1:-http://localhost:8443}}"
BASE_URL="${BASE_URL%/}"
PUBLIC_ONLY="${DM_SMOKE_PUBLIC_ONLY:-0}"
INSECURE_TLS="${DM_SMOKE_INSECURE_TLS:-0}"
EMAIL="${DM_SMOKE_EMAIL:-${STAGING_SMOKE_EMAIL:-}}"
PASSWORD="${DM_SMOKE_PASSWORD:-${STAGING_SMOKE_PASSWORD:-}}"
REQUIRE_AGENT="${DM_SMOKE_REQUIRE_AGENT:-0}"
AGENT_ID="${DM_SMOKE_AGENT_ID:-}"
REMOTE_SERVER_ID="${DM_SMOKE_REMOTE_SERVER_ID:-}"
REMOTE_IMAGE="${DM_SMOKE_REMOTE_IMAGE:-${DM_SMOKE_IMAGE:-}}"

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

expect_status_any() {
  local method="$1"
  local path="$2"
  local expected_csv="$3"
  local label="$4"
  shift 4

  local body="$tmp_dir/${label//[^a-zA-Z0-9]/_}.json"
  local code
  code="$(request "$method" "$path" "$body" "$@")"
  case ",$expected_csv," in
    *,"$code",*) pass "$label" ;;
    *) fail "$label returned HTTP ${code:-<empty>}, expected one of $expected_csv: $(head -c 300 "$body" 2>/dev/null || true)" ;;
  esac
}

json_field() {
  local field="$1"
  local file="$2"
  sed -n "s/.*\"$field\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p" "$file" | head -n1
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
        agents_body="$tmp_dir/agents.json"
        agents_code="$(request GET /api/v1/agents "$agents_body" -H "Authorization: Bearer $token")"
        if [ "$agents_code" = "200" ]; then
          pass "agents list"
          if [ "$REQUIRE_AGENT" = "1" ]; then
            if [ -n "$AGENT_ID" ]; then
              if grep -q "\"server_id\"[[:space:]]*:[[:space:]]*\"$AGENT_ID\"" "$agents_body"; then
                pass "required agent $AGENT_ID connected"
              else
                fail "required agent $AGENT_ID not present in agents list"
              fi
            elif grep -E '"server_id"[[:space:]]*:[[:space:]]*"[^"]+"' "$agents_body" | grep -vq '"local"'; then
              pass "at least one remote agent connected"
            else
              fail "no remote agent present in agents list"
            fi
          fi
        else
          fail "agents list returned HTTP ${agents_code:-<empty>}: $(head -c 300 "$agents_body" 2>/dev/null || true)"
        fi

        if [ -n "$REMOTE_SERVER_ID" ] || [ -n "$REMOTE_IMAGE" ]; then
          if [ -z "$REMOTE_SERVER_ID" ] || [ -z "$REMOTE_IMAGE" ]; then
            fail "remote deploy smoke requires both DM_SMOKE_REMOTE_SERVER_ID and DM_SMOKE_REMOTE_IMAGE"
          else
            app_name="dm-smoke-$(date +%s)"
            app_body="$tmp_dir/remote_app.json"
            app_code="$(
              curl "${curl_args[@]}" \
                -X POST \
                -H 'Content-Type: application/json' \
                -H "Authorization: Bearer $token" \
                -d "{\"name\":\"$app_name\",\"source_type\":\"image\",\"source_url\":\"$(json_escape "$REMOTE_IMAGE")\",\"server_id\":\"$(json_escape "$REMOTE_SERVER_ID")\"}" \
                -o "$app_body" \
                -w '%{http_code}' \
                "$BASE_URL/api/v1/apps" || true
            )"
            if [ "$app_code" = "201" ]; then
              pass "remote image app create"
              app_id="$(json_field id "$app_body")"
              if [ -n "$app_id" ]; then
                expect_status_any POST "/api/v1/apps/$app_id/deploy" "200,202" "remote image app deploy" -H "Authorization: Bearer $token"
                expect_status_any DELETE "/api/v1/apps/$app_id" "204,404" "remote image app cleanup" -H "Authorization: Bearer $token"
              else
                fail "remote image app create response missing id"
              fi
            else
              fail "remote image app create returned HTTP ${app_code:-<empty>}: $(head -c 300 "$app_body" 2>/dev/null || true)"
            fi
          fi
        fi
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
