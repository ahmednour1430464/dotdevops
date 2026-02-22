#!/bin/bash
set -e

echo "=== v0.3 Expression Tests ==="
echo

# Test valid expressions
echo "Testing valid expressions..."
for file in tests/v0_3/valid/*.devops; do
    echo -n "  $(basename $file): "
    if go run ./cmd/devopsctl plan build "$file" > /dev/null 2>&1; then
        echo "✓ PASS"
    else
        echo "✗ FAIL"
        go run ./cmd/devopsctl plan build "$file"
        exit 1
    fi
done

echo
echo "Testing invalid expressions (should fail)..."
for file in tests/v0_3/invalid/*.devops; do
    echo -n "  $(basename $file): "
    if go run ./cmd/devopsctl plan build "$file" > /dev/null 2>&1; then
        echo "✗ FAIL (should have errored)"
        exit 1
    else
        echo "✓ PASS (correctly rejected)"
    fi
done

echo
echo "=== All v0.3 tests passed! ==="
