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
}
