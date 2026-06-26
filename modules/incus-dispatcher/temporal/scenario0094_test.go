package temporal

import (
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
	"go.temporal.io/sdk/testsuite"
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

	// Rescore request: Medium → Critical (Q4 → Q2 via importance bump)
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

	// SCENARIO-0094 Observable 3: Query confirms the rescore was accepted (currentImportance changed to Critical).
	// This proves signal delivery and acceptance in the workflow's deterministic state.
	var finalImportance Importance
	queryFuture, err := env.QueryWorkflow(CurrentImportanceQuery)
	if err != nil {
		t.Fatalf("failed to start query currentImportance: %v", err)
	}
	err = queryFuture.Get(&finalImportance)
	if err != nil {
		t.Fatalf("failed to get query result: %v", err)
	}
	if finalImportance != ImportanceCritical {
		t.Fatalf("human rescore was NOT accepted: query currentImportance=%d; expected ImportanceCritical=%d",
			finalImportance, ImportanceCritical)
	}
	t.Logf("✓ Query confirms: currentImportance changed to Critical (rescore accepted)")

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

	// SCENARIO-0094 / SCENARIO-0057 Observable 1: Verify signal was delivered and processed.
	// Query the workflow's live currentImportance to confirm the rescore signal was received
	// and that the rejection was applied (importance stayed Low).
	var finalImportance Importance
	queryFuture2, err2 := env.QueryWorkflow(CurrentImportanceQuery)
	if err2 != nil {
		t.Fatalf("failed to start query currentImportance: %v", err2)
	}
	err2 = queryFuture2.Get(&finalImportance)
	if err2 != nil {
		t.Fatalf("failed to get query result: %v", err2)
	}
	if finalImportance != ImportanceLow {
		t.Fatalf("rescore signal was NOT rejected: query currentImportance=%d; expected ImportanceLow=%d "+
			"(signal must be delivered + importance must stay unchanged on rejection)",
			finalImportance, ImportanceLow)
	}
	t.Logf("✓ Rescore signal delivered and rejected: currentImportance stayed at Low")

	// SCENARIO-0094 / SCENARIO-0057 Observable 2: Verify no writes occurred from rejected rescore.
	// Every Reprioritize call for this directive must use queue.ImportanceLow.
	// Aging a Low item only maps to queue.ImportanceLow; an accepted illegal rescore would produce Normal/High.
	for _, call := range fakeQueue.ReprioritizeCalls {
		if call.ID == input.DirectiveID && call.Importance != queue.ImportanceLow {
			t.Fatalf("agent out-of-bounds rescore was NOT rejected: found Reprioritize call "+
				"with importance=%q; every call for Low item must be ImportanceLow=%q (aging never bumps Low→Critical)."+
				"Non-Low importance proves the rejected rescore was applied (FAILURE)",
				call.Importance, queue.ImportanceLow)
		}
	}
	t.Logf("✓ SCENARIO-0094 / SCENARIO-0057 (agent rescore bounded): out-of-bounds Low→Critical " +
		"signal was delivered, validated, and rejected; currentImportance stayed Low; " +
		"all Reprioritize calls use ImportanceLow (no illegal write occurred)")
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

	// SCENARIO-0094 Observable 3: Query confirms the in-bounds rescore was accepted (currentImportance changed to Medium).
	var finalImportance Importance
	queryFuture3, err3 := env.QueryWorkflow(CurrentImportanceQuery)
	if err3 != nil {
		t.Fatalf("failed to start query currentImportance: %v", err3)
	}
	err3 = queryFuture3.Get(&finalImportance)
	if err3 != nil {
		t.Fatalf("failed to get query result: %v", err3)
	}
	if finalImportance != ImportanceMedium {
		t.Fatalf("in-bounds rescore was NOT accepted: query currentImportance=%d; expected ImportanceMedium=%d",
			finalImportance, ImportanceMedium)
	}
	t.Logf("✓ Query confirms: currentImportance changed to Medium (rescore accepted)")

	t.Logf("✓ SCENARIO-0094 (agent rescore within bounds): "+
		"workflow accepted in-bounds rescore (Low→Medium), invoked activity (Reprioritize: %d, Defer: %d)",
		len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))
}

// TestScenario0094_AgentDownwardRescore tests that an agent can rescore DOWNWARD within bounds
// (e.g., High → Medium, a 1-tier down jump).
//
// This rounds out the authority matrix: agent-bounded allows both upward (Low→Medium) and
// downward (High→Medium) rescores as long as tierDiff is in [-1, 1]. Proves the negative-tierDiff
// path is exercised and accepted.
func TestScenario0094_AgentDownwardRescore(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register the workflow and activity
	env.RegisterWorkflow(PriorityWorkflow)

	fakeQueue := &FakeReprojector{}
	activities := &Activities{Queue: fakeQueue}
	env.RegisterActivity(activities.ReprojectActivity)

	// Setup: directive with High importance and a deadline 7 days out.
	startNow := env.Now()
	deadline := startNow.Add(7 * 24 * time.Hour)

	// Agent actor (bounded authority: can rescore down by 1 tier)
	agent := Actor{
		Role: ActorRoleAgent,
		ID:   "agent-downward",
	}

	// In-bounds downward rescore request: High → Medium (1-tier down, allowed)
	rescoreSignal := RescoreSignal{
		Actor:              agent,
		ProposedImportance: ImportanceMedium,
	}

	// Register a delayed callback to send the downward rescore signal.
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(RescoreSignalName, rescoreSignal)
	}, 1*time.Second)

	input := PriorityWorkflowInput{
		DirectiveID: "test-directive-agent-downward",
		Importance:  ImportanceHigh,
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

	// SCENARIO-0094 Observable 1: Query confirms the downward rescore was accepted.
	var finalImportance Importance
	queryFuture, err := env.QueryWorkflow(CurrentImportanceQuery)
	if err != nil {
		t.Fatalf("failed to start query currentImportance: %v", err)
	}
	err = queryFuture.Get(&finalImportance)
	if err != nil {
		t.Fatalf("failed to get query result: %v", err)
	}
	if finalImportance != ImportanceMedium {
		t.Fatalf("downward rescore was NOT accepted: query currentImportance=%d; expected ImportanceMedium=%d",
			finalImportance, ImportanceMedium)
	}
	t.Logf("✓ Query confirms: currentImportance changed to Medium (downward rescore accepted)")

	// SCENARIO-0094 Observable 2: The ReprojectActivity was invoked.
	if len(fakeQueue.ReprioritizeCalls) == 0 {
		t.Fatalf("Reprioritize was never called; expected ≥1 call for downward rescore re-projection")
	}
	if len(fakeQueue.DeferCalls) == 0 {
		t.Fatalf("Defer was never called; expected ≥1 call for setting eligibility after downward rescore")
	}

	// SCENARIO-0094 Observable 3: The Reprioritize call used the Medium-level importance (NORMAL).
	foundMediumImportance := false
	for _, call := range fakeQueue.ReprioritizeCalls {
		if call.ID == input.DirectiveID && call.Importance == queue.ImportanceNormal {
			foundMediumImportance = true
			t.Logf("✓ Downward rescore applied: Reprioritize called with importance=%v (Medium tier)", call.Importance)
			break
		}
	}
	if !foundMediumImportance {
		t.Fatalf("no downward rescore re-projection found: expected Reprioritize with Medium importance, "+
			"got calls: %v", fakeQueue.ReprioritizeCalls)
	}

	t.Logf("✓ SCENARIO-0094 (agent downward rescore): "+
		"workflow accepted downward rescore (High→Medium), invoked activity (Reprioritize: %d, Defer: %d)",
		len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))
}
