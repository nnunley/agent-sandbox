#!/usr/bin/env bash
# STORY-0069 diagnosis driver: spin a worker, push the probe FILE, run it inside nix develop.
set -uo pipefail
INCUS="incus"
GOLDEN="fleet-dogfood-base/pristine"
W="spike-0069d"
ROOT="/Users/ndn/development/agent-sandbox"
cleanup(){ $INCUS stop "$W" >/dev/null 2>&1 || true; $INCUS delete "$W" >/dev/null 2>&1 || true; }
trap cleanup EXIT
$INCUS delete "$W" --force >/dev/null 2>&1 || true
echo "[d] clone golden"; $INCUS copy "$GOLDEN" "$W" >/dev/null
$INCUS config device add "$W" nix-shared disk pool=default source=nix-shared path=/srv/nix-shared readonly=true >/dev/null 2>&1 || true
$INCUS start "$W" >/dev/null
for i in $(seq 1 20); do st=$($INCUS exec "$W" -- systemctl is-system-running 2>/dev/null||true); case "$st" in running|degraded) break;; esac; sleep 1; done
for i in $(seq 1 30); do $INCUS exec "$W" -- bash -lc 'nix store ping --store daemon >/dev/null 2>&1' && break; $INCUS exec "$W" -- systemctl start nix-daemon.socket nix-daemon.service >/dev/null 2>&1||true; sleep 1; done
$INCUS file push -r "$ROOT/fleet-worker" "$W/root/" >/dev/null
$INCUS file push "$ROOT/fleet-worker/spikes/leanctx-proxy-probe.sh" "$W/root/probe.sh" >/dev/null
echo "[d] run probe inside nix develop"
$INCUS exec "$W" --env HOME=/root -- bash -lc 'nix develop /root/fleet-worker --accept-flake-config --no-sandbox --command bash /root/probe.sh' 2>&1
echo "[d] done"
