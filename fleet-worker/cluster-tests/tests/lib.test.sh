#!/usr/bin/env bash
# Local CI test for the cluster-verification harness PURE logic (stats + gates).
# No cluster, no incus — runnable on the Mac. The cluster-facing probes self-skip
# when agent-host is unreachable; this pins the measurement + acceptance-gate math.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/../lib.sh" || { echo "FAIL: cannot source lib.sh"; exit 1; }
fails=0
check() { if [ "$1" != "$2" ]; then echo "FAIL: $3 (want '$2' got '$1')"; fails=$((fails+1)); else echo "ok: $3"; fi; }

# compute_stats over a known sample → deterministic mean/p50/p99/min/max.
stats="$(compute_stats <<<$'10\n20\n30\n40\n100')"
check "$(stat_field "$stats" N)"    "5"   "compute_stats N"
check "$(stat_field "$stats" mean)" "40"  "compute_stats mean (200/5)"
check "$(stat_field "$stats" min)"  "10"  "compute_stats min"
check "$(stat_field "$stats" max)" "100"  "compute_stats max"
check "$(stat_field "$stats" p50)"  "30"  "compute_stats p50"
check "$(stat_field "$stats" p99)" "100"  "compute_stats p99"

# Robustness: blank/non-numeric lines (e.g. a trailing newline from accumulation) are
# ignored, NOT counted as 0 — else N inflates and min collapses to 0.
dirty="$(printf '833\n840\n\n1135\n')"
ds="$(compute_stats <<<"$dirty")"
check "$(stat_field "$ds" N)"   "3"    "compute_stats ignores blank lines (N)"
check "$(stat_field "$ds" min)" "833"  "compute_stats ignores blank lines (min not 0)"

# assert_le: actual <= gate passes (rc 0); actual > gate fails (rc 1).
assert_le 50 100 "under gate"   >/dev/null 2>&1; check "$?" "0" "assert_le passes when under gate"
assert_le 150 100 "over gate"   >/dev/null 2>&1; check "$?" "1" "assert_le fails when over gate"
assert_le 100 100 "equal gate"  >/dev/null 2>&1; check "$?" "0" "assert_le passes at the gate (<=)"

# assert_true: boolean predicate gate.
assert_true 1 "isolated"        >/dev/null 2>&1; check "$?" "0" "assert_true passes on 1"
assert_true 0 "not isolated"    >/dev/null 2>&1; check "$?" "1" "assert_true fails on 0"

if [ "$fails" -eq 0 ]; then echo "PASS: harness pure logic"; exit 0; fi
echo "FAILED: $fails"; exit 1
