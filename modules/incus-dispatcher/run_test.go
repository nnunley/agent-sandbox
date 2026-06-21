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
