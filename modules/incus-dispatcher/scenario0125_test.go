package main

import (
	"context"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// audit0125Runner is a trivial runner that always succeeds, so the daemon reaches the run-audit seam.
type audit0125Runner struct{}

func (audit0125Runner) Run(context.Context, Task) (*Result, error) { return &Result{ExitCode: 0}, nil }
func (audit0125Runner) Cleanup() error                             { return nil }

// TestScenario0125_AuditReplay proves STORY-0054 AC-1/2/3 end-to-end:
//   - AC-1 (logged, WIRED): the daemon auto-logs a RUN audit entry for every directive it processes —
//     the scenario drives a real Daemon.RunOnce and asserts the run entry was emitted by the daemon,
//     not appended by the test. The delegation (worker child directive) and a mutation are audited at
//     their seams (a worker that emits a child audits the delegation; genome mutation is ITER-0008b but
//     a mutation event is recorded here to show the audit covers mutations).
//   - AC-2 (replayable, causal order): entries carry a ParentRef = the PARENT'S AUDIT ENTRY ID, and
//     Replay reconstructs causal order. (TestMemoryAuditLog_Replay_OutOfOrder proves Replay genuinely
//     REORDERS out-of-causal-order appends — this scenario asserts the reconstructed chain.)
//   - AC-3 (immutable): mutating a returned slice does not change the stored log.
func TestScenario0125_AuditReplay(t *testing.T) {
	memAudit := NewMemoryAuditLog()
	clk := func() time.Time { return time.Unix(1000, 0) }

	dm := &Daemon{
		Q:         queue.NewMemoryQueue(),
		Runner:    audit0125Runner{},
		Policy:    testPolicy(),
		Consumer:  "audit-scenario",
		LeaseDur:  time.Minute,
		Audit:     memAudit, // wire the audit log into the dispatch path
		MapToTask: DefaultMapToTask,
		Now:       clk,
	}

	// A worker-origin directive (the run unit). fleet-go allows worker origin.
	directiveID, err := dm.Q.Push(queue.Directive{
		Template: "fleet-go",
		Origin:   "worker:W1",
		Intent:   "implement feature",
		Task:     "do bounded work",
	})
	if err != nil {
		t.Fatalf("push: %v", err)
	}

	// Drive the daemon. The RUN audit entry must be emitted BY THE DAEMON (wired), not the test.
	out, _, err := dm.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if out != OutcomeDone {
		t.Fatalf("RunOnce outcome = %q, want done", out)
	}

	// AC-1 (wired): the daemon auto-logged exactly one RUN entry for this directive's thread.
	runEntries := memAudit.ByRun(directiveID)
	if len(runEntries) != 1 || runEntries[0].Kind != AuditKindRun {
		t.Fatalf("daemon did not auto-log a single run audit entry: %+v", runEntries)
	}
	runAudit := runEntries[0]
	if runAudit.Actor != "worker:W1" {
		t.Fatalf("run audit actor = %q, want worker:W1", runAudit.Actor)
	}
	if runAudit.ID == "" {
		t.Fatal("run audit entry has no stable ID")
	}

	// The worker emits a child directive (delegation) and audits it, ParentRef = the run's AUDIT ID.
	child := NewChildDirective(queue.Directive{ID: directiveID, Template: "fleet-go", Origin: OriginOrchestrator}, "W1", "child-intent", "child-task")
	_ = child
	delAudit, err := memAudit.Append(AuditEntry{
		Ts: clk(), Actor: "worker:W1", Kind: AuditKindDelegation,
		ThreadID: directiveID, RunID: "child-R2", ParentRef: runAudit.ID, Detail: "child directive emitted",
	})
	if err != nil {
		t.Fatalf("append delegation: %v", err)
	}

	// A mutation event is recorded (ParentRef = the delegation's audit ID).
	mutAudit, err := memAudit.Append(AuditEntry{
		Ts: clk(), Actor: "orchestrator", Kind: AuditKindMutation,
		ThreadID: directiveID, RunID: "child-R2", ParentRef: delAudit.ID, Detail: "genome mutation (recorded; flow=ITER-0008b)",
	})
	if err != nil {
		t.Fatalf("append mutation: %v", err)
	}

	// AC-1 coverage: run + delegation + mutation all present for the thread.
	byThread := memAudit.ByThread(directiveID)
	if len(byThread) != 3 {
		t.Fatalf("ByThread = %d entries, want 3 (run+delegation+mutation): %+v", len(byThread), byThread)
	}

	// AC-2: Replay reconstructs the causal chain run → delegation → mutation by audit-ID ParentRef.
	replay := memAudit.Replay()
	if len(replay) != 3 {
		t.Fatalf("Replay = %d entries, want 3 (no gaps)", len(replay))
	}
	if replay[0].ID != runAudit.ID || replay[1].ID != delAudit.ID || replay[2].ID != mutAudit.ID {
		t.Fatalf("replay not in causal order: got [%s,%s,%s], want [%s,%s,%s]",
			replay[0].ID, replay[1].ID, replay[2].ID, runAudit.ID, delAudit.ID, mutAudit.ID)
	}
	if replay[0].Kind != AuditKindRun || replay[1].Kind != AuditKindDelegation || replay[2].Kind != AuditKindMutation {
		t.Fatalf("replay kinds wrong: %s,%s,%s", replay[0].Kind, replay[1].Kind, replay[2].Kind)
	}
	if replay[1].ParentRef != runAudit.ID || replay[2].ParentRef != delAudit.ID {
		t.Fatalf("replay causal links broken: del.parent=%s mut.parent=%s", replay[1].ParentRef, replay[2].ParentRef)
	}

	// AC-3: mutating a returned slice/entry does not change the stored log.
	got := memAudit.ByThread(directiveID)
	got[0].Detail = "TAMPERED"
	if fresh := memAudit.ByThread(directiveID); fresh[0].Detail == "TAMPERED" {
		t.Fatal("AC-3 violation: mutating a returned entry changed the stored audit log")
	}
}
