# EPIC-010 — Directive contract & queue substrate

**Summary:** Directive contract & queue substrate
**Stories:** STORY-0064
**Primary sources:** `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 0/1 done

## STORY-0064

**Epic:** EPIC-010 — Directive contract & queue substrate
**Title:** Directive carries intent + template + origin + importance + deadline + lane/repo/ref/task + optional handoff + optional grade + max_attempts

**As a** daemon
**I want** a structured JSON directive body with all required and optional fields for task execution
**So that** the executor can interpret intent and apply the template without ambiguity

**Acceptance criteria:**
- AC-1: Directive must include intent field (string describing the goal) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-2: Directive must include template field (PROPOSED; daemon validates vs allowlist + origin) · impact:`local` · seam:`integration` · scenario:`SCENARIO-0045`
- AC-3: Directive must include origin field (set by daemon, not author; values: orchestrator | worker:<id>) · impact:`cross-surface` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-4: Directive must include importance field (INPUT: how much it matters, author-set, NOT effective priority) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-5: Directive must include deadline field (INPUT, optional, drives urgency, absent => never urgent, Q4-eligible) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-6: Directive must include lane field (work lane identifier) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-7: Directive must include repo field (git bundle URL or path) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-8: Directive must include ref field (git reference to check out) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-9: Directive must include task field (literal brief or pointer to brief file in template) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-10: Directive may include handoff_in field (optional lean-ctx bundle to import) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-11: Directive may include grade field (optional; presence indicates authoritative external grade with oracle_ref, cmd, expect) · impact:`local` · seam:`integration` · scenario:`SCENARIO-0045`
- AC-12: Directive must include max_attempts field (number of retry attempts allowed) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-13: Directive MUST NOT include access_cmd field (reserved; template defines execution) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-14: Directive MUST NOT include root field (reserved; template defines execution) · impact:`local` · seam:`unit` · scenario:`SCENARIO-0045`
- AC-15: importance + deadline are inputs to Temporal for projection, not the effective schedule · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0045`
- AC-16: Agents may only PROPOSE changes to importance/deadline; humans set them freely · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0045`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:276-308`

**Status:** partial:ITER-0006 (AC-1..AC-14 done; AC-15/AC-16 → ITER-0007) — the Go `Directive` struct
already carries every field; ITER-0006 activated `ParseDirective` at the laneq opaque-`body` JSON boundary
and proves **AC-1..AC-14** done:ITER-0006 (all required/optional fields present; access_cmd/root rejected via
`DisallowUnknownFields`) via SCENARIO-0045 (unit, 22 AC-mapped sub-tests). AC-2's *template-vs-allowlist+origin validation*
half is ALREADY proven by the ITER-0002 D1 `ValidateTemplate` (`policy.go:35`,
`policy_test.go`/`scenario_d1_test.go`) — cited, not re-proven (PAR re-review B-critical resolved).
**AC-15/AC-16 → ITER-0007** (importance/deadline as
Temporal *projection inputs*, and the agents-may-only-PROPOSE-vs-humans-set-freely authority — both
cross-surface, requiring Temporal). Story = PARTIAL after ITER-0006; closes in ITER-0007.