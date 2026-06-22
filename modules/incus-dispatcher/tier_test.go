package main

import "testing"

// STORY-0023 AC-1: the isolation tier is declared by the TEMPLATE (D1 mechanism),
// resolved from the vetted TemplateRule — never an author-settable Directive field.
// Policy.TierFor returns the tier a validated template runs at.

func TestPolicy_TierFor_ReturnsDeclaredTier(t *testing.T) {
	p := &Policy{Templates: map[string]TemplateRule{
		"fleet-go":      {AllowWorkerOrigin: true, Tier: TierFast},
		"fleet-trading": {AllowWorkerOrigin: false, Tier: TierHard},
	}}
	if got := p.TierFor("fleet-go"); got != TierFast {
		t.Errorf("TierFor(fleet-go) = %q, want %q", got, TierFast)
	}
	if got := p.TierFor("fleet-trading"); got != TierHard {
		t.Errorf("TierFor(fleet-trading) = %q, want %q", got, TierHard)
	}
}

// Fail-safe: a template whose rule leaves Tier unset resolves to Hard (most isolated),
// so a misconfigured template degrades safely rather than running in the weaker fast tier.
func TestPolicy_TierFor_UnsetDefaultsToHard(t *testing.T) {
	p := &Policy{Templates: map[string]TemplateRule{
		"fleet-go": {AllowWorkerOrigin: true}, // Tier unset
	}}
	if got := p.TierFor("fleet-go"); got != TierHard {
		t.Errorf("TierFor(unset) = %q, want %q (fail-safe)", got, TierHard)
	}
}

// Defensive: TierFor on a template not in the allowlist also defaults to Hard.
// (TierFor is only called after ValidateTemplate succeeds, but it must never
// silently grant the weaker tier for an unknown template.)
func TestPolicy_TierFor_UnknownDefaultsToHard(t *testing.T) {
	p := &Policy{Templates: map[string]TemplateRule{}}
	if got := p.TierFor("nope"); got != TierHard {
		t.Errorf("TierFor(unknown) = %q, want %q (fail-safe)", got, TierHard)
	}
}
