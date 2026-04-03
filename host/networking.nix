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
