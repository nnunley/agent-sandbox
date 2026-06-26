package main

import (
	"testing"
)

// TestProviderInstance_WorkerEnvFeasibility verifies that a selected ProviderInstance
// can flow through ProviderWorkerEnv/applyProviderRouting without validation rejection
// (STORY-0038 AC-1). This proves the taxonomy is coherent.
func TestProviderInstance_WorkerEnvFeasibility(t *testing.T) {
	// For each of the 6 provider instances, verify:
	// 1. The instance can be resolved
	// 2. Its Provider passes ValidateProvider
	// 3. ProviderWorkerEnv accepts the provider without error
	instanceNames := []string{
		"claude-code-main",
		"openai-codex-main",
		"openrouter-main",
		"ollama-cloud",
		"ollama-local",
		"vllm-local",
	}

	for _, name := range instanceNames {
		t.Run(name, func(t *testing.T) {
			// 1. Resolve the instance.
			inst := GetProviderInstance(name)
			if inst == nil {
				t.Fatalf("instance %q not found", name)
			}

			// 2. Verify the provider is valid.
			provider := inst.Provider
			if err := provider.ValidateProvider(); err != nil {
				t.Errorf("ValidateProvider failed: %v", err)
			}

			// 3. Verify ProviderWorkerEnv accepts the provider without error.
			env, err := ProviderWorkerEnv(provider, inst.Model)
			if err != nil {
				t.Errorf("ProviderWorkerEnv failed: %v", err)
			}

			// Assert the env map is populated correctly.
			if env[providerEnvProvider] != string(provider) {
				t.Errorf("providerEnvProvider = %q, want %q", env[providerEnvProvider], string(provider))
			}

			if env[providerEnvModel] != inst.Model {
				t.Errorf("providerEnvModel = %q, want %q", env[providerEnvModel], inst.Model)
			}
		})
	}
}

// TestApplyProviderRouting_SelectedInstance verifies that applying provider routing
// with a selected instance works end-to-end without errors.
func TestApplyProviderRouting_SelectedInstance(t *testing.T) {
	// Create a task with a selected instance's provider/model.
	selectedInst := GetProviderInstance("ollama-local")
	if selectedInst == nil {
		t.Fatalf("ollama-local not found")
	}

	task := &Task{
		Name:     "test-task",
		Repo:     "https://example.com/repo",
		Ref:      "main",
		Cmd:      []string{"bash", "-c", "echo test"},
		Provider: selectedInst.Provider,
		Model:    selectedInst.Model,
		Env:      map[string]string{},
	}

	// Apply routing.
	err := applyProviderRouting(task)
	if err != nil {
		t.Fatalf("applyProviderRouting failed: %v", err)
	}

	// Verify the env was populated.
	if task.Env[providerEnvProvider] == "" {
		t.Errorf("providerEnvProvider not set in task.Env")
	}

	if task.Env[providerEnvModel] == "" {
		t.Errorf("providerEnvModel not set in task.Env")
	}
}
