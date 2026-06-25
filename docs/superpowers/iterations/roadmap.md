# Roadmap

Ordering principles (from the design + the provisional substrate decision):
- **ITER-0000** proves the core one-shot lifecycle journey on the PROVEN container
  backend, reusing `modules/incus-dispatcher`, with a *stub* queue behind an
  interface (real substrate deferred). Plus the two validation spikes.
- **Substrate-coupled work (ITER-0006) is deferred** until after the Patrick sync
  (2026-06-19). Nothing before it depends on real laneq — the stub Queue interface
  carries the skeleton.
- **Temporal time-plane + Eisenhower prioritization (ITER-0007)** comes after the
  substrate, because it needs laneq's `not-before` field.
- **Micro-VM / NixOS golden / isolation tiers (ITER-0005)** comes after the
  benchmark spike informs the tier choice.
- ~Half the corpus is NixOS/microvm infra → proven via smoke/readiness scenarios,
  not classic unit tests. The harness must support both.

## Walking skeleton (ITER-0000) — **the DOGFOOD milestone**

**Top priority: reach a real dogfood ASAP.** ITER-0000 is not a toy skeleton — it
is the minimal slice that lets us dispatch a *real* agent task to the cluster and
get an oracle-graded result (the same loop that produced the 13→0 fix). Exit
criterion (b) below is "the dogfood runs for real," not just "JOURNEY-0001 passes
on a fake backend."

**Dogfood critical path (everything here must work to dogfood):**
core lifecycle stories + **STORY-0067 (Go-exec PATH fix — pulled from ITER-0003;
hard `127` blocker)** + a **minimal container worker image** (thin slice of
STORY-0075: claude-code + lean-ctx + toolchain via cached substitution; full
golden / Ubuntu-retire / micro-VM stays ITER-0005).

**Explicitly OFF the dogfood critical path (do not block the dogfood on these):**
both spikes (STORY-0034, STORY-0025 — run in parallel); robust result contract
(STORY-0072, ITER-0003 — the external grade is authoritative, a missing
`result.json` doesn't break the dogfood); Temporal, substrate, escalation ladder,
tiers, audit.

**Intent:** Drive a single directive end-to-end on the container backend — claim
→ validate template → launch from golden → deliver repo → run runner → harvest
diff+result → authoritative external grade → minimal outcome (pass→done /
fail→requeue) → stop+reap — with a stub queue, and stand up the E2E journey
harness. Plus prove the two unvalidated assumptions (in parallel).

**Two exit criteria:** (a) JOURNEY-0001 passes in the harness (may use a fake
backend for CI); (b) **a real dogfood run** — a genuine agent task dispatched to a
real cluster container, producing an oracle-graded diff.

**Design rationale:** This is the thinnest slice that proves the product exists:
one directive becomes one graded result with the container cleaned up and no
`incus delete` hang. It reuses the existing dispatcher `Runner`
(launch/deliver/run/harvest/grade already work), adds only the minimal daemon
claim-loop + template validation + minimal outcome handling + the teardown fix,
and is backend- and substrate-agnostic so laneq, Temporal, escalation, and the
micro-VM backend graft on later without rework. The two spikes de-risk the
design's only unproven bets before we build on them.

**Journey scenario:** JOURNEY-0001 (full one-shot lifecycle, directive→completion).

**Harness-first task:** Task 0 builds the E2E journey harness — a scripted driver
that enqueues a directive (stub queue), runs the daemon loop once against a real
container (or a fake backend for CI), and asserts the journey's final observables
(graded result present, container stopped+reaped). Every later iteration extends
it. Supports Go integration tests + shell/smoke assertions for infra steps.
**Task 0 deliverables (per PAR scope review):** (a) a minimal valid **test
template** + the **allowlist** that authorizes it; (b) a **grader fixture** (the
existing dispatcher external-grading path + `context-anchored-patching`, or a
mock) with a defined grade-JSON shape; (c) **STORY-0060's teardown-regression
assertion** (delete-hang never occurs) lives here, not as a separate story;
(d) confirm lean-ctx is present in the worker image (already in
`fleet-worker/flake.nix`) — ITER-0000 runs the runner WITHOUT the bridge
(compression/bridge-ON is STORY-0069, ITER-0003).

**Stub Queue contract (boxing-in mitigation for ITER-0006):** the stub MUST model
the contract laneq will satisfy, so the ITER-0006 swap is drop-in: a `Directive`
struct with the full field set (intent, template, origin, importance, deadline,
lane, repo/ref/task, handoff_in?, grade?, max_attempts) + a `not-before`
placeholder, and a `Queue` interface with **atomic claim + lease (timeout +
renewal) + requeue** semantics — NOT a naive pop. Document this struct/interface
in the worker/daemon design notes as Task 0's first output.

**Stories committed:**
- STORY-0057 (EPIC-008) — daemon claims next directive (via a stub Queue interface)
- STORY-0050 (EPIC-006) — validate template against allowlist + origin
- STORY-0051 (EPIC-006) — launch worker container from golden + shared volumes
- STORY-0052 (EPIC-006) — deliver repo via bundle/clone (+ import handoff if present)
- STORY-0019 (EPIC-001) — run template runner (lean-ctx setup + agent invocation)
- STORY-0065 (EPIC-011) — harvest worker diff + result artifacts
- STORY-0066 (EPIC-011) — authoritative external grade on clean checkout
- STORY-0058 (EPIC-008) — coordination outcome **(ITER-0000 AC scope below)**
- STORY-0062 (EPIC-009) — stop-first-then-delete (no `incus delete -f` in the loop)
- STORY-0063 (EPIC-009) — stop worker + reap instance **(ITER-0000 AC scope below)**
- STORY-0067 (EPIC-012) — **Go-exec PATH fix** (pulled from ITER-0003; dogfood-critical, fixes `127`)
- STORY-0075 (EPIC-013) — **minimal container worker image** (thin slice: claude-code+lean-ctx+toolchain via cached substitution; full golden/Ubuntu-retire/micro-VM → ITER-0005)
- STORY-0034 (EPIC-004) — **SPIKE** (parallel, OFF critical path): ctx_handoff round-trip
- STORY-0025 (EPIC-002) — **SPIKE** (parallel, OFF critical path): disposable-unit spin-up benchmark

(Correction: STORY-0060 is "graceful container teardown without regression" — the
stop-then-delete mechanism (AC-1/AC-3) was delivered + cluster-validated in ITER-0000;
its async-reaper AC-2 + an automated delete-hang regression test carry to ITER-0001.
The Task 0 E2E harness is JOURNEY-0001 evidence, not a separate backlog story.)

**ITER-0000 AC scoping, gates & deferrals (from PAR scope review):**
- **STORY-0058** — IN: AC-22 (pass→done) + a *simple* fail→requeue (synchronous, no
  Temporal). DEFER: AC-23 full escalation ladder → ITER-0001; AC-24 Temporal-backed
  retry → ITER-0007; AC-25 fresh-handoff-on-retry → ITER-0004.
- **STORY-0063** — IN: AC-26/27 (stop + reap). DEFER: AC-28 (D6 decision-log write)
  → ITER-0001 (needs STORY-0056's writer interface). ITER-0000 may emit a plain
  stderr line, not the D6 log.
- **STORY-0052** — IN: AC-9 (deliver repo via bundle/clone). GATE: AC-10/11 (handoff
  import) execute only if STORY-0034 validates the round-trip; if the spike fails,
  defer to ITER-0004.
- **STORY-0019** — IN: AC-12/14 (lean-ctx `setup` + agent invocation). DEFER: AC-13
  (`lean-ctx serve` / bridge) → ITER-0003 (STORY-0069). Runner works without the
  bridge (no compression) for the skeleton.
- **STORY-0062** — drop the launch-from-golden AC (duplicate of STORY-0051); STORY-0062
  is teardown/reaper mechanism only.
- **STORY-0066** — fix dangling scenario ref (SCENARIO-0077 → the grading scenario /
  JOURNEY-0001 step 9); grader invocation + grade-JSON shape defined in Task 0.
- Volumes: STORY-0051 attaches nix-cache (RO) + handoff-store (RW); STORY-0052 only
  delivers repo + (gated) imports handoff.

**Status:** done (both exit criteria met: (a) JOURNEY-0001 automated harness green;
(b) real dogfood — graded `queue.Peek()` 10/10. Off-critical-path follow-ups deferred:
real-Runner→fleet-worker wiring → ITER-0003 (done); STORY-0034 spike → **PASS 2026-06-21** (cleared
the ITER-0004 gate; SCENARIO-0077); STORY-0025 benchmark spike → **DONE 2026-06-21** (nspawn 76 ms vs
microVM 1861 ms; SCENARIO-0008/0009; cleared the ITER-0005 gate).)

## Iteration list

### ITER-0001 — Coordination plane + audit hardening

**Stories (AC-scoped after PAR scope review — substrate/Temporal-independent only):**
- STORY-0055 (AC-1..AC-6: pass→done, fail-transient→retry-same, fail-repeats→stronger-worker,
  fail-still→hard-tier, authority-limit→human lane, privileged-only-via-human). **DEFER AC-7
  (Temporal re-surfaces stale escalations) → ITER-0007.**
- STORY-0058 (AC-23: synchronous escalation-ladder climb). AC-22 done:ITER-0000. **DEFER AC-24
  (Temporal-backed retry/backoff) → ITER-0007; AC-25 (fresh handoff on retry) → ITER-0004.**
- STORY-0059 (AC-1..AC-4: deterministic claim/lease/requeue/park against the stub queue).
- STORY-0061 (AC-1: autonomous climb of pre-approved rungs; AC-2: non-blocking human lane).
  **DEFER AC-3 (urgency-driven resurfacing) → ITER-0007** (carries SCENARIO-0087).
- STORY-0027 (AC-1: thread status field {queued,active,paused,blocked,done,abandoned};
  AC-2: transitions recorded). **DEFER AC-3 (operator pause/block/resume from TUI) → ITER-0008.**
- STORY-0056 (AC-1..AC-4: D6 append-only JSONL decision log behind a swappable writer interface).
- STORY-0063 (AC-28: decision-log write on stop+reap). AC-26/27 done:ITER-0000.

**Rationale:** Promote the skeleton's minimal outcome handling to the full D4 deterministic loop +
graduated escalation ladder (incl. non-blocking human escalations lane as a durable FIFO — NOT
Temporal-aged yet), thread-status tracking, and the D6 append-only decision log behind a swappable
writer interface. Everything here is substrate- AND Temporal-independent; the escalations lane is a
plain durable queue and retries are synchronous. The time-plane behaviors (urgency aging, Temporal
backoff, resurfacing) split out to ITER-0007; TUI control and agent/delegation/mutation audit to ITER-0008.
**Status:** done:ITER-0001 (2026-06-19) — T1 decision log / T2 thread status / T3 park
dogfood-authored (TDD + holdout); T4 ladder / T5 human lane / T6+T7 D4 daemon loop local TDD;
PAR-reviewed (concurrency + coverage fixes applied); suite 69 green under -race, JOURNEY-0001
sentinel green. Stories done: 0055, 0056, 0059, 0063; partial (deferred ACs remain): 0058
(AC-24→0007, AC-25→0004), 0061 (AC-3→0007), 0027 (AC-3→0008).
**PAR scope review (2026-06-19):** 2 adversarial reviewers → both REVISE. High-confidence findings,
all applied at roadmap AC-scoping level: (1) 3 Temporal-coupled ACs split to ITER-0007 (0055 AC-7,
0058 AC-24, 0061 AC-3); (2) STORY-0027 AC-3 (TUI) → ITER-0008; (3) **STORY-0054 (audit all runs/
delegations/mutations) dropped from ITER-0001** — its coordination-level audit is already STORY-0056;
its distinct delegation/mutation/replay value is ITER-0008 (STORY-0032) — deferred there to avoid
duplicating D6; (4) 0058 AC-25 confirmed → ITER-0004. Artifact-debt noted (non-blocking): source-line
citations for STORY-0055/0056/0059/0061 point at adjacent spec sections (ACs match design intent; D4
is spec ~205-224, not the cited 188-208) — fix in a docs pass.
**Impacted scenarios:** SCENARIO-0032 (pass→done), SCENARIO-0034/0035 (escalate worker/template),
SCENARIO-0036 (human lane), SCENARIO-0042 (decision-log JSONL), SCENARIO-0070 (claim/lease/requeue/park),
SCENARIO-0085 (autonomous climb). (SCENARIO-0087 urgency-resurface moves with AC-3 → ITER-0007.)
**Look-ahead check:** depends on ITER-0000's outcome hook; decision-log + claim/lease stay behind
interfaces so the ITER-0006 substrate swap and the ITER-0007 Temporal time-plane graft on without rework.

### ITER-0002 — D1 security perimeter + credential isolation

**Stories:** STORY-0049 (AC-1/2/3), STORY-0053 (AC-1/2), STORY-0048 (AC-1/2/3)
**Rationale:** D1 intent/template provisioning + origin/authority enforcement with audited
denial; secret broker (no raw provider creds to workers). Hardens the skeleton's minimal
template check.
**Status:** done:ITER-0002 (2026-06-20) — fleet-dogfooded (TDD + hidden holdout oracle on
clean checkouts): T1 queue.ParseDirective (strict schema, STORY-0049 AC-1), T2 denial-reason
audit + deterministic allowlist (STORY-0053), T5 SanitizeWorkerEnv fail-closed credential
guard (STORY-0048 AC-1). Evidence: SCENARIO-0025/0026/0074 + SCENARIO-0020 (broker proof,
container/proxy seam). incus-dispatcher 86 + llm-proxy 16 tests green under -race; vet clean.
PAR scope review (2 rounds REVISE→APPROVE) + PAR impl review applied.
**Scope revisions (PAR):** STORY-0049 AC-5 (immutable root + tmpfs) → ITER-0005; STORY-0049
AC-4 (worker child-directive inheritance) → ITER-0008; STORY-0016 + STORY-0011 (greenfield
policy/dispatch objects, no scenarios) → ITER-0008. ParseDirective is the JSON ingestion
boundary; live wiring rides the laneq substrate (ITER-0006).
**Impacted scenarios:** SCENARIO-0025 (D1 reject), SCENARIO-0026 (schema), SCENARIO-0074
(worker-origin denial + audit), SCENARIO-0020 (secret broker).
**Look-ahead check:** depends on ITER-0000 template validation; independent of substrate.

### ITER-0003 — Worker reliability & robust result contract

**Stories (revised after PAR scope review):** STORY-0072, STORY-0068, STORY-0069, STORY-0070, STORY-0071
**Rationale:** The productization-plan reliability cluster: truncation-robust result
contract, grading round-trip proof, lean-ctx bridge ON, canonical runner modes, ctx_*-aware
heartbeat. (Go-exec PATH fix STORY-0067 landed in ITER-0000.)
**Status:** done:ITER-0003 (2026-06-20) — fresh lean session per the checkpoint. Delivered: STORY-0069
(lean-ctx bridge+proxy, smoke), STORY-0070 (runner --fresh/--continue, CI shell test), STORY-0071
(projector AC-1 dogfooded + heartbeat renderer AC-2 CI), STORY-0072 (fallback result.json AC-1 smoke +
grader-is-truth AC-2 CI), STORY-0068 AC-1 (multi-gate external grader + grade JSON, CI vs synthetic
fixtures; `grade` subcommand; generated-artifact exclusion). **STORY-0068 AC-2 (let-go 13→0) is the one
carried item** — refs pinned (#249=23bfd87f1, target=parent d4c36cf2d), but local repro is toolchain-
sensitive (local go1.26.4 `make generate` regenerates a non-compiling lowered test pkg), so AC-2's green is
a cluster-worker run on the nix-pinned toolchain (its declared e2e seam) — carried to a cluster evidence
pass. Suite 118 green, -race clean; JOURNEY-0001 sentinel green. Commits: f2e847e, e6b847e (prior session)
+ this session's grader/runner/heartbeat commits. **Earlier checkpoint:** scope was PAR REVISE→revised
(2026-06-20); impl resumed this session.
**Scope revisions (PAR consensus — both reviewers REVISE):**
- **STORY-0015 (Run object/artifact_refs) DEFERRED → ITER-0008** — not in the productization spec;
  its Run shape collides with STORY-0011's Run (worker_id/worker_kind/policy_id). ITER-0003 keeps
  artifact capture via the existing `Result.Artifacts`.
- **Evidence durability (Critical):** the 13→0 fixture is now captured in-repo at
  `modules/incus-dispatcher/testdata/journey0003/` (was ephemeral `/tmp/lvl1-focused.diff`).
- **STORY-0068 split:** AC-1 = generic grader mechanism + grade-JSON shape, proven in **CI** vs a
  small synthetic in-repo fixture; AC-2 = reproduce 13→0 as a **cluster e2e** (JOURNEY-0003) using
  the captured fixture + a pinned `let-go` ref (ref TODO at impl time).
- **STORY-0071 split:** AC-1 = events.jsonl→working-state **projector logic (CI unit)**; AC-2 = live
  heartbeat (**integration/cluster**).
- **STORY-0069:** **spike first** — prove in-container `lean-ctx serve` + bridge reachability before
  building AC-1/AC-2 (container-only; microVM path is ITER-0005). Fix SCENARIO-0061 seam `unit`→`integration`.
- **STORY-0070:** sequence AFTER 0069+0072 (its AC composes them); scope to **interim
  container-runner modes** — multi-backend canonicalization → ITER-0005.
**Decomposition (two tracks; Track 2 proceeds even if the 0069 spike stalls Track 1):**
- Task 0 (DONE): capture 13→0 fixture into testdata.
- Track 1 (runner, cluster-gated): 0069-spike → STORY-0072 (fallback result.json) → STORY-0069
  (bridge ON) → STORY-0070 (runner --fresh/--continue capstone).
- Track 2 (grading/observability): STORY-0068 AC-1 (grader+synthetic fixture, CI) + STORY-0071 AC-1
  (projector, CI); then STORY-0068 AC-2 (13→0 e2e) + STORY-0071 AC-2 (live heartbeat) as cluster evidence.
- "Minimal Mac-off smoke" is harness scaffolding (full STORY-0074 → ITER-0008); not an owning story.
**Impacted scenarios:** SCENARIO-0061 (bridge-ON, integration), SCENARIO-0062 (heartbeat),
SCENARIO-0063 (truncation fallback), JOURNEY-0003 (13→0 grading, e2e).
**Look-ahead check:** runner work (0069/0070) is container-only → ITER-0005 microVM backend grafts a
new runner path; STORY-0015 Run object → ITER-0008.

### ITER-0004 — State passthrough & continuity (post-spike)

**Stories:** STORY-0029, STORY-0030, STORY-0033, STORY-0018, STORY-0031, **STORY-0058 AC-25 (fresh handoff bundle on retry — split in from ITER-0001 per PAR)**
**Rationale:** Build the lean-ctx-based handoff continuity once STORY-0034 proves
the round-trip: context preservation across thread boundaries, anti-reinvention,
branch/workspace claim checks, soft-state-not-authoritative discipline, stumble
signals. Gated on the ITER-0000 spike outcome. Also lands STORY-0058 AC-25 (a fresh
handoff bundle accompanies each retry — needs the handoff machinery built here).
**GATE CLEARED (2026-06-21):** STORY-0034 ctx_handoff round-trip spike → **PASS** (airtight nonce
round-trip across two `claude -p` invocations on a cluster worker, no data loss; SCENARIO-0077).
ITER-0004 may now start, and STORY-0052 AC-10/11 (handoff import, gated in ITER-0000) are unblocked.
**Implementation note from the spike:** the handoff machinery must resolve the explicit saved session id
(or rely on lean-ctx auto-context); bare `lean-ctx session load` (id=`latest`) returns "starting fresh".
**PAR scope review (2026-06-21, round 1 — 2 adversarial reviewers → both REVISE; high agreement).
Scope revisions applied (round 2 confirming review → APPROVE):**
- **STORY-0031 split:** KEEP AC-1 (Run.stumble_signals[] with a *defined* StumbleSignal struct) + AC-2
  (signal-type enum). **DEFER AC-3 (mutation proposal generated) + AC-4 (genome evidence_refs) → ITER-0008**
  with STORY-0032 (genome) — both reviewers: untestable here (no genome object + no pattern-detect heuristic).
- **STORY-0018 AC rescoping:** AC-1/2/3 stay (lean-ctx diary + knowledge + ctx_handoff bundle), and AC-3
  **must emit a formal versioned handoff-bundle schema** (doc) so ITER-0006 substrate can pass HandoffIn.
  AC-4 rescoped to the anti-reward-hack proof (handoff loss → oracle grade still authoritative). **Primary seam is
  CI unit/integration (NOT cluster-only — resolves round-2 carry-item risk):** a daemon-loop test with the fake
  backend where the handoff bundle is absent/corrupt asserts `passed()` still grades from Result.ExternalGradingResult.
  SCENARIO-0031 cluster e2e is optional enrichment, not the gating evidence. AC-5 = a design-discipline proof
  (architecture/guard test + code review: the daemon claims work only via the durable `queue.Queue` ledger, never a
  lean-ctx message bus).
- **STORY-0029 AC-4 split:** AC-4a (daemon *reconstructs* resume audit) stays in ITER-0004. **AC-4b (operator/TUI
  visibility of that audit) → ITER-0008.** Implementation path (resolves round-2 "unimplementable" finding): a
  **daemon-local thread store** (keyed thread_id; durable persistence deferred like the lease registry) is written
  on run completion (resume_summary + last_verified_state from STORY-0029 AC-1/AC-2 + last harvested diff from
  Result.PatchData). AC-4a = a `ReconstructResumeAudit(threadID)` method assembling `{branch, workspace, last_diff,
  last_grade, open_questions}` from the thread store + last Result. Unit/integration seam with the fake backend —
  no cluster needed for the reconstruction logic itself.
- **STORY-0033 workspace-lease registry:** a **separate daemon-local map** `map[workspaceKey]workspaceClaim`
  where `workspaceKey = {repo, branch}` and `workspaceClaim = {threadID, leaseToken, expiry}`. It is **independent
  of `queue.Lease`** (which keys by DirectiveID and is NOT modified/extended) — the registry records which thread
  *owns* a (repo, branch) workspace; the queue lease governs directive claim. STORY-0033 AC-1 consults this registry
  before reuse; AC-3 forces continue-or-supersede on an active claim. Durable persistence deferred → ITER-0006/0008.
- **Task 0 (upfront deliverable):** write the **formal versioned handoff-bundle schema** to
  `docs/plans/2026-06-21-handoff-bundle-schema.md` (fields, version, types: workflow_state, session_snapshot_ref,
  curated_knowledge) so ITER-0006 can pass `Directive.HandoffIn`. This is STORY-0018 AC-3's documentation deliverable.
- **Note on "structs don't exist yet":** correct — Thread/Run/StumbleSignal are *defined by this iteration's first
  tasks* (TDD), not a precondition. "Ready to decompose" means the schema shapes are locked in the roadmap below;
  Task 1 writes them in `types.go`.
- **Schema-lock-upfront (boxing-in mitigation):** before impl, define (a) **Thread** struct
  (thread_id, status[reuse ThreadStatus], current_branch, current_workspace, resume_summary{prior_work,next_step},
  last_verified_state, supersedes, superseded_by, **deadline *time.Time** — preemptive for ITER-0007); (b) **Run**
  struct (run_id, thread_id, parent_run_id, stumble_signals[]) **designed additive** with ITER-0008's STORY-0011/0015
  Run fields (worker_id/worker_kind/policy_id/artifact_refs/log_refs) to avoid a colliding Run definition;
  (c) **StumbleSignal** {type, ts, run_id, evidence_summary}; (d) versioned **handoff-bundle** schema.
- **STORY-0058 AC-25** sequenced AFTER STORY-0018 AC-3 (needs the bundle format); emit on requeue (orthogonal to
  Temporal — ITER-0007 only schedules the retry).
- **Artifact debt (RESOLVED 2026-06-21):** added explicit citation of Thread-object def lines (req.md:160-161) to
  STORY-0030 AC-1 sources in EPIC-004.md. Requirement-card sync (round-2 PAR) also applied: STORY-0031 AC-3/AC-4
  deferral note, STORY-0029 AC-4 split note, STORY-0018 AC-3 schema-doc deliverable + AC-4/AC-5 rescope note.
**Status:** done:ITER-0004 (2026-06-21) — all tasks T0–T8 delivered. T0 handoff-bundle schema doc; T1 Thread
struct + daemon-local thread store; T2 Run/StumbleSignal + 9-value enum; T3 workspace-lease registry; T4
ReconstructResumeAudit; T5 SCENARIO-0015 harness (resume-continues / supersede-with-reason); T6 LeanCtxProvider
(default ContextProvider adapter — diary/knowledge/handoff via real lean-ctx, evidence SCENARIO-0030 incl. a
genuine isolated round-trip); T7 NoopProvider anti-reward-hack + daemon work-queue guard (AC-4/AC-5,
SCENARIO-0031 CI); T8 fresh handoff bundle on each autonomous requeue (STORY-0058 AC-25, SCENARIO-0054). Stories
done: STORY-0029/0030/0033/0018; partial done: STORY-0031 (AC-1/2; AC-3/4 → ITER-0008), STORY-0058 (AC-25;
AC-24 → ITER-0007). Suite 165 green under -race, vet clean, JOURNEY-0001 sentinel green, zero TODO(ITER-0004).
**Scope was APPROVED** via 2 PAR rounds (R1 REVISE→revisions; R2 REVISE→artifact-sync; both R2 reviewers
VERIFIED the core design — additive Run, abstract lease, schema-lock, gate cleared). **Decision (2026-06-21):
T6/T8 built local-TDD against the real lean-ctx binary + fake-backend daemon seam** — the cross-one-shot session
round-trip itself was already cluster-proven by the STORY-0034 spike (SCENARIO-0077), so no fresh cluster run was
required for the adapter logic.

**Task decomposition (TDD; interleaved code + evidence; fleet-dogfooded — local only for quick sentinel checks):**
- **T0** (doc): write `docs/plans/2026-06-21-handoff-bundle-schema.md` — versioned handoff-bundle schema (STORY-0018 AC-3 deliverable; unblocks ITER-0006).
- **T1** (code, unit): define `Thread` struct (thread_id, status[reuse ThreadStatus], current_branch, current_workspace, resume_summary{prior_work,next_step}, last_verified_state, supersedes, superseded_by, deadline *time.Time) + daemon-local thread store. STORY-0029 AC-1/AC-2, STORY-0030 AC-1, STORY-0033 AC-2.
- **T2** (code, unit): define `Run` struct (run_id, thread_id, parent_run_id, stumble_signals[]) — additive with ITER-0008 fields — + `StumbleSignal` {type, ts, run_id, evidence_summary} + signal-type enum. STORY-0031 AC-1/AC-2.
- **T3** (code, unit): workspace-lease registry `map[workspaceKey]workspaceClaim` + check-before-reuse + continue-or-supersede. STORY-0033 AC-1/AC-3, STORY-0030 AC-2/AC-3 (reinvention → stumble capture).
- **T4** (code, unit/integration): `ReconstructResumeAudit(threadID)` → {branch, workspace, last_diff, last_grade, open_questions} from thread store + last Result; new run continues current branch by default. STORY-0029 AC-3/AC-4a.
- **T5** (evidence, integration): SCENARIO-0015 harness — directive A (repo,branch) → run → write thread state/handoff; directive B (same repo,branch) → detect thread → import handoff → resume OR explicit supersede. Covers STORY-0029/0030/0033.
- **T6** (code, integration, FLEET): STORY-0018 AC-1/AC-2/AC-3 — **behind a `ContextProvider` interface (DECISION
  2026-06-21: context abstraction, no hard lean-ctx coupling — lean-ctx has a commercial-license upsell, must be
  swappable; mirrors the queue.Queue coordination abstraction).** Build a `LeanCtxProvider` adapter (the default;
  wires ctx_agent diary write/recall + share/receive_knowledge + ctx_handoff create|export|import|pull into the
  runner/daemon, resolving the explicit saved session id per the spike note) + a `NoopProvider` double. The daemon/
  runner depend on the interface, never on lean-ctx directly. Evidence SCENARIO-0030 on a real worker (LeanCtxProvider).
  Schema/interface: docs/plans/2026-06-21-handoff-bundle-schema.md (Provider abstraction section). YAGNI: only the
  lean-ctx + noop adapters now, no speculative backends.
- **T7** (code+evidence, CI unit/integration): STORY-0018 AC-4 — daemon-loop test with the **NoopProvider** (handoff
  effectively lost) → `passed()` still grades from Result.ExternalGradingResult (SCENARIO-0031 CI primary; the noop
  adapter IS the anti-reward-hack lever). AC-5 — guard/architecture test: daemon claims only via queue.Queue, and the
  ContextProvider can never act as the work queue.
- **T8** (code+evidence, integration): STORY-0058 AC-25 — emit a fresh handoff bundle on requeue in the ladder/requeue path; assert at SCENARIO-0054 daemon seam (fake backend, no Temporal). Sequenced after T6.

**Impacted scenarios:** SCENARIO-0015 (resume on branch — explicit harness: directive A→handoff→directive B
detect/resume/supersede; covers 0029/0030/0033), SCENARIO-0030 (ctx_agent diary write/read, integration/cluster),
SCENARIO-0031 (authoritative state independent of handoff loss — **CI unit/integration primary**, cluster e2e
optional), SCENARIO-0016 (RESCOPED: stumble CAPTURE only here; model-escalation → ITER-0007), **SCENARIO-0054
(EXTENDED: STORY-0058 retry scenario now also asserts a fresh handoff bundle accompanies each retry — AC-25;
replaces the mistaken "SCENARIO-0078" ref, which is already taken by deadline-prioritization/STORY-0045)**,
SCENARIO-0077 (spike PASS). (SCENARIO-0018 pattern-learning → ITER-0008 with AC-3/AC-4.)
**Look-ahead check:** gate STORY-0034 (ITER-0000) **CLEARED**; independent of substrate; Run/Thread/bundle schemas
locked additive so ITER-0006 (substrate)/0007 (Temporal deadline)/0008 (Run augmentation + genome) graft without rework.

### ITER-0005 — Backend-abstraction & isolation-tier interface slice (CI-provable)

**Status:** done (2026-06-21) — interface seam locked + isolation-tier selection landed, all CI-provable.
T1 `IsolationTier`+`TemplateRule.Tier`+`Policy.TierFor` (STORY-0023 AC-1); T2 `BackendFactory`+
`staticBackendFactory.SelectRunner` (STORY-0004/0017 AC-1); T3 additive daemon wiring (tier→backend
+ D6 tier line + park-on-unavailable-tier); T4–T6 evidence (SCENARIO-0089/0028/0076 automated).
Stories done: STORY-0023 (full); STORY-0004/0017/0020 (in-scope interface ACs; microVM ACs →
ITER-0005b). Suite 177 green under `-race`, vet clean, JOURNEY-0001 + JOURNEY-0003 AC-1 sentinels
green, zero `TODO(ITER-0005)` debt (2 intentional `TODO(ITER-0005b)` graft markers in `backend.go`).
**Stories:** STORY-0004, STORY-0017, STORY-0020, STORY-0023
**SCOPE DECISION (2026-06-21, user):** ITER-0005 was split. This iteration is the **interface
slice** — the CI-provable / fleet-dogfoodable backend-abstraction + tier-selection work that
needs no live Firecracker/nspawn. The heavy NixOS-golden / Firecracker-microVM / nspawn-fast-tier /
agent-skills-nix infra stack (STORY-0005, 0007, 0008, 0021, 0022, 0024, 0075-full, 0076, 0077, 0078)
moved to **ITER-0005b** (cluster-resident, runs on `agent-host`). Rationale: prior iterations were
Mac-driven Go coordination code; this iteration keeps that momentum on the verifiable interface seam,
and the benchmark spike established that the nspawn fast tier can't run until a real Firecracker
microVM guest is stood up first (an ITER-0005b precondition).
**Rationale:** Lock the backend-agnostic execution seam so ITER-0005b's microVM/nspawn backends graft
in without rework. The `Runner` interface (`types.go:130`) + the container backend
(`cli_runner.go`/`client_runner.go`) + the `container_runner_test.go` contract already exist; this
iteration makes the abstraction *explicit and contract-tested* (STORY-0004 AC-1/AC-2, STORY-0017
AC-1/AC-2, STORY-0020 AC-1), and adds the genuinely-new **isolation-tier selection** (STORY-0023
AC-1: a `Fast|Hard` tier declared on the directive/template and validated through D1) plus a
tier→backend selection seam (factory) with the container backend as the only registered backend and a
documented `TODO(ITER-0005b)` graft point for microVM/nspawn.
**AC scoping (deferred ACs → ITER-0005b):** STORY-0004 AC-3 (microVM backend, benchmark-gated);
STORY-0017 AC-3 (microVM startup ≤5s measured) + AC-4 (microVM clean teardown); STORY-0020 AC-2
(microVM backend passes same contract). These are cluster/Firecracker-only and have no CI seam here.
**Design note (PAR-required, 2026-06-21):** `docs/plans/2026-06-21-iter0005-backend-tier-design.md` —
locks the two decisions the scope review required: (1) IsolationTier lives on `TemplateRule` (D1
mechanism), NOT on the Directive — so ITER-0006's strict `ParseDirective` is untouched and a
worker-origin directive cannot downgrade isolation; (2) tier→backend selection is a
`BackendFactory.SelectRunner(tier)` OUTSIDE `Runner.Run` (interface unchanged; ITER-0005b runners
register against it). Also documents tier ⊥ ITER-0008 `worker_kind` orthogonality.
**New work (vs already-coded interface):** IsolationTier type + `TemplateRule.Tier` + `Policy.TierFor`;
`BackendFactory`/`staticBackendFactory` + additive daemon wiring + decision-log tier line; explicit
contract tests (SCENARIO-0028/0076/0089) rather than implicit reliance on the existing `Runner`.
**Impacted scenarios:** SCENARIO-0028 (D2 backend interface abstracts container vs microVM — unit),
SCENARIO-0076 (container backend passes contract tests — integration; CI subset of the existing
`container_runner_test.go`), SCENARIO-0089 (NEW: template-declared tier selects the backend — integration).
(SCENARIO-0029 microVM ≤5s, SCENARIO-0005/0006/0007 tier-execution → ITER-0005b.)
**Look-ahead check:** STORY-0025 gate (ITER-0000) CLEARED; the factory seam is the graft point for
ITER-0005b's microVM/nspawn backends and ITER-0008's worker-kind dispatch (STORY-0011).
**Scope review:** 2 PAR reviewers (2026-06-21) → both REVISE→APPROVE-after-revisions. All
enumerated approval conditions resolved: tier-location + factory architecture documented in the
design note (CRITICAL ×2); worker_kind orthogonality documented (CRITICAL); SCENARIO-0089 added
(SERIOUS); new-work made explicit (SERIOUS). One B finding ("container_runner_test.go missing") was a
false positive — the file exists (326 lines; a nested-go.mod search miss).

### ITER-0005b — Firecracker micro-VM substrate & isolation tiers (cluster, post-spike)

**Stories:** STORY-0007, STORY-0008, STORY-0021, STORY-0022, STORY-0024, STORY-0005
**SCOPE DECISION (2026-06-21, PAR round 1 — both reviewers):** the original ITER-0005b (10 stories)
was split into two decoupled tracks with no cross-dependency. This is the **substrate track**:
durable Firecracker micro-VM, disposable units, the `nspawn --ephemeral` fast tier (real-kernel guest),
per-task Firecracker hard tier, the trust-boundary VM, and immutable golden copies. The **image track**
(FULL golden / provider routing / skills) moved to **ITER-0005c**. Rationale: orthogonal failure modes
(Firecracker boot vs skills-flake resolution) — a stall in one must not block the other.
**Cluster-resident — runs on `agent-host`; no CI seam on the Mac (no local Nix).** Grafts the
microVM/nspawn backends onto ITER-0005's `BackendFactory.SelectRunner(tier)` seam (`backend.go`).
**Task 0 (PAR-required, BLOCKING — both reviewers, CRITICAL):** define the **cluster verification
harness** BEFORE any substrate code. For a cluster-only iteration with no Mac CI, every AC is e2e and
currently every scenario is TBD — so the harness is the gate, not an afterthought. Deliverables:
(a) a boot-readiness sentinel definition per scenario (what "ready" means — e.g. `systemctl
is-system-running`, an llm-proxy reachability curl, an in-guest `nix develop` invocation); (b) a
measurement script producing `{mean,p50,p99,stddev}` for spin-up (extends `fleet-worker/spikes/
bench-spinup.sh`); (c) explicit ACCEPTANCE GATES (durable-VM boot-to-ready, sub-second nspawn fast-tier
in-guest, per-task hard-tier ~1.8s amortized per the STORY-0025 benchmark, clean teardown without
`incus delete`); (d) wire SCENARIO-0003/0004/0005/0006/0007/0029 corpus commands to the harness.
**Sequencing (PAR — both reviewers):** STORY-0007 (durable Firecracker micro-VM) is a HARD
PRECONDITION — build it FIRST; STORY-0008 (disposable units), STORY-0021 (in-guest nspawn fast tier),
STORY-0022 (per-task Firecracker hard tier) follow; STORY-0024 last. STORY-0005 (golden copies, image
definition + `incus copy`) can proceed in parallel (independent of the VM).
**STORY-0024 RESCOPE (PAR — both reviewers):** IN: a single durable VM as a hardware trust boundary
with disposable units inside (AC-1 + the single-domain reading of AC-2). DEFER: dynamic multi-domain
VM provisioning + cross-domain routing/operationalization (the full multi-tenancy of AC-2/AC-3) →
ITER-0006+ (needs the substrate + a domain-routing owner). Add a topology note: multi-tenancy "falls
out" structurally but is not operationalized here.
**Split-in (from ITER-0002 PAR):** STORY-0049 AC-5 (immutable root + writable /workspace,/tmp scratch)
lands here as part of the disposable-unit/golden substrate; plus SCENARIO-0020's microVM host
credential-socket isolation (the broker proof itself shipped in ITER-0002 at the container/proxy seam).
**Also lands the deferred microVM ACs from ITER-0005:** STORY-0004 AC-3, STORY-0017 AC-3/AC-4,
STORY-0020 AC-2 (and SCENARIO-0029 microVM ≤5s — owned by STORY-0017 AC-3, measured by Task 0's harness).
**In-guest benchmark (Task 0 priority):** measure `nspawn --ephemeral` spin-up INSIDE an actual
Firecracker micro-VM guest with a btrfs template — the faithful in-guest fast-tier number (the 76 ms
figure was a privileged-Incus-container proxy; a real-kernel VM was confirmed to run nspawn natively
without privilege — `fleet-worker/spikes/STORY-0025-vm-vs-lxc-comparison.md`).
**Impacted scenarios:** SCENARIO-0004 (durable microVM), SCENARIO-0005 (fast tier), SCENARIO-0006
(hard tier), SCENARIO-0007 (trust-domain VM, single-domain v1), SCENARIO-0029 (microVM ≤5s),
SCENARIO-0003 (golden launch / STORY-0005); SCENARIO-0008/0009 (benchmark, done).
**Boxing-in (PAR — both PASS):** grafts onto ITER-0005's factory; STORY-0007's "hosts coordinator +
queue client" is resource topology (where the one-shot loop runs), NOT scheduling semantics — it does
NOT commit ITER-0006's queue substrate (still Patrick-blocked) or ITER-0007's Temporal; tier ⊥
worker_kind (ITER-0008).
**Status:** done:ITER-0005b (cluster, 2026-06-22) — all 6 stories landed + MEASURED on agent-host.
STORY-0007 (durable coord VM, SCENARIO-0004); STORY-0021 (fast-tier nspawn --ephemeral 64ms mean +
PID-ns isolation, SCENARIO-0005) + NspawnRunner@TierFast; STORY-0022 (per-task Firecracker 737ms mean /
909ms p99, SCENARIO-0006) + FirecrackerRunner@TierHard; STORY-0008 (disposable units + unit-kill
teardown 111ms incus-free, SCENARIO-0004/0008ac2); STORY-0024 (trust boundary: guest kernel 6.12.78 ≠
host 6.8.0, single-domain v1, SCENARIO-0007); STORY-0005 (immutable golden + incus-copy launch 2.9s
CoW, no live build, SCENARIO-0003). Both TODO(ITER-0005b) graft markers resolved; the serve entrypoint
wires the real tier factory. go vet + go test -race ./... green incl. live e2e. ITER-0006 stays
Patrick-blocked; ITER-0005c (FULL golden / skills / provider routing) eligible next.
Substrate decision (evidence-backed): two-tier — `nspawn --ephemeral` inside the durable Firecracker
guest for trusted lanes (76ms spike → 64ms in-guest), per-task Firecracker for sensitive lanes; nspawn
can NOT run in the unprivileged agent-host LXC even with `security.nesting` (proc-mount/idmap, codified
in `host/configuration.nix`), so the fast tier lives in the VM guest.
**Scope review:** 2 PAR reviewers (2026-06-21) → both REVISE/conditional-APPROVE. Conditions applied:
(1) split substrate/image → ITER-0005b + ITER-0005c (CRITICAL, both); (2) Task 0 cluster verification
harness made the BLOCKING first deliverable (CRITICAL, both); (3) STORY-0024 rescoped to single-domain
v1 (SERIOUS, both); (4) STORY-0022 AC-2 wording aligned to the ~1.8s benchmark; SCENARIO-0029 ownership
pinned to STORY-0017 AC-3. A's "STORY-0021/0022/0024 missing from EPIC-001" was a false positive — they
are in EPIC-002 (citation check passes; all 78 cited stories exist).
**Look-ahead check:** STORY-0025 gate CLEARED; grafts onto ITER-0005's factory seam; ITER-0005c (image)
runs in parallel.

### ITER-0005c — NixOS golden image, provider routing & curated skills (cluster, parallel to ITER-0005b)

**Stories:** STORY-0078 (skills-layout discovery — GATES 0077), STORY-0077 (vendor skills via
agent-skills-nix), STORY-0075 (FULL golden), STORY-0076 (provider routing via llm-agents.nix)
**SCOPE DECISION (2026-06-21, PAR round 1):** the **image track**, split out of ITER-0005b. Independent
of the substrate track — the golden image + skills + provider routing run identically on a container or
a microVM, so this can proceed in parallel with (and does not block) ITER-0005b.
**Sequencing (PAR — A):** STORY-0078 (confirm upstream agent-skills-nix subdir/idPrefix layout +
`filter.maxDepth`) is **pre-work discovery that GATES STORY-0077** (the bundle config) — run it FIRST;
its AC-5/AC-6 are a validated layout doc (proof: the resolved bundle exists with the expected paths,
folded into SCENARIO-0068/0069). Then STORY-0075 (FULL golden: snapshot + `incus copy` AC-1,
clean-room byte-identical-regen integrity gate AC-2, bridge-ON graded run AC-3) → STORY-0076 (provider
routing, needs llm-agents.nix in the golden). STORY-0077 (skills bundle baked into the golden) composes
with STORY-0075.
**Cluster-resident — runs on `agent-host`; no CI seam on the Mac.** Reuses ITER-0005b's Task 0 cluster
verification harness for golden launch + graded-run proofs.
**Impacted scenarios:** SCENARIO-0065/0066 (golden built once + clean-room integrity), SCENARIO-0067
(provider routing), SCENARIO-0068/0069 (skills bundle + discovery path).
**Status:** done:ITER-0005c (cluster, 2026-06-22) — 3/4 stories fully done + STORY-0075 AC-1; AC-2/AC-3
carried per the PAR carry-allowance. **Delivered, all cluster-verified on agent-host:** T0 harness
(5 scenarios wired); T1 STORY-0078 (agent-skills-nix + agent-skills flake=false hash-pinned, curated
bundle builds — SCENARIO-0069 PASS, layout doc); T2 STORY-0077 (13-skill copy-tree at
/etc/claude/skills, 0 symlinks — SCENARIO-0068 PASS); T3 STORY-0075 AC-1 (FULL `fleet-golden` built
once via build-golden.sh, realized toolchain + skills, copy-per-task zero rebuild — SCENARIO-0065
PASS); T4 STORY-0076 (golden exports codex/gemini/qwen — SCENARIO-0067 PASS — + dispatcher
`--provider`/`--model` passthrough now plumbed, grader-determinism — TestScenario0067 CI). **T5
STORY-0075 AC-2/AC-3 CARRIED:** the clean-room regen was run on the golden's nix-pinned go1.26.4
(cleanroom-attempt.sh): `make generate` succeeds but the regenerated let-go native-Go lowered TEST
package does not compile — an UPSTREAM let-go codegen bug, reproduced on the pinned toolchain (refutes
ITER-0003's Mac-artifact hypothesis; same blocker as STORY-0068 AC-2 / JOURNEY-0003). Golden + grader
are correct. Suite green (`go test -race ./...`, vet clean); JOURNEY-0001/0003 AC-1 sentinels green;
zero `TODO(ITER-0005c)`. **AUDIT (PAR 2-auditor 3-tier, 2026-06-22): CLEAN** — both auditors returned
zero critical/serious/minor; Tier 1 all ACs PASS (AC-2/AC-3 carry authorized + durably evidenced),
Tier 2 impacted scenarios PASS, Tier 3 sentinels green. Iteration confirmed done.
**PAR scope review (2026-06-22) — 2 adversarial reviewers → both REVISE; revisions applied (below),
re-review to APPROVE.** Aggregated findings + resolutions (same-issue-from-both = high confidence;
severity disagreement = worst):
- **(CRITICAL, B; = A-S3) STORY-0078 proof undefined / timing paradox** → RESOLVED: STORY-0078's
  standalone gate is the **bundle BUILD** `nix build .#agent-skills-bundle` (needs only the small
  bundle derivation, not the golden) → runs before STORY-0077's golden integration, no paradox.
  Discovery already executed on the cluster (upstream non-flake, flat `skills/<name>/SKILL.md`,
  subdir=skills/idPrefix=null/maxDepth=1, 13 skills present). Captured in EPIC-013 STORY-0078 +
  the layout-validation doc deliverable `docs/plans/2026-06-22-skills-layout-validation.md`.
- **(CRITICAL, B) no ITER-0005c harness wiring** → RESOLVED: **Task 0** wires SCENARIO-0065/0066/0067/
  0068/0069 into `fleet-worker/cluster-tests/run.sh` with the same PENDING(2)/PASS(0)/SKIP(0)/FAIL(1)
  discipline + gates, BEFORE story work.
- **(SERIOUS, B "split" vs A "not split-worthy") STORY-0075 heterogeneous ACs** → RESOLVED by
  task-level ordering + carry-allowance (no renumber): AC-1 (build/snapshot/copy, integration,
  must-pass) → AC-2 (clean-room byte-identical regen, e2e, toolchain-sensitive GATE) → AC-3 (bridge-ON
  graded run, e2e). **Carry-allowance** (precedent ITER-0003 STORY-0068 AC-2): AC-1 must pass; AC-2/AC-3
  may carry with an explicit let-go-toolchain reason if they hit the wall, without blocking 0076/0077/0078.
- **(SERIOUS, A-S2 + B) STORY-0075 AC-3 bridge proof unspecified** → RESOLVED: SCENARIO-0066 action
  extended to require the graded run execute with the lean-ctx bridge ON.
- **(SERIOUS, B) STORY-0076 routing opaque** → RESOLVED: no new story — AC-1 = golden EXPORTS the CLIs
  (nix: uncomment `fleet-worker/flake.nix:55`) + dispatcher routing ALREADY exists (flags + proxy.go);
  contract test asserts flag passthrough + grader determinism (SCENARIO-0067 verification clarified).
- **(SERIOUS, B; A-minor) SCENARIO-0065/0066 "nix on host" contradicts cluster-only** → RESOLVED:
  preconditions rewritten to cluster-only (nix inside agent-host/nix-server, not the Mac).
- **(SERIOUS, A) EPIC-013 header wrong iteration** (ITER-0005b→0005c) → RESOLVED.
- **(minor, B) STORY-0078 seam** → clarified as nix-eval/build proof.
**PAR re-review (2026-06-22, round 2) — both reviewers confirm 8/8 round-1 findings RESOLVED + core
scope sound; raised 5 clarification asks (all doc, no scope/architecture change), codified below →
APPROVE:**
- **(A-ISSUE-1) T0 timing/PENDING semantics** → T0 runs FIRST and is executable IMMEDIATELY (the
  harness scaffolding needs no substrate). Its scenarios report PENDING(2) until each ITER-0005c
  STORY's *own* evidence lands (the bundle build / golden / provider export) — PENDING tracks
  0065-0069 story evidence, NOT the ITER-0005b substrate (which is done). Each flips PENDING→PASS as
  T1–T5 complete.
- **(A-ISSUE-2) AC-2/AC-3 carry trigger** → STORY-0075 marks AC-2/AC-3 PARTIAL+carry ONLY if, with
  AC-1 (golden launch) otherwise green and reproducible, the cluster run hits one of: (a) `make
  generate` produces non-compiling / non-byte-identical artifacts due to a host-vs-golden toolchain
  version skew, OR (b) the graded run / regen exceeds a 15-min wall per attempt across 2 attempts.
  Anything else (a real golden defect) is a FAIL, not a carry.
- **(A-ISSUE-3) bundle-build closure semantics** → `packages.agent-skills-bundle` = `mkBundle`'s
  copy-tree of ONLY the 13 curated SKILL.md dirs (a `runCommand`/rsync derivation); its closure is the
  13 skill source trees + coreutils/rsync, NOT nixpkgs toolchain, claude-code, or the golden system.
  Inputs `agent-skills`/`agent-skills-nix` are source fetches (already cached on nix-server). Expect a
  sub-minute build — genuinely a small standalone gate, independent of the golden.
- **(B-new-1) T5 repository target** → pinned: T5 uses the **let-go repository + the captured
  `testdata/journey0003/lvl1-focused.diff` fixture from ITER-0003 STORY-0068** (JOURNEY-0003), the same
  toolchain-sensitive case — not a new/simpler project.
- **(B-new-2) "no Ubuntu fallback"** → it is a *sufficiency assertion* (the NixOS golden alone runs the
  graded task end-to-end), NOT a separate Ubuntu-retire story. There is no Ubuntu fallback in scope; the
  Ubuntu stopgap was already abandoned at ITER-0000's real dogfood.
**Task decomposition (TDD where code; evidence interleaved; cluster-resident on agent-host):**
- **T0** (harness, executable now): wire SCENARIO-0065/0066/0067/0068/0069 into `cluster-tests/run.sh`
  (each PENDING until ITS OWN STORY evidence lands — per A-ISSUE-1) + add gates. The verification GATE
  for this cluster-only iteration.
- **T1** (STORY-0078, discovery): write `docs/plans/2026-06-22-skills-layout-validation.md`; add the
  `agent-skills` (flake=false) + `agent-skills-nix` inputs to `fleet-worker/flake.nix`; expose
  `packages.agent-skills-bundle` via `selectSkills`+`mkBundle` (13 ids, subdir=skills, maxDepth=1;
  small copy-tree derivation per A-ISSUE-3); prove by `nix build .#agent-skills-bundle` listing all 13
  (SCENARIO-0069). GATES T2.
- **T2** (STORY-0077): place the bundle at `environment.etc."claude/skills".source` via copy-tree (not
  symlink) in the worker/golden config; prove the 13 SKILL.md resolve at the discovery path in a golden
  copy, regular files not symlinks (SCENARIO-0068).
- **T3** (STORY-0075 AC-1): build golden once (`nix develop` realized: claude-code+lean-ctx+go+make+skills),
  snapshot as golden image, `incus copy` per task = zero rebuild (SCENARIO-0065, must-pass).
- **T4** (STORY-0076): uncomment codex/gemini-cli/qwen-code export in flake.nix; export-presence check
  in a golden copy + dispatcher `--provider`/`--model` passthrough contract test + grader-determinism
  (SCENARIO-0067).
- **T5** (STORY-0075 AC-2/AC-3, e2e, CARRY-ALLOWED): clean-room byte-identical regen gate + bridge-ON
  headless graded run on the **let-go repository + the ITER-0003 `testdata/journey0003/lvl1-focused.diff`
  fixture** (SCENARIO-0066). Carry-trigger per A-ISSUE-2 above; "no Ubuntu fallback" = sufficiency
  assertion (B-new-2). If toolchain wall → mark PARTIAL + carry with reason.
**Impacted scenarios:** SCENARIO-0065/0066 (golden built once + clean-room integrity + bridge-ON graded),
SCENARIO-0067 (provider routing), SCENARIO-0068/0069 (skills bundle + discovery path).
**Look-ahead check:** independent of the substrate track; reuses ITER-0005b Task 0 harness pattern;
STORY-0049 AC-5 immutable-root scratch is shared substrate (tracked in ITER-0005b). No boxing-in of
ITER-0006/0007/0008 (both reviewers PASS check 3 — skills+provider are image-layer config, additive
flake inputs, orthogonal to queue/Temporal/worker_kind).

### ITER-0006 — Queue substrate: laneq gRPC binding + Go adapter + directive contract (CI-provable)

**Stories:** STORY-0010 (partial), STORY-0044 (partial), STORY-0002 (partial), STORY-0064 (partial)
**Rationale:** Replace the stub `MemoryQueue` with the chosen substrate — **laneq**
(`selamy-labs/laneq`, Python: SQLite core + CLI + MCP server) — wired to the Go
dispatcher through a **gRPC binding**. **SUBSTRATE CONFIRMED (2026-06-22): Patrick
open to extending laneq + added Norman as contributor; the substrate is laneq.**

**Discovery (2026-06-22):** laneq already ships upstream (v0.4.0 + #18) `not_before` +
`blocked_by` TIME-plane deferral, lease-based `take`/`touch`/`reap`, `peek`, `defer`,
`set_status`, threading. States pending/taken/deferred/done/dropped. A laneq directive is an
**opaque `body` string** + scheduling columns (priority P0/P1/P2, lane, parent, not_before,
blocked_by, taken_by, lease_until, requeue_count). So STORY-0044's not-before is largely DONE
upstream; ITER-0006 *validates + integrates* it.

**Architecture (PAR-revised 2026-06-22, both reviewers REVISE→addressed):**
- **gRPC binding:** a `laneq.proto` + a Python gRPC server over `core.py` ops, developed on a
  **laneq branch we control, pinned by commit hash** (upstream PR best-effort/non-blocking).
  pytest-covered in the laneq repo. Adds a clean **`parked` status** to laneq core (Park must be a
  durable, non-auto-promoting hold — do NOT overload `deferred`, which auto-promotes).
- **Storage schema (resolves A-critical "schema undefined"):** scheduling fields are **laneq
  columns** (Importance↔priority, NotBefore↔not_before via `Defer`, Lane↔lane,
  Attempts↔requeue_count) = single source for scheduling; semantic fields
  (intent/template/origin/repo/ref/task/grade/handoff_in/deadline) live in laneq's opaque **`body`
  JSON**, parsed by the Go `ParseDirective`. No field duplication.
- **Lease mapping (resolves B-critical "token lost on restart"):** `Lease.Token ↔ (id, consumer)`
  — both are durable SQLite columns, recoverable after daemon restart; no synthesized in-memory
  token, no upstream token field needed.
- **Go gRPC client `LaneqQueue`** implements the existing `queue.Queue` interface (drop-in for
  `MemoryQueue`), wired in `serve_cmd.go` behind a `--queue=laneq|memory` flag (default `memory`
  until the ITER-0006b cluster deploy).
- **Temporal-sole-writer seam (resolves boxing-in):** not_before/priority are written ONLY via gRPC
  `Defer`/`Reprioritize`; in ITER-0007 Temporal becomes the sole caller of those. The daemon claim
  path only READS. Documented at the seam so ITER-0007 grafts on without rework.

**laneq fork handling (PAR re-review 2026-06-22, A+B serious — resolved):** the gRPC server + `parked`
status are developed on a **fork at `nnunley/laneq`** (DECIDED 2026-06-22; forked from
`selamy-labs/laneq` @ f9c159a; Norman is also a contributor upstream), **pinned by commit hash** in
`fleet-worker/flake.lock`-style fashion (same hash-pin discipline as agent-skills in ITER-0005c). The
ITER-0006b Nix package builds laneq from that pinned hash. An upstream PR to selamy-labs is
**best-effort / non-blocking**; until merged we build from the pinned branch (acceptable maintenance
cost, single small feature). This removes any external-merge block on ITER-0006/0007.

**Task decomposition:**
- **T0** `laneq.proto` contract (Push/Take/Peek/Defer/SetStatus/Touch/Reap/Stats/Show/List/Reprioritize/ThreadStatus + Park/Unpark). **PAR-gate T0 in a mini-review** so the proto doesn't box in ITER-0007 (blocking-dependency / lane / thread semantics must be expressible).
- **T1** laneq-side gRPC server (Python, our branch) over core.py + add `parked` status (**careful SQL: `parked` MUST be excluded from `take`/`peek`/`reap`/`reclaim_deferred` and MUST NOT auto-promote — B-serious T1 impl-review item**); pytest. Pin by hash.
- **T2** Go generated stubs + `LaneqQueue` adapter (full contract mapping above).
- **T3 (evidence)** SCENARIO-0091 (integration, **CI-native**): Go adapter drives a faithful
  in-process **fake** laneq gRPC server through the full lifecycle — claim/lease/requeue/defer/reap/park
  PLUS **lane-FIFO + lane isolation** and **`blocked_by` dependency-chain + thread_status** (B-critical
  coverage fix) → proves STORY-0002 AC-1 (priority/**lanes/threading**/leasing), STORY-0044 AC-1/AC-2,
  STORY-0010 AC-4.
- **T4** directive contract: activate `ParseDirective` at the laneq `body` JSON boundary +
  SCENARIO-0045 unit tests for STORY-0064 AC-1..AC-14 **field-presence/schema** (incl. access_cmd/root
  rejection). AC-2's *template-vs-allowlist+origin validation* half is ALREADY proven by the ITER-0002
  D1 `ValidateTemplate` (policy.go:35, `policy_test.go`/`scenario_d1_test.go`) — cited, not re-proven.
- **T5** wire `serve_cmd.go` `--queue=laneq|memory` flag + document the Temporal-sole-writer seam.
- **T6 (evidence, this iteration — A-critical de-boxing)** SCENARIO-0092 (e2e, **real Python laneq**
  via `uvx --from git+https://github.com/<our-fork>/laneq@<hash>`): run the SCENARIO-0091 lifecycle
  against the REAL laneq gRPC server (runnable on the dev Mac / cluster — Python present; NOT in Go CI).
  This proves the gRPC binding is wire-compatible **before** ITER-0007 builds Temporal on the seam, so
  the seam is not merely fake-proven. (The CI sentinel stays SCENARIO-0091; SCENARIO-0092 is a
  this-iteration real-wire proof, re-run productionized in ITER-0006b after Nix packaging.)

**Story outcomes this iteration (PARTIAL where dependency-gated):**
- STORY-0002: AC-1 done (durable cluster-resident queue: priority/**lanes/threading**/leasing — all exercised in SCENARIO-0091 + real-wire SCENARIO-0092); **AC-2 → ITER-0007** (deferred work lives in Temporal until eligible).
- STORY-0044: AC-1/AC-2 done (not_before field + `next` filters eligible-then-importance); **AC-3 → ITER-0007** (Temporal sole writer).
- STORY-0064: AC-1..AC-14 done (contract schema via SCENARIO-0045 unit; AC-2 validation half cites D1 `ValidateTemplate`); **AC-15/AC-16 → ITER-0007** (importance/deadline as Temporal inputs; agents-propose-vs-humans-set authority — cross-surface, need Temporal).
- STORY-0010: AC-4 done (not-before eligibility gate); decision RESOLVED (laneq); AC-2/AC-3/AC-5 = **not-chosen decision outcomes** (closed by decision, not unmet); **AC-1 (Mac-off cluster e2e) → ITER-0006b**.

**Status:** done:ITER-0006 (2026-06-22) — laneq gRPC binding integrated end-to-end. Delivered all 7
tasks (T0 proto contract; T1 Python gRPC server + `parked` status + requeue_count on the fork
`nnunley/laneq@2d1b59e`, PR #19 CI green; T2 Go `LaneqQueue` adapter; T3 in-process fake + SCENARIO-0091
CI gate; T4 SCENARIO-0045 directive contract AC-1..14; T5 `--queue=memory|laneq` selector + Close seam;
T6 real-wire SCENARIO-0092 via uvx). PAR caught + fixed 7 real wire-fidelity bugs pre-merge (Touch
seconds, Lane overlay, Peek reclaim/promote fidelity, hollow park test, fork timestamp-UTC, fork
hardcoded-priority/missing-fields, fork gRPC error-codes) + an honest test-weakening (artificial
ErrLeaseLost) reverted. Story outcomes: STORY-0002 AC-1 / STORY-0044 AC-1,AC-2 / STORY-0064 AC-1..14 /
STORY-0010 AC-4 done; deferred AC-2/AC-3/AC-15/AC-16 → ITER-0007, STORY-0010 AC-1 → ITER-0006b,
AC-2/3/5 not-chosen. Default suite green (`go test -race ./...`), 0091 CI sentinel green, 0092 gated
real-wire PASS, JOURNEY-0001/0003 AC-1 sentinels green, zero `TODO(ITER-0006)`. **Divergence logged
for ITER-0008:** real laneq leases are NOT consumer-exclusive (no per-consumer token enforcement).
**AUDIT (PAR 2-auditor 3-tier, 2026-06-22):** auditor B found a GAP (SCENARIO-0092 `TouchAndReap` was
timing-flaky against the real server — intermittent FAIL despite the "PASS" claim; auditor A had not
actually run the gated real-wire test). Verified directly (reproduced the flake), then FIXED: split
touch-renew from the reap-increment proof and widened reap margins to a 1s lease + 2.5s wait → 4/4
deterministic real-wire PASS; default `-race` suite green (0092 still gated). Tier 1 ACs sound, Tier
2/3 sentinels green. Iteration confirmed done post-fix.
**Impacted scenarios:** SCENARIO-0091 (NEW, CI integration — gRPC adapter lifecycle incl. lanes/threading); SCENARIO-0092 (NEW, real-wire e2e via uvx @2d1b59e); SCENARIO-0045 (directive contract, unit, 22 AC-mapped); SCENARIO-0012 (Mac-off → ITER-0006b)
**Look-ahead check:** substrate confirmed; the gRPC `Defer`/`Reprioritize` seam is built AND real-wire-proven (SCENARIO-0092) so ITER-0007 Temporal becomes the sole writer without rework. Unblocks ITER-0006b + ITER-0007.

### ITER-0006b — laneq Nix package + cluster deploy + Mac-off acceptance (cluster)

**Stories:** STORY-0010 (closeout AC-1)
**Rationale:** Productionize laneq as a cluster-resident service: package the pinned-hash fork as a
**Nix package**, deploy the gRPC service on `ndn-desktop` (SQLite DB on an Incus host volume), and
prove the Mac-off property. The Go↔real-laneq wire was proven in ITER-0006 via `uvx` on the dev Mac;
this iteration proves it over a real network port from a Nix-packaged cluster service (mirrors the
ITER-0005/0005b/0005c CI-vs-cluster split).

**MUST-PASS core (PAR-revised 2026-06-22, A+B "carry-abuse risk" resolved — the iteration must ship
real STORY-0010 substance even if the full Mac-off carries):**
- **T0 — Nix package** for laneq (`nnunley/laneq@2d1b59e`) on `nix-server` via `buildPythonApplication`
  (fetchFromGitHub pinned), exposing `laneq-grpc`. **Version skew is a solvable packaging detail, NOT a
  carry risk (nixos MCP-confirmed nixpkgs 25.11: protobuf 6.33.1, grpcio 1.76.0, grpcio-tools 1.76.0):**
  the robust recipe REGENERATES the proto stubs in-build with grpcio-tools 1.76 from the fork's
  `proto/laneq.proto` (nativeBuildInputs), so the generated `*_pb2*.py` always match the runtime grpcio
  — ignoring the fork's committed 1.81 stubs. (Nix definitions are flexible: if regen is awkward, an
  `overridePythonAttrs`/custom derivation pinning grpcio 1.81 is the fallback; protobuf 6.33.1↔6.33.6 is
  already compatible.) **Pre-flight gate (must pass FIRST):** `nix build` succeeds AND the built
  interpreter can `import laneq.grpc.laneq_pb2_grpc` AND a local smoke RPC round-trips. This retires the
  PAR B-serious "protobuf/grpcio mismatch" risk.
- **T1 — Deploy** the laneq gRPC service on `ndn-desktop`: a systemd unit (on a container) with the
  SQLite DB on an Incus host volume; a **documented deploy contract** (port, DB path, readiness probe,
  log location). **Temporal-sole-writer note (PAR):** the deploy doc MUST state Temporal (ITER-0007)
  writes not_before/priority ONLY via gRPC `Defer`/`Reprioritize` — never direct SQLite — so the
  single-service/host-volume DB is not boxed in.
- **T2 — SCENARIO-0092 over the wire:** re-run the real-wire lifecycle against the deployed Nix service
  over a real network port (Go adapter from a cluster container → the systemd laneq). This is NEW
  evidence over the uvx-on-Mac 0092 (proves packaging + systemd lifecycle + host-volume persistence +
  network wire), not redundant.

**CARRY-ELIGIBLE (PAR-revised — honest, no silent carry):**
- **T3 — SCENARIO-0012 Mac-off acceptance:** the NARROW substrate proof — a **cluster-side** drain
  (claim/process loop run ON the cluster via `incus exec agent-host`, NOT orchestrated from the Mac)
  claims+drains directives enqueued into the deployed laneq while the Mac's client is disconnected; the
  host-volume DB persists; Mac reconnect shows no loss. Driven + observed entirely cluster-side with a
  durable captured log (precedent: ITER-0005c cleanroom evidence). **Carry-eligible** ONLY if it hits
  a real wall (e.g., it requires baking the full Go dispatcher into a cluster coordinator service that
  does not yet exist) — in which case carry with an explicit documented reason and defer the FULL
  sustained-Mac-off + escalation to **ITER-0008 (STORY-0074, the full Mac-off acceptance)**. Must NOT
  silently carry: deliver either a genuine narrow cluster-autonomous Mac-off proof OR a documented wall.

**Status:** done:ITER-0006b (cluster, 2026-06-23) — STORY-0010 closed. Delivered: T0 laneq Nix package
(`fleet-worker/laneq.nix` buildPythonPackage on the `nnunley/laneq@2d1b59e` fork, in-build proto stub
regen with grpcio-tools 1.76, checkPhase runs the fork's 72 grpc.aio tests — real-RPC proof, NOT a
serialize-tautology); T1 deploy (systemd `laneq-grpc` on ndn-desktop:nix-server:9999, SQLite on Incus
host volume `/srv/laneq`, deploy doc with the Temporal-sole-writer note; serves real gRPC + data
survives restart) + a Nix-wired `laneq-client` env (python3.withPackages — NO hardcoded store paths);
T2 SCENARIO-0092 over the wire (Go adapter ↔ deployed service via an incus proxy, 5/5 deterministic
after fixing a ParkDurability lease-expiry flake); T3 SCENARIO-0012 Mac-off **PASS-NARROW** (cluster
consumer drains autonomously via a `systemd-run` detached unit, Mac uninvolved, DB persists). Scenario
tests renamed to semantic names (directive_contract/laneq_fake_lifecycle/laneq_realwire_lifecycle, keep
`// Proves SCENARIO-NNNN`). Default `go test -race ./...` green (283; gated 0092 skipped); zero
`TODO(ITER-0006b)`. **Real-laneq divergences logged:** reap() return-count differs from the fake
(effect hard-asserted via Attempts==1); leases NOT consumer-exclusive (no token enforcement). PAR +
direct verification caught + corrected 3 T0 weakenings, T1 nc-vs-gRPC/mount-vs-data/hardcoded-paths,
T2 over-wire flake, and T3's overclaimed "Mac-off" (and a FALSE "can't detach" wall). **Full sustained
operator/fleet Mac-off → ITER-0008 STORY-0074.**
**AUDIT (PAR 2-auditor 3-tier, 2026-06-23): CLEAN** — both auditors ran the cluster scenarios live;
Tier 1 STORY-0010 PASS (AC-1 Mac-off genuinely detached via systemd-run, AC-4 done, AC-2/3/5 not-chosen),
SCENARIO-0092/0012 evidence adequate; Tier 2 renamed semantic tests + queue suite PASS; Tier 3 sentinels
green; no hardcoded `/nix/store` paths; zero `TODO(ITER-0006b)`; divergences (reap return-count, lease
non-exclusivity) documented honestly. Iteration confirmed done.
**Impacted scenarios:** SCENARIO-0092 (over-the-wire vs deployed Nix service, must-pass, PASS); SCENARIO-0012 (Mac-off acceptance, cluster-driven systemd-run detached drain, PASS-NARROW)
**Look-ahead check:** depends on ITER-0006 binding (real-wire proven); the must-pass core (T0–T2)
ships real STORY-0010 substance regardless; ITER-0007 (Temporal) builds on the deployed service via the
documented gRPC-only write seam. Full operator/sustained Mac-off → ITER-0008 STORY-0074.

### ITER-0007 — Eisenhower prioritization logic (CI-provable slice)

**SCOPE DECISION (2026-06-23, PAR scope review — 2 adversarial reviewers → both REVISE, high
agreement):** the original ITER-0007 (13 stories + 5 split-in ACs, Temporal standup) was split,
mirroring ITER-0005→0005/0005b/0005c and ITER-0006→0006/0006b. Temporal is a heavyweight NEW external
dependency (durable workflow engine + server + worker fleet, cluster-resident on ndn-desktop) — its
deployment + live sole-writer wiring + wall-clock aging + restart-survival are inherently cluster e2e,
no Mac CI seam. This iteration is the **CI-provable projection/authority logic slice** (pure Go,
unit/integration with a fake/mock Temporal + the existing laneq adapter). The live-Temporal cluster
work moves to **ITER-0007b**. Five orthogonal stories (provider/budget/thread-aging/multi-repo) move to
**ITER-0008**.

**Stories (CI-provable ACs only):** STORY-0040 (full — quadrant mapping), STORY-0045 (full — projection
determinism), STORY-0043 AC-1/AC-3 (urgency = monotonic f(deadline,now); Q4 never ages — pure math),
STORY-0042 (full — human-unrestricted/agent-bounded rescore validation), STORY-0047 AC-2/AC-3
(agent-bounded-rescore rejection logic + privileged→approval routing; AC-3 reuses the ITER-0001
escalation lane; **AC-1 live human-rescore → ITER-0007b**), STORY-0046 AC-1 (single-writer **guard** test:
no non-Temporal actor writes effective-priority/not-before), STORY-0041 AC-3 (laneq.next returns
highest-importance eligible — already proven ITER-0006 SCENARIO-0091, re-asserted here), STORY-0001 AC-3
(single-writer-constraint design, mock-Temporal). **Logic portions of split-ins:** STORY-0064 AC-15/AC-16
(directive importance/deadline inputs + agents-propose validation; human-authority already done ITER-0002),
STORY-0058 AC-24 (retry-backoff projection logic, fake clock), STORY-0061 AC-3 / STORY-0055 AC-7
(urgency-reprojection logic for stale escalations — the **operator-resurface journey** SCENARIO-0087 →
ITER-0008), STORY-0002 AC-2 (deferral-holder contract, mock), STORY-0044 AC-3 (Temporal-as-sole-caller
logic against a mock laneq; live gRPC → ITER-0007b).

**Rationale:** Lock the Eisenhower projection + rescore-authority + single-writer logic as pure,
deterministic Go (importance×urgency → effective-priority + not-before; bounded vs unrestricted rescore;
the guard that only the Temporal role writes scheduling fields) so ITER-0007b's live Temporal grafts the
*deployment + wiring* onto proven logic without re-litigating the algorithm. All CI-provable on the Mac
with a fake clock + the existing `queue.Queue`/laneq mock.

**Boxing-in mitigations (PAR):** (1) **No `Run` struct here** — the deferred STORY-0035 owned
Run.provider_instance/budget_snapshot; deferring it to ITER-0008 (alongside STORY-0011/0015's Run shape)
avoids the colliding-Run definition that PAR flagged twice (the same lesson ITER-0003 learned). (2) The
single-writer design (STORY-0046 AC-1 / STORY-0001 AC-3) is **process-level** (one Temporal role writes
the fields), explicitly **orthogonal to laneq lease exclusivity** — it must NOT assume consumer-exclusive
leases (ITER-0006 proved real laneq leases are non-exclusive); documented so ITER-0008 multi-consumer
delegation is not boxed in.

**Status:** done:ITER-0007 (2026-06-23) — Eisenhower projection/authority/single-writer/escalation LOGIC
implemented pure-Go in `modules/incus-dispatcher/temporal/` (`projection.go`/`authority.go`/`writer.go`/`escalate.go`),
100 new tests green under -race (suite 283→383). Stories: STORY-0040/0042/0045 done:ITER-0007 (full);
STORY-0041/0043/0044/0046/0047 + split-ins STORY-0001/0002/0055/0058/0061/0064 — CI-logic ACs done:ITER-0007,
live ACs → ITER-0007b (STORY-0064 fully CLOSED). Evidence SCENARIO-0078/0057/0082/0081/0087 wired + runnable.
TODO(ITER-0007) re-tagged → ITER-0007b (laneq Stats() observability is live-cluster, out of CI-logic scope).
**Impacted scenarios:** SCENARIO-0078 (deadline→Q2/Q1 + Q4-idle projection, **unit/fake-clock**),
SCENARIO-0057 (agent-bounded rescore rejection, unit/integration), SCENARIO-0082 (authority routing,
integration — human/agent identity via directive origin from ITER-0002), SCENARIO-0081 (single-writer
guard, integration + code-review). SCENARIO-0056 (live projection) → ITER-0007b.
SCENARIO-0087 is split three ways: the **urgency-reprojection LOGIC** (STORY-0061 AC-3 / STORY-0055 AC-7,
fake clock) is proven HERE; Temporal live re-raise → ITER-0007b; operator/TUI acts-on-resurfaced →
ITER-0008. **Seam reclassification (PAR):** deadline-aging proofs are fake-clock unit here;
wall-clock-over-real-time is ITER-0007b.
**Artifact debt (PAR, non-blocking):** EPIC-005 design-doc citations point at stale line numbers (the
`2026-06-18-fleet-orchestration-design.md` was restructured; e.g. STORY-0045's 405-409 now lands in the
queue-substrate section). Re-anchor EPIC-005 citations in a docs pass during decomposition; ACs are
internally coherent and unaffected.
**Look-ahead check:** depends on ITER-0006 not-before (done); the projection logic is the graft base for
ITER-0007b's live Temporal; deferring STORY-0035 + documenting non-exclusive leases keeps ITER-0008
(Run object, recursive delegation) unblocked.

### ITER-0007b — Temporal time plane: deployment, sole-writer wiring & live aging (cluster)

**SCOPE DECISION (2026-06-23, PAR):** the **cluster-resident live-Temporal slice** split out of ITER-0007.
Runs on `ndn-desktop`/`agent-host`; no Mac CI seam (mirrors ITER-0005b/0005c, ITER-0006b). Grafts the
deployed Temporal onto ITER-0007's proven projection logic + ITER-0006's deployed laneq gRPC
`Defer`/`Reprioritize` seam.

**Stories (live/e2e ACs):** STORY-0001 AC-1 (Temporal plane owns Schedules + durable timers + retry
backoff) + AC-2 (server+workers on ndn-desktop; **state survives host restart** — e2e), STORY-0041
AC-1/AC-2 (Temporal is the live sole writer of effective-priority + not-before; re-projects on rescore —
over the real laneq gRPC seam), STORY-0044 AC-3 (live: Temporal is the sole *caller* of
Defer/Reprioritize), STORY-0043 AC-2 (Q2→Q1 as deadline nears over **wall-clock** time, no human
intervention), STORY-0046 AC-2 (concurrent reads consistent under live Temporal), STORY-0047 AC-1 (live: human
rescore moves an item to any bucket via the deployed Temporal rescore path). **Live portions of split-ins:** STORY-0058 AC-24 (Temporal re-pushes
retries with backoff durably), STORY-0061 AC-3 / STORY-0055 AC-7 (Temporal re-raises stale escalations as
urgency rises), STORY-0002 AC-2 (deferred/future work durably held in Temporal until eligible).

**Resolved design decisions (PAR scope review, 2026-06-23 — verified against nixpkgs/cluster):**
- **Temporal package & service:** use the UPSTREAM `services.temporal` NixOS module (`temporal` **1.29.4**,
  pinned `nixos-25.11` — confirmed available; module options `enable`/`package`/`settings`/`dataDir`). It runs
  the full `temporal-server` (gRPC frontend :7233). No hand-rolled `buildGoModule` needed.
  *(Availability verified 2026-06-23 on the cluster against the pinned channel: `nix eval --raw
  github:NixOS/nixpkgs/nixos-25.11#temporal.version` → `1.29.4`; module at
  `nixos/modules/services/cluster/temporal/default.nix`, wired in `module-list.nix:494`.)*
- **Durable persistence (STORY-0001 AC-2):** the upstream NixOS *test* uses in-memory SQLite (`mode=memory`,
  NOT durable). ITER-0007b MUST configure `settings.persistence` with **file-backed SQLite** (`pluginName=sqlite`,
  db file under `dataDir`, `mode≠memory`) on an **Incus host-mounted volume** (mirror laneq-data at /srv/laneq),
  so deferred workflows/timers survive container/host restart.
- **Sole-writer enforcement model (boxing-in answer):** laneq leases are **non-exclusive** (SCENARIO-0092: server
  keys leases by directive id, no per-consumer token). The sole-writer guarantee is therefore **process-level
  disciplined-client** (exactly one Temporal worker role calls `Defer`/`Reprioritize`), NOT lease/RBAC exclusivity.
  This is orthogonal to ITER-0008 multi-consumer delegation and MUST be stated upfront in the deploy doc.
- **Aging proof = compressed real-wall-clock (not fake-clock, not multi-day):** SCENARIO-0056/STORY-0043 AC-2
  ("over wall-clock time") is proven LIVE by setting a deadline a few seconds out and letting a real Temporal
  timer fire on actual wall-clock, then asserting Q2→Q1 + laneq reflects it. This is genuine wall-clock aging
  (real time advances) but cluster-runnable in seconds — distinct from ITER-0007's fake-clock CI proof.
- **Temporal worker is Go:** the worker uses the Temporal Go SDK and imports ITER-0007's `temporal/projection.go`
  /`writer.go`/`authority.go`/`escalate.go` directly (same language); it calls the laneq Go gRPC client.

**Task 0 (PAR-pattern, BLOCKING) — decomposed:**
- **T0.1 — Deploy:** `services.temporal` module wired into the agent-host NixOS config (file-backed SQLite on a
  host volume, readiness probe on :7233), shipped via `scripts/deploy.sh`. Mirror laneq discipline.
- **T0.2 — Cluster verification harness:** boot-to-ready sentinel (server answers on :7233), restart-survival
  check (enqueue deferred workflow → restart service/container → assert it reloads & still fires), and the
  compressed-wall-clock aging probe scaffold.
- **T0.3 — Deploy doc (COMMITTED deliverable, not optional):** `docs/plans/2026-06-23-iter0007b-temporal-deploy.md`
  — **stub committed at scope-approval time (2026-06-23)** with the sole-writer contract already written; T0.1/T0.2
  fill in deployment steps + readiness output. Contains container/service/volume/readiness/durability steps AND,
  prominently upfront, the **sole-writer seam contract**:
  Temporal writes scheduling fields ONLY via gRPC `Defer`/`Reprioritize` (never direct laneq DB), enforced by
  process-level discipline, **non-exclusive-lease** assumption restated for ITER-0008.

**Story sequencing:** T0 (deploy + harness + doc) is a hard gate. STORY-0001 AC-1 (Temporal owns
schedules/timers/backoff) is provable at the **integration** seam once the worker exists; STORY-0001 AC-2
(restart survival), STORY-0043 AC-2 (wall-clock aging), STORY-0046 AC-2 (concurrent reads), STORY-0047 AC-1
(human rescore), STORY-0044 AC-3 (sole caller) all depend on the LIVE deployed Temporal → run **after** the T0 gate.

**GATE for ITER-0008 (no carries permitted):** STORY-0041 AC-1/AC-2 (live sole writer) + STORY-0044 AC-3
(Temporal sole caller of Defer/Reprioritize) are gating for ITER-0008 dispatch/delegation work. If either slips
or carries, ITER-0008 cannot proceed — the time plane would not be stable.

**Status:** done:ITER-0007b (2026-06-24) — durable Temporal time plane deployed + wired live. Code C2–C5 (all
two-stage PAR): C2 PriorityWorkflow + sole-writer ReprojectActivity (Reprioritize+Defer seam) + lease-free
`LaneqQueue.Defer`; C3 rescore signal (human-unrestricted / agent-bounded) + `currentImportance` query; C4
RetryWorkflow (exp backoff re-push) + EscalationWorkflow (time-driven stale re-raise) + DeferWorkflow
(hold-until-eligible), all via the sole-writer seam; C5 concurrent-read consistency (both guarded fields, -race).
E1 live cluster harness (env-gated `TEMPORAL_LIVE=1`, cross-compiled + `incus exec` in agent-host) **independently
re-run by the orchestrator**: SCENARIO-0094 LIVE-PROVEN (human rescore flips laneq P1→P0 with a real worker
executing the workflow), SCENARIO-0001 LIVE-PROVEN (DeferWorkflow survives a REAL Temporal restart PID 6976→7066,
same runID Running→Completed post-restart, directive fired — STORY-0001 AC-2 durable timer, not laneq natural
expiry). Mac suite 387→429 -race green; gated live tests skip in CI. **ITER-0008 GATE MET:** STORY-0041 AC-1/AC-2
(live sole writer — C2/C3 + E1 priority flip) + STORY-0044 AC-3 (sole caller — C2 CI + E1 0093) proven; no carries.
**Honest live/CI split:** SCENARIO-0056 — live proves the durable timer + real-gRPC reproject MECHANISM; Q2→Q1
quadrant logic is CI-PROVEN (testsuite time-skip), and live wall-clock Q2→Q1 is NOT compressible to seconds
(urgency calibrated in days → seconds-out deadline starts already-Q1) — needs a multi-day runner or urgency knob
(→ ITER-0008/ops). SCENARIO-0081/0093 live = concurrent Peek over real gRPC succeeds + process-level sole-caller;
value-consistency & sole-writer enforcement are CI-PROVEN (C5/C2).
**Impacted scenarios:** SCENARIO-0056 (live single-writer projection + compressed-wall-clock deadline aging,
integration), SCENARIO-0001 (Temporal plane durable + restart survival, e2e), SCENARIO-0081 (live concurrent-read
consistency under single writer, integration), **SCENARIO-0093 (NEW — Temporal is the sole live caller of laneq
`Defer`/`Reprioritize`; STORY-0044 AC-3, integration)**, **SCENARIO-0094 (NEW — live human rescore via deployed
Temporal moves an item to any bucket; STORY-0047 AC-1, integration)**. The operator-experience half of
SCENARIO-0087 (a human/TUI consumer acts on the re-raised escalation) → ITER-0008.
**Artifact-debt note (non-blocking):** EPIC-005/EPIC-008 cards for STORY-0055 AC-7, STORY-0061 AC-3, STORY-0002
AC-2 read as single ACs but are split logic(ITER-0007)/live(ITER-0007b)/operator(ITER-0008); annotate on a docs pass.
**Look-ahead check:** depends on ITER-0007 logic + ITER-0006 deployed laneq; Temporal becomes the sole
writer of the gRPC seam so ITER-0008 steering/delegation builds on a stable time plane.

### ITER-0007c — laneq grant signing (PASETO host-to-host auth), Phase 1

**Origin:** human interrupt (2026-06-24) — secure the laneq gRPC for non-local networks (host-to-host
signing) + a grant mechanism on laneq (addresses laneq's "security story" feedback). Approved design:
`docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md`.

**Intent (Phase 1, end-to-end):** sign every laneq gRPC call with a **PASETO v4.public (Ed25519)** grant
so laneq is safe across non-local (Tailscale) networks; verify **inside laneq** (`nnunley/laneq` gRPC
interceptor); roll out **`log-only` → `enforce`** against the live cluster. Trust root (Ed25519 private
key + service passwords) stays on the user's Mac; laneq holds only public key(s).

**Scope:** (a) grant token format — `iss`/`sub`/`aud`(laneq instance)/`iat`/`nbf`/`exp`/`jti` + footer
`kid`; (b) Mac issuer `laneq-grant` CLI (mint + renew; private key in macOS Keychain / secstore vault —
seeds the `2026-06-16-credential-broker-architecture.md` `brokerd`); (c) Go client `GrantSource` +
gRPC client interceptor attaching the token to `LaneqQueue` RPCs (additive; absent grant == legacy
passthrough); (d) laneq Python `ServerInterceptor` verifying v4.public sig + `exp`/`nbf`/`aud`, config
mode `off|log-only|enforce`, `kid` key rotation; (e) Phase-1 token delivery via Incus systemd-credential
push + Mac-side renewal helper.

**Stories/scenarios (extracted 2026-06-24 → EPIC-014):** STORY-0079 (PASETO v4.public grant format + Mac
issuer), STORY-0080 (Go client grant attachment), STORY-0081 (laneq Python verify interceptor + log-only/enforce
modes + kid rotation), STORY-0082 (token delivery, rollout, transport). Scenarios SCENARIO-0117 (host-signed RPC
accepted), SCENARIO-0118 (forged/expired/wrong-`aud` rejected `UNAUTHENTICATED`), SCENARIO-0119 (`log-only`
allows+logs, then enforce rejects), SCENARIO-0120 (`kid` key rotation). Real-wire evidence extends
`queue/run-laneq-wire.sh` + `laneq_realwire_lifecycle_test.go`.

**Phasing:** Phase 2 (separate spec/iteration) adds `cap:{ops,lanes}` per-consumer/op capabilities and
ENFORCES the sole-writer rule at laneq (only the Temporal-role grant may `Defer`/`Reprioritize`) — upgrading
the ITER-0007b process-level invariant to authz. The full provider-credential broker stays the broker doc.

**Sequencing:** runs **before/alongside ITER-0008** — ITER-0008's multi-consumer/recursive delegation
(non-exclusive leases) benefits from real per-consumer grants, and Phase 2 sole-writer enforcement builds
directly on the ITER-0007b seam. **Status:** IN PROGRESS (formal `running-an-iteration` resumed 2026-06-25;
verification side + Go signing core + GrantSource (T1 `e0f4a5d`) committed). **Scope revised by PAR scope review 2026-06-25:**
STORY-0082 AC-1 split into **AC-1a (local e2e log-only→enforce via `run-laneq-wire.sh`, in-scope)** and
**AC-1b (live-cluster rollout + external laneq PR — DEFERRED, gated on operator authorization, outward-facing)**;
STORY-0080 AC-3 gains a **mandatory automated Go real-wire evidence task** (replacing "proven manually");
STORY-0080 AC-2+AC-3 implemented as one TDD task block.
**ITER-0008 precondition (PAR boxing-in finding 2026-06-25):** the Phase-1 grant is host-level (`sub=agent-host`),
single-consumer. ITER-0008 recursive delegation (STORY-0028/0073) + Phase-2 sole-writer authz REQUIRE the Mac
issuer to mint **per-role grants** (`--sub temporal-writer|daemon-consumer`) and `GrantSource` to select among
them. The ITER-0007c issuer/`GrantSource` are intentionally NOT consumer-aware; this issuer upgrade is a
precondition for ITER-0008 multi-consumer delegation, not a Phase-1 blocker.

**Python/uv toolchain decision (2026-06-24 research):** the laneq side (STORY-0081) is a `uv`-managed
Python project; the offline fleet worker has no PyPI egress (binary-cache only) and prebuilt manylinux
wheels won't run on NixOS without nix-ld/FHS. **Two tracks:** *Track 1 (first PR, on the connected Mac)* —
plain `uv` (no Nix; Nix isn't on the Mac): `uv sync` + `uv run ruff format --check / ruff check / pytest /
coverage` for exact parity with laneq's 4 CI gates. *Track 2 (fleet-worker offline grading, the dogfood)* —
**uv2nix** (`pyproject-nix/uv2nix` + `pyproject-build-systems`) so laneq's `uv.lock` closure is realized by
Nix on `nix-server` and pulled from the shared cache like everything else (no PyPI, no manylinux/FHS
problem); oracle runs `ruff/pytest/coverage` from the uv2nix venv, NOT `uv run`. Track 1 is first-PR path;
Track 2 is a worker-image investment (STORY-0075 territory), not first-PR-blocking. uv2nix is stable
(dropped "experimental" early 2025).
**Look-ahead check:** spans two repos (agent-sandbox token+client+issuer; nnunley/laneq interceptor);
rollout is non-breaking (log-only first) so it does not disrupt the live ITER-0007b cluster.

### ITER-0008 — Tier-2 coordinator, recursive delegation & operator UX

**Stories:** STORY-0073, STORY-0028, STORY-0012, STORY-0013, STORY-0014, STORY-0026, STORY-0006, STORY-0003, STORY-0009, STORY-0032, STORY-0074, **STORY-0027 AC-3 (operator pause/block/resume from TUI — split in from ITER-0001), STORY-0054 (audit all runs/delegations/mutations + replayability — moved from ITER-0001, folds into STORY-0032's genome/delegation audit), STORY-0016 (versioned execution policies — moved from ITER-0002 PAR: delegation_rules/mutation_allowed gain meaning with recursive delegation here), STORY-0011 (policy-driven worker dispatch — moved from ITER-0002 PAR: needs multiple worker_kinds (post-ITER-0005) + Tier-2 dispatch decisions), STORY-0049 AC-4 (worker-authored child-directive inherits non-privileged provisioning — moved from ITER-0002 PAR: needs the recursive child-directive emit path built here), STORY-0015 (capture artifacts: Run object with run_id/artifact_refs/log_refs — moved from ITER-0003 PAR: build with STORY-0011's Run shape to avoid a colliding/duplicate Run definition), STORY-0035 (Run provider_instance/model_id/budget_snapshot — moved from ITER-0007 PAR 2026-06-23: its Run fields MUST be defined with STORY-0011/0015's Run shape, same anti-collision lesson), STORY-0036 (multi-level budget guardrails — moved from ITER-0007 PAR: budget enforcement is a dispatch/resource-allocation concern, not the time plane), STORY-0037 (thread aging + queue classes + stale-thread resurfacing — moved from ITER-0007 PAR: thread-registry/operator concern; pairs with the operator-resurface half of SCENARIO-0087), STORY-0038 (provider instances + escalation routing — moved from ITER-0007 PAR: dispatch routing, composes with STORY-0011 worker_kind), STORY-0039 (multi-repo thread coordination — moved from ITER-0007 PAR: thread-spanning, operationalized with the coordinator/TUI here)**
**Rationale:** Bidirectional steering (file-feed now), operator TUI for
thread/worker management (incl. STORY-0027 AC-3 thread pause/block/resume — it needs the TUI built
here), the full agent/delegation/mutation audit + replay (STORY-0054, alongside STORY-0032 genome
mutation — distinct from ITER-0001's coordination-level D6 decision log), durable
message-queue-first recursive delegation,
one-shot vs long-running modes, the Mac-off-stateless-client framing made
concrete, deterministic-loop + service-discovery stories, safe/auditable genome
mutation, and the **full Mac-off acceptance test (STORY-0074)** — now that
substrate (ITER-0006) + Temporal (ITER-0007) + the escalation ladder exist to
exercise it end-to-end. Capstone integration.
**Status:** pending
**Impacted scenarios:** bidirectional-steer; operator-TUI; recursive-delegation; Mac-off-client
**Look-ahead check:** depends on ITER-0007 + the coordination plane; final integration.
**Substrate constraint (from ITER-0006 T6 real-wire, 2026-06-22):** real laneq leases are NOT
consumer-exclusive — the server keys leases by directive id and does NOT enforce per-consumer token
ownership on Touch/Done (verified in SCENARIO-0092; the in-process fake is stricter). Recursive
delegation / multi-consumer / work-stealing here MUST NOT assume lease exclusivity; if it's required,
add an opaque per-claim token to laneq upstream (nnunley/laneq) first. See `queue/laneq.go`
`directiveFromProto` divergence note.

## Deferred / cross-cutting

- **STORY-0037** (thread aging) appears in ITER-0007 (urgency aging is a Temporal concern).
- **Story split pending PAR:** STORY-0058 (ITER-0000 minimal outcome ↔ ITER-0001 full
  ladder) to be formalized in the requirements index during scope review.
