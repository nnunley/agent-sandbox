package main

import (
	"sync"
)

// ResultStore is an in-memory store of run results (keyed by run ID or thread ID).
// It holds artifacts, patch data, and execution metadata for inspection and replay.
// Thread-safe for concurrent use. Also tracks runs per thread for budget enforcement (STORY-0036 AC-3).
type ResultStore struct {
	mu         sync.Mutex
	results    map[string]*Result // by thread/run ID
	threadRuns map[string][]*Run  // thread ID → list of runs for budget checking
}

// NewResultStore returns an empty ResultStore.
func NewResultStore() *ResultStore {
	return &ResultStore{
		results:    make(map[string]*Result),
		threadRuns: make(map[string][]*Run),
	}
}

// Store saves a result by ID (typically thread ID or run ID) and tracks it per thread.
// threadID should be the thread ID for budget tracking.
func (rs *ResultStore) Store(id string, result *Result) {
	rs.StoreWithThread(id, id, result)
}

// StoreWithThread saves a result and explicitly tracks it under a thread ID.
// This allows budget enforcement to sum per-thread costs.
func (rs *ResultStore) StoreWithThread(id, threadID string, result *Result) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.results[id] = result
	if result != nil {
		// Track the run in the thread's list (create a Run with costs from the Result).
		run := &Run{
			RunID:     id,
			ThreadID:  threadID,
			SpendUSD:  result.SpendUSD,
			TokensIn:  result.TokensIn,
			TokensOut: result.TokensOut,
			LatencyMs: result.LatencyMs,
		}
		rs.threadRuns[threadID] = append(rs.threadRuns[threadID], run)
	}
}

// Get retrieves a result by ID. ok is false when absent.
func (rs *ResultStore) Get(id string) (*Result, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r, ok := rs.results[id]
	return r, ok
}

// ByThread returns all runs (with costs) for a given thread ID.
// Used by budget enforcement to compute total thread spend.
func (rs *ResultStore) ByThread(threadID string) []*Run {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	runs := rs.threadRuns[threadID]
	if runs == nil {
		return []*Run{}
	}
	// Return a defensive copy.
	out := make([]*Run, len(runs))
	copy(out, runs)
	return out
}
