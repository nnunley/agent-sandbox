#!/usr/bin/env bash
# Real-cluster SCENARIO-0001/0056/0093/0094/0081: Go adapter ↔ live Temporal gRPC server
#
# This script:
# 1. Verifies Temporal (127.0.0.1:7233) and laneq (127.0.0.1:9999) are reachable
# 2. Cross-compiles gated live test binary for linux/amd64 (to run in container)
# 3. Pushes binary to agent-host container
# 4. Runs gated tests with TEMPORAL_LIVE=1
# 5. For restart test: optionally restarts Temporal service mid-flight
#
# Usage:
#   bash modules/incus-dispatcher/temporal/run-temporal-live.sh
#
# Exit codes:
#   0  PASS  — all tests passed
#   1  FAIL  — test failed or service unreachable
#   2  SKIP  — cross-compile or container access unavailable

set -uo pipefail

TEMPORAL_ADDR="${TEMPORAL_LIVE_ADDR:-127.0.0.1:7233}"
TEMPORAL_HOST="${TEMPORAL_ADDR%%:*}"
TEMPORAL_PORT="${TEMPORAL_ADDR##*:}"

LANEQ_ADDR="${LANEQ_LIVE_ADDR:-127.0.0.1:9999}"
LANEQ_HOST="${LANEQ_ADDR%%:*}"
LANEQ_PORT="${LANEQ_ADDR##*:}"

INCUS_REMOTE="${INCUS_REMOTE:-ndn-desktop}"
INCUS_CONTAINER="${INCUS_REMOTE}:agent-host"
TIMEOUT_SEC=30
TEST_BIN="/tmp/temporal-live.test"
REMOTE_TEST_BIN="/root/temporal-live.test"
TEST_OUTPUT=$(mktemp)

# Cleanup on exit
cleanup() {
	rm -f "$TEST_OUTPUT" "$TEST_BIN" 2>/dev/null || true
}
trap cleanup EXIT

# Check prerequisites: incus, go, nc
if ! command -v incus &>/dev/null; then
	echo "SKIP: incus not found"
	exit 2
fi

if ! command -v go &>/dev/null; then
	echo "SKIP: go not found"
	exit 2
fi

if ! command -v nc &>/dev/null; then
	echo "SKIP: nc (netcat) not found"
	exit 2
fi

echo "Verifying remote services..."

# Check Temporal reachability from the container
echo "  Checking Temporal at ${TEMPORAL_ADDR}..."
if ! incus exec "$INCUS_CONTAINER" -- nc -zv "$TEMPORAL_HOST" "$TEMPORAL_PORT" &>/dev/null; then
	echo "FAIL: Temporal at ${TEMPORAL_ADDR} is not reachable from container"
	exit 1
fi
echo "  ✓ Temporal reachable"

# Check laneq reachability from the container
echo "  Checking laneq at ${LANEQ_ADDR}..."
if ! incus exec "$INCUS_CONTAINER" -- nc -zv "$LANEQ_HOST" "$LANEQ_PORT" &>/dev/null; then
	echo "FAIL: laneq at ${LANEQ_ADDR} is not reachable from container"
	exit 1
fi
echo "  ✓ laneq reachable"

echo "Cross-compiling test binary for linux/amd64..."
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$TEST_DIR"

# Cross-compile: CGO_ENABLED=0 to avoid C dependencies
if ! CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -o "$TEST_BIN" ./; then
	echo "FAIL: Cross-compile failed"
	exit 1
fi

if [ ! -f "$TEST_BIN" ]; then
	echo "FAIL: Test binary not created"
	exit 1
fi
echo "✓ Test binary compiled: $TEST_BIN"

echo "Pushing binary to container..."
if ! incus file push "$TEST_BIN" "$INCUS_CONTAINER$REMOTE_TEST_BIN" 2>/dev/null; then
	echo "FAIL: Failed to push binary to container"
	exit 1
fi
echo "✓ Binary pushed"

echo "Running gated live tests..."
export TEMPORAL_LIVE=1
export TEMPORAL_LIVE_ADDR="${TEMPORAL_ADDR}"
export LANEQ_LIVE_ADDR="${LANEQ_ADDR}"

if incus exec "$INCUS_CONTAINER" -- env TEMPORAL_LIVE=1 TEMPORAL_LIVE_ADDR="${TEMPORAL_ADDR}" LANEQ_LIVE_ADDR="${LANEQ_ADDR}" \
	"$REMOTE_TEST_BIN" -test.run 'TestTemporal' -test.v -test.timeout 10m >"$TEST_OUTPUT" 2>&1; then
	echo "PASS: Tests passed"
	cat "$TEST_OUTPUT"
	TEST_RESULT=0
else
	echo "FAIL: Tests failed"
	cat "$TEST_OUTPUT"
	TEST_RESULT=1
fi

echo ""
echo "Test execution complete"
exit $TEST_RESULT
