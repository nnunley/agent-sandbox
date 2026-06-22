# guests/coordinator-vm.nix
#
# STORY-0007 — the DURABLE coordinator micro-VM (Firecracker, real kernel). Unlike the
# per-task worker-vm (hard tier) and the minimal test-vm, this VM stays UP across tasks and
# is the durable layer that:
#   - hosts the coordinator daemon (`dispatcher serve`, one persistent process — D3/AC-2)
#   - carries a WARM /nix store (writable overlay over the shared read-only host store) so
#     in-guest disposable units (STORY-0008) and the nspawn fast tier (STORY-0021) substitute
#     prebuilt closures with zero rebuild (AC-1)
#   - is nspawn-capable (real kernel) — the fast tier can't run in the unprivileged agent-host
#     LXC, only inside this guest (see host/configuration.nix)
#
# Reached over br-microvm at a STATIC 10.88.0.2 (outside dnsmasq's .10–.254 DHCP range) so the
# cluster-verification harness has a stable SSH target. SSH'd into FROM agent-host (which holds
# the root@agent-host private key).
#
# NOTE: baking the `dispatcher` binary in as a live systemd service is a follow-up — it needs
# the incus-client-heavy binary Nix-packaged (vendorHash) AND a real queue to drain (laneq,
# ITER-0006). The daemon ENTRYPOINT (`dispatcher serve`) and loop are delivered + tested in Go.
{ pkgs, ... }:
{
  imports = [ ./fleet-ready.nix ];

  microvm = {
    hypervisor = "firecracker";
    mem = 4096;
    vcpu = 2;

    # Unique tap + MAC (must not collide with test-vm ...01 / worker-vm ...02).
    interfaces = [{ type = "tap"; id = "vm-coord"; mac = "02:00:00:00:00:03"; }];

    # Warm /nix store: writable overlay over the read-only shared host store so the guest can
    # realize new paths (AC-1) without a rebuild.
    writableStoreOverlay = "/nix/.rw-store";
    volumes = [
      { mountPoint = "/nix/.rw-store"; image = "coord-store.img"; size = 8192; }
      { mountPoint = "/home";          image = "coord-home.img";  size = 2048; }
    ];
  };

  system.stateVersion = "25.11";
  documentation.enable = false;

  # Static IP on br-microvm (stable harness SSH target; .2 is outside the DHCP range).
  networking = { hostName = "fleet-coord"; useDHCP = false; };
  systemd.network = {
    enable = true;
    networks."10-vm-eth" = {
      matchConfig.MACAddress = "02:00:00:00:00:03";
      networkConfig = { Address = "10.88.0.2/24"; Gateway = "10.88.0.1"; DNS = "10.88.0.1"; };
      linkConfig.RequiredForOnline = "no";
    };
  };

  # nspawn-capable real kernel for the STORY-0021 fast tier (machinectl/systemd-nspawn).
  boot.enableContainers = true;

  # SSH in from agent-host (root holds the matching private key).
  services.openssh = {
    enable = true;
    settings = { PasswordAuthentication = false; PermitRootLogin = "prohibit-password"; };
  };
  users.users.root.openssh.authorizedKeys.keys = [
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEKAM+3oJVyiKZVdzi0ed5muG8SdMcDTHQm/LzCcundk root@agent-host"
  ];

  nix.settings = {
    experimental-features = [ "nix-command" "flakes" ];
    trusted-users = [ "root" ];
    extra-substituters = [ "https://cache.numtide.com" ];
    extra-trusted-public-keys = [ "niks3.numtide.com-1:DTx8wZduET09hRmMtKdQDxNNthLQETkc/yaX7M4qK0g=" ];
  };

  environment.systemPackages = with pkgs; [ git jq ];
}
