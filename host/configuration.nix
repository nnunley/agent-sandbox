# host/configuration.nix
{ config, pkgs, lib, ... }:

{
  imports = [
    ./networking.nix
    ../modules/llm-proxy.nix
    ../fleet-worker/laneq-service.nix
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

  # KSM (kernel same-page merging) can't be toggled from the unprivileged agent-host LXC
  # (/sys/kernel/mm/ksm/run is read-only → enable-ksm.service fails the activation). Disable
  # it declaratively so `switch-to-configuration` completes cleanly; KSM is a host-kernel
  # memory optimization, not required for correctness.
  hardware.ksm.enable = lib.mkForce false;

  # Test micro-VM using base guest image
  microvm.vms.test-vm = {
    config = {
      imports = [ ../guests/base.nix ];
    };
  };

  # Fleet worker micro-VM (hard isolation tier): non-root worker + SSH + nix cache.
  # Fully declarative — the guest is defined in-place and built/activated with the
  # host via nixos-rebuild (a guest change rebuilds the host closure; that's the
  # accepted cost of keeping the whole stack declarative).
  microvm.vms.worker-vm = {
    config = {
      imports = [ ../guests/worker-vm.nix ];
    };
  };

  # Durable COORDINATOR micro-VM (STORY-0007): stays up across tasks, warm /nix store,
  # nspawn-capable real kernel (hosts the STORY-0021 fast tier + STORY-0008 disposable
  # units). Reached at a static 10.88.0.2 over br-microvm. The coordinator daemon
  # (`dispatcher serve`) entrypoint exists; baking it in as a live service is gated on
  # Nix-packaging the binary + a real queue (ITER-0006).
  microvm.vms.fleet-coord = {
    config = {
      imports = [ ../guests/coordinator-vm.nix ];
    };
  };

  # NOTE: the fast-tier nspawn unit does NOT belong here. systemd-nspawn cannot
  # mount /proc inside the unprivileged agent-host LXC container (verified
  # 2026-06-18: "Failed to mount proc ... Operation not permitted", even with
  # security.nesting=true). The fast tier runs INSIDE the durable Firecracker
  # micro-VM (real kernel), per the design's nested topology — see
  # docs/plans/2026-06-18-fleet-orchestration-design.md.
  #
  # And making agent-host PRIVILEGED is not an escape hatch either (verified
  # 2026-06-21): security.privileged=true changes the container idmap, so the
  # SHARED nix-shared cache volume (idmap-shifted for the unprivileged container,
  # and shared with fleet-dogfood-base) refuses to mount and the container fails
  # to start ("Idmaps of container and storage volume nix-shared are not
  # identical"). So privileged-direct-nspawn-in-agent-host is off the table while
  # the binary cache is a shared unprivileged volume. The micro-VM guest path
  # (real kernel) sidesteps BOTH the proc-mount limit and the volume-idmap clash.
}
