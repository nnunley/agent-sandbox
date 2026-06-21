#!/usr/bin/env bash
# Smoke-test the UPDATED fleet-worker/runner.sh end-to-end: lean-ctx enablement + proxy chain
# (STORY-0069) + fallback result.json (STORY-0072), fail-open. Inspects new artifacts before reap.
set -uo pipefail
INCUS="incus"; GOLDEN="fleet-dogfood-base/pristine"; W="smoke-runner"; ROOT="/Users/ndn/development/agent-sandbox"
cleanup(){ $INCUS stop "$W" >/dev/null 2>&1 || true; $INCUS delete "$W" >/dev/null 2>&1 || true; }
trap cleanup EXIT
$INCUS delete "$W" --force >/dev/null 2>&1 || true
echo "[smoke] clone golden"; $INCUS copy "$GOLDEN" "$W" >/dev/null
$INCUS config device add "$W" nix-shared disk pool=default source=nix-shared path=/srv/nix-shared readonly=true >/dev/null 2>&1 || true
$INCUS start "$W" >/dev/null
for i in $(seq 1 20); do st=$($INCUS exec "$W" -- systemctl is-system-running 2>/dev/null||true); case "$st" in running|degraded) break;; esac; sleep 1; done
for i in $(seq 1 30); do $INCUS exec "$W" -- bash -lc 'nix store ping --store daemon >/dev/null 2>&1' && break; $INCUS exec "$W" -- systemctl start nix-daemon.socket nix-daemon.service >/dev/null 2>&1||true; sleep 1; done
$INCUS file push -r "$ROOT/fleet-worker" "$W/root/" >/dev/null
printf '%s' "$(cat ~/.fleet-token)" | $INCUS exec "$W" -- tee /root/.fleet-token >/dev/null
# Set up $HOME/let-go (runner.sh cds there) with a large, compressible file + a brief.
$INCUS exec "$W" -- bash -lc '
  set -e; mkdir -p /root/let-go && cd /root/let-go && git init -q
  for i in $(seq 1 4000); do echo "line $i: lorem ipsum dolor sit amet consectetur $i"; done > big.txt
  git add -A && git -c user.email=a@b.c -c user.name=x commit -qm init
  printf "Use your Bash tool to run: cat big.txt   (it is large). Then reply with just DONE." > /root/brief.txt
'
echo "[smoke] run UPDATED runner.sh (120s cap)"
$INCUS exec "$W" --env HOME=/root --env IS_SANDBOX=1 -- bash -lc 'nix develop /root/fleet-worker --accept-flake-config --no-sandbox --command bash /root/fleet-worker/runner.sh 120' 2>&1 | tail -4
echo "===PROXY.OUT==="; $INCUS exec "$W" -- bash -lc 'sed "s/\x1b\[[0-9;]*m//g" /root/lean-ctx-proxy.out 2>/dev/null | head -12 || echo MISSING'
echo "===WORKER.LOG (tail)==="; $INCUS exec "$W" -- bash -lc 'tail -20 /root/worker.log' 2>&1 | sed "s/$(printf '\033')\[[0-9;]*m//g"
echo "===RESULT.JSON==="; $INCUS exec "$W" -- bash -lc 'cat /root/result.json 2>/dev/null || echo MISSING'
echo "===PROXY STATUS==="; $INCUS exec "$W" -- bash -lc 'cat /root/lean-ctx-proxy-status.txt 2>/dev/null | sed "s/\x1b\[[0-9;]*m//g" | grep -iE "process|requests|compressed|tokens|compression" || echo MISSING'
echo "===GAIN: Bridge OFF present?==="; $INCUS exec "$W" -- bash -lc 'grep -c "Bridge: OFF" /root/lean-ctx-gain.txt 2>/dev/null || echo "no gain file"'
echo "===WORKER.DIFF bytes==="; $INCUS exec "$W" -- bash -lc 'wc -c < /root/worker.diff 2>/dev/null || echo MISSING'
echo "[smoke] done"
