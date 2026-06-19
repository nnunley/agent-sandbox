#!/usr/bin/env bash
# populate-nix-cache.sh — publish the fleet-worker toolchain closure into the
# shared LOCAL nix binary cache so workers substitute claude-code / lean-ctx / go
# from file:///srv/nix-shared with ZERO network (Mac-off / offline ready).
#
# WHY inputDerivation (the non-obvious bit): `nix develop --profile` captures the
# shell ENVIRONMENT but its closure does NOT retain the devShell's buildInputs, so
# `nix copy <profile>` leaves claude-code/lean-ctx/go OUT of the cache. The devShell's
# `.inputDerivation` output references ALL build inputs — copying ITS closure is what
# actually populates the cache completely. (Verified 2026-06-19.)
#
# Run from the repo root. Idempotent: re-run after the fleet-worker flake changes.
#
#   scripts/populate-nix-cache.sh [PUBLISHER_CONTAINER] [CACHE_PATH]
#
# Defaults: publisher=nix-server, cache=file:///srv/nix-shared (RO-mounted on workers).
set -euo pipefail

PUBLISHER="${1:-nix-server}"
CACHE="${2:-file:///srv/nix-shared}"
FLAKE_LOCAL="fleet-worker"
FLAKE_REMOTE="/root/fleet-worker"
NIXFLAGS='--extra-experimental-features "nix-command flakes"'

# Use the agent-host wrapper / rtk proxy convention to keep incus output quiet.
INCUS="${INCUS:-rtk proxy incus}"

echo "==> 1/5 push $FLAKE_LOCAL flake to $PUBLISHER:$FLAKE_REMOTE"
$INCUS file push -r "$FLAKE_LOCAL" "$PUBLISHER$FLAKE_REMOTE/../" >/dev/null

echo "==> 2/5 build the devShell inputDerivation closure on $PUBLISHER (one-time numtide fetch)"
$INCUS exec "$PUBLISHER" -- bash -lc "
  set -e
  nix build $FLAKE_REMOTE#devShells.x86_64-linux.default.inputDerivation \
    --out-link /root/devshell-inputs --accept-flake-config --no-sandbox $NIXFLAGS
"

echo "==> 3/5 copy the full closure into the local cache ($CACHE)"
$INCUS exec "$PUBLISHER" -- bash -lc "
  set -e
  IN=\$(readlink -f /root/devshell-inputs)
  echo \"closure: \$IN ( \$(nix-store --query --requisites \"\$IN\" | wc -l) paths )\"
  nix copy --to $CACHE \"\$IN\" $NIXFLAGS
"

echo "==> 4/5 GC roots (the out-link /root/devshell-inputs pins the closure on $PUBLISHER)"
$INCUS exec "$PUBLISHER" -- bash -lc "nix-store --query --roots \$(readlink -f /root/devshell-inputs) $NIXFLAGS | head -3"

echo "==> 5/5 verify agent CLIs are in the cache"
$INCUS exec "$PUBLISHER" -- bash -lc '
  for p in claude-code-2 lean-ctx go-1.; do
    h=$(ls /nix/store | grep -m1 "^[a-z0-9]\{32\}-$p" ); hash=${h%%-*}
    [ -f "/srv/nix-shared/$hash.narinfo" ] && echo "  OK  $h" || echo "  MISS $p"
  done
'
echo "done — workers with file:///srv/nix-shared (worker-container.nix) now pull local-first."
