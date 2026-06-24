# ITER-0007b T0.3: Temporal Time-Plane Deployment (cluster)

**Status:** STUB — committed at scope-approval time (PAR 2026-06-23). Deployment steps + readiness
output get filled in during ITER-0007b Task 0 (T0.1/T0.2). This stub exists NOW so the sole-writer
seam contract and the non-exclusive-lease assumption are recorded before any code is written.

---

## Sole-Writer Seam Contract (READ FIRST — gates ITER-0008)

Temporal is the **single writer** of the laneq scheduling fields (`effective_priority`, `not_before`).

- **How it writes:** ONLY via the laneq gRPC seam — `Defer(id, not_before)` and `Reprioritize(id, priority)`
  (`modules/incus-dispatcher/queue/laneq.go:278-287`). Temporal NEVER touches the laneq SQLite DB directly.
- **How "single writer" is enforced:** **process-level discipline.** Exactly one Temporal worker role is
  configured with the Defer/Reprioritize client capability. It is **NOT** enforced by lease ownership or RBAC
  on laneq.
- **Non-exclusive-lease assumption (boxing-in note for ITER-0008):** real laneq leases are **NOT
  consumer-exclusive** — the server keys leases by directive id and does not enforce per-consumer token
  ownership on Touch/Done (verified in SCENARIO-0092; the in-process fake is stricter). Therefore:
  - The sole-writer guarantee is orthogonal to laneq lease exclusivity.
  - ITER-0008 recursive delegation / multi-consumer / work-stealing **MUST NOT** assume lease exclusivity.
    If exclusivity is ever required, add an opaque per-claim token to laneq upstream (`nnunley/laneq`) first.

---

## Overview

Stands up Temporal on `ndn-desktop`/`agent-host` as the durable time plane that owns Schedules, durable
timers, and retry backoff, then re-projects effective priority / not-before onto the deployed laneq over
its gRPC seam. Grafts the deployed Temporal onto ITER-0007's proven pure-Go projection logic
(`modules/incus-dispatcher/temporal/`) and ITER-0006's deployed laneq.

## Deployment Contract (target — verify during T0.1)

### Package & Service
- **Package:** upstream `temporal` **1.29.4** (pinned `nixos-25.11`; the flake pins
  `github:NixOS/nixpkgs/nixos-25.11`). Full `temporal-server`, gRPC frontend on **:7233**.
- **Service:** upstream `services.temporal` NixOS module (`enable`/`package`/`settings`/`dataDir`), wired
  into the agent-host config (`host/configuration.nix` imports), shipped via `scripts/deploy.sh`.

> **Availability verification (authoritative, on the cluster against the pinned channel, 2026-06-23):**
> ```
> $ nix eval --raw github:NixOS/nixpkgs/nixos-25.11#temporal.version
> 1.29.4
> # package: pkgs/by-name/te/temporal/package.nix
> # module:  nixos/modules/services/cluster/temporal/default.nix  (in nixos/modules/module-list.nix:494)
> ```
> Confirms both the `temporal` 1.29.4 package and the `services.temporal` module exist in nixos-25.11
> (the flake-pinned channel). No flake.lock in repo; the channel is pinned by branch in flake.nix:5.

### Storage (durability — STORY-0001 AC-2 / STORY-0002 AC-2)
- `services.temporal.settings.persistence` configured with **file-backed SQLite** (`pluginName=sqlite`,
  db file under `dataDir`, `mode` ≠ `memory`). The upstream NixOS *test* uses `mode=memory` (in-memory,
  NOT durable) — do NOT copy that.
- `dataDir` lives on an **Incus host-mounted volume** (mirror the laneq-data volume at /srv/laneq) so
  deferred workflows/timers survive container/host restart.

### Logs
- journald (`journalctl -u temporal ...`). Filled in during T0.1.

## Deployment Steps (For Reproducibility) — TODO(T0.1)
1. Create host volume for Temporal dataDir.
2. Wire `services.temporal` module (file-backed SQLite, port 7233) into agent-host config.
3. Build closure on the cluster (no nix on macOS); `scripts/deploy.sh` (profile switch + container restart).
4. Record store paths + activation output here.

## Readiness Check — TODO(T0.2)
- Boot-to-ready sentinel: server answers on :7233 (e.g. `temporal operator cluster health` / a gRPC dial).
- Record unit status + port-listen output here.

## Durability / Restart-Survival (MUST-PASS — SCENARIO-0001) — TODO(T0.2)
- Enqueue a deferred workflow/timer (future not-before) → restart the temporal service AND the container →
  assert the deferred workflow still exists and fires when eligible (state reloaded from host-volume DB).

## Aging Proof (SCENARIO-0056 / STORY-0043 AC-2) — TODO(T0.2)
- **Decision: compressed real-wall-clock** (not fake-clock). Set a deadline a few seconds out, let a real
  Temporal timer fire on actual wall-clock, assert Q2→Q1 and that laneq.next reflects it. Genuine wall-clock
  aging, cluster-runnable in seconds. (ITER-0007's fake-clock CI proof is SCENARIO-0078, a different seam.)

## Security & TLS
- gRPC unsecured on the cluster trust boundary (same posture as laneq). TLS deferred.
