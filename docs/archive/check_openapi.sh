#!/bin/bash
# Quick check: bootstrap openapi allowlist and show results

cd /home/ersinkoc/Codebox/deploy-monster

echo "=== Running openapi-gen bootstrap ==="
go run ./cmd/openapi-gen -bootstrap

echo ""
echo "=== Current allowlist entry count ==="
wc -l docs/openapi-drift-allowlist.txt

echo ""
echo "=== Sample of undocumented routes ==="
head -20 docs/openapi-drift-allowlist.txt