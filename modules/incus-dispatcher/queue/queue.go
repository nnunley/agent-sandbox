// Package queue is the directive substrate for the fleet coordinator.
//
// ITER-0000 ships an in-memory STUB (MemoryQueue) that deliberately models the
// full contract the real substrate (laneq, ITER-0006) must satisfy, so the swap
// is drop-in and ITER-0000 does not box in the substrate choice:
//
//   - Directive carries the full field set + a NotBefore eligibility gate.
//   - Queue offers ATOMIC claim + LEASE (timeout + renewal) + REQUEUE — not a
//     naive pop — so daemon code written against this interface keeps working
//     when laneq (with real leasing) replaces the stub.
//
// Importance/Deadline are INPUTS. The effective scheduling fields (Priority,
// NotBefore) are projected — in ITER-0000 the stub projects Priority from
// Importance directly; in ITER-0007 Temporal becomes the single writer of those
// fields (see docs/plans/2026-06-18-fleet-orchestration-design.md).
package queue

import (
	"errors"
	"time"
)

// ErrEmpty is returned by Claim when no eligible pending directive exists.
var ErrEmpty = errors.New("queue: no eligible directive")

// ErrLeaseLost is returned when a lease has expired or is unknown (the directive
// may have been reaped and requeued, or completed by another consumer).
var ErrLeaseLost = errors.New("queue: lease lost or unknown")

// Importance is how much a directive matters (author-set input; never the
// effective schedule). Orthogonal to urgency (derived from Deadline).
type Importance string

const (
	ImportanceHigh   Importance = "high"
	ImportanceNormal Importance = "normal"
	ImportanceLow    Importance = "low"
)

// priorityOf projects an Importance to an effective priority (lower = sooner).
// ITER-0000 stub projection; ITER-0007 Temporal replaces this as single writer.
func priorityOf(i Importance) int {
	switch i {
	case ImportanceHigh:
		return 0
	case ImportanceLow:
		return 2
	default:
		return 1
	}
}

// GradeSpec describes an optional authoritative external grade. Presence on a
// Directive means the daemon runs the oracle on a clean checkout after harvest.
type GradeSpec struct {
	OracleRef string         `json:"oracle_ref"`
	Cmd       string         `json:"cmd"`
	Expect    map[string]any `json:"expect,omitempty"`
}

// Directive is one unit of work. No access_cmd, no root flag (D1): the Template
// (validated against an allowlist + Origin) defines how the work runs.
type Directive struct {
	ID         string     `json:"id"`
	Intent     string     `json:"intent"`
	Template   string     `json:"template"`   // PROPOSED; daemon validates vs allowlist + origin
	Origin     string     `json:"origin"`     // "orchestrator" | "worker:<id>"; set by daemon, not author
	Importance Importance `json:"importance"` // INPUT
	Deadline   *time.Time `json:"deadline,omitempty"`
	NotBefore  time.Time  `json:"not_before,omitempty"` // eligibility gate; zero = always eligible
	Lane       string     `json:"lane,omitempty"`
	Repo       string     `json:"repo,omitempty"`
	Ref        string     `json:"ref,omitempty"`
	Task       string     `json:"task,omitempty"`
	HandoffIn  string     `json:"handoff_in,omitempty"` // optional lean-ctx bundle (gated on the ctx_handoff spike)
	Grade      *GradeSpec `json:"grade,omitempty"`
	// MaxAttempts: DEPRECATED as of ITER-0001. The D4 graduated escalation ladder
	// (daemon nextRung) supersedes a flat attempt cap — failures climb retry-same →
	// stronger-worker → hard-tier → human by attempt count. Retained for wire
	// compatibility; not read by the coordinator.
	MaxAttempts int `json:"max_attempts,omitempty"`

	// Attempts counts how many times this directive has been claimed+requeued.
	Attempts int `json:"attempts"`
}

// Lease is a claim token with an expiry. Renew with Touch; release with Done or
// Requeue. A lease that outlives its Expiry without a Touch is reclaimed by Reap.
type Lease struct {
	DirectiveID string
	Token       string
	Expiry      time.Time
}

// Queue is the directive substrate contract. MemoryQueue (ITER-0000) and laneq
// (ITER-0006) both satisfy it.
type Queue interface {
	// Push enqueues a directive, assigning an ID if empty. Returns the ID.
	Push(d Directive) (string, error)

	// Claim atomically reserves the highest-priority ELIGIBLE (NotBefore <= now)
	// pending directive and returns it with a Lease. Returns ErrEmpty if none.
	Claim(consumer string, leaseDur time.Duration) (Directive, Lease, error)

	// Touch renews a lease. Returns ErrLeaseLost if expired/unknown.
	Touch(lease Lease, leaseDur time.Duration) (Lease, error)

	// Done marks a claimed directive complete and removes it.
	Done(lease Lease) error

	// Requeue returns a claimed directive to pending, incrementing Attempts and
	// setting its NotBefore (zero = immediately eligible).
	Requeue(lease Lease, notBefore time.Time) error

	// Reap reclaims expired leases (requeues them). Returns the count reclaimed.
	Reap() (int, error)

	// Peek returns the directive Claim would return next — the highest-priority
	// eligible (NotBefore <= now) pending directive — without claiming it.
	// No lease is created and the queue is not mutated. Returns ErrEmpty if
	// no eligible pending directive exists.
	Peek() (Directive, error)

	// Park moves a CLAIMED directive (held by lease) into a DURABLE parked hold state —
	// distinct from done (removed) and pending (claimable). A parked directive is NOT returned
	// by Claim/Peek and is NOT reclaimed by Reap; it stays until manual intervention.
	// Returns ErrLeaseLost if the lease is expired/unknown (match Done/Requeue lease handling).
	Park(lease Lease) error

	// Len reports pending + claimed directive counts (for tests/observability).
	Len() (pending, claimed int)

	// DeferDirective returns a claimed directive to pending with Attempts PRESERVED (not incremented)
	// and NotBefore set to the specified time (for backoff/rate-limiting without failure escalation).
	// Used by dispatch gates (e.g., paused/blocked threads) to hold work without consuming retries.
	// Returns ErrLeaseLost if the lease is expired/unknown (match Done/Requeue lease handling).
	// Note: LaneqQueue has a separate Defer(id, notBefore) method on the Reprojector interface.
	DeferDirective(lease Lease, notBefore time.Time) error
}
