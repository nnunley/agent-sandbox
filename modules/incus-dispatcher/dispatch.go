package main

import (
	"errors"
	"fmt"
	"sync/atomic"
)

// ErrNoEligibleWorker is returned when no worker satisfies the dispatch requirements
// (required capability AND policy allowlist).
var ErrNoEligibleWorker = errors.New("dispatch: no eligible worker")

// Dispatcher is a registry-backed worker selection engine. It maintains a list of
// available workers and performs dispatch decisions based on capability and policy constraints
// (STORY-0011 AC-3/4, STORY-0035 AC-1/2).
//
// Dispatch(requiredCapability, policyID, provider, model, budget) selects an eligible
// worker and returns a stamped Run with WorkerID, WorkerKind, PolicyID, ProviderInstance,
// ModelID, and BudgetSnapshot.
type Dispatcher struct {
	// workers is the immutable registry of available workers in stable order (stable for deterministic selection).
	workers []Worker
}

// NewDispatcher creates a Dispatcher from a worker registry.
// The workers slice is retained in registration order (for deterministic selection —
// when multiple workers are eligible, the first one in the registry is chosen).
func NewDispatcher(workers []Worker) *Dispatcher {
	// Retain the slice in its given order; no sorting or reordering.
	// This ensures deterministic selection across calls.
	return &Dispatcher{
		workers: workers,
	}
}

// Dispatch selects an eligible worker and returns a Run stamped with dispatch context.
//
// Parameters:
//   - requiredCapability: the capability the directive requires (e.g., "code-review")
//   - policyID: the versioned policy ID (e.g., "policy-1@v1")
//   - provider: the LLM provider (e.g., "anthropic")
//   - model: the model ID (e.g., "claude-3-5-haiku")
//   - budget: a snapshot of the token budget at dispatch time
//
// Returns a Run with:
//   - WorkerID, WorkerKind: the selected worker's identity
//   - PolicyID: the dispatching policy (as given)
//   - ProviderInstance, ModelID: the provider/model (as given, not resolved; see ITER-0008b STORY-0035 AC-3)
//   - BudgetSnapshot: a copy of the budget snapshot
//
// Selection criteria:
// 1. Worker MUST have the requiredCapability.
// 2. Worker MUST list policyID in AllowedPolicies (STORY-0011 AC-3 enforcement).
// 3. If multiple workers are eligible, the first in registry order is selected (deterministic).
//
// If no worker satisfies both criteria, returns (nil, ErrNoEligibleWorker) and creates NO Run.
func (d *Dispatcher) Dispatch(
	requiredCapability, policyID, provider, model string,
	budget *BudgetSnapshot,
) (*Run, error) {
	// Scan the registry in order, select the first eligible worker.
	var selectedWorker *Worker
	for i := range d.workers {
		w := &d.workers[i]
		if w.HasCapability(requiredCapability) && w.IsPolicyAllowed(policyID) {
			selectedWorker = w
			break
		}
	}

	if selectedWorker == nil {
		// No worker satisfies both capability and policy constraints.
		return nil, fmt.Errorf("%w: required_capability=%q, policy_id=%q", ErrNoEligibleWorker, requiredCapability, policyID)
	}

	// Create a Run stamped with the dispatch decision (STORY-0011 AC-4, STORY-0035 AC-1/2).
	run := &Run{
		RunID:            generateRunID(), // Helper to create a unique run ID
		WorkerID:         selectedWorker.WorkerID,
		WorkerKind:       string(selectedWorker.WorkerKind),
		PolicyID:         policyID,
		ProviderInstance: provider,
		ModelID:          model,
		BudgetSnapshot:   copyBudgetSnapshot(budget),
	}

	return run, nil
}

// runIDCounter backs generateRunID's monotonic sequence. A process-local atomic counter gives
// genuinely distinct ids without a time/random dependency (so tests are deterministic across the
// process while still unique). ITER-0008b can swap in a durable/global id scheme (UUID, snowflake).
var runIDCounter atomic.Uint64

// generateRunID returns a process-unique run id (e.g. "run-1", "run-2", ...). Distinct ids matter
// downstream: the audit log (SCENARIO-0125) and parent/child delegation lineage key off RunID, so a
// constant id would collide.
func generateRunID() string {
	return fmt.Sprintf("run-%d", runIDCounter.Add(1))
}

// copyBudgetSnapshot returns a deep copy of the budget snapshot (so the returned Run's
// snapshot is independent of mutations to the input).
func copyBudgetSnapshot(b *BudgetSnapshot) *BudgetSnapshot {
	if b == nil {
		return nil
	}
	return &BudgetSnapshot{
		LimitTokens: b.LimitTokens,
		SpentTokens: b.SpentTokens,
	}
}
