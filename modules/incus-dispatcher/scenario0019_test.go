package main

import (
	"testing"
)

// TestScenario0019_RecursiveDelegation proves STORY-0012 (message object + topics + graph)
// and STORY-0014 (cheap emission + depth limit) through the recursive delegation chain:
// coordinator emits research.request → research agent checks policy, emits web.fetch.request
// (with depth limit enforced) → web worker emits response → research agent correlates and emits
// research.response. Proves AC-1..4 of STORY-0012 and AC-1..4 of STORY-0014.
func TestScenario0019_RecursiveDelegation(t *testing.T) {
	// STORY-0012 AC-1: Message includes ThreadID, RunID, ParentRunID, Depth
	// STORY-0012 AC-2: Message includes CorrelationID
	// STORY-0014 AC-2: message routing preserves ThreadID, RunID, CorrelationID across emit→receive
	bus := NewMessageBus()

	// Coordinator emits research.request (ThreadID=research-1, RunID=coordinator-1)
	coordMsg := Message{
		ThreadID:      "research-1",
		RunID:         "coordinator-1",
		CorrelationID: "corr-root",
		Topic:         "research.request",
		Kind:          MessageKindRequest,
		Depth:         0,
		Payload:       "find recent trends in LLMs",
	}
	if err := bus.Emit(coordMsg); err != nil {
		t.Fatalf("coordinator emit to research.request failed: %v", err)
	}

	// Research agent receives the request
	msgs, err := bus.Receive("research.request")
	if err != nil {
		t.Fatalf("research.request Receive failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 research.request message, got %d", len(msgs))
	}
	researchReq := msgs[0]
	if researchReq.ThreadID != "research-1" {
		t.Fatalf("research.request lost ThreadID: got %q, want research-1", researchReq.ThreadID)
	}
	if researchReq.CorrelationID != "corr-root" {
		t.Fatalf("research.request lost CorrelationID: got %q, want corr-root", researchReq.CorrelationID)
	}

	// STORY-0014 AC-1: agent can emit work to request topic WHEN POLICY ALLOWS
	// Build a policy that allows research agent to emit to web.fetch.request but NOT to forbidden topics
	researchPolicy := ExecutionPolicy{
		Kind:            PolicyKindResearchBurst,
		DelegationRules: []string{"web.fetch.request", "wiki.update.request"}, // allowed topics
		Constraints:     map[string]string{"max_depth": "2"},
	}

	// Research agent emits child message to web.fetch.request (ParentRunID=coordinator-1, Depth=1)
	childMsg := Message{
		ThreadID:      "research-1", // preserved from parent
		RunID:         "research-1",
		ParentRunID:   "coordinator-1", // tracks parent
		CorrelationID: "corr-123",      // new correlation for this request chain
		Topic:         "web.fetch.request",
		Kind:          MessageKindRequest,
		Depth:         1, // incremented
		Payload:       "fetch LLM trend articles",
	}

	// STORY-0014 AC-1: emit succeeds because policy allows web.fetch.request
	const maxDepth = 2
	if err := EmitUnderPolicy(bus, researchPolicy, childMsg, maxDepth); err != nil {
		t.Fatalf("research agent emit to web.fetch.request failed (should be allowed): %v", err)
	}

	// STORY-0014 AC-3: a Depth field + max-depth limit PREVENTS unbounded recursion
	// Try to emit a message at max depth
	deepMsg := Message{
		ThreadID:      "research-1",
		RunID:         "web-fetch-1",
		ParentRunID:   "research-1",
		CorrelationID: "corr-456",
		Topic:         "web.fetch.request", // even allowed topic fails at max depth
		Kind:          MessageKindRequest,
		Depth:         maxDepth + 1, // beyond max
		Payload:       "another request",
	}
	if err := EmitUnderPolicy(bus, researchPolicy, deepMsg, maxDepth); err == nil {
		t.Fatalf("should reject emit at depth %d (max %d), but did not", maxDepth+1, maxDepth)
	}

	// STORY-0014 AC-1: emit to a disallowed topic is rejected
	forbiddenMsg := Message{
		ThreadID:      "research-1",
		RunID:         "research-1",
		ParentRunID:   "coordinator-1",
		CorrelationID: "corr-789",
		Topic:         "dangerous.request", // NOT in DelegationRules
		Kind:          MessageKindRequest,
		Depth:         1,
		Payload:       "try to escalate",
	}
	if err := EmitUnderPolicy(bus, researchPolicy, forbiddenMsg, maxDepth); err == nil {
		t.Fatalf("should reject emit to disallowed topic dangerous.request, but did not")
	}

	// STORY-0012 AC-3: the bus supports REQUEST/RESPONSE AND EVENT/STATUS topics
	// Web fetch worker emits a response back
	webResponseMsg := Message{
		ThreadID:      "research-1", // preserved
		RunID:         "web-fetch-1",
		ParentRunID:   "research-1",
		CorrelationID: "corr-123", // matches original request (AC-2)
		Topic:         "research.response",
		Kind:          MessageKindResponse,
		Depth:         1,
		Payload:       "articles found",
	}
	if err := bus.Emit(webResponseMsg); err != nil {
		t.Fatalf("web worker emit response failed: %v", err)
	}

	// Research agent receives the response
	respMsgs, err := bus.Receive("research.response")
	if err != nil {
		t.Fatalf("research.response Receive failed: %v", err)
	}
	if len(respMsgs) != 1 {
		t.Fatalf("expected 1 research.response message, got %d", len(respMsgs))
	}
	webResp := respMsgs[0]
	if webResp.CorrelationID != "corr-123" {
		t.Fatalf("response lost CorrelationID: got %q, want corr-123", webResp.CorrelationID)
	}

	// Research agent emits synthesized result
	synthesisMsg := Message{
		ThreadID:      "research-1",
		RunID:         "research-1",
		CorrelationID: "corr-123", // same correlation as the child chain
		Topic:         "research.response",
		Kind:          MessageKindResponse,
		Depth:         0, // back to root level
		Payload:       "synthesized results",
	}
	if err := bus.Emit(synthesisMsg); err != nil {
		t.Fatalf("research agent emit synthesis failed: %v", err)
	}

	// Coordinator receives the final result
	// Note: after the first Receive above, the topic was cleared, so the synthesis msg is the only one left
	finalMsgs, err := bus.Receive("research.response")
	if err != nil {
		t.Fatalf("final research.response Receive failed: %v", err)
	}
	if len(finalMsgs) != 1 { // only the synthesis msg remains (web response was received earlier)
		t.Fatalf("expected 1 research.response message (synthesis), got %d", len(finalMsgs))
	}

	// STORY-0012 AC-4: delegate graph is reconstructible from ParentRunID + correlation metadata
	log := bus.MessageLog()
	graph := ReconstructDelegationGraph(log)

	// Graph should show coordinator-1 → research-1 and research-1 → web-fetch-1
	if !hasEdge(graph, "coordinator-1", "research-1") {
		t.Fatalf("delegation graph missing edge coordinator-1 → research-1")
	}
	if !hasEdge(graph, "research-1", "web-fetch-1") {
		t.Fatalf("delegation graph missing edge research-1 → web-fetch-1")
	}

	// Correlation pairs: corr-123 should link request → response
	if !hasCorrelationPair(log, "corr-123", "web.fetch.request", "research.response") {
		t.Fatalf("correlation corr-123 does not pair web.fetch.request with research.response")
	}

	// STORY-0014 AC-2: routing preserves ThreadID, RunID, CorrelationID
	allMsgs := bus.MessageLog()
	for _, msg := range allMsgs {
		if msg.ThreadID != "research-1" {
			t.Fatalf("ThreadID not preserved in message: got %q, want research-1", msg.ThreadID)
		}
		// Every message has a RunID, CorrelationID
		if msg.RunID == "" {
			t.Fatalf("empty RunID in message")
		}
		if msg.CorrelationID == "" {
			t.Fatalf("empty CorrelationID in message")
		}
	}

	// No heavyweight in-memory orchestration: the messages went through the bus, no direct calls
	t.Logf("SCENARIO-0019 PASSED: recursive delegation via message emission works. %d messages in log.", len(allMsgs))
}

// hasEdge checks if the graph contains a parent → child edge
func hasEdge(graph map[string][]string, parent, child string) bool {
	children, ok := graph[parent]
	if !ok {
		return false
	}
	for _, c := range children {
		if c == child {
			return true
		}
	}
	return false
}

// hasCorrelationPair checks if the message log contains a request on topicA and response on topicB
// with matching correlationID (proving they are paired)
func hasCorrelationPair(log []Message, corrID, topicA, topicB string) bool {
	var hasReq, hasResp bool
	for _, msg := range log {
		if msg.CorrelationID == corrID && msg.Topic == topicA {
			hasReq = true
		}
		if msg.CorrelationID == corrID && msg.Topic == topicB {
			hasResp = true
		}
	}
	return hasReq && hasResp
}
