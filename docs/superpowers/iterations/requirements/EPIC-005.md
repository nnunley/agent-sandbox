# EPIC-005 — Prioritization & scheduling (Temporal)

**Summary:** Prioritization & scheduling (Temporal)
**Stories:** STORY-0035, STORY-0036, STORY-0037, STORY-0038, STORY-0039, STORY-0040, STORY-0041, STORY-0042, STORY-0043, STORY-0044, STORY-0045, STORY-0046, STORY-0047
**Primary sources:** `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`, `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 0/13 done
## STORY-0035

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Apply provider instance and model policy to each run

**As a** coordinator
**I want** to track which provider instance and model was used for each run
**So that** cost and quality outcomes can be measured and policies evolved

**Acceptance criteria:**
- AC-1: Run object includes provider_instance and model_id fields · impact:`local` · seam:`unit` · scenario:`SCENARIO-0016`
- AC-2: Run object includes budget_snapshot capturing budget at start of run · impact:`local` · seam:`unit` · scenario:`SCENARIO-0016`
- AC-3: System resolves model name to explicit provider instance, not by guessing · impact:`local` · seam:`unit` · scenario:`SCENARIO-0016`
- AC-4: Run object captures tokens, latency, and spend for provider/model accounting · impact:`local` · seam:`unit` · scenario:`SCENARIO-0016`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:164-176, 309-314, 358-416`

**Status:** pending

## STORY-0036

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Enforce budget guardrails at multiple levels

**As a** coordinator
**I want** budgets to be tracked and enforced at message, run, thread, worker, provider, and time-window levels
**So that** runaway costs are prevented and resource allocation is controlled

**Acceptance criteria:**
- AC-1: Budget policy object supports levels: per-message, per-run, per-thread, per-worker-class, per-provider, per-time-window · impact:`local` · seam:`unit` · scenario:`SCENARIO-0022`
- AC-2: Budget guardrails remain protected from automatic mutation unless explicitly human-approved · impact:`local` · seam:`unit` · scenario:`SCENARIO-0022`
- AC-3: When budget threshold is exceeded, run escalates or is rejected · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0022`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:387-398, 455-463`

**Status:** pending

## STORY-0037

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Prioritize and age threads to prevent starvation

**As a** coordinator
**I want** to actively surface stale high-value threads so active work doesn't consume all attention
**So that** long-term ideas are not forgotten

**Acceptance criteria:**
- AC-1: Thread object includes priority and aging_score fields · impact:`local` · seam:`unit` · scenario:`SCENARIO-0017`
- AC-2: Queue ordering is by priority plus aging · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0017`
- AC-3: System supports multiple queue classes: urgent, active, incubating, maintenance · impact:`local` · seam:`unit` · scenario:`SCENARIO-0017`
- AC-4: System resurfaces long-dormant but valuable threads via stale-thread resurfacing policy · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0017`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:150-151, 506-519`

**Status:** pending

## STORY-0038

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Support multiple provider instances for fallback and cost optimization

**As a** coordinator
**I want** to route work to providers configured as explicit instances
**So that** I can try cheap models first and escalate on failure

**Acceptance criteria:**
- AC-1: Provider instances include: claude-code-main, openai-codex-main, openrouter-main, ollama-cloud, ollama-local, vllm-local · impact:`local` · seam:`unit` · scenario:`SCENARIO-0016`
- AC-2: Escalation rules encode: try cheap local model first, escalate to stronger cloud model on failure/uncertainty · impact:`local` · seam:`unit` · scenario:`SCENARIO-0016`
- AC-3: Model selection uses task type, worker type, policy type, quality tier, latency, cost, context size, historical success rate · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0016`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:362-385`

**Status:** pending

## STORY-0039

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Coordinate work across multiple repositories

**As a** coordinator
**I want** to manage threads that span multiple repositories
**So that** cross-repo coordination is preserved

**Acceptance criteria:**
- AC-1: Thread object includes repo_refs array · impact:`local` · seam:`unit`
- AC-2: Thread can have multiple active repos checked out simultaneously · impact:`cross-surface` · seam:`integration`
- AC-3: System prevents repo starvation by scheduling work across repo set · impact:`cross-surface` · seam:`integration`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:37-38, 141, 519`

**Status:** pending

## STORY-0040

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Disentangle priority into orthogonal axes: importance and urgency

**As a** Temporal scheduler
**I want** to separate importance (author-set, static) from urgency (deadline-derived, dynamic)
**So that** one axis doesn't conflate multiple concerns and scheduling logic remains deterministic

**Acceptance criteria:**
- AC-1: Directive has two orthogonal input fields: importance (tier) and deadline/urgency intent · impact:`local` · seam:`unit`
- AC-2: Importance is static until explicitly rescored; urgency derives from (deadline, now) and changes autonomously · impact:`local` · seam:`unit`
- AC-3: Quadrant mapping is stable: Q1 (important+urgent)→run now; Q2 (important+not-urgent)→schedule; Q3 (not-important+urgent)→soon; Q4 (not-important+not-urgent)→idle-only · impact:`local` · seam:`unit`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:231-247`

**Status:** pending

## STORY-0041

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Temporal is the single writer of effective priority and not-before eligibility

**As a** laneq provisioner
**I want** Temporal to be the sole writer projecting (importance, urgency) → (effective priority, not-before) and re-evaluating over time
**So that** there is no contention and laneq only needs to pick the highest-importance eligible item

**Acceptance criteria:**
- AC-1: Temporal computes effective priority and not-before from directive inputs; no other actor writes these fields · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0056`
- AC-2: When inputs (importance or deadline) change via rescore, Temporal re-projects effective priority and not-before deterministically · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0056`
- AC-3: laneq.next returns the highest-importance item whose not-before time has passed, with no knowledge of urgency logic · impact:`local` · seam:`unit` · scenario:`SCENARIO-0056`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:248-254`

**Status:** pending

## STORY-0042

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Human rescore is unrestricted; agent rescore is bounded; both trigger Temporal re-projection

**As a** priority manager
**I want** humans to rescore any item to any quadrant without restriction, and agents to propose bounded rescores with approval gates for big jumps
**So that** humans retain override authority while preventing agents from self-promoting to Q1/P0 and dominating the fleet

**Acceptance criteria:**
- AC-1: Human rescore accepts any (importance, deadline) and updates directive immediately; Temporal re-projects · impact:`local` · seam:`integration` · scenario:`SCENARIO-0057`
- AC-2: Agent rescore is bounded: agent may propose rescore via D1 dispose, but big jumps or privileged implications require approval · impact:`local` · seam:`integration` · scenario:`SCENARIO-0057`
- AC-3: A drifting agent cannot self-promote to Q1/P0 without explicit bounds being exceeded and blocked · impact:`local` · seam:`unit` · scenario:`SCENARIO-0057`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:256-263`

**Status:** pending

## STORY-0043

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Deadline-driven starvation prevention: items with deadlines age up autonomously

**As a** Temporal scheduler
**I want** items with deadlines to automatically increase in urgency as the deadline nears, moving from Q2 toward Q1
**So that** time-sensitive work is never starved and runs before the deadline expires

**Acceptance criteria:**
- AC-1: Urgency is a monotonic function of (deadline, now); as now approaches deadline, urgency increases deterministically · impact:`local` · seam:`unit` · scenario:`SCENARIO-0056`
- AC-2: An item in Q2 with a deadline will eventually be promoted to Q1 as deadline nears, without human intervention · impact:`journey` · seam:`integration` · scenario:`SCENARIO-0056`
- AC-3: An item with no deadline and low importance (Q4) remains idle-only by design and never ages up · impact:`local` · seam:`unit` · scenario:`SCENARIO-0056`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:266-268`

**Status:** pending

## STORY-0044

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** laneq extends substrate with not-before eligibility gate

**As a** laneq
**I want** a new field not-before on directives to gate eligibility, so next means highest importance among the eligible
**So that** Temporal can defer items without changing the priority ordering, enabling delayed-start scheduling

**Acceptance criteria:**
- AC-1: Directive model includes not-before timestamp field · impact:`local` · seam:`unit`
- AC-2: laneq.next filters to items where not-before <= now, then returns highest-importance item from eligible set · impact:`local` · seam:`unit`
- AC-3: Temporal is the sole writer of not-before; no other actor modifies it · impact:`local` · seam:`integration`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:270-272`

**Status:** partial:ITER-0000+ITER-0006 (AC-1, AC-2 done; AC-3 → ITER-0007) —
**Discovery: laneq already ships `not_before` + `blocked_by` deferral upstream (v0.4.0 + #18)**, so
ITER-0006 VALIDATED + INTEGRATED rather than added it. AC-1 (not_before field) + AC-2 (`next` filters
not_before<=now then highest importance) done:ITER-0006 via the gRPC adapter + SCENARIO-0091 (fake CI)
+ SCENARIO-0092 (real-wire @2d1b59e). **AC-3 (Temporal sole writer) → ITER-0007** — Temporal becomes
the sole caller of the laneq gRPC `Defer`/`Reprioritize` seam built in ITER-0006; closes in ITER-0007.

## STORY-0045

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Prioritization with temporal projection

**As a** scheduler
**I want** to project (importance, deadline) tuples to effective priority + not-before gates deterministically
**So that** tasks are scheduled fairly and escalation thresholds trigger predictably

**Acceptance criteria:**
- AC-1: deadline approaching promotes item from Q2 to Q1 within acceptable threshold · impact:`local` · seam:`unit` · scenario:`SCENARIO-0078`
- AC-2: no-deadline low-importance item stays in Q4 (idle-only) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0078`
- AC-3: laneq next returns highest-importance *eligible* item only · impact:`local` · seam:`unit` · scenario:`SCENARIO-0078`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:405-409`

**Status:** pending

## STORY-0046

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Single-writer priority update invariant

**As a** daemon operator
**I want** to enforce that only Temporal writes effective priority and not-before fields
**So that** priority decisions are not corrupted by concurrent writes from other actors

**Acceptance criteria:**
- AC-1: no actor other than Temporal can write effective priority or not-before · impact:`local` · seam:`integration` · scenario:`SCENARIO-0081`
- AC-2: concurrent reads of priority by multiple daemon instances are consistent · impact:`local` · seam:`integration` · scenario:`SCENARIO-0081`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:409`

**Status:** pending

## STORY-0047

**Epic:** EPIC-005 — Prioritization & scheduling (Temporal)
**Title:** Authority-based rescoring

**As a** system operator
**I want** to allow humans to rescore any item to any bucket, reject agent-proposed rescores beyond their bound, and route privileged rescores to approval
**So that** priority decisions reflect operational intent and agents cannot escalate without human oversight

**Acceptance criteria:**
- AC-1: human rescore moves item to any bucket without restriction · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0082`
- AC-2: agent-proposed rescore beyond its bound is rejected with reason logged · impact:`local` · seam:`unit` · scenario:`SCENARIO-0082`
- AC-3: agent-proposed rescore with privileged implication routes to approval queue · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0082`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:410-412`

**Status:** pending