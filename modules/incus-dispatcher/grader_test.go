package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// STORY-0068 AC-1: the external grader emits a structured grade JSON
// {passed, clusterA, check_generated, untagged_fails, e2e} built from the
// ordered oracle gates. These tests prove the report-building + gate
// orchestration in CI against a small synthetic in-repo fixture, independent
// of the external let-go toolchain (which is the AC-2 cluster e2e path).

func TestBuildGradeReport_FixedPasses(t *testing.T) {
	// The 13→0 "fixed" state: generate clean, no cluster-A failures, generated
	// files match, untagged suite green, e2e green.
	gates := []GateOutcome{
		{Name: GateGenerate, ExitCode: 0, Output: "ok\n"},
		{Name: GateClusterA, ExitCode: 0, Output: "ok  \tlet-go/pkg/ir\t0.42s\n"},
		{Name: GateCheckGenerated, ExitCode: 0, Output: ""},
		{Name: GateUntagged, ExitCode: 0, Output: "ok  \tlet-go/...\t1.0s\n"},
		{Name: GateE2E, ExitCode: 0, Output: "PASS\n"},
	}
	got := BuildGradeReport(gates)
	want := GradeReport{Passed: true, ClusterA: 0, CheckGenerated: true, UntaggedFails: 0, E2E: true}
	if got != want {
		t.Fatalf("fixed grade mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

func TestBuildGradeReport_RegressedFailsWithCount(t *testing.T) {
	// The pre-fix "13 failures" state, loaded from the synthetic fixture.
	failOut := readFixture(t, "grade/gogen_ir.fail13.txt")
	gates := []GateOutcome{
		{Name: GateGenerate, ExitCode: 0, Output: "ok\n"},
		{Name: GateClusterA, ExitCode: 1, Output: failOut},
		{Name: GateCheckGenerated, ExitCode: 0, Output: ""},
		{Name: GateUntagged, ExitCode: 0, Output: "ok\n"},
		{Name: GateE2E, ExitCode: 0, Output: "PASS\n"},
	}
	got := BuildGradeReport(gates)
	if got.Passed {
		t.Errorf("expected Passed=false when cluster-A has failures, got passed")
	}
	if got.ClusterA != 13 {
		t.Errorf("expected ClusterA=13 from fixture, got %d", got.ClusterA)
	}
}

func TestBuildGradeReport_GateFailuresGateThePass(t *testing.T) {
	cases := []struct {
		name  string
		mutga func(g []GateOutcome) []GateOutcome
	}{
		{"check_generated dirty", func(g []GateOutcome) []GateOutcome {
			g[2].ExitCode = 1
			return g
		}},
		{"untagged fails", func(g []GateOutcome) []GateOutcome {
			g[3].ExitCode = 1
			g[3].Output = "--- FAIL: TestX\n--- FAIL: TestY\n"
			return g
		}},
		{"e2e fails", func(g []GateOutcome) []GateOutcome {
			g[4].ExitCode = 2
			return g
		}},
		{"generate fails", func(g []GateOutcome) []GateOutcome {
			g[0].ExitCode = 1
			return g
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := []GateOutcome{
				{Name: GateGenerate, ExitCode: 0, Output: "ok\n"},
				{Name: GateClusterA, ExitCode: 0, Output: "ok\n"},
				{Name: GateCheckGenerated, ExitCode: 0, Output: ""},
				{Name: GateUntagged, ExitCode: 0, Output: "ok\n"},
				{Name: GateE2E, ExitCode: 0, Output: "PASS\n"},
			}
			got := BuildGradeReport(tc.mutga(base))
			if got.Passed {
				t.Errorf("%s: expected Passed=false, got passed (%+v)", tc.name, got)
			}
		})
	}
}

func TestBuildGradeReport_MissingGateIsNotPassed(t *testing.T) {
	// A report missing a required gate must not be reported as passed
	// (anti-reward-hack: silence is not success).
	got := BuildGradeReport([]GateOutcome{{Name: GateGenerate, ExitCode: 0}})
	if got.Passed {
		t.Errorf("expected Passed=false when gates are missing, got %+v", got)
	}
}

func TestGradeReport_JSONShape(t *testing.T) {
	r := GradeReport{Passed: true, ClusterA: 0, CheckGenerated: true, UntaggedFails: 0, E2E: true}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"passed", "clusterA", "check_generated", "untagged_fails", "e2e"} {
		if _, ok := m[k]; !ok {
			t.Errorf("grade JSON missing key %q (got %s)", k, b)
		}
	}
}

// TestRunGrade_SyntheticGates proves the executor orchestrates real gate
// commands in the checkout dir and is the authoritative source of truth
// (STORY-0072 AC-2): the report comes from the executor's own gate runs, not
// any worker self-report. Uses sh-only synthetic gates — no let-go toolchain.
func TestRunGrade_SyntheticGates(t *testing.T) {
	gates := []GateSpec{
		{Name: GateGenerate, Cmd: []string{"sh", "-c", "echo ok"}},
		{Name: GateClusterA, Cmd: []string{"sh", "-c", "echo 'ok  pkg/ir'"}},
		{Name: GateCheckGenerated, Cmd: []string{"sh", "-c", "exit 0"}},
		{Name: GateUntagged, Cmd: []string{"sh", "-c", "echo ok"}},
		{Name: GateE2E, Cmd: []string{"sh", "-c", "echo PASS"}},
	}
	report, outcomes, err := RunGrade(context.Background(), "", nil, gates)
	if err != nil {
		t.Fatalf("RunGrade: %v", err)
	}
	if len(outcomes) != len(gates) {
		t.Fatalf("expected %d gate outcomes, got %d", len(gates), len(outcomes))
	}
	if !report.Passed {
		t.Errorf("expected synthetic all-green to pass, got %+v", report)
	}
}

func TestRunGrade_SyntheticClusterAFailure(t *testing.T) {
	gates := []GateSpec{
		{Name: GateGenerate, Cmd: []string{"sh", "-c", "echo ok"}},
		{Name: GateClusterA, Cmd: []string{"sh", "-c", "printf -- '--- FAIL: a\\n--- FAIL: b\\n'; exit 1"}},
		{Name: GateCheckGenerated, Cmd: []string{"sh", "-c", "exit 0"}},
		{Name: GateUntagged, Cmd: []string{"sh", "-c", "echo ok"}},
		{Name: GateE2E, Cmd: []string{"sh", "-c", "echo PASS"}},
	}
	report, _, err := RunGrade(context.Background(), "", nil, gates)
	if err != nil {
		t.Fatalf("RunGrade: %v", err)
	}
	if report.Passed {
		t.Errorf("expected fail when cluster-A gate fails, got %+v", report)
	}
	if report.ClusterA != 2 {
		t.Errorf("expected ClusterA=2, got %d", report.ClusterA)
	}
}

// TestRunGrade_AppliesDiffToCheckout proves the clone + wholesale-apply plumbing
// in CI using a real temp git repo (no let-go toolchain): the worker diff is
// applied to the clone, and a gate observes the patched content.
func TestRunGrade_AppliesDiffToCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte("before\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "f.txt")
	run("commit", "-qm", "init")

	diff := []byte("diff --git a/f.txt b/f.txt\n" +
		"index 0000000..1111111 100644\n" +
		"--- a/f.txt\n+++ b/f.txt\n" +
		"@@ -1 +1 @@\n-before\n+after\n")

	gates := []GateSpec{
		{Name: GateGenerate, Cmd: []string{"sh", "-c", "true"}},
		{Name: GateClusterA, Cmd: []string{"sh", "-c", "grep -q after f.txt && echo ok || (echo '--- FAIL: TestApply'; exit 1)"}},
		{Name: GateCheckGenerated, Cmd: []string{"sh", "-c", "true"}},
		{Name: GateUntagged, Cmd: []string{"sh", "-c", "true"}},
		{Name: GateE2E, Cmd: []string{"sh", "-c", "true"}},
	}
	report, _, err := RunGrade(context.Background(), repo, diff, gates)
	if err != nil {
		t.Fatalf("RunGrade: %v", err)
	}
	if !report.Passed {
		t.Errorf("expected pass after diff applied (gate sees 'after'), got %+v", report)
	}
}

// TestGraderIgnoresWorkerSelfReport proves STORY-0072 AC-2 (anti-reward-hack):
// the grade verdict is computed solely from the grader's own gate runs. Here a
// worker writes a result.json claiming success while the oracle gate actually
// fails — the grader still returns Passed=false. The grader never reads the
// worker's result.json (RunGrade takes a checkout + diff + gates, nothing else).
func TestGraderIgnoresWorkerSelfReport(t *testing.T) {
	// A lying worker self-report sitting in a sibling dir — never consulted.
	lyingReport := t.TempDir()
	if err := os.WriteFile(filepath.Join(lyingReport, "result.json"),
		[]byte(`{"status":"PASS","passed":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	gates := []GateSpec{
		{Name: GateGenerate, Cmd: []string{"sh", "-c", "true"}},
		{Name: GateClusterA, Cmd: []string{"sh", "-c", "echo '--- FAIL: TestCluster_A/case_00'; exit 1"}},
		{Name: GateCheckGenerated, Cmd: []string{"sh", "-c", "true"}},
		{Name: GateUntagged, Cmd: []string{"sh", "-c", "true"}},
		{Name: GateE2E, Cmd: []string{"sh", "-c", "true"}},
	}
	report, _, err := RunGrade(context.Background(), "", nil, gates)
	if err != nil {
		t.Fatalf("RunGrade: %v", err)
	}
	if report.Passed {
		t.Errorf("grader trusted the worker's lying self-report; expected Passed=false, got %+v", report)
	}
	if report.ClusterA != 1 {
		t.Errorf("expected ClusterA=1 from the grader's own gate run, got %d", report.ClusterA)
	}
}

func readFixture(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", rel))
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	return string(b)
}
