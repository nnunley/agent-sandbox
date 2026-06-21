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
- ✅ T1+T2 DONE — Data model: thread.go (Thread/ThreadStore) + run.go (Run/StumbleSignal/9 StumbleTypes). Committed
  04b8687, then PAR code-quality cleanup committed 8663fe4 (stumble_signals omitempty + AddStumble comment + gofmt).
  Suite 131 -race green, vet clean, holdout oracle passes on clean checkout. Covers STORY-0029/0030/0031 AC-1/AC-2.
  (Note: a redundant re-dispatch this turn reproduced the data model identically — confirms reproducibility.)
- ✅ T3 DONE — workspace-lease registry (WorkspaceKey/Claim/Registry, ReuseDecision,
  Claim/ActiveClaim/Release/DecideReuse/Supersede; independent of queue.Lease). Fleet-dogfooded on the golden path;
  holdout passed on a clean checkout (authoritative). PAR quality (2 reviewers) → both APPROVE. Committed 4e2b2e7.
  Suite 145 -race green, vet clean. Covers STORY-0033 AC-1/2/3 + STORY-0030 AC-2/3.
- ✅ T4 DONE — ReconstructResumeAudit + ResumeAudit + Thread.OpenQuestions + ContinueRun (STORY-0029 AC-3/AC-4a).
  Fleet-dogfooded; holdout passed on clean checkout. PAR (2) → both APPROVE (applied shared note: document
  ContinueRun shallow-copy). Committed 59b6a3c.
- ✅ T5 DONE — scenario0015_test.go: SCENARIO-0015 integration evidence (resume-on-branch CONTINUES prior thread
  with reconstructed context; different thread must supersede-with-reason → StumbleDuplicateWork). Committed 59b6a3c.
  Corpus + scenario card updated to passing (`go test . -run TestScenario0015`).
- ⏳ **T6 NEXT (CLUSTER seam)** — STORY-0018 AC-1/2/3: wire ctx_agent diary (write/recall) + share/receive_knowledge
  + ctx_handoff create|export|import|pull into runner.sh/daemon per docs/plans/2026-06-21-handoff-bundle-schema.md;
  evidence SCENARIO-0030 on a REAL worker (resolve explicit saved session id per STORY-0034 spike note). This is the
  genuine cluster-integration task — needs a fleet worker with lean-ctx, not CI.
- ⏳ T7 — STORY-0018 AC-4 (CI: daemon-loop, fake backend, handoff absent/corrupt → passed() still grades from
  Result.ExternalGradingResult = SCENARIO-0031 CI-primary) + AC-5 (guard/code-review: daemon claims only via
  queue.Queue). Authorable as CI evidence (like T5).
- ⏳ T8 — STORY-0058 AC-25: daemon emits a FRESH handoff bundle on requeue (ladder path); assert at SCENARIO-0054
  daemon seam (fake backend, no Temporal). Needs a Go HandoffBundle representation per the T0 schema; sequence after T6.
**Grade false-negative (non-blocking):** cluster grade.json reports patch_applied:false for BOTH data-model & T3,
but the full worker.diff applies clean locally AND the holdout passes on a clean checkout (verified 2 ways). Root
cause not the stray files (new-file hunks apply fine) — a cluster-grade-env flake. Authoritative grade = local
clean-checkout holdout (worker never saw it). dogfood-out/ gitignored; `tee` makes bg dispatch exit 0 → read grade.json.
**Sequencing:** Each dogfood grades on a clean HEAD checkout; graded diffs applied+committed before next dispatch.
**FLEET BLOCKER (this turn) + RECOVERY:** T3 fresh-launch failed 2× at spin-up — `nix-env`/`nixos-rebuild` in the
freshly-launched worker races the nix-daemon socket (`cannot connect to socket .../daemon-socket/socket`); the golden
`pristine` snapshot that sidesteps the rebuild was gone (`.mode` empty → fresh fallback). Diagnosis: base
`fleet-dogfood-base` is a bare NixOS container (toolchain via `nix develop` at runtime); the dogfood fresh path
doesn't wait for systemd before rebuild (prep DOES — that's why prep works). User chose: REBUILD GOLDEN. Running
`fleet-dogfood-prep.sh --threshold 1` (forces golden snapshot regardless of cache speed). Commit checkpoint: T0/T1/T2
done (04b8687, 8663fe4, docs d67823a).
**RECOVERY DONE:** root-caused the nix-daemon-socket race; patched fleet-dogfood.sh + fleet-dogfood-prep.sh to
poll `nix store ping --store daemon` (nudging the socket) BEFORE any nix op in a fresh launch (committed b4806c8).
Re-ran prep → golden `pristine` snapshot restored (.mode=golden), validated end-to-end. **T3 re-dispatched from the
golden path** (clones pre-built worker, no rebuild race) — awaiting grade.
**On resume:** harvest dogfood-out/iter0004-workspace/grade.json (read JSON not shell exit) → verify holdout locally
→ PAR quality → apply (exclude stray files) → commit → dispatch T4 (ReconstructResumeAudit).

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

