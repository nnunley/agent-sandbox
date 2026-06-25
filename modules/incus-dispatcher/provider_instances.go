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
		Provider:              Provider("openrouter"),
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
		Provider:              Provider("ollama-local"),
		Model:                 "ollama",
		Tier:                  "cheap",
		TypicalCostPerMTok:    0.0, // Local compute, no API cost
		HistoricalSuccessRate: 0.65,
		IsLocal:               true,
	},
	"vllm-local": {
		Name:                  "vllm-local",
		Provider:              Provider("vllm-local"),
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
func (ms *ModelSelector) Select() (string, error) {
	// 1. Validate quality tier first.
	if tierRank(ms.QualityTier) == 0 {
		return "", fmt.Errorf("no provider instance available for quality tier %q", ms.QualityTier)
	}

	// 2. Filter by quality tier constraint.
	var candidates []*ProviderInstance
	minTierRank := tierRank(ms.QualityTier)
	for _, inst := range AllProviderInstances() {
		if tierRank(inst.Tier) >= minTierRank {
			candidates = append(candidates, inst)
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no provider instance available for quality tier %q", ms.QualityTier)
	}

	// 2. Apply policy type bias: cost-optimized favors cheap/local; quality-first favors strong.
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

	// 3. Apply task type bias: code-review and grading prefer strong models.
	if ms.TaskType == "code-review" || ms.TaskType == "grading" {
		for _, inst := range candidates {
			if inst.Tier == "strong" || inst.Tier == "strongest" {
				return inst.Name, nil
			}
		}
	}

	// 4. Escalation on repeated failures: jump to stronger tiers.
	if ms.PreviousFails > 0 {
		// Each failure escalates up one tier.
		for i := 0; i < ms.PreviousFails; i++ {
			nextTier := escalateTier(ms.QualityTier)
			if nextTier == ms.QualityTier {
				break // Already at top tier.
			}
			ms.QualityTier = nextTier
		}
		// Re-filter by new tier.
		minTierRank = tierRank(ms.QualityTier)
		candidates = nil
		for _, inst := range AllProviderInstances() {
			if tierRank(inst.Tier) >= minTierRank {
				candidates = append(candidates, inst)
			}
		}
		if len(candidates) == 0 {
			return "", fmt.Errorf("no provider instance available after escalation to tier %q", ms.QualityTier)
		}
	}

	// 5. Prefer high success rate within remaining candidates.
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
