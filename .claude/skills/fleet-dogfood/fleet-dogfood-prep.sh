#!/usr/bin/env bash
# fleet-dogfood-prep — configure a base worker, measure fresh-launch readiness, and
# choose fresh-launch vs golden-snapshot. Idempotent.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"; source "$HERE/lib.sh"
REPO_ROOT="$(cd "$HERE/../../.." && pwd)"   # .claude/skills/fleet-dogfood -> repo root
BASE="fleet-dogfood-base"; THRESHOLD=120
while [ $# -gt 0 ]; do case "$1" in --threshold) THRESHOLD="$2"; shift 2;; *) die "unknown arg: $1";; esac; done

# Recreate the base worker from the declarative config.
$INCUS delete "$BASE" --force >/dev/null 2>&1 || true
log "launch + configure base worker ($BASE) from worker-container.nix"
$INCUS launch images:nixos/25.11 "$BASE" >/dev/null
for i in $(seq 1 20); do st=$($INCUS exec "$BASE" -- systemctl is-system-running 2>/dev/null || true); case "$st" in running|degraded) break;; esac; done
$INCUS file push "$REPO_ROOT/fleet-worker/worker-container.nix" "$BASE/etc/nixos/configuration.nix"
# nix-shared RO mount so the local cache is visible during the rebuild + nix develop.
$INCUS config device add "$BASE" nix-shared disk pool=default source=nix-shared path=/srv/nix-shared readonly=true >/dev/null 2>&1 || true
$INCUS exec "$BASE" -- bash -lc 'export NIX_CONFIG="sandbox = false"; nix-channel --update && nixos-rebuild switch' >/dev/null

# Measure: time the worker resolving the toolchain from the local cache (the slow bit a golden would skip).
log "measuring fresh-worker toolchain readiness (local-cache nix develop)…"
$INCUS file push -r "$REPO_ROOT/fleet-worker" "$BASE/home/worker/" >/dev/null
$INCUS exec "$BASE" -- chown -R worker /home/worker/fleet-worker >/dev/null 2>&1 || true
start=$(date +%s)
$INCUS exec "$BASE" --user 1000 --env HOME=/home/worker -- bash -lc \
  'nix develop /home/worker/fleet-worker --accept-flake-config --no-sandbox --command claude --version' >/dev/null
elapsed=$(( $(date +%s) - start ))
log "fresh worker ready in ${elapsed}s (threshold ${THRESHOLD}s)"

if [ "$elapsed" -le "$THRESHOLD" ]; then
  echo "fresh" > "$HERE/.mode"
  log "mode=fresh (local cache fast enough; no golden snapshot needed)"
else
  $INCUS snapshot create "$BASE" pristine >/dev/null
  echo "golden:$BASE/pristine" > "$HERE/.mode"
  log "mode=golden ($BASE/pristine) — fresh launch exceeded threshold"
fi
