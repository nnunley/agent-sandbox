# guests/base.nix
{ config, pkgs, lib, ... }:

{
  imports = [ ./fleet-ready.nix ];

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

  # Networking via DHCP from the host's dnsmasq. Use systemd-networkd with an
  # explicit DHCP network matched by MAC — the `interfaces.eth0.useDHCP` shortcut
  # did not reliably bring up a DHCP client on the firecracker virtio interface
  # (guest never leased even with the tap bridged, 2026-06-18).
  networking = {
    hostName = "agent-vm";
    useDHCP = false;
  };
  systemd.network = {
    enable = true;
    networks."10-vm-eth" = {
      matchConfig.MACAddress = "02:00:00:00:00:01";
      networkConfig.DHCP = "ipv4";
      linkConfig.RequiredForOnline = "no";
    };
  };

  # Agent user
  users.users.agent = {
    isNormalUser = true;
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
