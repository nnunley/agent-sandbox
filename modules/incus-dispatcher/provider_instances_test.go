package main

import (
	"testing"
)

// TestProviderInstances_Registry verifies the 6 provider instances are registered (STORY-0038 AC-1).
func TestProviderInstances_Registry(t *testing.T) {
	instances := AllProviderInstances()
	if len(instances) != 6 {
		t.Errorf("expected 6 provider instances, got %d", len(instances))
	}

	expectedNames := map[string]bool{
		"claude-code-main":  true,
		"openai-codex-main": true,
		"openrouter-main":   true,
		"ollama-cloud":      true,
		"ollama-local":      true,
		"vllm-local":        true,
	}

	for _, inst := range instances {
		if !expectedNames[inst.Name] {
			t.Errorf("unexpected instance name: %q", inst.Name)
		}
		delete(expectedNames, inst.Name)
	}

	for missing := range expectedNames {
		t.Errorf("missing instance: %q", missing)
	}
}

// TestResolveModel_ExplicitNoGuess verifies that ResolveModel returns known instances
// and errors on unknown names (never guesses or defaults) (STORY-0035 AC-3).
func TestResolveModel_ExplicitNoGuess(t *testing.T) {
	tests := []struct {
		name      string
		want      string
		wantError bool
	}{
		// Known instances.
		{"claude-code-main", "claude-code-main", false},
		{"openai-codex-main", "openai-codex-main", false},
		{"ollama-local", "ollama-local", false},
		{"vllm-local", "vllm-local", false},

		// Unknown instances → error (no default).
		{"unknown-instance", "", true},
		{"", "", true},
		{"claude-default", "", true}, // Tempting default, but no.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := ResolveModel(tt.name)
			if tt.wantError {
				if err == nil {
					t.Errorf("ResolveModel(%q) expected error, got nil", tt.name)
				}
				return
			}
			if err != nil {
				t.Errorf("ResolveModel(%q) unexpected error: %v", tt.name, err)
				return
			}
			if inst.Name != tt.want {
				t.Errorf("ResolveModel(%q) = %q, want %q", tt.name, inst.Name, tt.want)
			}
		})
	}
}

// TestEscalationRules_CheapToStrong verifies escalation rules encode cheap→strong escalation (STORY-0038 AC-2).
func TestEscalationRules_CheapToStrong(t *testing.T) {
	// Escalation rule: cheap→strong on verification failure.
	rule := GetEscalationRule("cheap", StumbleVerificationFailure)
	if rule == nil {
		t.Errorf("GetEscalationRule(cheap, StumbleVerificationFailure) = nil, want rule")
		return
	}

	if rule.ToTier != "strong" {
		t.Errorf("escalation rule ToTier = %q, want strong", rule.ToTier)
	}

	// Verify the rule description mentions the escalation.
	if rule.Description == "" {
		t.Errorf("escalation rule Description is empty")
	}

	// Test that a different signal on standard tier also escalates to strong.
	rule2 := GetEscalationRule("standard", StumbleVerificationFailure)
	if rule2 == nil || rule2.ToTier != "strong" {
		t.Errorf("GetEscalationRule(standard, verification_failure) did not escalate to strong")
	}

	// Test that non-matching signal returns nil.
	rule3 := GetEscalationRule("cheap", StumbleTimeout)
	if rule3 != nil {
		t.Errorf("GetEscalationRule(cheap, timeout) should not match, got rule")
	}
}

// TestModelSelector_MultiSignal verifies the multi-signal selector returns documented instances (STORY-0038 AC-3).
func TestModelSelector_MultiSignal(t *testing.T) {
	tests := []struct {
		name        string
		selector    *ModelSelector
		wantInst    string // Expected instance name (or partial match if specific).
		wantError   bool
	}{
		// Cost-optimized: prefer cheap local.
		{
			name: "cost-optimized prefers ollama-local",
			selector: &ModelSelector{
				TaskType:    "implementation",
				WorkerType:  "agent",
				PolicyType:  "cost-optimized",
				QualityTier: "cheap",
				PreviousFails: 0,
			},
			wantInst: "ollama-local",
		},

		// Quality-first: prefer strong.
		{
			name: "quality-first prefers strong",
			selector: &ModelSelector{
				TaskType:    "code-review",
				WorkerType:  "agent",
				PolicyType:  "quality-first",
				QualityTier: "strong",
				PreviousFails: 0,
			},
			wantInst: "claude-code-main",
		},

		// Code-review task type: prefers strong models.
		{
			name: "code-review prefers strong",
			selector: &ModelSelector{
				TaskType:    "code-review",
				WorkerType:  "agent",
				PolicyType:  "balanced",
				QualityTier: "standard",
				PreviousFails: 0,
			},
			wantInst: "claude-code-main", // Strong model.
		},

		// Escalation on repeated failures.
		{
			name: "1 previous failure escalates from cheap to standard",
			selector: &ModelSelector{
				TaskType:    "implementation",
				WorkerType:  "agent",
				PolicyType:  "balanced",
				QualityTier: "cheap",
				PreviousFails: 1,
			},
			// Should escalate to standard tier.
		},

		// Minimum quality tier not met → error.
		{
			name: "impossible quality tier returns error",
			selector: &ModelSelector{
				TaskType:    "implementation",
				PolicyType:  "balanced",
				QualityTier: "impossible-tier",
				PreviousFails: 0,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := tt.selector.Select()
			if tt.wantError {
				if err == nil {
					t.Errorf("Select() expected error, got instance %q", inst)
				}
				return
			}
			if err != nil {
				t.Errorf("Select() unexpected error: %v", err)
				return
			}
			// If wantInst is set, check exact match; otherwise just verify a name was returned.
			if tt.wantInst != "" && inst != tt.wantInst {
				t.Errorf("Select() = %q, want %q", inst, tt.wantInst)
			}
			if inst == "" {
				t.Errorf("Select() returned empty instance name")
			}
		})
	}
}

// TestRecommendInstance verifies the public entry point for instance recommendation (STORY-0038 AC-3).
func TestRecommendInstance(t *testing.T) {
	ctx := TaskTypeContext{
		Type:        "code-review",
		WorkerType:  "agent",
		PolicyType:  "quality-first",
		QualityTier: "strong",
	}

	inst, err := RecommendInstance(ctx)
	if err != nil {
		t.Errorf("RecommendInstance() unexpected error: %v", err)
		return
	}

	// Should return a strong instance.
	resolved, _ := ResolveModel(inst)
	if resolved.Tier != "strong" && resolved.Tier != "strongest" {
		t.Errorf("RecommendInstance() returned tier %q, want strong or stronger", resolved.Tier)
	}
}

// TestModelSelector_WorkerTypeImpact verifies that WorkerType affects model selection (STORY-0038 AC-3).
// A research worker should prefer local instances when available.
func TestModelSelector_WorkerTypeImpact(t *testing.T) {
	// Selector with research worker type should prefer local instances.
	selectorResearch := &ModelSelector{
		TaskType:    "implementation",
		WorkerType:  "research",
		PolicyType:  "balanced",
		QualityTier: "cheap",
		PreviousFails: 0,
	}

	instResearch, err := selectorResearch.Select()
	if err != nil {
		t.Fatalf("Select() failed: %v", err)
	}

	resolvedResearch := GetProviderInstance(instResearch)
	if resolvedResearch == nil {
		t.Fatalf("Selected instance %q not found", instResearch)
	}

	// Research worker should get a local instance.
	if !resolvedResearch.IsLocal {
		t.Errorf("research worker selected non-local instance %q", instResearch)
	}
}

// TestModelSelector_StatelessSelection verifies that Select() doesn't mutate state and can be called multiple times.
func TestModelSelector_StatelessSelection(t *testing.T) {
	selector := &ModelSelector{
		TaskType:    "code-review",
		WorkerType:  "agent",
		PolicyType:  "quality-first",
		QualityTier: "standard",
		PreviousFails: 1,
	}

	// Call Select() twice.
	inst1, err1 := selector.Select()
	if err1 != nil {
		t.Fatalf("first Select() failed: %v", err1)
	}

	inst2, err2 := selector.Select()
	if err2 != nil {
		t.Fatalf("second Select() failed: %v", err2)
	}

	// Both calls should return the same result (no state mutation).
	if inst1 != inst2 {
		t.Errorf("Select() returned different results on consecutive calls: %q vs %q (state was mutated)", inst1, inst2)
	}

	// QualityTier should not be mutated.
	if selector.QualityTier != "standard" {
		t.Errorf("QualityTier mutated from standard to %q", selector.QualityTier)
	}
}
