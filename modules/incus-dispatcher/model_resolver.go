package main

import (
	"fmt"
)

// ResolveModel converts a model name to an explicit provider instance, without guessing or defaulting
// (STORY-0035 AC-3). An unknown name returns an error; this enforces explicit configuration.
//
// The resolution process:
//  1. If name is a known instance name (from the registry), return that instance.
//  2. If name is unknown, return an error (NOT a silent default).
//
// This ensures that model selection is deterministic and auditable.
func ResolveModel(name string) (*ProviderInstance, error) {
	if name == "" {
		return nil, fmt.Errorf("resolve_model: empty model name")
	}

	// Check if name is a registered instance.
	if inst := GetProviderInstance(name); inst != nil {
		return inst, nil
	}

	// Unknown name: return error, never guess or default.
	return nil, fmt.Errorf("resolve_model: unknown provider instance %q (must be one of: claude-code-main, openai-codex-main, openrouter-main, ollama-cloud, ollama-local, vllm-local)", name)
}
