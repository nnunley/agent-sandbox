# host/configuration.nix
{ config, pkgs, lib, ... }:

{
  imports = [
    ./networking.nix
    ../modules/llm-proxy.nix
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

  # LLM proxy on the bridge gateway
  services.llm-proxy.enable = true;

  # microvm.nix host defaults
  microvm.host.enable = true;

  # Test micro-VM using base guest image
  microvm.vms.test-vm = {
    config = {
      imports = [ ../guests/base.nix ];
    };
  };

  # Fleet worker micro-VM (hard isolation tier): non-root worker + SSH + nix cache.
  microvm.vms.worker-vm = {
    config = {
      imports = [ ../guests/worker-vm.nix ];
    };
  };

  # NOTE: the fast-tier nspawn unit does NOT belong here. systemd-nspawn cannot
  # mount /proc inside the unprivileged agent-host LXC container (verified
  # 2026-06-18: "Failed to mount proc ... Operation not permitted", even with
  # security.nesting=true). The fast tier runs INSIDE the durable Firecracker
  # micro-VM (real kernel), per the design's nested topology — see
  # docs/plans/2026-06-18-fleet-orchestration-design.md.
}
