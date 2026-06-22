package main

import (
	"testing"
)

// TestScenario0067 is the CI contract for STORY-0076 AC-1 (provider routing): the dispatcher
// forwards --provider/--model to the worker (cheap implementer), and the grader is
// deterministic (git-based, no model). The golden-side export of the provider CLIs is the
// cluster check `run.sh provider-routing` (SCENARIO-0067); this proves the dispatcher half.
func TestScenario0067_ProviderRoutingForwardsToWorker(t *testing.T) {
	cases := []struct {
		name      string
		provider  Provider
		model     string
		wantProv  string
		wantModel string // "" => FLEET_MODEL must be absent
	}{
		{"openai cheap implementer", ProviderOpenAI, "gpt-4o-mini", "openai", "gpt-4o-mini"},
		{"ollama-cloud qwen", ProviderOllamaCloud, "qwen-coder", "ollama-cloud", "qwen-coder"},
		{"anthropic explicit", ProviderAnthropic, "claude-3-5-haiku", "anthropic", "claude-3-5-haiku"},
		{"empty provider defaults to anthropic, no model", Provider(""), "", "anthropic", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := ProviderWorkerEnv(tc.provider, tc.model)
			if err != nil {
				t.Fatalf("ProviderWorkerEnv(%q,%q) error: %v", tc.provider, tc.model, err)
			}
			if got := env[providerEnvProvider]; got != tc.wantProv {
				t.Errorf("FLEET_PROVIDER = %q, want %q", got, tc.wantProv)
			}
			got, present := env[providerEnvModel]
			if tc.wantModel == "" {
				if present {
					t.Errorf("FLEET_MODEL should be absent for empty model, got %q", got)
				}
			} else if got != tc.wantModel {
				t.Errorf("FLEET_MODEL = %q, want %q", got, tc.wantModel)
			}
		})
	}
}

func TestScenario0067_InvalidProviderRejected(t *testing.T) {
	if _, err := ProviderWorkerEnv(Provider("bogus"), "x"); err == nil {
		t.Fatal("expected error for invalid provider, got nil")
	}
	task := &Task{Provider: Provider("nope")}
	if err := applyProviderRouting(task); err == nil {
		t.Fatal("applyProviderRouting should reject an invalid provider")
	}
}

func TestScenario0067_ApplyRoutingMergesWithoutClobbering(t *testing.T) {
	// Caller-supplied env wins; routing fills the rest.
	task := &Task{
		Provider: ProviderOpenAI,
		Model:    "gpt-4o-mini",
		Env:      map[string]string{"EXISTING": "1", providerEnvProvider: "preset"},
	}
	if err := applyProviderRouting(task); err != nil {
		t.Fatalf("applyProviderRouting: %v", err)
	}
	if task.Env["EXISTING"] != "1" {
		t.Errorf("existing env clobbered: EXISTING=%q", task.Env["EXISTING"])
	}
	if task.Env[providerEnvProvider] != "preset" {
		t.Errorf("caller override clobbered: FLEET_PROVIDER=%q, want preset", task.Env[providerEnvProvider])
	}
	if task.Env[providerEnvModel] != "gpt-4o-mini" {
		t.Errorf("routing did not fill FLEET_MODEL: %q", task.Env[providerEnvModel])
	}
}

func TestScenario0067_ApplyRoutingInitializesNilEnv(t *testing.T) {
	task := &Task{Provider: ProviderOllamaCloud, Model: "qwen-coder"} // Env is nil
	if err := applyProviderRouting(task); err != nil {
		t.Fatalf("applyProviderRouting: %v", err)
	}
	if task.Env[providerEnvProvider] != "ollama-cloud" || task.Env[providerEnvModel] != "qwen-coder" {
		t.Errorf("routing not applied to nil env: %+v", task.Env)
	}
}

// TestScenario0067_GraderIsDeterministic proves the oracle never routes through a model: every
// default grade gate is a deterministic git/build command (make / go), with no LLM/provider
// tool in the command path. RunGrade's signature takes no provider/model (compile-time).
func TestScenario0067_GraderIsDeterministic(t *testing.T) {
	allowed := map[string]bool{"make": true, "go": true}
	for _, g := range defaultGradeGates() {
		if len(g.Cmd) == 0 {
			t.Errorf("gate %q has empty command", g.Name)
			continue
		}
		tool := g.Cmd[0]
		if !allowed[tool] {
			t.Errorf("grade gate %q uses non-deterministic tool %q (want make/go — no LLM/provider on the grade path)", g.Name, tool)
		}
	}
}
