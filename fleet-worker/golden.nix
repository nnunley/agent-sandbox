# fleet-worker/golden.nix
#
# STORY-0005 — the IMMUTABLE GOLDEN worker image definition (AC-1). Extends the declarative
# worker-container.nix with the golden substrate contract:
#
#   - IMMUTABLE ROOT: the published `fleet-golden` image IS the immutable source of truth.
#     Per-task workers are disposable copy-on-write clones (`incus copy`, btrfs CoW) — never
#     built live (AC-2). The Nix store is read-only by construction; the system closure is
#     fixed at golden-build time, so a copy is byte-for-byte the golden until it writes scratch.
#
#   - WRITABLE SCRATCH: only /workspace (the task working tree) and /tmp are writable, as
#     tmpfs so each disposable copy starts clean and leaves nothing behind. Everything else a
#     task touches lives under these or is rejected by the read-only root intent. This is the
#     STORY-0049 AC-5 split-in (immutable root + writable /workspace,/tmp scratch).
#
# The FULL golden (agent skills + provider routing baked in, clean-room byte-identical
# regeneration) is STORY-0075 / ITER-0005c; this file is the substrate those compose onto.
{ lib, ... }:

{
  imports = [ ./worker-container.nix ];

  # Writable scratch — the ONLY writable surfaces in a disposable copy. tmpfs so a fresh
  # copy starts empty and teardown leaves no residue (disposable-unit discipline).
  fileSystems."/workspace" = {
    device = "tmpfs";
    fsType = "tmpfs";
    options = [ "rw" "nosuid" "nodev" "mode=1777" "size=4G" ];
  };
  fileSystems."/tmp" = lib.mkForce {
    device = "tmpfs";
    fsType = "tmpfs";
    options = [ "rw" "nosuid" "nodev" "mode=1777" "size=2G" ];
  };

  # Immutable root intent: the Nix store is read-only (golden is the source); a disposable
  # copy must not mutate the system profile. Writes belong in /workspace or /tmp.
  boot.readOnlyNixStore = true;

  # Marker so a launched copy can be proven to be a fleet-golden clone (not a freshly built
  # stock image) without a live build — read by the golden-launch cluster probe (AC-2).
  environment.etc."fleet-golden-version".text = "fleet-golden v1 (STORY-0005, ITER-0005b)\n";
}
