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
- **Server invocation decision (refined during T0.1, empirically validated):** run the durable single-node
  server via **`temporal-cli` 1.5.1** → `temporal server start-dev --db-filename <dataDir>/temporal.db
  --ip 0.0.0.0 --headless` as a hand-rolled systemd unit (mirrors `fleet-worker/laneq-service.nix`).
  - **Why not the stock `services.temporal` module / `temporal-server start`:** that path requires explicit
    schema bootstrapping (`temporal-sql-tool setup-schema`) for a file-backed SQLite store; the upstream NixOS
    *test* dodges this with `mode=memory` (non-durable). `start-dev --db-filename` **auto-bootstraps the schema
    on first boot and reopens it on restart** — the documented persistent single-node path. This stays within
    the approved Task 0 mandate ("Nix package + systemd service + host-volume persistence").
- **Restart-survival empirically proven on the cluster (2026-06-24):**
  ```
  boot1: temporal server start-dev --db-filename /tmp/spike/temporal.db  → READY (30s, schema bootstrap)
         operator namespace create spike-ns → "successfully registered"; db grew to 573 KB
  kill boot1
  boot2: same --db-filename → READY in 2s; `namespace describe spike-ns` → STILL PRESENT (survived restart)
  ```
- **`dataDir` lives on an Incus host-mounted volume** `temporal-data` at `/srv/temporal` (mirror laneq-data at
  /srv/laneq) so the SQLite DB + deferred workflows/timers survive container/host restart, not just service restart.

### Logs
- journald (`journalctl -u temporal ...`). Filled in during T0.1.

## Deployment Steps (For Reproducibility) — DONE T0.1 (2026-06-24)
1. `incus storage volume create default temporal-data -t filesystem` → "created".
2. `incus config device add ndn-desktop:agent-host temporal-data disk pool=default source=temporal-data
   path=/srv/temporal` → mounted live (btrfs subvol), writable.
3. Add `../fleet-worker/temporal-service.nix` to `host/configuration.nix` imports (hand-rolled systemd unit
   running `temporal-cli` `server start-dev --db-filename /srv/temporal/temporal.db --ip 0.0.0.0 --port 7233
   --headless`).
4. `scripts/agent-host deploy` (LIVE switch, no container restart — does not disrupt laneq/micro-VMs/llm-proxy).
   Built closure: `/nix/store/x412x7r82mrzx6hhqzh1yaivwl2azlnf-nixos-system-agent-host-lxc-25.11...`; activation "ok".

## Readiness Check — DONE T0.2 (2026-06-24)
- `systemctl is-active temporal` → `active` (running).
- Boot-to-ready: `temporal operator cluster health --address 127.0.0.1:7233` → READY 22s after start (first-boot
  schema bootstrap), then ~2s on subsequent boots.
- Port: `ss -tlnp` → `LISTEN *:7233 users:(("temporal",pid=...))`.
- DB: `/srv/temporal/temporal.db` (573 KB) on the host volume + `temporal.db-journal`.

## Durability / Restart-Survival — DONE T0.2 service-level (2026-06-24); container-level e2e → SCENARIO-0001
- Service-restart proof on the DEPLOYED unit: `namespace create iter0007b-ns` → `systemctl restart temporal`
  → ready in 2s → `namespace describe iter0007b-ns` → **State: Registered (survived)**.
- The SQLite DB lives on the `temporal-data` host volume (btrfs subvol on the host disk), so it definitionally
  survives a container restart too. Full e2e SCENARIO-0001 (container restart + a deferred workflow firing
  post-restart) is proven in the evidence phase once the Go worker enqueues a real deferred workflow.

## Aging Proof (SCENARIO-0056 / STORY-0043 AC-2) — TODO(T0.2)
- **Decision: compressed real-wall-clock** (not fake-clock). Set a deadline a few seconds out, let a real
  Temporal timer fire on actual wall-clock, assert Q2→Q1 and that laneq.next reflects it. Genuine wall-clock
  aging, cluster-runnable in seconds. (ITER-0007's fake-clock CI proof is SCENARIO-0078, a different seam.)

## Security & TLS
- gRPC unsecured on the cluster trust boundary (same posture as laneq). TLS deferred.
