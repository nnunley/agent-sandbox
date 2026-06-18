# guests/fleet-ready.nix
#
# Boot-readiness sentinel. The instant userspace reaches multi-user.target, emit
# a fixed marker to the console. A host-side timer watching the managing unit's
# journal (microvm@<name> for a micro-VM, machine journal for an nspawn unit)
# measures launch -> ready WITHOUT guest/host clock sync — it just waits for the
# marker line to appear and stamps the time host-side.
{ pkgs, ... }:
{
  systemd.services.fleet-ready = {
    description = "Emit fleet boot-readiness sentinel to the console";
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      StandardOutput = "journal+console";
      ExecStart = "${pkgs.coreutils}/bin/echo FLEET-READY-SENTINEL";
    };
  };
}
