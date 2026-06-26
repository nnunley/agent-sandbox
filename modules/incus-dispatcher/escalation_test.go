package main

import (
	"testing"
	"time"
)

// TestEscalateRun verifies the production escalation logic (STORY-0038 AC-2, AC-3).
// Given a run with a verification_failure stumble signal, EscalateRun should:
// 1. Match the escalation rule for the run's tier + stumble signal
// 2. Select a stronger instance using the multi-signal selector
// 3. Return a NEW Run with escalated ProviderInstance/ModelID and ParentRunID set
func TestEscalateRun(t *testing.T) {
	// Create a cheap run that failed.
	run1 := &Run{
		RunID:            "run-1",
		ThreadID:         "thread-1",
		ProviderInstance: "ollama-local",
		ModelID:          "ollama",
	}

	// Add a verification failure stumble signal.
	run1.AddStumble(StumbleSignal{
		Type:            StumbleVerificationFailure,
		Ts:              time.Now(),
		EvidenceSummary: "test suite failed",
	})

	// Create the result with cost metrics.
	result1 := &Result{
		ExitCode:   1,
		TokensIn:   100,
		TokensOut:  50,
		LatencyMs:  500,
		SpendUSD:   0.001,
	}

	// Copy cost to the run.
	run1.CostFromResult(result1)

	// Now escalate.
	run2, escalated, err := EscalateRun(run1)
	if err != nil {
		t.Fatalf("EscalateRun failed: %v", err)
	}

	if !escalated {
		t.Errorf("escalated = false, want true (should have found an escalation rule)")
	}

	if run2 == nil {
		t.Fatalf("run2 is nil")
	}

	// Verify the escalated run has parent linkage.
	if run2.ParentRunID != "run-1" {
		t.Errorf("ParentRunID = %q, want run-1", run2.ParentRunID)
	}

	// Verify the escalated run has a stronger instance.
	escalatedInst := GetProviderInstance(run2.ProviderInstance)
	if escalatedInst == nil {
		t.Fatalf("escalated instance %q not found in registry", run2.ProviderInstance)
	}

	origInst := GetProviderInstance(run1.ProviderInstance)
	if tierRank(escalatedInst.Tier) <= tierRank(origInst.Tier) {
		t.Errorf("escalated tier %q is not stronger than original tier %q", escalatedInst.Tier, origInst.Tier)
	}

	// Verify ThreadID is preserved.
	if run2.ThreadID != "thread-1" {
		t.Errorf("ThreadID = %q, want thread-1", run2.ThreadID)
	}
}

// TestEscalateRun_NoMatch verifies that EscalateRun returns escalated=false when no rule matches.
func TestEscalateRun_NoMatch(t *testing.T) {
	// Create a strong run (already at top tier).
	run := &Run{
		RunID:            "run-strong",
		ThreadID:         "thread-1",
		ProviderInstance: "claude-code-main",
		ModelID:          "claude-3-5-haiku",
	}

	// Add a stumble signal that doesn't trigger escalation for this tier.
	run.AddStumble(StumbleSignal{
		Type:            StumbleTimeout,
		Ts:              time.Now(),
		EvidenceSummary: "task timed out",
	})

	// Escalate should return false (no rule matched timeout for strong tier).
	run2, escalated, err := EscalateRun(run)
	if err != nil {
		t.Fatalf("EscalateRun failed: %v", err)
	}

	if escalated {
		t.Errorf("escalated = true, want false (no rule should match timeout for strong)")
	}

	if run2 != nil {
		t.Errorf("run2 should be nil when no escalation matched")
	}
}

// TestEscalateRun_MultipleStumbles verifies escalation uses the last/most recent stumble.
func TestEscalateRun_MultipleStumbles(t *testing.T) {
	run := &Run{
		RunID:            "run-multi",
		ThreadID:         "thread-1",
		ProviderInstance: "ollama-local",
		ModelID:          "ollama",
	}

	// Add multiple stumble signals.
	run.AddStumble(StumbleSignal{
		Type:            StumbleRetry,
		Ts:              time.Now(),
		EvidenceSummary: "first failure",
	})
	run.AddStumble(StumbleSignal{
		Type:            StumbleVerificationFailure,
		Ts:              time.Now().Add(time.Second),
		EvidenceSummary: "verification failed",
	})

	// Escalate should match the most recent stumble (verification_failure).
	run2, escalated, err := EscalateRun(run)
	if err != nil {
		t.Fatalf("EscalateRun failed: %v", err)
	}

	if !escalated {
		t.Errorf("escalated = false, want true")
	}

	if run2 == nil {
		t.Fatalf("run2 is nil")
	}

	if run2.ParentRunID != "run-multi" {
		t.Errorf("ParentRunID = %q, want run-multi", run2.ParentRunID)
	}
}
