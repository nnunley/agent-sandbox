# EPIC-008 — Coordination loop & escalation

**Summary:** Coordination loop & escalation
**Stories:** STORY-0055, STORY-0056, STORY-0057, STORY-0058, STORY-0059, STORY-0060, STORY-0061
**Primary sources:** `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 4/7 done
## STORY-0055

**Epic:** EPIC-008 — Coordination loop & escalation
**Title:** D4: Deterministic coordination loop with graduated escalation ladder; human escalation non-blocking

**As a** daemon loop operator
**I want** to apply fixed rules (pass → done, fail-transient → retry, fail-repeats → escalate worker, fail-still → escalate resources, authority-limit → human lane) without model in loop
**So that** coordination is deterministic, Mac-off-safe, and privileged rungs are reachable only via human approval

**Acceptance criteria:**
- AC-1: pass grade → mark thread done · impact:`local` · seam:`unit` · scenario:`SCENARIO-0032`
- AC-2: fail (transient) grade → retry same with temporal backoff · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0032`
- AC-3: fail (repeats) grade → escalate to stronger worker model (pre-approved rung) · impact:`cross-surface` · seam:`process-level` · scenario:`SCENARIO-0032`
- AC-4: fail (still) grade → escalate to bigger/hard-tier template (pre-approved rung) · impact:`cross-surface` · seam:`process-level` · scenario:`SCENARIO-0032`
- AC-5: authority/judgment limit → escalate to human escalations lane (distinct durable state, non-blocking, threaded to origin) · impact:`journey` · seam:`process-level` · scenario:`SCENARIO-0032`
- AC-6: Privileged rungs (root/sensitive template) reachable only via human escalations lane, never autonomously · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0032`
- AC-7: Every ladder transition logged to D6 decision-log; Temporal re-surfaces stale human-pending escalations (urgency rises → re-notify) · impact:`journey` · seam:`process-level` · scenario:`SCENARIO-0032`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:188-208`

**Status:** done:ITER-0001 (D4 loop + ladder AC-1..6; AC-7 Temporal resurface→ITER-0007)

## STORY-0056

**Epic:** EPIC-008 — Coordination loop & escalation
**Title:** D6: Append-only decision log behind swappable writer interface for future tamper-evident upgrade

**As a** compliance auditor
**I want** to log every coordination decision (directive id, grade summary, rule fired, action, ts) in append-only JSONL format
**So that** audit trail is immutable and can be swapped to HMAC-chained tamper-evident log without rearchitecting

**Acceptance criteria:**
- AC-1: Decision log behind writer interface (append-only JSONL v1, upgradeable to tamper-evident) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0042`
- AC-2: Each coordination decision logged (directive id, grade summary, rule fired, action, timestamp) · impact:`local` · seam:`integration` · scenario:`SCENARIO-0042`
- AC-3: All D4 ladder transitions logged to decision log · impact:`local` · seam:`integration` · scenario:`SCENARIO-0042`
- AC-4: Writer interface is swappable without coordination logic changes · impact:`local` · seam:`unit` · scenario:`SCENARIO-0042`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:222-228`

**Status:** done:ITER-0001 (D6 decision log AC-1..4: log type in T1, decisions emitted from the D4 loop in T7)

## STORY-0057

**Epic:** EPIC-008 — Coordination loop & escalation
**Title:** Daemon claims next directive from queue

**As a** fleet daemon
**I want** to atomically claim the next available directive with priority and lease enforcement
**So that** multiple workers do not process the same directive and work is fairly distributed by priority

**Acceptance criteria:**
- AC-1: Daemon retrieves directive with highest priority in queue · impact:`local` · seam:`unit` · scenario:`JOURNEY-0001`
- AC-2: Lease is acquired atomically to prevent concurrent claims · impact:`cross-surface` · seam:`integration` · scenario:`JOURNEY-0001`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:312`

**Status:** done:ITER-0000

## STORY-0058

**Epic:** EPIC-008 — Coordination loop & escalation
**Title:** Execute coordination loop with escalation ladder on failure

**As a** fleet daemon
**I want** to coordinate pass/fail outcomes and escalate failures through a tiered retry strategy
**So that** transient failures are retried with stronger resources and human intervention is triggered for hard failures

**Acceptance criteria:**
- AC-22: Pass result routes directive to done state · impact:`local` · seam:`unit` · scenario:`JOURNEY-0001`
- AC-23: Fail result triggers escalation ladder: retry-same → stronger-worker → bigger-hard-tier → human-escalations · impact:`cross-surface` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-24: Each retry is re-pushed by Temporal with backoff · impact:`cross-surface` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-25: Fresh handoff bundle is provided with each retry · impact:`cross-surface` · seam:`integration` · scenario:`JOURNEY-0001`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:322-324`

**Status:** partial:ITER-0000+0001+0004 (AC-22 done ITER-0000, AC-23 synchronous ladder done ITER-0001, **AC-25 done:ITER-0004** — daemon emits a fresh handoff bundle on each autonomous requeue, evidence SCENARIO-0054 daemon seam); AC-24 Temporal→ITER-0007

## STORY-0059

**Epic:** EPIC-008 — Coordination loop & escalation
**Title:** Deterministic coordination loop

**As a** fleet orchestrator
**I want** to claim, lease, requeue, and park tasks deterministically against a temp queue DB
**So that** the coordination loop produces repeatable behavior under controlled conditions

**Acceptance criteria:**
- AC-1: claim rule: task transitions from unowned to owned by a single daemon instance · impact:`local` · seam:`unit` · scenario:`SCENARIO-0070`
- AC-2: lease rule: owned task extends its ownership window without losing intermediate state · impact:`local` · seam:`unit` · scenario:`SCENARIO-0070`
- AC-3: requeue rule: task returns to unowned queue on daemon release · impact:`local` · seam:`unit` · scenario:`SCENARIO-0070`
- AC-4: park rule: task enters durable hold state pending manual intervention · impact:`local` · seam:`unit` · scenario:`SCENARIO-0070`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:395`

**Status:** done:ITER-0001 (claim/lease/requeue/park AC-1..4; park added in T3)

## STORY-0060

**Epic:** EPIC-008 — Coordination loop & escalation
**Title:** Graceful container teardown without regression

**As a** daemon operator
**I want** to stop-then-delete containers with a bounded timeout, routing stop-timeout to the reaper without blocking the loop
**So that** the delete-hang regression does not reoccur and the daemon remains responsive

**Acceptance criteria:**
- AC-1: stop command completes within configured timeout or routes to reaper immediately · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0075`
- AC-2: daemon loop continues processing other tasks while reaper handles the stalled stop · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0075`
- AC-3: delete-hang regression test passes: stop-timeout never blocks the coordination loop · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0075`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:398-399`

**Status:** partial:ITER-0000 (AC-1/AC-3 stop-then-delete done, validated on cluster); AC-2 async-reaper + automated delete-hang regression test→ITER-0001

## STORY-0061

**Epic:** EPIC-008 — Coordination loop & escalation
**Title:** Escalation ladder climbing

**As a** orchestrator
**I want** to climb pre-approved escalation rungs autonomously, land privileged/judgment escalations in the escalations lane without blocking other lanes, and resurface stale escalations when urgency rises
**So that** critical decisions get human attention without deadlocking the workflow

**Acceptance criteria:**
- AC-1: autonomous climb: low-cost escalation rungs are approved and executed without human intervention · impact:`local` · seam:`integration` · scenario:`SCENARIO-0085`
- AC-2: privileged/judgment escalations land in escalations lane and do not block other workflow lanes · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0085`
- AC-3: stale escalation re-surface: when urgency rises, old escalations are resurfaced in priority order · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0085`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:413-415`

**Status:** partial:ITER-0001 (AC-1 autonomous climb + AC-2 non-blocking human lane done); AC-3 urgency resurface→ITER-0007