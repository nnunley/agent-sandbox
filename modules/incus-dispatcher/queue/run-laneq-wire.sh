#!/usr/bin/env bash
# Real-wire SCENARIO-0092: Go adapter ↔ real Python laneq gRPC server
#
# This script:
# 1. Starts the real laneq gRPC server via uvx (nnunley/laneq@2d1b59e)
# 2. Waits for the server to be reachable
# 3. Runs SCENARIO-0092 test with LANEQ_GRPC_REAL=1
# 4. Tears down the server
#
# Usage:
#   bash modules/incus-dispatcher/queue/run-laneq-wire.sh
#
# Exit codes:
#   0  PASS  — all tests passed
#   1  FAIL  — test failed or server unreachable
#   2  SKIP  — uvx or Python not available

set -uo pipefail

ADDR="${LANEQ_GRPC_ADDR:-localhost:50051}"
HOST="${ADDR%%:*}"
PORT="${ADDR##*:}"
FORK_HASH="2d1b59e"
LANEQ_DB="$(mktemp)"
TIMEOUT_SEC=30
SERVER_PID=""

# Cleanup on exit: kill server and remove temp DB
cleanup() {
	if [ -n "$SERVER_PID" ]; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	rm -f "$LANEQ_DB" "${LANEQ_DB}.log" 2>/dev/null || true
}
trap cleanup EXIT

# Check prerequisites
if ! command -v uvx &>/dev/null; then
  echo "SKIP: uvx not found (Python environment required)"
  exit 2
fi

if ! python3 --version &>/dev/null; then
  echo "SKIP: python3 not found"
  exit 2
fi

echo "Starting real laneq gRPC server (nnunley/laneq@${FORK_HASH}) at ${ADDR}..."
echo "  Using fresh LANEQ_DB=${LANEQ_DB}"

# Start the server in the background
export LANEQ_DB
uvx --from "git+https://github.com/nnunley/laneq@${FORK_HASH}[grpc]" \
  laneq-grpc --addr "${ADDR}" \
  >"${LANEQ_DB}.log" 2>&1 &
SERVER_PID="$!"

# Wait for server to be reachable
echo "Waiting for server to be reachable (${TIMEOUT_SEC}s timeout)..."
START_TIME=$(date +%s)
READY=0
while [ $(($(date +%s) - START_TIME)) -lt $TIMEOUT_SEC ]; do
  if nc -z "$HOST" "$PORT" 2>/dev/null; then
    READY=1
    echo "Server is ready at ${ADDR}"
    break
  fi
  sleep 0.2
done

if [ $READY -ne 1 ]; then
  echo "FAIL: Server did not become reachable within ${TIMEOUT_SEC}s"
  echo "--- Server log: ---"
  cat "${LANEQ_DB}.log" 2>/dev/null || echo "<no log>"
  exit 1
fi

# Run the test
echo "Running TestLaneqRealWireLifecycle against real laneq server..."
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$TEST_DIR"

export LANEQ_GRPC_REAL=1
export LANEQ_GRPC_ADDR="${ADDR}"

TEST_OUTPUT=$(mktemp)
if go test -run TestLaneqRealWireLifecycle -v -timeout 5m >"$TEST_OUTPUT" 2>&1; then
  echo "PASS: TestLaneqRealWireLifecycle passed"
  cat "$TEST_OUTPUT"
  TEST_RESULT=0
else
  echo "FAIL: TestLaneqRealWireLifecycle failed"
  cat "$TEST_OUTPUT"
  TEST_RESULT=1
fi

echo "Test complete (cleanup via trap)"
rm -f "$TEST_OUTPUT"
exit $TEST_RESULT
