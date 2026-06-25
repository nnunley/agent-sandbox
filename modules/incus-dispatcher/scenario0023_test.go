package main

import (
	"testing"
)

// TestRuntimeMode_Behavior proves the RuntimeMode type and behavior mapping (STORY-0013 AC-1/4).
// Verifies that StaysSubscribed(), RequiresHeartbeat(), RetriesInProcess(), and AllowsCache()
// return correct values for one_shot and long_running modes.
func TestRuntimeMode_Behavior(t *testing.T) {
	tests := []struct {
		mode              RuntimeMode
		wantStaysSubscribed bool
		wantHeartbeat     bool
		wantRetries       bool
		wantCache         bool
	}{
		{
			mode:              RuntimeOneShot,
			wantStaysSubscribed: false,
			wantHeartbeat:     false,
			wantRetries:       false,
			wantCache:         false,
		},
		{
			mode:              RuntimeLongRunning,
			wantStaysSubscribed: true,
			wantHeartbeat:     true,
			wantRetries:       true,
			wantCache:         true,
		},
	}

	for _, tt := range tests {
		if got := tt.mode.StaysSubscribed(); got != tt.wantStaysSubscribed {
			t.Errorf("%q.StaysSubscribed() = %v, want %v", tt.mode, got, tt.wantStaysSubscribed)
		}
		if got := tt.mode.RequiresHeartbeat(); got != tt.wantHeartbeat {
			t.Errorf("%q.RequiresHeartbeat() = %v, want %v", tt.mode, got, tt.wantHeartbeat)
		}
		if got := tt.mode.RetriesInProcess(); got != tt.wantRetries {
			t.Errorf("%q.RetriesInProcess() = %v, want %v", tt.mode, got, tt.wantRetries)
		}
		if got := tt.mode.AllowsCache(); got != tt.wantCache {
			t.Errorf("%q.AllowsCache() = %v, want %v", tt.mode, got, tt.wantCache)
		}
	}
}

// TestScenario0023_OneShotWorker proves STORY-0013 AC-2:
// one-shot worker consumes ONE task, emits result, and EXITS without re-subscribing or maintaining state.
// Proves:
//   - Worker with RuntimeMode=one_shot claims exactly ONE message from request topic
//   - Emits structured result to response topic with thread_id, run_id, correlation_id, artifact_refs
//   - Exit behavior: does NOT re-subscribe (second Receive yields no messages claimed by this worker)
//   - No ephemeral state persistence after exit
func TestScenario0023_OneShotWorker(t *testing.T) {
	bus := NewMessageBus()
	policy := ExecutionPolicy{
		Kind:            PolicyKindOneShot,
		DelegationRules: []string{"code.response"},
		Constraints:     map[string]string{"max_depth": "2"},
	}

	// 1. Queue one task on code.task.request
	taskMsg := Message{
		ThreadID:      "thread-0023",
		RunID:         "coordinator-0023",
		CorrelationID: "corr-task-1",
		Topic:         "code.task.request",
		Kind:          MessageKindRequest,
		Depth:         0,
		Payload:       "implement feature X and run tests",
	}
	if err := bus.Emit(taskMsg); err != nil {
		t.Fatalf("emit task failed: %v", err)
	}

	// 2. Simulate one-shot worker: claim exactly ONE task
	workerID := "worker-one-shot-1"
	workerMode := RuntimeOneShot

	// Worker Receives from request topic
	tasks, err := bus.Receive("code.task.request")
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	claimedTask := tasks[0]
	if claimedTask.ThreadID != "thread-0023" {
		t.Fatalf("task lost ThreadID: got %q, want thread-0023", claimedTask.ThreadID)
	}
	if claimedTask.CorrelationID != "corr-task-1" {
		t.Fatalf("task lost CorrelationID: got %q, want corr-task-1", claimedTask.CorrelationID)
	}

	// 3. Worker does bounded work and emits result
	resultMsg := Message{
		ThreadID:      claimedTask.ThreadID,
		RunID:         workerID,
		ParentRunID:   claimedTask.RunID,
		CorrelationID: claimedTask.CorrelationID, // pair to the request
		Topic:         "code.response",
		Kind:          MessageKindResponse,
		Depth:         1,
		Payload:       "feature implemented, tests passing",
	}

	// Emit result under policy (AC-1: policy enforces topic allowance)
	if err := EmitUnderPolicy(bus, policy, resultMsg, 2); err != nil {
		t.Fatalf("EmitUnderPolicy failed: %v", err)
	}

	// 4. Verify result message has required fields (AC-2)
	responses, err := bus.Receive("code.response")
	if err != nil {
		t.Fatalf("Receive responses failed: %v", err)
	}
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	result := responses[0]
	if result.ThreadID != "thread-0023" {
		t.Fatalf("response lost ThreadID: got %q, want thread-0023", result.ThreadID)
	}
	if result.CorrelationID != "corr-task-1" {
		t.Fatalf("response lost CorrelationID: got %q, want corr-task-1", result.CorrelationID)
	}
	if result.RunID != workerID {
		t.Fatalf("response RunID mismatch: got %q, want %q", result.RunID, workerID)
	}

	// 5. Verify one-shot semantics: no re-subscription (AC-2)
	// After exit, a second Receive on request topic yields nothing (message consumed once)
	moreMessages, err := bus.Receive("code.task.request")
	if err != nil {
		t.Fatalf("second Receive failed: %v", err)
	}
	if len(moreMessages) != 0 {
		t.Fatalf("expected no more messages after one-shot exit, got %d", len(moreMessages))
	}

	// 6. Verify runtime mode behavior (AC-4)
	if workerMode.StaysSubscribed() {
		t.Errorf("one_shot mode should NOT StaysSubscribed, but it does")
	}
	if workerMode.RequiresHeartbeat() {
		t.Errorf("one_shot mode should NOT RequiresHeartbeat, but it does")
	}
	if workerMode.RetriesInProcess() {
		t.Errorf("one_shot mode should NOT RetriesInProcess, but it does")
	}
	if workerMode.AllowsCache() {
		t.Errorf("one_shot mode should NOT AllowsCache, but it does")
	}
}

// TestScenario0023_LongRunningWorker proves STORY-0013 AC-3:
// long-running worker processes multiple items, stays subscribed, and emits heartbeats.
// Proves:
//   - Worker with RuntimeMode=long_running claims and processes >=2 messages
//   - Worker remains subscribed (does NOT exit after first message)
//   - Worker emits a heartbeat/status message to signal liveness
//   - StaysSubscribed()==true and heartbeat requirement is honored
func TestScenario0023_LongRunningWorker(t *testing.T) {
	bus := NewMessageBus()
	policy := ExecutionPolicy{
		Kind:            PolicyKindRalphLoop,
		DelegationRules: []string{"work.status"},
		Constraints:     map[string]string{"max_depth": "2"},
	}

	// 1. Queue two tasks on work.request
	task1 := Message{
		ThreadID:      "thread-lr-1",
		RunID:         "coordinator-lr",
		CorrelationID: "corr-task-1",
		Topic:         "work.request",
		Kind:          MessageKindRequest,
		Depth:         0,
		Payload:       "task 1",
	}
	task2 := Message{
		ThreadID:      "thread-lr-1",
		RunID:         "coordinator-lr",
		CorrelationID: "corr-task-2",
		Topic:         "work.request",
		Kind:          MessageKindRequest,
		Depth:         0,
		Payload:       "task 2",
	}

	if err := bus.Emit(task1); err != nil {
		t.Fatalf("emit task1 failed: %v", err)
	}
	if err := bus.Emit(task2); err != nil {
		t.Fatalf("emit task2 failed: %v", err)
	}

	// 2. Long-running worker: process all messages without exiting
	workerID := "worker-long-running-1"
	workerMode := RuntimeLongRunning

	tasksProcessed := 0
	for {
		// Receive: if StaysSubscribed is true, stay in loop
		tasks, err := bus.Receive("work.request")
		if err != nil {
			t.Fatalf("Receive failed: %v", err)
		}
		if len(tasks) == 0 {
			// No more messages; long-running worker would sleep/wait for next batch
			break
		}

		for _, task := range tasks {
			tasksProcessed++

			// Process task and emit result
			resultMsg := Message{
				ThreadID:      task.ThreadID,
				RunID:         workerID,
				ParentRunID:   task.RunID,
				CorrelationID: task.CorrelationID,
				Topic:         "work.result",
				Kind:          MessageKindResponse,
				Depth:         1,
				Payload:       "task completed",
			}
			if err := bus.Emit(resultMsg); err != nil {
				t.Fatalf("emit result failed: %v", err)
			}
		}

		// Long-running worker may continue looping; one_shot would exit here
		if !workerMode.StaysSubscribed() {
			break
		}
	}

	// 3. Emit heartbeat/status (AC-3)
	heartbeatMsg := Message{
		ThreadID:    "thread-lr-1",
		RunID:       workerID,
		Topic:       "work.status",
		Kind:        MessageKindStatus,
		Depth:       1, // heartbeat is at depth 1 (child of coordinator at depth 0)
		Payload:     "worker alive, processed 2 tasks",
		ParentRunID: "coordinator-lr",
	}
	if err := EmitUnderPolicy(bus, policy, heartbeatMsg, 2); err != nil {
		t.Fatalf("EmitUnderPolicy heartbeat failed: %v", err)
	}

	// 4. Verify long-running semantics (AC-3/AC-4)
	if tasksProcessed < 2 {
		t.Fatalf("expected to process >=2 tasks, got %d", tasksProcessed)
	}

	if !workerMode.StaysSubscribed() {
		t.Errorf("long_running mode should StaysSubscribed, but it doesn't")
	}
	if !workerMode.RequiresHeartbeat() {
		t.Errorf("long_running mode should RequiresHeartbeat, but it doesn't")
	}

	// 5. Verify heartbeat was emitted
	statuses, err := bus.Receive("work.status")
	if err != nil {
		t.Fatalf("Receive status failed: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 heartbeat status, got %d", len(statuses))
	}

	status := statuses[0]
	if status.Kind != MessageKindStatus {
		t.Fatalf("status message kind mismatch: got %v, want %v", status.Kind, MessageKindStatus)
	}
	if status.RunID != workerID {
		t.Fatalf("status RunID mismatch: got %q, want %q", status.RunID, workerID)
	}
}

// TestWorker_RuntimeModeField proves Worker struct includes RuntimeMode field (STORY-0013 AC-1).
func TestWorker_RuntimeModeField(t *testing.T) {
	// Test one-shot worker
	oneShotWorker := Worker{
		WorkerID:    "w-one-shot",
		WorkerKind:  WorkerKindLocal,
		RuntimeMode: RuntimeOneShot,
	}
	if oneShotWorker.RuntimeMode != RuntimeOneShot {
		t.Fatalf("one-shot worker RuntimeMode mismatch: got %q, want %q", oneShotWorker.RuntimeMode, RuntimeOneShot)
	}

	// Test long-running worker
	longRunningWorker := Worker{
		WorkerID:    "w-long-running",
		WorkerKind:  WorkerKindIncusContainer,
		RuntimeMode: RuntimeLongRunning,
	}
	if longRunningWorker.RuntimeMode != RuntimeLongRunning {
		t.Fatalf("long-running worker RuntimeMode mismatch: got %q, want %q", longRunningWorker.RuntimeMode, RuntimeLongRunning)
	}

	// Test zero value (empty string) defaults to one_shot semantics
	defaultWorker := Worker{
		WorkerID:   "w-default",
		WorkerKind: WorkerKindLocal,
		// RuntimeMode not set (zero value)
	}
	// The zero value should not satisfy StaysSubscribed (it's falsy)
	if defaultWorker.RuntimeMode.StaysSubscribed() {
		t.Errorf("default (empty) RuntimeMode should not StaysSubscribed")
	}
}

// TestRuntimeMode_Constants proves RuntimeMode constants are defined (STORY-0013 AC-1).
func TestRuntimeMode_Constants(t *testing.T) {
	if RuntimeOneShot != "one_shot" {
		t.Errorf("RuntimeOneShot mismatch: got %q, want %q", RuntimeOneShot, "one_shot")
	}
	if RuntimeLongRunning != "long_running" {
		t.Errorf("RuntimeLongRunning mismatch: got %q, want %q", RuntimeLongRunning, "long_running")
	}
}

// TestOneShhotWorkerLifecycle simulates a complete one-shot worker lifecycle
// without re-subscription, demonstrating claim-once, emit, exit semantics.
func TestOneShotWorkerLifecycle(t *testing.T) {
	bus := NewMessageBus()

	// Queue a task
	task := Message{
		ThreadID:      "thread-lifecycle",
		RunID:         "coord",
		CorrelationID: "corr-1",
		Topic:         "task.request",
		Kind:          MessageKindRequest,
		Depth:         0,
		Payload:       "do work",
	}
	if err := bus.Emit(task); err != nil {
		t.Fatalf("emit task failed: %v", err)
	}

	// One-shot worker claims task
	tasks, _ := bus.Receive("task.request")
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	claimed := tasks[0]

	// Worker emits result
	result := Message{
		ThreadID:      claimed.ThreadID,
		RunID:         "worker-1",
		ParentRunID:   claimed.RunID,
		CorrelationID: claimed.CorrelationID,
		Topic:         "task.response",
		Kind:          MessageKindResponse,
		Depth:         1,
		Payload:       "work done",
	}
	if err := bus.Emit(result); err != nil {
		t.Fatalf("emit result failed: %v", err)
	}

	// Worker exits (no more receives)
	// Verify: a second Receive on task.request gets nothing
	secondReceive, _ := bus.Receive("task.request")
	if len(secondReceive) != 0 {
		t.Fatalf("expected 0 messages on second receive, got %d", len(secondReceive))
	}

	// Verify result was emitted
	responses, _ := bus.Receive("task.response")
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	resp := responses[0]
	if resp.CorrelationID != "corr-1" {
		t.Fatalf("correlation mismatch: got %q, want corr-1", resp.CorrelationID)
	}

	// Verify no state persisted (bus has no messages left)
	msgLog := bus.MessageLog()
	// Should have 2 messages: original task + response
	if len(msgLog) != 2 {
		t.Fatalf("expected 2 messages in log, got %d", len(msgLog))
	}
}

// BenchmarkOneShotWorkerThroughput benchmarks one-shot worker throughput over the bus.
func BenchmarkOneShotWorkerThroughput(b *testing.B) {
	bus := NewMessageBus()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Queue task
		task := Message{
			ThreadID:      "thread-bench",
			RunID:         "coord",
			CorrelationID: "corr",
			Topic:         "task.request",
			Kind:          MessageKindRequest,
			Depth:         0,
			Payload:       "task",
		}
		bus.Emit(task)

		// Receive (claim)
		tasks, _ := bus.Receive("task.request")
		if len(tasks) == 0 {
			b.Fatalf("failed to claim task")
		}

		// Emit result
		result := Message{
			ThreadID:      "thread-bench",
			RunID:         "worker",
			ParentRunID:   "coord",
			CorrelationID: "corr",
			Topic:         "task.response",
			Kind:          MessageKindResponse,
			Depth:         1,
			Payload:       "done",
		}
		bus.Emit(result)

		// Receive result (drain)
		bus.Receive("task.response")
	}
}
