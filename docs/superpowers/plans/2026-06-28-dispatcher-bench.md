# Dispatcher Bench Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `incus-dispatcher bench` to run versioned benchmark suites through the existing dispatcher runner + external grader path and emit ranked scorecards with JSON output and provider-cost accounting.

**Architecture:** Keep the new surface inside `modules/incus-dispatcher/`. Add a suite loader for `suites/<name>/<version>/`, a bench runner that reuses `Runner.Run` + `runGradeCommand` semantics through `ExternalGradingCheckout`, and a pure scorecard aggregator/formatter. The CLI subcommand wires these pieces together with an injectable runner so tests can cover a dry-run/stubbed path without live model dispatch.

**Tech Stack:** Go in `modules/incus-dispatcher/`, stdlib only plus existing internal packages. No new third-party deps.

---

## File Map

- Create: `modules/incus-dispatcher/bench_types.go`
- Create: `modules/incus-dispatcher/bench_suite.go`
- Create: `modules/incus-dispatcher/bench_runner.go`
- Create: `modules/incus-dispatcher/bench_scorecard.go`
- Create: `modules/incus-dispatcher/bench_cmd.go`
- Create: `modules/incus-dispatcher/suites/fleet-core/v1/manifest.json`
- Create: `modules/incus-dispatcher/suites/fleet-core/v1/tasks/*`
- Create: `modules/incus-dispatcher/bench_suite_test.go`
- Create: `modules/incus-dispatcher/bench_scorecard_test.go`
- Create: `modules/incus-dispatcher/bench_runner_test.go`
- Create: `modules/incus-dispatcher/bench_cmd_test.go`
- Modify: `modules/incus-dispatcher/main.go`

## Global Constraints

- Reuse the existing dispatcher execution path: construct `Task` values and call `Runner.Run`; do not invent a second dispatch stack.
- Reuse the existing grade semantics by driving the existing external grading path (`Task.ExternalGradingCheckout` / `runGradeCommand` contract), not a parallel oracle implementation.
- Wire `bench` in `main.go` with the same `if len(os.Args) > 1 && os.Args[1] == "bench"` pattern used by `grade` / `serve` / `tui` / `usage`.
- Every task follows strict TDD: write failing test, run to see the failure, implement the minimum code, rerun, then run `cd modules/incus-dispatcher && go test -race ./...` and `go vet ./...` before committing.
- Ignore the pre-existing `TestFirecrackerRunner_Integration_RealWorkerVM` failure during verification; no new code may depend on it.
- Work in the current workspace (`/Users/ndn/development/agent-sandbox`), not a separate worktree, because the user explicitly directed this location.

## Chunk 1: Suite Loading + v1 Fixture

### Task 1: Versioned suite loader + content hash

**Files:**
- Create: `modules/incus-dispatcher/bench_types.go`
- Create: `modules/incus-dispatcher/bench_suite.go`
- Create: `modules/incus-dispatcher/bench_suite_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLoadBenchSuite_ComputesStableHashAndTasks(t *testing.T) {
	root := t.TempDir()
	writeSuiteFixture(t, root, "fleet-core", "v1", []suiteFixtureTask{
		{name: "task-a", brief: "fix A", oracleRef: "/oracle/a", repo: "/repo/a", ref: "main"},
		{name: "task-b", brief: "fix B", oracleRef: "/oracle/b", repo: "/repo/b", ref: "feature"},
	})

	suite, err := LoadBenchSuite(root, "fleet-core@v1")
	if err != nil {
		t.Fatalf("LoadBenchSuite: %v", err)
	}
	if suite.Name != "fleet-core" || suite.Version != "v1" {
		t.Fatalf("suite id mismatch: %+v", suite)
	}
	if len(suite.Tasks) != 2 {
		t.Fatalf("tasks=%d want 2", len(suite.Tasks))
	}
	if suite.Hash == "" {
		t.Fatal("expected non-empty content hash")
	}
	suite2, err := LoadBenchSuite(root, "fleet-core@v1")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if suite.Hash != suite2.Hash {
		t.Fatalf("hash not stable: %q vs %q", suite.Hash, suite2.Hash)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./... -run TestLoadBenchSuite_ComputesStableHashAndTasks -v`
Expected: FAIL with `undefined: LoadBenchSuite` and missing bench types.

- [ ] **Step 3: Write minimal implementation**

```go
type BenchSuite struct {
	Name    string
	Version string
	Hash    string
	Tasks   []BenchTaskSpec
}

type BenchTaskSpec struct {
	Name      string
	Brief     string
	Repo      string
	Ref       string
	OracleRef string
}

func LoadBenchSuite(root, suiteID string) (*BenchSuite, error) {
	// Parse name@version, read manifest/tasks, build deterministic task order,
	// hash manifest + task payloads with sha256, return BenchSuite.
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test -race ./... -run TestLoadBenchSuite_ComputesStableHashAndTasks -v`
Expected: PASS.

- [ ] **Step 5: Run full module verification**

Run:
```bash
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher
go test -race ./...
go vet ./...
```
Expected: PASS except the known unrelated `TestFirecrackerRunner_Integration_RealWorkerVM` failure.

- [ ] **Step 6: Commit**

```bash
cd /Users/ndn/development/agent-sandbox
git add modules/incus-dispatcher/bench_types.go modules/incus-dispatcher/bench_suite.go modules/incus-dispatcher/bench_suite_test.go
git commit -m "dispatcher: add bench suite loader"
```

### Task 2: Ship the tiny `fleet-core@v1` curated suite

**Files:**
- Create: `modules/incus-dispatcher/suites/fleet-core/v1/manifest.json`
- Create: `modules/incus-dispatcher/suites/fleet-core/v1/tasks/task-*/brief.txt`
- Create: `modules/incus-dispatcher/suites/fleet-core/v1/tasks/task-*/meta.json`
- Modify: `modules/incus-dispatcher/bench_suite_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLoadBenchSuite_FleetCoreV1Fixture(t *testing.T) {
	suite, err := LoadBenchSuite(".", "fleet-core@v1")
	if err != nil {
		t.Fatalf("LoadBenchSuite: %v", err)
	}
	if len(suite.Tasks) != 3 {
		t.Fatalf("tasks=%d want 3", len(suite.Tasks))
	}
	for _, task := range suite.Tasks {
		if task.Brief == "" || task.OracleRef == "" {
			t.Fatalf("task missing hidden-oracle fixture: %+v", task)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./... -run TestLoadBenchSuite_FleetCoreV1Fixture -v`
Expected: FAIL because the suite fixture does not exist yet.

- [ ] **Step 3: Add the minimal suite files**

```json
{
  "name": "fleet-core",
  "version": "v1",
  "tasks": ["task-001", "task-002", "task-003"]
}
```

Each task directory contains:
- `brief.txt` with the visible prompt.
- `meta.json` with `{ "repo": "...", "ref": "...", "oracle_ref": "...", "notes": "hidden oracle path only" }`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test -race ./... -run TestLoadBenchSuite_FleetCoreV1Fixture -v`
Expected: PASS.

- [ ] **Step 5: Run full module verification**

Run:
```bash
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher
go test -race ./...
go vet ./...
```
Expected: PASS except the known unrelated `TestFirecrackerRunner_Integration_RealWorkerVM` failure.

- [ ] **Step 6: Commit**

```bash
cd /Users/ndn/development/agent-sandbox
git add modules/incus-dispatcher/suites/fleet-core/v1 modules/incus-dispatcher/bench_suite_test.go
git commit -m "dispatcher: add fleet-core v1 bench suite"
```

## Chunk 2: Pure Scorecard Aggregation

### Task 3: Aggregate per-task outcomes into ranked scorecards

**Files:**
- Create: `modules/incus-dispatcher/bench_scorecard.go`
- Create: `modules/incus-dispatcher/bench_scorecard_test.go`
- Modify: `modules/incus-dispatcher/bench_types.go`

- [ ] **Step 1: Write the failing test**

```go
func TestBuildScorecard_RanksByPassRateThenJudgeThenWallTime(t *testing.T) {
	suite := &BenchSuite{Name: "fleet-core", Version: "v1", Hash: "abc123"}
	results := []BenchTaskResult{
		{Candidate: BenchCandidate{Name: "alpha"}, TaskName: "t1", Passed: true, JudgeScore: 0.8, WallTimeMs: 1200, TokensByProvider: map[string]BenchTokenCost{"openai": {InputTokens: 10, OutputTokens: 20, SpendUSD: 0.12}}},
		{Candidate: BenchCandidate{Name: "alpha"}, TaskName: "t2", Passed: false, WallTimeMs: 1500},
		{Candidate: BenchCandidate{Name: "beta"}, TaskName: "t1", Passed: true, JudgeScore: 0.7, WallTimeMs: 1000},
		{Candidate: BenchCandidate{Name: "beta"}, TaskName: "t2", Passed: true, JudgeScore: 0.6, WallTimeMs: 1300},
	}

	card := BuildScorecard(suite, results)
	if got := card.Ranking[0].Candidate.Name; got != "beta" {
		t.Fatalf("top rank=%q want beta", got)
	}
	if card.Suite.Hash != "abc123" {
		t.Fatalf("suite hash=%q", card.Suite.Hash)
	}
	if len(card.Candidates) != 2 {
		t.Fatalf("candidates=%d want 2", len(card.Candidates))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./... -run TestBuildScorecard_RanksByPassRateThenJudgeThenWallTime -v`
Expected: FAIL with `undefined: BuildScorecard`.

- [ ] **Step 3: Write minimal implementation**

```go
type BenchScorecard struct {
	Suite      BenchSuiteRef          `json:"suite"`
	Candidates []BenchCandidateScore  `json:"candidates"`
	Ranking    []BenchRankingEntry    `json:"ranking"`
}

func BuildScorecard(suite *BenchSuite, results []BenchTaskResult) BenchScorecard {
	// Group by candidate, compute pass rate / judge average / wall time / provider cost,
	// sort deterministically, and emit both detailed candidates and ranking table rows.
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test -race ./... -run TestBuildScorecard_RanksByPassRateThenJudgeThenWallTime -v`
Expected: PASS.

- [ ] **Step 5: Run full module verification**

Run:
```bash
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher
go test -race ./...
go vet ./...
```
Expected: PASS except the known unrelated `TestFirecrackerRunner_Integration_RealWorkerVM` failure.

- [ ] **Step 6: Commit**

```bash
cd /Users/ndn/development/agent-sandbox
git add modules/incus-dispatcher/bench_types.go modules/incus-dispatcher/bench_scorecard.go modules/incus-dispatcher/bench_scorecard_test.go
git commit -m "dispatcher: add bench scorecard aggregation"
```

### Task 4: Table + JSON scorecard formatting

**Files:**
- Modify: `modules/incus-dispatcher/bench_scorecard.go`
- Modify: `modules/incus-dispatcher/bench_scorecard_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestBenchScorecard_RenderIncludesSuiteAndProviderCost(t *testing.T) {
	card := BenchScorecard{
		Suite: BenchSuiteRef{Name: "fleet-core", Version: "v1", Hash: "abc123"},
		Candidates: []BenchCandidateScore{
			{
				Candidate: BenchCandidate{Name: "beta", Provider: ProviderOpenAI, Model: "gpt-4o-mini"},
				PassRate: 1.0,
				WallTimeMs: 2300,
				TokensByProvider: map[string]BenchTokenCost{"openai": {InputTokens: 10, OutputTokens: 20, SpendUSD: 0.12}},
			},
		},
		Ranking: []BenchRankingEntry{{Rank: 1, Candidate: BenchCandidate{Name: "beta"}}},
	}

	table := card.RenderTable()
	if !strings.Contains(table, "fleet-core@v1") || !strings.Contains(table, "abc123") {
		t.Fatalf("table missing suite info:\n%s", table)
	}
	if !strings.Contains(table, "0.12") {
		t.Fatalf("table missing spend:\n%s", table)
	}
	if _, err := card.MarshalJSON(); err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./... -run TestBenchScorecard_RenderIncludesSuiteAndProviderCost -v`
Expected: FAIL because the render helpers do not exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
func (c BenchScorecard) RenderTable() string {
	// Render a deterministic plain-text ranked table with suite name/version/hash,
	// pass rate, judge score, wall ms, and per-provider token/spend summary.
}

func (c BenchScorecard) MarshalJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test -race ./... -run TestBenchScorecard_RenderIncludesSuiteAndProviderCost -v`
Expected: PASS.

- [ ] **Step 5: Run full module verification**

Run:
```bash
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher
go test -race ./...
go vet ./...
```
Expected: PASS except the known unrelated `TestFirecrackerRunner_Integration_RealWorkerVM` failure.

- [ ] **Step 6: Commit**

```bash
cd /Users/ndn/development/agent-sandbox
git add modules/incus-dispatcher/bench_scorecard.go modules/incus-dispatcher/bench_scorecard_test.go
git commit -m "dispatcher: render bench scorecards"
```

## Chunk 3: Bench Execution Loop

### Task 5: Stub-friendly runner loop that reuses `Runner.Run`

**Files:**
- Create: `modules/incus-dispatcher/bench_runner.go`
- Create: `modules/incus-dispatcher/bench_runner_test.go`
- Modify: `modules/incus-dispatcher/bench_types.go`

- [ ] **Step 1: Write the failing test**

```go
func TestBenchRunner_RunSuite_UsesRunnerAndCollectsOracleOutcome(t *testing.T) {
	fake := &fakeBenchRunner{
		results: map[string]*Result{
			"alpha/task-1": {
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./... -run TestBenchRunner_RunSuite_UsesRunnerAndCollectsOracleOutcome -v`
Expected: FAIL with `undefined: BenchRunner`.

- [ ] **Step 3: Write minimal implementation**

```go
type BenchRunner struct {
	Runner Runner
}

func (b BenchRunner) RunSuite(ctx context.Context, suite *BenchSuite, candidates []BenchCandidate) ([]BenchTaskResult, error) {
	// For each candidate x task, build a Task using the existing dispatcher path,
	// apply provider routing, call Runner.Run, and map Result/GradingResult to BenchTaskResult.
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test -race ./... -run TestBenchRunner_RunSuite_UsesRunnerAndCollectsOracleOutcome -v`
Expected: PASS.

- [ ] **Step 5: Run full module verification**

Run:
```bash
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher
go test -race ./...
go vet ./...
```
Expected: PASS except the known unrelated `TestFirecrackerRunner_Integration_RealWorkerVM` failure.

- [ ] **Step 6: Commit**

```bash
cd /Users/ndn/development/agent-sandbox
git add modules/incus-dispatcher/bench_types.go modules/incus-dispatcher/bench_runner.go modules/incus-dispatcher/bench_runner_test.go
git commit -m "dispatcher: add bench runner loop"
```

### Task 6: Dry-run / injected-runner path + failure accounting

**Files:**
- Modify: `modules/incus-dispatcher/bench_runner.go`
- Modify: `modules/incus-dispatcher/bench_runner_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestBenchRunner_RunSuite_RecordsFailuresAndSkipsWithoutAborting(t *testing.T) {
	fake := &fakeBenchRunner{
		errs: map[string]error{"beta/task-2": context.DeadlineExceeded},
		results: map[string]*Result{
			"alpha/task-1": {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: true}},
			"beta/task-1":  {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 1, PatchApplied: true}},
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
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./... -run TestBenchRunner_RunSuite_RecordsFailuresAndSkipsWithoutAborting -v`
Expected: FAIL because dry-run/failure accounting does not exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
type BenchRunner struct {
	Runner Runner
	DryRun bool
}

// Extend RunSuite so per-task infra errors become recorded task results with status/reason,
// never aborting the full suite unless the suite/candidate input is invalid.
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test -race ./... -run TestBenchRunner_RunSuite_RecordsFailuresAndSkipsWithoutAborting -v`
Expected: PASS.

- [ ] **Step 5: Run full module verification**

Run:
```bash
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher
go test -race ./...
go vet ./...
```
Expected: PASS except the known unrelated `TestFirecrackerRunner_Integration_RealWorkerVM` failure.

- [ ] **Step 6: Commit**

```bash
cd /Users/ndn/development/agent-sandbox
git add modules/incus-dispatcher/bench_runner.go modules/incus-dispatcher/bench_runner_test.go
git commit -m "dispatcher: record bench task failures without aborting suite"
```

## Chunk 4: CLI Wiring

### Task 7: `bench` subcommand wiring + JSON output

**Files:**
- Create: `modules/incus-dispatcher/bench_cmd.go`
- Create: `modules/incus-dispatcher/bench_cmd_test.go`
- Modify: `modules/incus-dispatcher/main.go`

- [ ] **Step 1: Write the failing test**

```go
func TestRunBenchCommand_DryRunWritesScorecardJSON(t *testing.T) {
	root := t.TempDir()
	writeSuiteFixture(t, root, "fleet-core", "v1", []suiteFixtureTask{
		{name: "task-1", brief: "fix", oracleRef: "/oracle", repo: "/repo", ref: "main"},
	})
	outPath := filepath.Join(root, "scorecard.json")
	fake := &fakeBenchRunner{
		results: map[string]*Result{
			"alpha/task-1": {ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: true}},
		},
	}

	code := runBenchCommandWithDeps([]string{
		"--suite", "fleet-core@v1",
		"--suite-root", root,
		"--candidate", "alpha=openai:gpt-4o-mini",
		"--out", outPath,
		"--dry-run",
	}, benchCommandDeps{newRunner: func(string) (Runner, error) { return fake, nil }})

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./... -run TestRunBenchCommand_DryRunWritesScorecardJSON -v`
Expected: FAIL with `undefined: runBenchCommandWithDeps`.

- [ ] **Step 3: Write minimal implementation**

```go
func runBenchCommand(args []string) int {
	return runBenchCommandWithDeps(args, defaultBenchCommandDeps())
}

func runBenchCommandWithDeps(args []string, deps benchCommandDeps) int {
	// Parse flags, load suite, build candidates, create runner, run suite, aggregate,
	// print table to stdout, write JSON to --out when requested.
}
```

Add to `main.go` before the flag-based main path:

```go
	if len(os.Args) > 1 && os.Args[1] == "bench" {
		os.Exit(runBenchCommand(os.Args[2:]))
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test -race ./... -run TestRunBenchCommand_DryRunWritesScorecardJSON -v`
Expected: PASS.

- [ ] **Step 5: Run full module verification**

Run:
```bash
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher
go test -race ./...
go vet ./...
```
Expected: PASS except the known unrelated `TestFirecrackerRunner_Integration_RealWorkerVM` failure.

- [ ] **Step 6: Commit**

```bash
cd /Users/ndn/development/agent-sandbox
git add modules/incus-dispatcher/bench_cmd.go modules/incus-dispatcher/bench_cmd_test.go modules/incus-dispatcher/main.go
git commit -m "dispatcher: wire bench subcommand"
```

## Chunk 5: Final Hardening

### Task 8: Final bench end-to-end dry-run coverage

**Files:**
- Modify: `modules/incus-dispatcher/bench_cmd_test.go`
- Modify: `modules/incus-dispatcher/bench_runner_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestRunBenchCommand_DryRunTableIncludesSuiteHashAndPerTaskResults(t *testing.T) {
	// Execute a 2-candidate, 2-task dry run via runBenchCommandWithDeps, capture stdout,
	// and assert suite name/version/hash, ranked candidates, and per-task status all appear.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd modules/incus-dispatcher && go test ./... -run TestRunBenchCommand_DryRunTableIncludesSuiteHashAndPerTaskResults -v`
Expected: FAIL until the final formatting/details are present.

- [ ] **Step 3: Write minimal implementation**

```go
// Tighten the formatter / command output only enough to satisfy the dry-run acceptance:
// suite id + hash, ranked summary, and per-task result lines.
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd modules/incus-dispatcher && go test -race ./... -run TestRunBenchCommand_DryRunTableIncludesSuiteHashAndPerTaskResults -v`
Expected: PASS.

- [ ] **Step 5: Run full module verification**

Run:
```bash
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher
go test -race ./...
go vet ./...
```
Expected: PASS except the known unrelated `TestFirecrackerRunner_Integration_RealWorkerVM` failure.

- [ ] **Step 6: Commit**

```bash
cd /Users/ndn/development/agent-sandbox
git add modules/incus-dispatcher/bench_cmd_test.go modules/incus-dispatcher/bench_runner_test.go modules/incus-dispatcher/bench_scorecard.go
git commit -m "dispatcher: finish bench dry-run coverage"
```

## Final Verification + Push

- [ ] Re-read the spec and this plan line-by-line; check for scope drift, missed constraints, and any duplicate grading/dispatch logic.
- [ ] Perform an adversarial self-review of all bench files for critical/important bugs and spec deviations; fix them with the same red/green discipline if any are found.
- [ ] Run fresh verification:

```bash
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher
go test -race ./...
go vet ./...
```

- [ ] Push only to origin:

```bash
cd /Users/ndn/development/agent-sandbox
git push origin main
```

## Done Criteria

- `incus-dispatcher bench --suite fleet-core@v1 ...` exists as a first-class subcommand.
- Suites load from `suites/<name>/<version>/` and record `name`, `version`, and content `hash`.
- The bench runner reuses the existing dispatcher `Runner` path and the existing external-grading semantics.
- Scorecards emit ranked table + JSON with suite metadata, per-task results, wall time, and per-provider token/spend totals from `Result`.
- A tiny `fleet-core@v1` suite with 3 hidden-oracle tasks ships in-tree.
- Dry-run/injected-runner tests cover loader, runner, scoring, scorecard rendering, and command wiring without requiring live models/GPU.
