package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
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

// D4 escalation ladder: a persistently-failing directive climbs retry-same →
// stronger-worker → hard-tier (autonomous requeues) → human (parked + escalated), with a D6
// decision-log entry per transition and thread status ending blocked. (Replaces the
// ITER-0000 park-after-max behavior, which the ladder supersedes.)
func TestRunOnce_LadderClimbsThenEscalates(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 1}} // always fails
	q := queue.NewMemoryQueue()
	logmem := NewMemoryDecisionLog()
	tracker := NewThreadTracker(func() time.Time { return time.Unix(0, 0) })
	lane := NewMemoryEscalationLane()
	dm := &Daemon{
		Q: q, Runner: r, Policy: testPolicy(), Consumer: "test", LeaseDur: time.Minute,
		Log: logmem, Threads: tracker, Escalations: lane, Now: func() time.Time { return time.Unix(0, 0) },
		MapToTask: func(d queue.Directive) Task { return Task{Name: d.ID, Cmd: []string{"true"}} },
	}
	id, _ := q.Push(validDirective())

	for i, want := range []DirectiveOutcome{OutcomeRequeued, OutcomeRequeued, OutcomeRequeued, OutcomeEscalated} {
		out, _, _ := dm.RunOnce(context.Background())
		if out != want {
			t.Fatalf("run %d: outcome %q, want %q", i, out, want)
		}
	}

	// Ladder rule sequence, read back from the D6 decision log.
	var rungs []string
	for _, d := range logmem.Records() {
		if d.Action == "requeue" || d.Action == "escalate-human" {
			rungs = append(rungs, d.Rule)
		}
	}
	if got, want := strings.Join(rungs, ","), "retry-same,stronger-worker,hard-tier,human"; got != want {
		t.Fatalf("ladder rungs = %q, want %q", got, want)
	}
	// Terminal: thread blocked, directive parked (durable hold), present in the lane.
	if tracker.Status(id) != StatusBlocked {
		t.Fatalf("final status = %q, want blocked", tracker.Status(id))
	}
	if q.Parked() != 1 || len(lane.List()) != 1 {
		t.Fatalf("want parked=1 lane=1, got parked=%d lane=%d", q.Parked(), len(lane.List()))
	}
}

// AC-6: an autonomous climb (rungs 0..2) never lands in the escalations lane; only the human
// rung does. Here a single failure (attempts=0 → retry-same) must NOT touch the lane.
func TestRunOnce_AutonomousRungDoesNotEscalate(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 1}}
	q := queue.NewMemoryQueue()
	lane := NewMemoryEscalationLane()
	dm := &Daemon{Q: q, Runner: r, Policy: testPolicy(), Consumer: "t", LeaseDur: time.Minute, Escalations: lane,
		MapToTask: func(d queue.Directive) Task { return Task{Name: d.ID, Cmd: []string{"true"}} }}
	q.Push(validDirective())
	if out, _, _ := dm.RunOnce(context.Background()); out != OutcomeRequeued {
		t.Fatalf("first fail → %q, want requeued (autonomous)", out)
	}
	if len(lane.List()) != 0 {
		t.Fatalf("autonomous rung leaked into the escalations lane: %+v", lane.List())
	}
}

// D6: a passing run writes a reap decision (teardown, AC-28) then a done decision, in order.
func TestRunOnce_PassWritesReapThenDone(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 0}}
	q := queue.NewMemoryQueue()
	logmem := NewMemoryDecisionLog()
	dm := &Daemon{Q: q, Runner: r, Policy: testPolicy(), Consumer: "t", LeaseDur: time.Minute, Log: logmem,
		MapToTask: func(d queue.Directive) Task { return Task{Name: d.ID, Cmd: []string{"true"}} }}
	q.Push(validDirective())
	dm.RunOnce(context.Background())
	var actions []string
	for _, d := range logmem.Records() {
		actions = append(actions, d.Action)
	}
	if got := strings.Join(actions, ","); got != "reap,done" {
		t.Fatalf("decision actions = %q, want \"reap,done\"", got)
	}
}

// Review fix (PAR-A): at the human rung WITHOUT an escalations lane, the directive must not
// be lost — it is durably parked and the escalate-human transition is decision-logged.
func TestRunOnce_HumanRungParksWithoutLane(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 1}}
	q := queue.NewMemoryQueue()
	logmem := NewMemoryDecisionLog()
	dm := &Daemon{Q: q, Runner: r, Policy: testPolicy(), Consumer: "t", LeaseDur: time.Minute, Log: logmem,
		MapToTask: func(d queue.Directive) Task { return Task{Name: d.ID, Cmd: []string{"true"}} }}
	d := validDirective()
	d.Attempts = 3 // terminal rung → human
	q.Push(d)
	if out, _, _ := dm.RunOnce(context.Background()); out != OutcomeEscalated {
		t.Fatalf("human rung → %q, want escalated", out)
	}
	if q.Parked() != 1 {
		t.Fatalf("Parked() = %d, want 1 (recoverable hold even without a lane)", q.Parked())
	}
	var escalated bool
	for _, dec := range logmem.Records() {
		if dec.Action == "escalate-human" && dec.Rule == "human" {
			escalated = true
		}
	}
	if !escalated {
		t.Fatalf("escalate-human decision not logged: %+v", logmem.Records())
	}
}

// Review fix (PAR-B): a passing run records the full status chain active → done.
func TestRunOnce_PassStatusChain(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 0}}
	q := queue.NewMemoryQueue()
	tracker := NewThreadTracker(func() time.Time { return time.Unix(0, 0) })
	dm := &Daemon{Q: q, Runner: r, Policy: testPolicy(), Consumer: "t", LeaseDur: time.Minute, Threads: tracker,
		MapToTask: func(d queue.Directive) Task { return Task{Name: d.ID, Cmd: []string{"true"}} }}
	id, _ := q.Push(validDirective())
	dm.RunOnce(context.Background())
	if tracker.Status(id) != StatusDone {
		t.Fatalf("status = %q, want done", tracker.Status(id))
	}
	var chain []ThreadStatus
	for _, tr := range tracker.Transitions(id) {
		chain = append(chain, tr.To)
	}
	if len(chain) != 2 || chain[0] != StatusActive || chain[1] != StatusDone {
		t.Fatalf("status chain = %v, want [active done]", chain)
	}
}

// Review fix (PAR-B): ThreadTracker and MemoryDecisionLog must be concurrent-safe (run with
// -race). Without the mutexes this trips the race detector / a concurrent-map-write panic.
func TestConcurrentTrackerAndLog(t *testing.T) {
	tracker := NewThreadTracker(func() time.Time { return time.Unix(0, 0) })
	logmem := NewMemoryDecisionLog()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "t" + strconv.Itoa(n%5)
			tracker.Set(id, StatusActive)
			_ = tracker.Status(id)
			_ = tracker.Transitions(id)
			_ = logmem.Append(Decision{DirectiveID: id, Action: "x"})
			_ = logmem.Records()
		}(i)
	}
	wg.Wait()
	if len(logmem.Records()) != 50 {
		t.Fatalf("concurrent appends recorded %d, want 50", len(logmem.Records()))
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

