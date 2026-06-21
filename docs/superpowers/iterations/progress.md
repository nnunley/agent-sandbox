# Progress

**Phase:** ITER-0004 SCOPE REVIEW (2026-06-21) — State passthrough & continuity. Gate (STORY-0034 spike) CLEARED.
Sentinel baseline this session: JOURNEY-0001 green, JOURNEY-0003 AC-1 green; incus-dispatcher 123 tests -race-clean, vet clean.
Citation check OK (78 cited stories exist). Scope: STORY-0029/0030/0033/0018/0031 + STORY-0058 AC-25.
**PAR scope review round 1 → both REVISE** (high agreement). Revisions applied to roadmap: STORY-0031 AC-3/AC-4
→ ITER-0008 (keep AC-1/2 capture); STORY-0018 AC-4→anti-reward-hack test, AC-5→discipline proof, AC-3→emit
bundle schema; STORY-0029 AC-4 split (4a here / 4b TUI→ITER-0008); STORY-0033 lease registry daemon-local;
schema-lock-upfront (Thread+deadline / Run additive / StumbleSignal / handoff-bundle); SCENARIO-0078 new (AC-25).
**PAR scope review COMPLETE (2 rounds):** R1 both REVISE (substantive); R2 both REVISE (artifact-sync + 2
clarifications) — both R2 reviewers VERIFIED core design (additive Run, abstract lease, schema-lock, gate cleared).
All findings applied: SCENARIO-0078 collision → SCENARIO-0054; STORY-0029 AC-4a daemon-reconstruct path; SCENARIO-0031
CI-primary seam (no carry-item); STORY-0033 lease registry = separate daemon-local map (not queue.Lease); req-card
sync (EPIC-001/004). Citation check OK. **Scope APPROVED.**
**Decomposed into T0–T8** (T0 bundle-schema doc; T1 Thread struct+store; T2 Run/StumbleSignal; T3 workspace-lease
registry; T4 ReconstructResumeAudit; T5 SCENARIO-0015 harness; T6 lean-ctx wiring FLEET; T7 AC-4/AC-5 CI; T8 AC-25
fresh-handoff-on-retry). Executing FLEET-DOGFOODED (TDD brief + hidden holdout oracle, graded on clean checkout).
**Local memory guard set:** GOMEMLIMIT=2GiB in .claude/settings.local.json (user's 16GB Mac — avoid macOS OOM).

**Status this session (2026-06-21 continued):**
- ✅ T0 DONE — docs/plans/2026-06-21-handoff-bundle-schema.md (STORY-0018 AC-3 deliverable).
- ✅ T1+T2 DONE — Data model: thread.go (Thread/ThreadStore) + run.go (Run/StumbleSignal/9 StumbleTypes). Fleet
  dogfood executed, oracle verified locally (all 107 tests pass -race-clean, vet clean). Diff applied+committed to
  HEAD (commit 04b8687). Deliverable hygiene verified (excluded stray AGENTS.md/LEAN-CTX.md that broke grader).
  Covers STORY-0029/0030/0031 AC-1/AC-2.
- ⏳ T3 READY TO DISPATCH — workspace-lease registry (WorkspaceKey, WorkspaceClaim, WorkspaceRegistry, ReuseDecision,
  Claim/ActiveClaim/Release/DecideReuse/Supersede). Brief + oracle staged (.iter-scratch/iter0004-workspace-*).
  Depends on T1+T2 at HEAD (✅ committed). Covers STORY-0033 AC-1/2/3 + STORY-0030 AC-2/3.
**Sequencing:** Each dogfood grades on a clean HEAD checkout; graded diffs applied+committed before next dispatch
(T3 needs thread.go/run.go ✅ at HEAD; T4 needs Thread+Result; T6/T7/T8 build further).
**Next:** Dispatch T3 fleet dogfood (brief: .iter-scratch/iter0004-workspace-brief.txt; oracle:
.iter-scratch/iter0004-workspace-oracle.sh; ref: HEAD). Fleet infrastructure via incus + nix shared cache.

---

**Phase (prior):** ITER-0003 DONE + AUDIT CLEAN (2026-06-21) — Worker reliability & robust result contract.
Closed this fresh lean session, resuming the scope-locked checkpoint. **PAR audit (2 adversarial auditors,
3 tiers) → CLEAN:** every ITER-0003 AC met at the correct seam; STORY-0068 AC-2 confirmed honestly carried
(not falsely done); JOURNEY-0001 sentinel green; one MINOR (runner lib-only guard return-vs-exit) fixed.
**Next: ITER-0004 (State passthrough & continuity)** — **GATE CLEARED 2026-06-21.** The STORY-0034
ctx_handoff round-trip spike ran on a cluster worker → **PASS (airtight)**: a 48-bit nonce injected into
iteration-1 only was recorded via `lean-ctx session`, serialized to disk, and recovered EXACTLY by
iteration-2 (a separate `claude -p` process). No data loss. STORY-0034 done:ITER-0000; STORY-0052
AC-10/11 (handoff import) unblocked. Harness in-repo: `fleet-worker/spikes/leanctx-handoff-{spike,probe}.sh`.
**Implementation note for ITER-0004:** resolve the explicit saved session id (or use auto-context); bare
`lean-ctx session load latest` returns "starting fresh" though the decision IS on disk. ITER-0004 is
cluster-heavy (continuity across thread boundaries).
**Sentinel baseline (this session):** JOURNEY-0001 green; incus-dispatcher 118 -race, vet clean.
**ITER-0003 delivered:** STORY-0069 (lean-ctx bridge+proxy, smoke), STORY-0070 (runner
--fresh/--continue, CI shell test), STORY-0071 (projector AC-1 dogfooded + heartbeat renderer AC-2 CI),
STORY-0072 (fallback result.json AC-1 smoke + grader-is-truth AC-2 CI), STORY-0068 **AC-1** (multi-gate
external grader + grade JSON, CI vs synthetic fixtures; `grade` subcommand; generated-artifact exclusion).
STORY-0015 stayed deferred → ITER-0008.
**CARRIED (the one open item) — STORY-0068 AC-2 (let-go 13→0 cluster e2e):** refs PINNED (fix
#249=23bfd87f1, target=parent d4c36cf2d; testdata/journey0003/README.md). Local repro (go1.26.4) showed
the captured FOCUSED `lvl1-focused.diff` is a SUBSET of #249 — it fixes the cluster-A lowering divergence
but leaves the test-package lowering (register-test!/use-fixtures), so the whole-package `gogen_ir` build
gate fails. Remaining work (cluster, nix-pinned toolchain): a cluster-A-isolating gate (count divergence
without gating on the full lowered-package build) OR a complete #249-equivalent diff. Grader is ready.
**Resume:** "continue iterative development" → running-an-iteration picks ITER-0004. Optionally schedule a
cluster-evidence pass for STORY-0068 AC-2 + STORY-0071 AC-2 live heartbeat-print on a real fleet worker.
**Iterations:** 4/9 done (ITER-0000/0001/0002/0003); ITER-0004 next (ITER-0006 blocked on Patrick sync).

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

