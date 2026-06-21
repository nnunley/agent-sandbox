# Progress

**Phase:** ITER-0004 DONE + AUDIT CLEAN; **STORY-0025 benchmark spike DONE → ITER-0005 GATE CLEARED** (2026-06-21).
**Iterations:** 5/9 done (ITER-0000/0001/0002/0003/0004). **Next eligible: ITER-0005** (micro-VM backend / NixOS golden / isolation tiers — gate cleared). ITER-0006 BLOCKED (Patrick sync).
**Sentinel corpus:** JOURNEY-0001 green; incus-dispatcher + llm-proxy 166 tests green under -race; vet clean; zero TODO(ITER-0004).
**STORY-0025 spike result:** nspawn Fast tier **76 ms** mean/97 ms p99 (N=100, nesting-enabled Incus NixOS container, warm /nix) vs Firecracker Hard tier **1861 ms** mean/2134 ms p99 (N=20). nspawn 24.5× faster = substrate-selection signal; microVM boot amortizes to <0.7% of a 5–10 min task. Decision: **two-tier** — nspawn (`security.nesting=true`) for trusted lanes, Firecracker for untrusted. Nesting research: Incus PR #2624 (in host's 6.23) drops sys/proc AppArmor protections when `security.nesting=true` → no privileged needed; default-off nesting is why agent-host failed. Artifacts: `fleet-worker/spikes/` (bench-spinup.sh + results + STORY-0025-benchmark-results.md).
**Open follow-up (optional):** set `security.nesting=true` on `agent-host` permanently (needs container restart, briefly bounces microVMs) — currently a noted follow-up, not done.
**Last event:** 2026-06-21 — STORY-0025 benchmark spike complete (nspawn measured); ITER-0005 gate cleared.

**ITER-0004 delivered (commits):**
- T0 handoff-bundle schema doc (d67823a); T1+T2 Thread/Run/StumbleSignal data model (04b8687, 8663fe4);
  T3 workspace-lease registry (4e2b2e7); T4+T5 ReconstructResumeAudit + SCENARIO-0015 (59b6a3c);
  T7 ContextProvider interface + NoopProvider + AC-4/AC-5 (e8c2ca2); **T6 LeanCtxProvider default adapter
  (2a1e447)**; **T8 fresh handoff bundle on autonomous requeue / STORY-0058 AC-25 (467a93e)**.
- Stories done: STORY-0029/0030/0033/0018; partial: STORY-0031 (AC-1/2; AC-3/4 → ITER-0008), STORY-0058
  (AC-25; AC-24 → ITER-0007).
- Scenarios: SCENARIO-0015/0030/0031/0054 automated; SCENARIO-0077 (spike, prior). Corpus commands wired.

**Decision (2026-06-21):** T6/T8 built local-TDD against the real lean-ctx binary + the fake-backend daemon
seam, NOT a fresh cluster dogfood — the cross-one-shot session round-trip was already cluster-proven by the
STORY-0034 spike (SCENARIO-0077). lean-ctx knowledge is project-scoped by CWD, so the integration test runs in
an isolated temp project (skips if lean-ctx absent) and never touches the repo's own store.

**Environment note (2026-06-21):** local disk is ~full (≈877Mi free of 460Gi) — /tmp writes hit ENOSPC mid-run.
Git commits unaffected. Cluster dogfood would need disk headroom on the worker host too.

**On resume:** "continue iterative development" → orchestrator runs `auditing-progress` (PAR, 3-tier) on
ITER-0004, then picks the next pending iteration (ITER-0005, gated on the STORY-0025 disposable-unit benchmark
spike; ITER-0006 stays blocked on the Patrick sync). Optional cluster-evidence pass: STORY-0068 AC-2 (let-go
13→0) + a live SCENARIO-0030 diary round-trip on a real fleet worker.
