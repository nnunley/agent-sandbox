package main

import "testing"

func TestMemoryEscalationLane(t *testing.T) {
	var lane EscalationLane = NewMemoryEscalationLane()

	if err := lane.Push(EscalationItem{DirectiveID: "d1", Reason: "authority-limit", Origin: "orchestrator"}); err != nil {
		t.Fatalf("push d1: %v", err)
	}
	if err := lane.Push(EscalationItem{DirectiveID: "d2", Reason: "hard-fail", Origin: "worker:7"}); err != nil {
		t.Fatalf("push d2: %v", err)
	}

	items := lane.List()
	if len(items) != 2 {
		t.Fatalf("lane has %d items, want 2", len(items))
	}
	// FIFO order, threaded to origin.
	if items[0].DirectiveID != "d1" || items[0].Reason != "authority-limit" || items[0].Origin != "orchestrator" {
		t.Fatalf("item 0 wrong: %+v", items[0])
	}
	if items[1].DirectiveID != "d2" {
		t.Fatalf("FIFO order wrong: %+v", items)
	}

	// List returns a defensive copy — mutating it must not corrupt the lane.
	items[0].DirectiveID = "mutated"
	if lane.List()[0].DirectiveID != "d1" {
		t.Fatalf("List() did not return a defensive copy")
	}
}
