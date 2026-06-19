#!/usr/bin/env bash
set -uo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"; fail=0
check() { if eval "$1"; then echo "ok: $2"; else echo "FAIL: $2"; fail=1; fi; }

# Tiny repo (fast); worker creates a NEW file (also exercises runner.sh add -N).
repo=$(mktemp -d); ( cd "$repo" && git init -q && echo hi > README.md && git add . && git commit -qm init )
printf 'Create a NEW file SMOKE.txt at the repo root containing exactly the line: dogfood smoke ok\nDo not commit.\n' > /tmp/df-brief.txt
printf '#!/usr/bin/env bash\ngrep -qx "dogfood smoke ok" SMOKE.txt\n' > /tmp/df-oracle.sh; chmod +x /tmp/df-oracle.sh

FLEET_TOKEN="${FLEET_TOKEN:-$(cat ~/.fleet-token 2>/dev/null)}" \
bash "$HERE/fleet-dogfood.sh" --name smoke --brief /tmp/df-brief.txt \
  --repo "$repo" --ref HEAD --oracle /tmp/df-oracle.sh --output-dir /tmp/df-smoke --timeout 600
rc=$?
check 'test -f /tmp/df-smoke/worker.diff' "worker.diff harvested"
check 'test -f /tmp/df-smoke/events.jsonl' "events.jsonl harvested"
check 'test -s /tmp/df-smoke/worker.diff' "worker.diff is non-empty (new file captured by add -N)"
check 'grep -q SMOKE.txt /tmp/df-smoke/worker.diff' "diff contains the new SMOKE.txt"
echo "dispatch exit=$rc; grade=$(cat /tmp/df-smoke/grade.json 2>/dev/null)"
exit $fail
