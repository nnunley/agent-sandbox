# Agent Sandbox: Initial Deployment Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Get a Firecracker micro-VM booting inside a NixOS container on the Incus remote, with bridge networking and a smoke test passing.

**Architecture:** NixOS container on `ndn-desktop` Incus remote with KVM passthrough. microvm.nix manages Firecracker VMs declaratively. A shared `/nix/store` volume (btrfs subvolume) is mounted into the container — this means builds cache across containers and future NixOS containers can share the same store. All Nix builds happen inside the container (no local nix install required). Scripts drive deployment from the Mac via `incus` CLI.

**Tech Stack:** Nix flakes, microvm.nix, Firecracker, NixOS 25.11, Incus, bash scripts

**Discoveries:**
- No `nix` installed locally on Mac. Build inside the container.
- Remote is `ndn-desktop` at 192.168.86.49, btrfs storage pool named `default`.
- Three existing NixOS containers (`nativelink-worker`, `omnisearch`, `oxiz-bench`) each with independent `/nix/store` — they can be migrated to the shared volume later.
- AMD CPU with SVM (AMD-V) — KVM supported. Existing containers don't have KVM passthrough.

---

### Task 1: Create shared /nix volume on Incus remote

This creates a btrfs storage volume that will be shared across all NixOS containers. Content-addressed `/nix/store` is safe to share.

- [ ] **Step 1: Create the volume**

```bash
incus storage volume create ndn-desktop:default nix-store
```

- [ ] **Step 2: Verify it was created**

```bash
incus storage volume list ndn-desktop:default | grep nix-store
```

Expected: a row showing `custom | nix-store | ... | filesystem | 0`

---

### Task 2: Initialize the repo and flake

**Files:**
- Create: `flake.nix`
- Create: `.gitignore`

- [ ] **Step 1: Init git repo**

```bash
cd ~/development/agent-sandbox
git init
```

- [ ] **Step 2: Create .gitignore**

```
result
.direnv
```

- [ ] **Step 3: Create flake.nix with minimal inputs**

```nix
{
  description = "Agent Sandbox: Firecracker micro-VMs on Incus via NixOS";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    microvm = {
      url = "github:microvm-nix/microvm.nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, microvm }: let
    system = "x86_64-linux";
    pkgs = nixpkgs.legacyPackages.${system};
  in {
    nixosConfigurations.agent-host = nixpkgs.lib.nixosSystem {
      inherit system;
      modules = [
        microvm.nixosModules.host
        ./host/configuration.nix
      ];
    };
  };
}
```

- [ ] **Step 4: Commit**

```bash
git add flake.nix .gitignore
git commit -m "init: flake with nixpkgs and microvm.nix inputs"
```

---

### Task 3: Host container NixOS configuration

**Files:**
- Create: `host/configuration.nix`
- Create: `host/networking.nix`

- [ ] **Step 1: Create host/configuration.nix**

NixOS config for the container on the Incus remote. Imports microvm.nix host module (via flake.nix), sets up the system, enables Firecracker.

```nix
# host/configuration.nix
{ config, pkgs, lib, ... }:

{
  imports = [
    ./networking.nix
  ];

  # Basic system
  system.stateVersion = "25.11";
  time.timeZone = "UTC";

  # Nix configuration - enable flakes for rebuilds inside the container
  nix.settings = {
    experimental-features = [ "nix-command" "flakes" ];
    trusted-users = [ "root" ];
  };

  # Firecracker needs KVM
  # /dev/kvm is passed through by Incus device config, not NixOS
  users.groups.kvm = {};

  # Minimal packages for the host
  environment.systemPackages = with pkgs; [
    firecracker
    git
    jq
    htop
  ];

  # Shared storage for VM artifacts
  systemd.tmpfiles.rules = [
    "d /var/lib/agent-sandbox 0755 root root -"
    "d /var/lib/agent-sandbox/runs 0755 root root -"
  ];

  # microvm.nix host defaults
  microvm.host.enable = true;

  # Test micro-VM using base guest image
  microvm.vms.test-vm = {
    config = {
      imports = [ ../guests/base.nix ];
    };
  };
}
```

- [ ] **Step 2: Create host/networking.nix**

Bridge, dnsmasq, NAT, and IP forwarding for micro-VM networking.

```nix
# host/networking.nix
{ config, pkgs, lib, ... }:

{
  # Enable IP forwarding for NAT
  boot.kernel.sysctl = {
    "net.ipv4.ip_forward" = 1;
  };

  networking = {
    hostName = "agent-host";

    # Bridge for micro-VMs
    bridges.br-microvm.interfaces = [];

    interfaces.br-microvm = {
      ipv4.addresses = [{
        address = "192.168.100.1";
        prefixLength = 24;
      }];
    };

    # NAT from micro-VM bridge to container's outbound interface
    nat = {
      enable = true;
      internalInterfaces = [ "br-microvm" ];
      externalInterface = "eth0";
    };

    # Firewall: allow DHCP and DNS from micro-VMs
    firewall = {
      enable = true;
      trustedInterfaces = [ "br-microvm" ];
    };
  };

  # DHCP + DNS for micro-VMs
  services.dnsmasq = {
    enable = true;
    settings = {
      interface = "br-microvm";
      bind-interfaces = true;
      dhcp-range = "192.168.100.10,192.168.100.254,24h";
      dhcp-option = [
        "3,192.168.100.1"   # gateway
        "6,192.168.100.1"   # DNS
      ];
    };
  };
}
```

- [ ] **Step 3: Commit**

```bash
git add host/
git commit -m "feat: host NixOS config with bridge networking, dnsmasq, NAT"
```

---

### Task 4: Base guest micro-VM definition

**Files:**
- Create: `guests/base.nix`

- [ ] **Step 1: Create guests/base.nix**

Minimal NixOS guest image for Firecracker.

```nix
# guests/base.nix
{ config, pkgs, lib, ... }:

{
  microvm = {
    hypervisor = "firecracker";
    mem = 4096;
    vcpu = 2;

    interfaces = [{
      type = "tap";
      id = "vm-test";
      mac = "02:00:00:00:00:01";
    }];

    volumes = [{
      mountPoint = "/output";
      image = "output.img";
      size = 2048;
    }];
  };

  # Minimal NixOS guest config
  system.stateVersion = "25.11";

  documentation.enable = false;
  environment.noXlibs = lib.mkDefault true;

  # Networking via DHCP from host's dnsmasq
  networking = {
    hostName = "agent-vm";
    useDHCP = false;
    interfaces.eth0.useDHCP = true;
  };

  # Agent user
  users.users.agent = {
    isNormalName = true;
    extraGroups = [ "wheel" ];
    home = "/home/agent";
  };
  security.sudo.wheelNeedsPassword = false;

  # Core tools
  environment.systemPackages = with pkgs; [
    git
    curl
    wget
    jq
    ripgrep
    fd
    tree
    gcc
    gnumake
    pkg-config
  ];
}
```

- [ ] **Step 2: Commit**

```bash
git add guests/base.nix
git commit -m "feat: base guest micro-VM definition with Firecracker"
```

---

### Task 5: Deploy script

**Files:**
- Create: `scripts/deploy.sh`

- [ ] **Step 1: Create scripts/deploy.sh**

Creates or updates the NixOS container on the Incus remote. Pushes flake source and builds inside the container. Mounts the shared `/nix` volume.

```bash
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
  # The stock NixOS image has /nix populated. We need to:
  # 1. Stop the container
  # 2. Mount the shared volume
  # 3. If the shared volume is empty, copy the existing /nix into it
  # 4. Start the container
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

  # If the shared /nix volume is empty (first use), we need to seed it
  # from the stock NixOS image. The container won't boot properly without
  # /nix/store populated. Check by looking for /nix/store/nix-path-registration.
  echo "==> Checking if shared /nix needs seeding..."
  if ! incus exec "${REMOTE}:${CONTAINER}" -- test -d /nix/store; then
    echo "==> Shared /nix is empty. This is the first container using it."
    echo "    The container may have failed to boot properly."
    echo "    Workaround: destroy, seed the volume from an existing container, and re-init."
    echo ""
    echo "    To seed from an existing NixOS container:"
    echo "      incus stop ${REMOTE}:nativelink-worker"
    echo "      incus config device add ${REMOTE}:nativelink-worker nix-store disk pool=default source=${NIX_VOLUME} path=/nix-shared"
    echo "      incus start ${REMOTE}:nativelink-worker"
    echo "      incus exec ${REMOTE}:nativelink-worker -- cp -a /nix/. /nix-shared/"
    echo "      incus stop ${REMOTE}:nativelink-worker"
    echo "      incus config device remove ${REMOTE}:nativelink-worker nix-store"
    echo "      incus start ${REMOTE}:nativelink-worker"
    echo ""
    echo "    Then re-run: ./scripts/deploy.sh destroy && ./scripts/deploy.sh init"
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
```

- [ ] **Step 2: Make executable**

```bash
chmod +x scripts/deploy.sh
```

- [ ] **Step 3: Commit**

```bash
git add scripts/deploy.sh
git commit -m "feat: deploy script with shared /nix volume support"
```

---

### Task 6: Smoke test script

**Files:**
- Create: `scripts/smoke-test.sh`

- [ ] **Step 1: Create scripts/smoke-test.sh**

```bash
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
```

- [ ] **Step 2: Make executable and commit**

```bash
chmod +x scripts/smoke-test.sh
git add scripts/smoke-test.sh
git commit -m "feat: smoke test script for validating deployment"
```

---

### Task 7: Deploy and iterate

This is the hands-on task. Expect issues.

- [ ] **Step 1: Create the shared /nix volume**

```bash
incus storage volume create ndn-desktop:default nix-store
```

- [ ] **Step 2: Seed the shared volume from an existing NixOS container**

The shared volume starts empty. We need to populate it from one of the existing NixOS containers before agent-host can boot with it mounted at `/nix`.

```bash
# Temporarily mount the shared volume alongside the existing /nix
incus config device add ndn-desktop:nativelink-worker nix-shared disk \
  pool=default source=nix-store path=/nix-shared

# Copy the existing store into the shared volume
incus exec ndn-desktop:nativelink-worker -- bash -c 'cp -a /nix/. /nix-shared/'

# Remove the temporary mount
incus config device remove ndn-desktop:nativelink-worker nix-shared
```

This may take a while depending on the store size. The copy preserves all hardlinks and permissions.

- [ ] **Step 3: Run deploy init**

```bash
cd ~/development/agent-sandbox
./scripts/deploy.sh init
```

Watch for errors. Common issues:
- `nixos-rebuild` may not be in PATH on the stock image → use full path `/run/current-system/sw/bin/nixos-rebuild`
- The stock NixOS 25.11 image may have a different stateVersion → check with `incus exec ndn-desktop:agent-host -- cat /etc/os-release`
- `security.nesting` may not be sufficient for all iptables operations → may need `security.syscalls.intercept.mknod=true`
- microvm.nix may need kernel modules not available in a container (containers share host kernel)
- The shared /nix mount may conflict with the stock image's /nix contents → container must boot with the shared volume from the start

- [ ] **Step 4: Debug and fix any issues**

For each error:
1. Read the error message
2. Check if it's a container privilege issue
3. Check if it's a missing package or module
4. Fix the relevant `.nix` file or deploy script
5. Run `./scripts/deploy.sh update` to apply fixes

- [ ] **Step 5: Run smoke test**

```bash
./scripts/smoke-test.sh
```

If the smoke test passes:
- NixOS container on remote ✓
- Shared /nix volume working ✓
- KVM passthrough ✓
- Firecracker installed ✓
- Bridge networking ✓
- dnsmasq ✓
- micro-VM can start and stop ✓

- [ ] **Step 6: Commit any fixes**

```bash
cd ~/development/agent-sandbox
git add -A
git commit -m "fix: deployment issues discovered during first deploy"
```

---

### Task 8: Clean script

**Files:**
- Create: `scripts/clean.sh`

- [ ] **Step 1: Create scripts/clean.sh**

```bash
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
```

- [ ] **Step 2: Make executable and commit**

```bash
chmod +x scripts/clean.sh
git add scripts/clean.sh
git commit -m "feat: clean script to list/stop all micro-VMs"
```
