package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// AuditKind enumerates the kinds of audit events (STORY-0054 AC-1).
type AuditKind string

const (
	AuditKindRun       AuditKind = "run"
	AuditKindDelegation AuditKind = "delegation"
	AuditKindTransition AuditKind = "transition"
	AuditKindToolAction AuditKind = "tool_action"
	AuditKindMutation   AuditKind = "mutation"
)

// AuditEntry is one immutable record in the audit log (STORY-0054 AC-1/AC-3).
// ID is a stable, unique identifier assigned by Append if empty.
// Ts is the timestamp; Actor is who performed the action (e.g., "orchestrator", "worker:W1", "temporal").
// Kind identifies the event type; ThreadID and RunID identify the work context.
// ParentRef is the causal parent (run ID or entry ID) enabling ordered reconstruction (AC-2).
// Detail is a small payload with additional context.
type AuditEntry struct {
	ID        string    `json:"id"`
	Ts        time.Time `json:"ts"`
	Actor     string    `json:"actor"`
	Kind      AuditKind `json:"kind"`
	ThreadID  string    `json:"thread_id"`
	RunID     string    `json:"run_id"`
	ParentRef string    `json:"parent_ref,omitempty"`
	Detail    string    `json:"detail,omitempty"`
}

// AuditLog is the swappable interface for audit logging (STORY-0054 AC-3).
// Implementations must be append-only and immutable.
type AuditLog interface {
	// Append adds an entry to the log, assigning a stable ID if empty.
	// Returns the stored entry (with ID filled in) and any error.
	Append(AuditEntry) (AuditEntry, error)

	// Entries returns all appended entries in append order (a defensive copy).
	Entries() []AuditEntry

	// ByThread returns all entries for a given threadID in append order (a defensive copy).
	ByThread(threadID string) []AuditEntry

	// ByRun returns all entries for a given runID in append order (a defensive copy).
	ByRun(runID string) []AuditEntry

	// Replay returns all entries in causal order (a defensive copy).
	// Entries are ordered so that each entry's ParentRef (if non-empty) appears before it.
	Replay() []AuditEntry
}

// JSONLAuditLog writes audit entries as JSONL (one JSON object per line) to an io.Writer.
// Append-only and immutable (STORY-0054 AC-3). Thread-safe for concurrent use.
type JSONLAuditLog struct {
	mu        sync.Mutex
	w         io.Writer
	idCounter atomic.Int64
	now       func() time.Time
	// in-memory copy for Entries/ByThread/ByRun/Replay (never mutated after Append).
	records []AuditEntry
}

// NewJSONLAuditLog returns a JSONLAuditLog that writes to w and uses now to stamp zero-valued Ts.
func NewJSONLAuditLog(w io.Writer, now func() time.Time) *JSONLAuditLog {
	return &JSONLAuditLog{
		w:   w,
		now: now,
	}
}

// Append adds an entry to the log, assigning a stable ID if empty, and writes it as JSONL.
// The mutex makes Append atomic; the line is never interleaved with another goroutine's.
func (j *JSONLAuditLog) Append(e AuditEntry) (AuditEntry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	// Assign stable ID if empty.
	if e.ID == "" {
		counter := j.idCounter.Add(1)
		e.ID = fmt.Sprintf("audit-%d", counter)
	}

	// Stamp Ts if zero.
	if e.Ts.IsZero() {
		e.Ts = j.now()
	}

	// Append to in-memory copy (for Entries/ByThread/ByRun/Replay).
	j.records = append(j.records, e)

	// Write to the file (JSONL: one JSON object per line).
	b, err := json.Marshal(e)
	if err != nil {
		return AuditEntry{}, err
	}
	b = append(b, '\n')
	_, err = j.w.Write(b)
	if err != nil {
		return AuditEntry{}, err
	}

	return e, nil
}

// Entries returns all appended entries in append order (a defensive copy).
func (j *JSONLAuditLog) Entries() []AuditEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]AuditEntry, len(j.records))
	copy(out, j.records)
	return out
}

// ByThread returns all entries for a given threadID in append order (a defensive copy).
func (j *JSONLAuditLog) ByThread(threadID string) []AuditEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []AuditEntry
	for _, e := range j.records {
		if e.ThreadID == threadID {
			out = append(out, e)
		}
	}
	return out
}

// ByRun returns all entries for a given runID in append order (a defensive copy).
func (j *JSONLAuditLog) ByRun(runID string) []AuditEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []AuditEntry
	for _, e := range j.records {
		if e.RunID == runID {
			out = append(out, e)
		}
	}
	return out
}

// Replay returns all entries in causal order (a defensive copy).
// Causal order is determined by ParentRef: each entry's ParentRef (if non-empty) should appear before it.
func (j *JSONLAuditLog) Replay() []AuditEntry {
	j.mu.Lock()
	defer j.mu.Unlock()

	// Build a map of entry ID → entry for lookup.
	entryByID := make(map[string]AuditEntry)
	var roots []AuditEntry

	for _, e := range j.records {
		entryByID[e.ID] = e
		if e.ParentRef == "" {
			roots = append(roots, e)
		}
	}

	// Perform a topological sort: start from roots, follow ParentRef pointers.
	// This reconstructs the causal chain deterministically.
	var ordered []AuditEntry
	visitedIDs := make(map[string]bool)

	// Helper to visit an entry and its children in causal order.
	var visit func(AuditEntry)
	visit = func(e AuditEntry) {
		if visitedIDs[e.ID] {
			return // already visited
		}
		visitedIDs[e.ID] = true

		// Visit parent first (if it exists).
		if e.ParentRef != "" {
			if parent, ok := entryByID[e.ParentRef]; ok {
				visit(parent)
			}
		}

		// Then visit this entry.
		ordered = append(ordered, e)
	}

	// Visit all entries (roots will pull their descendants in causal order).
	for _, e := range j.records {
		visit(e)
	}

	// Return a defensive copy.
	out := make([]AuditEntry, len(ordered))
	copy(out, ordered)
	return out
}

// MemoryAuditLog is an in-memory AuditLog for testing (STORY-0054 AC-1/AC-3).
// It records appended entries in order and is safe for concurrent use.
type MemoryAuditLog struct {
	mu        sync.Mutex
	records   []AuditEntry
	idCounter atomic.Int64
	now       func() time.Time
}

// NewMemoryAuditLog returns an empty MemoryAuditLog.
func NewMemoryAuditLog() *MemoryAuditLog {
	return &MemoryAuditLog{now: time.Now}
}

// Append adds an entry to the log, assigning a stable ID if empty.
func (m *MemoryAuditLog) Append(e AuditEntry) (AuditEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Assign stable ID if empty.
	if e.ID == "" {
		counter := m.idCounter.Add(1)
		e.ID = fmt.Sprintf("audit-%d", counter)
	}

	// Stamp Ts if zero.
	if e.Ts.IsZero() {
		e.Ts = m.now()
	}

	m.records = append(m.records, e)
	return e, nil
}

// Entries returns all appended entries in append order (a defensive copy).
func (m *MemoryAuditLog) Entries() []AuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]AuditEntry, len(m.records))
	copy(out, m.records)
	return out
}

// ByThread returns all entries for a given threadID in append order (a defensive copy).
func (m *MemoryAuditLog) ByThread(threadID string) []AuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []AuditEntry
	for _, e := range m.records {
		if e.ThreadID == threadID {
			out = append(out, e)
		}
	}
	return out
}

// ByRun returns all entries for a given runID in append order (a defensive copy).
func (m *MemoryAuditLog) ByRun(runID string) []AuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []AuditEntry
	for _, e := range m.records {
		if e.RunID == runID {
			out = append(out, e)
		}
	}
	return out
}

// Replay returns all entries in causal order (a defensive copy).
func (m *MemoryAuditLog) Replay() []AuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build a map of entry ID → entry for lookup.
	entryByID := make(map[string]AuditEntry)
	var roots []AuditEntry

	for _, e := range m.records {
		entryByID[e.ID] = e
		if e.ParentRef == "" {
			roots = append(roots, e)
		}
	}

	// Perform a topological sort: start from roots, follow ParentRef pointers.
	var ordered []AuditEntry
	visitedIDs := make(map[string]bool)

	var visit func(AuditEntry)
	visit = func(e AuditEntry) {
		if visitedIDs[e.ID] {
			return
		}
		visitedIDs[e.ID] = true

		// Visit parent first.
		if e.ParentRef != "" {
			if parent, ok := entryByID[e.ParentRef]; ok {
				visit(parent)
			}
		}

		// Then visit this entry.
		ordered = append(ordered, e)
	}

	// Visit all entries.
	for _, e := range m.records {
		visit(e)
	}

	// Return a defensive copy.
	out := make([]AuditEntry, len(ordered))
	copy(out, ordered)
	return out
}
