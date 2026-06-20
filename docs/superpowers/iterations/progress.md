# Progress

**Phase:** ITER-0003 SCOPE LOCKED (2026-06-20) — Worker reliability & robust result contract.
Scope reviewed (PAR REVISE→revised→approved). **Implementation NOT yet started — checkpointed for a
fresh lean session** (ITER-0003 is cluster-heavy + 0069-spike-gated; this orchestration session is
OOM-prone per the architecture note — restart-before-implement is the recorded policy).
**Sentinel baseline:** JOURNEY-0001 green. Suite: incus-dispatcher 86 + llm-proxy 16 -race.
**Revised ITER-0003 scope (see roadmap ITER-0003 block for full detail):**
- Stories: STORY-0072, 0068, 0069, 0070, 0071. **STORY-0015 deferred → ITER-0008** (Run-object collision).
- Done now: Task 0 — 13→0 fixture captured to modules/incus-dispatcher/testdata/journey0003/ (was ephemeral /tmp).
- Splits: 0068 AC-1(CI)/AC-2(cluster e2e); 0071 AC-1(CI projector)/AC-2(integration). 0069 spike-first.
  0070 sequenced after 0069+0072, container-only interim. SCENARIO-0061 seam unit→integration.
- Two tracks: Track1 runner (0069-spike→0072→0069→0070, cluster); Track2 grading/observability
  (0068 AC-1 + 0071 AC-1 CI cores; then cluster e2e). Track2 survives if the 0069 spike stalls Track1.
**Resume:** "continue iterative development" → running-an-iteration picks ITER-0003; scope is recorded,
so proceed to the 0069 spike + decomposition; dogfood isolatable code tasks to the fleet (dispatch policy).
**Iterations:** 3/9 done (ITER-0000/0001/0002); ITER-0003 scope locked, impl pending.

**ITER-0002 — fleet-dogfooded (TDD + hidden holdout oracle on clean checkouts):**
- T1 STORY-0049 AC-1 — queue.ParseDirective strict schema (reject access_cmd/root/unknown) — pass
- T2 STORY-0053 AC-1/2 — Decision.Reason audited denial + deterministic allowlist — pass
- T5 STORY-0048 AC-1 — SanitizeWorkerEnv credential guard (+ fail-closed hardening) — pass
Evidence (orchestrator): SCENARIO-0025/0074 (daemon), SCENARIO-0020 (broker), SCENARIO-0026 (unit).
Harness fix: fleet-dogfood.sh nix-daemon socket readiness wait (concurrent-clone robustness).

**Scope revisions (2 PAR rounds REVISE→APPROVE + PAR impl review):** STORY-0049 AC-5→ITER-0005;
STORY-0049 AC-4 + STORY-0016 + STORY-0011 → ITER-0008. ParseDirective wiring rides laneq (ITER-0006).

**Commits (branch iter-0002):** db2d3ec (reviewed code) + hardening/artifacts commit.
**Last event:** 2026-06-20 — all 6 tasks green, artifacts updated, log+citations validated.
