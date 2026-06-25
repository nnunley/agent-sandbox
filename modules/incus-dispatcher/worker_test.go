package main

import (
	"errors"
	"testing"
)

// TestWorker_HasCapability proves Worker.HasCapability (STORY-0011 AC-2).
func TestWorker_HasCapability(t *testing.T) {
	w := Worker{
		WorkerID:     "w1",
		WorkerKind:   WorkerKindIncusContainer,
		Capabilities: []string{"code-review", "synthesis", "verification"},
	}

	if !w.HasCapability("code-review") {
		t.Error("HasCapability should return true for code-review")
	}
	if !w.HasCapability("synthesis") {
		t.Error("HasCapability should return true for synthesis")
	}
	if w.HasCapability("nonexistent") {
		t.Error("HasCapability should return false for nonexistent capability")
	}
	if w.HasCapability("") {
		t.Error("HasCapability should return false for empty string")
	}
}

// TestWorker_IsPolicyAllowed proves Worker.IsPolicyAllowed (STORY-0011 AC-3).
func TestWorker_IsPolicyAllowed(t *testing.T) {
	w := Worker{
		WorkerID:        "w1",
		WorkerKind:      WorkerKindLocal,
		AllowedPolicies: []string{"policy-1@v1", "policy-2@v2"},
	}

	if !w.IsPolicyAllowed("policy-1@v1") {
		t.Error("IsPolicyAllowed should return true for policy-1@v1")
	}
	if !w.IsPolicyAllowed("policy-2@v2") {
		t.Error("IsPolicyAllowed should return true for policy-2@v2")
	}
	if w.IsPolicyAllowed("policy-3@v1") {
		t.Error("IsPolicyAllowed should return false for policy-3@v1")
	}
	if w.IsPolicyAllowed("") {
		t.Error("IsPolicyAllowed should return false for empty string")
	}
}

// TestWorker_Fields proves Worker struct contains required fields (STORY-0011 AC-1/2/3).
func TestWorker_Fields(t *testing.T) {
	w := Worker{
		WorkerID:         "test-worker",
		WorkerKind:       WorkerKindMicroVM,
		Capabilities:     []string{"code-review"},
		AllowedPolicies:  []string{"policy-1@v1"},
	}

	if w.WorkerID != "test-worker" {
		t.Errorf("WorkerID mismatch: got %q", w.WorkerID)
	}
	if w.WorkerKind != WorkerKindMicroVM {
		t.Errorf("WorkerKind mismatch: got %v", w.WorkerKind)
	}
	if len(w.Capabilities) != 1 || w.Capabilities[0] != "code-review" {
		t.Errorf("Capabilities mismatch: got %v", w.Capabilities)
	}
	if len(w.AllowedPolicies) != 1 || w.AllowedPolicies[0] != "policy-1@v1" {
		t.Errorf("AllowedPolicies mismatch: got %v", w.AllowedPolicies)
	}
}

// TestWorkerKind_Constants proves WorkerKind type and named constants (STORY-0011 AC-1).
func TestWorkerKind_Constants(t *testing.T) {
	// Verify the constants are defined and have expected values.
	if WorkerKindLocal != "local" {
		t.Errorf("WorkerKindLocal mismatch: got %q", WorkerKindLocal)
	}
	if WorkerKindIncusContainer != "incus-container" {
		t.Errorf("WorkerKindIncusContainer mismatch: got %q", WorkerKindIncusContainer)
	}
	if WorkerKindMicroVM != "microvm" {
		t.Errorf("WorkerKindMicroVM mismatch: got %q", WorkerKindMicroVM)
	}
	if WorkerKindResearch != "research" {
		t.Errorf("WorkerKindResearch mismatch: got %q", WorkerKindResearch)
	}
}

// TestDispatcher_SelectionOrder proves deterministic selection: first eligible worker in registry order.
func TestDispatcher_SelectionOrder(t *testing.T) {
	// Two workers with the same capability; first should be selected.
	w1 := Worker{
		WorkerID:         "first",
		WorkerKind:       WorkerKindLocal,
		Capabilities:     []string{"code-review"},
		AllowedPolicies:  []string{"policy@v1"},
	}
	w2 := Worker{
		WorkerID:         "second",
		WorkerKind:       WorkerKindLocal,
		Capabilities:     []string{"code-review"},
		AllowedPolicies:  []string{"policy@v1"},
	}

	d := NewDispatcher([]Worker{w1, w2})
	run, err := d.Dispatch("code-review", "policy@v1", "anthropic", "claude-3-5-haiku", nil)

	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if run.WorkerID != "first" {
		t.Fatalf("expected first worker to be selected, got %q", run.WorkerID)
	}

	// Reverse order; verify first in registry order is still selected.
	d2 := NewDispatcher([]Worker{w2, w1})
	run2, err := d2.Dispatch("code-review", "policy@v1", "anthropic", "claude-3-5-haiku", nil)

	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if run2.WorkerID != "second" {
		t.Fatalf("expected second worker (first in reversed registry) to be selected, got %q", run2.WorkerID)
	}
}

// TestDispatcher_CapabilityMismatch proves dispatch rejects if capability is not found.
func TestDispatcher_CapabilityMismatch(t *testing.T) {
	w := Worker{
		WorkerID:        "w1",
		WorkerKind:      WorkerKindLocal,
		Capabilities:    []string{"code-review"},
		AllowedPolicies: []string{"policy@v1"},
	}

	d := NewDispatcher([]Worker{w})
	run, err := d.Dispatch("nonexistent", "policy@v1", "anthropic", "claude-3-5-haiku", nil)

	if err == nil {
		t.Fatal("dispatch should fail for nonexistent capability")
	}
	if run != nil {
		t.Fatal("dispatch should return nil run on failure")
	}
	// Check that the error wraps ErrNoEligibleWorker.
	if !errors.Is(err, ErrNoEligibleWorker) {
		t.Fatalf("expected error to wrap ErrNoEligibleWorker, got %v", err)
	}
}

// TestDispatcher_PolicyMismatch proves dispatch rejects if policy is not in allowed list.
func TestDispatcher_PolicyMismatch(t *testing.T) {
	w := Worker{
		WorkerID:        "w1",
		WorkerKind:      WorkerKindLocal,
		Capabilities:    []string{"code-review"},
		AllowedPolicies: []string{"policy-a@v1"},
	}

	d := NewDispatcher([]Worker{w})
	run, err := d.Dispatch("code-review", "policy-b@v1", "anthropic", "claude-3-5-haiku", nil)

	if err == nil {
		t.Fatal("dispatch should fail for disallowed policy")
	}
	if run != nil {
		t.Fatal("dispatch should return nil run on failure")
	}
}

// TestDispatcher_BudgetSnapshot proves budget is captured as a snapshot.
func TestDispatcher_BudgetSnapshot(t *testing.T) {
	w := Worker{
		WorkerID:        "w1",
		WorkerKind:      WorkerKindLocal,
		Capabilities:    []string{"code-review"},
		AllowedPolicies: []string{"policy@v1"},
	}

	d := NewDispatcher([]Worker{w})
	budget := &BudgetSnapshot{
		LimitTokens: 100000,
		SpentTokens: 10000,
	}

	run, err := d.Dispatch("code-review", "policy@v1", "anthropic", "claude-3-5-haiku", budget)

	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	// Verify the snapshot is present and correct.
	if run.BudgetSnapshot == nil {
		t.Fatal("BudgetSnapshot is nil")
	}
	if run.BudgetSnapshot.LimitTokens != 100000 {
		t.Fatalf("LimitTokens mismatch: got %d, want 100000", run.BudgetSnapshot.LimitTokens)
	}
	if run.BudgetSnapshot.SpentTokens != 10000 {
		t.Fatalf("SpentTokens mismatch: got %d, want 10000", run.BudgetSnapshot.SpentTokens)
	}

	// Verify the snapshot is independent (mutating the original doesn't affect the run).
	budget.SpentTokens = 50000
	if run.BudgetSnapshot.SpentTokens != 10000 {
		t.Fatalf("BudgetSnapshot was mutated by budget change: got %d, want 10000", run.BudgetSnapshot.SpentTokens)
	}
}

// TestDispatcher_NilBudgetSnapshot proves dispatch handles nil budget gracefully.
func TestDispatcher_NilBudgetSnapshot(t *testing.T) {
	w := Worker{
		WorkerID:        "w1",
		WorkerKind:      WorkerKindLocal,
		Capabilities:    []string{"code-review"},
		AllowedPolicies: []string{"policy@v1"},
	}

	d := NewDispatcher([]Worker{w})
	run, err := d.Dispatch("code-review", "policy@v1", "anthropic", "claude-3-5-haiku", nil)

	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	// BudgetSnapshot should be nil if input was nil.
	if run.BudgetSnapshot != nil {
		t.Fatal("BudgetSnapshot should be nil when budget is nil")
	}
}
