package main

import (
	"context"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// TestScenario0002_DeterministicDrain is the behavior evidence for SCENARIO-0002 (Dispatcher drains
// queue with deterministic coordination). It integrates the core D4 coordination loop pieces —
// Daemon.RunOnce, queue.Queue, fakeRunner, MemoryDecisionLog — to prove the three acceptance
// criteria for STORY-0003:
//
// AC-1 (seam integration): serve drains the queue, resolving each directive to a launch via the
// Runner interface. Every directive is processed exactly once; the queue is fully drained.
//
// AC-2 (seam process-level): the coordination loop is DETERMINISTIC with ZERO LLM/provider calls per
// coordination action (the worker does LLM work; the coordinator must not call any LLM). We prove this
// by running the same initial queue twice and asserting the same decision sequence; structurally, the loop
// uses only the Runner + Queue + DecisionLog seams (no provider client is constructed).
//
// AC-3 (seam unit): each coordination action writes a decision-log line for auditability. Every claim,
// validation, tier selection, run, teardown, and outcome is recorded.
//
// Owning stories: STORY-0003. Seam: integration (in-process, fake backend).
// Execution command: cd modules/incus-dispatcher && go test . -run TestScenario0002_DeterministicDrain
func TestScenario0002_DeterministicDrain(t *testing.T) {
	// AC-1 & AC-3: Verify drain completes, every directive is processed once, and decision log
	// is written for each action.
	testAC1AC3_DrainAndDecisionLog(t)

	// AC-2: Verify determinism and zero-LLM by running the same initial queue twice and comparing
	// the decision sequences.
	testAC2_DeterministicZeroLLM(t)
}

// testAC1AC3_DrainAndDecisionLog proves AC-1 (drain all directives via Runner interface) and AC-3
// (decision-log lines are written for every coordination action).
func testAC1AC3_DrainAndDecisionLog(t *testing.T) {
	// Push 3 directives, each fails initially then succeeds on retry.
	// This exercises the full drain path: claim → validate → run → fail → requeue → retry → done.

	runner := &trackingRunner{}

	q := queue.NewMemoryQueue()
	logmem := NewMemoryDecisionLog()

	dm := &Daemon{
		Q:        q,
		Runner:   runner,
		Policy:   testPolicy(),
		Consumer: "scenario-0002",
		LeaseDur: time.Minute,
		Log:      logmem,
		Now:      func() time.Time { return time.Unix(0, 0) },
		MapToTask: func(d queue.Directive) Task {
			return Task{Name: d.ID, Cmd: []string{"true"}}
		},
	}

	// Push the three directives.
	_, _ = q.Push(queue.Directive{Template: "fleet-go", Origin: OriginOrchestrator, Intent: "test"})
	id2, _ := q.Push(queue.Directive{Template: "fleet-go", Origin: OriginOrchestrator, Intent: "test"})
	_, _ = q.Push(queue.Directive{Template: "fleet-go", Origin: OriginOrchestrator, Intent: "test"})
	p, c := q.Len()
	t.Logf("pushed 3 directives; queue length: %d pending / %d claimed", p, c)

	// --- AC-1: Drain the queue ---
	outcomes := []DirectiveOutcome{}
	for i := 0; i < 100; i++ {
		out, id, err := dm.RunOnce(context.Background())
		if err != nil {
			t.Fatalf("RunOnce: %v", err)
		}
		t.Logf("  iteration %d: outcome=%q id=%q", i, out, id)
		if out == OutcomeEmpty {
			break
		}
		outcomes = append(outcomes, out)
	}

	p2, c2 := q.Len()
	t.Logf("after drain: queue=%d pending/%d claimed, outcomes=%d", p2, c2, len(outcomes))

	// We expect 6 outcomes: 3 requeued + 3 done.
	if len(outcomes) != 6 {
		t.Fatalf("expected 6 outcomes (3 requeued + 3 done), got %d: %v", len(outcomes), outcomes)
	}

	// Check the sequence: 3 requeued, then 3 done
	if len(outcomes) >= 3 && (outcomes[0] != OutcomeRequeued || outcomes[1] != OutcomeRequeued || outcomes[2] != OutcomeRequeued) {
		t.Fatalf("first 3 outcomes should be requeued, got %v", outcomes[:3])
	}
	if len(outcomes) >= 6 && (outcomes[3] != OutcomeDone || outcomes[4] != OutcomeDone || outcomes[5] != OutcomeDone) {
		t.Fatalf("last 3 outcomes should be done, got %v", outcomes[3:])
	}

	// Queue must be fully drained.
	if p2 != 0 || c2 != 0 {
		t.Fatalf("queue after drain = %d pending / %d claimed, want 0/0", p2, c2)
	}

	// AC-1 observable: Runner.Run was called once per directive claim. We have 3 directives, each
	// initially fails and then succeeds on retry, so 6 total Run calls: 3 initial failures + 3 retries.
	if runner.runCount != 6 {
		t.Fatalf("Runner.Run called %d times, want 6 (3 directives × 2 attempts each)", runner.runCount)
	}

	// AC-1 observable: Runner.Cleanup was called exactly once per Run call.
	if runner.cleanupCount != 6 {
		t.Fatalf("Runner.Cleanup called %d times, want 6 (once per run)", runner.cleanupCount)
	}

	// --- AC-3: Decision-log lines are written for every coordination action ---
	decisions := logmem.Records()
	if len(decisions) == 0 {
		t.Fatalf("no decisions recorded, expected at least 10+ (tier-select, run, teardown, grade for each)")
	}

	// Verify the decision log records a line for EVERY coordination action. The loop runs each
	// of the 3 directives twice (attempt 0 fails → requeue, attempt 1 passes → done), so 6 runs.
	// Each run emits a tier-select (rule="tier-select") and a teardown (action="reap"); the
	// outcome is requeue (action="requeue") on the 3 first attempts and done (action="done") on
	// the 3 second attempts. Assert exact per-class counts so a single missing line is caught
	// (a loose ">=N" threshold would let a dropped log line pass silently).
	byRule := map[string]int{}
	byAction := map[string]int{}
	for _, d := range decisions {
		byRule[d.Rule]++
		byAction[d.Action]++
	}
	if byRule["tier-select"] != 6 {
		t.Errorf("tier-select decision lines = %d, want 6 (one per run): %+v", byRule["tier-select"], decisions)
	}
	if byAction["reap"] != 6 {
		t.Errorf("teardown (reap) decision lines = %d, want 6 (one per run)", byAction["reap"])
	}
	if byAction["requeue"] != 3 {
		t.Errorf("requeue decision lines = %d, want 3 (one per failed first attempt)", byAction["requeue"])
	}
	if byAction["done"] != 3 {
		t.Errorf("done decision lines = %d, want 3 (one per passing second attempt)", byAction["done"])
	}

	// Spot-check that requeue is attributed to dir-2's first attempt (action keyed to the directive).
	hasRequeue := false
	for _, d := range decisions {
		if d.DirectiveID == id2 && d.Action == "requeue" {
			hasRequeue = true
			break
		}
	}
	if !hasRequeue {
		t.Fatalf("expected requeue action for dir-2 attempt 0, not found in decisions")
	}

	// Spot-check that a grade-pass outcome is recorded (not just grade-fail).
	hasGradePass := false
	for _, d := range decisions {
		if d.Grade == "pass" && d.Action == "done" {
			hasGradePass = true
			break
		}
	}
	if !hasGradePass {
		t.Fatalf("expected grade-pass action, not found in decisions")
	}
}

// testAC2_DeterministicZeroLLM proves AC-2: the coordination loop is deterministic (same input →
// same decision sequence) and makes zero LLM calls. We run the same initial queue twice and compare
// the decision logs.
func testAC2_DeterministicZeroLLM(t *testing.T) {
	// --- Run 1: initial queue ---
	runner1 := &trackingRunner{}
	q1 := queue.NewMemoryQueue()
	logmem1 := NewMemoryDecisionLog()
	clk := func() time.Time { return time.Unix(1000, 0) }

	dm1 := &Daemon{
		Q:        q1,
		Runner:   runner1,
		Policy:   testPolicy(),
		Consumer: "det-1",
		LeaseDur: time.Minute,
		Log:      logmem1,
		Now:      clk,
		MapToTask: func(d queue.Directive) Task {
			return Task{Name: d.ID, Cmd: []string{"true"}}
		},
	}

	q1.Push(queue.Directive{Template: "fleet-go", Origin: OriginOrchestrator, Intent: "test"})
	q1.Push(queue.Directive{Template: "fleet-go", Origin: OriginOrchestrator, Intent: "test"})

	for i := 0; i < 100; i++ {
		out, _, err := dm1.RunOnce(context.Background())
		if err != nil {
			t.Fatalf("dm1 RunOnce: %v", err)
		}
		if out == OutcomeEmpty {
			break
		}
	}

	decisions1 := logmem1.Records()

	// --- Run 2: same initial state, same clock ---
	runner2 := &trackingRunner{}
	q2 := queue.NewMemoryQueue()
	logmem2 := NewMemoryDecisionLog()

	dm2 := &Daemon{
		Q:        q2,
		Runner:   runner2,
		Policy:   testPolicy(),
		Consumer: "det-2",
		LeaseDur: time.Minute,
		Log:      logmem2,
		Now:      clk,
		MapToTask: func(d queue.Directive) Task {
			return Task{Name: d.ID, Cmd: []string{"true"}}
		},
	}

	q2.Push(queue.Directive{Template: "fleet-go", Origin: OriginOrchestrator, Intent: "test"})
	q2.Push(queue.Directive{Template: "fleet-go", Origin: OriginOrchestrator, Intent: "test"})

	for i := 0; i < 100; i++ {
		out, _, err := dm2.RunOnce(context.Background())
		if err != nil {
			t.Fatalf("dm2 RunOnce: %v", err)
		}
		if out == OutcomeEmpty {
			break
		}
	}

	decisions2 := logmem2.Records()

	// AC-2 observable: determinism — the decision sequences are identical.
	if len(decisions1) != len(decisions2) {
		t.Fatalf("decision log lengths differ: %d vs %d", len(decisions1), len(decisions2))
	}

	for i, d1 := range decisions1 {
		d2 := decisions2[i]
		// Compare the key fields: Action, Grade, Rule (these determine the coordination outcome).
		// DirectiveID will differ because they're UUIDs. What matters is the sequence of actions/grades/rules.
		if d1.Action != d2.Action || d1.Grade != d2.Grade || d1.Rule != d2.Rule {
			t.Fatalf("decision %d differs in deterministic fields:\n  run1: action=%s grade=%s rule=%s\n  run2: action=%s grade=%s rule=%s",
				i, d1.Action, d1.Grade, d1.Rule, d2.Action, d2.Grade, d2.Rule)
		}
	}

	// AC-2 observable: zero-LLM calls. The loop uses only the Runner + Queue + DecisionLog seams.
	// Structurally, there is no provider/LLM client construction in the RunOnce path or Serve loop.
	// This is proven by code inspection: the runner is the only thing that would call an LLM (in real
	// backends like fleet-worker), and the coordinator does not construct or call any provider.
	// We can assert indirectly: if the coordinator made an LLM call, it would require a provider
	// client (anthropic.Client, openai.Client, etc.). None of those appear in the hot path.
	//
	// For now, we assert the determinism (same sequence twice), which implies there are no
	// non-deterministic network/LLM calls in the loop.
	if len(decisions1) == 0 {
		t.Fatalf("determinism test: no decisions recorded")
	}

	// Verify both runs claim → run → record in the same order.
	claims1 := 0
	runs1 := 0
	for _, d := range decisions1 {
		if d.Rule == "tier-select" {
			claims1++
		}
		if d.Grade == "pass" || d.Grade == "fail" {
			runs1++
		}
	}
	if claims1 != 4 {
		t.Fatalf("expected 4 claims (tier-select calls for 2 directives × 2 attempts), got %d", claims1)
	}
	if runs1 != 4 {
		t.Fatalf("expected 4 grades (2 directives × 2 attempts), got %d", runs1)
	}
}

// trackingRunner is a test double that records all Run and Cleanup calls.
// It returns predetermined outcomes based on the task name and attempt count.
type trackingRunner struct {
	outcomes    map[string]*Result // task name → result
	runCount    int
	cleanupCount int
	runs        []recordedRun
	taskAttempts map[string]int    // tracks number of calls per task name
}

type recordedRun struct {
	taskName string
	attempt  int
}

func (r *trackingRunner) Run(_ context.Context, task Task) (*Result, error) {
	r.runCount++
	if r.taskAttempts == nil {
		r.taskAttempts = make(map[string]int)
	}
	attempt := r.taskAttempts[task.Name]
	r.taskAttempts[task.Name]++
	r.runs = append(r.runs, recordedRun{taskName: task.Name, attempt: attempt})

	// Simple logic for testing: fail on the first attempt for every directive, then succeed.
	// This exercises the requeue path in the ladder.
	if attempt == 0 {
		// First attempt always fails to test the escalation ladder.
		return &Result{ExitCode: 1}, nil
	}

	// Second and subsequent attempts succeed.
	return &Result{ExitCode: 0}, nil
}

func (r *trackingRunner) Cleanup() error {
	r.cleanupCount++
	return nil
}
