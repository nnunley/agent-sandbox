package main

import (
	"fmt"
	"time"
)

// BudgetLevel enumerates the budget tracking levels (STORY-0036 AC-1).
type BudgetLevel string

const (
	BudgetLevelPerMessage     BudgetLevel = "per_message"
	BudgetLevelPerRun         BudgetLevel = "per_run"
	BudgetLevelPerThread      BudgetLevel = "per_thread"
	BudgetLevelPerWorkerClass BudgetLevel = "per_worker_class"
	BudgetLevelPerProvider    BudgetLevel = "per_provider"
	BudgetLevelPerTimeWindow  BudgetLevel = "per_time_window"
)

// BudgetLimit represents a single budget threshold at one level (STORY-0036 AC-1).
type BudgetLimit struct {
	Level              BudgetLevel `json:"level"`
	HardCeiling        float64     `json:"hard_ceiling_usd"`     // Immutable unless human-approved (AC-2)
	EscalationThreshold float64    `json:"escalation_threshold_usd"` // When to escalate (can be auto-mutated)
	TimeWindowSecs     int64       `json:"time_window_secs,omitempty"` // For per-time-window limits
}

// BudgetPolicy groups budget limits across all six levels (STORY-0036 AC-1).
// Hard ceilings are protected from automatic mutation; escalation heuristics may be tuned.
type BudgetPolicy struct {
	PolicyID       string       `json:"policy_id"`
	PerMessage     *BudgetLimit `json:"per_message,omitempty"`
	PerRun         *BudgetLimit `json:"per_run,omitempty"`
	PerThread      *BudgetLimit `json:"per_thread,omitempty"`
	PerWorkerClass *BudgetLimit `json:"per_worker_class,omitempty"`
	PerProvider    *BudgetLimit `json:"per_provider,omitempty"`
	PerTimeWindow  *BudgetLimit `json:"per_time_window,omitempty"`
	LastModified   time.Time    `json:"last_modified"`
	LastModifiedBy string       `json:"last_modified_by"`
}

// NewBudgetPolicy creates a new BudgetPolicy with all levels initialized to nil.
// The caller populates the levels and hardceilings as needed.
func NewBudgetPolicy(policyID string) *BudgetPolicy {
	return &BudgetPolicy{
		PolicyID:     policyID,
		LastModified: time.Now(),
		LastModifiedBy: "system",
	}
}

// AllowAutoMutation reports whether a field name may be automatically mutated.
// Hard-ceiling fields (e.g., "per_thread_hard_ceiling") return false (protected).
// Escalation-heuristic fields (e.g., "per_thread_escalation_threshold") return true (tunable).
// STORY-0036 AC-2: hard guardrails remain protected unless explicitly human-approved.
func (bp *BudgetPolicy) AllowAutoMutation(fieldName string) bool {
	// Hard-ceiling fields are never auto-mutable; explicit operator action required.
	switch fieldName {
	case "per_message_hard_ceiling",
		"per_run_hard_ceiling",
		"per_thread_hard_ceiling",
		"per_worker_class_hard_ceiling",
		"per_provider_hard_ceiling",
		"per_time_window_hard_ceiling":
		return false
	}
	// Escalation heuristics are auto-mutable (tuned by the genome engine).
	switch fieldName {
	case "per_message_escalation_threshold",
		"per_run_escalation_threshold",
		"per_thread_escalation_threshold",
		"per_worker_class_escalation_threshold",
		"per_provider_escalation_threshold",
		"per_time_window_escalation_threshold":
		return true
	}
	// Unknown field: conservative default is no auto-mutation.
	return false
}

// ApplyAutoMutation attempts to mutate a tunable field value (escalation threshold).
// If the field is protected (hard ceiling), it returns an error.
// Otherwise, it updates the field and returns nil. This guards STORY-0032 AC-3
// (genome engine cannot raise hard ceilings without human approval).
func (bp *BudgetPolicy) ApplyAutoMutation(fieldName string, value float64) error {
	if !bp.AllowAutoMutation(fieldName) {
		return fmt.Errorf("budget_policy: field %q is protected from automatic mutation; requires human approval", fieldName)
	}
	// Escalation heuristics: update the corresponding limit's escalation threshold.
	switch fieldName {
	case "per_message_escalation_threshold":
		if bp.PerMessage != nil {
			bp.PerMessage.EscalationThreshold = value
		}
	case "per_run_escalation_threshold":
		if bp.PerRun != nil {
			bp.PerRun.EscalationThreshold = value
		}
	case "per_thread_escalation_threshold":
		if bp.PerThread != nil {
			bp.PerThread.EscalationThreshold = value
		}
	case "per_worker_class_escalation_threshold":
		if bp.PerWorkerClass != nil {
			bp.PerWorkerClass.EscalationThreshold = value
		}
	case "per_provider_escalation_threshold":
		if bp.PerProvider != nil {
			bp.PerProvider.EscalationThreshold = value
		}
	case "per_time_window_escalation_threshold":
		if bp.PerTimeWindow != nil {
			bp.PerTimeWindow.EscalationThreshold = value
		}
	}
	return nil
}

// ApplyOperatorMutation updates a field with explicit operator approval (STORY-0036 AC-2).
// It bypasses the auto-mutation guard and records the operator's identity for audit.
// Returns the old value and any error.
func (bp *BudgetPolicy) ApplyOperatorMutation(fieldName string, value float64, operator string) (float64, error) {
	var oldValue float64
	switch fieldName {
	case "per_message_hard_ceiling":
		if bp.PerMessage != nil {
			oldValue = bp.PerMessage.HardCeiling
			bp.PerMessage.HardCeiling = value
		}
	case "per_run_hard_ceiling":
		if bp.PerRun != nil {
			oldValue = bp.PerRun.HardCeiling
			bp.PerRun.HardCeiling = value
		}
	case "per_thread_hard_ceiling":
		if bp.PerThread != nil {
			oldValue = bp.PerThread.HardCeiling
			bp.PerThread.HardCeiling = value
		}
	case "per_worker_class_hard_ceiling":
		if bp.PerWorkerClass != nil {
			oldValue = bp.PerWorkerClass.HardCeiling
			bp.PerWorkerClass.HardCeiling = value
		}
	case "per_provider_hard_ceiling":
		if bp.PerProvider != nil {
			oldValue = bp.PerProvider.HardCeiling
			bp.PerProvider.HardCeiling = value
		}
	case "per_time_window_hard_ceiling":
		if bp.PerTimeWindow != nil {
			oldValue = bp.PerTimeWindow.HardCeiling
			bp.PerTimeWindow.HardCeiling = value
		}
	default:
		return 0, fmt.Errorf("budget_policy: unknown field %q", fieldName)
	}
	bp.LastModified = time.Now()
	bp.LastModifiedBy = operator
	return oldValue, nil
}

// BudgetEnforcement is the result of checking a run against budget limits.
type BudgetEnforcement struct {
	Allowed      bool    // true if the run may proceed
	LimitLevel   BudgetLevel // which level triggered the limit
	CurrentSpend float64 // current spend on the limited level
	HardCeiling  float64 // the hard ceiling for the limited level
	Reason       string  // human-readable explanation
}

// EnforceRunBudget checks whether a run would exceed any budget threshold.
// It returns an enforcement decision and the (current, hard ceiling) pair for the limiting level.
// A run is allowed only if all levels are within their hard ceilings.
// priorRuns is an optional list of prior runs for this thread (used to aggregate spend by provider/worker-class).
func (bp *BudgetPolicy) EnforceRunBudget(run *Run, currentThreadSpend float64, priorRuns ...[]*Run) *BudgetEnforcement {
	// Check per-thread level.
	if bp.PerThread != nil {
		nextSpend := currentThreadSpend + run.SpendUSD
		if nextSpend > bp.PerThread.HardCeiling {
			return &BudgetEnforcement{
				Allowed:      false,
				LimitLevel:   BudgetLevelPerThread,
				CurrentSpend: currentThreadSpend,
				HardCeiling:  bp.PerThread.HardCeiling,
				Reason: fmt.Sprintf(
					"per-thread budget exceeded: current=%.3f, run=%.3f, limit=%.3f",
					currentThreadSpend, run.SpendUSD, bp.PerThread.HardCeiling,
				),
			}
		}
	}

	// Check per-run level.
	if bp.PerRun != nil && run.SpendUSD > bp.PerRun.HardCeiling {
		return &BudgetEnforcement{
			Allowed:      false,
			LimitLevel:   BudgetLevelPerRun,
			CurrentSpend: run.SpendUSD,
			HardCeiling:  bp.PerRun.HardCeiling,
			Reason: fmt.Sprintf(
				"per-run budget exceeded: run=%.3f, limit=%.3f",
				run.SpendUSD, bp.PerRun.HardCeiling,
			),
		}
	}

	// Check per-message level (same as per-run since message granularity maps to individual runs).
	if bp.PerMessage != nil && run.SpendUSD > bp.PerMessage.HardCeiling {
		return &BudgetEnforcement{
			Allowed:      false,
			LimitLevel:   BudgetLevelPerMessage,
			CurrentSpend: run.SpendUSD,
			HardCeiling:  bp.PerMessage.HardCeiling,
			Reason: fmt.Sprintf(
				"per-message budget exceeded: run=%.3f, limit=%.3f",
				run.SpendUSD, bp.PerMessage.HardCeiling,
			),
		}
	}

	// Check per-provider level (requires prior runs for aggregation).
	if bp.PerProvider != nil && len(priorRuns) > 0 && len(priorRuns[0]) > 0 {
		priorList := priorRuns[0]
		providerSpend := run.SpendUSD
		for _, pr := range priorList {
			if pr != nil && pr.ProviderInstance == run.ProviderInstance {
				providerSpend += pr.SpendUSD
			}
		}
		if providerSpend > bp.PerProvider.HardCeiling {
			return &BudgetEnforcement{
				Allowed:      false,
				LimitLevel:   BudgetLevelPerProvider,
				CurrentSpend: providerSpend - run.SpendUSD,
				HardCeiling:  bp.PerProvider.HardCeiling,
				Reason: fmt.Sprintf(
					"per-provider (%s) budget exceeded: current=%.3f, run=%.3f, limit=%.3f",
					run.ProviderInstance, providerSpend-run.SpendUSD, run.SpendUSD, bp.PerProvider.HardCeiling,
				),
			}
		}
	}

	// Check per-worker-class level (requires prior runs for aggregation).
	if bp.PerWorkerClass != nil && len(priorRuns) > 0 && len(priorRuns[0]) > 0 {
		priorList := priorRuns[0]
		workerSpend := run.SpendUSD
		for _, pr := range priorList {
			if pr != nil && pr.WorkerKind == run.WorkerKind {
				workerSpend += pr.SpendUSD
			}
		}
		if workerSpend > bp.PerWorkerClass.HardCeiling {
			return &BudgetEnforcement{
				Allowed:      false,
				LimitLevel:   BudgetLevelPerWorkerClass,
				CurrentSpend: workerSpend - run.SpendUSD,
				HardCeiling:  bp.PerWorkerClass.HardCeiling,
				Reason: fmt.Sprintf(
					"per-worker-class (%s) budget exceeded: current=%.3f, run=%.3f, limit=%.3f",
					run.WorkerKind, workerSpend-run.SpendUSD, run.SpendUSD, bp.PerWorkerClass.HardCeiling,
				),
			}
		}
	}

	// Per-time-window is not enforced in this implementation.
	// REASON: The enforcement function does not have access to timestamps at the point of checking.
	// A full implementation would require passing the entire run history with timestamps,
	// or materializing a time-windowed aggregate at the dashboard/monitoring layer.
	// This is deferred to a future enhancement that layers on real-time usage tracking.

	// All levels passed: allow the run.
	return &BudgetEnforcement{
		Allowed: true,
		Reason:  "within all budget limits",
	}
}
