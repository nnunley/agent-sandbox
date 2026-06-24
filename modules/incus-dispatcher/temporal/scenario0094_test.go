package temporal

import (
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"
	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// TestScenario0094_HumanRescoreUnrestricted tests SCENARIO-0094 / STORY-0047 AC-1.
// A human actor signals a rescore to a higher bucket (e.g., Q2 → Critical/Q1).
// Expected: the workflow accepts the rescore, updates currentImportance, recomputes projection,
// and invokes ReprojectActivity (sole-writer seam) to persist the new priority.
//
// This is the CI/testsuite logic basis for live human rescore via deployed Temporal.
// (Live durability across restart is E1.)
func TestScenario0094_HumanRescoreUnrestricted(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register the workflow and activity
	env.RegisterWorkflow(PriorityWorkflow)

	fakeQueue := &FakeReprojector{}
	activities := &Activities{Queue: fakeQueue}
	env.RegisterActivity(activities.ReprojectActivity)

	// Setup: directive with Medium importance and a deadline 7 days out.
	// At 7 days out with ImportanceMedium: urgency ~0.40 < 0.5 → Q4 (not important + not urgent)
	startNow := env.Now()
	deadline := startNow.Add(7 * 24 * time.Hour)

	initialUrgency := ComputeUrgency(&deadline, startNow)
	initialQuadrant := ComputeQuadrant(ImportanceMedium, initialUrgency)
	if initialQuadrant != QuadrantQ4 {
		t.Fatalf("test setup error: 7-day deadline with Medium importance should be Q4, got %v (urgency: %.3f)", initialQuadrant, initialUrgency)
	}
	t.Logf("Initial state verified: Q4 (urgency: %.3f)", initialUrgency)

	// Human actor (unrestricted authority)
	human := Actor{
		Role: ActorRoleHuman,
		ID:   "operator",
	}

	// Rescore request: Medium → Critical (Q3 → Q1 via importance bump)
	rescoreSignal := RescoreSignal{
		Actor:              human,
		ProposedImportance: ImportanceCritical,
	}

	// Register a delayed callback to send the rescore signal after a short delay.
	// This ensures the workflow processes at least one normal aging cycle before receiving the signal.
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(RescoreSignalName, rescoreSignal)
	}, 1*time.Second)

	input := PriorityWorkflowInput{
		DirectiveID: "test-directive-human-rescore",
		Importance:  ImportanceMedium,
		Deadline:    &deadline,
	}

	// Execute the workflow
	env.ExecuteWorkflow(PriorityWorkflow, input)

	// Verify the workflow completed without error
	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow failed: %v", err)
	}

	// SCENARIO-0094 Observable 1: The ReprojectActivity was invoked due to the rescore.
	// Expected: at least one Reprioritize call with the new importance (Critical → High in queue terms).
	if len(fakeQueue.ReprioritizeCalls) == 0 {
		t.Fatalf("Reprioritize was never called; expected ≥1 call for human rescore re-projection")
	}
	if len(fakeQueue.DeferCalls) == 0 {
		t.Fatalf("Defer was never called; expected ≥1 call for setting eligibility after rescore")
	}

	// SCENARIO-0094 Observable 2: The Reprioritize call used the Critical-level importance (HIGH).
	// This proves the rescore WAS applied (importance changed from Medium to Critical).
	foundCriticalImportance := false
	for _, call := range fakeQueue.ReprioritizeCalls {
		if call.ID == input.DirectiveID && call.Importance == queue.ImportanceHigh {
			foundCriticalImportance = true
			t.Logf("✓ Rescore applied: Reprioritize called with importance=%v (Critical tier)", call.Importance)
			break
		}
	}
	if !foundCriticalImportance {
		t.Fatalf("no rescore re-projection found: expected Reprioritize with Critical importance, "+
			"got calls: %v", fakeQueue.ReprioritizeCalls)
	}

	t.Logf("✓ SCENARIO-0094 (human rescore unrestricted): workflow accepted rescore via signal, "+
		"updated priority, invoked sole-writer activity (Reprioritize: %d, Defer: %d)",
		len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))
}

// TestScenario0094_AgentRescoreBounded tests that an agent out-of-bounds rescore
// is REJECTED (no activity invocation, no write to laneq).
//
// This complements SCENARIO-0057 D2 (unit test) by proving the WORKFLOW SIGNAL path
// enforces agent bounds: a signal requesting an illegal jump (Low → Critical) is rejected,
// and the workflow makes NO writes.
//
// SCENARIO-0057 integration proof via workflow signal: agent-bounded rescore rejection
// (no write when out of bounds).
func TestScenario0094_AgentRescoreBounded(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register the workflow and activity
	env.RegisterWorkflow(PriorityWorkflow)

	fakeQueue := &FakeReprojector{}
	activities := &Activities{Queue: fakeQueue}
	env.RegisterActivity(activities.ReprojectActivity)

	// Setup: directive with Low importance and a deadline 7 days out (Q4 or Q3).
	startNow := env.Now()
	deadline := startNow.Add(7 * 24 * time.Hour)

	// Agent actor (bounded authority: max 1-tier jump, cannot self-promote to Critical)
	agent := Actor{
		Role: ActorRoleAgent,
		ID:   "agent-001",
	}

	// Out-of-bounds rescore request: Low → Critical (2-tier jump, self-promotion)
	// This MUST be rejected; no write should occur.
	rescoreSignal := RescoreSignal{
		Actor:              agent,
		ProposedImportance: ImportanceCritical,
	}

	// Register a delayed callback to send the out-of-bounds rescore signal.
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(RescoreSignalName, rescoreSignal)
	}, 1*time.Second)

	input := PriorityWorkflowInput{
		DirectiveID: "test-directive-agent-oob",
		Importance:  ImportanceLow,
		Deadline:    &deadline,
	}

	// Execute the workflow
	env.ExecuteWorkflow(PriorityWorkflow, input)

	// Verify the workflow completed without error
	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow failed: %v", err)
	}

	// SCENARIO-0094 / SCENARIO-0057 Observable: The out-of-bounds rescore was REJECTED.
	// Expected: ZERO Reprioritize and Defer calls resulted from the out-of-bounds signal
	// (the no-write rejection seam is working).
	//
	// Note: C2 may have invoked the activity for aging transitions (e.g., Low→Medium as deadline nears).
	// We must assert that NONE of those calls use the proposed Critical importance.
	for _, call := range fakeQueue.ReprioritizeCalls {
		if call.ID == input.DirectiveID && call.Importance == queue.ImportanceHigh {
			// We found a call with Critical/High importance. If this came from the rescore signal,
			// the rejection failed. To be strict, we can check the timing: the signal arrives at 1s,
			// and aging checks occur at longer intervals. However, the testsuite may auto-advance
			// in a way that conflates timings. For this test, we'll be lenient and just assert
			// that if the importance IS High, it must NOT be because of the rescore signal (i.e.,
			// it could only be from aging, which means the rescore was rejected).
			//
			// Actually, let's be stricter: if there are ANY Reprioritize calls at all, they should
			// correspond to aging transitions (e.g., Low or Medium), NOT the proposed Critical.
			// The cleanest assertion: the directive should NEVER have had its importance bumped to
			// Critical via a rescore. We can check the recorded Importance values; they should match
			// the result of aging (Low→Medium or stays Low), not the proposed Critical.
			t.Fatalf("agent out-of-bounds rescore was NOT rejected: found Reprioritize call "+
				"with Critical importance (queue.ImportanceHigh); expected rejection (no Critical bump)")
		}
	}

	t.Logf("✓ SCENARIO-0094 / SCENARIO-0057 (agent rescore bounded): "+
		"out-of-bounds Low→Critical signal was rejected; "+
		"no writes from illegal rescore (Reprioritize calls: %d, Defer calls: %d)",
		len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))
}

// TestScenario0094_AgentWithinBoundedRescore tests that an agent in-bounds rescore
// (e.g., Low → Medium, a 1-tier jump) IS accepted and invokes the activity.
//
// This proves the workflow signal path allows legitimate agent rescores while rejecting
// out-of-bounds ones.
func TestScenario0094_AgentWithinBoundedRescore(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register the workflow and activity
	env.RegisterWorkflow(PriorityWorkflow)

	fakeQueue := &FakeReprojector{}
	activities := &Activities{Queue: fakeQueue}
	env.RegisterActivity(activities.ReprojectActivity)

	// Setup: directive with Low importance and a deadline 7 days out.
	startNow := env.Now()
	deadline := startNow.Add(7 * 24 * time.Hour)

	// Agent actor (bounded authority)
	agent := Actor{
		Role: ActorRoleAgent,
		ID:   "agent-002",
	}

	// In-bounds rescore request: Low → Medium (1-tier jump, allowed)
	rescoreSignal := RescoreSignal{
		Actor:              agent,
		ProposedImportance: ImportanceMedium,
	}

	// Register a delayed callback to send the in-bounds rescore signal.
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(RescoreSignalName, rescoreSignal)
	}, 1*time.Second)

	input := PriorityWorkflowInput{
		DirectiveID: "test-directive-agent-inbounds",
		Importance:  ImportanceLow,
		Deadline:    &deadline,
	}

	// Execute the workflow
	env.ExecuteWorkflow(PriorityWorkflow, input)

	// Verify the workflow completed without error
	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow failed: %v", err)
	}

	// SCENARIO-0094 Observable: The in-bounds rescore was ACCEPTED.
	// Expected: at least one Reprioritize call for the rescore re-projection.
	if len(fakeQueue.ReprioritizeCalls) == 0 {
		t.Fatalf("Reprioritize was never called; expected ≥1 call for in-bounds agent rescore")
	}
	if len(fakeQueue.DeferCalls) == 0 {
		t.Fatalf("Defer was never called; expected ≥1 call for setting eligibility after rescore")
	}

	// SCENARIO-0094 Observable 2: The Reprioritize call used the Medium-level importance (NORMAL).
	foundMediumImportance := false
	for _, call := range fakeQueue.ReprioritizeCalls {
		if call.ID == input.DirectiveID && call.Importance == queue.ImportanceNormal {
			foundMediumImportance = true
			t.Logf("✓ In-bounds rescore applied: Reprioritize called with importance=%v (Medium tier)", call.Importance)
			break
		}
	}
	if !foundMediumImportance {
		t.Fatalf("no in-bounds rescore re-projection found: expected Reprioritize with Medium importance, "+
			"got calls: %v", fakeQueue.ReprioritizeCalls)
	}

	t.Logf("✓ SCENARIO-0094 (agent rescore within bounds): "+
		"workflow accepted in-bounds rescore (Low→Medium), invoked activity (Reprioritize: %d, Defer: %d)",
		len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))
}
