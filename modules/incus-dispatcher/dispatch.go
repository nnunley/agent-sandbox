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
	// Defensively copy the registry (preserving order) so a later mutation of the caller's slice
	// cannot change dispatch decisions. Selection stays deterministic: first eligible in order.
	registry := append([]Worker(nil), workers...)
	return &Dispatcher{
		workers: registry,
	}
}

// Dispatch selects an eligible worker and returns a Run stamped with dispatch context.
//
// Parameters:
//   - requiredCapability: the capability the directive requires (e.g., "code-review")
//   - policyID: the versioned policy ID (e.g., "policy-1@v1")
//   - providerInstance: the provider instance name (e.g., "claude-code-main", "ollama-local")
//     or legacy provider string (e.g., "anthropic"). Will be resolved to an explicit instance
//     via ResolveModel (STORY-0035 AC-3). Unknown instances return an error (no guessing).
//   - model: the model ID within the provider (e.g., "claude-3-5-haiku"). Deprecated: for
//     backwards compatibility, if providerInstance is not a known instance name, provider and
//     model are used as-is (legacy path). New code should use instance names.
//   - budget: a snapshot of the token budget at dispatch time
//
// Returns a Run with:
//   - WorkerID, WorkerKind: the selected worker's identity
//   - PolicyID: the dispatching policy (as given)
//   - ProviderInstance: the resolved or explicit provider instance name
//   - ModelID: the resolved model ID
//   - BudgetSnapshot: a copy of the budget snapshot
//
// Selection criteria:
// 1. Worker MUST have the requiredCapability.
// 2. Worker MUST list policyID in AllowedPolicies (STORY-0011 AC-3 enforcement).
// 3. If multiple workers are eligible, the first in registry order is selected (deterministic).
//
// Model resolution (STORY-0035 AC-3):
// - If providerInstance is a known instance name, use it directly.
// - If providerInstance is unknown and model is set, fall back to legacy (provider, model) path.
// - If both are unknown, return an error (never silent default).
//
// If no worker satisfies both criteria, returns (nil, ErrNoEligibleWorker) and creates NO Run.
func (d *Dispatcher) Dispatch(
	requiredCapability, policyID, providerInstance, model string,
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

	// Resolve the provider instance (STORY-0035 AC-3).
	// Try to resolve providerInstance as an explicit instance name.
	var resolvedProvider string
	var resolvedModel string

	if inst := GetProviderInstance(providerInstance); inst != nil {
		// Known instance name: use it directly.
		resolvedProvider = providerInstance
		resolvedModel = inst.Model
	} else {
		// Unknown instance name: fall back to legacy (provider, model) path if model is set.
		// This maintains backwards compatibility with existing callers.
		if model != "" {
			resolvedProvider = providerInstance
			resolvedModel = model
		} else {
			// Neither a known instance nor a legacy provider+model: error.
			return nil, fmt.Errorf("dispatch: unknown provider instance or model: provider_instance=%q, model=%q", providerInstance, model)
		}
	}

	// Create a Run stamped with the dispatch decision (STORY-0011 AC-4, STORY-0035 AC-1/2/3).
	run := &Run{
		RunID:            generateRunID(), // Helper to create a unique run ID
		WorkerID:         selectedWorker.WorkerID,
		WorkerKind:       string(selectedWorker.WorkerKind),
		PolicyID:         policyID,
		ProviderInstance: resolvedProvider,
		ModelID:          resolvedModel,
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
