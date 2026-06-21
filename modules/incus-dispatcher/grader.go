package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// External grader (STORY-0068). The grader runs the oracle gates on a clean
// checkout with the worker's diff applied, then reduces the gate outcomes to a
// structured GradeReport. It is the authoritative source of truth for pass/fail
// (STORY-0072 AC-2): the verdict comes from the grader's own gate runs, never
// from the worker's self-reported result.json.

// Gate names recognized by the grader. The ordered oracle for the let-go 13→0
// reproduction (JOURNEY-0003) is: make generate → go test -tags gogen_ir
// ./pkg/ir/ (cluster-A) → make check-generated → untagged go test ./... → e2e.
const (
	GateGenerate       = "generate"        // make generate (regenerate artifacts)
	GateClusterA       = "gogen_ir"        // go test -tags gogen_ir ./pkg/ir/
	GateCheckGenerated = "check_generated" // make check-generated
	GateUntagged       = "untagged"        // go test ./...
	GateE2E            = "e2e"             // end-to-end suite
)

// GradeReport is the structured grade JSON emitted by the external grader.
type GradeReport struct {
	Passed         bool `json:"passed"`
	ClusterA       int  `json:"clusterA"`
	CheckGenerated bool `json:"check_generated"`
	UntaggedFails  int  `json:"untagged_fails"`
	E2E            bool `json:"e2e"`
}

// GateOutcome is the result of running one oracle gate.
type GateOutcome struct {
	Name     string
	ExitCode int
	Output   string
}

// GateSpec describes a gate command to run during grading.
type GateSpec struct {
	Name string
	Cmd  []string
}

var failLineRE = regexp.MustCompile(`(?m)^\s*--- FAIL: (\S+)`)

// countLeafFailures counts failing leaf tests in go test output. A failed test
// name is a "leaf" if no other failed name is one of its subtests (i.e. no
// other failed name starts with name+"/"). This counts the 13 failing
// cluster-A subtest cases without double-counting their parent, and counts flat
// top-level failures in the untagged suite.
func countLeafFailures(output string) int {
	m := failLineRE.FindAllStringSubmatch(output, -1)
	names := make([]string, 0, len(m))
	for _, g := range m {
		names = append(names, g[1])
	}
	leaves := 0
	for _, n := range names {
		isParent := false
		for _, other := range names {
			if other != n && strings.HasPrefix(other, n+"/") {
				isParent = true
				break
			}
		}
		if !isParent {
			leaves++
		}
	}
	return leaves
}

// BuildGradeReport reduces ordered gate outcomes to a structured GradeReport.
// A run passes only when every required gate is present and green: generate
// succeeded, cluster-A has zero failing cases, generated files are clean, the
// untagged suite has zero failures, and e2e passed. A missing required gate is
// never treated as success (anti-reward-hack: silence is not a pass).
func BuildGradeReport(gates []GateOutcome) GradeReport {
	byName := make(map[string]GateOutcome, len(gates))
	for _, g := range gates {
		byName[g.Name] = g
	}

	r := GradeReport{}
	generate, hasGenerate := byName[GateGenerate]
	clusterA, hasClusterA := byName[GateClusterA]
	checkGen, hasCheckGen := byName[GateCheckGenerated]
	untagged, hasUntagged := byName[GateUntagged]
	e2e, hasE2E := byName[GateE2E]

	r.ClusterA = countLeafFailures(clusterA.Output)
	r.CheckGenerated = hasCheckGen && checkGen.ExitCode == 0
	r.UntaggedFails = countLeafFailures(untagged.Output)
	r.E2E = hasE2E && e2e.ExitCode == 0

	generateOK := hasGenerate && generate.ExitCode == 0
	clusterClean := hasClusterA && clusterA.ExitCode == 0 && r.ClusterA == 0
	untaggedClean := hasUntagged && untagged.ExitCode == 0 && r.UntaggedFails == 0

	r.Passed = generateOK && clusterClean && r.CheckGenerated && untaggedClean && r.E2E
	return r
}

// defaultGradeGates returns the canonical let-go oracle gates (JOURNEY-0003).
func defaultGradeGates() []GateSpec {
	return []GateSpec{
		{Name: GateGenerate, Cmd: []string{"make", "generate"}},
		{Name: GateClusterA, Cmd: []string{"go", "test", "-tags", "gogen_ir", "./pkg/ir/"}},
		{Name: GateCheckGenerated, Cmd: []string{"make", "check-generated"}},
		{Name: GateUntagged, Cmd: []string{"go", "test", "./..."}},
		{Name: GateE2E, Cmd: []string{"make", "e2e"}},
	}
}

// defaultGeneratedExcludes are paths the grader must NOT patch in from the
// worker diff: they are build artifacts (a binary core image + its checksum
// manifest) that `make generate` reproduces from the patched source. Patching
// them directly fails (binary patch without a full index) and would also defeat
// the check-generated gate, which exists to prove the worker's sources
// regenerate byte-identical artifacts. "Source files wholesale" (STORY-0068
// AC-1) means: apply the source hunks, regenerate the rest.
func defaultGeneratedExcludes() []string {
	return []string{
		"pkg/rt/generated.sums",
		"**/*.lgb",
	}
}

// RunGrade clones the clean checkout, applies the worker's diff wholesale, runs
// each gate in order, and returns the structured report plus the raw outcomes.
// When checkout is empty the gates run directly in a fresh temp dir (used by the
// CI synthetic-fixture tests, which need no source tree). The grader — never the
// worker — owns the verdict (STORY-0072 AC-2).
func RunGrade(ctx context.Context, checkout string, workerDiff []byte, gates []GateSpec, excludeGlobs ...string) (GradeReport, []GateOutcome, error) {
	tempDir, err := os.MkdirTemp("/tmp", "grade-")
	if err != nil {
		return GradeReport{}, nil, fmt.Errorf("create temp grading dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	workDir := tempDir
	if checkout != "" {
		workDir = filepath.Join(tempDir, "src")
		clone := newCmdContext(ctx, "git", "clone", checkout, workDir)
		if out, err := clone.CombinedOutput(); err != nil {
			return GradeReport{}, nil, fmt.Errorf("clone clean checkout: %s", out)
		}
		if len(workerDiff) > 0 {
			if err := applyWorkerDiff(ctx, workDir, workerDiff, excludeGlobs); err != nil {
				return GradeReport{}, nil, err
			}
		}
	}

	outcomes := make([]GateOutcome, 0, len(gates))
	for _, g := range gates {
		oc := runGate(ctx, workDir, g)
		outcomes = append(outcomes, oc)
	}
	return BuildGradeReport(outcomes), outcomes, nil
}

// applyWorkerDiff applies the worker's unified diff to the checkout, excluding
// any excludeGlobs (generated artifacts the make-generate gate reproduces). It
// checks first, then applies for real; a non-applying patch is a hard grading
// error. --3way lets the apply fall back to a merge when context drifted.
func applyWorkerDiff(ctx context.Context, dir string, diff []byte, excludeGlobs []string) error {
	patchFile := filepath.Join(dir, ".worker.patch")
	if err := os.WriteFile(patchFile, diff, 0644); err != nil {
		return fmt.Errorf("write patch file: %w", err)
	}
	defer os.Remove(patchFile)

	args := []string{"-C", dir, "apply"}
	for _, g := range excludeGlobs {
		args = append(args, "--exclude="+g)
	}
	if out, err := newCmdContext(ctx, "git", append(append([]string{}, args...), "--check", patchFile)...).CombinedOutput(); err != nil {
		return fmt.Errorf("worker diff does not apply cleanly (after excluding generated artifacts): %s", out)
	}
	if out, err := newCmdContext(ctx, "git", append(append([]string{}, args...), "--3way", patchFile)...).CombinedOutput(); err != nil {
		return fmt.Errorf("apply worker diff: %s", out)
	}
	return nil
}

// runGate executes one gate command in dir, capturing combined output and exit code.
func runGate(ctx context.Context, dir string, g GateSpec) GateOutcome {
	oc := GateOutcome{Name: g.Name}
	if len(g.Cmd) == 0 {
		oc.ExitCode = -1
		return oc
	}
	var buf bytes.Buffer
	cmd := newCmdContext(ctx, g.Cmd[0], g.Cmd[1:]...)
	cmd.Dir = dir
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	cmd.Env = append(os.Environ(), "PATH="+workerToolPath(os.Getenv("PATH")))
	err := cmd.Run()
	oc.Output = buf.String()
	if exitErr, ok := err.(*exec.ExitError); ok {
		oc.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		oc.ExitCode = -1
	}
	return oc
}
