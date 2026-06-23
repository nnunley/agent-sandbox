# Iteration Log

## ITER-0000 — Dogfood milestone (DONE — closed 2026-06-19)

**Started:** 2026-06-18

### Task 0 — stub Queue contract (DONE)
- `modules/incus-dispatcher/queue/`: `Directive` (full field set + `NotBefore`),
  `Queue` interface (atomic Claim + Lease/Touch + Done + Requeue + Reap), and
  `MemoryQueue` stub with importance→priority projection + not-before eligibility.
- Models laneq's contract so the ITER-0006 substrate swap is drop-in (PAR boxing-in fix).
- Evidence: `go test ./queue/` → 7 passing; `go vet` clean.
- Stories advanced: STORY-0057 (claim/lease substrate), STORY-0044 (not-before, stub form).

### Tasks 1–5 — daemon path (DONE, commit 5817399)
- Template/origin validation (STORY-0050): `policy.go` — allowlist + D1 authority split
  (worker proposing a privileged template is DENIED; fail-closed on unknown origin). 6 tests.
- Daemon claim-loop + Directive→Task mapping + minimal outcome (STORY-0057/0058): `daemon.go`
  — pass→done / fail→requeue / park-after-max / reject-invalid-template; external grade is
  authoritative (grade-fail ⇒ fail even if cmd exited 0). 7 tests w/ fake Runner.
- Teardown stop-then-delete (STORY-0062/0063): both runners now stop (bounded) BEFORE delete —
  fixes the verified `incus delete -f` hang.
- Go-exec PATH fix (STORY-0067): `workerToolPath` prepends worker nix-profile + ~/.local/bin
  so agent tools resolve (fixes exit 127). 2 tests.
- Total: 29 tests green, `go build`/`go vet` clean.

### Minimal worker image — flavor 1 (STORY-0075 slice) — VALIDATED on cluster 2026-06-18
- Stock `images:nixos/25.11` (unprivileged) + `nix develop ./fleet-worker --accept-flake-config
  --no-sandbox` → claude 2.1.181, lean-ctx 3.8.8, go 1.26.4, git, jq all resolve (exit 0).
  claude-code/lean-ctx SUBSTITUTE from cache.numtide.com — no build, no baked image, no
  nix-server republish. The two flags are required (cache trust + unprivileged-sandbox).
- `fleet-worker/flake.nix` gained the `nixConfig` substituter (commit a1ab0e0);
  `runner.sh` header documents the two flags.
- Teardown fix (STORY-0062/0063) VALIDATED on a real container: stop-then-delete took 2s,
  no `incus delete -f` hang.

### Non-root NixOS worker (STORY-0075) — declarative, VALIDATED 2026-06-18
- `fleet-worker/worker-container.nix`: non-root `worker` user + nix (flakes, numtide cache,
  trusted-users, `sandbox=false`, `allowUnfree`, `NIX_PATH` session var). Applied to a stock
  container via `nixos-rebuild switch` → `worker` uid 1000 with proper PAM/groups.
- Gotchas hit + fixed (now encoded in skills `nixos-incus-worker`, `nixos-declarative-configuration`):
  imperative useradd→PAM fail; non-login exec missing NIX_PATH; unprivileged sandbox wall on
  rebuild itself (NIX_CONFIG bootstrap + declarative sandbox=false); substituter trust for non-root.

### EXIT (b) — REAL DOGFOOD SUCCEEDED 2026-06-18 ✅
- Dispatched headless claude-sonnet to the NixOS worker on the Peek task (queue.Peek()).
  Worker ran via `nix develop ./fleet-worker --accept-flake-config --no-sandbox` (47 events, rc=0,
  6575-byte diff). go build/vet/test available in the devShell (stdenv pulls cc/gcc).
- **Authoritative clean-room grade PASSED**: applied the worker's diff to an untouched checkout +
  the HIDDEN oracle test → `go build`/`go vet` clean, `go test ./queue/` **10/10** (7 orig + 3
  hidden Peek tests the worker never saw). The Peek implementation is correct.
- The full ITER-0000 loop is proven end-to-end: dispatch → NixOS worker → claude → diff →
  authoritative external grade.

### EXIT (a) — AUTOMATED JOURNEY-0001 HARNESS LANDED 2026-06-19 ✅
- `modules/incus-dispatcher/journey_test.go`: `TestJourney0001_OneShotLifecycle` drives the REAL
  `Daemon` + `DefaultMapToTask` against a recording fake backend (CI-permitted by the scenario card)
  and asserts the journey's contracted final observables — done outcome, queue drained 0/0, worker
  instance reaped exactly once, authoritative external grade reached, worker.diff + result.json
  harvested — plus the lifecycle phase order with teardown strictly last (stop-then-delete after run).
  `TestJourney0001_RejectedDirectiveNeverLaunches` proves the D1 authority gate (step 2 blocks step 3:
  a worker proposing a privileged template never launches the backend).
- Suite: 32 → **34 tests green**, `go vet` clean. behavior-scenarios.md JOURNEY-0001 automation
  status pending→automated; behavior-corpus.md command TBD→`go test ./modules/incus-dispatcher/
  -run TestJourney0001` (sentinel cadence).

### ITER-0000 closed — deferred follow-ups (off the dogfood critical path)
- **Real-Runner→fleet-worker wiring** (DefaultMapToTask currently emits a placeholder `bash -lc`;
  the proven `nix develop ./fleet-worker --accept-flake-config --no-sandbox --command bash runner.sh`
  path was validated MANUALLY on the cluster, EXIT b). Claiming this "done" in the Go path requires
  CLUSTER evidence, so it is deferred to **ITER-0003 (canonical runner modes / STORY-0070)** rather
  than fake-closed here. Evidence-before-assertions.
- **Spikes (parallel, off critical path):** ctx_handoff round-trip (STORY-0034), disposable-unit
  latency benchmark (STORY-0025, partly done) — to be picked up by the audit / a follow-up iteration.
- Optional: merge the dogfood's graded `Peek()` into the repo (oracle-passed, real, useful).

### ITER-0000 wrap-up (structured)

**Completed:** 2026-06-19

**Stories delivered:** STORY-0057 (claim/lease + claim-loop), STORY-0044 (not-before, stub),
STORY-0050 (template/origin D1 validation), STORY-0058 (minimal outcome: pass→done/fail→requeue/park),
STORY-0062 (teardown reaper) + STORY-0063 (stop+reap, partial — decision-log AC-28 → ITER-0001),
STORY-0060 (graceful teardown without regression, partial — stop-then-delete AC-1/AC-3 done +
cluster-validated; async-reaper AC-2 + automated delete-hang regression test → ITER-0001),
STORY-0067 (Go-exec PATH fix), STORY-0075 (minimal non-root NixOS worker, declarative),
STORY-0065/STORY-0066 (directive→Task mapping + JOURNEY-0001 grading step).
(The automated JOURNEY-0001 E2E harness is scenario evidence, NOT a separate backlog story —
earlier drafts mis-labeled it "STORY-0060"; STORY-0060 is the teardown story.)

**Tasks executed:** Task 0 stub Queue contract; Tasks 1–5 daemon path (commit 5817399);
minimal worker image flavor 1 (cluster-validated); non-root NixOS worker (declarative);
EXIT (b) real dogfood (graded Peek 10/10); EXIT (a) automated JOURNEY-0001 harness (journey_test.go).

**Scenarios:** JOURNEY-0001 — automated (fake backend, CI) via `go test ./modules/incus-dispatcher/
-run TestJourney0001` + manually validated E2E on cluster; behavior-corpus.md updated (sentinel cadence).

**Summary:** The walking skeleton is proven end-to-end. Both exit criteria met: (a) the automated
JOURNEY-0001 harness asserts the full one-shot lifecycle (claim→validate→launch→deliver→run→harvest→
grade→outcome→reap) with teardown strictly last and the D1 authority gate enforced; (b) a real agent
task dispatched to a NixOS worker produced an authoritatively-graded diff (10/10, incl. 3 hidden oracle
tests). Suite 36 green, vet clean. Deferred (off critical path, evidence-gated): real-Runner→fleet-worker
wiring → ITER-0003; spikes STORY-0034/STORY-0025 → audit follow-up.

**Audit (PAR, 2026-06-19):** Two parallel adversarial auditors. Verdict GAPS FOUND (0 critical
correctness bugs; gaps were evidence-quality), all resolved inline before confirming done:
- Broken sentinel command (`go test ./modules/incus-dispatcher/…` fails from repo root — nested
  go.mod) → corrected to `cd modules/incus-dispatcher && go test . -run TestJourney0001`.
- Harness claimed artifacts harvested but never asserted them → added `PatchData`/`result.json`/
  authoritative-grade assertions (`journeyBackend.lastResult`).
- Mutation gaps in `passed()` (grade `PatchApplied=false`; framework-error path) → added
  `TestRunOnce_GradePatchNotAppliedIsFail` + `TestRunOnce_FrameworkErrorIsFail`.
- Two JOURNEY-0001 observables (decision-log audit trail, shared-volume cleanliness) were listed
  but neither asserted nor marked deferred → annotated ⏳ in the scenario card (→ ITER-0001 / ITER-0005).
- Minor: softened the harness doc comment to match actual coverage; added explicit `cleanups==0`
  assertion to the rejection test.
Re-verified after fixes: 36 tests green, `go vet` clean, both validators pass.

## ITER-0001 — Coordination plane (DONE — closed 2026-06-19)

**Completed:** 2026-06-19

**Stories delivered:** STORY-0056 (D6 decision log, done), STORY-0027 (thread status AC-1/2,
partial — AC-3 TUI→ITER-0008), STORY-0059 (claim/lease/requeue/park, done — park added),
STORY-0055 (D4 loop + ladder AC-1..6, done — AC-7→ITER-0007), STORY-0058 (AC-23 synchronous
ladder, partial — AC-24→ITER-0007/AC-25→ITER-0004), STORY-0061 (autonomous rungs + human lane
AC-1/2, partial — AC-3→ITER-0007), STORY-0063 (decision-log write on reap AC-28, done).

**Tasks executed:**
- T1 D6 decision log (decisionlog.go), T2 thread status (threadstatus.go), T3 durable Park
  (queue) — all FLEET-AUTHORED via fleet-dogfood (worker TDD + hidden holdout oracle, reviewed,
  applied): commits d4e313a / 6ac3432 / fe67309.
- T4 escalation ladder (ladder.go), T5 human escalations lane (escalationlane.go), T6+T7 full
  D4 RunOnce composition (daemon.go) — local TDD: commits 3721bc4 / 95d1300 / 6aa2384.
- PAR review (2 adversarial reviewers, both CHANGES-NEEDED) → concurrency mutexes
  (ThreadTracker/MemoryDecisionLog/JSONLDecisionLog), nil-lane clarification + tests, MaxAttempts
  deprecation, status-chain coverage: commit d4b7d76. False positives dismissed with evidence.

**Scenarios:** JOURNEY-0001 sentinel stays GREEN through the daemon rewrite (no regression). D4
behavior covered by daemon_test.go: ladder-climbs-then-escalates (SCENARIO-0032/0034/0035 at the
integration seam), autonomous-rung-does-not-escalate (AC-6), human-rung-parks-without-lane,
pass-status-chain, pass-writes-reap-then-done (SCENARIO-0042), concurrent-tracker-and-log (-race).
Queue rules (SCENARIO-0070) in queue/park_test.go.

**Summary:** Promoted the ITER-0000 minimal outcome loop to the full D4 deterministic coordination
plane: claim → thread-status → D1 validate → run → reap-log → authoritative grade → graduated
escalation ladder (autonomous retry/stronger/hard-tier requeues → human rung parks + escalates
non-blocking), with a D6 decision-log entry per transition. Everything substrate/Temporal-independent
and behind interfaces (DecisionLog/EscalationLane/Queue) so ITER-0006 substrate + ITER-0007 Temporal
graft on without rework. Notably, 3 of 7 tasks were built BY THE FLEET ITSELF via the fleet-dogfood
skill — the fleet building the fleet. Suite 36→69 green under `go test -race`, vet clean.

**Audit (PAR, 2026-06-19):** Two parallel adversarial auditors, three tiers. Verdict CLEAN — no
correctness bugs, no semantics drift, `go test -race` clean (69), JOURNEY-0001 sentinel green, all
ITER-0001 ACs proven at the correct (daemon-integration) seam, honest partials, no dead code,
boxing-in low-risk (Queue/DecisionLog/EscalationLane clean interfaces; Policy is a struct — minor,
refactorable for ITER-0002). Two evidence-quality gaps found + resolved inline:
- Scenario-corpus registration (both auditors): SCENARIO-0032/0033/0034/0035/0036/0037/0042/0043/0044/
  0070/0085 were claimed proven in the log but marked Command:TBD in behavior-corpus.md → now
  registered with verified-passing `go test -run` commands.
- Status-transition coverage (auditor B): the climb test only pinned active→done → extended
  TestRunOnce_LadderClimbsThenEscalates to assert the full chain (8 transitions, 3 queued, ending
  blocked), pinning setStatus() on the requeue + escalate paths.
ITER-0001 CONFIRMED DONE.

## ITER-0002 — D1 security perimeter + credential isolation (DONE — closed 2026-06-20)

**Completed:** 2026-06-20

**Stories delivered:** STORY-0049 (D1 directives — AC-1/2/3 done; AC-4 child-inheritance→ITER-0008,
AC-5 immutable-root→ITER-0005), STORY-0053 (origin-restricted allowlist — AC-1/2 done),
STORY-0048 (secret broker — AC-1/2/3 done). Deferred whole by PAR scope review: STORY-0016
(versioned policy obj) + STORY-0011 (worker dispatch) → ITER-0008.

**Tasks executed:** Built BY THE FLEET via fleet-dogfood (TDD + hidden holdout oracle graded on a
clean checkout the worker never saw):
- T1 STORY-0049 AC-1 — `queue.ParseDirective` strict JSON decode (DisallowUnknownFields +
  trailing-data guard) rejecting access_cmd/root/unknown fields. (parse.go + parse_test.go)
- T2 STORY-0053 AC-1/2 — `Decision.Reason`; daemon records the denial reason on D1 reject; policy
  denial message 'worker-origin not allowed for privileged templates'; deterministic-concurrency
  policy test. (daemon.go/decisionlog.go/policy.go/policy_test.go)
- T5 STORY-0048 AC-1 — `SanitizeWorkerEnv` strips raw provider credentials from worker env, wired
  into main.go; **hardened fail-closed** (review follow-up) to also strip credential-pattern names
  (_API_KEY/_KEY/_TOKEN/_SECRET/PASSWORD) so an unlisted provider key cannot leak. (creds.go)
Evidence tasks (orchestrator-authored): scenario_d1_test.go (SCENARIO-0025/0074), llm-proxy
scenario0020_test.go (broker round-trip). Harness fix: fleet-dogfood.sh waits for the nix-daemon
socket before `nix develop` (fixed concurrent golden-clone "Connection refused").

**Scenarios:** SCENARIO-0026 (unit, ParseDirective) automated; SCENARIO-0025 + SCENARIO-0074
(daemon-integration: D1 reject + audited worker-origin denial) automated; SCENARIO-0020 (secret
broker) automated at the container/proxy integration seam — rescoped from its Firecracker-microVM
precondition, which (host credential-socket isolation) defers to ITER-0005. JOURNEY-0001 sentinel
stayed GREEN. incus-dispatcher 86 + llm-proxy 16 tests green under `go test -race`; vet clean.

**Summary:** Closed the D1/D2 security perimeter on the walking skeleton: directives are strictly
schema-checked at the JSON ingestion boundary (ready for the laneq substrate, ITER-0006), worker-
origin privileged-template proposals are denied with an audited reason in the D6 decision log,
allowlist evaluation is proven deterministic/race-free, and workers can never carry raw provider
credentials (fail-closed env guard) — they reach providers only through the broker proxy. Ran on
the fleet itself: 3 code tasks dogfooded with independent holdout grading; lease contention on
`main` was sidestepped with an isolated `iter-0002` worktree per user direction. Two PAR scope
rounds (REVISE→APPROVE) split heterogeneous/greenfield work to ITER-0005/0008; a PAR impl review
drove the ParseDirective-boundary documentation and the fail-closed credential hardening.

## ITER-0003 — Worker reliability & robust result contract (DONE — closed 2026-06-20)

**Completed:** 2026-06-20 (fresh lean session, resuming the scope-locked checkpoint)

**Stories delivered:** STORY-0069 (lean-ctx bridge+proxy — both ACs, smoke), STORY-0070 (canonical runner
--fresh/--continue — AC-1), STORY-0071 (heartbeat tracks ctx_* — AC-1 projector + AC-2 renderer),
STORY-0072 (robust result contract — AC-1 fallback + AC-2 grader-is-truth), STORY-0068 **AC-1** (external
multi-gate grader + grade-JSON contract). **STORY-0068 AC-2 (let-go 13→0) carried** as a cluster-evidence
item (see finding). STORY-0015 stayed deferred → ITER-0008 (Run-object collision, per prior PAR).

**Prior-session commits (checkpointed):** f2e847e (STORY-0071 AC-1 projector, dogfooded+holdout),
e6b847e (STORY-0069 lean-ctx + STORY-0072 AC-1 fallback, smoke-validated).

**Tasks executed:** (this session — local TDD, CI-provable cores)
- STORY-0068 AC-1 — `grader.go`: `GradeReport{passed,clusterA,check_generated,untagged_fails,e2e}` +
  `BuildGradeReport` (pure reducer; `countLeafFailures` maps 13 cluster-A subtest fails → clusterA=13
  without double-counting the parent) + `RunGrade` executor (clone clean checkout, wholesale source-only
  apply excluding generated artifacts via `git apply --exclude`, run ordered gates). `grade_cmd.go`:
  `incus-dispatcher grade --checkout --diff [--out]` subcommand. Proven in CI vs a synthetic in-repo
  fixture (testdata/grade/gogen_ir.fail13.txt) — no let-go toolchain needed.
- STORY-0070 AC-1 — `runner.sh` `parse_mode`/`prepare_worktree` behind a `RUNNER_LIB_ONLY` guard; local CI
  shell test `fleet-worker/tests/runner-modes.test.sh` (fresh wipes the applied change, continue preserves
  it). Backward compatible with the positional wall-clock the smoke harness passes.
- STORY-0071 AC-2 — `heartbeat.go` `RenderHeartbeat` surfaces the last ctx_shell cmd + activity age;
  '(no shell yet)' only when no shell command has run (CI: heartbeat_test.go).
- STORY-0072 AC-2 — `TestGraderIgnoresWorkerSelfReport`: a lying worker result.json claiming success still
  grades Passed=false; RunGrade structurally cannot read the worker self-report (anti-reward-hack, CI-locked).

**Scenarios:** SCENARIO-0061 (smoke, cluster — bridge ON + savings, seam unit→integration),
SCENARIO-0062 (CI — projector + heartbeat renderer), SCENARIO-0063 (CI — fallback + grader-is-truth),
JOURNEY-0003 (AC-1 CI-automated; AC-2 cluster-pending with refs pinned). Corpus commands wired off TBD.

**STORY-0068 AC-2 finding (durable):** refs PINNED — fix #249 = `23bfd87f1`, pre-fix target = its parent
`d4c36cf2d` (recorded in testdata/journey0003/README.md). Attempted local reproduction (go1.26.4):
applying the captured FOCUSED `lvl1-focused.diff` to the parent + `make generate` regenerates a lowered
`core_go_lowered/test/test.go` that fails to compile. Attribution (bare-vs-patched regen): the bare parent
has MANY lowering fallbacks (g-idoms/g-postorder/distinct-imports — what #249 fixes); the focused diff fixes
the cluster-A divergence but LEAVES the test-package lowering (register-test!/use-fixtures), so the
whole-package `gogen_ir` build gate fails. **The captured focused diff is a subset of #249, not a complete
reproduction** — AC-2's `{passed:true}` needs either a cluster-A-isolating gate (count divergence without
gating on the full lowered-package build) or a complete #249-equivalent diff, run on the nix-pinned cluster
worker (AC-2's declared e2e seam). The grader mechanism itself is ready and CI-proven.

**Sentinel corpus results:** JOURNEY-0001 sentinel GREEN (baseline and post-iteration). incus-dispatcher
suite 118 tests green under `go test -race`; vet clean. No `TODO(ITER-0003)` markers remained.

**Summary:** Promoted the skeleton's worker/grading reliability to the productization contract: the external
grader is now a structured, multi-gate, anti-reward-hack source of truth (CI-proven, with a `grade`
subcommand and generated-artifact-aware apply); the runner has canonical fresh/continue modes; the heartbeat
surfaces real ctx_* activity instead of falsely reading idle; truncation always yields a structured fallback
result. The one carried item, the let-go 13→0 e2e, is set up end-to-end (refs pinned, grader ready) and its
remaining work is precisely characterized — a cluster run on the pinned toolchain with a divergence-isolating
gate. PAR scope review was completed in the prior checkpoint (REVISE→revised→approved).

## ITER-0004 — State passthrough & continuity (DONE — closed 2026-06-21)

**Completed:** 2026-06-21

**Stories delivered:** STORY-0029 (resume audit + Thread store, AC-1/2/3/4a — AC-4b TUI → ITER-0008),
STORY-0030 (workspace-claim check + reinvention→stumble capture, AC-1/2/3), STORY-0033 (workspace-lease
registry + continue-or-supersede, AC-1/2/3), STORY-0018 (D3 lean-ctx state passthrough — AC-1/2/3 via
LeanCtxProvider, AC-4/5 via NoopProvider + guard), STORY-0031 (Run.stumble_signals[] + StumbleSignal enum,
AC-1/2 — AC-3/4 mutation/genome → ITER-0008), STORY-0058 AC-25 (fresh handoff bundle on retry — AC-24
Temporal → ITER-0007).

**Tasks executed:** (TDD; interleaved code + evidence)
- T0 — `docs/plans/2026-06-21-handoff-bundle-schema.md`: versioned (schema_version 1) handoff-bundle schema
  (STORY-0018 AC-3 doc deliverable; ITER-0006 targets `Directive.HandoffIn`). Commit d67823a.
- T1+T2 — data model: `thread.go` (Thread/ThreadStore/ResumeSummary), `run.go` (Run/StumbleSignal/9
  StumbleTypes). Commits 04b8687, 8663fe4 (PAR cleanup).
- T3 — workspace-lease registry (WorkspaceKey/Claim/Registry, DecideReuse/Supersede; independent of
  queue.Lease). Commit 4e2b2e7. PAR quality → both APPROVE.
- T4+T5 — `ReconstructResumeAudit` + ContinueRun + SCENARIO-0015 integration harness (resume continues prior
  thread; different thread supersedes-with-reason → StumbleDuplicateWork). Commit 59b6a3c.
- T7 — `context.go`: `ContextProvider` interface + `NoopProvider`; daemon depends only on the interface
  (AC-5), NoopProvider proves AC-4 (handoff loss → grade still authoritative). Commit e8c2ca2.
- T6 — `leanctx_provider.go`: `LeanCtxProvider`, the default adapter (diary↔knowledge facts,
  curated-knowledge exchange, schema-1 handoff bundle with EXPLICIT session id per the STORY-0034 spike).
  Injectable runner makes argv/parse logic unit-testable; a guarded integration test proves a genuine diary
  round-trip against a real lean-ctx in an isolated temp project. Commit 2a1e447.
- T8 — daemon emits a fresh handoff bundle via the ContextProvider on each autonomous requeue (best-effort);
  each retry gets a distinct bundle. Commit 467a93e.

**Scenarios:** SCENARIO-0015 (resume-on-branch: continue vs supersede — automated), SCENARIO-0030 (diary
write/read round-trip — automated at the adapter seam + real-lean-ctx round-trip), SCENARIO-0031 (authoritative
state independent of handoff loss — CI, from T7), SCENARIO-0054 (fresh handoff on requeue — automated, daemon
seam; AC-24 Temporal portion → ITER-0007). SCENARIO-0077 (STORY-0034 spike, cluster — prior). Corpus commands
wired off TBD for 0030/0054.

**Sentinel corpus results:** baseline clean (JOURNEY-0001 green, JOURNEY-0003 AC-1 green; 157 tests). Post-
iteration: incus-dispatcher + llm-proxy **165 tests green under `go test -race`**, vet clean, JOURNEY-0001
sentinel GREEN, zero `TODO(ITER-0004)` markers.

**Summary:** Built the soft-state continuity layer behind a `ContextProvider` abstraction (no hard lean-ctx
coupling — mirrors `queue.Queue`; lean-ctx's commercial upsell makes swappability a requirement). Threads carry
resume audits reconstructable by the daemon; a daemon-local workspace-lease registry forces continue-or-supersede
on (repo, branch) reuse and captures duplicate-work stumbles; the default `LeanCtxProvider` drives real lean-ctx
for diary/knowledge/handoff and materializes a versioned bundle with an explicit session id; and every autonomous
retry is provided a fresh handoff bundle. Authoritative state (diff + oracle grade) never flows through the
provider — the `NoopProvider` is simultaneously the test double and the anti-reward-hack proof. **Decision
(2026-06-21):** T6/T8 were built local-TDD against the real lean-ctx binary + the fake-backend daemon seam
rather than a fresh cluster dogfood — the cross-one-shot session round-trip was already cluster-proven by the
STORY-0034 spike (SCENARIO-0077), so re-proving it on the (then-flaky) cluster added risk without new evidence.
ITER-0006 (substrate) is BLOCKED on the Patrick sync; the next pending iteration is ITER-0005 (gated on the
STORY-0025 benchmark spike).

**Audit (PAR, 2026-06-21):** Two parallel adversarial auditors, three tiers. **Reviewer A: CLEAN** (all
ACs met at declared seams, deferred ACs genuinely not implemented, no reward-hacking, 165 `-race` green).
**Reviewer B: GAPS FOUND — 1 SERIOUS:** `LeanCtxProvider.CreateHandoff` silently wrote an EMPTY
`session_id` to manifest.json when `lean-ctx session save` output failed the parse regex, violating the
schema's REQUIRED `session_snapshot_ref.session_id` (handoff-bundle-schema.md:50); no test covered the
regex-non-match path. Both Tier-2/Tier-3 verdicts agreed: no regressions, JOURNEY-0001 + JOURNEY-0003 AC-1
sentinels green, no unrequested features, no leftover TODO markers. **Resolution (inline, TDD):**
`CreateHandoff` now captures+validates the explicit session id BEFORE any mkdir and FAILS CLOSED (returns
an error, writes no bundle) when no id can be parsed — correct for soft state (better no bundle than a
non-conformant one) and unblocks ITER-0006's reliance on a non-empty SessionID. Regression test
`TestLeanCtxProvider_CreateHandoffRequiresExplicitSessionID` added. Suite 166 green under `-race`, vet clean.
**ITER-0004 CONFIRMED DONE.**

## ITER-0005 — Backend-abstraction & isolation-tier interface slice (DONE — closed 2026-06-21)

**Completed:** 2026-06-21

**Scope decision (user, 2026-06-21):** the original ITER-0005 (14 stories, NixOS-golden +
Firecracker + nspawn + skills) was split. This iteration is the **CI-provable interface slice**
(STORY-0004, 0017, 0020, 0023); the heavy cluster-only infra (STORY-0005/0007/0008/0021/0022/0024/
0075-full/0076/0077/0078) moved to a new **ITER-0005b** (runs on `agent-host`; no local Nix). Prior
iterations were Mac-driven Go coordination code — this keeps that on the verifiable seam, and the
benchmark spike established that the nspawn fast tier can't run until a real Firecracker microVM guest
is stood up first (an ITER-0005b precondition).

**Stories delivered:** STORY-0023 (isolation tier selected per template — AC-1, full). STORY-0004
(execution-backend interface — AC-1/AC-2; AC-3 microVM → ITER-0005b). STORY-0017 (D2 backend-agnostic
interface — AC-1/AC-2; AC-3/AC-4 microVM → ITER-0005b). STORY-0020 (container backend contract — AC-1;
AC-2 microVM → ITER-0005b).

**Pre-iteration scope review (PAR, 2026-06-21):** 2 adversarial reviewers → both REVISE→APPROVE-after-
revisions, high agreement. Shared CRITICAL: tier-field location + tier→backend factory architecture
must be locked before code. Shared SERIOUS: STORY-0023 had no scenario card. A-unique CRITICAL: no
documented interaction with ITER-0008 `worker_kind` dispatch. A-unique SERIOUS: ambiguity on new-work
vs already-coded. **Resolutions (all applied, mapping 1:1 to both reviewers' stated approval
conditions):** (1) design note `docs/plans/2026-06-21-iter0005-backend-tier-design.md` — tier on
`TemplateRule` (D1; NOT on the Directive, so ITER-0006's strict `ParseDirective` is untouched and a
worker can't downgrade isolation), factory `SelectRunner(tier)` OUTSIDE `Runner.Run` (interface
unchanged), tier ⊥ worker_kind orthogonality; (2) SCENARIO-0089 added; (3) new-work made explicit in
the roadmap. One B finding ("`container_runner_test.go` missing") was a **false positive** — the file
exists (326 lines; a nested-go.mod search miss). No scope reduction was needed.

**Tasks executed:** (TDD red-green-refactor; local — all CI-provable, no cluster)
- T1 — `tier.go` (`IsolationTier` Fast/Hard) + `TemplateRule.Tier` + `Policy.TierFor` (unset/unknown →
  Hard, fail-safe). STORY-0023 AC-1. Commit (T1).
- T2 — `backend.go`: `BackendFactory` + `staticBackendFactory.SelectRunner`; unregistered tier errors
  with a `TODO(ITER-0005b)` graft point. STORY-0004/0017 AC-1. Commit (T2).
- T3 — daemon: additive `Backend` field (nil → fall back to `Runner`); `RunOnce` resolves the tier
  from the vetted template, selects the backend, records the resolved tier in D6, and PARKS +
  escalates a directive whose tier has no backend yet (never runs sensitive work on a weaker
  substrate). Updated `TestRunOnce_PassWritesReapThenDone` to the new chronological log order
  (`hard,reap,done`). Commit (T3).
- T4–T6 (evidence) — `scenario0089_test.go` (tier→backend selection; worker-cannot-propose-Hard D1
  guard), `scenario0028_test.go` (daemon substrate-agnostic via `Runner`; compile-time conformance for
  both container runners + the factory), SCENARIO-0076 wired to the existing `container_runner_test.go`
  contract (integration cases self-skip when incus unreachable). Corpus + scenario cards wired off TBD.
  Commit (T4–T6).

**Scenarios:** SCENARIO-0089 (NEW — tier selection, integration; automated), SCENARIO-0028 (D2 backend
interface, unit; automated), SCENARIO-0076 (container contract, integration; automated, self-skipping).
JOURNEY-0001 + JOURNEY-0003 AC-1 sentinels stayed GREEN.

**Sentinel corpus results:** baseline clean (166 tests, JOURNEY-0001 green). Post-iteration:
incus-dispatcher + queue **177 tests green under `go test -race`**, vet clean, JOURNEY-0001 +
JOURNEY-0003 AC-1 sentinels GREEN, zero `TODO(ITER-0005)` markers (2 intentional `TODO(ITER-0005b)`
graft markers remain in `backend.go`, by design).

**Summary:** Locked the backend-agnostic execution seam so ITER-0005b's microVM/nspawn backends graft
in without rework, and landed isolation-tier selection as a D1 (template-declared) property. The tier
lives on the vetted `TemplateRule` — never an author-settable Directive field — so a worker cannot
downgrade isolation and ITER-0006's substrate swap is untouched; tier→backend selection is a factory
OUTSIDE `Runner.Run`, keeping every backend on one interface. The daemon fails safe: a tier with no
registered backend parks the directive rather than running it on a weaker substrate. All work was
CI-provable on the Mac (no cluster needed); the Firecracker/nspawn/golden/skills stack is ITER-0005b.

**Audit (PAR, 2026-06-21):** Two parallel adversarial auditors, three tiers. **Auditor A: CLEAN**
(all ACs proven at declared seams; no correctness bugs; `-race` clean; D1 worker-cannot-downgrade
proven; backward-compat preserved; `TODO(ITER-0005b)` graft point real; no Directive.Tier field, so
ITER-0006's strict parser is untouched). **Auditor B: GAPS FOUND — 1:** STORY-0017 AC-2 ("worker
NixOS config single declarative source") was claimed done but evidenced only in prose — no CI
assertion. **Resolution (inline):** confirmed via episodic memory + the nix files that AC-2's
substance RAN end-to-end on ndn-desktop/agent-host 2026-06-18/19 (real dogfood via
`nix develop ./fleet-worker`; `worker-container.nix` applied via `nixos-rebuild switch`) — so the gap
was missing *pinning*, not missing behavior. Added `fleet-worker/tests/single-source.test.sh`
(SCENARIO-0090): a CI structural test pinning every required pattern in `flake.nix` +
`worker-container.nix` (devShell toolchain, non-root worker, sandbox=false, flakes, declarative
NIX_PATH, local-first `file:///srv/nix-shared` substituter ordering) against silent drift, so a golden
COPY replicates a working worker. AC-2 status refined: "delivered as incus container" DONE+validated;
"delivered as Firecracker guest" + golden-copy replication + immutable-root/writable-scratch
(STORY-0005 AC-1 / STORY-0049 AC-5) → ITER-0005b. **ITER-0005 CONFIRMED DONE.**

## ITER-0005b — Firecracker micro-VM substrate & isolation tiers (DONE — closed 2026-06-22)

**Completed:** 2026-06-22 (cluster-resident on `agent-host`; no Mac CI seam — every AC is e2e,
proven by the Task-0 cluster verification harness `fleet-worker/cluster-tests/run.sh`).

**Stories delivered:** STORY-0007 (durable coord VM — landed earlier in the iteration),
STORY-0021 (fast tier), STORY-0022 (hard tier), STORY-0008 (disposable units + teardown),
STORY-0024 (trust boundary, single-domain v1), STORY-0005 (immutable golden + incus-copy launch).
EPIC-002 now 5/5; EPIC-001 +0005/0008. Deferred microVM ACs from ITER-0005 (STORY-0004 AC-3 backend,
STORY-0017 AC-3 microVM ≤5s = SCENARIO-0029, STORY-0020 AC-2) are substantively proven by this
substrate harness (microvm-boot 708ms; tier runners on one Runner interface) — noted for the audit.

**Tasks executed:** (cluster work driven directly — single-VM/single-host serialization makes parallel
implementer subagents unsafe; the harness IS the PAR-mandated evidence gate. Documented judgment call.)
- T1 — fast-tier substrate: in-guest `fleet-unit.sh` (`systemd-nspawn --ephemeral` over warm RO /nix,
  guest system profile on PATH so units have the full toolchain). Probes `nspawn-fast` (64ms mean /
  72ms p99, N=20; genuine PID-ns isolation via real readlink) + `teardown` (incus-free, 111ms unit-kill).
  Commit 48c7035 (+ isolation correction in b1b22d5).
- T2 — `NspawnRunner` (Runner) under `TierFast`; nonzero-exit→Result, infra-err surfaces, Cleanup no-op
  (ephemeral self-teardown). Unit + live e2e tests. Commit cf7282d.
- T3 — hard-tier substrate: per-task worker microVM boot probe `hardtier` (737ms mean / 909ms p99, gate
  ≤2500ms). (Wired in cf7282d's sibling run.sh edits; measured this iteration.)
- T4 — `FirecrackerRunner` (Runner) under `TierHard`: boot → resolve dnsmasq lease IP → ssh worker →
  run; Cleanup `systemctl stop` (never incus delete). serve entrypoint wires the real Fast+Hard factory;
  both TODO(ITER-0005b) graft markers removed; `nspawnExec`→`hostExec`. Commit 7467bca.
- T5 — `golden.nix` (immutable root + tmpfs /workspace,/tmp scratch) + `golden-image.test.sh`
  (structural); published a real `fleet-golden` incus image; `golden-launch` probe (2.9–3.3s CoW, golden
  marker present = no live build, writable scratch, clean teardown). Commit f9e1f65.
- T6 — `trust-boundary` probe: own-kernel (guest 6.12.78 ≠ host 6.8.0-106-generic) + unit-inside-VM;
  fast-tier disposable-unit env made usable (coreutils on PATH), which also corrected the T1 isolation
  assertion to a genuine signal. Commit b1b22d5.

**Scenarios:** SCENARIO-0003 (golden-launch), SCENARIO-0004 (durable-vm + disposable units/teardown),
SCENARIO-0005 (fast tier), SCENARIO-0006 (hard tier), SCENARIO-0007 (trust boundary, single-domain v1)
— all automated (cluster) and MEASURED PASS 2026-06-22. SCENARIO-0029 (microVM ≤5s) PASS at T0.
Corpus commands were wired off-TBD at Task 0; scenario cards updated with measured results + commands.

**Sentinel corpus results:** baseline clean (durable-vm + microvm-boot PASS, JOURNEY-0001 green). Post-
iteration: all 7 cluster scenarios PASS; harness lib pure-logic + golden-image + single-source
structural tests PASS; `go vet` clean; `go test -race ./...` green (incl. live nspawn + firecracker
integration); zero `TODO(ITER-0005b)` markers remain.

**Summary:** Landed the two-tier isolation substrate on real hardware — `nspawn --ephemeral` disposable
units (Fast, ~64ms) inside the durable Firecracker coord VM for trusted lanes, and per-task Firecracker
microVMs (Hard, ~740ms, own kernel) for sensitive lanes — grafted onto ITER-0005's `BackendFactory`
seam with zero daemon/interface change (Fast→NspawnRunner, Hard→FirecrackerRunner; both graft markers
resolved). Disposable-unit teardown is unit-kill, structurally incus-free (D5 hang avoided). The
durable VM is a hardware trust boundary (distinct kernel from the host LXC); single-domain multi-tenancy
v1 (dynamic multi-domain provisioning deferred to ITER-0006+). Workers launch as immutable golden copies
via btrfs CoW with no live build (FULL golden/skills/provider → ITER-0005c). All cluster-only, proven by
the Task-0 verification harness; the Go backends are additionally CI-provable on the Mac.

**Audit (PAR, 2026-06-22):** two parallel adversarial auditors, three tiers, ran the live cluster
harness + Go suite independently. **Both returned CLEAN** (high-confidence agreement): all 6 stories'
ACs proven at their declared seams (measured on real hardware); Tier-2 impacted scenarios
(0003/0004/0005/0006/0007/0008/0029 + backend 0028/0076/0089) PASS; Tier-3 sentinels JOURNEY-0001 +
JOURNEY-0003 AC-1 green, `go vet` clean, `go test -race ./...` 192 tests pass (baseline 177, net +15 for
the new runner/backend tests — no regression). Zero critical/serious/minor findings; no unrequested
features; no leftover `fleet-golden-copy-*` instances; zero `TODO(ITER-0005b)` markers; exit-code
mapping (task-fail vs infra-err) and incus-delete-free teardown both independently verified.
**ITER-0005b CONFIRMED DONE.**

## ITER-0005c — NixOS golden image, provider routing & curated skills (DONE — closed 2026-06-22)

**Completed:** 2026-06-22 (cluster-resident on `agent-host`/`nix-server`; no Mac CI seam for the
image track — proven by the reused Task-0 harness `fleet-worker/cluster-tests/run.sh` + the Go
contract test on the Mac for provider passthrough).

**Stories delivered:** STORY-0078 (skills-layout discovery + curated bundle build), STORY-0077
(declaratively vendored 13-skill copy-tree at the discovery path), STORY-0076 (provider export +
dispatcher routing). STORY-0075 PARTIAL: AC-1 (FULL golden) done; AC-2/AC-3 carried. EPIC-013 now 3/4.

**Tasks executed:** T0 (cluster-harness wiring for SCENARIO-0065/0066/0067/0068/0069), T1 (STORY-0078
skills bundle + layout doc), T2 (STORY-0077 copy-tree skills into golden), T3 (STORY-0075 AC-1 FULL
golden build/publish via build-golden.sh), T4 (STORY-0076 provider export + dispatcher passthrough +
CI contract), T5 (STORY-0075 AC-2/AC-3 clean-room regen attempt → carried).

**Scope review (PAR):** 2 adversarial reviewers × 2 rounds. R1 → both REVISE (8 findings); revisions
applied. R2 → both confirm 8/8 resolved + 5 clarifications codified → APPROVE. Key resolutions:
STORY-0078 proof = standalone bundle build (no timing paradox); ITER-0005c Task-0 harness wiring;
STORY-0075 AC ordering + carry-allowance; STORY-0076 export-vs-routing split; cluster-only
preconditions; bridge-ON proof; EPIC header fix.

**Scenarios:** added/updated (all wired off-TBD + measured):
- SCENARIO-0069 (skills bundle builds, 13 skills) — **PASS** (`nix build .#agent-skills-bundle`, nix-server).
- SCENARIO-0068 (skills at /etc/claude/skills, copy-tree real files, 0 symlinks) — **PASS** (golden copy).
- SCENARIO-0065 (golden built once, realized toolchain, copy-per-task zero rebuild) — **PASS** (golden copy).
- SCENARIO-0067 (provider export + routing passthrough + grader-determinism) — **PASS** (cluster export + CI TestScenario0067).
- SCENARIO-0066 (clean-room byte-identical regen + bridge-ON graded run) — **CARRIED** (see below).

**Key artifacts:** `fleet-worker/flake.nix` (+agent-skills-nix/agent-skills inputs, agent-skills-bundle{,-etc}
packages, provider CLIs exported), `flake.lock` (hash-pinned), `fleet-worker/golden-full.nix` +
`build-golden.sh` (build+publish FULL `fleet-golden`), `docs/plans/2026-06-22-skills-layout-validation.md`,
`modules/incus-dispatcher/provider_routing.go` (+ TestScenario0067), harness cases 0065-0069 +
`cleanroom-attempt.sh` + `results/cleanroom-2026-06-22.log`.

**Carry (STORY-0075 AC-2/AC-3, per the PAR carry-allowance, trigger a):** the clean-room regen was run
on the golden's nix-pinned go1.26.4 (the declared ITER-0003 seam) against let-go @ d4c36cf2d + the
captured `lvl1-focused.diff`. `make generate` succeeds, but the regenerated native-Go lowered TEST
package does not compile (`pkg/rt/core_go_lowered/test/test.go`: "declared and not used: v73" /
"missing return"), failing `check-generated` (AC-2) and cluster-A. **This reproduces on the pinned
toolchain → it is a genuine upstream let-go native-Go-lowering codegen bug, NOT a Mac-toolchain
artifact — refuting ITER-0003's hypothesis. Same blocker as STORY-0068 AC-2 / JOURNEY-0003.** The
golden + grader are correct (AC-1 green). AC-2/AC-3 unblock when let-go's lowering is fixed upstream.

**Sentinel corpus results:** baseline clean (JOURNEY-0001 + JOURNEY-0003 AC-1 green). Post-iteration:
the 4 new image-track scenarios PASS (0065/0067/0068/0069), 0066 carried; `go test -race ./...` green
(provider_routing + TestScenario0067 added, no regression); `go vet` clean; golden-image +
single-source + harness-lib structural tests PASS; zero `TODO(ITER-0005c)` markers.

**Summary:** Retired the runtime-substitution stopgap with a FULL declarative golden: a one-time
`build-golden.sh` realizes the agent toolchain (claude-code/lean-ctx/go/make) + the 13-skill curated
copy-tree (vendored via agent-skills-nix from a hash-pinned non-flake upstream) + the cheap-implementer
provider CLIs (codex/gemini/qwen) into an immutable `fleet-golden` image; per-task copies launch via
btrfs CoW with zero rebuild. Provider routing is now actually plumbed (the `--provider`/`--model` flags
were previously dead) while the grader stays deterministic. The only gap is the clean-room graded-run
proof, blocked by a faithfully-reproduced upstream let-go codegen bug and carried per the approved
allowance.

## ITER-0006 — Queue substrate: laneq gRPC binding + Go adapter + directive contract

**Completed:** 2026-06-22

**Stories delivered:** STORY-0002 (partial — AC-1 done; AC-2 → ITER-0007), STORY-0044 (partial —
AC-1/AC-2 done; AC-3 → ITER-0007), STORY-0064 (partial — AC-1..AC-14 done; AC-15/AC-16 → ITER-0007),
STORY-0010 (partial — AC-4 done; AC-1 → ITER-0006b; AC-2/AC-3/AC-5 not-chosen decision outcomes).

**Tasks executed:** T0 `laneq.proto` gRPC contract + generated Go stubs (optional-int64 UTC-seconds
timestamps, field-number guard, buf-lint clean). T1 Python gRPC server on the fork `nnunley/laneq`
(branch `grpc-binding` @ `2d1b59e`) over `core.py` — added `parked` status (excluded from
take/peek/reap, no auto-promote) + requeue_count-on-requeue; structured NotFound/Precondition →
NOT_FOUND/FAILED_PRECONDITION error codes; PR #19 CI green (ruff format+check, 94 pytest, ≥95% coverage
with generated stubs omitted). T2 Go `LaneqQueue` adapter implementing `queue.Queue` over the gRPC
client (Importance↔priority, NotBefore↔not_before, Attempts↔requeue_count, Lease.Token↔consumer,
Park↔parked; column-authoritative overlay incl. Lane; strict `ParseDirective` body decode). T3
in-process faithful fake gRPC server + SCENARIO-0091 CI gate (priority/FIFO/lease/requeue/defer/
blocked_by/park/lanes/threading + Peek reclaim/promote parity). T4 SCENARIO-0045 directive-contract
unit evidence (22 AC-mapped sub-tests; AC-2 validation cites ITER-0002 D1 `ValidateTemplate`). T5
`--queue=memory|laneq` selector + `--laneq-addr` + `LaneqQueue.Close()` shutdown seam + Temporal-
sole-writer seam doc (default stays MemoryQueue until ITER-0006b). T6 real-wire SCENARIO-0092 (gated
`LANEQ_GRPC_REAL=1`): boots the real laneq server via `uvx --from git+...@2d1b59e[grpc]` and drives
the Go adapter through the full lifecycle. Every task ran the two-stage PAR (spec-compliance +
code-quality/boxing-in); PAR caught 7 real wire-fidelity bugs pre-merge (Touch seconds-vs-nanos, Lane
overlay omission, fake Peek reclaim/promote divergence, hollow Len-based park assertion, fork local-tz
timestamps, fork hardcoded-priority/missing Take-Peek fields, fork gRPC error-code mapping) and
reverted one honest test-weakening (artificial InvalidArgument-triggered ErrLeaseLost → realistic
FAILED_PRECONDITION/NotFound).

**Scenarios:** SCENARIO-0091 (NEW — CI contract gate, in-process fake, automated); SCENARIO-0092 (NEW —
real-wire e2e via uvx @2d1b59e, gated, automated PASS); SCENARIO-0045 (directive contract, unit,
automated). SCENARIO-0012 (Mac-off) carried → ITER-0006b.

**Sentinel corpus results:** baseline clean (JOURNEY-0001 + JOURNEY-0003 AC-1 green). Post-iteration:
`go test -race ./...` green (283 tests; LaneqQueue + laneqpb + 0091 + 0045 added, 0092 correctly
skipped without `LANEQ_GRPC_REAL`); `go vet` clean; JOURNEY-0001 + JOURNEY-0003 AC-1 sentinels green;
zero `TODO(ITER-0006)` markers; SCENARIO-0091 CI gate green; SCENARIO-0092 real-wire PASS against the
PR-green fork.

**Summary:** Replaced the in-memory `MemoryQueue` stub with the real **laneq** substrate via a new gRPC
binding, behind the unchanged `queue.Queue` interface. Discovery showed laneq already shipped
not_before/blocked_by deferral + leasing upstream, so the work shifted to validate+integrate: a shared
`laneq.proto` contract, a Python gRPC server on a controlled fork (`nnunley/laneq@2d1b59e`, PR #19
green) adding `parked` + requeue_count + adapter-matching error codes, a drop-in Go adapter, a
CI-native fake-based gate (0091), a directive-contract unit proof (0045), a `--queue` selector
(default still memory until the ITER-0006b cluster deploy), and a real-wire e2e (0092). Temporal
becomes the sole writer of the gRPC Defer/Reprioritize seam in ITER-0007. **Logged divergence for
ITER-0008:** real laneq leases are NOT consumer-exclusive (no per-consumer token enforcement on
Touch/Done) — the fake is stricter; multi-consumer delegation must not assume exclusivity.

## ITER-0006b — laneq Nix package + cluster deploy + Mac-off acceptance

**Completed:** 2026-06-23

**Stories delivered:** STORY-0010 (done:ITER-0006b) — AC-1 (Mac-off acceptance, NARROW substrate proof)
done; AC-4 done in ITER-0006; AC-2/AC-3/AC-5 = not-chosen decision outcomes (laneq chosen). The FULL
sustained operator/fleet Mac-off (dispatcher daemon) is tracked as STORY-0074/ITER-0008.

**Tasks executed:** T0 — laneq Nix package (`fleet-worker/laneq.nix`, `buildPythonApplication`→
`buildPythonPackage` on the `nnunley/laneq@2d1b59e` fork; in-build proto stub regen with grpcio-tools
1.76; `checkPhase` runs the fork's 72 real grpc.aio handler tests against the regenerated stubs —
hash reproducible; built on nix-server with `--no-sandbox` per the unprivileged-LXC norm). T1 — deploy
(`fleet-worker/laneq-service.nix` systemd `laneq-grpc` on ndn-desktop:nix-server:9999, SQLite on Incus
host volume `/srv/laneq`; `docs/plans/2026-06-23-laneq-deploy.md` with the Temporal-sole-writer note;
a Nix-wired `laneq-client` env via `python3.withPackages` so clients/probes resolve deps through Nix —
NO hardcoded store paths). T2 — SCENARIO-0092 over the wire (`run.sh laneq-wire`: Go `LaneqQueue`
adapter ↔ the deployed Nix service through an incus proxy 192.168.86.49:50551→:9999, full lifecycle,
5/5 deterministic after fixing a ParkDurability lease-expiry flake). T3 — SCENARIO-0012 Mac-off
PASS-NARROW (`run.sh laneq-macoff`: enqueue cluster-side, drain via a `systemd-run --collect` DETACHED
unit that runs under PID1 independent of the Mac, all done + host-volume DB persists, autonomous
completion). Scenario tests renamed to semantic names (`directive_contract_test.go`/
`laneq_fake_lifecycle_test.go`/`laneq_realwire_lifecycle_test.go`, each `// Proves SCENARIO-NNNN`).
Each task ran the two-stage PAR + direct orchestrator verification; caught + corrected: 3 T0
real-RPC-proof weakenings (listens → serialize-tautology → real 72-test grpc.aio), T1 nc-vs-gRPC +
mount-vs-data + hardcoded-store-path probes, T2 over-the-wire ParkDurability flake (1s lease expiring
across RPCs before Park), and T3's overclaimed "Mac-off" (Mac-session-dependent drain) PLUS a FALSE
"incus can't detach" wall (disproven — systemd-run works).

**Scenarios:** SCENARIO-0092 (`laneq_realwire_lifecycle_test.go` / `run.sh laneq-wire`) — over-the-wire
vs the deployed Nix service, PASS (deterministic). SCENARIO-0012 (`run.sh laneq-macoff`) — cluster-driven
Mac-off, PASS-NARROW (autonomous systemd-run detached drain). SCENARIO-0091/0045 renamed to semantic
test names (unchanged behavior). Real-laneq divergences documented: reap() return-count differs from the
fake (effect hard-asserted via Attempts==1); leases are NOT consumer-exclusive (no token enforcement).

**Sentinel corpus results:** baseline clean (JOURNEY-0001 + JOURNEY-0003 AC-1 green). Post-iteration:
`go test -race ./...` green (283; gated SCENARIO-0092 skipped in the default run); `go vet` clean;
JOURNEY-0001 + JOURNEY-0003 AC-1 sentinels green; zero `TODO(ITER-0006b)`; cluster `run.sh laneq-wire`
+ `run.sh laneq-macoff` PASS.

**Summary:** Productionized laneq as a cluster-resident service: a Nix package of the augmented fork
(gRPC binding + parked + requeue_count), a systemd deployment on ndn-desktop with a host-volume SQLite
DB, and a Nix-wired client env (no hardcoded store paths). Proved the Go adapter wire-compatible with
the deployed service over a real network hop (SCENARIO-0092), and proved the substrate coordinates
autonomously with the Mac uninvolved via a systemd-run detached drain (SCENARIO-0012 PASS-NARROW). The
full sustained operator/fleet Mac-off (dispatcher daemon + event loop) is deferred to ITER-0008
STORY-0074. ITER-0007 (Temporal) builds on the deployed service via the documented gRPC-only write seam.

## ITER-0007 — Eisenhower prioritization logic (CI-provable slice)

**Completed:** 2026-06-23

**Stories delivered:** STORY-0040 (done:ITER-0007 — importance/urgency orthogonality + Q1/Q2/Q3/Q4 quadrant
mapping), STORY-0042 (done:ITER-0007 — human-unrestricted / agent-bounded rescore validation), STORY-0045
(done:ITER-0007 — deterministic temporal projection). PARTIAL (CI-logic ACs closed here, live ACs → ITER-0007b):
STORY-0041 (AC-3 highest-importance-eligible re-asserted; AC-1/AC-2 live sole-writer → 0007b), STORY-0043
(AC-1/AC-3 urgency-math + Q4-never-ages; AC-2 wall-clock aging → 0007b), STORY-0044 (AC-3 sole-caller logic vs
mock laneq; live gRPC → 0007b), STORY-0046 (AC-1 single-writer guard; AC-2 live concurrent reads → 0007b),
STORY-0047 (AC-2/AC-3 agent-bounded rejection + privileged→approval; AC-1 live human-rescore → 0007b). Split-in
stories: STORY-0001 (AC-3 single-writer design done; AC-1/AC-2 → 0007b), STORY-0002 (AC-2 deferral-holder
contract done [mock]; live durable-hold → 0007b), STORY-0055 (AC-7 Temporal-resurface logic done; live re-raise
→ 0007b, operator journey → 0008), STORY-0058 (AC-24 retry-backoff projection logic done [fake clock]; live
durable re-push → 0007b), STORY-0061 (AC-3 urgency-reprojection logic done; live re-raise → 0007b, operator →
0008), **STORY-0064 fully CLOSED** (AC-15/AC-16 done:ITER-0007 — importance/deadline as projection inputs +
agents-propose-vs-humans-set authority; EPIC-010 now 1/1).

**Tasks executed:** T0 — re-anchored stale EPIC-005 design-doc citations (artifact debt from the
`2026-06-18-fleet-orchestration-design.md` restructure). T1 — Eisenhower projection core
(`temporal/projection.go`: `ComputeUrgency`/`ComputeQuadrant`/`ComputeEffectivePriority`, `ImportanceStringToTier`
bridge; 18 unit tests). T2 — SCENARIO-0078 evidence (quadrant transitions, deadline aging, Q4 stability over an
8-day fake-clock timeline). T3 — rescore authority (`temporal/authority.go`: `IsHumanUnrestricted`/`IsAgentBounded`,
agents bounded to 1-tier jumps, no self-promote to Critical). T4 — SCENARIO-0057/0082 evidence (human override,
agent bounds, approval escalation, drifting-agent prevention). T5 — single-writer guard
(`temporal/writer.go`: `GuardedDirective` with `sync.RWMutex` + private fields + role-checked setters). T6 —
SCENARIO-0081 evidence (sole-writer under concurrent reads + temporal writer). T7 — escalation/retry reprojection
(`temporal/escalate.go`: `EscalationThreshold`/`IsEscalationTriggered`/`ReprojectOnEscalation`, importance-dependent
7/5/3/1-day windows). T8 — SCENARIO-0087 logic evidence (7-day operator workflow). Wrap-up: marked story ACs across
EPIC-001/005/008/010; wired 5 scenario execution commands; re-tagged the laneq `Len()/Stats()` `TODO(ITER-0007)`
→ `TODO(ITER-0007b)` (live-cluster observability, out of CI-logic scope).

**Scenarios:** SCENARIO-0078 (`go test -race -run TestScenario0078 ./temporal/`, done:ITER-0007, fake-clock),
SCENARIO-0057 (`...TestScenario0057...`, done:ITER-0007, mock-Temporal), SCENARIO-0082 (`...TestScenario0082...`,
authority routing done; live human-rescore → 0007b), SCENARIO-0081 (`...TestScenario0081|TestMultipleDirectivesIndependent...`,
single-writer guard done; live concurrent-read AC-2 → 0007b), SCENARIO-0087 (`...TestOperatorScenario0087...`,
urgency-reprojection logic done; live re-raise → 0007b, operator journey → 0008). All five green under -race.

**Sentinel corpus results:** baseline 2026-06-23 clean (283 -race green, JOURNEY-0001 + JOURNEY-0003 AC-1 green,
citation check 78/78). Post-iteration: `go test -race ./...` **383 green** (+100 temporal; no regression in the
prior 283), `go vet ./...` clean, zero `TODO(ITER-0007)` markers remaining (re-tagged → 0007b), all 5 new scenario
commands PASS.

**Summary:** Locked the Eisenhower projection + rescore-authority + single-writer + escalation-reprojection logic
as pure, deterministic Go (`importance × urgency → effective-priority + not-before`; bounded vs unrestricted
rescore; the guard that only the Temporal role writes scheduling fields; fake-clock urgency aging) so ITER-0007b's
live Temporal grafts deployment + wiring onto proven algorithms without re-litigating them. No `Run` struct
introduced (defers STORY-0035 colliding-Run risk to ITER-0008); single-writer documented process-level and
orthogonal to laneq's non-exclusive leases. STORY-0064 fully closed; all live/wall-clock/e2e ACs explicitly
carried to ITER-0007b (cluster Temporal) and ITER-0008 (operator UX).

**Audit (PAR, 2026-06-23):** two adversarial auditors in parallel, both **CLEAN** — Tier 1 (every done:ITER-0007
AC PASS, evidence ADEQUATE, tests confirmed non-tautological), Tier 2 (SCENARIO-0091/0045 PASS; queue change is
a TODO re-tag only), Tier 3 (JOURNEY-0001 + JOURNEY-0003 AC-1 sentinels green; suite 283→383, no regression).
Both independently confirmed the ITER-0007b/0008 deferrals legitimate (live Temporal / wall-clock / e2e /
concurrent-daemon), counters correct (EPIC-005 3/13, EPIC-010 1/1), zero `TODO(ITER-0007)`, no unrequested work.
ITER-0007 confirmed done.
