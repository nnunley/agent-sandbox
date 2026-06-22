package main

// Provider routing (STORY-0076 AC-1): route the IMPLEMENTER to a cheap provider/model while
// the grader/oracle stays deterministic (git-based, no model — see grader.go RunGrade, which
// takes no provider). The dispatcher's --provider/--model flags forward to the worker as
// FLEET_PROVIDER / FLEET_MODEL env vars; the worker's runner reads them to select the model
// (for non-anthropic providers the implementer reaches the model through the llm-proxy +
// claude-code-router, so the raw provider key never touches the worker — see creds.go).
//
// This keeps the cheap-implementer / strong-deterministic-grader split: the flags only steer
// the implementer; nothing here is consulted on the grade path.

// providerEnvProvider / providerEnvModel are the worker env keys the runner consumes.
const (
	providerEnvProvider = "FLEET_PROVIDER"
	providerEnvModel    = "FLEET_MODEL"
)

// ProviderWorkerEnv returns the env additions that route the implementer to (provider, model).
// It validates+normalizes the provider (empty → anthropic). The grader never sees these.
func ProviderWorkerEnv(p Provider, model string) (map[string]string, error) {
	prov := p
	if err := prov.ValidateProvider(); err != nil {
		return nil, err
	}
	env := map[string]string{
		providerEnvProvider: string(prov),
	}
	if model != "" {
		env[providerEnvModel] = model
	}
	return env, nil
}

// applyProviderRouting merges the provider-routing env into the task's worker env, so the
// --provider/--model flags actually reach the worker (previously they were parsed but dead).
// Existing task.Env entries win (caller-supplied overrides are respected). Returns an error
// only if the provider is invalid.
func applyProviderRouting(task *Task) error {
	env, err := ProviderWorkerEnv(task.Provider, task.Model)
	if err != nil {
		return err
	}
	if task.Env == nil {
		task.Env = map[string]string{}
	}
	for k, v := range env {
		if _, ok := task.Env[k]; !ok {
			task.Env[k] = v
		}
	}
	return nil
}
