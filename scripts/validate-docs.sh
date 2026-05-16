#!/bin/bash
# validate-docs.sh - Validates ARCHITECTURE.md against actual codebase
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

ERRORS=0

echo "=== DeployMonster Documentation Validator ==="
echo

# 1. Check module count
echo "Checking modules..."
CODE_MODULES=$(find internal -mindepth 1 -maxdepth 1 -type d -exec test -f {}/module.go \; -exec grep -l "RegisterModule" {}/module.go \; 2>/dev/null | wc -l)
DOC_MODULE_LINES=$(grep -E "^[[:space:]]*\| \`[a-z]" ARCHITECTURE.md | wc -l)

if [ "$DOC_MODULE_LINES" -lt "$CODE_MODULES" ]; then
    echo -e "${YELLOW}  [WARN]${NC} Module count mismatch: docs show ~$DOC_MODULE_LINES table entries, code has $CODE_MODULES module directories"
else
    echo -e "${GREEN}  [OK]${NC} Module directories: $CODE_MODULES"
fi

# 2. Check event types count
echo "Checking event types..."
CODE_EVENTS=$(grep -cE "^[[:space:]]+Event[A-Z][a-zA-Z]+[[:space:]]*=" internal/core/events.go || echo 0)
DOC_EVENTS=$(grep -cE "EventApp|EventBuild|EventContainer|EventDeploy|EventDomain|EventServer|EventWebhook|EventBackup|EventAlert|EventUser|EventSecret|EventDatabase|EventNotification|EventBilling|EventProject|EventCron|EventDNS|EventRedirect|EventAutoscale|EventBasic|EventGPU|EventSystem" ARCHITECTURE.md || echo 0)

echo -e "  [INFO]${NC} Event types in code: $CODE_EVENTS"
echo -e "  [INFO]${NC} Event types in docs: $DOC_EVENTS (approximate)"

# 3. Check config sections
echo "Checking config structure..."
EXPECTED_SECTIONS="server database ingress acme dns docker backup notifications swarm vps_providers git_sources marketplace registration secrets billing limits enterprise observability"
for section in $EXPECTED_SECTIONS; do
    if ! grep -q "$section:" ARCHITECTURE.md; then
        echo -e "${RED}  [FAIL]${NC} Config section '$section' not documented"
        ERRORS=$((ERRORS + 1))
    else
        echo -e "${GREEN}  [OK]${NC} Config section: $section"
    fi
done

# 4. Check middleware chain
echo "Checking middleware chain..."
if grep -q "Tracing" ARCHITECTURE.md; then
    echo -e "${GREEN}  [OK]${NC} Tracing middleware documented"
else
    echo -e "${RED}  [FAIL]${NC} Tracing middleware not documented"
    ERRORS=$((ERRORS + 1))
fi

if grep -q "IP Allowlist" ARCHITECTURE.md; then
    echo -e "${GREEN}  [OK]${NC} IPAllowlist middleware documented"
else
    echo -e "${RED}  [FAIL]${NC} IPAllowlist middleware not documented"
    ERRORS=$((ERRORS + 1))
fi

# 5. Check BBolt buckets
echo "Checking BBolt bucket documentation..."
BUCKETS_FOUND=0
# Check for updated bucket names
for bucket in sessions ratelimit buildcache metrics_ring cronjobs app_pins autoscale basic_auth api_keys deploy_freeze deploy_notify deploy_approval maintenance app_middleware container_metrics announcements certificates ssh_keys log_retention event_webhooks webhook_logs webhooks revoked_tokens vault git_provider_connections; do
    if grep -q "| \`$bucket\` |" ARCHITECTURE.md; then
        BUCKETS_FOUND=$((BUCKETS_FOUND + 1))
    fi
done
# Also check for legacy bucket names
for bucket in idempotency rate_limit csrf_tokens; do
    if grep -q "| \`$bucket\` |" ARCHITECTURE.md; then
        BUCKETS_FOUND=$((BUCKETS_FOUND + 1))
    fi
done
if [ "$BUCKETS_FOUND" -ge 20 ]; then
    echo -e "${GREEN}  [OK]${NC} BBolt buckets documented: $BUCKETS_FOUND (comprehensive)"
else
    echo -e "${YELLOW}  [WARN]${NC} BBolt buckets documented: $BUCKETS_FOUND (expected 25+)"
fi

# 6. Check agent route exists
echo "Checking agent WebSocket route..."
if grep -q "/api/v1/agent/ws" internal/swarm/module.go; then
    echo -e "${GREEN}  [OK]${NC} Agent WebSocket route exists in code"
else
    echo -e "${RED}  [FAIL]${NC} Agent WebSocket route not found in code"
    ERRORS=$((ERRORS + 1))
fi

# 7. Check for required validation functions
echo "Checking config validation..."
if grep -q "AllowedCIDRs" internal/core/config.go && grep -q "ParseCIDR" internal/core/config.go; then
    echo -e "${GREEN}  [OK]${NC} CIDR validation implemented"
else
    echo -e "${YELLOW}  [WARN]${NC} CIDR validation may be missing"
fi

if grep -q "TracingURL" internal/core/config.go && grep -q "observability.tracing_url" internal/core/config.go; then
    echo -e "${GREEN}  [OK]${NC} TracingURL validation implemented"
else
    echo -e "${YELLOW}  [WARN]${NC} TracingURL validation may be missing"
fi

# 8. Check AuditSecrets is called
echo "Checking AuditSecrets usage..."
if grep -q "AuditSecrets" internal/core/app.go; then
    echo -e "${GREEN}  [OK]${NC} AuditSecrets called at startup"
else
    echo -e "${YELLOW}  [WARN]${NC} AuditSecrets not called at startup"
fi

echo
echo "=== Summary ==="
if [ $ERRORS -eq 0 ]; then
    echo -e "${GREEN}All critical checks passed!${NC}"
    exit 0
else
    echo -e "${RED}$ERRORS critical errors found${NC}"
    exit 1
fi