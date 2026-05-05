#!/usr/bin/env bash
# check-readme-coverage.sh — fail when the README coverage line drifts
# from the actual `go tool cover -func` numbers by more than ±0.5 pp.
#
# Two values are checked:
#   - filtered total (excluding tests/loadtest and tests/soak harnesses)
#   - raw total (entire repo)
#
# Inputs (env vars, all optional):
#   COVERAGE_PROFILE        path to an existing raw coverage profile.
#                           If set, the script reuses it instead of
#                           running `go test -short ./...` itself.
#   COVERAGE_DRIFT_TOLERANCE  max allowed |claimed - actual| in pp;
#                             default 0.5.
#
# Exit codes:
#   0  README within tolerance for both numbers
#   1  drift exceeds tolerance on at least one number
#   2  README does not contain the expected coverage line
#   3  go tooling failed to produce a profile

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
README="$REPO_ROOT/README.md"
TOLERANCE="${COVERAGE_DRIFT_TOLERANCE:-0.5}"

: "${COVERAGE_PROFILE:=}"

# 1. Produce a coverage profile if one wasn't handed in.
if [ -z "$COVERAGE_PROFILE" ]; then
    PROFILE="$(mktemp -t cov-readme.XXXXXX.out)"
    trap 'rm -f "$PROFILE" "${PROFILE}.filtered"' EXIT
    if ! ( cd "$REPO_ROOT" && go test -short -coverprofile="$PROFILE" ./... >/dev/null 2>&1 ); then
        echo "::error::go test -short failed; see make test for details" >&2
        exit 3
    fi
else
    PROFILE="$COVERAGE_PROFILE"
fi

if [ ! -s "$PROFILE" ]; then
    echo "::error::coverage profile $PROFILE is empty" >&2
    exit 3
fi

FILTERED_PROFILE="${PROFILE}.filtered"
grep -v '/tests/loadtest\|/tests/soak' "$PROFILE" >"$FILTERED_PROFILE" || true

ACTUAL_FILTERED=$(go tool cover -func="$FILTERED_PROFILE" | awk '/^total:/ {gsub("%","",$NF); print $NF}')
ACTUAL_RAW=$(go tool cover -func="$PROFILE" | awk '/^total:/ {gsub("%","",$NF); print $NF}')

if [ -z "$ACTUAL_FILTERED" ] || [ -z "$ACTUAL_RAW" ]; then
    echo "::error::could not derive total coverage from profile" >&2
    exit 3
fi

# 2. Pull the claimed numbers out of the README. The format is fixed
# enough that a targeted grep is more honest than a full-document parse.
# The "Raw coverage including those harnesses is N %" sentence is
# wrapped across two README lines; flatten newlines so the grep
# matches the whole phrase.
README_FLAT=$(tr '\n' ' ' <"$README")
# Markdown line wrap turns single newlines into runs of whitespace
# after the tr-flatten; tolerate that with [[:space:]]+ between words.
CLAIMED_FILTERED=$(printf '%s' "$README_FLAT" | grep -oE 'statement-weighted[[:space:]]+\*\*[0-9.]+[[:space:]]*%\*\*' | head -n1 | grep -oE '[0-9.]+')
CLAIMED_RAW=$(printf '%s' "$README_FLAT" | grep -oE 'Raw[[:space:]]+coverage[[:space:]]+including[[:space:]]+those[[:space:]]+harnesses[[:space:]]+is[[:space:]]+[0-9.]+[[:space:]]*%' | head -n1 | grep -oE '[0-9.]+')

if [ -z "$CLAIMED_FILTERED" ] || [ -z "$CLAIMED_RAW" ]; then
    echo "::error::could not find coverage claim in $README — expected 'statement-weighted **N %**' and 'Raw coverage including those harnesses is N %' patterns" >&2
    exit 2
fi

# 3. Compare each pair. awk handles the absolute value cleanly.
fail=0
report() {
    local label="$1" claimed="$2" actual="$3"
    local drift
    drift=$(awk -v a="$claimed" -v b="$actual" 'BEGIN{ d=a-b; if (d<0) d=-d; printf "%.2f", d }')
    local within
    within=$(awk -v d="$drift" -v t="$TOLERANCE" 'BEGIN{ print (d<=t) ? "yes" : "no" }')
    if [ "$within" = "yes" ]; then
        printf "  %-10s claimed=%s%% actual=%s%% drift=%spp ≤ %spp ✔\n" \
            "$label" "$claimed" "$actual" "$drift" "$TOLERANCE"
    else
        printf "  %-10s claimed=%s%% actual=%s%% drift=%spp > %spp ✘\n" \
            "$label" "$claimed" "$actual" "$drift" "$TOLERANCE" >&2
        fail=1
    fi
}

echo "README coverage drift check (tolerance ±${TOLERANCE} pp):"
report "filtered" "$CLAIMED_FILTERED" "$ACTUAL_FILTERED"
report "raw"      "$CLAIMED_RAW"      "$ACTUAL_RAW"

if [ "$fail" -ne 0 ]; then
    cat >&2 <<EOF
::error::README coverage line is stale. Update README.md with the
actual numbers above (run \`go test -short -coverprofile=cov.out
./... && go tool cover -func=cov.out | tail -1\` to confirm).
EOF
    exit 1
fi
