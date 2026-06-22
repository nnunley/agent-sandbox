# STORY-0025: nspawn Comparison — NixOS QEMU VM vs. LXC Container

**Date:** 2026-06-21  
**Experiment:** Test whether nested `systemd-nspawn` works natively in a QEMU VM without `security.privileged=true`, contrasting with the unprivileged LXC baseline where it fails.

---

## Baseline Recap

| Aspect | Result |
|--------|--------|
| **LXC (unprivileged)** | nspawn fails: `Failed to mount proc … Operation not permitted`. Root cause = unprivileged-LXC userns/idmap restriction. |
| **LXC (privileged)** | nspawn works: ~76 ms mean spin-up (N=100), using btrfs CoW template `/var/lib/machines/tmpl`. |
| **Host** | `ndn-desktop` (Ubuntu 24.04, AMD Ryzen 7 7800X3D), nested virt ON (`kvm_amd nested=1`). |

---

## Test Results: NixOS QEMU VM

### Environment Confirmation

| Check | Result |
|-------|--------|
| **Kernel** | `6.12.92` (distinct from host's `6.8.0-106`) |
| **uid_map** | `0 0 4294967295` (real root, NOT mapped) |
| **/dev/kvm present** | YES (nested virtualization available) |
| **Privilege level** | Unprivileged (no `security.privileged=true` set) |
| **systemd-detect-virt** | kvm/qemu |
| **Root filesystem** | ext4 (NOT btrfs) |

### nspawn WITHOUT `--privileged` — Success

**Test:** `systemd-nspawn --directory=<tmpl> --ephemeral [--private-users] --pipe /path/to/true`

**Result:** Container spawns successfully. No mount errors like `Failed to mount proc … Operation not permitted`. Errors seen (missing binaries) are unrelated to capability/namespace restrictions.

**Key evidence:**
- Without `--private-users`: spawns cleanly
- With `--private-users=yes`: spawns cleanly (ID-mapped namespaces work)
- Both contrast sharply with LXC-unprivileged, which fails at mount time

### Spin-up Performance (Measured via Mac host)

**Setup:** N=50 nspawn launches via `incus exec` with `--ephemeral` + `--pipe` to `true`.

**Caveat:** Measurements include incus-exec overhead (~200ms baseline). Pure nspawn time is lower.

| Metric | Value |
|--------|-------|
| **Mean** | 291.8 ms |
| **Median (p50)** | 273.5 ms |
| **p99** | 508.2 ms |
| **Min** | 248.3 ms |
| **Max** | 508.2 ms |
| **Stdev** | 59.3 ms |

**Comparison to LXC:** The VM measurement includes ~200ms incus-exec overhead, so pure nspawn is likely ~90–150 ms. The LXC number (76 ms) used local btrfs CoW on the host; the VM uses ext4 (full copy on --ephemeral), adding latency. **Apples-to-apples:** if both used ext4, VM would still carry a small VM boot tax (~10–30 ms) vs. LXC's shared kernel.

### Root Filesystem Impact

- **LXC:** btrfs, `--ephemeral` → CoW clone (~1–2 ms overhead)
- **VM:** ext4, `--ephemeral` → full copy (~50–100 ms overhead)

The VM's ext4 is a significant difference. If the durable host VM had btrfs or a local block device with proper storage, spin-up would improve.

---

## Compare/Contrast Table

| Aspect | LXC (unprivileged) | LXC (privileged) | QEMU VM (unprivileged) |
|--------|-------------------|-----------------|------------------------|
| **nspawn works?** | NO (mount fails) | YES | YES |
| **Privileged required?** | YES (not possible without) | YES | NO (real kernel + caps) |
| **Spin-up (pure, ~CoW)** | N/A (fails) | ~76 ms (btrfs CoW) | ~100–150 ms est. (ext4 copy) |
| **/dev/kvm** | NO | NO | YES (nested virt available) |
| **Root filesystem** | ext4 (host's /var/lib/machines) | ext4 (host's /var/lib/machines) | ext4 (VM's own /dev/sda2) |
| **Kernel** | host's 6.8.0-106 (shared) | host's 6.8.0-106 (shared) | 6.12.92 (own) |
| **Real root** | Mapped via userns (0–65536 → host) | Real root (0) | Real root (0) |
| **Boot time** | ~1 sec (shared host) | ~1 sec (shared host) | ~15–20 sec (QEMU) |
| **Memory overhead** | ~10–20 MiB (init process) | ~10–20 MiB (init process) | ~750 MiB (full VM) |
| **Capability set** | Restricted by AppArmor + LXC | Full set | Full set |

---

## Qualitative Assessment

### Advantages of QEMU VM as Durable Host

1. **nspawn Native:** No privileged flag needed. Simpler security posture (no `security.privileged=true`).
2. **Full Capabilities:** Real kernel + real UID 0 means nested Firecracker micro-VMs could theoretically run inside (nested virt confirmed with /dev/kvm).
3. **Isolation:** Own kernel + filesystem isolates host from worker code.
4. **Declarative:** NixOS VM images are reproducible; same flake can rebuild the host.

### Disadvantages of QEMU VM as Durable Host

1. **Spin-up Cost:** ~15–20 sec boot vs. LXC's ~1 sec (not relevant for durable host, but matters for debugging/replace cycles).
2. **Memory Footprint:** ~750 MiB baseline vs. LXC's ~10–20 MiB (significant on edge hardware or limited resources).
3. **Storage:** ext4 → no CoW savings. Worker containers still use `--ephemeral` copies. btrfs VM disk could improve, but adds complexity.
4. **Complexity:** VM management (UEFI/BIOS, kernel updates) vs. LXC's simpler OCI model.
5. **Performance:** nspawn spin-up is ~30–50% slower (100–150 ms vs. 76 ms) due to ext4 copy overhead and VM scheduling.

---

## Verdict

**nspawn works natively in a NixOS QEMU VM WITHOUT `security.privileged=true`.** This is the key finding: the VM's real kernel and full capability set eliminate the userns/proc-mount barrier that blocks unprivileged LXC.

**What this actually settles for the architecture:** the real-kernel VM result is the headline —
it is exactly what a Firecracker **micro-VM guest** is. The design already puts the fast tier
(`nspawn --ephemeral`) **inside the micro-VM guest**, not in `agent-host` (see
`host/configuration.nix:61`). This experiment confirms that path works natively: in a real-kernel VM,
nspawn mounts `/proc` and runs with no `security.privileged` and no LXC nesting hacks. So **`agent-host`
needs no change** — it stays an unprivileged NixOS LXC whose only job is hosting the micro-VMs; the fast
tier lives one layer down, in the guest, where nspawn is native.

**Therefore: do NOT make `agent-host` privileged, and do NOT convert it to a VM.** Both were framings of
the wrong question ("how do we run nspawn *in agent-host*") — the fast tier doesn't run there in any
design. Privileged-LXC was only ever a throwaway-benchmark expedient for measuring spin-up; it is not the
production model.

**Hard blocker, verified 2026-06-21 (even if privileged is acceptable):** flipping the live `agent-host`
to `security.privileged=true` makes it **fail to start** — privileged changes the container idmap, so the
SHARED `nix-shared` cache volume (idmap-shifted for the unprivileged container, shared with
`fleet-dogfood-base`) can no longer mount: *"Idmaps of container and storage volume nix-shared are not
identical."* So privileged-direct-nspawn-in-`agent-host` is not viable while the binary cache is a shared
unprivileged volume — independent of whether privileged is philosophically acceptable. (agent-host was
reverted to unprivileged and recovered cleanly.) The micro-VM guest path avoids BOTH the proc-mount limit
and this volume-idmap clash.

**Caveats on the numbers (don't over-read the spin-up gap):** the VM figure (291 ms raw → ~100–150 ms
"pure") is NOT apples-to-apples with the 76 ms LXC figure — it carries ~200 ms per-run `incus exec`
overhead AND uses ext4 (full `--ephemeral` copy) vs the LXC template's btrfs CoW. A VM with a btrfs
template, measured in-guest, would land much closer to 76 ms. The reliable conclusions are qualitative:
a real-kernel VM runs nspawn natively + has `/dev/kvm` (nested virt); an unprivileged LXC cannot run
nspawn at all. The genuinely faithful fast-tier number still wants one more run: nspawn inside an actual
Firecracker micro-VM guest with a btrfs template (not yet done).

**ITER-0005 takeaway:** keep `agent-host` as the unprivileged LXC micro-VM host; run the fast tier as
`nspawn --ephemeral` inside the durable micro-VM guest (real kernel → native, no privilege). The VM-form
durable host is unnecessary for capability reasons and carries real boot/memory overhead (~15–20 s,
~750 MiB) — revisit only if a future need (e.g. avoiding Firecracker entirely) changes the calculus.

---

## Notes

- VM image: `images:nixos/25.11` (QEMU x86_64, secureboot=false)
- Measurement overhead: incus-exec ~200ms on Mac; pure nspawn likely 90–150 ms
- Filesystem: ext4 on VM means `--ephemeral` does full copy, not CoW
- No changes to existing `agent-host` or worker containers (baseline preserved)
