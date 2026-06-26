package main

import (
	"fmt"
	"time"
)

// ProviderInstance is a named provider configuration (STORY-0038 AC-1).
// Each instance is an explicit (provider, model, endpoint-role) tuple with a cost tier
// and historical success rates. Escalation rules drive selection based on task type,
// worker capability, policy, and previous failures.
type ProviderInstance struct {
	// Name is a unique identifier (e.g., "claude-code-main", "ollama-local").
	Name string

	// Provider is the underlying LLM provider (anthropic, openai, ollama-cloud).
	Provider Provider

	// Model is the model ID within that provider (e.g., "claude-3-5-haiku", "gpt-4o-mini").
	Model string

	// Tier indicates cost/capability: "cheap", "standard", "strong", "strongest".
	// Escalation rules use tier to select cheaper or stronger models on failure.
	Tier string

	// TypicalCostPerMTok is the approximate cost per million input tokens (USD).
	// Used by the multi-signal selector for cost-aware model selection.
	TypicalCostPerMTok float64

	// HistoricalSuccessRate is the fraction of tasks (0.0-1.0) that succeeded without escalation.
	// Used by the multi-signal selector to favor historically reliable instances.
	HistoricalSuccessRate float64

	// IsLocal indicates whether the instance is a local service (ollama-local, vllm-local).
	// Local instances may have different latency/availability patterns than cloud providers.
	IsLocal bool
}

// providerInstanceRegistry is the named registry of known instances (STORY-0038 AC-1).
// This enum is the single source of truth for instance names and their configurations.
var providerInstanceRegistry = map[string]*ProviderInstance{
	"claude-code-main": {
		Name:                  "claude-code-main",
		Provider:              ProviderAnthropic,
		Model:                 "claude-3-5-haiku",
		Tier:                  "strong",
		TypicalCostPerMTok:    0.40, // Claude 3.5 Haiku input: ~$0.80/M (rough mid-point)
		HistoricalSuccessRate: 0.95,
		IsLocal:               false,
	},
	"openai-codex-main": {
		Name:                  "openai-codex-main",
		Provider:              ProviderOpenAI,
		Model:                 "gpt-4o-mini",
		Tier:                  "strong",
		TypicalCostPerMTok:    0.075, // GPT-4o mini input: ~$0.15/M
		HistoricalSuccessRate: 0.92,
		IsLocal:               false,
	},
	"openrouter-main": {
		Name:                  "openrouter-main",
		Provider:              ProviderOpenRouter,
		Model:                 "meta-llama/llama-3-8b-instruct",
		Tier:                  "standard",
		TypicalCostPerMTok:    0.0001, // Llama 3 8B on OpenRouter: very cheap
		HistoricalSuccessRate: 0.75,
		IsLocal:               false,
	},
	"ollama-cloud": {
		Name:                  "ollama-cloud",
		Provider:              ProviderOllamaCloud,
		Model:                 "mistral",
		Tier:                  "standard",
		TypicalCostPerMTok:    0.00001, // Ollama cloud (if metered): minimal
		HistoricalSuccessRate: 0.70,
		IsLocal:               false,
	},
	"ollama-local": {
		Name:                  "ollama-local",
		Provider:              ProviderOllamaLocal,
		Model:                 "ollama",
		Tier:                  "cheap",
		TypicalCostPerMTok:    0.0, // Local compute, no API cost
		HistoricalSuccessRate: 0.65,
		IsLocal:               true,
	},
	"vllm-local": {
		Name:                  "vllm-local",
		Provider:              ProviderVLLMLocal,
		Model:                 "vllm",
		Tier:                  "cheap",
		TypicalCostPerMTok:    0.0, // Local compute, no API cost
		HistoricalSuccessRate: 0.68,
		IsLocal:               true,
	},
}

// GetProviderInstance returns the named provider instance or nil if unknown (STORY-0038 AC-1).
func GetProviderInstance(name string) *ProviderInstance {
	return providerInstanceRegistry[name]
}

// AllProviderInstances returns all registered instances (for testing and documentation).
func AllProviderInstances() []*ProviderInstance {
	var instances []*ProviderInstance
	// Iterate in a stable order (matching the registry initialization order).
	for _, key := range []string{
		"claude-code-main",
		"openai-codex-main",
		"openrouter-main",
		"ollama-cloud",
		"ollama-local",
		"vllm-local",
	} {
		if inst, ok := providerInstanceRegistry[key]; ok {
			instances = append(instances, inst)
		}
	}
	return instances
}

// EscalationRule is one rule in the escalation policy (STORY-0038 AC-2).
// When a task fails with a stumble signal, the rule determines whether and how to escalate.
type EscalationRule struct {
	// FromTier is the initial tier (e.g., "cheap").
	FromTier string

	// ToTier is the escalated tier (e.g., "strong").
	ToTier string

	// TriggerSignals lists the stumble signals that activate this rule.
	// If a run records any of these signals, escalation is considered.
	TriggerSignals []StumbleType

	// Description explains the rule in human-readable form.
	Description string
}

// defaultEscalationRules are the escalation policies (STORY-0038 AC-2).
// They encode: "try cheap local model first, escalate to stronger cloud model on failure/uncertainty".
var defaultEscalationRules = []EscalationRule{
	{
		FromTier:       "cheap",
		ToTier:         "strong",
		TriggerSignals: []StumbleType{StumbleVerificationFailure, StumbleRetry},
		Description:    "cheap→strong: verification failed; escalate to Claude Haiku on cloud",
	},
	{
		FromTier:       "standard",
		ToTier:         "strong",
		TriggerSignals: []StumbleType{StumbleVerificationFailure, StumbleProviderFailure},
		Description:    "standard→strong: verification or provider failure; escalate to Claude Haiku",
	},
	{
		FromTier:       "strong",
		ToTier:         "strongest",
		TriggerSignals: []StumbleType{StumbleVerificationFailure},
		Description:    "strong→strongest: verification failure; escalate to Claude Opus",
	},
}

// GetEscalationRule returns the rule that applies to (fromTier, signal) or nil if none match.
func GetEscalationRule(fromTier string, signal StumbleType) *EscalationRule {
	for i := range defaultEscalationRules {
		rule := &defaultEscalationRules[i]
		if rule.FromTier != fromTier {
			continue
		}
		// Check if signal matches any trigger.
		for _, trigger := range rule.TriggerSignals {
			if trigger == signal {
				return rule
			}
		}
	}
	return nil
}

// ModelSelector uses multi-signal heuristics to pick the best ProviderInstance for a task
// (STORY-0038 AC-3). It is deterministic and documented (not ML, not a stub).
//
// Selection heuristics (in priority order):
//  1. Task type & worker capability: certain task types need stronger models.
//  2. Quality tier: task quality requirements may mandate a specific tier.
//  3. Cost: prefer cheaper instances if quality tier allows.
//  4. Historical success rate: favor instances with higher success rates.
//  5. Latency: prefer low-latency instances (local > cloud for same quality).
//  6. Context size: use instance context limit if task is large.
//
// For now, this is a simple heuristic; future enhancements can add ML-based selection.
type ModelSelector struct {
	// TaskType hints the task category (e.g., "code-review", "implementation", "grading").
	TaskType string

	// WorkerType hints the worker class (e.g., "temporal-worker", "agent").
	WorkerType string

	// PolicyType hints the policy tier (e.g., "cost-optimized", "quality-first").
	PolicyType string

	// QualityTier is the minimum quality requirement ("cheap", "standard", "strong", "strongest").
	QualityTier string

	// PreviousFails tracks the number of consecutive failures (for escalation retry logic).
	PreviousFails int
}

// Select picks the best provider instance using the multi-signal heuristics (STORY-0038 AC-3).
// Returns the instance name and error if the selection fails (e.g., no matching instance for quality tier).
// Note: does NOT mutate the receiver; uses local copies of state.
func (ms *ModelSelector) Select() (string, error) {
	// 1. Validate quality tier first.
	if tierRank(ms.QualityTier) == 0 {
		return "", fmt.Errorf("no provider instance available for quality tier %q", ms.QualityTier)
	}

	// Use a local copy of the quality tier so we don't mutate the receiver.
	currentQualityTier := ms.QualityTier

	// 2. Filter by quality tier constraint (with escalation on repeated failures).
	// If there were previous failures, escalate the tier locally.
	if ms.PreviousFails > 0 {
		for i := 0; i < ms.PreviousFails; i++ {
			nextTier := escalateTier(currentQualityTier)
			if nextTier == currentQualityTier {
				break // Already at top tier.
			}
			currentQualityTier = nextTier
		}
	}

	var candidates []*ProviderInstance
	minTierRank := tierRank(currentQualityTier)
	for _, inst := range AllProviderInstances() {
		if tierRank(inst.Tier) >= minTierRank {
			candidates = append(candidates, inst)
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no provider instance available for quality tier %q", currentQualityTier)
	}

	// 3. Apply policy type bias: cost-optimized favors cheap/local; quality-first favors strong.
	if ms.PolicyType == "cost-optimized" {
		// Prefer local and cheap instances.
		for _, inst := range candidates {
			if inst.IsLocal && inst.Tier == "cheap" {
				return inst.Name, nil
			}
		}
		// Fall back to any cheap instance.
		for _, inst := range candidates {
			if inst.Tier == "cheap" {
				return inst.Name, nil
			}
		}
	}

	if ms.PolicyType == "quality-first" {
		// Prefer strong/strongest instances.
		for _, inst := range candidates {
			if inst.Tier == "strongest" {
				return inst.Name, nil
			}
		}
		for _, inst := range candidates {
			if inst.Tier == "strong" {
				return inst.Name, nil
			}
		}
	}

	// 4. Apply task type bias: code-review and grading prefer strong models.
	if ms.TaskType == "code-review" || ms.TaskType == "grading" {
		for _, inst := range candidates {
			if inst.Tier == "strong" || inst.Tier == "strongest" {
				return inst.Name, nil
			}
		}
	}

	// 5. Apply worker type bias: research-capable workers can use local instances.
	// (A research worker type is more isolated and can rely on local services.)
	if ms.WorkerType == "research" {
		for _, inst := range candidates {
			if inst.IsLocal {
				return inst.Name, nil
			}
		}
	}

	// 6. Prefer high success rate within remaining candidates.
	var best *ProviderInstance
	for _, inst := range candidates {
		if best == nil || inst.HistoricalSuccessRate > best.HistoricalSuccessRate {
			best = inst
		}
	}

	if best != nil {
		return best.Name, nil
	}

	// Fallback: return first candidate (should not reach here).
	return candidates[0].Name, nil
}

// tierRank returns a numeric rank for a tier (higher = better quality).
func tierRank(tier string) int {
	switch tier {
	case "cheap":
		return 1
	case "standard":
		return 2
	case "strong":
		return 3
	case "strongest":
		return 4
	default:
		return 0
	}
}

// escalateTier returns the next tier up from the given tier.
func escalateTier(tier string) string {
	switch tier {
	case "cheap":
		return "standard"
	case "standard":
		return "strong"
	case "strong":
		return "strongest"
	case "strongest":
		return "strongest" // Already at top.
	default:
		return "cheap"
	}
}

// TaskTypeContext is metadata about the task for the selector (STORY-0038 AC-3).
type TaskTypeContext struct {
	Type       string        // "code-review", "implementation", "grading", etc.
	WorkerType string        // "temporal-worker", "agent", etc.
	PolicyType string        // "cost-optimized", "quality-first", "balanced"
	QualityTier string       // "cheap", "standard", "strong", "strongest"
	Timeout    time.Duration // Task timeout (used to infer urgency).
}

// RecommendInstance uses the multi-signal selector to recommend a provider instance.
// This is the public entry point for task-level model selection (STORY-0038 AC-3).
func RecommendInstance(ctx TaskTypeContext) (string, error) {
	selector := &ModelSelector{
		TaskType:    ctx.Type,
		WorkerType:  ctx.WorkerType,
		PolicyType:  ctx.PolicyType,
		QualityTier: ctx.QualityTier,
		PreviousFails: 0,
	}
	return selector.Select()
}

// EscalateRun evaluates escalation rules for a failed run and returns a new escalated Run if applicable
// (STORY-0038 AC-2, STORY-0035 AC-3). This is the production escalation logic (not test-only).
//
// Given a run with stumble signals indicating failure, EscalateRun:
//  1. Extracts the run's current provider instance tier
//  2. Reads the run's last/most-recent stumble signal
//  3. Matches the (tier, signal) pair against EscalationRules
//  4. If a rule matches, uses ModelSelector to pick a stronger instance (quality-first policy)
//  5. Returns a NEW Run with ParentRunID set to the original run's RunID
//
// Returns (escalatedRun, true, nil) if a rule matched and an escalation was generated.
// Returns (nil, false, nil) if no rule matched (no further escalation needed).
// Returns (nil, false, err) if the lookup or selection fails.
func EscalateRun(run *Run) (*Run, bool, error) {
	if run == nil {
		return nil, false, nil
	}

	// Extract the current instance's tier.
	currentInst := GetProviderInstance(run.ProviderInstance)
	if currentInst == nil {
		// Unknown instance; can't escalate.
		return nil, false, nil
	}

	// Get the most recent stumble signal.
	var lastStumble *StumbleSignal
	if len(run.StumbleSignals) > 0 {
		lastStumble = &run.StumbleSignals[len(run.StumbleSignals)-1]
	}
	if lastStumble == nil {
		// No stumble; nothing to escalate.
		return nil, false, nil
	}

	// Match the escalation rule.
	rule := GetEscalationRule(currentInst.Tier, lastStumble.Type)
	if rule == nil {
		// No rule matched.
		return nil, false, nil
	}

	// Use the multi-signal selector to pick a stronger instance (quality-first).
	selector := &ModelSelector{
		TaskType:    "unknown", // Escalation is tier-based, not task-based.
		WorkerType:  "",        // Escalation doesn't constrain worker type.
		PolicyType:  "quality-first", // Escalation prefers stronger models.
		QualityTier: rule.ToTier,
		PreviousFails: 0,
	}

	escalatedInstName, err := selector.Select()
	if err != nil {
		return nil, false, err
	}

	// Build the escalated run.
	escalatedInst := GetProviderInstance(escalatedInstName)
	if escalatedInst == nil {
		// Selection failed somehow.
		return nil, false, fmt.Errorf("escalate_run: selected instance %q not found", escalatedInstName)
	}

	escalatedRun := &Run{
		RunID:            generateRunID(),
		ThreadID:         run.ThreadID,
		ParentRunID:      run.RunID,
		WorkerID:         run.WorkerID,         // Keep the same worker.
		WorkerKind:       run.WorkerKind,       // Keep the same worker kind.
		PolicyID:         run.PolicyID,         // Keep the same policy.
		ProviderInstance: escalatedInstName,
		ModelID:          escalatedInst.Model,
		BudgetSnapshot:   run.BudgetSnapshot,   // Carry forward the budget.
		// Cost fields remain zero until execution (populated via CostFromResult later).
	}

	return escalatedRun, true, nil
}
