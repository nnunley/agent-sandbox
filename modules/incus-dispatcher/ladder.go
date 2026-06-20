package main

// Rung is a step on the graduated escalation ladder (D4 — STORY-0055/STORY-0058).
// On failure the coordinator climbs deterministically by prior-attempt count rather than
// flat retry: retry the same, then a stronger pre-approved worker model, then a bigger /
// hard-tier pre-approved template, then hand to a human. Rungs 0..2 are PRE-APPROVED and
// climb AUTONOMOUSLY (Mac-off-safe, no model in the loop); RungHuman is the authority /
// judgment limit, reachable only via the escalations lane (STORY-0061).
//
// NOTE (ITER-0001 scope): the climb is SYNCHRONOUS. Temporal-backed backoff/retry
// (STORY-0058 AC-24) and urgency-driven resurfacing (STORY-0061 AC-3) are deferred to
// ITER-0007 and layer on top of this rung model without changing it.
type Rung int

const (
	RungRetrySame      Rung = iota // 0: transient fail → retry the same worker
	RungStrongerWorker             // 1: repeats → escalate to a stronger pre-approved worker model
	RungHardTier                   // 2: still failing → escalate to a bigger/hard-tier pre-approved template
	RungHuman                      // 3: authority/judgment limit → human escalations lane
)

func (r Rung) String() string {
	switch r {
	case RungRetrySame:
		return "retry-same"
	case RungStrongerWorker:
		return "stronger-worker"
	case RungHardTier:
		return "hard-tier"
	case RungHuman:
		return "human"
	default:
		return "unknown"
	}
}

// Autonomous reports whether this rung is a pre-approved rung the coordinator may climb
// without human intervention. Only RungHuman is non-autonomous.
func (r Rung) Autonomous() bool { return r >= RungRetrySame && r <= RungHardTier }

// nextRung maps the number of PRIOR attempts to the ladder rung for the next action:
// 0 → retry-same, 1 → stronger-worker, 2 → hard-tier, ≥3 → human. Deterministic; no clock.
func nextRung(attempts int) Rung {
	switch {
	case attempts <= 0:
		return RungRetrySame
	case attempts == 1:
		return RungStrongerWorker
	case attempts == 2:
		return RungHardTier
	default:
		return RungHuman
	}
}
