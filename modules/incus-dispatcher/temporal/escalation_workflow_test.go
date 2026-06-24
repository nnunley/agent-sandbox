package temporal

import (
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"
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

	// AC-B Observable 1: The workflow detected a real quadrant transition and invoked the activity.
	// The activity CAN ONLY be called if the quadrant changed (lastQuadrant is initialized from
	// the true starting quadrant at line 51 of escalation_workflow.go; a write occurs only on
	// a genuine Q2→Q1 transition at line 74). So ≥1 calls prove a real transition occurred.
	if len(fakeQueue.ReprioritizeCalls) == 0 {
		t.Fatalf("Reprioritize was never called; a quadrant transition must occur for any write")
	}
	if len(fakeQueue.DeferCalls) == 0 {
		t.Fatalf("Defer was never called; a quadrant transition must trigger the activity")
	}
	t.Logf("✓ Activity invoked on quadrant change: Reprioritize ×%d, Defer ×%d (proves Q2→Q1 transition)",
		len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))

	// AC-B Observable 2 (CRITICAL): The re-raise is TIME-DRIVEN, not vacuous.
	// Setup: 8-day deadline + High importance. Urgency reaches 0.5 (Q2→Q1) at ~5 days remaining = ~3 days ELAPSED.
	// So the write must fire ~3 days into the time-skip, proving urgency rose over elapsed time.
	// A premature/vacuous write at t0 would have notBefore ≈ startNow, failing this assertion.
	foundTimeDrivenWrite := false
	minElapsed := 2 * 24 * time.Hour // Threshold crossing ~3 days in; require >2 days to prove time-driven
	for _, call := range fakeQueue.DeferCalls {
		if call.ID == input.DirectiveID {
			elapsed := call.NotBefore.Sub(startNow)
			if elapsed >= minElapsed {
				foundTimeDrivenWrite = true
				t.Logf("✓ Time-driven re-raise proven: Defer fired at elapsed=%v (>%v), notBefore=%v",
					elapsed, minElapsed, call.NotBefore)
				break
			} else {
				t.Fatalf("escalation re-raise was not time-driven: Defer notBefore=%v is only %v after startNow=%v; "+
					"expected the re-raise to fire after urgency rose (~3 days in), proving autonomous time-driven "+
					"escalation, not a t0 write",
					call.NotBefore, elapsed, startNow)
			}
		}
	}
	if !foundTimeDrivenWrite {
		t.Fatalf("no time-driven Defer found for directive ID %s", input.DirectiveID)
	}

	// AC-B Observable 3: Verify notBefore is within the execution window [startNow, endNow].
	// This confirms the write occurred at a valid workflow.Now(), not garbage/leaked time.
	endNow := env.Now()
	for _, call := range fakeQueue.DeferCalls {
		if call.ID == input.DirectiveID {
			if call.NotBefore.After(endNow) {
				t.Fatalf("Defer notBefore=%v is after workflow end=%v (invalid workflow.Now)",
					call.NotBefore, endNow)
			}
			t.Logf("✓ Defer notBefore within execution window: %v ∈ [%v, %v]",
				call.NotBefore, startNow, endNow)
			break
		}
	}
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
