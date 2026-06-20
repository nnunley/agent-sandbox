package main

import "sync"

// EscalationItem is one directive parked in the human escalations lane.
type EscalationItem struct {
	DirectiveID string
	Reason      string // why it escalated (rule fired / authority limit)
	Origin      string // the origin thread, so the escalation is threaded back
}

// EscalationLane is the durable, NON-BLOCKING human escalations lane (STORY-0055 AC-5,
// STORY-0061 AC-2). Authority/judgment-limited escalations — and any privileged rung —
// land here as a distinct durable state while the coordinator keeps processing other
// directives (landing in the lane never blocks the loop). A plain FIFO in ITER-0001;
// Temporal urgency-driven resurfacing (STORY-0061 AC-3) layers on in ITER-0007. Kept behind
// an interface so the ITER-0006 substrate can make it cluster-resident.
type EscalationLane interface {
	Push(EscalationItem) error
	List() []EscalationItem
}

// MemoryEscalationLane is an in-memory FIFO EscalationLane.
type MemoryEscalationLane struct {
	mu    sync.Mutex
	items []EscalationItem
}

func NewMemoryEscalationLane() *MemoryEscalationLane { return &MemoryEscalationLane{} }

func (l *MemoryEscalationLane) Push(it EscalationItem) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = append(l.items, it)
	return nil
}

// List returns the lane's items in FIFO order (a defensive copy).
func (l *MemoryEscalationLane) List() []EscalationItem {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]EscalationItem, len(l.items))
	copy(out, l.items)
	return out
}
