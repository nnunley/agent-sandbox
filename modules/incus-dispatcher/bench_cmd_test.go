package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunBenchCommand_DryRunWritesScorecardJSON(t *testing.T) {
	root := t.TempDir()
	writeSuiteFixture(t, root, "fleet-core", "v1", []suiteFixtureTask{
		{name: "task-1", brief: "fix", oracleRef: "/oracle", repo: "/repo", ref: "main"},
	})
	outPath := filepath.Join(root, "scorecard.json")
	fake := &fakeBenchRunner{
		results: map[string]*Result{
			"gpt-4o-mini/task-1": {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: true}},
		},
	}
	var stdout bytes.Buffer

	code := runBenchCommandWithDeps([]string{
		"--suite", "fleet-core@v1",
		"--suite-root", root,
		"--candidate", "alpha=openai:gpt-4o-mini",
		"--out", outPath,
		"--dry-run",
	}, benchCommandDeps{
		stdout: &stdout,
		stderr: &stdout,
		newRunner: func(runnerKind, remote string) (Runner, error) {
			return fake, nil
		},
	})

	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	if !bytes.Contains(data, []byte(`"suite"`)) || !bytes.Contains(data, []byte(`"ranking"`)) {
		t.Fatalf("bad json: %s", data)
	}
}

func TestRunBenchCommand_DryRunTableIncludesSuiteHashAndPerTaskResults(t *testing.T) {
	root := t.TempDir()
	writeSuiteFixture(t, root, "fleet-core", "v1", []suiteFixtureTask{
		{name: "task-1", brief: "fix", oracleRef: "/oracle/a", repo: "/repo/a", ref: "main"},
		{name: "task-2", brief: "fix", oracleRef: "/oracle/b", repo: "/repo/b", ref: "main"},
	})
	fake := &fakeBenchRunner{
		results: map[string]*Result{
			"gpt-4o-mini/task-1": {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: true}},
			"gpt-4o-mini/task-2": {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 1, PatchApplied: true}},
			"qwen3.6/task-1":     {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: true}},
			"qwen3.6/task-2":     {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: true}},
		},
	}
	var stdout bytes.Buffer

	code := runBenchCommandWithDeps([]string{
		"--suite", "fleet-core@v1",
		"--suite-root", root,
		"--candidate", "alpha=openai:gpt-4o-mini",
		"--candidate", "beta=ollama-local:qwen3.6",
		"--dry-run",
	}, benchCommandDeps{
		stdout: &stdout,
		stderr: &stdout,
		newRunner: func(runnerKind, remote string) (Runner, error) {
			return fake, nil
		},
	})

	if code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	out := stdout.String()
	for _, want := range []string{"fleet-core@v1", "hash=", "alpha", "beta", "task-1", "task-2", "failed", "passed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}
