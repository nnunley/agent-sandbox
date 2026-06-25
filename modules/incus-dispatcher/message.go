package main

import (
	"errors"
	"sync"
)

// MessageKind enumerates the message types (STORY-0012 AC-3, STORY-0014).
// Supports both request/response and event/status patterns.
type MessageKind string

const (
	MessageKindRequest  MessageKind = "request"
	MessageKindResponse MessageKind = "response"
	MessageKindEvent    MessageKind = "event"
	MessageKindStatus   MessageKind = "status"
)

// Message is one unit of work emission in the delegation bus (STORY-0012 AC-1, AC-2).
// It carries delegation-chain tracking (ThreadID, RunID, ParentRunID, Depth) and correlation
// metadata (CorrelationID) to enable async request/response pairing and graph reconstruction.
// Kind specifies the message type (request/response/event/status); Topic is the destination
// topic; Payload is the work description or status update.
type Message struct {
	// ThreadID: the root work unit ID, preserved across the entire delegation chain (STORY-0012 AC-1)
	ThreadID string

	// RunID: the ID of the agent/worker emitting this message (STORY-0012 AC-1)
	RunID string

	// ParentRunID: the ID of the agent that delegated to this run (STORY-0012 AC-1, enables graph reconstruction)
	ParentRunID string

	// Depth: how deep in the delegation chain this message sits (STORY-0014 AC-3)
	Depth int

	// CorrelationID: pairs async request/response messages (STORY-0012 AC-2, AC-4)
	CorrelationID string

	// Topic: the destination topic on the bus (e.g., "research.request", "web.fetch.request")
	Topic string

	// Kind: the message type (request/response/event/status) (STORY-0012 AC-3)
	Kind MessageKind

	// Payload: the work description or result (small, typically a goal or status update)
	Payload string
}

// Sentinel errors for policy and depth rejection (STORY-0014 AC-1, AC-3).
var (
	// ErrTopicNotAllowed is returned when EmitUnderPolicy rejects a topic not in the policy's DelegationRules.
	ErrTopicNotAllowed = errors.New("message: topic not allowed by policy")

	// ErrDepthExceeded is returned when EmitUnderPolicy rejects a message at or beyond max depth.
	ErrDepthExceeded = errors.New("message: depth limit exceeded")

	// ErrDepthNotMonotonic is returned when a child message's depth does not strictly exceed its
	// parent's depth. Enforcing this is what actually bounds recursion: without it a caller could
	// emit an unbounded chain of children all at the same depth and never hit the maxDepth ceiling.
	ErrDepthNotMonotonic = errors.New("message: child depth must exceed parent depth")
)

// MessageBus is an in-memory topic-based bus for message emission (STORY-0012 AC-3).
// It stores messages in topics and maintains a global log for graph reconstruction.
// This is a minimal seam; durable topics (laneq, NATS, etc.) are Phase-2 residual.
type MessageBus struct {
	mu     sync.Mutex
	topics map[string][]Message // topics[topicName] = []Message
	msgLog []Message            // global message log for reconstruction (STORY-0012 AC-4)
}

// NewMessageBus creates a new empty bus.
func NewMessageBus() *MessageBus {
	return &MessageBus{
		topics: make(map[string][]Message),
		msgLog: []Message{},
	}
}

// Emit appends a message to its topic and to the global log (STORY-0012 AC-3).
// No policy checks here; EmitUnderPolicy wraps this with policy enforcement (STORY-0014 AC-1).
func (b *MessageBus) Emit(msg Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Make a defensive copy to prevent external mutation of stored messages (T2a/T2c lesson).
	msgCopy := msg
	b.topics[msg.Topic] = append(b.topics[msg.Topic], msgCopy)
	b.msgLog = append(b.msgLog, msgCopy)
	return nil
}

// Receive retrieves all messages from a topic (and clears it for idempotent receive).
// Returns (messages, error). Callers should drain the topic and process messages.
func (b *MessageBus) Receive(topic string) ([]Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	msgs := b.topics[topic]
	// Return a copy to prevent external mutation of the bus's internal state.
	result := append([]Message(nil), msgs...)
	// Drain-and-clear: clearing the topic means a later Receive does NOT re-deliver these messages
	// (so a message is consumed once, not reprocessed). This is single-consumer-per-topic semantics
	// for the in-memory seam; a durable bus (Phase-2) would use acks/offsets instead.
	b.topics[topic] = []Message{}
	return result, nil
}

// MessageLog returns the full global message log (used for graph reconstruction).
// Returns a copy to prevent external mutation.
func (b *MessageBus) MessageLog() []Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Message(nil), b.msgLog...)
}

// EmitUnderPolicy enforces policy-gated emission with depth limits (STORY-0014 AC-1, AC-3).
// Rejections (no emission) and their sentinel errors:
//   - topic not in policy.DelegationRules → ErrTopicNotAllowed
//   - child depth does not exceed the observed parent's depth → ErrDepthNotMonotonic
//   - depth at/beyond the maxDepth ceiling → ErrDepthExceeded
//
// Monotonicity + the ceiling together guarantee recursion is BOUNDED: in any real delegation chain
// each hop's parent is a prior emitter, so depth strictly increases and the chain must terminate
// within maxDepth hops (STORY-0014 AC-3 "prevents unbounded recursion").
func EmitUnderPolicy(bus *MessageBus, policy ExecutionPolicy, msg Message, maxDepth int) error {
	// STORY-0014 AC-1: reject if topic not in DelegationRules
	topicAllowed := false
	for _, rule := range policy.DelegationRules {
		if rule == msg.Topic {
			topicAllowed = true
			break
		}
	}
	if !topicAllowed {
		return ErrTopicNotAllowed
	}

	// STORY-0014 AC-3 (monotonicity): when the parent run is observable on the bus, the child's depth
	// MUST strictly exceed it. (A ParentRunID naming a run the bus has not seen is a single message,
	// not a recursion chain, so it cannot be — and need not be — verified here.)
	//
	// Phase-1 seam notes for a future maintainer porting to a durable bus: this parent lookup is an
	// O(n) scan of the message log per emit (fine at CI scale; a durable bus would index run_id→depth),
	// and it is NOT atomic with the Emit below (a benign TOCTOU window — the worst case is a concurrent
	// emit changing the log between check and append; the maxDepth ceiling still hard-bounds recursion
	// regardless). A run's depth is fixed by its position in the tree, so the first log match by RunID
	// carries the correct parent depth.
	if msg.ParentRunID != "" {
		for _, m := range bus.MessageLog() {
			if m.RunID == msg.ParentRunID {
				if msg.Depth <= m.Depth {
					return ErrDepthNotMonotonic
				}
				break
			}
		}
	}

	// STORY-0014 AC-3 (ceiling): reject if depth >= maxDepth (depth is 0-indexed; maxDepth is the limit)
	if msg.Depth >= maxDepth {
		return ErrDepthExceeded
	}

	// All checks passed; emit the message to the bus.
	return bus.Emit(msg)
}

// ReconstructDelegationGraph builds a parent→children adjacency map from the message log (STORY-0012 AC-4).
// It uses ParentRunID to establish delegation edges. Returns a map[parentRunID][]childRunID.
// The graph captures the delegation HIERARCHY (who delegated to whom); request/response CorrelationID
// pairing is an orthogonal, application-level concern (the messages carry CorrelationID for callers to
// pair on, but it is not an edge in this hierarchy graph).
func ReconstructDelegationGraph(log []Message) map[string][]string {
	graph := make(map[string][]string)
	childSeen := make(map[string]bool) // track which children we've already added to avoid duplicates

	for _, msg := range log {
		// Skip root messages (no ParentRunID)
		if msg.ParentRunID == "" {
			continue
		}
		// Add an edge parent → child if not already added
		key := msg.ParentRunID + "->" + msg.RunID
		if !childSeen[key] {
			graph[msg.ParentRunID] = append(graph[msg.ParentRunID], msg.RunID)
			childSeen[key] = true
		}
	}

	return graph
}
