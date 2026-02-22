#!/usr/bin/env bash
set -e

echo "=== v0.5 Hash Stability Tests ==="

# Build
go build -o ./devopsctl ./cmd/devopsctl

# For-loop hash stability test
echo ""
echo "Testing for-loop hash stability..."
hash1=$(./devopsctl plan build --lang=v0.5 tests/v0_5/hash_stability/for_loop_generated.devops -o /tmp/for_generated.json && ./devopsctl plan hash /tmp/for_generated.json)
hash2=$(./devopsctl plan build --lang=v0.5 tests/v0_5/hash_stability/for_loop_manual.devops -o /tmp/for_manual.json && ./devopsctl plan hash /tmp/for_manual.json)

echo "  For-loop generated hash: $hash1"
echo "  Manually expanded hash:  $hash2"

if [ "$hash1" = "$hash2" ]; then
    echo "  ✓ PASS: Hashes match (for-loop stability verified)"
else
    echo "  ✗ FAIL: Hashes do not match"
    echo ""
    echo "Generated plan:"
    cat /tmp/for_generated.json | jq .
    echo ""
    echo "Manual plan:"
    cat /tmp/for_manual.json | jq .
    exit 1
fi

# Nested steps hash stability test
echo ""
echo "Testing nested steps hash stability..."
hash3=$(./devopsctl plan build --lang=v0.5 tests/v0_5/hash_stability/step_nested.devops -o /tmp/step_nested.json && ./devopsctl plan hash /tmp/step_nested.json)
hash4=$(./devopsctl plan build --lang=v0.5 tests/v0_5/hash_stability/step_expanded.devops -o /tmp/step_expanded.json && ./devopsctl plan hash /tmp/step_expanded.json)

echo "  Nested steps hash:  $hash3"
echo "  Expanded hash:      $hash4"

if [ "$hash3" = "$hash4" ]; then
    echo "  ✓ PASS: Hashes match (nested steps stability verified)"
else
    echo "  ✗ FAIL: Hashes do not match"
    echo ""
    echo "Nested plan:"
    cat /tmp/step_nested.json | jq .
    echo ""
    echo "Expanded plan:"
    cat /tmp/step_expanded.json | jq .
    exit 1
fi

echo ""
echo "✓ All hash stability tests passed"
