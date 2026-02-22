#!/usr/bin/env bash
set -e

echo "=== v0.5 Language Test Suite ==="

# Build
echo "Building devopsctl..."
go build -o ./devopsctl ./cmd/devopsctl

echo ""
echo "=== Testing Valid Cases ==="
# Test valid cases
for file in tests/v0_5/valid/*.devops; do
    echo -n "  $(basename $file) ... "
    ./devopsctl plan build --lang=v0.5 "$file" > /dev/null
    echo "✓ PASS"
done

echo ""
echo "=== Testing Invalid Cases ==="
# Test invalid cases (should fail)
for file in tests/v0_5/invalid/*.devops; do
    echo -n "  $(basename $file) ... "
    if ./devopsctl plan build --lang=v0.5 "$file" 2>&1 | grep -q "error:"; then
        echo "✓ PASS (correctly rejected)"
    else
        echo "✗ FAIL (should have been rejected)"
        exit 1
    fi
done

echo ""
echo "✓ All v0.5 tests passed"
