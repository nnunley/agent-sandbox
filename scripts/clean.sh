#!/usr/bin/env bash
set -euo pipefail

REMOTE="ndn-desktop"
CONTAINER="agent-host"

echo "==> Listing all micro-VMs on ${REMOTE}:${CONTAINER}..."
incus exec "${REMOTE}:${CONTAINER}" -- bash -c \
  'systemctl list-units "microvm@*" --no-pager --no-legend 2>/dev/null || echo "(none)"'

if [ "${1:-}" = "--force" ]; then
  echo "==> Stopping all micro-VMs..."
  incus exec "${REMOTE}:${CONTAINER}" -- bash -c \
    'for unit in $(systemctl list-units "microvm@*" --no-pager --no-legend | awk "{print \$1}"); do
      echo "  Stopping $unit..."
      systemctl stop "$unit"
    done'
  echo "==> All micro-VMs stopped."
else
  echo ""
  echo "Run with --force to stop all VMs."
fi
