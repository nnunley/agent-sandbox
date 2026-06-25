package main

import "time"

// ArtifactKind enumerates the artifact reference types (STORY-0015).
type ArtifactKind string

const (
	ArtifactDiff               ArtifactKind = "diff"
	ArtifactNote               ArtifactKind = "note"
	ArtifactSynthesis          ArtifactKind = "synthesis"
	ArtifactBenchmark          ArtifactKind = "benchmark"
	ArtifactVerificationReport ArtifactKind = "verification_report"
	ArtifactDesignDoc          ArtifactKind = "design_doc"
	ArtifactMutationProposal    ArtifactKind = "mutation_proposal"
)

// ArtifactRef is a typed reference to an artifact (STORY-0015 AC-1b).
// Kind identifies the artifact type; Ref is an opaque reference (path/URI/id).
type ArtifactRef struct {
	Kind ArtifactKind `json:"kind"`
	Ref  string       `json:"ref"`
}

// BudgetSnapshot captures the budget context at dispatch time (STORY-0035 AC-2).
// This is a read-only snapshot for auditing; enforcement happens in ITER-0008b (STORY-0036).
type BudgetSnapshot struct {
	LimitTokens int64  `json:"limit_tokens"`
	SpentTokens int64  `json:"spent_tokens"`
	Currency    string `json:"currency"`
}

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
// fields; ITER-0008 (STORY-0011/0015/0035) ADDS worker_id/worker_kind/policy_id (STORY-0011),
// artifact_refs/log_refs (STORY-0015), and provider_instance/model_id/budget_snapshot (STORY-0035 AC-1/2).
// All new fields are omitempty for back-compat. Reserved for ITER-0008b (STORY-0035 AC-3/4):
// tokens, latency, spend (enforcement metrics).
type Run struct {
	RunID            string          `json:"run_id"`
	ThreadID         string          `json:"thread_id"`
	ParentRunID      string          `json:"parent_run_id,omitempty"`
	WorkerID         string          `json:"worker_id,omitempty"`         // STORY-0011: dispatched worker id
	WorkerKind       string          `json:"worker_kind,omitempty"`       // STORY-0011: worker class (e.g., temporal-worker)
	PolicyID         string          `json:"policy_id,omitempty"`         // STORY-0011: policy id including version
	ArtifactRefs     []ArtifactRef   `json:"artifact_refs,omitempty"`     // STORY-0015: typed artifact references
	LogRefs          []string        `json:"log_refs,omitempty"`          // STORY-0015: log references
	ProviderInstance string          `json:"provider_instance,omitempty"` // STORY-0035 AC-1: LLM provider instance
	ModelID          string          `json:"model_id,omitempty"`          // STORY-0035 AC-1: model id
	BudgetSnapshot   *BudgetSnapshot `json:"budget_snapshot,omitempty"`   // STORY-0035 AC-2: budget at dispatch
	StumbleSignals   []StumbleSignal `json:"stumble_signals,omitempty"`
}

// AddStumble appends a stumble signal to the run (STORY-0031 AC-1). It stamps the signal's RunID
// with r.RunID; the caller provides Ts (the caller controls the clock — Ts is used as-is).
func (r *Run) AddStumble(s StumbleSignal) {
	s.RunID = r.RunID
	r.StumbleSignals = append(r.StumbleSignals, s)
}
