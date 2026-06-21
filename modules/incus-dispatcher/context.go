package main

import "time"

// ContextProvider is the seam between the coordinator/runner and whatever carries SOFT state
// between one-shot runs (diary, shared knowledge, handoff bundles). It exists so the fleet is NOT
// hard-coupled to lean-ctx: lean-ctx is one adapter (the default, built in T6 as LeanCtxProvider),
// never a hard dependency — a deliberate guard against vendor lock-in and lean-ctx's commercial
// upsell for teams/distributed operation. This mirrors how coordination is abstracted behind
// queue.Queue (the substrate swaps in ITER-0006).
//
// Soft state ONLY flows through here. The authoritative state — the code diff and the oracle grade —
// never does (STORY-0018 AC-4); losing or corrupting anything a ContextProvider returns must not
// change a run's correctness. By construction the interface also has NO work-claim method: a
// ContextProvider can never be used as the work queue (STORY-0018 AC-5).
type ContextProvider interface {
	// diary — STORY-0018 AC-1
	WriteDiary(threadID string, d DiaryEntry) error
	RecallDiary(threadID string) ([]DiaryEntry, error)
	// knowledge — STORY-0018 AC-2
	ShareKnowledge(threadID string, facts []Fact) error
	ReceiveKnowledge(threadID string) ([]Fact, error)
	// handoff bundle (see docs/plans/2026-06-21-handoff-bundle-schema.md) — STORY-0018 AC-3
	CreateHandoff(threadID, runID string, st WorkflowState) (bundlePath string, err error)
	ImportHandoff(bundlePath string) (HandoffManifest, error)
}

// DiaryEntry is one ctx_agent diary record: the soft progression notes for a run (STORY-0018 AC-1).
type DiaryEntry struct {
	Decisions []string  `json:"decisions"`
	Blockers  []string  `json:"blockers"`
	Progress  []string  `json:"progress"`
	Ts        time.Time `json:"ts"`
}

// Fact is one curated piece of shared knowledge exchanged between one-shot runs (STORY-0018 AC-2).
type Fact struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// WorkflowState is the soft progression hint carried in a handoff bundle's manifest (matches the
// handoff-bundle schema's workflow_state). It is NOT authoritative.
type WorkflowState struct {
	ResumeSummary    ResumeSummary `json:"resume_summary"`
	OpenQuestions    []string      `json:"open_questions,omitempty"`
	CurrentBranch    string        `json:"current_branch"`
	CurrentWorkspace string        `json:"current_workspace"`
}

// HandoffManifest is the (subset of the) bundle manifest a successor reads on import. SessionID is
// the EXPLICIT saved session id — never resolve by "latest" (STORY-0034 spike note).
type HandoffManifest struct {
	SchemaVersion int           `json:"schema_version"`
	ThreadID      string        `json:"thread_id"`
	RunID         string        `json:"run_id"`
	ParentRunID   string        `json:"parent_run_id,omitempty"`
	WorkflowState WorkflowState `json:"workflow_state"`
	SessionID     string        `json:"session_id"`
}

// NoopProvider is the fallback/test ContextProvider: it drops all soft state. It is also the
// mechanism that PROVES STORY-0018 AC-4 — with no real provider, a run still grades correctly from
// its Result, so correctness is independent of handoff loss. It is the default when a Daemon has no
// ContextProvider configured.
type NoopProvider struct{}

func (NoopProvider) WriteDiary(string, DiaryEntry) error      { return nil }
func (NoopProvider) RecallDiary(string) ([]DiaryEntry, error) { return nil, nil }
func (NoopProvider) ShareKnowledge(string, []Fact) error      { return nil }
func (NoopProvider) ReceiveKnowledge(string) ([]Fact, error)  { return nil, nil }
func (NoopProvider) CreateHandoff(string, string, WorkflowState) (string, error) {
	return "", nil
}
func (NoopProvider) ImportHandoff(string) (HandoffManifest, error) {
	return HandoffManifest{}, nil
}
