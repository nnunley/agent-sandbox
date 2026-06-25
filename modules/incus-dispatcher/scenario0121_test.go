package main

import (
	"testing"
)

// TestScenario0121_PolicyDrivenDispatch is the behavior evidence for SCENARIO-0121
// (Policy-driven dispatch produces a Run with worker_kind/policy_id). It proves that:
//
// STORY-0011 AC-1: Worker includes WorkerKind field (values like local, incus-container, microvm, research)
// STORY-0011 AC-2: Worker includes Capabilities []string describing available tools/features
// STORY-0011 AC-3: Worker includes AllowedPolicies constraint (which policies may dispatch to this worker)
// STORY-0011 AC-4: Run is created with WorkerID, WorkerKind, PolicyID matching the dispatch decision
//
// STORY-0035 AC-1/2: Run also records ProviderInstance, ModelID, and BudgetSnapshot captured at dispatch time
// (AC-3/4 — model resolution and spend tracking — are deferred to ITER-0008b)
//
// The proof seam is integration: register ≥2 workers with distinct capabilities, create a versioned
// ExecutionPolicy with allowed-policies relationship, dispatch a directive requiring a specific capability,
// and assert the Run records the correct worker_id/worker_kind/policy_id/provider_instance/model_id/budget_snapshot.
// Also assert selection respects allowed_policies (workers not in the allowlist are rejected) and that
// no Run is created if no eligible worker exists.
//
// Owning stories: STORY-0011 (worker registry + policy-driven selection), STORY-0035 AC-1/2 (Run captures provider/model/budget).
func TestScenario0121_PolicyDrivenDispatch(t *testing.T) {
	// --- Setup: Create a worker registry with 2+ workers with distinct capabilities ---

	// Worker 1: has "code-review" but NOT "deployment"
	w1 := Worker{
		WorkerID:       "worker-1",
		WorkerKind:     WorkerKindIncusContainer,
		Capabilities:   []string{"code-review", "synthesis"},
		AllowedPolicies: []string{"policy-review@v1", "policy-synthesis@v1"},
	}

	// Worker 2: has "deployment" and "code-review"
	w2 := Worker{
		WorkerID:       "worker-2",
		WorkerKind:     WorkerKindMicroVM,
		Capabilities:   []string{"deployment", "code-review", "verification"},
		AllowedPolicies: []string{"policy-deployment@v1", "policy-review@v1"},
	}

	// Worker 3: has "code-review" but is NOT in the policy's allowed workers
	w3 := Worker{
		WorkerID:       "worker-3",
		WorkerKind:     WorkerKindLocal,
		Capabilities:   []string{"code-review", "analysis"},
		AllowedPolicies: []string{}, // No policies allowed — will always be rejected
	}

	registry := []Worker{w1, w2, w3}

	// --- Create a versioned ExecutionPolicy ---
	policyStore := NewExecutionPolicyStore()
	pol := ExecutionPolicy{
		Kind:                     PolicyKindOneShot,
		Constraints:              map[string]string{"timeout": "1h"},
		DelegationRules:          []string{},
		VerificationRequirements: []string{},
		MutationAllowed:          false,
	}
	policyID := policyStore.Save("policy-review", pol)

	// --- Dispatch context: provider, model, budget ---
	const (
		provider = "anthropic"
		model    = "claude-3-5-haiku"
	)
	budget := &BudgetSnapshot{
		LimitTokens: 100000,
		SpentTokens: 5000,
	}

	// --- CASE 1: Dispatch a directive requiring "code-review" capability ---
	// Expected: worker-1 should be selected (first eligible by stable order, w1 before w2 in registry)
	dispatcher := NewDispatcher(registry)
	run, err := dispatcher.Dispatch(
		requiredCapability("code-review"),
		policyID,
		provider,
		model,
		budget,
	)

	if err != nil {
		t.Fatalf("dispatch with code-review capability failed: %v", err)
	}
	if run == nil {
		t.Fatal("dispatch returned nil run")
	}

	// Assert Run is stamped with w1's details
	if run.WorkerID != "worker-1" {
		t.Fatalf("run worker_id mismatch: got %q, want %q", run.WorkerID, "worker-1")
	}
	if run.WorkerKind != string(WorkerKindIncusContainer) {
		t.Fatalf("run worker_kind mismatch: got %q, want %q", run.WorkerKind, string(WorkerKindIncusContainer))
	}
	if run.PolicyID != policyID {
		t.Fatalf("run policy_id mismatch: got %q, want %q", run.PolicyID, policyID)
	}

	// Assert STORY-0035 AC-1/2: provider/model/budget are captured
	if run.ProviderInstance != provider {
		t.Fatalf("run provider_instance mismatch: got %q, want %q", run.ProviderInstance, provider)
	}
	if run.ModelID != model {
		t.Fatalf("run model_id mismatch: got %q, want %q", run.ModelID, model)
	}
	if run.BudgetSnapshot == nil {
		t.Fatal("run budget_snapshot is nil")
	}
	if run.BudgetSnapshot.LimitTokens != 100000 {
		t.Fatalf("run budget limit_tokens mismatch: got %d, want 100000", run.BudgetSnapshot.LimitTokens)
	}
	if run.BudgetSnapshot.SpentTokens != 5000 {
		t.Fatalf("run budget spent_tokens mismatch: got %d, want 5000", run.BudgetSnapshot.SpentTokens)
	}

	// --- CASE 2: Dispatch requiring "deployment" capability ---
	// Expected: only worker-2 has "deployment"; should be selected
	run2, err := dispatcher.Dispatch(
		requiredCapability("deployment"),
		policyID,
		provider,
		model,
		budget,
	)

	if err != nil {
		t.Fatalf("dispatch with deployment capability failed: %v", err)
	}
	if run2 == nil {
		t.Fatal("dispatch returned nil run for deployment")
	}

	if run2.WorkerID != "worker-2" {
		t.Fatalf("run2 worker_id mismatch: got %q, want %q", run2.WorkerID, "worker-2")
	}
	if run2.WorkerKind != string(WorkerKindMicroVM) {
		t.Fatalf("run2 worker_kind mismatch: got %q, want %q", run2.WorkerKind, string(WorkerKindMicroVM))
	}

	// --- CASE 3: Test allowed_policies constraint ---
	// Create a second policy that is NOT in w1's allowed list
	pol2 := ExecutionPolicy{
		Kind:        PolicyKindOneShot,
		Constraints: map[string]string{},
	}
	policyID2 := policyStore.Save("policy-strict", pol2)

	// Dispatch under policyID2: w1 cannot use it (not in AllowedPolicies), w2 cannot either.
	// Should fail.
	run3, err := dispatcher.Dispatch(
		requiredCapability("code-review"),
		policyID2,
		provider,
		model,
		budget,
	)

	if err == nil {
		t.Fatalf("dispatch should fail for policy not in allowed list, got run: %v", run3)
	}
	if run3 != nil {
		t.Fatalf("expected nil run on policy-allowlist violation, got %v", run3)
	}

	// --- CASE 4: Dispatch requiring a capability NO worker has ---
	// Expected: no eligible worker exists, dispatch fails, no Run created
	run4, err := dispatcher.Dispatch(
		requiredCapability("nonexistent-capability"),
		policyID,
		provider,
		model,
		budget,
	)

	if err == nil {
		t.Fatalf("dispatch should fail for nonexistent capability, got run: %v", run4)
	}
	if run4 != nil {
		t.Fatalf("expected nil run when no capability match, got %v", run4)
	}
}

// requiredCapability is a helper to express a required capability for dispatch.
// In the future, this could be expanded to other dispatch inputs (e.g., required_tier, required_resource).
func requiredCapability(cap string) string {
	return cap
}
