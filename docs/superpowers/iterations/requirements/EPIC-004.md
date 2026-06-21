# EPIC-004 — State passthrough & continuity

**Summary:** State passthrough & continuity
**Stories:** STORY-0029, STORY-0030, STORY-0031, STORY-0032, STORY-0033, STORY-0034
**Primary sources:** `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`, `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 1/6 done (STORY-0034 spike validated:ITER-0000; STORY-0029/0030/0031/0032/0033 → ITER-0004)

## STORY-0029

**Epic:** EPIC-004 — State passthrough & continuity
**Title:** Preserve work context across thread boundaries

**As a** system user
**I want** threads to capture and resume all relevant context when continuing work
**So that** agents can pick up prior work without reinventing or losing state

**Acceptance criteria:**
- AC-1: Thread object includes resume_summary field capturing prior work and next_step · impact:`local` · seam:`unit` · scenario:`SCENARIO-0015`
- AC-2: Thread object includes last_verified_state field documenting the last known good state · impact:`local` · seam:`unit` · scenario:`SCENARIO-0015`
- AC-3: When resuming a thread with active work, new runs continue current branch/workspace by default · impact:`journey` · seam:`integration` · scenario:`SCENARIO-0015`
- AC-4: System reconstructs authoritative branch/workspace, current diff, last verified result, and open questions before resuming · impact:`journey` · seam:`process-level` · scenario:`SCENARIO-0015`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:521-546`

**Status:** pending

## STORY-0030

**Epic:** EPIC-004 — State passthrough & continuity
**Title:** Prevent reinvention of in-progress work

**As a** coordinator
**I want** new work requests to detect and continue existing threads instead of creating duplicates
**So that** effort is not wasted on redundant implementations

**Acceptance criteria:**
- AC-1: Thread object includes supersedes and superseded_by fields for explicit thread relationships · impact:`local` · seam:`unit` · scenario:`SCENARIO-0015`
- AC-2: New work on a branch with active thread status must explicitly declare why prior path is insufficient to supersede it · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0015`
- AC-3: Reinvention signal is captured as structured stumble in run's stumble_signals array · impact:`local` · seam:`unit` · scenario:`SCENARIO-0015`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:35-37, 131, 536-542`

**Status:** pending

## STORY-0031

**Epic:** EPIC-004 — State passthrough & continuity
**Title:** Capture structured stumble signals for learning

**As a** coordinator
**I want** to collect repeated failure patterns so mutations can be proposed and evaluated
**So that** the system can improve its own prompts and policies

**Acceptance criteria:**
- AC-1: Run object includes stumble_signals array · impact:`local` · seam:`unit` · scenario:`SCENARIO-0016`
- AC-2: Stumble signal types include: retries, timeouts, verification failures, provider failures, delegation loops, workspace-loss, duplicate work, cost blowouts, starvation · impact:`local` · seam:`unit` · scenario:`SCENARIO-0016`
- AC-3: When stumble pattern repeats, mutation proposal is generated · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0016`
- AC-4: Genome entry captures evidence_refs linking mutations back to stumble signals · impact:`local` · seam:`unit` · scenario:`SCENARIO-0016`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:184, 433-445, 447-453`

**Status:** pending

## STORY-0032

**Epic:** EPIC-004 — State passthrough & continuity
**Title:** Make genome mutation safe and auditable

**As a** system
**I want** mutations to be proposed, trialed, measured, and promoted/reverted explicitly
**So that** learned changes can be inspected and reverted

**Acceptance criteria:**
- AC-1: Genome entry includes version, content_hash, source (bootstrap/learned/promoted/experiment), status (active/candidate/rejected/reverted) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0018`
- AC-2: Mutation can target: prompts, routing heuristics, provider/model heuristics, budget escalation, execution policy, thread handoff templates · impact:`local` · seam:`unit` · scenario:`SCENARIO-0018`
- AC-3: Secret handling, lease safety, audit requirements, hard budget guardrails, kernel safety remain protected from mutation · impact:`local` · seam:`unit` · scenario:`SCENARIO-0018`
- AC-4: Mutation flow is: detect pattern → propose → trial/experiment → measure outcome → promote/keep/reject/revert · impact:`journey` · seam:`process-level` · scenario:`SCENARIO-0018`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:256-277, 418-463`

**Status:** pending

## STORY-0033

**Epic:** EPIC-004 — State passthrough & continuity
**Title:** Check branch/workspace claims before reusing

**As a** coordinator
**I want** to verify workspace claim before using existing directory
**So that** concurrent work does not collide or corrupt state

**Acceptance criteria:**
- AC-1: Workspace lease is checked before reusing directory · impact:`local` · seam:`unit` · scenario:`SCENARIO-0015`
- AC-2: Thread owns authoritative branch/workspace metadata · impact:`local` · seam:`unit` · scenario:`SCENARIO-0015`
- AC-3: If workspace is already claimed by active thread, new request must either continue or explicitly supersede · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0015`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:129-130, 543-546`

**Status:** pending

## STORY-0034

**Epic:** EPIC-004 — State passthrough & continuity
**Title:** Context handoff round-trip validation (spike)

**As a** validation engineer
**I want** to verify that ctx_handoff round-trips decisions across two separate claude -p invocations on the worker without data loss
**So that** we gate the feature and confirm it is safe for production dogfood

**Acceptance criteria:**
- AC-1: context handoff spike: run two sequential claude -p invocations with lean-ctx compression and bridge enabled, verify decision state matches · impact:`journey` · seam:`integration` · scenario:`SCENARIO-0077`
- AC-2: spike validates that no decision state is lost in the round-trip · impact:`journey` · seam:`integration` · scenario:`SCENARIO-0077`
- AC-3: spike unblocks ctx_handoff feature for production dogfood rollout · impact:`journey` · seam:`integration` · scenario:`SCENARIO-0077`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:402-404`

**Status:** done:ITER-0000 (spike validated 2026-06-21). AC-1/AC-2/AC-3 met — cluster spike PASS
(airtight nonce round-trip across two `claude -p` invocations, no data loss; evidence in SCENARIO-0077).
This was the ITER-0000 off-critical-path spike gating ITER-0004; the gate is now cleared.