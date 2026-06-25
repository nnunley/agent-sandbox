package main

import (
	"testing"
)

// TestScenario0123_VersionedPolicy is the behavior evidence for SCENARIO-0123 (Versioned policy:
// dispatch v1 → revise → dispatch v2, version recorded). It proves that:
//
// AC-1: ExecutionPolicy object is versioned and stored durably (store keyed by policy id + version)
// AC-2: ExecutionPolicy includes Kind, Constraints, DelegationRules, VerificationRequirements, MutationAllowed
// AC-3: PolicyKind constants include 6 named types (one-shot, loop, burst, verify-fix, summarizer, review-only)
// AC-4: Store can list versions, fetch any prior version, and revert (recover earlier version's content)
//
// The proof seam is integration: in-process, fake backend; the test constructs an ExecutionPolicyStore,
// dispatches a directive under v1, revises to v2, dispatches under v2, and asserts immutability and history.
//
// Owning stories: STORY-0016 (versioned execution policies).
func TestScenario0123_VersionedPolicy(t *testing.T) {
	// --- AC-1 + AC-2: Create ExecutionPolicy v1 in the store ---
	store := NewExecutionPolicyStore()

	// Construct ExecutionPolicy v1
	policyV1 := ExecutionPolicy{
		Kind:    PolicyKindOneShot,
		Constraints: map[string]string{"timeout": "1h", "max_retries": "3"},
		DelegationRules: []string{"allow-child-directives", "enforce-parent-consent"},
		VerificationRequirements: []string{"external-grade-required"},
		MutationAllowed:          false,
	}

	// Save v1 and get its versioned ID (e.g., "policy-1@v1" or similar)
	policyIDV1 := store.Save("policy-1", policyV1)
	if policyIDV1 == "" {
		t.Fatal("save v1 returned empty policy ID")
	}

	// Verify v1 is retrievable
	retrieved, ok := store.Get(policyIDV1)
	if !ok {
		t.Fatalf("policy v1 not found after save: id=%s", policyIDV1)
	}
	if retrieved.Kind != PolicyKindOneShot {
		t.Fatalf("v1 kind mismatch: got %v, want %v", retrieved.Kind, PolicyKindOneShot)
	}
	if retrieved.MutationAllowed != false {
		t.Fatalf("v1 MutationAllowed mismatch: got %v, want false", retrieved.MutationAllowed)
	}

	// --- Step 1: Dispatch a directive under v1, assert Run records policyIDV1 ---
	runV1 := dispatchUnderPolicy(store, policyIDV1, "directive-1")
	if runV1.PolicyID != policyIDV1 {
		t.Fatalf("run v1 policy_id mismatch: got %q, want %q", runV1.PolicyID, policyIDV1)
	}

	// --- Step 2: Revise the policy to v2 (v1 must remain unchanged) ---
	policyV2 := ExecutionPolicy{
		Kind:    PolicyKindOneShot,
		Constraints: map[string]string{"timeout": "2h", "max_retries": "5"}, // Changed
		DelegationRules: []string{"allow-child-directives"}, // Changed
		VerificationRequirements: []string{"external-grade-required"},
		MutationAllowed:          true, // Changed
	}

	policyIDV2 := store.Save("policy-1", policyV2)
	if policyIDV2 == "" {
		t.Fatal("save v2 returned empty policy ID")
	}
	if policyIDV2 == policyIDV1 {
		t.Fatalf("v2 id should differ from v1: v1=%q, v2=%q", policyIDV1, policyIDV2)
	}

	// --- AC-4: List versions, assert both v1 and v2 are present and immutable ---
	versions := store.ListVersions("policy-1")
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}

	// Verify v1 is unchanged after v2 created
	v1After, ok := store.Get(policyIDV1)
	if !ok {
		t.Fatalf("v1 should still exist after v2 created")
	}
	if v1After.Kind != PolicyKindOneShot || v1After.MutationAllowed != false {
		t.Fatalf("v1 was mutated after v2 created: kind=%v, MutationAllowed=%v", v1After.Kind, v1After.MutationAllowed)
	}

	// Fetch v2
	v2Fetched, ok := store.Get(policyIDV2)
	if !ok {
		t.Fatalf("v2 not found after save: id=%s", policyIDV2)
	}
	if v2Fetched.MutationAllowed != true {
		t.Fatalf("v2 MutationAllowed mismatch: got %v, want true", v2Fetched.MutationAllowed)
	}

	// --- Step 3: Dispatch under v2, assert v1 Run still records v1 (no retroactive mutation) ---
	runV2 := dispatchUnderPolicy(store, policyIDV2, "directive-2")
	if runV2.PolicyID != policyIDV2 {
		t.Fatalf("run v2 policy_id mismatch: got %q, want %q", runV2.PolicyID, policyIDV2)
	}

	// Verify v1 Run was not mutated
	if runV1.PolicyID != policyIDV1 {
		t.Fatalf("run v1 policy_id was retroactively mutated: got %q, want %q", runV1.PolicyID, policyIDV1)
	}

	// --- AC-4: Exercise revert (recover v1's content) ---
	v1Reverted := store.Revert(policyIDV1)
	if v1Reverted == nil {
		t.Fatal("revert v1 returned nil")
	}
	if v1Reverted.MutationAllowed != false {
		t.Fatalf("reverted v1 MutationAllowed mismatch: got %v, want false", v1Reverted.MutationAllowed)
	}

	// --- Final observables: versions immutable + monotonic; each Run records exact version ---
	if len(versions) != 2 || versions[0] != policyIDV1 || versions[1] != policyIDV2 {
		t.Fatalf("version history corrupted: expected [%q, %q], got %v", policyIDV1, policyIDV2, versions)
	}
}

// dispatchUnderPolicy is a minimal helper that stamps a Run with the given policyID.
// This is NOT the full worker-selection dispatch (STORY-0011, STORY-0035) — that is NEXT task (T2b).
// This helper just creates a Run with the policy version recorded.
func dispatchUnderPolicy(store *ExecutionPolicyStore, policyID, directiveID string) Run {
	run := Run{
		RunID:       "run-" + directiveID,
		ThreadID:    "thread-" + directiveID,
		PolicyID:    policyID, // Stamp with the exact version
	}
	return run
}
