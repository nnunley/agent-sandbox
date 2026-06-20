# Progress

**Phase:** ITER-0001 DONE (closed 2026-06-19) — coordination plane shipped (3/7 tasks fleet-dogfooded,
4/7 local TDD), PAR-reviewed (concurrency + coverage fixes), 69 tests green under -race, JOURNEY-0001
sentinel green. **Iterations:** 2/9 done (ITER-0000, ITER-0001); ITER-0002 next pending.
**ITER-0001 stories:** done — 0055/0056/0059/0063; partial (deferred ACs) — 0058(→0007/0004),
0061(→0007), 0027(→0008).

---
(historical, pre-ITER-0001)

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

**ITER-0001 implementation — IN PROGRESS (hybrid: dogfood isolated tasks, local workflow for coupled):**
Isolated tasks built BY THE FLEET via fleet-dogfood (TDD + hidden holdout oracle, reviewed + committed):
- ✅ T1 STORY-0056 AC-1/AC-4 — D6 decision log (decisionlog.go) — commit d4e313a
- ✅ T2 STORY-0027 AC-1/AC-2 — thread status (threadstatus.go) — commit 6ac3432
- ✅ T3 STORY-0059 AC-4 — durable Park queue rule (+ claim/lease/requeue already existed) — commit fe67309
Remaining (tightly coupled to daemon.go → local review workflow, NOT independent dogfood):
- ⏳ T4 STORY-0055 AC-1..6 + STORY-0058 AC-23 — escalation ladder (rung model, synchronous climb)
- ⏳ T5 STORY-0061 AC-1/2 — non-blocking human escalations lane; privileged rungs human-only
- ⏳ T6 STORY-0063 AC-28 — decision-log write on stop+reap
- ⏳ T7 — wire the full D4 RunOnce (compose T1–T6); keep JOURNEY-0001 green; evidence/corpus updates

**Test suite:** 60 passing (36 → +T1/T2/T3 tests), go vet clean. **Sentinel (JOURNEY-0001):** green.
**Methodology proven:** fleet builds the fleet — worker does TDD, independent holdout grades acceptance
on a clean checkout it never saw. 3/7 ITER-0001 tasks delivered this way.
**Artifact debt (non-blocking):** source-line citations for STORY-0055/0056/0059/0061 point at adjacent
spec sections (ACs match intent) — fix in a docs pass.

**Next:** build the coupled integration tasks T4–T7 via the local review workflow (shared daemon.go),
then post-iteration impacted+sentinel runs → wrap up → audit.
**Last event:** 2026-06-19 — ITER-0001 T1/T2/T3 dogfood-authored + holdout-graded + committed.
