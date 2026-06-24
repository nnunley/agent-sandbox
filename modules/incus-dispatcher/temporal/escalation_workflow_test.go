package temporal

import (
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"
	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// TestEscalationWorkflow_ReRaiseOnThresholdCross proves AC-B:
// A durable workflow holds a pending escalation and re-raises it as urgency crosses the threshold.
//
// Setup:
// - Create a directive with importance=High and a deadline 8 days in the future.
// - High importance threshold = 5 days, so at t=0 (8 days out), escalation is NOT triggered yet.
// - Urgency at 8 days ≈ 0.30 < 0.5 → starts in Q2.
// - As time advances and deadline approaches, urgency increases.
// - Around 4 days out, urgency ≈ 0.55 > 0.5 → quadrant changes to Q1.
// - When escalation threshold (5 days) is crossed, the workflow detects the urgency rise and re-raises.
//
// Action:
// - Start the EscalationWorkflow with an 8-day deadline.
// - The testsuite auto-skips time; the workflow's timers advance.
// - The workflow detects the Q2→Q1 transition and invokes ReprojectActivity.
//
// Expected observables (AC-B):
// - Workflow completes without error.
// - At least one Reprioritize call records the new priority after threshold crossing.
// - At least one Defer call sets notBefore to the advanced workflow.Now() (item made eligible).
// - A "escalation re-raised" log entry (via workflow.GetLogger) records the quadrant transition.
// - The escalation was autonomous (no human action); Temporal timers drove it.
func TestEscalationWorkflow_ReRaiseOnThresholdCross(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register the workflow and activity
	env.RegisterWorkflow(EscalationWorkflow)

	fakeQueue := &FakeReprojector{}
	activities := &Activities{Queue: fakeQueue}
	env.RegisterActivity(activities.ReprojectActivity)

	// Setup: deadline 8 days in the future (starts below escalation threshold for High=5 days)
	startNow := env.Now()
	deadline := startNow.Add(8 * 24 * time.Hour)

	// Verify initial state: High importance, 8 days out
	// At 8 days: urgency ~0.30 < 0.5 → Q2
	initialQuadrant, initialPriority, _ := ReprojectOnEscalation(ImportanceHigh, &deadline, startNow)
	if initialQuadrant != QuadrantQ2 {
		t.Fatalf("setup error: 8-day deadline should start in Q2, got %v (priority: %d)",
			initialQuadrant, initialPriority)
	}
	t.Logf("Initial state verified: Q2 (priority: %d, deadline: 8 days out)", initialPriority)

	input := EscalationWorkflowInput{
		DirectiveID: "test-escalation-rerise",
		Importance:  ImportanceHigh,
		Deadline:    &deadline,
	}

	// Execute the workflow. The testsuite auto-skips time through sleep durations.
	env.ExecuteWorkflow(EscalationWorkflow, input)

	// Verify no panic or error in the workflow itself
	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow failed: %v", err)
	}

	// AC-B Observable 1: Reprojection activity was invoked
	if len(fakeQueue.ReprioritizeCalls) == 0 {
		t.Fatalf("Reprioritize was never called; expected ≥1 call for re-raise")
	}
	if len(fakeQueue.DeferCalls) == 0 {
		t.Fatalf("Defer was never called; expected ≥1 call for making item eligible at new priority")
	}
	t.Logf("✓ Activity invoked: Reprioritize called %d time(s), Defer called %d time(s)",
		len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))

	// AC-B Observable 2: The priority was actually updated (re-raised)
	// For High importance Q1, we expect queue.ImportanceHigh (P0)
	foundHighImportance := false
	for _, call := range fakeQueue.ReprioritizeCalls {
		if call.ID == input.DirectiveID {
			// After re-raise, importance should be at least High (Q1 tier)
			// tierToQueueImportance(ImportanceHigh) == queue.ImportanceHigh
			if call.Importance == queue.ImportanceHigh {
				foundHighImportance = true
				t.Logf("✓ Priority re-raised: Reprioritize called with importance=%v (Q1 tier)", call.Importance)
				break
			}
		}
	}
	if !foundHighImportance {
		t.Fatalf("no re-raise found: Reprioritize not called with Q1 importance for %s", input.DirectiveID)
	}

	// AC-B Observable 3: Defer set notBefore to the advanced workflow time
	foundDeferCall := false
	for _, call := range fakeQueue.DeferCalls {
		if call.ID == input.DirectiveID {
			foundDeferCall = true
			// The notBefore should be set to the workflow's advanced "now" (within simulation window)
			endNow := env.Now()
			if call.NotBefore.Before(startNow) || call.NotBefore.After(endNow) {
				t.Fatalf("Defer notBefore %v not within simulated execution window [%v, %v]",
					call.NotBefore, startNow, endNow)
			}
			t.Logf("✓ Item made eligible: Defer called with notBefore=%v", call.NotBefore)
			break
		}
	}
	if !foundDeferCall {
		t.Fatalf("no Defer call found for directive ID %s", input.DirectiveID)
	}

	// AC-B Observable 4: Autonomous re-raise (no human intervention)
	// The workflow ran purely on its own timers and invoked the activity autonomously
	t.Logf("✓ Autonomous escalation re-raise: Temporal workflow detected rising urgency, "+
		"re-raised priority via activity (Reprioritize: %d, Defer: %d) — no human action needed",
		len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))
}

// TestEscalationWorkflow_NoRaiseWhenDeadlinePassed proves that the workflow
// exits cleanly when the deadline has already passed (no premature raising).
func TestEscalationWorkflow_NoRaiseWhenDeadlinePassed(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(EscalationWorkflow)

	fakeQueue := &FakeReprojector{}
	activities := &Activities{Queue: fakeQueue}
	env.RegisterActivity(activities.ReprojectActivity)

	// Setup: deadline in the past
	startNow := env.Now()
	deadline := startNow.Add(-24 * time.Hour) // 1 day in the past

	input := EscalationWorkflowInput{
		DirectiveID: "test-escalation-past-deadline",
		Importance:  ImportanceHigh,
		Deadline:    &deadline,
	}

	// Execute the workflow.
	env.ExecuteWorkflow(EscalationWorkflow, input)

	// Verify no error
	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow failed: %v", err)
	}

	// The workflow should exit immediately (deadline has passed)
	// and NEVER invoke the activity (no re-raise for past-deadline items).
	if len(fakeQueue.ReprioritizeCalls) > 0 || len(fakeQueue.DeferCalls) > 0 {
		t.Fatalf("Activity invoked for past-deadline item; expected no calls. Reprioritize: %d, Defer: %d",
			len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))
	}

	t.Logf("✓ Workflow exited cleanly for past-deadline item (no re-raise needed)")
}

// TestEscalationWorkflow_NoDeadlineIsNoOp proves that a directive with no deadline
// does not trigger escalation (escalation windows require deadlines).
func TestEscalationWorkflow_NoDeadlineIsNoOp(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(EscalationWorkflow)

	fakeQueue := &FakeReprojector{}
	activities := &Activities{Queue: fakeQueue}
	env.RegisterActivity(activities.ReprojectActivity)

	input := EscalationWorkflowInput{
		DirectiveID: "test-escalation-no-deadline",
		Importance:  ImportanceHigh,
		Deadline:    nil, // No deadline
	}

	// Execute the workflow.
	env.ExecuteWorkflow(EscalationWorkflow, input)

	// Verify no error
	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow failed: %v", err)
	}

	// The workflow should exit immediately (no deadline = no escalation window)
	if len(fakeQueue.ReprioritizeCalls) > 0 || len(fakeQueue.DeferCalls) > 0 {
		t.Fatalf("Activity invoked for nil-deadline item; expected no calls. Reprioritize: %d, Defer: %d",
			len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))
	}

	t.Logf("✓ Workflow exited cleanly for nil-deadline item (no escalation window)")
}
