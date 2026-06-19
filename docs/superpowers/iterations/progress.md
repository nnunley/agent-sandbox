# Progress

**Phase:** ITER-0001 scope CONVERGED (PAR REVISE applied) → ready to decompose + implement
**Iterations:** 1/9 done (ITER-0000), ITER-0001 in prep, 7 pending (ITER-0006 blocked post-Patrick)
**Backlog reconciled:** ITER-0000's 7 fully-delivered stories marked done:ITER-0000; 4 splits marked
partial; epic counters corrected; STORY-0060 mis-citation fixed (it's the teardown story, not the harness)

**ITER-0001 converged scope (substrate/Temporal-independent):**
- STORY-0055 AC-1..6 (D4 loop + ladder rules), STORY-0058 AC-23 (synchronous climb),
  STORY-0059 AC-1..4 (claim/lease/requeue/park), STORY-0061 AC-1/2 (autonomous rungs + human lane),
  STORY-0027 AC-1/2 (thread status field + transitions), STORY-0056 AC-1..4 (D6 decision log),
  STORY-0063 AC-28 (decision-log write on reap)
- **Deferred by PAR:** Temporal ACs (0055 AC-7, 0058 AC-24, 0061 AC-3) → ITER-0007;
  0058 AC-25 → ITER-0004; 0027 AC-3 (TUI) → ITER-0008; STORY-0054 (agent/delegation audit) → ITER-0008

**Test suite:** 36 passing, go vet clean. **Sentinel baseline (JOURNEY-0001):** green.
**Artifact debt (non-blocking):** source-line citations for STORY-0055/0056/0059/0061 point at adjacent
spec sections (ACs match intent) — fix in a docs pass.

**Next:** decompose ITER-0001 into TDD code + evidence tasks → dispatch implementing-tasks →
post-iteration impacted+sentinel runs → wrap up → audit.
**Last event:** 2026-06-19 — ITER-0001 scope PAR (2 reviewers, both REVISE) applied; roadmap re-scoped.
