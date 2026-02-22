#!/usr/bin/env bash
# Test runner for v0.4 language features

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "=== v0.4 Language Test Suite ==="
echo

# Build the tool
echo "Building devopsctl..."
go build -o ./devopsctl ./cmd/devopsctl
echo "✓ Build successful"
echo

# Test valid cases
echo "Testing valid cases..."
for file in tests/v0_4/valid/*.devops; do
    basename=$(basename "$file")
    echo -n "  $basename ... "
    if ./devopsctl plan build --lang=v0.4 "$file" > /dev/null 2>&1; then
        echo "✓ PASS"
    else
        echo "✗ FAIL"
        ./devopsctl plan build --lang=v0.4 "$file"
        exit 1
    fi
done
echo

# Test invalid cases
echo "Testing invalid cases (should fail)..."
for file in tests/v0_4/invalid/*.devops; do
    basename=$(basename "$file")
    echo -n "  $basename ... "
    if ./devopsctl plan build --lang=v0.4 "$file" > /dev/null 2>&1; then
        echo "✗ FAIL (should have failed)"
        exit 1
    else
        echo "✓ PASS (correctly rejected)"
    fi
done
echo

# Test hash stability
echo "Testing hash stability..."
echo -n "  Compiling with_step.devops ... "
./devopsctl plan build --lang=v0.4 tests/v0_4/hash_stability/with_step.devops -o /tmp/with_step.json
HASH1=$(./devopsctl plan hash /tmp/with_step.json)
echo "✓ $HASH1"

echo -n "  Compiling without_step.devops ... "
./devopsctl plan build --lang=v0.4 tests/v0_4/hash_stability/without_step.devops -o /tmp/without_step.json
HASH2=$(./devopsctl plan hash /tmp/without_step.json)
echo "✓ $HASH2"

echo -n "  Comparing hashes ... "
if [ "$HASH1" = "$HASH2" ]; then
    echo "✓ PASS (hashes match)"
else
    echo "✗ FAIL (hashes differ)"
    echo "    with_step:    $HASH1"
    echo "    without_step: $HASH2"
    exit 1
fi
echo

echo "=== All v0.4 tests passed! ==="
