# Progress

**Phase:** ITER-0005b DONE (2026-06-22) — awaiting orchestrator audit (`auditing-progress`).
**Iterations:** 7/9 done (ITER-0000/0001/0002/0003/0004/0005/0005b). **Next eligible: ITER-0005c**
(FULL golden / provider routing / curated skills — gated only on the STORY-0025 benchmark, CLEARED;
runs on `agent-host`). ITER-0006 BLOCKED (Patrick sync); ITER-0007/0008 pending.

**Sentinel corpus:** all 7 cluster scenarios PASS (golden-launch, durable-vm, nspawn-fast, hardtier,
trust-boundary, microvm-boot, teardown); harness lib + golden-image + single-source structural tests
PASS; `go vet` clean; `go test -race ./...` green (incl. live e2e); JOURNEY-0001 green; zero
`TODO(ITER-0005b)` markers.

**ITER-0005b delivered (commits):**
- T1 fast-tier nspawn substrate + probes (48c7035); T2 NspawnRunner@TierFast (cf7282d);
  T4 FirecrackerRunner@TierHard + serve factory wiring + graft markers removed (7467bca);
  T5 golden.nix + fleet-golden image + golden-launch probe (f9e1f65);
  T6 trust-boundary probe + usable disposable-unit env / isolation correction (b1b22d5).
- Stories done: STORY-0007/0021/0022/0008/0024/0005 (EPIC-002 5/5; EPIC-001 +0005/0008).

**Measured (agent-host, 2026-06-22):** fast tier nspawn 64ms mean / 72ms p99 (gate ≤1000); hard tier
per-task Firecracker 737ms mean / 909ms p99 (gate ≤2500); durable-VM in-guest unit 16ms mean / 19ms p99;
teardown 111ms unit-kill (incus-free); golden copy launch ~2.9–3.3s CoW (no live build); trust boundary
guest kernel 6.12.78 ≠ host 6.8.0-106-generic.

**Substrate decision (evidence-backed):** two-tier — `nspawn --ephemeral` inside the durable Firecracker
coord VM for trusted lanes; per-task Firecracker for sensitive lanes. Grafted onto ITER-0005's
`BackendFactory` (Fast→NspawnRunner, Hard→FirecrackerRunner) with no daemon/interface change.

**Deferred (noted for audit):** STORY-0024 multi-domain provisioning/routing → ITER-0006+; FULL golden /
skills / provider routing (STORY-0075/0076/0077/0078) → ITER-0005c. ITER-0005's deferred microVM ACs
(STORY-0004 AC-3, STORY-0017 AC-3/4, STORY-0020 AC-2) substantively proven by this substrate harness.

**Last event:** 2026-06-22 — ITER-0005b complete; all artifacts synced; ready for `auditing-progress`.

**On resume:** "continue iterative development" → orchestrator runs `auditing-progress` (PAR, 3-tier) on
ITER-0005b, then picks ITER-0005c (next pending; ITER-0006 stays Patrick-blocked).
