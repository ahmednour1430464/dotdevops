#!/bin/bash
set -e

echo "=== Cleaning up old agents ==="
pkill -f 'devopsctl agent' || true
sleep 1

echo "=== Building devopsctl ==="
go build -o devopsctl cmd/devopsctl/main.go

echo "=== Starting agent ==="
./devopsctl agent &
AGENT_PID=$!
sleep 1

cat << 'EOF' > tests/e2e/plan_resume.json
{
  "version": "1.0",
  "targets": [
    { "id": "local", "address": "127.0.0.1:7700" }
  ],
  "nodes": [
    {
      "id": "node_a",
      "type": "process.exec",
      "targets": ["local"],
      "inputs": { "cmd": ["echo", "Node A"], "cwd": "/tmp" }
    },
    {
      "id": "node_b",
      "type": "process.exec",
      "targets": ["local"],
      "inputs": { "cmd": ["echo", "Node B"], "cwd": "/tmp" },
      "depends_on": ["node_a"]
    },
    {
      "id": "node_c",
      "type": "process.exec",
      "targets": ["local"],
      "inputs": { "cmd": ["sh", "-c", "if [ -f /tmp/fail_c ]; then echo 'Fails'; exit 1; else echo 'Succeeds'; fi"], "cwd": "/tmp" },
      "depends_on": ["node_b"]
    },
    {
      "id": "node_d",
      "type": "process.exec",
      "targets": ["local"],
      "inputs": { "cmd": ["echo", "Node D"], "cwd": "/tmp" },
      "depends_on": ["node_c"]
    }
  ]
}
EOF

# Ensure clean state
rm -f ~/.devopsctl/state.db
touch /tmp/fail_c

echo "=== Running plan (expected to fail at C) ==="
./devopsctl apply tests/e2e/plan_resume.json || echo "Apply failed as expected"

echo "=== State after first run ==="
./devopsctl state list | tee /tmp/state_1.log

echo "=== Fixing condition for node C ==="
rm -f /tmp/fail_c

echo "=== Resuming plan ==="
./devopsctl apply tests/e2e/plan_resume.json --resume || { echo "Resume failed!"; exit 1; }

echo "=== State after resume ==="
./devopsctl state list | tee /tmp/state_2.log

echo "=== Testing reconcile ==="
# modify the plan to test reconcile
sed -i 's/"echo", "Node A"/"echo", "Node A Reconciled"/' tests/e2e/plan_resume.json
./devopsctl reconcile tests/e2e/plan_resume.json || { echo "Reconcile failed!"; exit 1; }

echo "=== Cleanup ==="
kill $AGENT_PID || true
echo "Success!"
