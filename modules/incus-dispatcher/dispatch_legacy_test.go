package main

import (
	"testing"
)

// TestDispatch_LegacyProviderModel verifies backward compatibility: dispatching with
// traditional (provider, model) instead of instance names still works.
func TestDispatch_LegacyProviderModel(t *testing.T) {
	dispatcher := NewDispatcher([]Worker{
		{
			WorkerID:         "worker-1",
			WorkerKind:       WorkerKindLocal,
			Capabilities:     []string{"code-review"},
			AllowedPolicies:  []string{"policy@v1"},
		},
	})

	budget := &BudgetSnapshot{
		LimitTokens: 5000,
		SpentTokens: 0,
	}

	// Dispatch with legacy provider/model (not an instance name).
	run, err := dispatcher.Dispatch("code-review", "policy@v1", "anthropic", "claude-3-5-sonnet", budget)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// The run should record the provider and model as-is (legacy path).
	if run.ProviderInstance != "anthropic" {
		t.Errorf("ProviderInstance = %q, want anthropic", run.ProviderInstance)
	}
	if run.ModelID != "claude-3-5-sonnet" {
		t.Errorf("ModelID = %q, want claude-3-5-sonnet", run.ModelID)
	}

	// The run should have captured the budget.
	if run.BudgetSnapshot == nil {
		t.Errorf("BudgetSnapshot is nil")
	} else if run.BudgetSnapshot.LimitTokens != 5000 {
		t.Errorf("BudgetSnapshot.LimitTokens = %d, want 5000", run.BudgetSnapshot.LimitTokens)
	}
}

// TestDispatch_InstanceNameOverridesLegacy verifies that instance names take precedence
// over legacy (provider, model) interpretation.
func TestDispatch_InstanceNameOverridesLegacy(t *testing.T) {
	dispatcher := NewDispatcher([]Worker{
		{
			WorkerID:         "worker-1",
			WorkerKind:       WorkerKindLocal,
			Capabilities:     []string{"code-review"},
			AllowedPolicies:  []string{"policy@v1"},
		},
	})

	budget := &BudgetSnapshot{
		LimitTokens: 5000,
		SpentTokens: 0,
	}

	// Dispatch using an actual instance name.
	run, err := dispatcher.Dispatch("code-review", "policy@v1", "claude-code-main", "", budget)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// The run should use the instance name and resolve its model.
	if run.ProviderInstance != "claude-code-main" {
		t.Errorf("ProviderInstance = %q, want claude-code-main", run.ProviderInstance)
	}

	// The model should be resolved from the instance registry.
	inst := GetProviderInstance("claude-code-main")
	if inst == nil {
		t.Fatalf("instance not found")
	}
	if run.ModelID != inst.Model {
		t.Errorf("ModelID = %q, want %q (from instance)", run.ModelID, inst.Model)
	}
}

// TestDispatch_CostFromResult verifies that cost metrics flow from Result to Run during dispatch.
func TestDispatch_CostFromResult(t *testing.T) {
	// Note: This is more of a unit test for the Run.CostFromResult method wired with dispatch.
	// The dispatcher itself doesn't call CostFromResult; that's done by callers after getting
	// a Result from runner.Run(). This test just verifies the wiring works.

	run := &Run{
		RunID:    "test-run",
		ThreadID: "test-thread",
	}

	result := &Result{
		ExitCode:   0,
		Stdout:     "success",
		TokensIn:   500,
		TokensOut:  200,
		LatencyMs:  1000,
		SpendUSD:   0.05,
	}

	// Wire cost from result.
	run.CostFromResult(result)

	// Verify all fields were copied.
	if run.TokensIn != 500 {
		t.Errorf("TokensIn = %d, want 500", run.TokensIn)
	}
	if run.TokensOut != 200 {
		t.Errorf("TokensOut = %d, want 200", run.TokensOut)
	}
	if run.LatencyMs != 1000 {
		t.Errorf("LatencyMs = %d, want 1000", run.LatencyMs)
	}
	if absFloat(run.SpendUSD-0.05) > 0.001 {
		t.Errorf("SpendUSD = %v, want 0.05", run.SpendUSD)
	}
}
