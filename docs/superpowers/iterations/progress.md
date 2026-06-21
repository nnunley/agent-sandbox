# Progress

**Phase:** ITER-0004 DONE (2026-06-21) — State passthrough & continuity. All tasks T0–T8 delivered.
**Iterations:** 5/9 done (ITER-0000/0001/0002/0003/0004). ITER-0006 BLOCKED (Patrick sync). Next pending: ITER-0005 (gated on STORY-0025 benchmark spike).
**Sentinel corpus:** JOURNEY-0001 green; incus-dispatcher + llm-proxy 165 tests green under -race; vet clean; zero TODO(ITER-0004).
**Last event:** 2026-06-21 — T6 (LeanCtxProvider) + T8 (fresh-handoff-on-requeue) committed; iteration wrapped, artifacts synced.

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
