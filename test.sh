#!/bin/bash
# Usage:
#   ./test.sh              — dry-run check only (requires local.json and local proxy)
#   TEST_FULL=1 TEST_REPO=/path/to/repo ./test.sh  — also runs full import checks

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PASS=0
FAIL=0

pass() {
    echo "[PASS] $1"
    PASS=$((PASS + 1))
}

fail() {
    echo "[FAIL] $1"
    FAIL=$((FAIL + 1))
}

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

echo "--- build ---"
if ! go build -C "$SCRIPT_DIR" .; then
    echo "ERROR: go build failed — aborting"
    exit 1
fi
pass "go build succeeded"

# ---------------------------------------------------------------------------
# Mode 1: dry-run check
# ---------------------------------------------------------------------------

echo ""
echo "--- dry-run check ---"

SETTINGS="$SCRIPT_DIR/local.json"

if [ ! -f "$SETTINGS" ]; then
    echo "SKIP: $SETTINGS not found — skipping dry-run section"
    exit 0
fi

DRY_RUN_OUTPUT=$("$SCRIPT_DIR/os-std-importer" -settings=local.json -dry-run -verbose 2>&1) || {
    fail "dry-run: binary exited non-zero"
    echo "  output: $DRY_RUN_OUTPUT"
    FAIL=$((FAIL + 1))
    # fall through to summary
    DRY_RUN_OUTPUT=""
}

if [ -n "$DRY_RUN_OUTPUT" ]; then
    if echo "$DRY_RUN_OUTPUT" | grep -qE '^(Repo is at or ahead of Onshape document version|New version available: .+)$'; then
        pass "dry-run: output is expected ('$DRY_RUN_OUTPUT')"
    else
        fail "dry-run: unexpected output: '$DRY_RUN_OUTPUT'"
    fi
fi

# ---------------------------------------------------------------------------
# Mode 2: full run (TEST_FULL=1 only)
# ---------------------------------------------------------------------------

if [ "${TEST_FULL:-}" = "1" ]; then
    echo ""
    echo "--- full run ---"

    if [ -z "${TEST_REPO:-}" ]; then
        echo "ERROR: TEST_FULL=1 but TEST_REPO is not set"
        exit 1
    fi

    if [ ! -d "$TEST_REPO/.git" ]; then
        echo "ERROR: TEST_REPO=$TEST_REPO is not a git repository"
        exit 1
    fi

    # Run the full import
    "$SCRIPT_DIR/os-std-importer" -settings=local.json -out="$TEST_REPO"

    # Check: versions.txt exists and is non-empty on with-versions
    git -C "$TEST_REPO" checkout with-versions --quiet
    VERSIONS_FILE="$TEST_REPO/versions.txt"
    if [ -f "$VERSIONS_FILE" ] && [ -s "$VERSIONS_FILE" ]; then
        pass "full run: versions.txt exists and is non-empty on with-versions"
    else
        fail "full run: versions.txt missing or empty on with-versions"
    fi

    # Check: at least one .fs file exists on with-versions
    FS_COUNT=$(find "$TEST_REPO" -maxdepth 1 -name "*.fs" | wc -l | tr -d ' ')
    if [ "$FS_COUNT" -gt 0 ]; then
        pass "full run: $FS_COUNT .fs file(s) found on with-versions"
    else
        fail "full run: no .fs files found on with-versions"
    fi

    # Switch to without-versions and run checks there
    git -C "$TEST_REPO" checkout without-versions --quiet

    # Check: no .fs file contains a bare FeatureScript version line
    # The with-versions branch has lines like:  FeatureScript 2478;
    # After stripping, they become:            FeatureScript ; /** without versions **/
    # So we look for the raw pattern: "FeatureScript <digits>" at line start.
    VERSIONED_FILES=$(grep -rl --include="*.fs" -E '^FeatureScript [0-9]' "$TEST_REPO" 2>/dev/null || true)
    if [ -z "$VERSIONED_FILES" ]; then
        pass "full run: no .fs file on without-versions contains bare FeatureScript version"
    else
        fail "full run: version strings not stripped in: $VERSIONED_FILES"
    fi

    # Check: versions.txt is present on without-versions too
    if [ -f "$TEST_REPO/versions.txt" ]; then
        pass "full run: versions.txt present on without-versions"
    else
        fail "full run: versions.txt missing on without-versions"
    fi
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

echo ""
if [ "$FAIL" -eq 0 ]; then
    echo "All checks passed."
else
    echo "Some checks failed. ($PASS passed, $FAIL failed)"
    exit 1
fi
