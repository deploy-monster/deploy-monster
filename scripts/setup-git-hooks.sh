#!/bin/bash
#
# setup-git-hooks.sh - Install pre-commit and pre-push hooks
# Run this script once to set up hooks
#
# Usage:
#   ./scripts/setup-git-hooks.sh
#

set -e

HOOKS_DIR=".git/hooks"
PRE_COMMIT_HOOK="$HOOKS_DIR/pre-commit"
PRE_PUSH_HOOK="$HOOKS_DIR/pre-push"

echo -e "${GREEN}Setting up Git hooks...${NC}"

# Create pre-commit hook
cat > "$PRE_COMMIT_HOOK" << 'EOF'
#!/bin/bash
#
# Pre-commit hook - Run quick checks before commit
# Runs: go fmt, go vet, go mod tidy check

echo "Running pre-commit checks..."

# 1. Go fmt check (only on staged files)
STAGED_GO_FILES=$(git diff --cached --name-only --diff-filter=d --grep '\.go$ | grep -v '_test.go' 2>/dev/null | head -20)

if [ -n "$STAGED_GO_FILES" ]; then
    echo "Checking Go formatting..."
    UNFORMATTED=0
    for file in $STAGED_GO_FILES; do
        if gofmt -d "$file" 2>/dev/null | grep -q .; then
            echo "  ERROR: $file needs go fmt"
            UNFORMATTED=1
        fi
    done

    if [ $UNFORMATTED -eq 1 ]; then
        echo "❌ Pre-commit blocked: Run 'gofmt -w' on changed files"
        exit 1
    fi
    echo "  ✓ Go formatting OK"
fi

# 2. Go vet
echo "Running go vet..."
if ! go vet ./... 22>&1; then
    echo "❌ Pre-commit blocked: go vet found issues"
    exit 1
fi
echo "  ✓ Go vet OK"

# 3. Go mod tidy check
echo "Checking go.mod..."
cp go.mod go.mod.bak
cp go.sum go.sum.bak

go mod tidy

if ! diff -q go.mod go.mod.bak 2>/dev/null || ! diff -q go.sum go.sum.bak 2>/dev/null; then
    echo "❌ Pre-commit blocked: go.mod needs go mod tidy"
    rm -f go.mod.bak go.sum.bak
    exit 1
fi

rm -f go.mod.bak go.sum.bak

echo "  ✓ Go mod tidy OK"

echo ""
echo "✅ Pre-commit checks passed"
exit 0
EOF

# Create pre-push hook
cat > "$PRE_PUSH_HOOK" << 'EOF'
#!/bin/bash
#
# Pre-push hook - Run full CI validation before push
# Runs the ./scripts/ci-local.sh --quick

echo "Running pre-push CI validation..."

# Run CI validation
./scripts/ci-local.sh --quick

if [ $? -ne 0 ]; then
    echo ""
    echo "❌ Pre-push blocked: CI validation failed"
    echo "   Run: ./scripts/ci-local.sh --fix"
    exit 1
fi

echo ""
echo "✅ Pre-push checks passed"
exit 0
EOF

# Make hooks executable
chmod +x "$PRE_COMMIT_HOOK" "$PRE_PUSH_HOOK"

echo -e "${GREEN}Git hooks installed!${NC}"
echo ""
echo "Hooks installed:"
echo "  - Pre-commit: Runs go fmt, go vet, go mod tidy"
echo "  - Pre-push: Runs full CI validation"
echo ""
echo -e "${YELLOW}To skip hooks temporarily:${NC}"
echo "  git commit --no-verify"
echo ""
echo -e "${BLUE}To test:${NC}"
echo "  ./scripts/ci-local.sh --quick"
echo ""
