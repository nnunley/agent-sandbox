package temporal

import (
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"
	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// ReprioritizeCall records a call to Reprioritize with its arguments.
type ReprioritizeCall struct {
	ID         string
	Importance queue.Importance
}

// DeferCall records a call to Defer with its arguments.
type DeferCall struct {
	ID        string
	NotBefore time.Time
}

// FakeReprojector is a test fake that records all calls to Reprioritize and Defer.
// It implements the Reprojector interface and is used to verify that:
// 1. The workflow calls the activity (not directly calling the queue).
// 2. The activity is the sole writer of scheduling fields.
// 3. The recorded arguments match the expected projections (priority level, eligibility time).
type FakeReprojector struct {
	ReprioritizeCalls []ReprioritizeCall
	DeferCalls        []DeferCall
	Err               error // If set, all calls return this error
}

func (f *FakeReprojector) Reprioritize(id string, importance queue.Importance) error {
	if f.Err != nil {
		return f.Err
	}
	f.ReprioritizeCalls = append(f.ReprioritizeCalls, ReprioritizeCall{id, importance})
	return nil
}

func (f *FakeReprojector) Defer(id string, notBefore time.Time) error {
	if f.Err != nil {
		return f.Err
	}
	f.DeferCalls = append(f.DeferCalls, DeferCall{id, notBefore})
	return nil
}

// TestScenario0056_Q2ToQ1Promotion tests that a Q2 item (high importance, deadline 7+ days)
// transitions to Q1 as the deadline nears, via the ReprojectActivity sole-writer seam.
// The testsuite's time-skipping auto-advances the workflow clock and fires timers,
// proving the workflow drives the aging transition autonomously.
//
// SCENARIO-0056: Q2 item promoted to Q1 as deadline nears, with no human intervention.
//
// Setup:
// - Create a directive with importance=high and a deadline 7 days in the future.
//   (7 days out with ImportanceHigh: urgency ~0.40 < 0.5 → starts in Q2).
// - Register the workflow and a fake Reprojector.
// - The testsuite environment's clock starts at env.Now().
//
// Action:
// - Start the workflow with deadline relative to env.Now() (7 days out).
// - The workflow enters its loop: first check at t=0 finds Q2, computes nextCheck = 7*24*60*60 / 4 ≈ 42 hours,
//   calls workflow.Sleep(42h).
// - The testsuite auto-skips: env.Now() advances to t+42h, workflow.Now(ctx) returns advanced time,
//   loop resumes.
// - At t+42h: timeRemaining = 7 - 1.75 = 5.25 days, urgency ~0.50, quadrant still Q2 or crossing.
// - Loop continues, next sleep is 5.25*24*60*60 / 4 ≈ 31.5 hours.
// - At t+74h: timeRemaining = 7 - 3 = 4 days, urgency ~0.55 > 0.5 → Q1.
// - Workflow detects Q2→Q1 change, invokes ReprojectActivity, records Reprioritize + Defer.
// - At Q1, workflow exits.
//
// Expected observables (SCENARIO-0056):
// - Workflow completes without error.
// - At least one Reprioritize call recorded with importance = queue.ImportanceHigh (Q1 level).
// - At least one Defer call recorded with notBefore <= workflow's now at write time.
// - The item genuinely AGED from Q2 to Q1 (not started in Q1), proving deadline-driven transitions.
// - No human intervention required; Temporal's durable timers drove the promotion autonomously.
//
// Automation status: ITER-0007b C2 (CI/testsuite proves workflow-driven Q2→Q1 aging via time-skipping;
// live wall-clock/restart proof deferred to E1).
func TestScenario0056_Q2ToQ1Promotion(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register the workflow and activity
	env.RegisterWorkflow(PriorityWorkflow)

	fakeQueue := &FakeReprojector{}
	activities := &Activities{Queue: fakeQueue}
	env.RegisterActivity(activities.ReprojectActivity)

	// Setup: deadline relative to the testsuite's simulated clock.
	// Use env.Now() to get the environment's current time, then add 7 days.
	startNow := env.Now()
	deadline := startNow.Add(7 * 24 * time.Hour) // 7 days out in simulated time

	// Verify the initial projection (at t=0) is Q2
	initialUrgency := ComputeUrgency(&deadline, startNow)
	initialQuadrant := ComputeQuadrant(ImportanceHigh, initialUrgency)
	if initialQuadrant != QuadrantQ2 {
		t.Fatalf("test setup error: 7-day deadline should start in Q2, got %v (urgency: %.3f)",
			initialQuadrant, initialUrgency)
	}
	t.Logf("Initial state verified: Q2 (urgency: %.3f, env.Now: %v)", initialUrgency, startNow)

	input := PriorityWorkflowInput{
		DirectiveID: "test-directive-q2-q1",
		Importance:  ImportanceHigh,
		Deadline:    &deadline,
	}

	// Execute the workflow. The testsuite's env will auto-skip time when the workflow
	// calls workflow.Sleep(); the workflow's workflow.Now(ctx) will march forward.
	env.ExecuteWorkflow(PriorityWorkflow, input)

	// Capture the end time to bracket the execution window
	endNow := env.Now()

	// Verify no panic or error in the workflow itself
	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow failed: %v", err)
	}

	// SCENARIO-0056 Observable 1: The workflow detected the Q2→Q1 aging transition
	// and invoked the ReprojectActivity (sole-writer seam).
	if len(fakeQueue.ReprioritizeCalls) == 0 {
		t.Fatalf("Reprioritize was never called; expected ≥1 call for Q2→Q1 aging transition")
	}
	if len(fakeQueue.DeferCalls) == 0 {
		t.Fatalf("Defer was never called; expected ≥1 call for making item eligible")
	}
	t.Logf("✓ Activity invoked: Reprioritize called %d time(s), Defer called %d time(s)",
		len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))

	// SCENARIO-0056 Observable 2: The Reprioritize call used a Q1-level importance (HIGH).
	// This proves the promotion actually RAISED priority (the core aging behavior).
	// Expected: tierToQueueImportance(ImportanceHigh) == queue.ImportanceHigh
	foundHighImportance := false
	for _, call := range fakeQueue.ReprioritizeCalls {
		if call.ID == input.DirectiveID && call.Importance == queue.ImportanceHigh {
			foundHighImportance = true
			t.Logf("✓ Priority promotion: Reprioritize called with importance=%v (Q1 tier)", call.Importance)
			break
		}
	}
	if !foundHighImportance {
		actualImportance := queue.Importance("")
		for _, call := range fakeQueue.ReprioritizeCalls {
			if call.ID == input.DirectiveID {
				actualImportance = call.Importance
				break
			}
		}
		t.Fatalf("no Q2→Q1 promotion found: Reprioritize called with importance=%v; expected %v",
			actualImportance, queue.ImportanceHigh)
	}

	// SCENARIO-0056 Observable 3: The Defer call set notBefore within the simulated execution window.
	// This proves notBefore was set to the workflow's advanced "now" (item is eligible),
	// not a zero/garbage value, and that real time-skipping occurred.
	foundDeferCall := false
	for _, call := range fakeQueue.DeferCalls {
		if call.ID == input.DirectiveID {
			foundDeferCall = true
			// Assert notBefore is within the simulated execution window [startNow, endNow].
			// The workflow sets notBefore = workflow.Now(ctx) at write time, which advances
			// as the testsuite skips through sleep durations.
			if call.NotBefore.Before(startNow) || call.NotBefore.After(endNow) {
				t.Fatalf("Defer notBefore %v not within simulated execution window [%v, %v]; "+
					"eligibility not set to advanced workflow-now", call.NotBefore, startNow, endNow)
			}
			t.Logf("✓ Eligibility set: Defer called with notBefore=%v (within window [%v, %v])",
				call.NotBefore, startNow, endNow)
			break
		}
	}
	if !foundDeferCall {
		t.Fatalf("no Defer call found for directive ID %s", input.DirectiveID)
	}

	// SCENARIO-0056 Observable 4: Autonomous aging (no human intervention).
	// Verified by structure: the workflow ran without external signals, used only its own timers,
	// and invoked the activity autonomously to write the promotion.
	t.Logf("✓ Autonomous Q2→Q1 aging: Temporal workflow detected deadline approach, "+
		"aged item from Q2→Q1, invoked activity (Reprioritize: %d, Defer: %d) — no human action needed",
		len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))
}

// TestScenario0093_SoleWriterSeam_NoDeadline tests that the sole-writer invariant holds:
// ONLY the activity writes to laneq scheduling RPCs.
//
// For a Q4 item (no deadline), the workflow should make NO calls to the activity.
// This verifies the process-level sole-writer discipline: if no quadrant change occurs,
// no writes happen.
//
// SCENARIO-0093: Only Temporal activity calls laneq Defer/Reprioritize.
//
// Setup:
// - Create a directive with low importance and NO deadline (Q4 tier, idle-only).
// - Register the workflow and a fake Reprojector.
//
// Action:
// - Start the workflow with the directive's parameters.
// - Let the workflow run to completion (Q4 items with no deadline exit immediately).
//
// Expected observables:
// - The fake Reprojector records ZERO calls to Reprioritize and Defer.
// - The workflow completes without error.
// - This verifies the sole-writer seam: the ONLY way writes happen is via the activity;
//   when no activity is invoked (because no quadrant change), no writes occur.
//
// Automation status: ITER-0007b C2 (CI/testsuite logic basis).
func TestScenario0093_SoleWriterSeam_NoDeadline(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register the workflow and activity
	env.RegisterWorkflow(PriorityWorkflow)

	fakeQueue := &FakeReprojector{}
	activities := &Activities{Queue: fakeQueue}
	env.RegisterActivity(activities.ReprojectActivity)

	// Setup: directive with low importance and NO deadline (Q4)
	input := PriorityWorkflowInput{
		DirectiveID: "test-directive-q4-no-deadline",
		Importance:  ImportanceLow,
		Deadline:    nil, // No deadline = Q4 (idle-only)
	}

	// Execute the workflow
	env.ExecuteWorkflow(PriorityWorkflow, input)

	// Verify no error
	if !env.IsWorkflowCompleted() {
		t.Errorf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Errorf("workflow failed: %v", err)
	}

	// SCENARIO-0093: Sole-writer invariant for Q4 no-deadline items
	// No quadrant change possible → no activity invoked → no writes.
	if len(fakeQueue.ReprioritizeCalls) != 0 {
		t.Errorf("Reprioritize was called %d times; expected 0 (Q4 no-deadline makes no writes)",
			len(fakeQueue.ReprioritizeCalls))
	}
	if len(fakeQueue.DeferCalls) != 0 {
		t.Errorf("Defer was called %d times; expected 0 (Q4 no-deadline makes no writes)",
			len(fakeQueue.DeferCalls))
	}

	t.Logf("✓ Sole-writer invariant verified: Q4 no-deadline item made zero writes "+
		"(Reprioritize: %d, Defer: %d)", len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))
}

// TestScenario0093_SoleWriterSeam_NoWriteWhenNoChange tests the secondary invariant of SCENARIO-0093:
// even for items with deadlines, if no quadrant change occurs, no writes should happen.
//
// This proves the process-level sole-writer discipline: the workflow computes projections,
// and ONLY invokes the activity (and thus only writes to laneq) when a real change is detected.
//
// Setup:
// - Create a directive with critical importance and a 1-day deadline (deadline soon).
//   At 1 day out, urgency ~0.69 >= 0.5, so quadrant is Q1 (important + urgent).
//   The workflow will detect Q1 on first check and exit (Q1 items are ready to run).
//
// Action:
// - Start the workflow.
// - The workflow computes Q1 on the first iteration and immediately exits (Q1 exit condition).
//
// Expected observables:
// - The fake Reprojector records ZERO calls to Reprioritize and Defer (Q1 exit prevents activity).
// - The workflow completes without error.
// - This verifies: items already in their final quadrant (Q1) don't trigger activity writes
//   because the loop exits before any change is detected.
//
// Automation status: ITER-0007b C2 (CI/testsuite logic basis).
func TestScenario0093_SoleWriterSeam_NoWriteWhenNoChange(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Register the workflow and activity
	env.RegisterWorkflow(PriorityWorkflow)

	fakeQueue := &FakeReprojector{}
	activities := &Activities{Queue: fakeQueue}
	env.RegisterActivity(activities.ReprojectActivity)

	// Setup: directive with critical importance and a 1-day deadline (already Q1, will exit immediately).
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	deadline := now.Add(1 * 24 * time.Hour) // 1 day out → Q1 (urgency ~0.69 >= 0.5)

	// Verify the projection is Q1 and stays Q1 (never changes)
	initialUrgency := ComputeUrgency(&deadline, now)
	initialQuadrant := ComputeQuadrant(ImportanceCritical, initialUrgency)
	if initialQuadrant != QuadrantQ1 {
		t.Fatalf("test setup error: 1-day deadline should be Q1, got %v (urgency: %.3f)",
			initialQuadrant, initialUrgency)
	}
	t.Logf("Setup verified: Q1 (urgent + critical) — workflow will exit on first check (no activity invoked)")

	input := PriorityWorkflowInput{
		DirectiveID: "test-directive-stable-q1",
		Importance:  ImportanceCritical,
		Deadline:    &deadline,
	}

	// Execute the workflow
	env.ExecuteWorkflow(PriorityWorkflow, input)

	// Verify no error
	if !env.IsWorkflowCompleted() {
		t.Errorf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Errorf("workflow failed: %v", err)
	}

	// SCENARIO-0093: Sole-writer invariant for Q1 items (no change = no activity).
	// Items in Q1 are ready to run, so the workflow exits immediately (see loop condition).
	// Since lastQuadrant is initialized to the computed initial Q1 quadrant, there's no
	// change on first iteration, so no activity is invoked.
	// This is a STRICT assertion (must fail if ANY write occurred).
	if len(fakeQueue.ReprioritizeCalls) != 0 {
		t.Fatalf("Reprioritize was called %d times for Q1 item; expected 0 "+
			"(Q1 causes immediate exit, no change = no writes). Calls: %v",
			len(fakeQueue.ReprioritizeCalls), fakeQueue.ReprioritizeCalls)
	}
	if len(fakeQueue.DeferCalls) != 0 {
		t.Fatalf("Defer was called %d times for Q1 item; expected 0 "+
			"(Q1 causes immediate exit, no change = no writes). Calls: %v",
			len(fakeQueue.DeferCalls), fakeQueue.DeferCalls)
	}

	t.Logf("✓ Sole-writer invariant verified: Q1 item made zero writes "+
		"(Reprioritize: %d, Defer: %d) — no change → no activity invocation",
		len(fakeQueue.ReprioritizeCalls), len(fakeQueue.DeferCalls))
}
