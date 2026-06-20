package main

import "time"

type ThreadStatus string

const (
	StatusQueued    ThreadStatus = "queued"
	StatusActive    ThreadStatus = "active"
	StatusPaused    ThreadStatus = "paused"
	StatusBlocked   ThreadStatus = "blocked"
	StatusDone      ThreadStatus = "done"
	StatusAbandoned ThreadStatus = "abandoned"
)

// Transition is one recorded status change for a thread.
type Transition struct {
	From ThreadStatus
	To   ThreadStatus
	Ts   time.Time
}

// ThreadTracker tracks the current status of each thread (keyed by directive id) and
// records every transition in order. Clock-injected for deterministic timestamps.
type ThreadTracker struct {
	now         func() time.Time
	current     map[string]ThreadStatus
	transitions map[string][]Transition
}

func NewThreadTracker(now func() time.Time) *ThreadTracker {
	return &ThreadTracker{
		now:         now,
		current:     make(map[string]ThreadStatus),
		transitions: make(map[string][]Transition),
	}
}

func (t *ThreadTracker) Set(id string, s ThreadStatus) {
	from := t.current[id]
	t.current[id] = s
	t.transitions[id] = append(t.transitions[id], Transition{
		From: from,
		To:   s,
		Ts:   t.now(),
	})
}

func (t *ThreadTracker) Status(id string) ThreadStatus {
	return t.current[id]
}

func (t *ThreadTracker) Transitions(id string) []Transition {
	src := t.transitions[id]
	out := make([]Transition, len(src))
	copy(out, src)
	return out
}
