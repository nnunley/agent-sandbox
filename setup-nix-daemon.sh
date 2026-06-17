#!/bin/bash
# Setup script for converged nix-daemon architecture:
# - Single persistent base "nix-server" container
# - Runs the SINGLE nix-daemon, only writer of shared /nix
# - Shared Incus filesystem volume (/nix) mounted read-write
# - Ephemeral worker containers mount same /nix volume read-write, use NIX_REMOTE=daemon

set -e

REMOTE="${REMOTE:-ndn-desktop}"
POOL="${POOL:-default}"
VOLUME_NAME="${VOLUME_NAME:-nix-shared}"
BASE_CONTAINER="${BASE_CONTAINER:-nix-server}"
IMAGE="${IMAGE:-images:nixos/25.11}"

echo "=== Step 1: Check if nix-shared volume exists ==="
if incus storage volume info $REMOTE:$POOL $VOLUME_NAME 2>/dev/null; then
    echo "✓ Volume $VOLUME_NAME already exists"
else
    echo "✗ Volume not found, creating..."
    incus storage volume create $REMOTE:$POOL $VOLUME_NAME -t filesystem
    echo "✓ Created $VOLUME_NAME"
fi

echo ""
echo "=== Step 2: Check if nix-server container exists ==="
if incus list $REMOTE: | grep -q $BASE_CONTAINER; then
    echo "✓ Container $BASE_CONTAINER already exists"
else
    echo "✗ Container not found, creating base nix-server..."
    incus launch $REMOTE:$IMAGE $BASE_CONTAINER --ephemeral=false
    echo "✓ Launched $BASE_CONTAINER"
fi

echo ""
echo "=== Step 3: Attach /nix volume to nix-server (read-write for daemon) ==="
# Try to add device; ignore if it already exists
incus config device add $REMOTE:$BASE_CONTAINER nix-store disk pool=$POOL source=$VOLUME_NAME path=/nix 2>/dev/null || \
    echo "  (device nix-store may already be attached)"

echo ""
echo "=== Step 4: Verify volume is mounted and daemon socket directory exists ==="
incus exec $REMOTE:$BASE_CONTAINER -- mkdir -p /nix/var/nix/daemon-socket
incus exec $REMOTE:$BASE_CONTAINER -- ls -ld /nix/var/nix/daemon-socket
echo "✓ Daemon socket directory ready"

echo ""
echo "=== Step 5: Start nix-daemon in nix-server container ==="
# This would require systemd service config in the NixOS container
# For now, document the daemon start command:
echo "  (Manual step: ensure nix-daemon runs in the nix-server container)"
echo "  Command: incus exec $REMOTE:$BASE_CONTAINER -- systemctl start nix-daemon"

echo ""
echo "=== Setup complete ==="
echo "Base container: $BASE_CONTAINER (runs single nix-daemon)"
echo "Shared volume: $VOLUME_NAME (mounted at /nix)"
echo "Socket path: /nix/var/nix/daemon-socket/socket"
echo ""
echo "Next: Configure worker launch to mount the same volume + set NIX_REMOTE=daemon"
