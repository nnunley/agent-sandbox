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
