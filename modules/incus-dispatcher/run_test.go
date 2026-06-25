package main

import (
	"encoding/json"
	"testing"
	"time"
)

// STORY-0031 AC-2: all nine StumbleType constants exist with exact string values.
func TestStumbleTypeValues(t *testing.T) {
	want := map[StumbleType]string{
		StumbleRetry:               "retry",
		StumbleTimeout:             "timeout",
		StumbleVerificationFailure: "verification_failure",
		StumbleProviderFailure:     "provider_failure",
		StumbleDelegationLoop:      "delegation_loop",
		StumbleWorkspaceLoss:       "workspace_loss",
		StumbleDuplicateWork:       "duplicate_work",
		StumbleCostBlowout:         "cost_blowout",
		StumbleStarvation:          "starvation",
	}
	if len(want) != 9 {
		t.Errorf("expected exactly 9 StumbleType values, got %d", len(want))
	}
	for k, v := range want {
		if string(k) != v {
			t.Errorf("StumbleType %q: got underlying value %q, want %q", k, string(k), v)
		}
	}
}

// STORY-0031 AC-1: AddStumble appends in order and sets each signal's RunID.
func TestRunAddStumble(t *testing.T) {
	r := Run{
		RunID:    "run-001",
		ThreadID: "thread-abc",
	}

	ts1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)

	r.AddStumble(StumbleSignal{
		Type:            StumbleRetry,
		Ts:              ts1,
		EvidenceSummary: "first attempt failed",
	})
	r.AddStumble(StumbleSignal{
		Type:            StumbleTimeout,
		Ts:              ts2,
		EvidenceSummary: "timed out on second attempt",
	})

	if len(r.StumbleSignals) != 2 {
		t.Fatalf("StumbleSignals length: got %d, want 2", len(r.StumbleSignals))
	}

	// Verify order is preserved.
	if r.StumbleSignals[0].Type != StumbleRetry {
		t.Errorf("StumbleSignals[0].Type: got %q, want %q", r.StumbleSignals[0].Type, StumbleRetry)
	}
	if r.StumbleSignals[1].Type != StumbleTimeout {
		t.Errorf("StumbleSignals[1].Type: got %q, want %q", r.StumbleSignals[1].Type, StumbleTimeout)
	}

	// AddStumble must stamp each signal's RunID with r.RunID.
	for i, sig := range r.StumbleSignals {
		if sig.RunID != r.RunID {
			t.Errorf("StumbleSignals[%d].RunID: got %q, want %q", i, sig.RunID, r.RunID)
		}
	}

	// Timestamps are preserved as caller-provided.
	if !r.StumbleSignals[0].Ts.Equal(ts1) {
		t.Errorf("StumbleSignals[0].Ts: got %v, want %v", r.StumbleSignals[0].Ts, ts1)
	}
	if !r.StumbleSignals[1].Ts.Equal(ts2) {
		t.Errorf("StumbleSignals[1].Ts: got %v, want %v", r.StumbleSignals[1].Ts, ts2)
	}

	// EvidenceSummary is preserved.
	if r.StumbleSignals[0].EvidenceSummary != "first attempt failed" {
		t.Errorf("StumbleSignals[0].EvidenceSummary: got %q, want %q",
			r.StumbleSignals[0].EvidenceSummary, "first attempt failed")
	}
}

// STORY-0031: Run JSON round-trip preserves all fields with exact tags.
func TestRunJSONRoundTrip(t *testing.T) {
	ts := time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC)
	r := Run{
		RunID:       "run-abc",
		ThreadID:    "thread-xyz",
		ParentRunID: "run-parent",
		StumbleSignals: []StumbleSignal{
			{
				Type:            StumbleVerificationFailure,
				Ts:              ts,
				RunID:           "run-abc",
				EvidenceSummary: "test suite failed",
			},
		},
	}

	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Verify exact JSON field names.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}
	for _, key := range []string{"run_id", "thread_id", "parent_run_id", "stumble_signals"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("JSON missing key %q", key)
		}
	}

	var got Run
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.RunID != r.RunID {
		t.Errorf("RunID: got %q, want %q", got.RunID, r.RunID)
	}
	if got.ThreadID != r.ThreadID {
		t.Errorf("ThreadID: got %q, want %q", got.ThreadID, r.ThreadID)
	}
	if got.ParentRunID != r.ParentRunID {
		t.Errorf("ParentRunID: got %q, want %q", got.ParentRunID, r.ParentRunID)
	}
	if len(got.StumbleSignals) != 1 {
		t.Fatalf("StumbleSignals length: got %d, want 1", len(got.StumbleSignals))
	}
	sig := got.StumbleSignals[0]
	if sig.Type != StumbleVerificationFailure {
		t.Errorf("StumbleSignals[0].Type: got %q, want %q", sig.Type, StumbleVerificationFailure)
	}
	if sig.RunID != "run-abc" {
		t.Errorf("StumbleSignals[0].RunID: got %q, want %q", sig.RunID, "run-abc")
	}
	if !sig.Ts.Equal(ts) {
		t.Errorf("StumbleSignals[0].Ts: got %v, want %v", sig.Ts, ts)
	}

	// parent_run_id is omitempty — verify it's absent when empty.
	r2 := Run{RunID: "run-no-parent", ThreadID: "t1"}
	b2, err := json.Marshal(r2)
	if err != nil {
		t.Fatalf("Marshal r2: %v", err)
	}
	var raw2 map[string]json.RawMessage
	if err := json.Unmarshal(b2, &raw2); err != nil {
		t.Fatalf("Unmarshal raw2: %v", err)
	}
	if _, ok := raw2["parent_run_id"]; ok {
		t.Error("parent_run_id should be omitted when empty")
	}
}

// TestRunShape verifies ITER-0008 unified Run struct shape: all new fields (STORY-0011/0015/0035)
// are present, round-trip correctly, and maintain back-compat (omitempty for zero values).
func TestRunShape(t *testing.T) {
	// Test 1: Fully-populated Run with all ITER-0008 fields round-trips correctly.
	fullRun := Run{
		RunID:       "run-full-001",
		ThreadID:    "thread-full-001",
		ParentRunID: "run-parent-001",
		WorkerID:    "worker-exec-01",       // STORY-0011
		WorkerKind:  "temporal-worker",      // STORY-0011
		PolicyID:    "policy-v2-enforce",    // STORY-0011
		ArtifactRefs: []ArtifactRef{         // STORY-0015
			{Kind: ArtifactDiff, Ref: "s3://bucket/run-001/diff.patch"},
			{Kind: ArtifactSynthesis, Ref: "file:///tmp/synthesis.txt"},
		},
		LogRefs: []string{"s3://bucket/run-001/log.txt", "file:///tmp/local.log"}, // STORY-0015
		ProviderInstance: "anthropic-primary",           // STORY-0035 AC-1
		ModelID:          "claude-3-5-haiku-20241022",   // STORY-0035 AC-1
		BudgetSnapshot: &BudgetSnapshot{                 // STORY-0035 AC-2
			LimitTokens: 1000000,
			SpentTokens: 45000,
			Currency:    "tokens",
		},
		StumbleSignals: []StumbleSignal{
			{Type: StumbleRetry, Ts: time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC), EvidenceSummary: "test"},
		},
	}

	// Marshal to JSON.
	b, err := json.Marshal(fullRun)
	if err != nil {
		t.Fatalf("Marshal fullRun: %v", err)
	}

	// Unmarshal to verify round-trip.
	var got Run
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal fullRun: %v", err)
	}

	// Verify all fields are present.
	if got.RunID != fullRun.RunID {
		t.Errorf("RunID: got %q, want %q", got.RunID, fullRun.RunID)
	}
	if got.ThreadID != fullRun.ThreadID {
		t.Errorf("ThreadID: got %q, want %q", got.ThreadID, fullRun.ThreadID)
	}
	if got.ParentRunID != fullRun.ParentRunID {
		t.Errorf("ParentRunID: got %q, want %q", got.ParentRunID, fullRun.ParentRunID)
	}

	// STORY-0011 fields
	if got.WorkerID != fullRun.WorkerID {
		t.Errorf("WorkerID: got %q, want %q", got.WorkerID, fullRun.WorkerID)
	}
	if got.WorkerKind != fullRun.WorkerKind {
		t.Errorf("WorkerKind: got %q, want %q", got.WorkerKind, fullRun.WorkerKind)
	}
	if got.PolicyID != fullRun.PolicyID {
		t.Errorf("PolicyID: got %q, want %q", got.PolicyID, fullRun.PolicyID)
	}

	// STORY-0015 ArtifactRefs
	if len(got.ArtifactRefs) != len(fullRun.ArtifactRefs) {
		t.Fatalf("ArtifactRefs length: got %d, want %d", len(got.ArtifactRefs), len(fullRun.ArtifactRefs))
	}
	for i, ar := range got.ArtifactRefs {
		if ar.Kind != fullRun.ArtifactRefs[i].Kind {
			t.Errorf("ArtifactRefs[%d].Kind: got %q, want %q", i, ar.Kind, fullRun.ArtifactRefs[i].Kind)
		}
		if ar.Ref != fullRun.ArtifactRefs[i].Ref {
			t.Errorf("ArtifactRefs[%d].Ref: got %q, want %q", i, ar.Ref, fullRun.ArtifactRefs[i].Ref)
		}
	}

	// STORY-0015 LogRefs
	if len(got.LogRefs) != len(fullRun.LogRefs) {
		t.Fatalf("LogRefs length: got %d, want %d", len(got.LogRefs), len(fullRun.LogRefs))
	}
	for i, lr := range got.LogRefs {
		if lr != fullRun.LogRefs[i] {
			t.Errorf("LogRefs[%d]: got %q, want %q", i, lr, fullRun.LogRefs[i])
		}
	}

	// STORY-0035 AC-1 fields
	if got.ProviderInstance != fullRun.ProviderInstance {
		t.Errorf("ProviderInstance: got %q, want %q", got.ProviderInstance, fullRun.ProviderInstance)
	}
	if got.ModelID != fullRun.ModelID {
		t.Errorf("ModelID: got %q, want %q", got.ModelID, fullRun.ModelID)
	}

	// STORY-0035 AC-2 BudgetSnapshot
	if got.BudgetSnapshot == nil {
		t.Fatal("BudgetSnapshot is nil, want non-nil")
	}
	if got.BudgetSnapshot.LimitTokens != fullRun.BudgetSnapshot.LimitTokens {
		t.Errorf("BudgetSnapshot.LimitTokens: got %d, want %d", got.BudgetSnapshot.LimitTokens, fullRun.BudgetSnapshot.LimitTokens)
	}
	if got.BudgetSnapshot.SpentTokens != fullRun.BudgetSnapshot.SpentTokens {
		t.Errorf("BudgetSnapshot.SpentTokens: got %d, want %d", got.BudgetSnapshot.SpentTokens, fullRun.BudgetSnapshot.SpentTokens)
	}
	if got.BudgetSnapshot.Currency != fullRun.BudgetSnapshot.Currency {
		t.Errorf("BudgetSnapshot.Currency: got %q, want %q", got.BudgetSnapshot.Currency, fullRun.BudgetSnapshot.Currency)
	}

	// Test 2: Minimal Run (RunID, ThreadID only) maintains back-compat by omitting all new fields in JSON.
	minimalRun := Run{
		RunID:    "run-min-001",
		ThreadID: "thread-min-001",
	}

	b2, err := json.Marshal(minimalRun)
	if err != nil {
		t.Fatalf("Marshal minimalRun: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b2, &raw); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	// New ITER-0008 fields should not appear in JSON when zero-valued (omitempty).
	newFields := []string{
		"worker_id", "worker_kind", "policy_id",
		"artifact_refs", "log_refs",
		"provider_instance", "model_id", "budget_snapshot",
	}
	for _, field := range newFields {
		if _, ok := raw[field]; ok {
			t.Errorf("minimal Run should not emit %q in JSON (omitempty), but it did", field)
		}
	}

	// Core fields (run_id, thread_id) must still be present.
	for _, field := range []string{"run_id", "thread_id"} {
		if _, ok := raw[field]; !ok {
			t.Errorf("minimal Run should emit %q, but it's missing", field)
		}
	}
}
