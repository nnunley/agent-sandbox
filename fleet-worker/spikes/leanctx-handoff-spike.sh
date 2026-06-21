#!/usr/bin/env bash
# Driver for the STORY-0034 ctx_handoff round-trip spike (SCENARIO-0077).
# Clones the golden snapshot, wires the nix-shared cache, pushes the worker tree +
# fleet token + probe, and runs the two-invocation handoff probe under nix develop.
# Reaps the worker on exit. Run from repo root; needs ~/.fleet-token and incus
# default remote = the cluster (ndn-desktop).
set -uo pipefail
INCUS="incus"; GOLDEN="fleet-dogfood-base/pristine"; W="spike-handoff"; ROOT="/Users/ndn/development/agent-sandbox"
cleanup(){ $INCUS stop "$W" >/dev/null 2>&1 || true; $INCUS delete "$W" >/dev/null 2>&1 || true; }
trap cleanup EXIT
$INCUS delete "$W" --force >/dev/null 2>&1 || true
echo "[handoff] clone golden"; $INCUS copy "$GOLDEN" "$W" >/dev/null
$INCUS config device add "$W" nix-shared disk pool=default source=nix-shared path=/srv/nix-shared readonly=true >/dev/null 2>&1 || true
$INCUS start "$W" >/dev/null
for i in $(seq 1 20); do st=$($INCUS exec "$W" -- systemctl is-system-running 2>/dev/null||true); case "$st" in running|degraded) break;; esac; sleep 1; done
for i in $(seq 1 30); do $INCUS exec "$W" -- bash -lc 'nix store ping --store daemon >/dev/null 2>&1' && break; $INCUS exec "$W" -- systemctl start nix-daemon.socket nix-daemon.service >/dev/null 2>&1||true; sleep 1; done
$INCUS file push -r "$ROOT/fleet-worker" "$W/root/" >/dev/null
printf '%s' "$(cat ~/.fleet-token)" | $INCUS exec "$W" -- tee /root/.fleet-token >/dev/null
$INCUS file push "$ROOT/fleet-worker/spikes/leanctx-handoff-probe.sh" "$W/root/probe.sh" >/dev/null
echo "[handoff] run probe (two real claude calls) inside nix develop"
$INCUS exec "$W" --env HOME=/root -- bash -lc 'nix develop /root/fleet-worker --accept-flake-config --no-sandbox --command bash /root/probe.sh' 2>&1
echo "[handoff] done"
