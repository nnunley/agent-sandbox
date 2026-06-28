package main

import "testing"

func TestConfigureBudget(t *testing.T) {
	rt, _ := newRoute("/anthropic", "https://api.anthropic.com", "k", "anthropic", true, classFleet)
	ro, _ := newRoute("/openai", "https://api.openai.com", "k", "openai", true, classFleet)
	s := newServer([]route{rt, ro}, &nopLogSink{})

	env := map[string]string{
		"LLM_PROXY_RESERVE_PCT":             "0.25",
		"LLM_PROXY_RESERVE_PCT_ANTHROPIC":   "0.40",
		"LLM_PROXY_CEILING_PRIOR_ANTHROPIC": "2000000",
		"LLM_PROXY_RESERVE_PCT_OPENAI":      "bogus", // ignored
	}
	configureBudget(s, func(k string) string { return env[k] })

	if s.reservePct != 0.25 {
		t.Fatalf("reservePct=%v want 0.25", s.reservePct)
	}
	if s.reserveFor("anthropic") != 0.40 {
		t.Fatalf("anthropic reserve=%v want 0.40", s.reserveFor("anthropic"))
	}
	if s.reserveFor("openai") != 0.25 {
		t.Fatalf("openai reserve=%v want default 0.25 (bogus ignored)", s.reserveFor("openai"))
	}
	if s.priors["anthropic"] != 2_000_000 {
		t.Fatalf("anthropic prior=%d want 2000000", s.priors["anthropic"])
	}
}
