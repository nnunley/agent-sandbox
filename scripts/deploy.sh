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

build_and_activate() {
  echo "==> Building NixOS configuration..."
  local build_path
  build_path=$(incus exec "${REMOTE}:${CONTAINER}" -- bash -c \
    "cd ${FLAKE_DIR} && nix --extra-experimental-features 'nix-command flakes' build .#nixosConfigurations.agent-host.config.system.build.toplevel --no-link --print-out-paths")

  echo "==> Built: ${build_path}"
  echo "==> Setting system profile..."
  incus exec "${REMOTE}:${CONTAINER}" -- \
    nix-env -p /nix/var/nix/profiles/system --set "${build_path}"

  echo "==> Registering for next boot..."
  incus exec "${REMOTE}:${CONTAINER}" -- \
    "${build_path}/bin/switch-to-configuration" boot

  echo "==> Restarting container to activate..."
  incus restart "${REMOTE}:${CONTAINER}" --force
  wait_for_container
}

cmd_init() {
  echo "==> Creating NixOS container on ${REMOTE}..."

  # Launch NixOS container
  incus launch images:nixos/25.11 "${REMOTE}:${CONTAINER}"
  wait_for_container

  # Configure KVM passthrough
  echo "==> Configuring KVM passthrough..."
  incus config device add "${REMOTE}:${CONTAINER}" kvm unix-char path=/dev/kvm

  # Enable nesting for iptables/NAT
  echo "==> Enabling security.nesting..."
  incus config set "${REMOTE}:${CONTAINER}" security.nesting=true

  # Restart to pick up device and config changes
  incus restart "${REMOTE}:${CONTAINER}" --force
  wait_for_container

  # Verify KVM is available
  echo "==> Verifying KVM..."
  incus exec "${REMOTE}:${CONTAINER}" -- ls -la /dev/kvm

  # Seed the shared /nix volume from this container's store
  echo "==> Seeding shared /nix volume from container's store..."
  incus config device add "${REMOTE}:${CONTAINER}" nix-shared disk \
    pool=default source="${NIX_VOLUME}" path=/nix-shared
  incus exec "${REMOTE}:${CONTAINER}" -- bash -c 'cp -an /nix/. /nix-shared/'
  incus config device remove "${REMOTE}:${CONTAINER}" nix-shared

  # Stop, mount shared /nix, restart
  echo "==> Switching to shared /nix volume..."
  incus stop "${REMOTE}:${CONTAINER}" --force
  incus config device add "${REMOTE}:${CONTAINER}" nix-store disk \
    pool=default source="${NIX_VOLUME}" path=/nix
  incus start "${REMOTE}:${CONTAINER}"
  wait_for_container

  # Push source and build
  push_source
  build_and_activate

  echo ""
  echo "==> Deploy complete! Container '${CONTAINER}' is running on '${REMOTE}'."
  echo "    Shared /nix volume: ${NIX_VOLUME}"
  echo "    To update: ./scripts/deploy.sh update"
  echo "    To check:  incus exec ${REMOTE}:${CONTAINER} -- systemctl status"
}

cmd_update() {
  echo "==> Updating ${REMOTE}:${CONTAINER}..."

  push_source
  build_and_activate

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
