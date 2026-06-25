package main

import (
	"testing"
)

// TestRunCostCapture verifies that Run.CostFromResult copies usage metrics from Result (STORY-0035 AC-4).
func TestRunCostCapture(t *testing.T) {
	// Create a Result with usage metrics.
	res := &Result{
		ExitCode:   0,
		Stdout:     "success",
		TokensIn:   1000,
		TokensOut:  500,
		LatencyMs:  250,
		SpendUSD:   0.05,
	}

	// Create a Run and copy cost from Result.
	run := &Run{
		RunID:    "run-1",
		ThreadID: "thread-1",
	}

	run.CostFromResult(res)

	// Verify all cost fields were copied.
	if run.TokensIn != 1000 {
		t.Errorf("TokensIn = %d, want 1000", run.TokensIn)
	}
	if run.TokensOut != 500 {
		t.Errorf("TokensOut = %d, want 500", run.TokensOut)
	}
	if run.LatencyMs != 250 {
		t.Errorf("LatencyMs = %d, want 250", run.LatencyMs)
	}
	if run.SpendUSD != 0.05 {
		t.Errorf("SpendUSD = %v, want 0.05", run.SpendUSD)
	}
}

// TestRunCostCapture_ZeroValues verifies that zero/empty Result values are not copied.
func TestRunCostCapture_ZeroValues(t *testing.T) {
	// Create a Result with zero/empty metrics (should not be copied).
	res := &Result{
		ExitCode:   1, // Non-zero, but no cost metrics.
		Stdout:     "failed",
		TokensIn:   0,
		TokensOut:  0,
		LatencyMs:  0,
		SpendUSD:   0.0,
	}

	run := &Run{
		RunID:    "run-2",
		ThreadID: "thread-2",
	}

	run.CostFromResult(res)

	// Verify no cost fields were copied (all remain zero).
	if run.TokensIn != 0 {
		t.Errorf("TokensIn = %d, want 0 (not copied from Result with zero)", run.TokensIn)
	}
	if run.TokensOut != 0 {
		t.Errorf("TokensOut = %d, want 0", run.TokensOut)
	}
	if run.LatencyMs != 0 {
		t.Errorf("LatencyMs = %d, want 0", run.LatencyMs)
	}
	if run.SpendUSD != 0.0 {
		t.Errorf("SpendUSD = %v, want 0.0", run.SpendUSD)
	}
}

// TestRunCostCapture_NilResult verifies that CostFromResult handles nil Result gracefully.
func TestRunCostCapture_NilResult(t *testing.T) {
	run := &Run{
		RunID:    "run-3",
		ThreadID: "thread-3",
	}

	// Call with nil Result should not panic.
	run.CostFromResult(nil)

	// Run should remain unchanged.
	if run.TokensIn != 0 {
		t.Errorf("TokensIn = %d after nil Result, want 0", run.TokensIn)
	}
}

// TestRunCostCapture_PartialMetrics verifies that only non-zero metrics are copied.
func TestRunCostCapture_PartialMetrics(t *testing.T) {
	// Result with only some metrics populated.
	res := &Result{
		ExitCode:   0,
		TokensIn:   500,
		TokensOut:  0, // Zero, not copied.
		LatencyMs:  100,
		SpendUSD:   0.0, // Zero, not copied.
	}

	run := &Run{
		RunID:    "run-4",
		ThreadID: "thread-4",
	}

	run.CostFromResult(res)

	if run.TokensIn != 500 {
		t.Errorf("TokensIn = %d, want 500", run.TokensIn)
	}
	if run.TokensOut != 0 {
		t.Errorf("TokensOut = %d, want 0 (not copied because Result had zero)", run.TokensOut)
	}
	if run.LatencyMs != 100 {
		t.Errorf("LatencyMs = %d, want 100", run.LatencyMs)
	}
	if run.SpendUSD != 0.0 {
		t.Errorf("SpendUSD = %v, want 0.0", run.SpendUSD)
	}
}
