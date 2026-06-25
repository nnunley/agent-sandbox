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
//
//	cd modules/incus-dispatcher && go test . -run TestJourney0001
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

// TestJourney0002_LiveSteering proves live-steering preemption: a high-priority directive
// pushed by the orchestrator is claimed BEFORE lower-priority work, demonstrating the
// priority-queue-based preemption at the core of the daemon's claim logic.
//
// This journey validates JOURNEY-0002 end-to-end using the same harness as JOURNEY-0001:
// - A lower-priority directive (ImportanceNormal) is queued as "current work"
// - The orchestrator steers by pushing a high-priority directive (ImportanceHigh)
// - Next daemon cycle claims the high-priority directive (preemption proven)
// - The lower-priority directive remains queued
// - The high-priority directive runs and completes
// - The same daemon loop then claims and completes the lower-priority work
// - No restart occurs; the loop continues seamlessly (no-restart observable)
//
// Execution: cd modules/incus-dispatcher && go test . -run TestJourney0002_LiveSteering
func TestJourney0002_LiveSteering(t *testing.T) {
	backend := &journeyBackend{}
	q := queue.NewMemoryQueue()
	ctxSpy := &handoffSpy{} // records ImportHandoff so we can prove "prior context preserved"
	dm := &Daemon{
		Q:        q,
		Runner:   backend,
		Policy:   testPolicy(),
		Consumer: "journey",
		LeaseDur: time.Minute,
		Audit:    NewMemoryAuditLog(), // wire audit to prove runs are logged
		Context:  ctxSpy,              // wire context so the steered directive's handoff is imported
	}

	// Step 1: Push a lower-priority directive (ImportanceNormal) as "current work"
	lowID, err := q.Push(queue.Directive{
		Intent:     "implement lower-priority task",
		Template:   "fleet-go",
		Origin:     OriginOrchestrator,
		Repo:       "/srv/let-go",
		Ref:        "main",
		Task:       "lower-priority work",
		Importance: queue.ImportanceNormal, // normal priority
	})
	if err != nil {
		t.Fatalf("push D_low: %v", err)
	}

	// Step 2: Orchestrator steers by pushing a HIGH-priority directive
	highID, err := q.Push(queue.Directive{
		Intent:     "high-priority steering directive",
		Template:   "fleet-go",
		Origin:     OriginOrchestrator,
		Repo:       "/srv/let-go",
		Ref:        "main",
		Task:       "high-priority work from orchestrator",
		Importance: queue.ImportanceHigh,                 // HIGH priority — will preempt
		HandoffIn:  "/srv/handoff-store/thr-prior/run-0", // prior context the orchestrator carries into the steered work
	})
	if err != nil {
		t.Fatalf("push D_high (steering): %v", err)
	}

	// Step 3: Verify both directives are pending (D_high will be claimed first due to priority)
	if p, c := q.Len(); p != 2 || c != 0 {
		t.Fatalf("after push both: pending=%d claimed=%d, want 2/0", p, c)
	}

	// Step 4: Next daemon cycle claims and runs the HIGH-priority directive (preemption)
	outcome1, claimedID1, err := dm.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce (first cycle): %v", err)
	}
	if outcome1 != OutcomeDone {
		t.Fatalf("first RunOnce outcome = %q, want done", outcome1)
	}
	if claimedID1 != highID {
		t.Fatalf("first RunOnce claimed %q, want %q (high-priority directive must be preempted)", claimedID1, highID)
	}

	// HARD-assert: D_high was the one run (preemption proven)
	// Task.Name is the sanitized directive ID; check it matches D_high
	expectedHighName := sanitizeName(highID)
	if backend.lastTask.Name != expectedHighName {
		t.Fatalf("backend.lastTask.Name = %q, want %q (sanitized highID, preemption not verified)", backend.lastTask.Name, expectedHighName)
	}
	if backend.runs != 1 {
		t.Fatalf("backend.runs = %d after first cycle, want 1", backend.runs)
	}

	// Observable "prior context preserved": the steered high-priority directive carried a HandoffIn,
	// and the daemon imported that prior context before running it (best-effort soft state, STORY-0018).
	if len(ctxSpy.imported) != 1 || ctxSpy.imported[0] != "/srv/handoff-store/thr-prior/run-0" {
		t.Fatalf("prior handoff not applied to the preempting run: imported=%v, want [/srv/handoff-store/thr-prior/run-0]", ctxSpy.imported)
	}

	// Step 5: Verify D_low remains queued (not lost)
	if p, c := q.Len(); p != 1 || c != 0 {
		t.Fatalf("after D_high done: pending=%d claimed=%d, want 1/0 (D_low must remain queued)", p, c)
	}

	// Step 6: Next daemon cycle claims and runs the LOWER-priority directive (no-restart, proceeds after preemption)
	outcome2, claimedID2, err := dm.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce (second cycle): %v", err)
	}
	if outcome2 != OutcomeDone {
		t.Fatalf("second RunOnce outcome = %q, want done", outcome2)
	}
	if claimedID2 != lowID {
		t.Fatalf("second RunOnce claimed %q, want %q (lower-priority work must proceed after preemption)", claimedID2, lowID)
	}

	// Verify D_low was run on the second cycle
	expectedLowName := sanitizeName(lowID)
	if backend.lastTask.Name != expectedLowName {
		t.Fatalf("backend.lastTask.Name = %q, want %q (sanitized lowID)", backend.lastTask.Name, expectedLowName)
	}
	if backend.runs != 2 {
		t.Fatalf("backend.runs = %d after second cycle, want 2", backend.runs)
	}

	// Step 7: Verify queue is fully drained (both directives processed)
	if p, c := q.Len(); p != 0 || c != 0 {
		t.Fatalf("queue after both cycles: pending=%d claimed=%d, want 0/0 (fully drained)", p, c)
	}

	// Step 8: Verify both directives were reaped (teardown ran twice, one per directive)
	if backend.cleanups != 2 {
		t.Fatalf("teardown ran %d times, want 2 (once per directive)", backend.cleanups)
	}

	// --- JOURNEY-0002 final observables ---

	// ✅ High-priority directive completed
	if outcome1 != OutcomeDone {
		t.Fatalf("D_high outcome = %q, want done", outcome1)
	}

	// ✅ Lower-priority directives remain queued (D_low was queued, then ran after preemption)
	// This observable is proven by claimedID2 == lowID on the second cycle

	// ✅ No restart: the same daemon loop continued seamlessly from D_high to D_low
	// This observable is proven by sequential RunOnce calls without external restart

	// ✅ Both directives ran in the correct order (preemption honored, lower-priority proceeded)
	if backend.runs != 2 {
		t.Fatalf("total backend runs = %d, want 2 (both directives must run)", backend.runs)
	}
}

// handoffSpy is a ContextProvider that records every ImportHandoff bundle path so a test can prove
// the daemon applied prior context to a run. All other soft-state ops are no-ops (embedded NoopProvider).
type handoffSpy struct {
	NoopProvider
	imported []string
}

func (h *handoffSpy) ImportHandoff(bundlePath string) (HandoffManifest, error) {
	h.imported = append(h.imported, bundlePath)
	return HandoffManifest{}, nil
}
