# guests/worker-vm.nix
#
# Fleet WORKER as a Firecracker micro-VM (hard isolation tier). Same worker NixOS
# config as the container (non-root worker + nix cache + agent tools via
# `nix develop`), delivered as a VM. Differences from the container backend:
#   - own kernel ⇒ the nix build sandbox WORKS (no --no-sandbox needed)
#   - not incus-exec-able ⇒ control via SSH over br-microvm
#   - host /nix/store is shared read-only ⇒ need a writableStoreOverlay so the
#     worker can substitute claude-code/lean-ctx into the guest
{ pkgs, ... }:
{
  imports = [ ./fleet-ready.nix ];

  microvm = {
    hypervisor = "firecracker";
    mem = 6144;
    vcpu = 4;

    # Unique tap + MAC (must not collide with test-vm's vm-test / ...01).
    interfaces = [{ type = "tap"; id = "vm-worker"; mac = "02:00:00:00:00:02"; }];

    # Writable overlay over the read-only shared host store so `nix develop` can
    # realize new paths (claude-code, lean-ctx) inside the guest.
    writableStoreOverlay = "/nix/.rw-store";
    volumes = [
      { mountPoint = "/nix/.rw-store"; image = "worker-store.img"; size = 8192; }
      { mountPoint = "/home";          image = "worker-home.img";  size = 4096; }
    ];
  };

  system.stateVersion = "25.11";
  documentation.enable = false;

  networking = { hostName = "fleet-worker-vm"; useDHCP = false; };
  systemd.network = {
    enable = true;
    networks."10-vm-eth" = {
      matchConfig.MACAddress = "02:00:00:00:00:02";
      networkConfig.DHCP = "ipv4";
      linkConfig.RequiredForOnline = "no";
    };
  };

  # Non-root worker (claude refuses --dangerously-skip-permissions as root) + SSH key.
  users.users.worker = {
    isNormalUser = true;
    home = "/home/worker";
    extraGroups = [ "wheel" ];
    openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4cDacSoTxUpn9/1q0t179zX1FelIfHU2ZzEkfRqCJI root@agent-host"
    ];
  };
  security.sudo.wheelNeedsPassword = false;
  services.openssh = { enable = true; settings.PasswordAuthentication = false; };

  # The /home volume mounts empty, so create the worker's home (else ssh warns
  # "Could not chdir to home directory" and nix/go have nowhere to write caches).
  systemd.tmpfiles.rules = [ "d /home/worker 0700 worker users - -" ];

  nix.settings = {
    experimental-features = [ "nix-command" "flakes" ];
    trusted-users = [ "root" "worker" ];
    extra-substituters = [ "https://cache.numtide.com" ];
    extra-trusted-public-keys = [ "niks3.numtide.com-1:DTx8wZduET09hRmMtKdQDxNNthLQETkc/yaX7M4qK0g=" ];
  };
  # NOTE: no `nixpkgs.config.allowUnfree` here — a micro-VM guest's nixpkgs is an
  # externally-created instance (from the host), so setting config there is a hard
  # eval error. The guest system installs only free pkgs; claude-code (unfree) is
  # realized at runtime by the worker's `nix develop`, where the fleet-worker flake
  # sets allowUnfree itself.

  environment.systemPackages = with pkgs; [ git ];
}
