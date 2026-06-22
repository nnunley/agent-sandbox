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

**Status:** pending
**Impacted scenarios:** SCENARIO-0091 (NEW, CI integration — gRPC adapter lifecycle incl. lanes/threading); SCENARIO-0092 (NEW, real-wire e2e via uvx, **this iteration**); SCENARIO-0045 (directive contract, unit); SCENARIO-0012 (Mac-off → ITER-0006b)
**Look-ahead check:** substrate confirmed; the gRPC `Defer`/`Reprioritize` seam is built AND real-wire-proven (SCENARIO-0092) so ITER-0007 Temporal becomes the sole writer without rework. Unblocks ITER-0006b + ITER-0007.

### ITER-0006b — laneq Nix package + cluster deploy + Mac-off acceptance (cluster)

**Stories:** STORY-0010 (closeout AC-1)
**Rationale:** Package laneq + its gRPC server (pinned-hash fork) as a **Nix package** for the NixOS
cluster; deploy on `ndn-desktop` with the SQLite DB on a host volume; run the **Mac-off acceptance
test**. The Go↔real-laneq wire is already proven in ITER-0006 (SCENARIO-0092 via uvx); this iteration
*productionizes* it (Nix-packaged service) and proves the cluster Mac-off property — the
cluster/deploy half that cannot run in Go-only CI (mirrors the ITER-0005/0005b/0005c CI-vs-cluster
split).
**Task sketch:** Nix derivation for laneq (pinned hash) + gRPC server entrypoint; deploy unit on
ndn-desktop (DB host volume); re-run SCENARIO-0092 against the Nix-packaged service; SCENARIO-0012
(Mac-off: close Mac → workers keep claiming/processing via laneq on ndn-desktop; DB survives;
reconnect with no loss).
**Status:** pending
**Impacted scenarios:** SCENARIO-0012 (Mac-off acceptance); SCENARIO-0092 (re-run productionized)
**Look-ahead check:** depends on ITER-0006 binding (real-wire already proven); closes STORY-0010 AC-1;
carry-allowance applies if the cluster Mac-off e2e hits a deploy wall (precedent: ITER-0005c).

### ITER-0007 — Time plane & Eisenhower prioritization (Temporal)

**Stories:** STORY-0001, STORY-0040, STORY-0041, STORY-0042, STORY-0043, STORY-0045, STORY-0046, STORY-0047, STORY-0037, STORY-0035, STORY-0036, STORY-0038, STORY-0039
**Rationale:** Stand up Temporal as the time plane and single writer; importance×
urgency projection to effective-priority + not-before; bounded vs unrestricted
rescore authority; deadline-driven aging; provider/budget/multi-repo scheduling
policy. Needs laneq's `not-before` (ITER-0006). **Also lands the Temporal-coupled escalation
ACs split in from ITER-0001 (per PAR): STORY-0055 AC-7 (re-surface stale human-pending escalations),
STORY-0058 AC-24 (retry re-pushed by Temporal with backoff), STORY-0061 AC-3 (urgency-driven
resurfacing in priority order; carries SCENARIO-0087).** These graft onto ITER-0001's escalations
lane + decision log without reworking them — the lane was built as a plain durable FIFO precisely so
Temporal aging layers on top. **Also lands the substrate ACs deferred from ITER-0006 (PAR 2026-06-22):
STORY-0044 AC-3 (Temporal sole writer of not_before — becomes the sole caller of the laneq gRPC
`Defer`/`Reprioritize` seam built in ITER-0006), STORY-0002 AC-2 (deferred/future work lives in
Temporal until eligible, not in laneq), STORY-0064 AC-15/AC-16 (importance/deadline are Temporal
projection inputs; agents may only PROPOSE, humans set freely — cross-surface enforcement). These
graft onto ITER-0006's gRPC seam without rework.**
**Status:** pending
**Impacted scenarios:** single-writer-projection; rescore-authority; deadline-aging; budget;
escalation-resurface (SCENARIO-0087)
**Look-ahead check:** depends on ITER-0006 not-before; blocks ITER-0008 steering.

### ITER-0008 — Tier-2 coordinator, recursive delegation & operator UX

**Stories:** STORY-0073, STORY-0028, STORY-0012, STORY-0013, STORY-0014, STORY-0026, STORY-0006, STORY-0003, STORY-0009, STORY-0032, STORY-0074, **STORY-0027 AC-3 (operator pause/block/resume from TUI — split in from ITER-0001), STORY-0054 (audit all runs/delegations/mutations + replayability — moved from ITER-0001, folds into STORY-0032's genome/delegation audit), STORY-0016 (versioned execution policies — moved from ITER-0002 PAR: delegation_rules/mutation_allowed gain meaning with recursive delegation here), STORY-0011 (policy-driven worker dispatch — moved from ITER-0002 PAR: needs multiple worker_kinds (post-ITER-0005) + Tier-2 dispatch decisions), STORY-0049 AC-4 (worker-authored child-directive inherits non-privileged provisioning — moved from ITER-0002 PAR: needs the recursive child-directive emit path built here), STORY-0015 (capture artifacts: Run object with run_id/artifact_refs/log_refs — moved from ITER-0003 PAR: build with STORY-0011's Run shape to avoid a colliding/duplicate Run definition)**
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

## Deferred / cross-cutting

- **STORY-0037** (thread aging) appears in ITER-0007 (urgency aging is a Temporal concern).
- **Story split pending PAR:** STORY-0058 (ITER-0000 minimal outcome ↔ ITER-0001 full
  ladder) to be formalized in the requirements index during scope review.
