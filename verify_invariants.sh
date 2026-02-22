#!/usr/bin/env bash
# Invariant verification script
# Automatically checks that architectural invariants are maintained

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "🔍 Verifying Architectural Invariants..."
echo

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

FAILURES=0

# Helper functions
pass() {
    echo -e "${GREEN}✓${NC} $1"
}

fail() {
    echo -e "${RED}✗${NC} $1"
    FAILURES=$((FAILURES + 1))
}

warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

echo "=== Invariant 1: Lowering Is a One-Way Door ==="
echo

# Check that internal/plan/ contains no high-level language constructs
echo "Checking internal/plan/ for language constructs..."

STEP_REFS=$(grep -r "type.*Step" internal/plan/ 2>/dev/null | wc -l || echo 0)
FOR_REFS=$(grep -r "type.*For" internal/plan/ 2>/dev/null | wc -l || echo 0)
LET_REFS=$(grep -r "type.*Let" internal/plan/ 2>/dev/null | wc -l || echo 0)
PARAM_REFS=$(grep -r "type.*Param" internal/plan/ 2>/dev/null | wc -l || echo 0)
IMPORT_REFS=$(grep -r "type.*Import" internal/plan/ 2>/dev/null | wc -l || echo 0)

if [ "$STEP_REFS" -eq 0 ] && [ "$FOR_REFS" -eq 0 ] && [ "$LET_REFS" -eq 0 ] && \
   [ "$PARAM_REFS" -eq 0 ] && [ "$IMPORT_REFS" -eq 0 ]; then
    pass "Runtime schema contains no language constructs"
else
    fail "Runtime schema contains language constructs (Step: $STEP_REFS, For: $FOR_REFS, Let: $LET_REFS, Param: $PARAM_REFS, Import: $IMPORT_REFS)"
fi

# Check that controller doesn't import devlang
echo "Checking controller independence from language..."

DEVLANG_IMPORTS=$(grep -r "devopsctl/internal/devlang" internal/controller/ 2>/dev/null | wc -l || echo 0)

if [ "$DEVLANG_IMPORTS" -eq 0 ]; then
    pass "Controller does not import language package"
else
    fail "Controller imports language package ($DEVLANG_IMPORTS occurrences)"
fi

echo
echo "=== Invariant 2: Hashes After Full Expansion ==="
echo

# Check that hash computation happens on lowered plan
echo "Checking hash computation location..."

# Hash should be in plan package, operating on Node type
HASH_IN_PLAN=$(grep -n "func.*Hash" internal/plan/schema.go 2>/dev/null | wc -l || echo 0)

if [ "$HASH_IN_PLAN" -gt 0 ]; then
    pass "Hash computation found in plan package"
else
    fail "Hash computation not found in expected location"
fi

echo
echo "=== Invariant 3: Deterministic Order ==="
echo

# Check for unsorted map iteration in lowering
echo "Checking for potential non-deterministic map iteration..."

RANGE_OVER_MAP=$(grep -n "for.*range.*steps\[" internal/devlang/lower.go 2>/dev/null | wc -l || echo 0)

if [ "$RANGE_OVER_MAP" -gt 0 ]; then
    warn "Potential unsorted map iteration in lower.go (review needed)"
else
    pass "No obvious unsorted map iteration detected"
fi

echo
echo "=== Invariant 4: Version-Strict Validation ==="
echo

# Check that each version has its own validation function
echo "Checking version-specific validation functions..."

HAS_V0_1=$(grep -n "func ValidateV0_1" internal/devlang/validate.go 2>/dev/null | wc -l || echo 0)
HAS_V0_2=$(grep -n "func ValidateV0_2" internal/devlang/validate.go 2>/dev/null | wc -l || echo 0)
HAS_V0_3=$(grep -n "func ValidateV0_3" internal/devlang/validate.go 2>/dev/null | wc -l || echo 0)
HAS_V0_4=$(grep -n "func ValidateV0_4" internal/devlang/validate.go 2>/dev/null | wc -l || echo 0)
HAS_V0_5=$(grep -n "func ValidateV0_5" internal/devlang/validate.go 2>/dev/null | wc -l || echo 0)

if [ "$HAS_V0_1" -gt 0 ] && [ "$HAS_V0_2" -gt 0 ] && [ "$HAS_V0_3" -gt 0 ] && [ "$HAS_V0_4" -gt 0 ] && [ "$HAS_V0_5" -gt 0 ]; then
    pass "All version validation functions present (v0.1-v0.5)"
else
    fail "Missing version validation functions"
fi

echo
echo "=== Additional Checks ==="
echo

# Check that DESIGN.md exists
if [ -f "DESIGN.md" ]; then
    pass "DESIGN.md exists"
else
    fail "DESIGN.md not found"
fi

# Check that test structure exists
if [ -d "tests/v0_5" ]; then
    pass "Test directory structure exists"
else
    warn "Test directory structure incomplete"
fi

# Check for test scripts
TEST_SCRIPTS=0
[ -f "test_v0_1.sh" ] && TEST_SCRIPTS=$((TEST_SCRIPTS + 1))
[ -f "test_v0_2.sh" ] && TEST_SCRIPTS=$((TEST_SCRIPTS + 1))
[ -f "test_v0_3.sh" ] && TEST_SCRIPTS=$((TEST_SCRIPTS + 1))
[ -f "test_v0_4.sh" ] && TEST_SCRIPTS=$((TEST_SCRIPTS + 1))
[ -f "test_v0_5.sh" ] && TEST_SCRIPTS=$((TEST_SCRIPTS + 1))

if [ "$TEST_SCRIPTS" -eq 5 ]; then
    pass "All test scripts present (v0.1-v0.5)"
else
    warn "Some test scripts missing ($TEST_SCRIPTS/5 found)"
fi

echo
echo "=== Summary ==="
echo

if [ "$FAILURES" -eq 0 ]; then
    echo -e "${GREEN}✓ All invariants verified successfully${NC}"
    exit 0
else
    echo -e "${RED}✗ $FAILURES invariant violation(s) detected${NC}"
    exit 1
fi
