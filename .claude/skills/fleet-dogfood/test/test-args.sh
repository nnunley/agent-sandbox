#!/usr/bin/env bash
set -uo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"
fail=0
check() { if eval "$1"; then echo "ok: $2"; else echo "FAIL: $2"; fail=1; fi; }

# Missing required args → non-zero exit + a clear message naming the missing flag.
out=$(bash "$HERE/fleet-dogfood.sh" --name x --repo /tmp --oracle /tmp/o 2>&1); rc=$?
check '[ "$rc" -ne 0 ]' "missing --brief exits non-zero"
check 'echo "$out" | grep -qi "brief"' "error names the missing --brief"

# --help exits 0 and lists the flags.
out=$(bash "$HERE/fleet-dogfood.sh" --help 2>&1); rc=$?
check '[ "$rc" -eq 0 ]' "--help exits 0"
check 'echo "$out" | grep -q -- "--oracle"' "--help lists --oracle"

exit $fail
