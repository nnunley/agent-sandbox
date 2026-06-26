package main

import (
	"testing"
)

// TestBudgetPolicy_Levels proves AC-1: Budget policy object supports all 6 levels.
// Each level is settable, readable, and distinguishable.
func TestBudgetPolicy_Levels(t *testing.T) {
	bp := NewBudgetPolicy("policy-budget-1")

	// Set all 6 levels with distinct values.
	bp.PerMessage = &BudgetLimit{
		Level:       BudgetLevelPerMessage,
		HardCeiling: 1.0,
	}
	bp.PerRun = &BudgetLimit{
		Level:       BudgetLevelPerRun,
		HardCeiling: 2.0,
	}
	bp.PerThread = &BudgetLimit{
		Level:       BudgetLevelPerThread,
		HardCeiling: 10.0,
	}
	bp.PerWorkerClass = &BudgetLimit{
		Level:       BudgetLevelPerWorkerClass,
		HardCeiling: 50.0,
	}
	bp.PerProvider = &BudgetLimit{
		Level:       BudgetLevelPerProvider,
		HardCeiling: 100.0,
	}
	bp.PerTimeWindow = &BudgetLimit{
		Level:           BudgetLevelPerTimeWindow,
		HardCeiling:     500.0,
		TimeWindowSecs:  3600, // 1 hour
	}

	// Verify all levels are set and readable.
	tests := []struct {
		name        string
		got         *BudgetLimit
		wantLevel   BudgetLevel
		wantCeiling float64
	}{
		{"PerMessage", bp.PerMessage, BudgetLevelPerMessage, 1.0},
		{"PerRun", bp.PerRun, BudgetLevelPerRun, 2.0},
		{"PerThread", bp.PerThread, BudgetLevelPerThread, 10.0},
		{"PerWorkerClass", bp.PerWorkerClass, BudgetLevelPerWorkerClass, 50.0},
		{"PerProvider", bp.PerProvider, BudgetLevelPerProvider, 100.0},
		{"PerTimeWindow", bp.PerTimeWindow, BudgetLevelPerTimeWindow, 500.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got == nil {
				t.Fatalf("level %s is nil", tc.name)
			}
			if tc.got.Level != tc.wantLevel {
				t.Errorf("Level = %q, want %q", tc.got.Level, tc.wantLevel)
			}
			if tc.got.HardCeiling != tc.wantCeiling {
				t.Errorf("HardCeiling = %v, want %v", tc.got.HardCeiling, tc.wantCeiling)
			}
		})
	}

	// Verify policy ID is set.
	if bp.PolicyID != "policy-budget-1" {
		t.Errorf("PolicyID = %q, want policy-budget-1", bp.PolicyID)
	}
}

// TestBudgetPolicy_ProtectedFromAutoMutation proves AC-2: hard ceilings are protected.
// Only explicit operator-approved paths can change hard ceilings; auto-mutation is rejected.
func TestBudgetPolicy_ProtectedFromAutoMutation(t *testing.T) {
	bp := NewBudgetPolicy("policy-protected")
	bp.PerThread = &BudgetLimit{
		Level:       BudgetLevelPerThread,
		HardCeiling: 10.0,
	}

	// Attempt 1: Auto-mutation of hard ceiling FAILS.
	err := bp.ApplyAutoMutation("per_thread_hard_ceiling", 20.0)
	if err == nil {
		t.Fatalf("ApplyAutoMutation should reject protected field, got nil error")
	}
	if bp.PerThread.HardCeiling != 10.0 {
		t.Errorf("hard ceiling was mutated despite protection: got %v, want 10.0", bp.PerThread.HardCeiling)
	}

	// Attempt 2: Operator-approved mutation of hard ceiling SUCCEEDS.
	oldValue, err := bp.ApplyOperatorMutation("per_thread_hard_ceiling", 20.0, "operator-alice")
	if err != nil {
		t.Fatalf("ApplyOperatorMutation failed: %v", err)
	}
	if oldValue != 10.0 {
		t.Errorf("oldValue = %v, want 10.0", oldValue)
	}
	if bp.PerThread.HardCeiling != 20.0 {
		t.Errorf("hard ceiling not updated: got %v, want 20.0", bp.PerThread.HardCeiling)
	}

	// Verify the operator is recorded in the audit trail.
	if bp.LastModifiedBy != "operator-alice" {
		t.Errorf("LastModifiedBy = %q, want operator-alice", bp.LastModifiedBy)
	}

	// Attempt 3: Auto-mutation of escalation heuristic SUCCEEDS (allowed tuning).
	err = bp.ApplyAutoMutation("per_thread_escalation_threshold", 8.0)
	if err != nil {
		t.Fatalf("ApplyAutoMutation should allow escalation heuristic, got error: %v", err)
	}

	// Attempt 4: AllowAutoMutation reports the correct protection status for hard ceilings.
	hardCeilingAllowed := bp.AllowAutoMutation("per_thread_hard_ceiling")
	escalationAllowed := bp.AllowAutoMutation("per_thread_escalation_threshold")
	if hardCeilingAllowed {
		t.Errorf("AllowAutoMutation(per_thread_hard_ceiling) = true, want false (protected)")
	}
	if !escalationAllowed {
		t.Errorf("AllowAutoMutation(per_thread_escalation_threshold) = false, want true (tunable)")
	}
}

// TestBudgetEnforce_ExceedRejectsOrEscalates proves AC-3: enforcement decides to reject or escalate.
// This is a unit test of the enforcement decision logic (not a full integration test).
// The integration test (TestScenario0022) drives the decision through the daemon.
func TestBudgetEnforce_ExceedRejectsOrEscalates(t *testing.T) {
	bp := NewBudgetPolicy("policy-enforce")
	bp.PerThread = &BudgetLimit{
		Level:       BudgetLevelPerThread,
		HardCeiling: 10.0,
	}

	// Create a run that would exceed the per-thread limit.
	run := &Run{
		RunID:    "run-expensive",
		ThreadID: "thread-1",
		SpendUSD: 5.0,
	}

	// Current thread spend is $8.
	currentThreadSpend := 8.0

	// Enforcement decision: run would cause $8 + $5 = $13, exceeding $10 limit.
	decision := bp.EnforceRunBudget(run, currentThreadSpend)

	if decision == nil {
		t.Fatalf("EnforceRunBudget returned nil decision")
	}

	if decision.Allowed {
		t.Errorf("Allowed = true, want false (run exceeds limit)")
	}

	if decision.LimitLevel != BudgetLevelPerThread {
		t.Errorf("LimitLevel = %q, want per_thread", decision.LimitLevel)
	}

	if decision.CurrentSpend != 8.0 {
		t.Errorf("CurrentSpend = %v, want 8.0", decision.CurrentSpend)
	}

	if decision.HardCeiling != 10.0 {
		t.Errorf("HardCeiling = %v, want 10.0", decision.HardCeiling)
	}

	// Run within budget should be allowed.
	withinBudget := &Run{
		RunID:    "run-cheap",
		ThreadID: "thread-1",
		SpendUSD: 1.5,
	}

	decision2 := bp.EnforceRunBudget(withinBudget, currentThreadSpend)
	if !decision2.Allowed {
		t.Errorf("Allowed = false, want true (run within limit)")
	}
}

// TestBudgetPolicy_ComputeThreadSpend verifies the helper that sums per-thread spending.
func TestBudgetPolicy_ComputeThreadSpend(t *testing.T) {
	runs := []*Run{
		{RunID: "run-1", SpendUSD: 2.5},
		{RunID: "run-2", SpendUSD: 3.0},
		{RunID: "run-3", SpendUSD: 1.5},
	}

	total := ComputeThreadSpend(runs)
	const epsilon = 0.0001
	if absDiff(total, 7.0) > epsilon {
		t.Errorf("ComputeThreadSpend = %v, want 7.0", total)
	}
}

// absDiff returns the absolute difference between x and y.
func absDiff(x, y float64) float64 {
	if x < y {
		return y - x
	}
	return x - y
}
