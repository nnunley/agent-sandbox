#!/usr/bin/env bash
set -uo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"; fail=0
check() { if eval "$1"; then echo "ok: $2"; else echo "FAIL: $2"; fail=1; fi; }
# Build a tiny git repo + a diff that adds a MARKER; oracle greps for it.
tmp=$(mktemp -d); ( cd "$tmp" && git init -q && echo base > f.txt && git add . && git commit -qm base )
( cd "$tmp" && echo MARKER >> f.txt && git --no-pager diff --no-ext-diff > /tmp/df-grade.diff && git checkout -q -- f.txt )
printf '#!/usr/bin/env bash\ngrep -q MARKER f.txt\n' > /tmp/df-grade-oracle.sh; chmod +x /tmp/df-grade-oracle.sh
rm -rf /tmp/df-grade-out; mkdir -p /tmp/df-grade-out; cp /tmp/df-grade.diff /tmp/df-grade-out/worker.diff
out=$(DF_OUTDIR=/tmp/df-grade-out DF_REPO="$tmp" DF_REF=HEAD DF_ORACLE=/tmp/df-grade-oracle.sh \
  bash -c 'source '"$HERE"'/grade.sh'; echo "rc=$?")
check 'grep -q "\"pass\": *true" /tmp/df-grade-out/grade.json' "passing diff grades pass=true"
check 'echo "$out" | grep -q "rc=0"' "exit 0 on pass"

# Failing case: empty diff (no MARKER) → pass=false, non-zero.
rm -rf /tmp/df-grade-out2; mkdir -p /tmp/df-grade-out2; : > /tmp/df-grade-out2/worker.diff
out=$(DF_OUTDIR=/tmp/df-grade-out2 DF_REPO="$tmp" DF_REF=HEAD DF_ORACLE=/tmp/df-grade-oracle.sh \
  bash -c 'source '"$HERE"'/grade.sh'; echo "rc=$?")
check 'grep -q "\"pass\": *false" /tmp/df-grade-out2/grade.json' "no-MARKER diff grades pass=false"
check 'echo "$out" | grep -q "rc=1"' "exit 1 on fail"
exit $fail
