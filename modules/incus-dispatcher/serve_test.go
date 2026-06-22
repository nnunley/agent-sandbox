package main

import (
	"context"
	"testing"
	"time"
)

// STORY-0007 AC-2: the coordinator is a DAEMON, not per-agent. Serve is the long-running
// loop that drains directives by calling RunOnce repeatedly — one coordinator process for
// many tasks (resolves D3 agent-lifecycle: agents are one-shot, the coordinator persists).

// UntilEmpty mode: drains every eligible directive then returns (a bounded coordinator pass).
func TestServe_DrainsUntilEmpty(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 0}}
	dm, q := newDaemon(r)
	for i := 0; i < 3; i++ {
		q.Push(validDirective())
	}
	stats, err := Serve(context.Background(), dm, ServeOptions{UntilEmpty: true})
	if err != nil {
		t.Fatalf("Serve err = %v", err)
	}
	if stats.Done != 3 {
		t.Errorf("Done = %d, want 3 (drained all)", stats.Done)
	}
	if r.runs != 3 {
		t.Errorf("runner ran %d times, want 3", r.runs)
	}
	if p, c := q.Len(); p != 0 || c != 0 {
		t.Errorf("queue not drained: %d/%d", p, c)
	}
}

// Long-running mode: the daemon keeps polling an empty queue and stops promptly on context
// cancellation (graceful shutdown), NOT by exiting when the queue momentarily empties.
func TestServe_LoopsUntilContextCanceled(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 0}}
	dm, q := newDaemon(r)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan ServeStats, 1)
	go func() {
		st, _ := Serve(ctx, dm, ServeOptions{PollInterval: time.Millisecond})
		done <- st
	}()

	// Feed one directive after the loop is already polling the empty queue.
	q.Push(validDirective())
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case st := <-done:
		if st.Done != 1 {
			t.Errorf("Done = %d, want 1 (processed the late directive, then kept polling until cancel)", st.Done)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after context cancel — not respecting shutdown")
	}
}

// A directive that fails grading is requeued (autonomous rung), counted, and the loop
// continues — the daemon does not die on a single task failure.
func TestServe_CountsOutcomesAndContinues(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 1}} // always fails grading
	dm, q := newDaemon(r)
	q.Push(validDirective())
	// UntilEmpty stops at the first OutcomeEmpty; a requeue keeps the directive eligible,
	// so bound the work with a deadline to avoid an infinite retry loop in the test.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	stats, _ := Serve(ctx, dm, ServeOptions{PollInterval: time.Millisecond})
	if stats.Requeued < 1 {
		t.Errorf("Requeued = %d, want >=1 (failing task requeued, loop continued)", stats.Requeued)
	}
}
