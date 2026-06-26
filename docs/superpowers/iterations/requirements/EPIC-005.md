# EPIC-005 — Prioritization & scheduling (Temporal)

**Summary:** Prioritization & scheduling (Temporal)
**Stories:** STORY-0035, STORY-0036, STORY-0037, STORY-0038, STORY-0039, STORY-0040, STORY-0041, STORY-0042, STORY-0043, STORY-0044, STORY-0045, STORY-0046, STORY-0047
**Primary sources:** `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`, `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 13/13 done (STORY-0040/0042/0045 done:ITER-0007; STORY-0041/0043/0044/0046/0047 done:ITER-0007b — live Temporal ACs proven (E1), 0043 AC-2 with documented wall-clock-aging limitation; STORY-0035/0036/0037/0038/0039 done:ITER-0008b — provider routing/cost, budget guardrails, thread aging, multi-repo)
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

**Seam note (PAR scope review 2026-06-25):** AC-4's tokens/latency/spend are captured ON the Run from the
worker `Result` payload (the runner reports usage it already collected), NOT pulled from a live provider API.
The seam is therefore genuinely `unit`: tests populate a `Result` with usage fields and assert the Run records
them. A live-provider feed (llm-proxy usage headers) is future enrichment, out of scope here.

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:164-176, 309-314, 358-416`

**Status:** done:ITER-0008b (AC-1/2 done:ITER-0008 T2b — Run.provider_instance/model_id/budget_snapshot, SCENARIO-0121; AC-3 explicit model→instance resolution, no-guess + AC-4 tokens/latency/spend captured from the worker Result done:ITER-0008b TG2; SCENARIO-0016)

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

**Guardrail-semantics reconciliation (PAR scope review 2026-06-25):** AC-2 ("protected unless explicitly
human-approved") and STORY-0032 AC-3 ("hard budget guardrails remain protected from mutation") are
consistent under one distinction: a **hard budget ceiling** can NEVER be raised by the autonomous genome
mutation engine (STORY-0032's protected-invariant hard block, `genome-pattern-detection.md` §4) — only by an
explicit **operator action through the TUI** (STORY-0028). "Human-approved" therefore means an operator edit,
not an autonomous promotion. In a Mac-off scenario (no operator), hard guardrails stay fixed — the safe
default. The tunable `budget_escalation` *heuristic* (when to escalate to a cheaper/stronger model under cost
pressure) is a legitimate mutation target; the hard *ceiling* fields are not.

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:387-398, 455-463`

**Status:** done:ITER-0008b (TG3 — BudgetPolicy with 6 levels (AC-1; per-provider/worker-class/thread/run/message
enforced, per-time-window documented-deferred), hard-ceiling protected from auto-mutation with an operator-only
raise path via the TUI (AC-2), thread-keyed spend aggregation → escalate/reject on exceed (AC-3); SCENARIO-0022)

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

**Temporal-decoupling note (PAR scope review 2026-06-25):** AC-2/AC-4 are **daemon-local, NOT Temporal-coupled**.
The Temporal-side wall-clock *urgency aging* (Q2→Q1) is STORY-0043 (done:ITER-0007b). STORY-0037 is the distinct
thread-registry concern: a deterministic queue-class + aging_score ordering computed locally (e.g.
`resurface if thread.last_served < now - staleThreshold` → boost effective priority). It consumes an injected
clock like the rest of the codebase; no Temporal workflow dependency. CI-provable.

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:150-151, 506-519`

**Status:** done:ITER-0008b (TG5 — Thread.priority/aging_score/queue_class + 4 queue classes (AC-1/AC-3),
deterministic priority+aging ordering with injected clock (AC-2), stale-thread resurfacing with a live
MarkServed lifecycle wired into daemon completion (AC-4); SCENARIO-0017)

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

**Status:** done:ITER-0008b (TG2 — 6 named provider instances (AC-1), cheap-local-first→stronger-cloud escalation
rules + production EscalateRun (AC-2), deterministic multi-signal selector (AC-3); provider-taxonomy made coherent
with worker-env routing; SCENARIO-0016)

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

**Status:** done:ITER-0008b (TG6 — Thread.repo_refs (AC-1), simultaneous multi-repo workspace claims via the real
WorkspaceRegistry (AC-2), deterministic LRU repo-fairness scheduler proven under skew with a live MarkRepoServed
wiring into daemon completion to prevent starvation (AC-3); SCENARIO-0126. Daemon claim-order repo-fairness is a
documented follow-on boundary.)

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
- `docs/plans/2026-06-18-fleet-orchestration-design.md:250-263`

**Status:** done:ITER-0007 — AC-1/AC-2 (importance static vs urgency deriving from (deadline,now)) +
AC-3 (Q1/Q2/Q3/Q4 quadrant mapping stable) proven pure-Go in `modules/incus-dispatcher/temporal/projection.go`
(`ComputeUrgency`/`ComputeQuadrant`/`ComputeEffectivePriority`), evidence SCENARIO-0078 (fake-clock, 8-day
timeline, Q4 stability + deadline aging).

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
- `docs/plans/2026-06-18-fleet-orchestration-design.md:265-271`

**Status:** partial:ITER-0006+ITER-0007→**done:ITER-0007b** (AC-3 done; **AC-1, AC-2 done:ITER-0007b — live sole writer of effective-priority + not-before (C2/C3 + E1); ITER-0008 GATE met**) — AC-3 (laneq.next returns
highest-importance eligible item, no urgency knowledge) proven ITER-0006 SCENARIO-0091/0092 and re-asserted
here against the temporal projection seam. **AC-1/AC-2 (Temporal is the live sole writer of effective-priority
+ not-before; re-projects on rescore over the real laneq gRPC seam) → ITER-0007b** (cluster, live Temporal).

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
- `docs/plans/2026-06-18-fleet-orchestration-design.md:273-281`

**Status:** done:ITER-0007 — AC-1 (human rescore accepts any (importance,deadline), Temporal re-projects),
AC-2 (agent rescore bounded; big/privileged jumps require approval), AC-3 (drifting agent cannot self-promote
to Q1/P0) proven in `modules/incus-dispatcher/temporal/authority.go` (`IsHumanUnrestricted`/`IsAgentBounded`),
evidence SCENARIO-0057 (rescore-authority unit/integration) + SCENARIO-0082 (drifting-agent prevention).

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
- `docs/plans/2026-06-18-fleet-orchestration-design.md:283-285`

**Status:** partial:ITER-0007→**done:ITER-0007b** (AC-1, AC-3 done; **AC-2 done:ITER-0007b — Q2→Q1 CI-PROVEN (C2 time-skip); live durable-timer+gRPC reproject mechanism (E1); NOTE live wall-clock Q2→Q1 not compressible to seconds (urgency calibrated in days) → urgency knob / multi-day runner deferred to ITER-0008/ops**) — AC-1 (urgency monotonic in (deadline,now))
+ AC-3 (no-deadline low-importance Q4 never ages up) proven pure-math/fake-clock in
`modules/incus-dispatcher/temporal/projection.go`, evidence SCENARIO-0078. **AC-2 (Q2→Q1 promotion over
wall-clock time, no human intervention — `journey`/`integration`) → ITER-0007b** (live Temporal wall-clock aging).

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
- `docs/plans/2026-06-18-fleet-orchestration-design.md:287-289`

**Status:** partial:ITER-0000+ITER-0006+ITER-0007→**done:ITER-0007b** (AC-1, AC-2 done; **AC-3 done:ITER-0007b — Temporal is the live sole caller of Defer/Reprioritize (C2 seam + E1 0093); ITER-0008 GATE met**) [AC-3 logic done:ITER-0007 mock laneq],
live gRPC → ITER-0007b) —
**Discovery: laneq already ships `not_before` + `blocked_by` deferral upstream (v0.4.0 + #18)**, so
ITER-0006 VALIDATED + INTEGRATED rather than added it. AC-1 (not_before field) + AC-2 (`next` filters
not_before<=now then highest importance) done:ITER-0006 via the gRPC adapter + SCENARIO-0091 (fake CI)
+ SCENARIO-0092 (real-wire @2d1b59e). **AC-3 (Temporal sole writer) logic done:ITER-0007** — the
single-writer-as-sole-caller contract is proven against a mock laneq via the `GuardedDirective` writer-role
guard (`temporal/writer.go`); **live Temporal-as-sole-caller over the real laneq gRPC `Defer`/`Reprioritize`
seam → ITER-0007b**.

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
- `docs/plans/2026-06-18-fleet-orchestration-design.md:453-457`

**Status:** done:ITER-0007 — AC-1 (deadline approaching promotes Q2→Q1 within threshold), AC-2 (no-deadline
low-importance stays Q4 idle-only), AC-3 (laneq.next returns highest-importance eligible only) proven
deterministically in `modules/incus-dispatcher/temporal/projection.go`, evidence SCENARIO-0078 (fake-clock
8-day timeline). Projection is pure and deterministic (same inputs → same quadrant/priority).

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
- `docs/plans/2026-06-18-fleet-orchestration-design.md:456-457`

**Status:** partial:ITER-0007→**done:ITER-0007b** (AC-1 done; **AC-2 done:ITER-0007b — concurrent-read consistency under single writer (C5 both-fields -race + E1 live Peek)**) — AC-1 (only Temporal can write effective-priority
+ not-before; no other actor) proven by the `GuardedDirective` writer-role guard
(`modules/incus-dispatcher/temporal/writer.go`: private fields + role-checked setters reject Queue/Human writes),
evidence SCENARIO-0081 (single-writer guard, unit/integration + code-review). Single-writer is process-level and
**orthogonal to laneq lease exclusivity** (ITER-0006 proved real laneq leases are non-exclusive). **AC-2
(concurrent reads consistent under live Temporal across daemon instances) → ITER-0007b.**

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
- `docs/plans/2026-06-18-fleet-orchestration-design.md:458-460`

**Status:** partial:ITER-0007→**done:ITER-0007b** (AC-2, AC-3 done; **AC-1 done:ITER-0007b — live human rescore moves item to any bucket via deployed Temporal (E1: laneq P1→P0)**) — AC-2 (agent-proposed rescore beyond bound
rejected with reason logged) + AC-3 (privileged-implication rescore routes to approval queue, reusing the
ITER-0001 escalation lane) proven in `modules/incus-dispatcher/temporal/authority.go`, evidence SCENARIO-0082.
**AC-1 (live human rescore moves an item to any bucket via the deployed Temporal rescore path) → ITER-0007b.**