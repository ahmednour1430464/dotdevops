#!/usr/bin/env bash
set -e

echo "=== v2.0 Valid Tests ==="
for f in tests/v2_0/valid/*.devops; do
  echo "Testing: $(basename "$f")"
  ./devopsctl plan build "$f" > /dev/null || {
    echo "  ✗ FAIL"
    exit 1
  }
  echo "  ✓ PASS"
done

echo ""
echo "=== v2.0 Invalid Tests ==="

echo "Testing: circular_import_a.devops"
if ./devopsctl plan build tests/v2_0/invalid/circular_import_a.devops 2>&1 | grep -q "circular import"; then
  echo "  ✓ PASS (circular import detected)"
else
  echo "  ✗ FAIL (expected circular import error)"
  exit 1
fi

echo "Testing: import_not_found.devops"
if ./devopsctl plan build tests/v2_0/invalid/import_not_found.devops 2>&1 | grep -q "no such file"; then
  echo "  ✓ PASS (missing file detected)"
else
  echo "  ✗ FAIL (expected missing file error)"
  exit 1
fi

echo ""
echo "=== v2.0 Hash Stability ==="

# Function expansion hash stability
hash1=$(./devopsctl plan build tests/v2_0/hash_stability/fn_expansion.devops | sha256sum | cut -d' ' -f1)
hash2=$(./devopsctl plan build tests/v2_0/hash_stability/fn_manual.devops | sha256sum | cut -d' ' -f1)

echo "  fn_expansion.devops hash: $hash1"
echo "  fn_manual.devops hash:    $hash2"

if [ "$hash1" = "$hash2" ]; then
  echo "  ✓ PASS (function expansion produces stable hash)"
else
  echo "  ✗ FAIL (hash mismatch for function expansion)"
  exit 1
fi

# Import resolution hash stability  
hash3=$(./devopsctl plan build tests/v2_0/hash_stability/import_expansion.devops | sha256sum | cut -d' ' -f1)
hash4=$(./devopsctl plan build tests/v2_0/hash_stability/import_manual.devops | sha256sum | cut -d' ' -f1)

echo "  import_expansion.devops hash: $hash3"
echo "  import_manual.devops hash:    $hash4"

if [ "$hash3" = "$hash4" ]; then
  echo "  ✓ PASS (import resolution produces stable hash)"
else
  echo "  ✗ FAIL (hash mismatch for import resolution)"
  exit 1
fi

echo ""
echo "✅ All v2.0 tests passed"
