# Progress

**Phase:** ITER-0002 DONE (closed 2026-06-20) — D1 security perimeter + credential isolation,
on branch `iter-0002` (main was lease-held; isolated worktree per user direction). Awaiting
orchestrator audit (auditing-progress) + merge to main.
**Iterations:** 3/9 done (ITER-0000, ITER-0001, ITER-0002); ITER-0003 next pending.
**Sentinel corpus:** JOURNEY-0001 green. Suite: incus-dispatcher 86 + llm-proxy 16 under -race.

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
