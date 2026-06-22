# ITER-0005 — Backend abstraction & isolation-tier selection (design note)

**Date:** 2026-06-21
**Iteration:** ITER-0005 (interface slice; infra → ITER-0005b)
**Stories:** STORY-0004, STORY-0017, STORY-0020, STORY-0023
**Purpose:** Lock the two design decisions the pre-iteration PAR scope review (2 reviewers,
both REVISE→APPROVE-after-revisions) required in writing before code: (1) where the
isolation tier lives, and (2) the tier→backend factory architecture. Also documents the
orthogonality with ITER-0008's `worker_kind` dispatch so neither iteration boxes in the other.

## Decision 1 — IsolationTier lives on the TemplateRule, NOT on the Directive

STORY-0023 AC-1: *"**Template** declares isolation tier selection (Fast or Hard) via D1
mechanism."* The Directive struct comment (`queue/queue.go:61`) already establishes the D1
discipline: *"No access_cmd, no root flag (D1): the Template (validated against an allowlist +
Origin) defines how the work runs."* The isolation tier is exactly such a "how the work runs"
property, so it belongs to the daemon-side, allowlist-controlled `TemplateRule` — never to an
author/worker-settable field on the Directive.

```go
// types.go (or tier.go)
type IsolationTier string
const (
    TierFast IsolationTier = "fast" // namespace isolation (nspawn) — trusted lanes
    TierHard IsolationTier = "hard" // hardware isolation (Firecracker microVM) — sensitive lanes
)

// policy.go — TemplateRule gains the tier the template runs at (D1 mechanism).
type TemplateRule struct {
    AllowWorkerOrigin bool
    Tier              IsolationTier // "" defaults to TierHard (fail-safe: most isolated)
}
```

**Why this beats adding `Directive.Tier` (the alternative one reviewer suggested):**
- **D1 integrity:** a compromised/drifting worker cannot request `Fast` (weaker isolation) for
  sensitive work — the tier is fixed by the vetted template, disposed by policy, not proposed by
  the author. This mirrors the existing `AllowWorkerOrigin` authority split.
- **No ITER-0006 boxing-in:** the tier never appears in the Directive JSON, so ITER-0006's
  `queue.ParseDirective` strict schema (`DisallowUnknownFields`, landed ITER-0002) is untouched —
  no laneq wire-schema change, no field to thread through the substrate swap.
- **Fail-safe default:** an unset tier resolves to `TierHard` (most isolated), so a misconfigured
  template degrades safely rather than silently running sensitive work in the fast tier.

## Decision 2 — Tier→backend selection is a factory OUTSIDE Runner.Run

The selection happens in the daemon AFTER D1 template validation and BEFORE the run; the chosen
`Runner` is then invoked through the unchanged `Runner.Run(ctx, task)` interface. Selection is
NOT pushed inside `Run` — every backend keeps the same signature, so ITER-0005b's microVM/nspawn
runners graft in by registering against the factory, with zero interface churn.

```go
// backend.go (new)
// BackendFactory resolves an IsolationTier to the Runner that implements it.
type BackendFactory interface {
    SelectRunner(tier IsolationTier) (Runner, error)
}

// staticBackendFactory maps tiers to pre-constructed runners.
// ITER-0005: container backend registered for the tiers it can serve.
// TODO(ITER-0005b): register the Firecracker microVM runner (TierHard) and the
// nspawn --ephemeral runner (TierFast, real-kernel guest) here. No daemon/interface change.
type staticBackendFactory struct{ byTier map[IsolationTier]Runner }
func (f *staticBackendFactory) SelectRunner(tier IsolationTier) (Runner, error) { ... }
```

Daemon wiring is **additive / backward-compatible** (preserves all 166 existing tests that set
`dm.Runner` directly): if `dm.Backend` (a `BackendFactory`) is set, the daemon resolves the tier
from the validated template's `TemplateRule.Tier` and calls `Backend.SelectRunner(tier)`;
otherwise it falls back to the single `dm.Runner` field.

```
RunOnce: Claim → setStatus(active) → Policy.ValidateTemplate(d)
       → tier := Policy.TierFor(d.Template)        // resolved from the vetted TemplateRule
       → runner := selectRunner(tier)              // factory if set, else dm.Runner
       → runner.Run(ctx, task) → runner.Cleanup() → grade → outcome
```

The resolved tier is recorded in the D6 decision log so a human/audit can see which substrate
ran the work (additive to the existing `record(...)` calls).

## Decision 3 — IsolationTier ⊥ worker_kind (ITER-0008 STORY-0011)

These are orthogonal axes and must not be conflated:
- **IsolationTier (this iteration):** `Fast | Hard` — the *trust/isolation substrate* a template
  runs on (namespace vs hardware). A template property, fixed by D1.
- **worker_kind (ITER-0008 STORY-0011):** `local | incus-container | microvm | research | …` —
  a *capability/dispatch* selector the Tier-2 coordinator uses to route work to a worker with the
  right tools, set by policy at dispatch time.

The relationship: tier *constrains* which worker_kinds are eligible (e.g. `Hard` requires a
worker_kind whose substrate provides hardware isolation), but it does not *pick* one — that is
ITER-0008's policy decision. Example: a `tier=Hard` directive could target worker_kind `microvm`
(local Firecracker) or a future `remote-microvm`, both hardware-isolated; policy chooses by
capability/availability. Because the factory keys on `IsolationTier` (not worker_kind), ITER-0008
layers worker_kind dispatch *above* the factory without modifying it.

## What is genuinely NEW in ITER-0005 (closing the "is this just docs?" question)

The `Runner` interface (`types.go:130`), the container runners (`cli_runner.go`,
`client_runner.go`), and the `container_runner_test.go` contract already exist — so STORY-0004
AC-1/AC-2, STORY-0017 AC-1/AC-2, and STORY-0020 AC-1 are *substantially* satisfied. The new work
this iteration adds is:
1. `IsolationTier` type + `TemplateRule.Tier` + `Policy.TierFor(template)` resolution (STORY-0023).
2. `BackendFactory` + `staticBackendFactory` + additive daemon wiring + decision-log tier line.
3. **Explicit contract tests** proving the abstraction (not just relying on it implicitly):
   SCENARIO-0028 (a fake backend and the container runner both satisfy `Runner`; the daemon drives
   either through the factory), SCENARIO-0076 (the existing `container_runner_test.go` CI subset
   wired as the corpus command), and SCENARIO-0089 (NEW: a `Fast` template resolves to the fast
   runner, a `Hard` template to the hard runner, an unset tier defaults to `Hard`).

## Scenario plan

| Scenario | Story | Seam | What it proves |
|---|---|---|---|
| SCENARIO-0028 | STORY-0017 AC-1 | unit | Backend interface abstracts delivery; daemon runs container OR fake backend via the factory |
| SCENARIO-0076 | STORY-0020 AC-1 | integration | Container backend passes `container_runner_test.go` (CI subset; incus tests skip when unreachable) |
| SCENARIO-0089 | STORY-0023 AC-1 | integration | Template-declared tier resolves to the correct backend; unset → Hard; D1 keeps tier off the Directive |

(STORY-0004 AC-2 "container proven" is covered by SCENARIO-0076 + the daemon driving the container
runner through the factory; no separate card.)
