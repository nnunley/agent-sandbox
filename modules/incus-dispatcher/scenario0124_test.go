package main

import (
	"context"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// TestScenario0124_MacStatelessClient proves STORY-0006 AC-1 ("Mac holds no fleet state; it
// authors directives and reviews results only") via the author → disconnect → reconnect → review
// lifecycle, with NO replay on reconnect.
//
// Model: there is exactly ONE durable CLUSTER substrate — the queue, the audit/decision log, and
// the thread store. In production these are laneq/Temporal/audit (durable, network-resident); here
// they are in-memory stand-ins. The honesty caveat: real cross-process durability + a queryable
// post-completion thread view is ITER-0006/Temporal's responsibility (MemoryQueue itself drops a
// directive once Done, which is why the reconnect "review" reads the thread store + audit log, the
// durable substrate components, not the queue). What THIS test proves is the STATELESSNESS PATTERN:
//
//   - AC author: the Mac's only action is to push a directive to the cluster queue, then it holds
//     NOTHING — there is deliberately no Mac-side object carried past the author step.
//   - AC offline: the fleet processes the directive with no Mac involvement; the cluster substrate
//     (thread store + audit log) records the completed outcome.
//   - AC reconnect (observable a): a fresh, stateless reader reconstructs the full picture PURELY
//     from handles to the SAME cluster substrate — the completed work IS visible (HARD assertions
//     on thread status == done AND a pass/done completion marker in the audit log).
//   - AC no-replay (observable b): a fresh reader daemon over the same substrate finds NOTHING to
//     run (OutcomeEmpty) and the Runner invocation count is UNCHANGED — the directive is not
//     recomputed.
//   - AC stateless (observable c): the Mac never held fleet state; its entire reconnect view comes
//     from the cluster substrate.
//
// Owning stories: STORY-0006. Seam: e2e (in-process, single shared substrate, genuine reader boundary).
// Execution command: cd modules/incus-dispatcher && go test . -run TestScenario0124_MacStatelessClient
func TestScenario0124_MacStatelessClient(t *testing.T) {
	clk := func() time.Time { return time.Unix(0, 0) }

	// The ONE durable CLUSTER substrate. The Mac holds none of this.
	clusterQueue := queue.NewMemoryQueue()
	clusterLog := NewMemoryDecisionLog()
	clusterThreads := NewThreadTracker(clk)
	runner := &trackingRunner0124{}

	// --- Phase 1: Mac AUTHORS a directive, then DISCONNECTS ---
	// The Mac's only action is to push to the cluster queue; afterward it holds nothing.
	directiveID, err := clusterQueue.Push(queue.Directive{
		Template: "fleet-go",
		Origin:   OriginOrchestrator,
		Intent:   "test-stateless",
	})
	if err != nil {
		t.Fatalf("author push: %v", err)
	}
	if p, c := clusterQueue.Len(); p != 1 || c != 0 {
		t.Fatalf("after author: want 1 pending / 0 claimed, got %d/%d", p, c)
	}

	// --- Phase 2: FLEET processes OFFLINE (the Mac is gone) ---
	// The fleet daemon shares NO state with the author — it only has handles to the cluster
	// substrate. It drains and processes the authored directive.
	fleet := &Daemon{
		Q:         clusterQueue,
		Runner:    runner,
		Policy:    testPolicy(),
		Consumer:  "fleet",
		LeaseDur:  time.Minute,
		Log:       clusterLog,
		Threads:   clusterThreads,
		Now:       clk,
		MapToTask: func(d queue.Directive) Task { return Task{Name: d.ID, Cmd: []string{"true"}} },
	}
	processed := 0
	for i := 0; i < 100; i++ {
		out, _, err := fleet.RunOnce(context.Background())
		if err != nil {
			t.Fatalf("fleet RunOnce: %v", err)
		}
		if out == OutcomeEmpty {
			break
		}
		processed++
	}
	if processed != 1 {
		t.Fatalf("fleet processed %d directives, want exactly 1", processed)
	}
	offlineInvocations := runner.invocationCount
	if offlineInvocations != 1 {
		t.Fatalf("fleet Runner invocations = %d, want 1", offlineInvocations)
	}
	// The cluster thread store records completion (durable substrate, survives the queue dropping
	// the Done directive).
	if got := clusterThreads.Status(directiveID); got != StatusDone {
		t.Fatalf("after fleet: cluster thread status = %q, want %q", got, StatusDone)
	}

	// --- Phase 3: Mac RECONNECTS as a fresh, stateless reader (observable a) ---
	// The reader carries NO state from the author/fleet steps; its entire view comes from handles
	// to the SAME cluster substrate. The completed work MUST be visible on reconnect — HARD
	// assertions (a regression that stopped recording status or audit lines fails the test).
	if got := clusterThreads.Status(directiveID); got != StatusDone {
		t.Fatalf("reconnect: completed work not visible — cluster thread status = %q, want %q", got, StatusDone)
	}
	records := clusterLog.Records()
	if len(records) == 0 {
		t.Fatalf("reconnect: audit log empty — fresh reader cannot see what the fleet did offline")
	}
	hasCompletion := false
	for _, d := range records {
		if d.DirectiveID == directiveID && d.Grade == "pass" && d.Action == "done" {
			hasCompletion = true
			break
		}
	}
	if !hasCompletion {
		t.Fatalf("reconnect: no pass/done completion marker for directive %s in the audit log", directiveID)
	}

	// --- Phase 4: NO REPLAY on reconnect (observable b) ---
	// A fresh reader daemon over the same cluster substrate must find NOTHING to run and must NOT
	// re-execute the completed directive. This actually EXERCISES the reconnect reader (not vacuous).
	freshReader := &Daemon{
		Q:         clusterQueue,
		Runner:    runner,
		Policy:    testPolicy(),
		Consumer:  "mac-reconnect",
		LeaseDur:  time.Minute,
		Log:       clusterLog,
		Threads:   clusterThreads,
		Now:       clk,
		MapToTask: func(d queue.Directive) Task { return Task{Name: d.ID, Cmd: []string{"true"}} },
	}
	out, _, err := freshReader.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("reconnect reader RunOnce: %v", err)
	}
	if out != OutcomeEmpty {
		t.Fatalf("reconnect reader found work (outcome %q); a completed directive must not be re-runnable", out)
	}
	if runner.invocationCount != offlineInvocations {
		t.Fatalf("replay detected: Runner invoked %d times after reconnect, want %d (no recompute)",
			runner.invocationCount, offlineInvocations)
	}
}

// trackingRunner0124 is a test double that records Run invocations. It always succeeds (exit 0) to
// keep the test focused on the statelessness/no-replay property, not error handling.
type trackingRunner0124 struct {
	invocationCount int
}

func (r *trackingRunner0124) Run(_ context.Context, task Task) (*Result, error) {
	r.invocationCount++
	return &Result{ExitCode: 0}, nil
}

func (r *trackingRunner0124) Cleanup() error {
	return nil
}
