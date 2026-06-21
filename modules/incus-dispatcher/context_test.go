package main

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// failingProvider is a ContextProvider whose every operation fails — it models a lost/corrupt
// handoff store. It exists to prove correctness is independent of soft-state availability.
type failingProvider struct{}

func (failingProvider) WriteDiary(string, DiaryEntry) error      { return errors.New("ctx down") }
func (failingProvider) RecallDiary(string) ([]DiaryEntry, error) { return nil, errors.New("ctx down") }
func (failingProvider) ShareKnowledge(string, []Fact) error      { return errors.New("ctx down") }
func (failingProvider) ReceiveKnowledge(string) ([]Fact, error)  { return nil, errors.New("ctx down") }
func (failingProvider) CreateHandoff(string, string, WorkflowState) (string, error) {
	return "", errors.New("ctx down")
}
func (failingProvider) ImportHandoff(string) (HandoffManifest, error) {
	return HandoffManifest{}, errors.New("ctx down")
}

// TestRunOnce_HandoffLossDoesNotAffectGrade is the anti-reward-hack evidence for SCENARIO-0031 /
// STORY-0018 AC-4: even when the context provider is fully down AND the directive points at a
// handoff bundle, the run's outcome is decided solely by its Result (diff + oracle grade). Soft
// state is lossy-OK; authoritative state is not. CI-primary seam (no cluster needed).
func TestRunOnce_HandoffLossDoesNotAffectGrade(t *testing.T) {
	// A passing run still completes despite a dead provider + a (lost) handoff path.
	rPass := &fakeRunner{result: &Result{ExitCode: 0}}
	dmPass, qPass := newDaemon(rPass)
	dmPass.Context = failingProvider{}
	dp := validDirective()
	dp.HandoffIn = "/srv/handoff-store/thr-1/run-0" // bundle the provider can't read
	qPass.Push(dp)
	if out, _, err := dmPass.RunOnce(context.Background()); err != nil || out != OutcomeDone {
		t.Fatalf("handoff loss must not break a passing run → (%q,%v), want done", out, err)
	}

	// A failing run still requeues — grade authoritative, unaffected by the dead provider.
	rFail := &fakeRunner{result: &Result{ExitCode: 1}}
	dmFail, qFail := newDaemon(rFail)
	dmFail.Context = failingProvider{}
	df := validDirective()
	df.HandoffIn = "/srv/handoff-store/thr-1/run-0"
	qFail.Push(df)
	if out, _, _ := dmFail.RunOnce(context.Background()); out != OutcomeRequeued {
		t.Fatalf("handoff loss must not change fail→requeue, got %q", out)
	}

	// The oracle grade stays authoritative under handoff loss: framework exit 0 but oracle fail → fail.
	rOracle := &fakeRunner{result: &Result{ExitCode: 0, ExternalGradingResult: &GradingResult{PatchApplied: true, ExitCode: 1}}}
	dmOracle, qOracle := newDaemon(rOracle)
	dmOracle.Context = failingProvider{}
	do := validDirective()
	do.HandoffIn = "/srv/handoff-store/thr-1/run-0"
	qOracle.Push(do)
	if out, _, _ := dmOracle.RunOnce(context.Background()); out != OutcomeRequeued {
		t.Fatalf("oracle-fail under handoff loss must requeue, got %q", out)
	}
}

// TestContextProviderIsNotAWorkQueue is the structural guard for STORY-0018 AC-5: the lean-ctx
// message bus (or any ContextProvider) must NEVER be the work queue. The interface has no claim/pop
// method, so a ContextProvider value cannot satisfy queue.Queue — the daemon's only work source.
func TestContextProviderIsNotAWorkQueue(t *testing.T) {
	var cp ContextProvider = NoopProvider{}
	if _, ok := any(cp).(queue.Queue); ok {
		t.Fatal("ContextProvider must not satisfy queue.Queue — context is soft state, not dispatch (STORY-0018 AC-5)")
	}
	// And the failing provider likewise cannot be a queue.
	if _, ok := any(failingProvider{}).(queue.Queue); ok {
		t.Fatal("no ContextProvider may be a work queue (STORY-0018 AC-5)")
	}
}

// TestNoopProvider_DropsSoftStateWithoutError pins the default adapter's contract: every operation
// succeeds and returns empty — soft state is silently dropped, never an error path the daemon must handle.
func TestNoopProvider_DropsSoftStateWithoutError(t *testing.T) {
	var p ContextProvider = NoopProvider{}
	if err := p.WriteDiary("t", DiaryEntry{}); err != nil {
		t.Fatalf("WriteDiary: %v", err)
	}
	if d, err := p.RecallDiary("t"); err != nil || d != nil {
		t.Fatalf("RecallDiary → (%v,%v), want (nil,nil)", d, err)
	}
	if path, err := p.CreateHandoff("t", "r", WorkflowState{}); err != nil || path != "" {
		t.Fatalf("CreateHandoff → (%q,%v), want ('',nil)", path, err)
	}
	if m, err := p.ImportHandoff("/whatever"); err != nil || m.ThreadID != "" {
		t.Fatalf("ImportHandoff → (%+v,%v), want (zero,nil)", m, err)
	}
}
