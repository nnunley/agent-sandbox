package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// SCENARIO-0025 — D1: a worker directive proposing a privileged template is rejected BEFORE
// any container is launched (STORY-0049 AC-2/AC-3). The runner is never invoked and the
// directive leaves the queue.
func TestScenario0025_PrivilegedWorkerDirectiveRejectedNoLaunch(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 0}}
	dm, q := newDaemon(r)
	d := validDirective()
	d.ID = "w1-root"
	d.Template = "fleet-go-root" // privileged, orchestrator-only
	d.Origin = "worker:W1"
	q.Push(d)

	out, id, err := dm.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if out != OutcomeRejected || id != "w1-root" {
		t.Fatalf("outcome=%q id=%q, want rejected/w1-root", out, id)
	}
	if r.runs != 0 || r.cleanups != 0 {
		t.Fatalf("rejected directive caused launch (runs=%d cleanups=%d) — D1 security failure", r.runs, r.cleanups)
	}
	if p, c := q.Len(); p != 0 || c != 0 {
		t.Fatalf("queue after reject = %d/%d, want 0/0 (directive removed, not retried)", p, c)
	}
}

// SCENARIO-0074 — Template allowlist: a worker-origin privileged-template proposal is denied
// with the denial REASON and directive ID recorded in the audit (D6 decision) log
// (STORY-0053 AC-1).
func TestScenario0074_WorkerOriginPrivilegedDenialAudited(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 0}}
	q := queue.NewMemoryQueue()
	logmem := NewMemoryDecisionLog()
	dm := &Daemon{
		Q: q, Runner: r, Policy: testPolicy(), Consumer: "t", LeaseDur: time.Minute, Log: logmem,
		MapToTask: func(d queue.Directive) Task { return Task{Name: "n"} },
	}
	d := validDirective()
	d.ID = "proposal-42"
	d.Template = "fleet-go-root"
	d.Origin = "worker:rogue"
	q.Push(d)

	out, _, _ := dm.RunOnce(context.Background())
	if out != OutcomeRejected {
		t.Fatalf("outcome=%q, want rejected", out)
	}

	var rejected *Decision
	for i := range logmem.Records() {
		if logmem.Records()[i].Action == "rejected" {
			rec := logmem.Records()[i]
			rejected = &rec
		}
	}
	if rejected == nil {
		t.Fatalf("no rejected decision recorded; records=%+v", logmem.Records())
	}
	if rejected.DirectiveID != "proposal-42" {
		t.Fatalf("denial decision DirectiveID=%q, want proposal-42", rejected.DirectiveID)
	}
	if !strings.Contains(rejected.Reason, "worker-origin not allowed for privileged templates") {
		t.Fatalf("denial Reason=%q, want the worker-origin denial phrase", rejected.Reason)
	}
}
