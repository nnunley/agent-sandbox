# Progress

**Phase:** ITER-0007b — **COMPLETE** (done:2026-06-24). Ready for `auditing-progress`, then ITER-0008.
**Iterations:** 11/12 done (ITER-0000..0007, 0007b). **Current: none active.** ITER-0008 pending (capstone:
Tier-2 coordinator, recursive delegation, operator TUI, Run object, budget, multi-consumer).

**Sentinel corpus:** `go vet ./...` clean; `go test -race ./...` **429 green** (387→429, +42 across C2–C5);
tree clean; zero `TODO(ITER-0007b)` markers (re-tagged → ITER-0008). Live tests env-gated (`TEMPORAL_LIVE=1`),
excluded from default CI.

**ITER-0007b delivered (all code tasks two-stage PAR; E1 orchestrator-executed & verified):**
- **C2** — PriorityWorkflow + sole-writer ReprojectActivity (Reprioritize+Defer) + lease-free `LaneqQueue.Defer`. `4bc32ae`.
- **C3** — rescore signal (human-unrestricted / agent-bounded) + `currentImportance` query handler. `958f6ca`.
- **C4** — RetryWorkflow (exp backoff re-push) + EscalationWorkflow (time-driven stale re-raise) + DeferWorkflow
  (hold-until-eligible), all via the sole-writer seam; `nextCheckInterval` helper; dead `ReprojectRequest` fields removed. `e58ea72`.
- **C5** — concurrent-read consistency, both guarded fields, `-race`. `401abc0`.
- **E1** — worker-driven live cluster harness (`temporal/temporal_live_test.go` + `run-temporal-live.sh`, gated).
  **SCENARIO-0094 LIVE-PROVEN** (human rescore flips laneq P1→P0, real worker executing). **SCENARIO-0001 LIVE-PROVEN**
  (DeferWorkflow survives a REAL Temporal restart PID 6976→7066; same runID Running→Completed; directive fired —
  STORY-0001 AC-2 durable timer, not laneq natural expiry). 0081/0093 live = honest (concurrent Peek / process-level
  sole-caller; value-consistency & sole-writer enforcement CI-PROVEN in C5/C2). Latest E1 commit `c55a35d`.

**ITER-0008 GATE: MET (no carries).** STORY-0041 AC-1/AC-2 (live sole writer) + STORY-0044 AC-3 (live sole caller).

**ITER-0008 design notes (recorded):** PriorityWorkflow early-exit-at-Q1 → post-Q1 operator pause/block/resume
needs a separate control plane; live wall-clock Q2→Q1 not compressible to seconds (urgency calibrated in days —
needs a knob or multi-day runner; logic is CI-PROVEN); rejected agent rescores → operator approval queue;
laneq `Stats()/Len()` observability deferred.

**Last event:** 2026-06-24 — ITER-0007b complete; artifacts updated (roadmap done, iteration-log appended, TODOs
re-tagged, .bak removed). **On resume:** orchestrator runs `auditing-progress` (three-tier), then ITER-0008.
