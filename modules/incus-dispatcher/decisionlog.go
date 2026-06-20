package main

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// Decision records the outcome of one directive through the coordinator.
type Decision struct {
	DirectiveID string
	Grade       string    // short grade summary
	Rule        string    // coordination rule that fired
	Action      string    // action taken: done | requeue | park | escalate | reap | ...
	Ts          time.Time
}

// DecisionLog is the swappable writer interface (AC-4): coordination code depends ONLY on
// this interface, never on a concrete type, so the JSONL writer can later be replaced by a
// tamper-evident (HMAC-chained) one without changing callers.
type DecisionLog interface{ Append(Decision) error }

// JSONLDecisionLog writes one compact JSON object per line (append-only, AC-1) to w.
// If a Decision's Ts is the zero value, it is stamped with now() at Append time.
type JSONLDecisionLog struct {
	mu  sync.Mutex
	w   io.Writer
	now func() time.Time
}

// NewJSONLDecisionLog returns a JSONLDecisionLog that writes to w and uses now to
// stamp zero-valued Ts fields.
func NewJSONLDecisionLog(w io.Writer, now func() time.Time) *JSONLDecisionLog {
	return &JSONLDecisionLog{w: w, now: now}
}

// Append marshals d as compact JSON followed by a single newline and writes it as ONE
// Write to w. The mutex makes Append atomic under concurrent callers (the line is never
// interleaved with another goroutine's).
func (j *JSONLDecisionLog) Append(d Decision) error {
	if d.Ts.IsZero() {
		d.Ts = j.now()
	}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	j.mu.Lock()
	defer j.mu.Unlock()
	_, err = j.w.Write(b)
	return err
}

// MemoryDecisionLog is an in-memory DecisionLog that records appended Decisions in order
// (a test double and a base for future writers). Safe for concurrent use.
type MemoryDecisionLog struct {
	mu      sync.Mutex
	records []Decision
}

// NewMemoryDecisionLog returns an empty MemoryDecisionLog.
func NewMemoryDecisionLog() *MemoryDecisionLog {
	return &MemoryDecisionLog{}
}

// Append records d in order.
func (m *MemoryDecisionLog) Append(d Decision) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, d)
	return nil
}

// Records returns all appended Decisions in append order (a defensive copy).
func (m *MemoryDecisionLog) Records() []Decision {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Decision, len(m.records))
	copy(out, m.records)
	return out
}
