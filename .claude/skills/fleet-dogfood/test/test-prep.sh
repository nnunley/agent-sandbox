#!/usr/bin/env bash
set -uo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"; fail=0
check() { if eval "$1"; then echo "ok: $2"; else echo "FAIL: $2"; fail=1; fi; }
bash "$HERE/fleet-dogfood-prep.sh" --threshold 120 2>&1 | tee /tmp/df-prep.log
check 'grep -qE "ready in [0-9]+s" /tmp/df-prep.log' "prep reports a measured readiness time"
check 'test -f "$HERE/.mode"' ".mode file written"
check 'grep -qE "^(fresh|golden:)" "$HERE/.mode"' ".mode is fresh or golden:<snap>"
exit $fail
