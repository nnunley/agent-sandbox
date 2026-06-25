package main

import (
	"sync"
)

// ResultStore is an in-memory store of run results (keyed by run ID or thread ID).
// It holds artifacts, patch data, and execution metadata for inspection and replay.
// Thread-safe for concurrent use.
type ResultStore struct {
	mu      sync.Mutex
	results map[string]*Result // by thread/run ID
}

// NewResultStore returns an empty ResultStore.
func NewResultStore() *ResultStore {
	return &ResultStore{
		results: make(map[string]*Result),
	}
}

// Store saves a result by ID (typically thread ID or run ID).
func (rs *ResultStore) Store(id string, result *Result) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.results[id] = result
}

// Get retrieves a result by ID. ok is false when absent.
func (rs *ResultStore) Get(id string) (*Result, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r, ok := rs.results[id]
	return r, ok
}
