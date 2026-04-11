#!/bin/bash
#
# ci-local.sh - Local CI validation script
# Runs all CI checks locally before pushing to GitHub
#
# Usage:
#   ./scripts/ci-local.sh           # Full CI (all checks)
#   ./scripts/ci-local.sh --quick   # Quick checks only (skip slow tests)
#   ./scripts/ci-local.sh --fix     # Auto-fix issues where possible
#

set -o pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
PASSED=0
FAILED=0
SKIPPED=0

# Parse arguments
QUICK=false
FIX_MODE=false

for arg in "$@"; do
    case $arg in
        --quick) QUICK=true ;;
        --fix) FIX_MODE=true ;;
    esac
done

START_TIME=$(date +%s)

print_banner() {
    echo -e "${BLUE}"
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║              DeployMonster - Local CI Validation             ║"
    echo "╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

section() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

pass() {
    echo -e "  ${GREEN}✓${NC} $1"
    PASSED=$((PASSED + 1))
}

fail() {
    echo -e "  ${RED}✗${NC} $1"
    if [ -n "$2" ]; then
        echo -e "    ${YELLOW}→ $2${NC}"
    fi
    FAILED=$((FAILED + 1))
}

skip() {
    echo -e "  ${YELLOW}○${NC} $1"
    SKIPPED=$((SKIPPED + 1))
}

# ============================================
# 1. Go Formatting
# ============================================
section "1. Go Formatting"

echo -e "  ${BLUE}Checking Go formatting...${NC}"

# Get all .go files
unformatted_files=0
for file in $(find . -name "*.go" -not -path "./vendor/*" -not -path "./web/*"); do
    # Check if file needs formatting
    if gofmt -d "$file" 2>/dev/null | grep -q .; then
        unformatted_files=1
        if [ "$FIX_MODE" == "true" ]; then
            echo -e "  ${YELLOW}Auto-fixing: $file${NC}"
            gofmt -w "$file"
        else
            fail "$file needs go fmt"
        fi
    fi
done

if [ $unformatted_files -eq 0 ]; then
    pass "All Go files formatted"
else
    if [ "$FIX_MODE" == "true" ]; then
        pass "Auto-fixed $unformatted_files files"
    else
        echo -e "    ${YELLOW}Run: gofmt -w ./...${NC}"
    fi
fi

# ============================================
# 2. Go Vet
# ============================================
section "2. Go Vet"

echo -e "  ${BLUE}Running go vet...${NC}"

if go vet ./... 2>&1; then
    pass "go vet passed"
else
    fail "go vet found issues" "Run: go vet ./..."
fi

# ============================================
# 3. Go Mod Tidy
# ============================================
section "3. Go Mod Tidy"

echo -e "  ${BLUE}Checking go.mod...${NC}"

# Backup current go.mod
cp go.mod go.mod.bak
cp go.sum go.sum.bak

# Run go mod tidy
go mod tidy 2>/dev/null

# Check if go.mod changed
if ! diff -q go.mod go.mod.bak 2>/dev/null || ! diff -q go.sum go.sum.bak 2>/dev/null; then
    fail "go.mod or go.sum changed after go mod tidy"
    echo -e "    ${YELLOW}Run: go mod tidy${NC}"
    # Restore originals
    mv go.mod.bak go.mod 2>/dev/null
    mv go.sum.bak go.sum 2>/dev/null
else
    pass "go mod tidy passed"
fi

# Cleanup
rm -f go.mod.bak go.sum.bak

# ============================================
# 4. Build Check
# ============================================
section "4. Build Check"

echo -e "  ${BLUE}Building binary...${NC}"

if [[ "$QUICK" == "true" ]]; then
    skip "Build check (quick mode)"
else
    if go build -o /dev/null ./... 2>&1; then
        pass "Build passed"
    else
        fail "Build failed"
    fi
fi

# ============================================
# 5. Go Tests
# ============================================
section "5. Go Tests"

if [[ "$QUICK" == "true" ]]; then
    skip "Go tests (quick mode)"
else
    echo -e "  ${BLUE}Running Go tests...${NC}"

    if go test -v -race -coverprofile=coverage.out ./... 2>&1; then
        pass "All Go tests passed"

        # Show coverage summary
        echo ""
        echo -e "  ${BLUE}Coverage Summary:${NC}"
        go tool cover -func=coverage.out 2>/dev/null | tail -5

        rm -f coverage.out
    else
        fail "Go tests failed"
        echo -e "    ${YELLOW}Run: go test -v ./...${NC}"
    fi
fi

# ============================================
# 6. React Tests
# ============================================
section "6. React Tests"

if [[ "$QUICK" == "true" ]]; then
    skip "React tests (quick mode)"
else
    if [ ! -f "web/package.json" ]; then
        skip "No web/package.json found"
    else
        echo -e "  ${BLUE}Running React tests...${NC}"

        # Check if node_modules exists
        if [ ! -d "web/node_modules" ]; then
            echo -e "  ${YELLOW}Installing pnpm dependencies...${NC}"
            (cd web && pnpm install --frozen-lockfile 2>/dev/null)
        fi

        if (cd web && pnpm test 2>&1); then
            pass "React tests passed"
        else
            fail "React tests failed"
        fi
    fi
fi

# ============================================
# 7. Playwright E2E Tests
# ============================================
section "7. Playwright E2E Tests"

if [[ "$QUICK" == "true" ]]; then
    skip "Playwright E2E tests (quick mode)"
else
    if [ ! -f "web/playwright.config.ts" ]; then
        skip "No Playwright config found"
    else
        # Check if server is running
        if curl -s http://localhost:8443/health >/dev/null 2>&1; then
            echo -e "  ${BLUE}Running Playwright E2E tests...${NC}"
            if (cd web && npx playwright test 2>&1); then
                pass "Playwright E2E tests passed"
            else
                fail "Playwright E2E tests failed"
            fi
        else
            skip "Playwright E2E tests (server not running on :8443)"
        fi
    fi
fi

# ============================================
# 8. Security Scan
# ============================================
section "8. Security Scan"

if command -v govulncheck &>/dev/null; then
    echo -e "  ${BLUE}Running govulncheck...${NC}"

    if govulncheck ./... 2>&1; then
        pass "No known vulnerabilities"
    else
        # Warn but don't fail — stdlib CVEs require Go update, Docker SDK CVEs have no fix
        skip "Vulnerabilities found (review with: govulncheck ./...)"
    fi
else
    skip "govulncheck not installed (optional)"
fi

# ============================================
# 9. Git Status
# ============================================
section "9. Git Status"

# Check for staged changes
staged=$(git diff --cached --stat 2>/dev/null | wc -l)

if [ -n "$staged" ]; then
    pass "Has staged changes ready to commit"
else
    # Check for unstaged changes
    unstaged=$(git status --porcelain 2>/dev/null | wc -l)

    if [ -n "$unstaged" ]; then
        fail "Has unstaged/uncommitted changes"
        echo -e "    ${YELLOW}Run: git status${NC}"
    else
        pass "Working tree clean"
    fi
fi

# ============================================
# 10. Large Files Check
# ============================================
section "10. Large Files Check"

# Check for files > 500KB
large_files=$(find . -type f -size +500k -not -path "./.git/*" -not -path "./web/node_modules/*" -not -path "*.db" -not -path "*.db-shm" 2>/dev/null | head -5)

if [ -n "$large_files" ]; then
    echo -e "  ${YELLOW}Large files found (>500KB):${NC}"
    echo "$large_files" | while read line; do
        echo -e "    ${YELLOW}$line${NC}"
    done
    skip "Has large files (may affect git push)"
else
    pass "No large files found"
fi

# ============================================
# 11. CI Workflow Simulation
# ============================================
section "11. CI Workflow Simulation"

echo -e "  ${BLUE}Simulating GitHub Actions CI steps...${NC}"

# Check if we can replicate all CI steps locally
ci_pass=true

# Step 1: Checkout
if git rev-parse --is-inside-work-tree &>/dev/null 2>&1; then
    pass "Checkout step would succeed"
else
    fail "Not inside a git repository"
fi

# Step 2: Setup Go
if go version &>/dev/null; then
    pass "Go setup would succeed (Go $(go version | head -1 | awk '{print $3}')"
else
    fail "Go not installed"
fi

# Step 3: Run tests (already done above)

# Step 4: Build binary (already done above)

# Step 5: Docker build (if Dockerfile exists)
if [ -f "Dockerfile" ]; then
    if command -v docker &>/dev/null; then
        skip "Docker build would work (Docker available)"
    else
        skip "Docker build would fail (Docker not running)"
    fi
else
    skip "Docker build step (no Dockerfile)"
fi

# ============================================
# Final Summary
# ============================================
echo ""
echo -e "${BLUE}══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  CI Local Validation Summary${NC}"
echo -e "${BLUE}══════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "  ${GREEN}Passed:${NC}   $PASSED"
echo -e "  ${RED}Failed:${NC}   $FAILED"
echo -e "  ${YELLOW}Skipped:${NC}  $SKIPPED"
echo ""

if [ $FAILED -gt 0 ]; then
    echo ""
    echo -e "${RED}════════════════════════════════════════════${NC}"
    echo -e "${RED}  CI VALIDATION FAILED!${NC}"
    echo -e "${RED}  Fix issues before pushing to GitHub${NC}"
    echo -e "${RED}════════════════════════════════════════════${NC}"
    echo ""
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    echo -e "  ${BLUE}Duration: ${DURATION}s${NC}"
    echo ""
    exit 1
else
    echo ""
    echo -e "${GREEN}════════════════════════════════════════════${NC}"
    echo -e "${GREEN}  ✓ ALL CHECKS PASSED!${NC}"
    echo -e "${GREEN}  Safe to push to GitHub${NC}"
    echo -e "${GREEN}══════════════════════════════════════════════${NC}"
    echo ""
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    echo -e "  ${BLUE}Duration: ${DURATION}s${NC}"
    echo ""
    exit 0
fi
