package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// SCENARIO-0054 — STORY-0058 AC-25: a FRESH handoff bundle is provided with each retry.
//
// Proven at the daemon seam with a fake backend (no Temporal). The daemon's responsibility on an
// autonomous requeue is to EMIT a fresh handoff bundle through the ContextProvider, capturing the
// just-failed run's soft state. Consuming that bundle is the provider+successor's job (proven on a
// real worker in T6 / SCENARIO-0030). Emission is best-effort: handoff loss never changes
// correctness (STORY-0018 AC-4, proven separately), so a provider error must not break the requeue.

// spyProvider records CreateHandoff invocations; NoopProvider supplies the rest of the interface.
type spyProvider struct {
	NoopProvider
	handoffs  []spyHandoff
	createErr error
}

type spyHandoff struct {
	threadID, runID string
	state           WorkflowState
}

func (s *spyProvider) CreateHandoff(threadID, runID string, st WorkflowState) (string, error) {
	s.handoffs = append(s.handoffs, spyHandoff{threadID: threadID, runID: runID, state: st})
	if s.createErr != nil {
		return "", s.createErr
	}
	return fmt.Sprintf("/handoff/%s/%s.json", threadID, runID), nil
}

func newRequeueDaemon(r Runner, q *queue.MemoryQueue, ctx ContextProvider) *Daemon {
	return &Daemon{
		Q:        q,
		Runner:   r,
		Policy:   testPolicy(),
		Consumer: "test",
		LeaseDur: time.Minute,
		Context:  ctx,
		MapToTask: func(d queue.Directive) Task {
			return Task{Name: d.ID, Cmd: []string{"true"}}
		},
	}
}

func TestRunOnce_RequeueEmitsFreshHandoff(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 1}} // always fails → autonomous requeue
	q := queue.NewMemoryQueue()
	spy := &spyProvider{}
	dm := newRequeueDaemon(r, q, spy)

	d := validDirective()
	d.Repo = "github.com/x/y"
	d.Ref = "feature-branch"
	id, _ := q.Push(d)

	// First failure → autonomous requeue → exactly one fresh handoff emitted.
	if out, _, _ := dm.RunOnce(context.Background()); out != OutcomeRequeued {
		t.Fatalf("first run → %q, want requeued", out)
	}
	if len(spy.handoffs) != 1 {
		t.Fatalf("want 1 fresh handoff emitted on requeue, got %d", len(spy.handoffs))
	}
	h0 := spy.handoffs[0]
	if h0.threadID != id {
		t.Fatalf("handoff threadID = %q, want directive id %q", h0.threadID, id)
	}
	if h0.state.CurrentBranch != "feature-branch" || h0.state.CurrentWorkspace != "github.com/x/y" {
		t.Fatalf("handoff state did not capture the run's workspace: %+v", h0.state)
	}
	if h0.state.ResumeSummary.NextStep == "" {
		t.Fatalf("handoff state must carry a resume hint for the retry; got empty NextStep")
	}

	// Second failure (attempts now incremented) → a SECOND, DISTINCT fresh handoff.
	if out, _, _ := dm.RunOnce(context.Background()); out != OutcomeRequeued {
		t.Fatalf("second run → %q, want requeued", out)
	}
	if len(spy.handoffs) != 2 {
		t.Fatalf("want a fresh handoff per retry, got %d total", len(spy.handoffs))
	}
	if spy.handoffs[1].runID == spy.handoffs[0].runID {
		t.Fatalf("each retry must get a FRESH (distinct) bundle; runIDs collided: %q", spy.handoffs[1].runID)
	}
}

// A passing run is terminal (no retry) → no handoff bundle is emitted: AC-25 is retry-specific.
func TestRunOnce_PassEmitsNoHandoff(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 0}}
	q := queue.NewMemoryQueue()
	spy := &spyProvider{}
	dm := newRequeueDaemon(r, q, spy)
	q.Push(validDirective())

	if out, _, _ := dm.RunOnce(context.Background()); out != OutcomeDone {
		t.Fatalf("pass → %q, want done", out)
	}
	if len(spy.handoffs) != 0 {
		t.Fatalf("a passing run must not emit a retry handoff, got %d", len(spy.handoffs))
	}
}

// A CreateHandoff error on the requeue path is best-effort: the requeue still succeeds
// (handoff loss never affects correctness — STORY-0018 AC-4).
func TestRunOnce_RequeueHandoffErrorIsBestEffort(t *testing.T) {
	r := &fakeRunner{result: &Result{ExitCode: 1}}
	q := queue.NewMemoryQueue()
	spy := &spyProvider{createErr: fmt.Errorf("provider down")}
	dm := newRequeueDaemon(r, q, spy)
	q.Push(validDirective())

	out, _, err := dm.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("requeue must not surface a handoff-emit error, got %v", err)
	}
	if out != OutcomeRequeued {
		t.Fatalf("handoff error → %q, want requeued (best-effort emit)", out)
	}
	if p, _ := q.Len(); p != 1 {
		t.Fatalf("directive must still be requeued despite handoff error, pending=%d", p)
	}
}
