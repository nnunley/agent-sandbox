#!/usr/bin/env bash
set -euo pipefail

REMOTE="ndn-desktop"
CONTAINER="agent-host"

echo "==> Smoke test: agent-sandbox on ${REMOTE}:${CONTAINER}"

# 1. Check container is running
echo "--- Checking container status..."
incus exec "${REMOTE}:${CONTAINER}" -- true
echo "    Container is reachable."

# 2. Check KVM is available
echo "--- Checking /dev/kvm..."
incus exec "${REMOTE}:${CONTAINER}" -- test -c /dev/kvm
echo "    KVM is available."

# 3. Check firecracker binary
echo "--- Checking firecracker binary..."
incus exec "${REMOTE}:${CONTAINER}" -- firecracker --version
echo "    Firecracker is installed."

# 4. Check bridge exists
echo "--- Checking br-microvm bridge..."
incus exec "${REMOTE}:${CONTAINER}" -- ip addr show br-microvm
echo "    Bridge is up."

# 5. Check dnsmasq is running
echo "--- Checking dnsmasq..."
incus exec "${REMOTE}:${CONTAINER}" -- systemctl is-active dnsmasq
echo "    dnsmasq is running."

# 6. Check micro-VM service exists
echo "--- Checking microvm@test-vm service..."
incus exec "${REMOTE}:${CONTAINER}" -- systemctl status microvm@test-vm --no-pager || true

# 7. Try starting the test VM
echo "--- Starting test-vm..."
incus exec "${REMOTE}:${CONTAINER}" -- systemctl start microvm@test-vm

echo "--- Waiting 10s for VM to boot..."
sleep 10

# 8. Check VM is running
echo "--- Checking VM process..."
incus exec "${REMOTE}:${CONTAINER}" -- systemctl is-active microvm@test-vm
echo "    test-vm is running."

# 9. Stop the VM
echo "--- Stopping test-vm..."
incus exec "${REMOTE}:${CONTAINER}" -- systemctl stop microvm@test-vm
echo "    test-vm stopped."

echo ""
echo "==> Smoke test PASSED"
