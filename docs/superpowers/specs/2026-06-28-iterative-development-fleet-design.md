# Fleet-Native Iterative Development — Design

**Status:** design (brainstormed + approved 2026-06-28). A fleet-dispatching variant of the
`iterative-development` orchestrator. Builds on the budget-aware worklist orchestrator umbrella
([design](2026-06-26-budget-aware-worklist-orchestrator-design.md)); consumes the usage meter
(sub-project 1) and the reserve cap (sub-project 2). It is, in effect, the iterative-development
*producer* + *supervisor* specialized to the iterative-development process.

## Problem

The `iterative-development` orchestrator runs every unit of work — requirement extraction,
implementation, and the parallel-adversarial-review (PAR) gates — as Claude subagents in the
interactive session. That makes a long autonomous run consume the Claude Max quota that should be
reserved for interactive work, and serializes the loop on one session. We want a variant that
offloads the grind to the agent-sandbox fleet (laneq queue → ephemeral worker containers →
oracle external-grading) while **preserving PAR as the gate that completes an iteration**, and
without spending the Claude Max quota on fleet implementation.

## Goal & success criteria

- A new standalone skill, `iterative-development-fleet`, runs as a **thin local Claude
  coordinator**: it reuses the existing iterative-development artifacts, turns each iteration's
  tasks into fleet directives, enqueues them to laneq, polls for graded results, and advances the
  loop. No in-session Claude subagents do implementation work.
- **Per-task gate:** the held-out **oracle** external-grade on a clean checkout (existing
  STORY-0068 path) decides a single task passed.
- **Per-iteration gate (PAR, preserved):** an iteration is marked complete only when a **parallel
  adversarial review panel** approves its behavior evidence — independent reviewers hunting for
  coverage gaps, weak evidence, and boxing-in across the iteration's corpus. Green oracle grades
  alone do NOT complete an iteration.
- **Quota isolation:** fleet implementation spends **zero Claude Max implementer tokens**;
  per-provider spend is visible in the usage meter and bounded by the reserve cap.
- One iteration runs end-to-end on the fleet as acceptance (see §Acceptance).

## Decisions locked during brainstorming

- **Standalone skill**, sibling to `iterative-development` — reuses the same artifact files
  (`requirements/`, `roadmap.md`, `behavior-scenarios.md`, `behavior-corpus.md`,
  `iteration-log.md`, `progress.md`); leaves the proven in-session skill untouched.
- **Thin local coordinator** (not a cluster-resident service). Re-hostable on the cluster later
  (umbrella Approach B / sub-project 5) behind the same contracts.
- **Two-level quality:** deterministic oracle per task; PAR panel per iteration completion.
- **PAR runs as a fleet strong-model panel, budget-managed.** Reviewers are dispatched as fleet
  directives — a provider-diverse panel of strong models (codex/openai, other strong providers,
  and Claude *only within the reserve cap*). Provider diversity strengthens adversarial review;
  panel spend is metered per provider. Iteration completes only on panel approval.
- **Executor tiers (quota-aware routing):**
  - **ollama-local is the unlimited floor.** It has no token quota and runs indefinitely, so it is
    the default workhorse for fleet *implementation* tasks. The loop never fully stalls.
  - **Quota-bearing strong providers** (codex/openai, ollama-cloud, Claude-within-reserve) are
    capable but metered + reserve-capped per provider — each its own managed budget. Used for the
    PAR panel and for hard implementation tasks the local floor cannot carry. A provider over its
    reserve cap is skipped.

## Architecture

```
roadmap iteration ─► COORDINATOR (local Claude skill)
                       │  1. emit one directive per task {repo, ref, task, hidden-oracle}
                       │     provider := pickImplementer(budget)   // default ollama-local
                       ▼
                     laneq queue ─► dispatcher ─► ephemeral worker (chosen provider via llm-proxy)
                       │                              │ implement
                       │                              ▼
                       │                         ORACLE external-grade (clean checkout)
                       ▼  2. poll grades; pass→story done, fail→requeue/escalate
                     iteration tasks all graded
                       │  3. PAR GATE: dispatch strong-model review panel (fleet directives,
                       │     provider-diverse, within reserve cap) over the iteration corpus
                       ▼
                     panel approves? ── no ─► append gap stories, revise roadmap, continue
                       │ yes
                       ▼  mark iteration complete; update iteration-log.md / progress.md
```

## Components

- **Coordinator (the skill).** Reuses iterative-development's extract/scope outputs. Per iteration:
  decompose into task directives, enqueue, poll, advance. Pure orchestration — no implementation or
  review logic of its own; it composes the fleet + the PAR panel.
- **Implementer router** — `pickImplementer(taskDifficulty, budgetSnapshot) → provider`. Defaults to
  `ollama-local`; escalates to a quota-bearing provider with headroom for hard tasks; never selects a
  provider over its reserve cap. Reads the meter (sub-project 1) for budget state.
- **Oracle gate** — reuses the dispatcher's external-grading path (`--external-grading`); the
  authoritative per-task verdict.
- **PAR panel** — dispatches K independent reviewer directives (provider-diverse strong models) over
  the iteration's behavior corpus, aggregates verdicts (majority/▶ any-blocking per the PAR rubric),
  and returns approve / gaps. Reviewer provider selection is budget-managed; Claude is eligible only
  within the reserve cap.
- **Artifacts** — unchanged schema from iterative-development; this skill is a different *engine* over
  the same files, so a run can be inspected/resumed with the existing tooling.

## Data flow / error handling

1. Coordinator reads the next pending roadmap iteration, emits one directive per task (provider
   chosen by `pickImplementer`).
2. Dispatcher runs each in an ephemeral worker; oracle grades on a clean checkout.
3. Coordinator polls: `pass → mark story done`; `fail → requeue with bounded retries → escalate to
   the durable escalation lane` for a later session.
4. When all of the iteration's tasks are graded, run the **PAR gate**. Approve → mark iteration
   complete + update artifacts. Reject → append gap stories/scenarios, revise the roadmap, continue.
5. **Budget exhaustion is a normal event, not an error.** Implementation falls back to the
   ollama-local floor when paid quotas are spent. **PAR integrity is preserved over speed:** the
   panel requires strong models, so if that budget is exhausted the iteration *completion* parks
   until the window resets (it does NOT complete on a degraded review) while implementation keeps
   progressing on the floor.
6. **Crash-safe / resumable:** all state lives in the artifact files + the laneq queue + the
   escalation lane — the same resumability guarantee as the in-session skill. Re-invoking the skill
   resumes from the artifacts.

## Acceptance

Drive one real iteration end-to-end purely on the fleet: a roadmap iteration's tasks are enqueued,
implemented by fleet workers (defaulting to ollama-local, **zero Claude-Max implementer tokens**),
oracle-graded, then gated by a strong-model PAR panel; a story flips to `done` only after panel
approval, and per-provider PAR spend is visible in `dispatcher usage`.

## Out of scope (v1)

- Cluster-resident orchestrator (umbrella Approach B / sub-project 5).
- The producer for non-iterative-development worklists (umbrella sub-project 3 generally).
- Tiered escalation beyond a single PAR panel (e.g., escalate-on-disagreement to a larger panel).
- Any change to the in-session `iterative-development` skill.
