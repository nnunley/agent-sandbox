package temporal

import (
	"testing"
)

// TestHumanUnrestrictedRescore validates that humans can rescore to any importance level without bounds.
func TestHumanUnrestrictedRescore(t *testing.T) {
	human := Actor{
		Role: ActorRoleHuman,
		ID:   "operator",
	}

	tests := []struct {
		name      string
		current   Importance
		proposed  Importance
		wantAllow bool
	}{
		{"Low to Critical", ImportanceLow, ImportanceCritical, true},
		{"Critical to Low", ImportanceCritical, ImportanceLow, true},
		{"Medium to High", ImportanceMedium, ImportanceHigh, true},
		{"No change", ImportanceHigh, ImportanceHigh, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, escalation, err := ValidateRescoreRequest(human, tt.current, tt.proposed, nil)
			if allowed != tt.wantAllow {
				t.Errorf("allowed = %v, want %v", allowed, tt.wantAllow)
			}
			if escalation {
				t.Errorf("escalation = %v, want false", escalation)
			}
			if err != nil {
				t.Errorf("err = %v, want nil", err)
			}
		})
	}
}

// TestAgentBoundedRescore validates that agents are limited to 1-tier jumps and cannot self-promote to Critical.
func TestAgentBoundedRescore(t *testing.T) {
	agent := Actor{
		Role: ActorRoleAgent,
		ID:   "agent-001",
	}

	tests := []struct {
		name              string
		current           Importance
		proposed          Importance
		wantAllow         bool
		wantEscalation    bool
		wantErrorContains string
	}{
		{
			"Low to Medium (1 tier up, allowed)",
			ImportanceLow,
			ImportanceMedium,
			true,
			false,
			"",
		},
		{
			"Medium to High (1 tier up, allowed)",
			ImportanceMedium,
			ImportanceHigh,
			true,
			false,
			"",
		},
		{
			"High to Low (2 tiers down, not allowed)",
			ImportanceHigh,
			ImportanceLow,
			false,
			true,
			"tier jump",
		},
		{
			"Low to Critical (3 tiers up, tier jump blocked)",
			ImportanceLow,
			ImportanceCritical,
			false,
			true,
			"tier jump",
		},
		{
			"Medium to Critical (2 tiers up, tier jump blocked)",
			ImportanceMedium,
			ImportanceCritical,
			false,
			true,
			"tier jump",
		},
		{
			"High to Medium (1 tier down, allowed)",
			ImportanceHigh,
			ImportanceMedium,
			true,
			false,
			"",
		},
		{
			"Critical to High (1 tier down, allowed)",
			ImportanceCritical,
			ImportanceHigh,
			true,
			false,
			"",
		},
		{
			"No change (same tier, allowed)",
			ImportanceHigh,
			ImportanceHigh,
			true,
			false,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, escalation, err := ValidateRescoreRequest(agent, tt.current, tt.proposed, nil)
			if allowed != tt.wantAllow {
				t.Errorf("allowed = %v, want %v", allowed, tt.wantAllow)
			}
			if escalation != tt.wantEscalation {
				t.Errorf("escalation = %v, want %v", escalation, tt.wantEscalation)
			}
			if tt.wantErrorContains != "" {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErrorContains)
				} else if !contains(err.Error(), tt.wantErrorContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErrorContains)
				}
			} else {
				if err != nil {
					t.Errorf("err = %v, want nil", err)
				}
			}
		})
	}
}

// TestEscalationRouting validates that out-of-bounds requests are routed to approval.
func TestEscalationRouting(t *testing.T) {
	agent := Actor{
		Role: ActorRoleAgent,
		ID:   "agent-002",
	}

	// Agent tries to self-promote to Critical
	allowed, escalation, err := ValidateRescoreRequest(agent, ImportanceLow, ImportanceCritical, nil)
	if allowed {
		t.Errorf("allowed = true, want false")
	}
	if !escalation {
		t.Errorf("escalation = false, want true")
	}
	if err == nil {
		t.Errorf("err = nil, want error")
	}

	t.Logf("Escalation error: %v", err)
}

// TestIsHumanUnrestricted validates the IsHumanUnrestricted helper function.
func TestIsHumanUnrestricted(t *testing.T) {
	human := Actor{Role: ActorRoleHuman, ID: "operator"}
	agent := Actor{Role: ActorRoleAgent, ID: "agent-001"}

	if !IsHumanUnrestricted(human) {
		t.Errorf("IsHumanUnrestricted(human) = false, want true")
	}
	if IsHumanUnrestricted(agent) {
		t.Errorf("IsHumanUnrestricted(agent) = true, want false")
	}
}

// TestIsAgentBounded validates the IsAgentBounded helper function.
func TestIsAgentBounded(t *testing.T) {
	agent := Actor{Role: ActorRoleAgent, ID: "agent-001"}

	tests := []struct {
		name        string
		current     Importance
		proposed    Importance
		wantAllowed bool
	}{
		{"1 tier up (allowed)", ImportanceLow, ImportanceMedium, true},
		{"1 tier down (allowed)", ImportanceHigh, ImportanceMedium, true},
		{"2 tiers up (not allowed)", ImportanceLow, ImportanceHigh, false},
		{"To Critical from High (not allowed)", ImportanceHigh, ImportanceCritical, false},
		{"From Critical to High (allowed)", ImportanceCritical, ImportanceHigh, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := IsAgentBounded(agent, tt.current, tt.proposed)
			if allowed != tt.wantAllowed {
				t.Errorf("allowed = %v, want %v", allowed, tt.wantAllowed)
			}
			if allowed && err != nil {
				t.Errorf("expected no error for allowed case, got %v", err)
			}
			if !allowed && err == nil {
				t.Errorf("expected error for not-allowed case, got nil")
			}
		})
	}
}

// contains is a helper to check if a string contains a substring.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
