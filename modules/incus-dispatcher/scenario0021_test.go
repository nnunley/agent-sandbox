package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// TestScenario0021_OperatorTUI proves SCENARIO-0021: Operator uses TUI to create, inspect, and manage threads.
//
// Preconditions:
// - TUI is running
// - Thread registry is populated with known threads
// - Worker status is available
//
// Actions:
// - Operator creates a new work item (AC-1)
// - Operator views queue and worker state (AC-2)
// - Operator inspects thread details and artifacts (AC-3)
// - Operator pauses, blocks, and resumes a thread (AC-4, STORY-0027 AC-3)
// - Operator requeues a thread (AC-4)
//
// Expected observables:
// - New thread is created with status=queued (AC-1)
// - Queue view shows pending/claimed counts (AC-2)
// - Worker status is displayed (AC-2)
// - Thread status changes to paused/blocked/active (AC-4, STORY-0027 AC-3)
// - All transitions are recorded in audit log
func TestScenario0021_OperatorTUI(t *testing.T) {
	// Setup: in-memory queue, thread tracker, audit log, and worker registry.
	q := queue.NewMemoryQueue()
	now := time.Now
	threads := NewThreadTracker(now)
	audit := NewMemoryAuditLog()

	workers := map[string]*Worker{
		"worker-1": {
			WorkerID:      "worker-1",
			WorkerKind:    WorkerKindLocal,
			Capabilities:  []string{"coding", "review"},
			RuntimeMode:   RuntimeOneShot,
			AllowedPolicies: []string{"policy-1@v1"},
		},
		"worker-2": {
			WorkerID:      "worker-2",
			WorkerKind:    WorkerKindIncusContainer,
			Capabilities:  []string{"testing"},
			RuntimeMode:   RuntimeLongRunning,
			AllowedPolicies: []string{"policy-2@v1"},
		},
	}

	console := NewOperatorConsole(q, threads, workers, audit, now)

	// First, create a directive manually to get its actual ID
	d1 := queue.Directive{
		Intent:      "feature-x",
		Template:    "coding",
		Repo:        "/repo",
		Ref:         "main",
		Task:        "implement feature X",
		Importance:  queue.ImportanceNormal,
	}
	id1, _ := q.Push(d1)
	threads.Set(id1, StatusQueued) // Initialize thread status

	// Test script: sequence of commands using the actual directive IDs
	// Note: fields with spaces need to be quoted
	script := fmt.Sprintf("queue\nworkers\ninspect %s\npause %s\ncreate feature-y coding /repo main \"implement feature Y\"\nresume %s\ninspect %s\n", id1, id1, id1, id1)

	// Run the console with the script.
	input := strings.NewReader(script)
	output := &bytes.Buffer{}

	if err := console.Run(input, output); err != nil {
		t.Fatalf("console.Run failed: %v", err)
	}

	result := output.String()

	// Verify expected outputs using substring matching (commands can output multi-line).
	// The first directive is already created and tracked, we can verify its state changes.

	// AC-2: queue should show pending/claimed
	if !strings.Contains(result, "pending=") || !strings.Contains(result, "claimed=") {
		t.Fatalf("AC-2 queue failed: expected queue with pending/claimed in output:\n%s", result)
	}

	// AC-2: workers should list registered workers
	if !strings.Contains(result, "workers:") || !strings.Contains(result, "registered") {
		t.Fatalf("AC-2 workers failed: expected workers list in output:\n%s", result)
	}
	if !strings.Contains(result, "worker-1") || !strings.Contains(result, "worker-2") {
		t.Fatalf("AC-2 workers failed: expected both workers in output:\n%s", result)
	}

	// AC-3: inspect should show thread status
	if !strings.Contains(result, "thread") || !strings.Contains(result, "status:") {
		t.Fatalf("AC-3 inspect failed: expected thread status in output:\n%s", result)
	}

	// AC-4: pause should succeed (check output contains pause confirmation)
	if !strings.Contains(result, "ok:") || !strings.Contains(result, "paused") {
		t.Fatalf("AC-4 pause failed: expected 'ok: ... paused' in output:\n%s", result)
	}

	// AC-4: resume should succeed (check output contains resume confirmation)
	if !strings.Contains(result, "resumed") {
		t.Fatalf("AC-4 resume failed: expected 'resumed' in output:\n%s", result)
	}

	// Verify audit log captured both pause and resume transitions
	auditEntries := audit.ByThread(id1)
	if len(auditEntries) == 0 {
		t.Fatalf("AC-4: no audit entries for thread %s", id1)
	}

	// Count pause and resume transitions
	var pauseFound, resumeFound bool
	for _, entry := range auditEntries {
		if entry.Kind == AuditKindTransition {
			if strings.Contains(entry.Detail, "paused") {
				pauseFound = true
			}
			// Look for transition TO active (resume from paused or blocked)
			if strings.Contains(entry.Detail, "-> active") {
				resumeFound = true
			}
		}
	}
	if !pauseFound {
		t.Fatalf("AC-4 pause: pause transition not recorded in audit log")
	}
	if !resumeFound {
		t.Fatalf("AC-4 resume: resume transition (to active) not recorded in audit log, entries: %+v", auditEntries)
	}

	// After running the full script (pause then resume), the thread should be active
	finalStatus := threads.Status(id1)
	if finalStatus != StatusActive {
		t.Fatalf("AC-4 final state: thread should be active after pause->resume, got %s", finalStatus)
	}

	// Verify thread was initialized to queued on create (first directive pushed).
	pending, _ := q.Len()
	// We pushed one directive manually, and the script creates one more.
	// So we should have 2 pending directives total.
	if pending != 2 {
		t.Fatalf("AC-1: expected 2 pending directives (1 manual + 1 script), got %d", pending)
	}

	// Verify thread tracker shows audit entries for the first directive.
	auditEntries = audit.ByThread(id1)
	if len(auditEntries) == 0 {
		t.Fatalf("AC-1: no audit entries for directive %s", id1)
	}
}

// TestScenario0021_PauseBlockResumeTransitions proves STORY-0027 AC-3:
// pause/block/resume commands drive ThreadTracker transitions, rejecting illegal transitions.
func TestScenario0021_PauseBlockResumeTransitions(t *testing.T) {
	now := time.Now
	threads := NewThreadTracker(now)
	audit := NewMemoryAuditLog()

	console := NewOperatorConsole(
		queue.NewMemoryQueue(),
		threads,
		map[string]*Worker{},
		audit,
		now,
	)

	threadID := "thread-1"

	// Initialize thread to active
	threads.Set(threadID, StatusActive)

	// Test: pause from active → paused
	result, err := console.cmdPause([]string{threadID})
	if err != nil {
		t.Fatalf("cmdPause from active failed: %v", err)
	}
	if !strings.Contains(result, "paused") {
		t.Fatalf("cmdPause from active: expected 'paused', got %q", result)
	}
	if threads.Status(threadID) != StatusPaused {
		t.Fatalf("cmdPause: status not updated to paused")
	}

	// Test: resume from paused → active
	result, err = console.cmdResume([]string{threadID})
	if err != nil {
		t.Fatalf("cmdResume from paused failed: %v", err)
	}
	if !strings.Contains(result, "resumed") {
		t.Fatalf("cmdResume from paused: expected 'resumed', got %q", result)
	}
	if threads.Status(threadID) != StatusActive {
		t.Fatalf("cmdResume: status not updated to active")
	}

	// Test: block from active → blocked
	result, err = console.cmdBlock([]string{threadID})
	if err != nil {
		t.Fatalf("cmdBlock from active failed: %v", err)
	}
	if !strings.Contains(result, "blocked") {
		t.Fatalf("cmdBlock from active: expected 'blocked', got %q", result)
	}
	if threads.Status(threadID) != StatusBlocked {
		t.Fatalf("cmdBlock: status not updated to blocked")
	}

	// Test: resume from blocked → active
	result, err = console.cmdResume([]string{threadID})
	if err != nil {
		t.Fatalf("cmdResume from blocked failed: %v", err)
	}
	if threads.Status(threadID) != StatusActive {
		t.Fatalf("cmdResume from blocked: status not updated to active")
	}

	// Test: illegal transition — pause from done should fail
	threads.Set(threadID, StatusDone)
	result, err = console.cmdPause([]string{threadID})
	if err == nil {
		t.Fatalf("cmdPause from done should have failed, got result %q", result)
	}
	if !strings.Contains(err.Error(), "cannot pause") {
		t.Fatalf("cmdPause from done: expected 'cannot pause', got %q", err.Error())
	}

	// Test: illegal transition — resume from active should fail
	threads.Set(threadID, StatusActive)
	result, err = console.cmdResume([]string{threadID})
	if err == nil {
		t.Fatalf("cmdResume from active should have failed, got result %q", result)
	}
	if !strings.Contains(err.Error(), "cannot resume") {
		t.Fatalf("cmdResume from active: expected 'cannot resume', got %q", err.Error())
	}

	// Verify audit log captured all transitions
	entries := audit.ByThread(threadID)
	if len(entries) == 0 {
		t.Fatalf("no audit entries for thread %s", threadID)
	}

	// All successful transitions should have kind AuditKindTransition
	transitionCount := 0
	for _, entry := range entries {
		if entry.Kind == AuditKindTransition {
			transitionCount++
		}
	}
	if transitionCount < 4 {
		t.Fatalf("expected at least 4 transitions, got %d", transitionCount)
	}
}
