package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// fakeRunner is a test double for the execution backend.
type fakeRunner struct {
	result   *Result
	err      error
	runs     int
	cleanups int
	lastTask Task
}

func (f *fakeRunner) Run(_ context.Context, task Task) (*Result, error) {
	f.runs++
	f.lastTask = task
	return f.result, f.err
}
func (f *fakeRunner) Cleanup() error { f.cleanups++; return nil }

func newDaemon(r Runner) (*Daemon, *queue.MemoryQueue) {
	q := queue.NewMemoryQueue()
	return &Daemon{
		Q:        q,
		Runner:   r,
		Policy:   testPolicy(),
		Consumer: "test",
		LeaseDur: time.Minute,
		MapToTask: func(d queue.Directive) Task {
			return Task{Name: d.ID, Cmd: []string{"true"}}
		},
	}, q
}

func validDirective() queue.Directive {
	return queue.Directive{Template: "fleet-go", Origin: OriginOrchestrator, Intent: "x"}
}

func TestRunOnce_EmptyQueue(t *testing.T) {
	r := &fakeRunner{}
	dm, _ := newDaemon(r)
	out, id, err := dm.RunOnce(context.Background())
	if err != nil || out != OutcomeEmpty || id != "" {
		t.Fatalf("empty queue → (%q,%q,%v), want (empty,'',nil)", out, id, err)
	}
	if r.runs != 0 {
		t.Fatalf("runner called %d times on empty queue, want 0", r.runs)
	}
}

func TestRunOnce_Pass(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 0}}
	dm, q := newDaemon(r)
	q.Push(validDirective())
	out, _, err := dm.RunOnce(context.Background())
	if err != nil || out != OutcomeDone {
		t.Fatalf("pass → (%q,%v), want done", out, err)
	}
	if p, c := q.Len(); p != 0 || c != 0 {
		t.Fatalf("queue after done = %d/%d, want 0/0", p, c)
	}
	if r.runs != 1 || r.cleanups != 1 {
		t.Fatalf("runs=%d cleanups=%d, want 1/1", r.runs, r.cleanups)
	}
}

func TestRunOnce_FailRequeues(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 1}}
	dm, q := newDaemon(r)
	q.Push(validDirective()) // default max attempts = 3
	out, _, _ := dm.RunOnce(context.Background())
	if out != OutcomeRequeued {
		t.Fatalf("fail → %q, want requeued", out)
	}
	if p, _ := q.Len(); p != 1 {
		t.Fatalf("pending after requeue = %d, want 1", p)
	}
}

func TestRunOnce_ParkAfterMaxAttempts(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 1}}
	dm, q := newDaemon(r)
	d := validDirective()
	d.MaxAttempts = 1
	q.Push(d)
	out, _, _ := dm.RunOnce(context.Background())
	if out != OutcomeParked {
		t.Fatalf("fail at max attempts → %q, want parked", out)
	}
	if p, c := q.Len(); p != 0 || c != 0 {
		t.Fatalf("queue after park = %d/%d, want 0/0", p, c)
	}
}

func TestRunOnce_RejectedTemplateNotRun(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 0}}
	dm, q := newDaemon(r)
	d := validDirective()
	d.Template = "fleet-go-root"
	d.Origin = "worker:evil" // worker proposing a privileged template
	q.Push(d)
	out, _, _ := dm.RunOnce(context.Background())
	if out != OutcomeRejected {
		t.Fatalf("worker+privileged → %q, want rejected", out)
	}
	if r.runs != 0 {
		t.Fatalf("rejected directive was RUN (%d times) — security failure!", r.runs)
	}
	if p, c := q.Len(); p != 0 || c != 0 {
		t.Fatalf("queue after reject = %d/%d, want 0/0", p, c)
	}
}

func TestRunOnce_GradeFailIsFail(t *testing.T) {
	// Command exits 0 but the authoritative external grade fails → treated as fail.
	r := &fakeRunner{result: &Result{ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 1, PatchApplied: true}}}
	dm, q := newDaemon(r)
	q.Push(validDirective())
	out, _, _ := dm.RunOnce(context.Background())
	if out != OutcomeRequeued {
		t.Fatalf("grade-fail → %q, want requeued (grade is authoritative)", out)
	}
}

func TestRunOnce_GradePassIsDone(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: true}}}
	dm, q := newDaemon(r)
	q.Push(validDirective())
	out, _, _ := dm.RunOnce(context.Background())
	if out != OutcomeDone {
		t.Fatalf("grade-pass → %q, want done", out)
	}
}

func TestRunOnce_GradePatchNotAppliedIsFail(t *testing.T) {
	// Oracle exited 0 but the worker's patch FAILED to apply → not a real pass.
	// Pins the `PatchApplied &&` half of passed() (daemon.go): a `&&`→`||` mutation
	// would wrongly call this done.
	r := &fakeRunner{result: &Result{ExitCode: 0, ExternalGradingResult: &GradingResult{ExitCode: 0, PatchApplied: false}}}
	dm, q := newDaemon(r)
	q.Push(validDirective())
	out, _, _ := dm.RunOnce(context.Background())
	if out != OutcomeRequeued {
		t.Fatalf("grade-exit-0-but-patch-not-applied → %q, want requeued", out)
	}
}

func TestRunOnce_FrameworkErrorIsFail(t *testing.T) {
	// A framework/infra error (NOT an "exec command" error) with a zero exit code
	// must fail — the run never legitimately completed. Pins the
	// `runErr != nil && !isCommandErr(runErr)` clause in passed() (daemon.go:96):
	// deleting it would let a zero-exit result slip through as done.
	r := &fakeRunner{result: &Result{ExitCode: 0}, err: errors.New("incus: instance launch failed")}
	dm, q := newDaemon(r)
	q.Push(validDirective())
	out, _, _ := dm.RunOnce(context.Background())
	if out != OutcomeRequeued {
		t.Fatalf("framework error → %q, want requeued (run did not complete)", out)
	}
}
