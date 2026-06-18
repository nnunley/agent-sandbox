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

## Walking skeleton (ITER-0000)

**Intent:** Drive a single directive end-to-end on the container backend — claim
→ validate template → launch from golden → deliver repo → run runner → harvest
diff+result → authoritative external grade → minimal outcome (pass→done /
fail→requeue) → stop+reap — with a stub queue, and stand up the E2E journey
harness. Plus prove the two unvalidated assumptions.

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
- STORY-0034 (EPIC-004) — **SPIKE**: ctx_handoff round-trip validation
- STORY-0025 (EPIC-002) — **SPIKE**: disposable-unit spin-up vs VM boot benchmark

(STORY-0060 folded into Task 0 harness — see above.)

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

**Status:** pending

## Iteration list

### ITER-0001 — Coordination plane + audit hardening

**Stories:** STORY-0055, STORY-0058 (ladder remainder), STORY-0059, STORY-0061, STORY-0027, STORY-0056, STORY-0054, STORY-0063 (decision-log remainder)
**Rationale:** Promote the skeleton's minimal outcome handling to the full D4
deterministic loop + graduated escalation ladder (incl. non-blocking human
escalation lane), thread-status tracking, and the D6 append-only decision log
behind a swappable writer interface. All substrate-independent.
**Status:** pending
**Impacted scenarios:** coordination-loop + escalation scenarios; thread-status; audit-log
**Look-ahead check:** depends on ITER-0000's outcome hook; blocks nothing downstream.

### ITER-0002 — Provisioning & template security hardening

**Stories:** STORY-0049, STORY-0053, STORY-0048, STORY-0016, STORY-0011
**Rationale:** Full D1 intent/template provisioning + origin/authority enforcement,
secret broker (no raw provider creds to workers), externalized/versioned execution
policies, policy-driven dispatch. Hardens the skeleton's minimal template check.
**Status:** pending
**Impacted scenarios:** privilege-escalation-denial; secret-isolation; policy dispatch
**Look-ahead check:** depends on ITER-0000 template validation; independent of substrate.

### ITER-0003 — Worker reliability & robust result contract

**Stories:** STORY-0067, STORY-0072, STORY-0068, STORY-0069, STORY-0070, STORY-0015, STORY-0071
**Rationale:** The productization-plan reliability cluster: Go-exec PATH fix,
truncation-robust result contract, grading round-trip proof, lean-ctx bridge ON,
canonical runner modes, artifact capture, ctx_*-aware heartbeat. Plus a *minimal*
Mac-off smoke in the harness (daemon keeps claiming/running/grading while the Mac
is disconnected — no Temporal, no full ladder yet). The FULL Mac-off acceptance
test (STORY-0074) is rescheduled to ITER-0008 (after substrate + Temporal exist).
**Status:** pending
**Impacted scenarios:** result-survives-truncation; minimal Mac-off smoke; bridge-ON; heartbeat
**Look-ahead check:** minimal smoke exercises ITER-0000..0002; full STORY-0074 → ITER-0008.

### ITER-0004 — State passthrough & continuity (post-spike)

**Stories:** STORY-0029, STORY-0030, STORY-0033, STORY-0018, STORY-0031
**Rationale:** Build the lean-ctx-based handoff continuity once STORY-0034 proves
the round-trip: context preservation across thread boundaries, anti-reinvention,
branch/workspace claim checks, soft-state-not-authoritative discipline, stumble
signals. Gated on the ITER-0000 spike outcome.
**Status:** pending
**Impacted scenarios:** handoff-round-trip; continuity; claim-before-reuse
**Look-ahead check:** gated by STORY-0034 (ITER-0000); independent of substrate.

### ITER-0005 — Micro-VM backend, NixOS golden & isolation tiers (post-benchmark)

**Stories:** STORY-0075, STORY-0077, STORY-0078, STORY-0076, STORY-0005, STORY-0007, STORY-0008, STORY-0021, STORY-0022, STORY-0023, STORY-0024, STORY-0017, STORY-0020, STORY-0004
**Rationale:** The declarative-worker track: NixOS golden (retire Ubuntu), skills
via agent-skills-nix, provider routing, immutable golden copies, the durable VM
hosting disposable tiered units, fast/hard isolation tiers selected per template,
trust-domain VMs, and the second (micro-VM) backend behind the interface. Gated on
STORY-0025 benchmark choosing the disposable substrate.
**Status:** pending
**Impacted scenarios:** tier-selection; immutable-image; VM-boot-readiness; backend-parity
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
policy. Needs laneq's `not-before` (ITER-0006).
**Status:** pending
**Impacted scenarios:** single-writer-projection; rescore-authority; deadline-aging; budget
**Look-ahead check:** depends on ITER-0006 not-before; blocks ITER-0008 steering.

### ITER-0008 — Tier-2 coordinator, recursive delegation & operator UX

**Stories:** STORY-0073, STORY-0028, STORY-0012, STORY-0013, STORY-0014, STORY-0026, STORY-0006, STORY-0003, STORY-0009, STORY-0032, STORY-0074
**Rationale:** Bidirectional steering (file-feed now), operator TUI for
thread/worker management, durable message-queue-first recursive delegation,
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
