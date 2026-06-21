package main

import (
	"sync"
	"time"
)

// ResumeSummary is the soft progression hint carried across thread boundaries (STORY-0029 AC-1).
type ResumeSummary struct {
	PriorWork string `json:"prior_work"`
	NextStep  string `json:"next_step"`
}

// Thread is the continuity object for a unit of work (keyed by thread id = the Directive.ID
// lineage). It holds SOFT state only — never the authoritative diff/grade. Reuses ThreadStatus
// from threadstatus.go (do NOT redefine it).
type Thread struct {
	ID                string        `json:"thread_id"`
	Status            ThreadStatus  `json:"status"`
	CurrentBranch     string        `json:"current_branch"`
	CurrentWorkspace  string        `json:"current_workspace"`
	ResumeSummary     ResumeSummary `json:"resume_summary"`
	LastVerifiedState string        `json:"last_verified_state"`     // STORY-0029 AC-2
	Supersedes        string        `json:"supersedes,omitempty"`    // STORY-0030 AC-1
	SupersededBy      string        `json:"superseded_by,omitempty"` // STORY-0030 AC-1
	Deadline          *time.Time    `json:"deadline,omitempty"`      // preemptive ITER-0007; nil = none
}

// ThreadStore is a daemon-local, concurrency-safe registry of Threads keyed by Thread.ID.
// (Durable persistence is deferred to ITER-0006/0008 — in-memory is correct for now.)
type ThreadStore struct {
	mu      sync.Mutex
	threads map[string]Thread
}

// NewThreadStore returns an empty ThreadStore.
func NewThreadStore() *ThreadStore {
	return &ThreadStore{threads: make(map[string]Thread)}
}

// Put inserts or replaces the thread by t.ID.
func (s *ThreadStore) Put(t Thread) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.threads[t.ID] = t
}

// Get returns the thread with the given id. ok is false when absent.
func (s *ThreadStore) Get(id string) (Thread, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.threads[id]
	return t, ok
}
