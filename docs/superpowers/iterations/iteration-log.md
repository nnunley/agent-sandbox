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
