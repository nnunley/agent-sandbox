# Cluster verification harness (ITER-0005b Task 0)

The **gate** for ITER-0005b (Firecracker micro-VM substrate & isolation tiers). ITER-0005b is
cluster-only — it runs on `agent-host` (`ndn-desktop`, Incus) and has **no Mac CI seam** — so each
story's acceptance is proven by a scenario meeting its gate here, not by a unit test. The PAR scope
review (2026-06-21, both reviewers) made this harness the BLOCKING first deliverable: a cluster-only
iteration with no defined verification mechanism is a quality risk.

## Layout
- `lib.sh` — pure measurement (`compute_stats`, `stat_field`) + acceptance gates (`assert_le`,
  `assert_true`) + cluster reachability (`cluster_reachable`) + readiness pollers (`wait_microvm_ready`).
  The pure logic is TDD-tested on the Mac in `tests/lib.test.sh`.
- `gates.env` — declarative acceptance gates + sample sizes. **Tune here, not in `run.sh`.**
- `run.sh <scenario>` — runs one scenario's readiness probe + measurement + gate assertion.
- `tests/lib.test.sh` — Mac-runnable test of the measurement/gate math (`bash tests/lib.test.sh`).

## Exit codes (and why)
| Code | Meaning | When |
|---|---|---|
| 0 PASS | gate met | substrate present, probe ran, gate satisfied |
| 0 SKIP | cluster unreachable | off-cluster (no Mac Nix) — corpus commands stay CI-safe |
| 2 PENDING | substrate not provisioned | the owning story hasn't landed — NOT a false PASS |
| 1 FAIL | gate not met | real regression / unmet AC |

PENDING (2) vs PASS (0) is the key discipline: a scenario only PASSes once its substrate exists AND
its gate is met. Until then it names the owning story, so the harness can't silently claim coverage.

## Readiness sentinels & gates (the substance the scope review required)

| Scenario | Story / AC | Readiness sentinel (what "ready" means) | Gate (`gates.env`) |
|---|---|---|---|
| `microvm-boot` (0029) | STORY-0017 AC-3 | microVM unit `is-active` **and** MainPID present, + network settle | boot-to-ready mean ≤ `GATE_MICROVM_BOOT_MS` (5000) |
| `durable-vm` (0004) | STORY-0007/0008 | VM stays `active` across K task cycles (0 restarts); in-guest unit spins up | `GATE_DURABLE_RESTARTS` (0); unit spin-up p99 ≤ `GATE_UNIT_SPINUP_P99_MS` (1000) |
| `nspawn-fast` (0005) | STORY-0021 | in-guest `systemd-nspawn --ephemeral … echo READY` completes; PID/mnt/net ns differ from guest | spin-up mean ≤ `GATE_NSPAWN_SPINUP_MS` (1000) + namespaces isolated |
| `hardtier` (0006) | STORY-0022 AC-2 | per-task Firecracker microVM boot-to-ready | spin-up p99 ≤ `GATE_HARDTIER_SPINUP_P99_MS` (2500) |
| `trust-boundary` (0007) | STORY-0024 | guest `uname -r` ≠ host (own kernel = hardware boundary); disposable unit runs inside | own-kernel true (single-domain v1) |
| `golden-launch` (0003) | STORY-0005 | `incus copy` golden→fresh boots ready with **no live nix build** in the launch path | ready + zero live builds |
| `teardown` (0008ac2) | STORY-0008 AC-2 | teardown via unit-kill (machinectl/systemctl), **no `incus delete`** in the hot path | bounded ≤ `GATE_TEARDOWN_MS` (5000) |

Gates derive from the STORY-0025 benchmark (nspawn 76 ms / Firecracker 1861 ms mean, 2134 ms p99) and
the story ACs (STORY-0017 AC-3 ≤5s; STORY-0008 AC-3 sub-second unit spin-up).

## Corpus wiring
`run.sh <scenario>` is the corpus command for SCENARIO-0003/0004/0005/0006/0007/0029 (and the
STORY-0008 AC-2 teardown check). As each substrate story lands, its scenario flips PENDING→PASS.

## Status (2026-06-21)
- `tests/lib.test.sh` green on the Mac.
- `microvm-boot` (SCENARIO-0029 / STORY-0017 AC-3) **PASS on agent-host**: boot-to-ready
  **mean 826 ms / p99 1840 ms / min 634 ms (N=20)** — well under the 5 s gate. (A durable
  `microvm@test-vm` already exists, partial STORY-0007 substrate.)
- All other scenarios report **PENDING** with their owning story — they flip to PASS as
  STORY-0007/0008/0021/0022/0024/0005 land.
