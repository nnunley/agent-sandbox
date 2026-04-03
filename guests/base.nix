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

  # Networking via DHCP from host's dnsmasq
  networking = {
    hostName = "agent-vm";
    useDHCP = false;
    interfaces.eth0.useDHCP = true;
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
