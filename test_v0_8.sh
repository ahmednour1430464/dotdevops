#!/usr/bin/env bash
set -e

./devopsctl help > /dev/null || { echo "devopsctl binary not found, please build it first"; exit 1; }

echo "=== v0.8 Valid Tests ==="
for f in tests/v0_8/valid/*.devops; do
  echo "Testing: $(basename "$f")"
  ./devopsctl plan build "$f" > /dev/null || {
    echo "  ✗ FAIL: $f"
    exit 1
  }
  echo "  ✓ PASS"
done

echo ""
echo "=== v0.8 Invalid Tests ==="
set +e
FAILS=0

function check_invalid() {
    local file=$1
    local pattern=$2
    echo "Testing (expect fail): $(basename "$file")"
    OUTPUT=$(./devopsctl plan build "$file" 2>&1)
    if echo "$OUTPUT" | grep -q "$pattern"; then
        echo "  ✓ PASS (caught: $pattern)"
    else
        echo "  ✗ FAIL: $file (expected pattern '$pattern' not found)"
        echo "  Output was:"
        echo "$OUTPUT"
        FAILS=$((FAILS + 1))
    fi
}

check_invalid "tests/v0_8/invalid/fleet_no_match.devops" "do not match any defined targets"
check_invalid "tests/v0_8/invalid/contract_invalid_side_effects.devops" "invalid side_effects"
check_invalid "tests/v0_8/invalid/contract_unsupported_rollback.devops" "rollback_cmd is only supported on process.exec"
check_invalid "tests/v0_8/invalid/contract_invalid_retry.devops" "retry attempts must be strictly positive"
check_invalid "tests/v0_8/invalid/duplicate_name.devops" "collides with target name"
check_invalid "tests/v0_8/invalid/duplicate_name_symmetric.devops" "collides with fleet name"

if [ $FAILS -gt 0 ]; then
    echo "✗ $FAILS invalid tests failed to be caught correctly"
    exit 1
fi
set -e

echo ""
echo "=== v0.8 Hash Stability ==="
# Compare fleet expansion vs manual list expansion. 
# They should produce the same Plan IR structure (and thus same hash).
hash1=$(./devopsctl plan build tests/v0_8/hash_stability/fleet_generated.devops | sha256sum | cut -d' ' -f1)
hash2=$(./devopsctl plan build tests/v0_8/hash_stability/fleet_manual.devops | sha256sum | cut -d' ' -f1)

echo "  fleet_generated.devops hash: $hash1"
echo "  fleet_manual.devops hash:    $hash2"

if [ "$hash1" = "$hash2" ]; then
  echo "  ✓ PASS (hashes match - fleet expansion is content-stable)"
else
  echo "  ✗ FAIL (hash mismatch)"
  exit 1
fi

echo ""
echo "✅ All v0.8 tests passed"
