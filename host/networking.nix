# host/networking.nix
{ config, lib, ... }:

let
  # Micro-VM bridge network configuration — change these to avoid subnet collisions
  bridge = {
    name = "br-microvm";
    gateway = "10.88.0.1";
    prefix = 24;
    dhcpStart = "10.88.0.10";
    dhcpEnd = "10.88.0.254";
  };
in {
  # Enable IP forwarding for NAT
  boot.kernel.sysctl = {
    "net.ipv4.ip_forward" = 1;
  };

  networking = {
    hostName = "agent-host";

    # Use systemd-networkd (matches stock Incus NixOS image)
    dhcpcd.enable = false;
    useDHCP = false;
    useHostResolvConf = false;

    # NAT from micro-VM bridge to container's outbound interface
    nat = {
      enable = true;
      internalInterfaces = [ bridge.name ];
      externalInterface = "eth0";
    };

    # Firewall: allow DHCP and DNS from micro-VMs
    firewall = {
      enable = true;
      trustedInterfaces = [ bridge.name ];
    };
  };

  # systemd-networkd for eth0 (matches stock Incus config)
  systemd.network = {
    enable = true;
    networks."50-eth0" = {
      matchConfig.Name = "eth0";
      networkConfig = {
        DHCP = "ipv4";
        IPv6AcceptRA = true;
      };
      linkConfig.RequiredForOnline = "routable";
    };

    # Bridge for micro-VMs
    netdevs."10-br-microvm" = {
      netdevConfig = {
        Name = bridge.name;
        Kind = "bridge";
      };
    };
    networks."10-br-microvm" = {
      matchConfig.Name = bridge.name;
      addresses = [{ Address = "${bridge.gateway}/${toString bridge.prefix}"; }];
      networkConfig.ConfigureWithoutCarrier = true;
      linkConfig.RequiredForOnline = "no";
    };
  };

  # DHCP + DNS for micro-VMs
  services.dnsmasq = {
    enable = true;
    settings = {
      interface = bridge.name;
      bind-dynamic = true;  # retry binding if interface isn't ready yet
      dhcp-range = "${bridge.dhcpStart},${bridge.dhcpEnd},24h";
      dhcp-option = [
        "3,${bridge.gateway}"   # gateway
        "6,${bridge.gateway}"   # DNS
      ];
    };
  };
}
