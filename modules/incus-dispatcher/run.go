package main

import "time"

// StumbleType enumerates the structured failure-signal kinds (STORY-0031 AC-2).
type StumbleType string

const (
	StumbleRetry               StumbleType = "retry"
	StumbleTimeout             StumbleType = "timeout"
	StumbleVerificationFailure StumbleType = "verification_failure"
	StumbleProviderFailure     StumbleType = "provider_failure"
	StumbleDelegationLoop      StumbleType = "delegation_loop"
	StumbleWorkspaceLoss       StumbleType = "workspace_loss"
	StumbleDuplicateWork       StumbleType = "duplicate_work"
	StumbleCostBlowout         StumbleType = "cost_blowout"
	StumbleStarvation          StumbleType = "starvation"
)

// StumbleSignal is one structured stumble captured during a run (STORY-0031 AC-1/AC-2).
type StumbleSignal struct {
	Type            StumbleType `json:"type"`
	Ts              time.Time   `json:"ts"`
	RunID           string      `json:"run_id"`
	EvidenceSummary string      `json:"evidence_summary"`
}

// Run is one execution attempt within a Thread. ITER-0004 carries only continuity/learning
// fields; ITER-0008 (STORY-0011/0015) will ADD worker_id/worker_kind/policy_id/artifact_refs/
// log_refs — keep this struct ADDITIVE-friendly (do not claim those names now).
type Run struct {
	RunID          string          `json:"run_id"`
	ThreadID       string          `json:"thread_id"`
	ParentRunID    string          `json:"parent_run_id,omitempty"`
	StumbleSignals []StumbleSignal `json:"stumble_signals,omitempty"`
}

// AddStumble appends a stumble signal to the run (STORY-0031 AC-1). It stamps the signal's RunID
// with r.RunID; the caller provides Ts (the caller controls the clock — Ts is used as-is).
func (r *Run) AddStumble(s StumbleSignal) {
	s.RunID = r.RunID
	r.StumbleSignals = append(r.StumbleSignals, s)
}
