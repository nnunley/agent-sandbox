#!/usr/bin/env bash
set -euo pipefail

REMOTE="ndn-desktop"
CONTAINER="agent-host"
FLAKE_DIR="/etc/agent-sandbox"
NIX_VOLUME="nix-store"
SCRIPT_DIR="$(cd "$(dirname "$0")"; pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.."; pwd)"

usage() {
  echo "Usage: deploy.sh [init|update|destroy]"
  exit 1
}

wait_for_container() {
  echo "==> Waiting for container to be ready..."
  local attempts=0
  while ! incus exec "${REMOTE}:${CONTAINER}" -- true 2>/dev/null; do
    attempts=$((attempts + 1))
    if [ $attempts -gt 30 ]; then
      echo "ERROR: Container failed to become ready after 30 attempts"
      exit 1
    fi
    sleep 2
  done
}

push_source() {
  echo "==> Pushing flake source to ${REMOTE}:${CONTAINER}..."
  incus exec "${REMOTE}:${CONTAINER}" -- mkdir -p "${FLAKE_DIR}"

  # Tar the flake source (excluding .git, result, docs) and push
  tar -cf - \
    --exclude='.git' \
    --exclude='result' \
    --exclude='docs' \
    -C "${REPO_DIR}" . \
    | incus exec "${REMOTE}:${CONTAINER}" -- tar -xf - -C "${FLAKE_DIR}"

  echo "==> Source pushed."
}

cmd_init() {
  echo "==> Creating NixOS container on ${REMOTE}..."

  # Launch NixOS container
  incus launch images:nixos/25.11 "${REMOTE}:${CONTAINER}"
  wait_for_container

  # Mount shared /nix volume
  echo "==> Mounting shared /nix volume..."
  incus stop "${REMOTE}:${CONTAINER}"

  # Add the shared nix-store volume mounted at /nix
  incus config device add "${REMOTE}:${CONTAINER}" nix-store disk \
    pool=default source="${NIX_VOLUME}" path=/nix

  # Configure KVM passthrough
  echo "==> Configuring KVM passthrough..."
  incus config device add "${REMOTE}:${CONTAINER}" kvm unix-char path=/dev/kvm

  # Enable nesting for iptables/NAT
  echo "==> Enabling security.nesting..."
  incus config set "${REMOTE}:${CONTAINER}" security.nesting=true

  # Start with new config
  incus start "${REMOTE}:${CONTAINER}"
  wait_for_container

  # Check if /nix/store exists (shared volume was seeded)
  echo "==> Checking if shared /nix is seeded..."
  if ! incus exec "${REMOTE}:${CONTAINER}" -- test -d /nix/store; then
    echo "ERROR: Shared /nix volume is empty. Seed it first:"
    echo ""
    echo "  # Mount temporarily on an existing NixOS container:"
    echo "  incus config device add ${REMOTE}:nativelink-worker nix-shared disk pool=default source=${NIX_VOLUME} path=/nix-shared"
    echo "  incus exec ${REMOTE}:nativelink-worker -- bash -c 'cp -a /nix/. /nix-shared/'"
    echo "  incus config device remove ${REMOTE}:nativelink-worker nix-shared"
    echo ""
    echo "  Then re-run: ./scripts/deploy.sh destroy && ./scripts/deploy.sh init"
    exit 1
  fi

  # Verify KVM is available
  echo "==> Verifying KVM..."
  incus exec "${REMOTE}:${CONTAINER}" -- ls -la /dev/kvm

  # Push source and rebuild
  push_source

  echo "==> Running nixos-rebuild switch (this will take a while on first run)..."
  incus exec "${REMOTE}:${CONTAINER}" -- bash -c \
    "cd ${FLAKE_DIR} && nixos-rebuild switch --flake .#agent-host"

  echo ""
  echo "==> Deploy complete! Container '${CONTAINER}' is running on '${REMOTE}'."
  echo "    Shared /nix volume: ${NIX_VOLUME}"
  echo "    To update: ./scripts/deploy.sh update"
  echo "    To check:  incus exec ${REMOTE}:${CONTAINER} -- systemctl status"
}

cmd_update() {
  echo "==> Updating ${REMOTE}:${CONTAINER}..."

  push_source

  echo "==> Running nixos-rebuild switch..."
  incus exec "${REMOTE}:${CONTAINER}" -- bash -c \
    "cd ${FLAKE_DIR} && nixos-rebuild switch --flake .#agent-host"

  echo "==> Update complete."
}

cmd_destroy() {
  echo "==> Destroying ${REMOTE}:${CONTAINER}..."
  incus delete "${REMOTE}:${CONTAINER}" --force
  echo "==> Destroyed."
  echo "    Note: shared /nix volume '${NIX_VOLUME}' was NOT deleted."
  echo "    To delete it: incus storage volume delete ${REMOTE}:default ${NIX_VOLUME}"
}

case "${1:-}" in
  init)    cmd_init ;;
  update)  cmd_update ;;
  destroy) cmd_destroy ;;
  *)       usage ;;
esac
