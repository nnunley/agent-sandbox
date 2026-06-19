package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// JOURNEY-0001 — Complete one-shot lifecycle: directive to completion.
//
// This is the automated harness behind behavior-scenarios.md JOURNEY-0001. It
// drives the REAL Daemon + DefaultMapToTask against a fake execution backend
// (permitted by the scenario card for CI) and asserts the journey's contracted
// final observables AND that the lifecycle phases occurred in order.
//
// The fake backend stands in for the proven container/NixOS path that EXIT (b)
// validated on the cluster on 2026-06-18; here we prove the in-process wiring
// (claim → validate → launch → deliver → run → harvest → grade → outcome →
// stop+reap) without needing a live remote, so the journey runs deterministically
// in CI. Execution command (run inside the module dir — it is a nested go.mod):
//   cd modules/incus-dispatcher && go test . -run TestJourney0001
//
// Coverage note: this harness asserts the observables that live at the daemon
// seam — directive→done, queue drained, instance reaped, authoritative grade,
// and worker.diff/result.json harvested. Two JOURNEY-0001 observables are NOT
// asserted here because they belong to deferred subsystems: the decision-log
// audit trail (STORY-0063 AC-28 → ITER-0001) and shared-volume cleanliness
// (real-backend property → ITER-0005). The scenario card flags both as deferred.
type journeyBackend struct {
	phases     []string // ordered record of lifecycle phases the backend performed
	runs       int
	cleanups   int
	lastTask   Task
	lastResult *Result // the harvested Result returned to the daemon
}

func (b *journeyBackend) Run(_ context.Context, task Task) (*Result, error) {
	b.runs++
	b.lastTask = task

	// Step 3/5: a fresh instance is launched and the repository delivered.
	if task.Name == "" {
		return nil, errCannotLaunch
	}
	b.phases = append(b.phases, "launch")
	if task.Repo != "" {
		b.phases = append(b.phases, "deliver")
	}

	// Step 7: the template runner executes the agent and produces output.
	b.phases = append(b.phases, "run")

	// Step 8: harvest worker.diff + result.json artifacts.
	b.phases = append(b.phases, "harvest")
	res := &Result{
		ExitCode:      0,
		ContainerName: ContainerNamePrefix + task.Name,
		PatchData:     []byte("diff --git a/x b/x\n@@ worker change @@\n"),
		Artifacts:     map[string][]byte{"result.json": []byte(`{"status":"ok"}`)},
	}

	// Step 9: authoritative external grade on a clean checkout (when requested).
	if task.ExternalGradingCheckout != "" {
		b.phases = append(b.phases, "grade")
		res.ExternalGradingResult = &GradingResult{
			ExitCode:     0,
			PatchApplied: true,
		}
	}
	b.lastResult = res
	return res, nil
}

// Cleanup is the stop-then-delete teardown (step 11): the worker instance is
// reaped after the run, never before.
func (b *journeyBackend) Cleanup() error {
	b.cleanups++
	b.phases = append(b.phases, "teardown")
	return nil
}

var errCannotLaunch = &journeyErr{"backend: empty task name, cannot launch container"}

type journeyErr struct{ msg string }

func (e *journeyErr) Error() string { return e.msg }

func keysOf(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// drain runs the daemon loop until the queue is empty, returning the terminal
// outcome of the last non-empty directive processed.
func drain(t *testing.T, dm *Daemon) DirectiveOutcome {
	t.Helper()
	var last DirectiveOutcome
	for i := 0; i < 100; i++ {
		out, _, err := dm.RunOnce(context.Background())
		if err != nil {
			t.Fatalf("RunOnce: %v", err)
		}
		if out == OutcomeEmpty {
			return last
		}
		last = out
	}
	t.Fatalf("daemon did not drain queue in 100 iterations")
	return ""
}

func TestJourney0001_OneShotLifecycle(t *testing.T) {
	backend := &journeyBackend{}
	q := queue.NewMemoryQueue()
	dm := &Daemon{
		Q:        q,
		Runner:   backend,
		Policy:   testPolicy(),
		Consumer: "journey",
		LeaseDur: time.Minute,
		// Real mapping under test — NOT the trivial test double.
	}

	// A full, well-formed directive: trusted origin, allowlisted template, repo to
	// deliver, a brief to run, and an oracle ref that routes through authoritative
	// external grading.
	if _, err := q.Push(queue.Directive{
		Intent:   "implement queue.Peek()",
		Template: "fleet-go",
		Origin:   OriginOrchestrator,
		Repo:     "/srv/let-go",
		Ref:      "main",
		Task:     "implement Peek and make the hidden oracle pass",
		Grade:    &queue.GradeSpec{OracleRef: "oracle/peek_test.go", Cmd: "go test ./queue/"},
	}); err != nil {
		t.Fatalf("push directive: %v", err)
	}

	outcome := drain(t, dm)

	// --- JOURNEY-0001 final observables ---

	// Directive state is done (grade passed → done, not escalated).
	if outcome != OutcomeDone {
		t.Fatalf("final outcome = %q, want done", outcome)
	}

	// Directive state: queue fully drained (0 pending, 0 in-flight).
	if p, c := q.Len(); p != 0 || c != 0 {
		t.Fatalf("queue after journey = %d pending / %d claimed, want 0/0", p, c)
	}

	// Worker instance no longer exists: teardown/reap ran exactly once.
	if backend.cleanups != 1 {
		t.Fatalf("teardown ran %d times, want exactly 1 (instance must be reaped)", backend.cleanups)
	}
	if backend.runs != 1 {
		t.Fatalf("backend ran %d times, want 1", backend.runs)
	}

	// The real mapping routed the brief through the grading path (step 9 reached).
	if backend.lastTask.ExternalGradingCheckout == "" {
		t.Fatalf("DefaultMapToTask did not propagate the oracle ref → external grade was skipped")
	}
	if backend.lastTask.Repo == "" {
		t.Fatalf("DefaultMapToTask did not propagate the repo → nothing to deliver")
	}

	// Result artifacts are persisted: worker.diff (PatchData) and result.json
	// (Artifacts) are harvested and returned to the daemon (step 8).
	if r := backend.lastResult; r == nil {
		t.Fatal("no Result harvested from the run")
	} else {
		if len(r.PatchData) == 0 {
			t.Fatal("worker.diff (Result.PatchData) was not harvested")
		}
		if _, ok := r.Artifacts["result.json"]; !ok {
			t.Fatalf("result.json not in harvested artifacts: %v", keysOf(r.Artifacts))
		}
		// Authoritative grade is present and passing (step 9 → step 10 done).
		if r.ExternalGradingResult == nil || !r.ExternalGradingResult.PatchApplied || r.ExternalGradingResult.ExitCode != 0 {
			t.Fatalf("authoritative grade missing or not passing: %+v", r.ExternalGradingResult)
		}
	}

	// Lifecycle phases occurred in the journey's contracted order, and teardown is
	// strictly last (stop-then-delete after the run, never before).
	want := []string{"launch", "deliver", "run", "harvest", "grade", "teardown"}
	if strings.Join(backend.phases, ",") != strings.Join(want, ",") {
		t.Fatalf("lifecycle phases = %v, want %v", backend.phases, want)
	}
}

// TestJourney0001_RejectedDirectiveNeverLaunches proves the D1 authority split is
// enforced inside the full journey: a worker-authored directive proposing a
// privileged template is rejected BEFORE any instance is launched (step 2 gates
// step 3) — the security-critical observable of the lifecycle.
func TestJourney0001_RejectedDirectiveNeverLaunches(t *testing.T) {
	backend := &journeyBackend{}
	q := queue.NewMemoryQueue()
	dm := &Daemon{Q: q, Runner: backend, Policy: testPolicy(), Consumer: "journey", LeaseDur: time.Minute}

	if _, err := q.Push(queue.Directive{
		Intent:   "escalate",
		Template: "fleet-go-root", // privileged
		Origin:   "worker:evil",   // worker proposing it
	}); err != nil {
		t.Fatalf("push: %v", err)
	}

	outcome := drain(t, dm)
	if outcome != OutcomeRejected {
		t.Fatalf("worker+privileged template → %q, want rejected", outcome)
	}
	if backend.runs != 0 || len(backend.phases) != 0 {
		t.Fatalf("rejected directive launched the backend (runs=%d phases=%v) — security failure", backend.runs, backend.phases)
	}
	if backend.cleanups != 0 {
		t.Fatalf("rejected directive triggered teardown (cleanups=%d) — nothing was launched to reap", backend.cleanups)
	}
	if p, c := q.Len(); p != 0 || c != 0 {
		t.Fatalf("queue after reject = %d/%d, want 0/0", p, c)
	}
}
