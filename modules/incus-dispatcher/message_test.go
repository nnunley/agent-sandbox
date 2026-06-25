package main

import (
	"errors"
	"fmt"
	"testing"
)

// TestEmitUnderPolicy_DepthMonotonicityBoundsRecursion proves STORY-0014 AC-3: depth monotonicity
// + the maxDepth ceiling together genuinely bound a delegation chain. A runaway chain that keeps
// delegating (each child of the previous emitter) strictly increases depth and MUST terminate at
// maxDepth; and a child that does not exceed its parent's depth is rejected outright.
func TestEmitUnderPolicy_DepthMonotonicityBoundsRecursion(t *testing.T) {
	bus := NewMessageBus()
	policy := ExecutionPolicy{Kind: PolicyKindResearchBurst, DelegationRules: []string{"t.request"}}
	const maxDepth = 3

	// Root emit (no parent): allowed at depth 0.
	root := Message{ThreadID: "th", RunID: "a0", Topic: "t.request", Kind: MessageKindRequest, Depth: 0}
	if err := EmitUnderPolicy(bus, policy, root, maxDepth); err != nil {
		t.Fatalf("root emit should succeed, got %v", err)
	}

	// A "runaway" chain: each hop delegates from the previous emitter, strictly incrementing depth.
	// It must run out at maxDepth, NOT continue forever.
	parent := "a0"
	depth := 0
	hops := 0
	var lastErr error
	for i := 1; i <= 100; i++ {
		depth++
		child := Message{
			ThreadID:    "th",
			RunID:       fmt.Sprintf("a%d", i),
			ParentRunID: parent,
			Depth:       depth,
			Topic:       "t.request",
			Kind:        MessageKindRequest,
		}
		lastErr = EmitUnderPolicy(bus, policy, child, maxDepth)
		if lastErr != nil {
			break
		}
		hops++
		parent = child.RunID
	}
	if !errors.Is(lastErr, ErrDepthExceeded) {
		t.Fatalf("runaway chain should terminate with ErrDepthExceeded, got %v after %d hops", lastErr, hops)
	}
	if hops != maxDepth-1 { // depths 1..maxDepth-1 succeed; depth==maxDepth is rejected
		t.Fatalf("chain ran %d hops before termination, want %d (bounded by maxDepth=%d)", hops, maxDepth-1, maxDepth)
	}

	// A non-monotonic child (same depth as its parent) is rejected — this is the loophole that
	// would otherwise let a chain run forever at a fixed depth.
	sibling := Message{ThreadID: "th", RunID: "x1", ParentRunID: "a0", Depth: 0, Topic: "t.request", Kind: MessageKindRequest}
	if err := EmitUnderPolicy(bus, policy, sibling, maxDepth); !errors.Is(err, ErrDepthNotMonotonic) {
		t.Fatalf("same-depth child should be rejected with ErrDepthNotMonotonic, got %v", err)
	}
}

// TestEmitUnderPolicy_EmptyDelegationRules proves a policy with no DelegationRules rejects every
// topic (fail-closed) — STORY-0014 AC-1.
func TestEmitUnderPolicy_EmptyDelegationRules(t *testing.T) {
	bus := NewMessageBus()
	policy := ExecutionPolicy{Kind: PolicyKindReviewOnly, DelegationRules: []string{}}
	msg := Message{ThreadID: "th", RunID: "r", Topic: "anything.request", Kind: MessageKindRequest}
	if err := EmitUnderPolicy(bus, policy, msg, 5); !errors.Is(err, ErrTopicNotAllowed) {
		t.Fatalf("empty DelegationRules should reject all topics, got %v", err)
	}
}

// TestMessageBus_EventAndStatusTopics proves STORY-0012 AC-3's "AND event/status" half: event and
// status kind messages round-trip through the bus (not only request/response).
func TestMessageBus_EventAndStatusTopics(t *testing.T) {
	bus := NewMessageBus()

	ev := Message{ThreadID: "th", RunID: "w", Topic: "run.events", Kind: MessageKindEvent, Payload: "started"}
	st := Message{ThreadID: "th", RunID: "w", Topic: "run.status", Kind: MessageKindStatus, Payload: "healthy"}
	if err := bus.Emit(ev); err != nil {
		t.Fatalf("emit event: %v", err)
	}
	if err := bus.Emit(st); err != nil {
		t.Fatalf("emit status: %v", err)
	}

	gotEv, _ := bus.Receive("run.events")
	if len(gotEv) != 1 || gotEv[0].Kind != MessageKindEvent || gotEv[0].Payload != "started" {
		t.Fatalf("event topic round-trip failed: %+v", gotEv)
	}
	gotSt, _ := bus.Receive("run.status")
	if len(gotSt) != 1 || gotSt[0].Kind != MessageKindStatus || gotSt[0].Payload != "healthy" {
		t.Fatalf("status topic round-trip failed: %+v", gotSt)
	}
}
