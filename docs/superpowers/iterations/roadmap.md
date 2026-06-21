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
the ITER-0004 gate; SCENARIO-0077); STORY-0025 benchmark spike → still pending (gates ITER-0005).)

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
**Status:** pending (gate cleared — ready to start)
**Impacted scenarios:** handoff-round-trip (SCENARIO-0077, spike PASS); continuity; claim-before-reuse
**Look-ahead check:** gate STORY-0034 (ITER-0000) **CLEARED**; independent of substrate.

### ITER-0005 — Micro-VM backend, NixOS golden & isolation tiers (post-benchmark)

**Stories:** STORY-0075, STORY-0077, STORY-0078, STORY-0076, STORY-0005, STORY-0007, STORY-0008, STORY-0021, STORY-0022, STORY-0023, STORY-0024, STORY-0017, STORY-0020, STORY-0004
**Rationale:** The declarative-worker track (STORY-0075 here = the FULL golden /
retire-Ubuntu / micro-VM build; the minimal container worker image already landed
in ITER-0000 for the dogfood): NixOS golden, skills
via agent-skills-nix, provider routing, immutable golden copies, the durable VM
hosting disposable tiered units, fast/hard isolation tiers selected per template,
trust-domain VMs, and the second (micro-VM) backend behind the interface. Gated on
STORY-0025 benchmark choosing the disposable substrate.
**Split-in (from ITER-0002 PAR):** STORY-0049 AC-5 (launched template is immutable root with
writable scratch — /workspace, /tmp tmpfs/overlay) lands here as part of the golden image
(STORY-0075); plus SCENARIO-0020's microVM host credential-socket isolation (the broker proof
itself shipped in ITER-0002 at the container/proxy seam).
**Status:** pending
**Impacted scenarios:** tier-selection; immutable-image; VM-boot-readiness; backend-parity;
immutable-root-scratch (STORY-0049 AC-5)
**Look-ahead check:** gated by STORY-0025 (ITER-0000); reuses ITER-0000 backend interface.

### ITER-0006 — Queue substrate (POST-PATRICK; substrate-coupled)

**Stories:** STORY-0010, STORY-0044, STORY-0002, STORY-0064
**Rationale:** Replace the stub queue with the chosen substrate (provisionally
extend laneq + `not-before`), cluster-resident, passing the Mac-off acceptance
test; finalize the directive contract fields. **BLOCKED on the Patrick sync —
do not start until the substrate is confirmed.**
**Status:** blocked
**Impacted scenarios:** queue-substrate Mac-off; not-before eligibility; directive schema
**Look-ahead check:** blocked-on-decision; unblocks ITER-0007.

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
Temporal aging layers on top.
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
