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
**Iterations:** 3/9 done (ITER-0000/0001/0002); ITER-0003 IN PROGRESS.

**ITER-0003 progress (2026-06-20) — checkpoint; impl continues in a fresh session:**
- ✅ STORY-0071 AC-1 (working-state projector) — fleet-dogfooded + holdout-graded, committed f2e847e.
- ✅ STORY-0069 (lean-ctx FULL enablement) — committed e6b847e, smoke-validated on a real worker.
  DIAGNOSIS (resolved): `gain`'s "Bridge: OFF — proxy not reachable" is NOT the serve daemon (29/29
  doctor, daemon healthy); it needs lean-ctx's SEPARATE compression **proxy** (`proxy enable` +
  `proxy start --port 4444`, "compress tool_results before LLM API"). The fleet **OAuth Bearer token
  forwards transparently through the proxy** to api.anthropic.com (spike-proven). runner.sh now wires
  init+setup+serve --daemon (AC-2) + proxy enable + setsid-nohup proxy start + ANTHROPIC_BASE_URL,
  gated on a curl healthcheck, FAIL-OPEN. Smoke: Tokens saved 376, no "Bridge: OFF".
  (Chain = worker→lean-ctx proxy→Anthropic for the dogfood; worker→lean-ctx→fleet-llm-proxy is ITER-0005.)
- ✅ STORY-0072 AC-1 (fallback result.json on truncation) — committed e6b847e, smoke-validated.
- Commits this iteration: f2e847e (0071 AC-1), e6b847e (0069 + 0072 AC-1). Suite 106 -race (Go side).

**ITER-0003 REMAINING (fresh session):**
- STORY-0070 (runner --fresh/--continue modes; composes 0069+0072 — now both exist).
- STORY-0068 AC-1 (grader + grade-JSON {passed,clusterA,...}; CI vs synthetic fixture) + AC-2 (13→0
  e2e using the captured fixture at modules/incus-dispatcher/testdata/journey0003/ + pin let-go ref).
- STORY-0071 AC-2 (live heartbeat integration), STORY-0072 AC-2 evidence (grader-is-truth).
- Evidence/corpus: SCENARIO-0061/0062/0063 + JOURNEY-0003 commands; wrap-up; then auditing-progress.
**Resume:** "continue iterative development" → ITER-0003 scope is recorded in the roadmap; pick up at
STORY-0070. Reusable harness preserved IN-REPO (not /tmp): `fleet-worker/spikes/` (lean-ctx runner
smoke + chain/doctor spikes + README with the proven recipe); ITER-0003 dogfood brief/oracle in
`.iter-scratch/iter0003-t71-*`.

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
