#!/usr/bin/env bash
# Real-wire PASETO grant auth e2e test (SCENARIO-0117/0118/0119)
#
# This script orchestrates the cross-language PASETO auth contract test:
# - Go grantauth package (issuer MintGrant + client SignProof)
# - Python laneq verifier (grant + proof validation)
# - Real gRPC wire
#
# The test proves:
#   SCENARIO-0117: enforce mode accepts authenticated client
#   SCENARIO-0118: enforce mode rejects unauthenticated client (Unauthenticated code)
#   SCENARIO-0119: log-only mode allows unauthenticated client (safe rollout)
#
# Usage:
#   bash modules/incus-dispatcher/queue/run-laneq-auth-wire.sh
#
# Environment:
#   LANEQ_SRC      path to laneq repo (default: /Users/ndn/development/laneq)
#
# Exit codes:
#   0  PASS  — all scenarios passed
#   1  FAIL  — test failed
#   2  SKIP  — prerequisites missing (uv, python3, laneq repo)

set -uo pipefail

LANEQ_SRC="${LANEQ_SRC:-/Users/ndn/development/laneq}"
TIMEOUT_SEC=5

# Check prerequisites
if ! command -v uv &>/dev/null; then
  echo "SKIP: uv not found (Python environment required)"
  exit 2
fi

if ! python3 --version &>/dev/null; then
  echo "SKIP: python3 not found"
  exit 2
fi

if [ ! -d "$LANEQ_SRC" ]; then
  echo "SKIP: LANEQ_SRC not found at $LANEQ_SRC"
  exit 2
fi

echo "Real-wire PASETO auth e2e test"
echo "  LANEQ_SRC=$LANEQ_SRC"
echo ""

# Run the test
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$TEST_DIR"

export LANEQ_AUTH_WIRE=1
export LANEQ_SRC

TEST_OUTPUT=$(mktemp)
if timeout "${TIMEOUT_SEC}m" go test -run TestLaneqAuthWire -v >"$TEST_OUTPUT" 2>&1; then
  echo "PASS: TestLaneqAuthWire passed"
  echo ""
  cat "$TEST_OUTPUT"
  TEST_RESULT=0
else
  echo "FAIL: TestLaneqAuthWire failed (exit code $?)"
  echo ""
  cat "$TEST_OUTPUT"
  TEST_RESULT=1
fi

rm -f "$TEST_OUTPUT"
exit $TEST_RESULT
