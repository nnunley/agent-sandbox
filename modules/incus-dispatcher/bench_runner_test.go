package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type fakeBenchRunner struct {
	results  map[string]*Result
	errs     map[string]error
	calls    int
	cleanups int
	lastTask Task
}

func (f *fakeBenchRunner) Run(_ context.Context, task Task) (*Result, error) {
	f.calls++
	f.lastTask = task
	key := fmt.Sprintf("%s/%s", task.Model, task.Name)
	if err := f.errs[key]; err != nil {
		return nil, err
	}
	if result := f.results[key]; result != nil {
		return result, nil
	}
	return &Result{}, nil
}

func (f *fakeBenchRunner) Cleanup() error {
	f.cleanups++
	return nil
}

func TestBenchRunner_RunSuite_UsesRunnerAndCollectsOracleOutcome(t *testing.T) {
	fake := &fakeBenchRunner{
		results: map[string]*Result{
			"gpt-4o-mini/task-1": {
				ExitCode: 0,
				Duration: 2 * time.Second,
				PatchData: []byte("diff"),
				ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: true},
				TokensIn: 11,
				TokensOut: 22,
				SpendUSD: 0.33,
			},
		},
	}
	bench := BenchRunner{Runner: fake}
	suite := &BenchSuite{Name: "fleet-core", Version: "v1", Hash: "abc", Tasks: []BenchTaskSpec{{Name: "task-1", Brief: "fix", Repo: "/repo", Ref: "main", OracleRef: "/oracle"}}}
	candidates := []BenchCandidate{{Name: "alpha", Provider: ProviderOpenAI, Model: "gpt-4o-mini"}}

	results, err := bench.RunSuite(context.Background(), suite, candidates)
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("results=%+v", results)
	}
	if fake.calls != 1 {
		t.Fatalf("runner calls=%d want 1", fake.calls)
	}
	if got := fake.lastTask.ExternalGradingCheckout; got != "/oracle" {
		t.Fatalf("oracle=%q want /oracle", got)
	}
	if got := fake.lastTask.Env[providerEnvProvider]; got != string(ProviderOpenAI) {
		t.Fatalf("provider env=%q", got)
	}
}

func TestBenchRunner_RunSuite_RecordsFailuresAndSkipsWithoutAborting(t *testing.T) {
	fake := &fakeBenchRunner{
		errs: map[string]error{"qwen3.6/task-2": context.DeadlineExceeded},
		results: map[string]*Result{
			"gpt-4o-mini/task-1": {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: true}},
			"qwen3.6/task-1":     {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 1, PatchApplied: true}},
		},
	}
	bench := BenchRunner{Runner: fake, DryRun: true}
	suite := &BenchSuite{Name: "fleet-core", Version: "v1", Hash: "abc", Tasks: []BenchTaskSpec{
		{Name: "task-1", Brief: "a", Repo: "/repo", Ref: "main", OracleRef: "/oracle/a"},
		{Name: "task-2", Brief: "b", Repo: "/repo", Ref: "main", OracleRef: "/oracle/b"},
	}}
	candidates := []BenchCandidate{
		{Name: "alpha", Provider: ProviderOpenAI, Model: "gpt-4o-mini"},
		{Name: "beta", Provider: ProviderOllamaLocal, Model: "qwen3.6"},
	}

	results, err := bench.RunSuite(context.Background(), suite, candidates)
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("results=%d want 4", len(results))
	}
	if !hasStatus(results, "beta", "task-2", "error") {
		t.Fatalf("missing error record: %+v", results)
	}
	if !hasStatus(results, "beta", "task-1", "failed") {
		t.Fatalf("missing failed grade record: %+v", results)
	}
}

func TestBenchRunner_RunSuite_CleansUpAfterEachTask(t *testing.T) {
	fake := &fakeBenchRunner{
		results: map[string]*Result{
			"gpt-4o-mini/task-1": {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: true}},
			"gpt-4o-mini/task-2": {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: true}},
		},
	}
	bench := BenchRunner{Runner: fake}
	suite := &BenchSuite{Name: "fleet-core", Version: "v1", Hash: "abc", Tasks: []BenchTaskSpec{
		{Name: "task-1", Brief: "a", Repo: "/repo", Ref: "main", OracleRef: "/oracle/a"},
		{Name: "task-2", Brief: "b", Repo: "/repo", Ref: "main", OracleRef: "/oracle/b"},
	}}
	candidates := []BenchCandidate{{Name: "alpha", Provider: ProviderOpenAI, Model: "gpt-4o-mini"}}

	if _, err := bench.RunSuite(context.Background(), suite, candidates); err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	if fake.cleanups != 2 {
		t.Fatalf("cleanups=%d want 2", fake.cleanups)
	}
}

func hasStatus(results []BenchTaskResult, candidateName, taskName, want string) bool {
	for _, result := range results {
		if result.Candidate.Name == candidateName && result.TaskName == taskName && result.Status == want {
			return true
		}
	}
	return false
}
