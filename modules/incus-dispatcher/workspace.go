package main

import (
	"sync"
	"time"
)

// WorkspaceKey identifies a workspace by repo + branch (STORY-0033 AC-2: the thread owns this).
type WorkspaceKey struct {
	Repo   string
	Branch string
}

// WorkspaceClaim records which thread owns a (repo,branch) workspace and until when.
// This is SEPARATE from queue.Lease (directive-claim); do not import or modify queue.Lease.
type WorkspaceClaim struct {
	ThreadID   string
	LeaseToken string
	Expiry     time.Time
}

// WorkspaceRegistry is a daemon-local, concurrency-safe map[WorkspaceKey]WorkspaceClaim.
// Clock-injected for deterministic expiry. (Durable persistence deferred to ITER-0006/0008.)
type WorkspaceRegistry struct {
	mu     sync.Mutex
	now    func() time.Time
	claims map[WorkspaceKey]WorkspaceClaim
}

// NewWorkspaceRegistry returns an empty WorkspaceRegistry using the provided clock.
func NewWorkspaceRegistry(now func() time.Time) *WorkspaceRegistry {
	return &WorkspaceRegistry{
		now:    now,
		claims: make(map[WorkspaceKey]WorkspaceClaim),
	}
}

// ActiveClaim returns the current non-expired claim for key (ok=false if none or expired).
func (r *WorkspaceRegistry) ActiveClaim(key WorkspaceKey) (WorkspaceClaim, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeLocked(key)
}

// activeLocked is the lock-free inner read; caller must hold r.mu.
func (r *WorkspaceRegistry) activeLocked(key WorkspaceKey) (WorkspaceClaim, bool) {
	c, ok := r.claims[key]
	if !ok || !r.now().Before(c.Expiry) {
		return WorkspaceClaim{}, false
	}
	return c, true
}

// Claim records threadID's claim on key for ttl and returns it. ok=false (no state change) when an
// ACTIVE claim by a DIFFERENT thread already exists — the caller must supersede instead
// (STORY-0033 AC-1: check before reuse; AC-3: an active claim forces continue-or-supersede).
// A same-thread re-Claim RENEWS the expiry (continuation).
func (r *WorkspaceRegistry) Claim(key WorkspaceKey, threadID, token string, ttl time.Duration) (WorkspaceClaim, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.activeLocked(key); ok && existing.ThreadID != threadID {
		return WorkspaceClaim{}, false
	}

	c := WorkspaceClaim{
		ThreadID:   threadID,
		LeaseToken: token,
		Expiry:     r.now().Add(ttl),
	}
	r.claims[key] = c
	return c, true
}

// Release drops any claim on key.
func (r *WorkspaceRegistry) Release(key WorkspaceKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.claims, key)
}

// ReuseDecision is the verdict for reusing a workspace (STORY-0033 AC-3).
type ReuseDecision string

const (
	ReuseFree      ReuseDecision = "free"      // no active claim — safe to take
	ReuseContinue  ReuseDecision = "continue"  // active claim is THIS thread's — continue it
	ReuseSupersede ReuseDecision = "supersede" // active claim is ANOTHER thread's — must supersede
)

// DecideReuse classifies whether threadID may reuse key's workspace (STORY-0033 AC-1/AC-3).
func (r *WorkspaceRegistry) DecideReuse(key WorkspaceKey, threadID string) ReuseDecision {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.activeLocked(key)
	if !ok {
		return ReuseFree
	}
	if c.ThreadID == threadID {
		return ReuseContinue
	}
	return ReuseSupersede
}

// Supersede transfers an active claim on key from a DIFFERENT thread to newThreadID. STORY-0030
// AC-2: a non-empty reason is REQUIRED (declaring why the prior path is insufficient) — empty
// reason → ok=false, no change. STORY-0030 AC-3: the reinvention is captured as a structured
// StumbleSignal of Type=StumbleDuplicateWork (Ts from the registry clock, EvidenceSummary=reason)
// which is RETURNED for the caller to append to the run's stumble_signals via Run.AddStumble.
// Returns ok=false when there is no active claim by a different thread.
func (r *WorkspaceRegistry) Supersede(key WorkspaceKey, newThreadID, token, reason string, ttl time.Duration) (priorThreadID string, stumble StumbleSignal, ok bool) {
	if reason == "" {
		return "", StumbleSignal{}, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, active := r.activeLocked(key)
	if !active || existing.ThreadID == newThreadID {
		return "", StumbleSignal{}, false
	}

	priorThreadID = existing.ThreadID
	stumble = StumbleSignal{
		Type:            StumbleDuplicateWork,
		Ts:              r.now(),
		EvidenceSummary: reason,
	}

	r.claims[key] = WorkspaceClaim{
		ThreadID:   newThreadID,
		LeaseToken: token,
		Expiry:     r.now().Add(ttl),
	}
	return priorThreadID, stumble, true
}
