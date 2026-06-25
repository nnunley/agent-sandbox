# Genome Pattern-Detection & Mutation-Proposal Design (ITER-0008b, STORY-0032 AC-4)

**Date:** 2026-06-25
**Status:** design note (Task-0, BLOCKING for STORY-0032 AC-4)
**Owners:** STORY-0032 (safe/auditable genome mutation), builds on STORY-0031 (StumbleSignal),
STORY-0054 (audit data layer).

## Why this note exists

STORY-0032 AC-4 requires the full mutation flow: **detect pattern → propose → trial/experiment →
measure outcome → promote/keep/reject/revert**. The PAR scope review (2026-06-25, both reviewers,
CRITICAL boxing-in) flagged that "detect pattern" is under-specified: without a precise definition of
*what a stumble-pattern repeat is*, *who detects it*, and *the mutation-proposal format*, STORY-0032
AC-4 is untestable and the mutation engine risks coupling to ad-hoc heuristics. This note locks those
three things so the story can be decomposed into TDD tasks.

It deliberately reuses primitives that already exist in the codebase rather than inventing new ones:
- `StumbleSignal{Type, Ts, RunID, EvidenceSummary}` and the `StumbleType` enum (`run.go:34-55`).
- `Run.StumbleSignals []StumbleSignal` (`run.go:74`).
- The `AuditLog` interface with `ByThread`/`ByRun`/`Entries` and `AuditKindMutation` (`audit.go`).
- `ArtifactMutationProposal` artifact kind (`run.go:15`).

## 1. What a "stumble-pattern repeat" is

A **stumble pattern** is a *recurrence of the same `StumbleType` across distinct recent runs within a
bounded window*. It is defined by exactly three parameters:

| Parameter | Meaning | Default |
|---|---|---|
| `signalType` | the `StumbleType` being counted (e.g. `timeout`) | — (one detector per type, or a wildcard scan) |
| `threshold`  | minimum count of distinct runs exhibiting `signalType` to fire | 3 |
| `window`     | look-back bound; either a wall-clock duration **or** a run-count | last 10 runs **or** 1h, whichever is tighter |

**Precise firing rule.** Given an ordered list of recent runs (newest first) scoped to a detection
domain (see §2), a pattern of `signalType` **fires** when:

```
count( distinct run R in window where R.StumbleSignals contains a signal of type signalType )
    >= threshold
```

Notes that make this testable and deterministic:
- **Distinct runs, not distinct signals.** Three timeouts in one run is one data point, not three.
  A pattern is about *repetition across attempts*, matching SCENARIO-0018 ("Three recent runs all
  failed with timeout").
- **Window is a half-open interval** `[now-window, now]` by run-count or by `Ts`; the caller supplies
  `now` (clock injection, matching the project convention — `AuditLog.now`, `AddStumble` Ts-as-given).
- **De-dup of proposals.** A pattern that has already produced an *open* (candidate/trial) mutation
  proposal for the same `(domain, signalType)` does NOT fire again until that proposal is resolved
  (promoted/rejected/reverted). This prevents proposal storms. The detector consults the genome's
  proposal index keyed by `(domain, signalType)`.

## 2. Detection domain

Patterns are detected **per dispatch domain**, not globally, so a timeout pattern on the
coding-worker genome does not get diluted by unrelated research-worker runs. The domain key is:

```
domainKey = worker_kind            // e.g. "incus-container", "research"
```

(`Run.WorkerKind`, `run.go:67`). v1 keys on `worker_kind` only; `policy_id` refinement is a documented
future extension (YAGNI — not needed for SCENARIO-0018). A wildcard domain (`""`) scans all runs and is
used only by tests / operator-triggered audits.

## 3. Who detects it

A **daemon-local detector task** — `GenomeDetector` — runs the scan. It is a pure function over inputs
(no I/O of its own), so it is unit-testable without the daemon:

```
DetectStumblePatterns(runs []Run, cfg DetectorConfig, now time.Time, openProposals map[ProposalKey]bool)
    -> []StumblePattern
```

- **Inputs:** the recent `Run` slice (the daemon supplies it from its thread/run store — the same
  store STORY-0029/0011 already maintain), the `DetectorConfig{Threshold, Window}`, the injected
  clock, and the set of already-open proposal keys (for de-dup).
- **Invocation point:** the daemon invokes the detector **after each run completes** (right after it
  records `Run` + appends the `AuditKindRun` entry), querying its own in-memory run history. This is the
  natural seam — the daemon already owns run completion. No new background timer is required for v1.
- **Output:** zero or more `StumblePattern{Domain, SignalType, Count, Window, EvidenceRunIDs[]}`.
  `EvidenceRunIDs` are the run IDs that satisfied the rule — these become the proposal's `evidence_refs`
  (SCENARIO-0018: "Mutation evidence_refs links to prior run IDs").

The detector reads from the audit log / run store; it **never mutates**. Proposal generation is a
separate step (§4), keeping detection side-effect-free and easy to test.

## 4. The mutation-proposal format

When a pattern fires, the coordinator generates a **MutationProposal**. This is the genome's candidate
entry (STORY-0032 AC-1 genome schema) in `status=candidate`:

```go
type MutationProposal struct {
    ID           string         // stable id, e.g. "mut-<n>"
    Domain       string         // worker_kind the mutation applies to
    Target       MutationTarget // what is being mutated (enum, AC-2)
    Source       GenomeSource   // "learned" for detector-generated (AC-1)
    Status       GenomeStatus   // "candidate" at creation (AC-1)
    Version      int            // genome version this proposal would supersede
    ContentHash  string         // hash of proposed content (AC-1)
    Rationale    string         // human-readable: which pattern triggered it
    EvidenceRefs []string       // prior run IDs from the firing pattern (AC-4 trail)
    ProposedAt   time.Time
}
```

`MutationTarget` enumerates exactly the six **allowed** categories (AC-2) and nothing else:

```
prompt_tweak | routing_heuristic | provider_model_heuristic |
budget_escalation | execution_policy | thread_handoff_template
```

`GenomeSource` = `bootstrap | learned | promoted | experiment` (AC-1).
`GenomeStatus` = `active | candidate | rejected | reverted` (AC-1).

### Protected invariants (AC-3) — hard block

Proposal generation MUST refuse to target any protected category. The blacklist is a hard guard,
checked at proposal construction and again at promotion:

```
secret_handling | lease_safety | audit_requirements | hard_budget_guardrails | kernel_safety
```

Any attempt to construct a `MutationProposal` whose effect touches a protected category returns an
error and is itself recorded as an `AuditKindMutation` entry with `Detail="rejected: protected-invariant"`.
SCENARIO-0018 asserts: "Protected invariants (budget guardrails, secret handling) were not mutated."

> Mapping note: AC-2 lists `budget_escalation` as a *mutable* target while AC-3 protects
> `hard_budget_guardrails`. These are distinct: a mutation may tune the *escalation heuristic* (when to
> escalate to a stronger/cheaper model on cost pressure), but may never relax a **hard** budget ceiling
> (STORY-0036 AC-2: guardrails protected from automatic mutation unless human-approved). The guard keys
> on the hard-ceiling fields specifically.

## 5. Trial → measure → promote/reject/revert (the rest of AC-4)

The detector + proposal cover "detect → propose". The remaining flow is deterministic state machine
logic on the genome entry, testable as unit/process-level (SCENARIO-0018 proof seam = process-level):

1. **Trial.** The next dispatch in `Domain` uses the candidate genome content in an *experiment* run
   (`Run` stamped so the audit links experiment → proposal). Production dispatch is unchanged until promote.
2. **Measure.** Compare an outcome metric over the trial window vs the pre-trial baseline. v1 metric =
   **stumble-recurrence rate** (did the same `signalType` recur in the trial run?). SCENARIO-0018:
   "Trial run completes successfully" → recurrence rate drops.
3. **Promote / keep / reject / revert.**
   - `promote`: trial improved the metric beyond a threshold (or human approval) → genome entry
     `status=active`, `source=promoted`, new `version`, prior active entry retained (revertible).
   - `keep`: inconclusive → stays `candidate` for another trial.
   - `reject`: trial did not improve → `status=rejected`; proposal key freed for future detection.
   - `revert`: a previously promoted mutation later regresses → restore prior `version`,
     mark `status=reverted`. AC: "Mutation is auditable and revertible."

Every transition appends an `AuditKindMutation` entry (actor, proposal ID, from/to status, evidence),
so the whole lifecycle is replayable via `AuditLog.Replay()` (AC-4 evidence trail; revertibility).

## 6. Scope boundary for ITER-0008b

**In scope (CI-provable, unit/process-level — SCENARIO-0018):**
- `GenomeDetector.DetectStumblePatterns` (pure, unit-tested with synthetic runs).
- `MutationProposal` + the genome entry schema (AC-1), target enum (AC-2), protected-invariant guard (AC-3).
- The promote/keep/reject/revert state machine + audit trail (AC-4), exercised with an in-memory
  `AuditLog` and a fake run history. The "trial run" is simulated by feeding a follow-up `Run` with no
  recurring stumble — no live cluster needed for the lifecycle logic.

**Out of scope / deliberately not built (YAGNI):**
- ML-based or multi-factor pattern detection — v1 is count-in-window only.
- Automatic content synthesis of the new prompt/heuristic. v1 proposals carry a *rationale + target*;
  the actual content edit is operator-authored or a trivial templated tweak. The flow (propose→trial→
  measure→promote) is what AC-4 proves, not an AI that writes prompts.
- `policy_id`-refined detection domains.

## 7. Test obligations (feeds SCENARIO-0018 automation)

- `TestGenomeDetector_FiresOnThreeTimeouts` — 3 distinct runs with `timeout` in window → one pattern,
  `EvidenceRunIDs` = the 3 run IDs.
- `TestGenomeDetector_BelowThresholdNoFire` / `_OutsideWindowNoFire` / `_DistinctRunsNotSignals`.
- `TestGenomeDetector_DedupOpenProposal` — does not re-fire while a proposal is open.
- `TestMutationProposal_RejectsProtectedTarget` — secret/lease/audit/hard-budget/kernel blocked + audited.
- `TestMutationLifecycle_PromoteRejectRevert` — full AC-4 state machine + audit entries.
- `TestScenario0018` — end-to-end: 3 timeouts → detect → propose(candidate, learned, prompt_tweak) →
  trial → measure(success) → promote(active) → audit trail present, protected invariants untouched.
