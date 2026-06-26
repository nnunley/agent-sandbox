package main

import (
	"context"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// TestDaemonCostCapture_Integration verifies that Daemon.RunOnce captures cost metrics
// from the Result onto the Run when storing results (STORY-0035 AC-4).
// This tests the real daemon path: daemon processes a directive → runner returns Result
// with usage → daemon stores Result with cost fields preserved.
func TestDaemonCostCapture_Integration(t *testing.T) {
	// 1. Set up a fake queue with one directive.
	fakeQ := NewTestQueue(
		queue.Directive{
			ID:       "test-directive-1",
			Template: "test-template",
			Repo:     "https://example.com/repo",
			Ref:      "main",
			Task:     "go test ./...",
			Origin:   OriginOrchestrator,
		},
	)

	// 2. Set up a fake runner that returns a Result with usage metrics.
	fakeRunner := &TestRunner{
		nextResult: &Result{
			ExitCode:   0,
			Stdout:     "tests passed",
			Stderr:     "",
			Duration:   5 * time.Second,
			TokensIn:   250,  // Cost metric from the LLM provider
			TokensOut:  100,  // Cost metric from the LLM provider
			LatencyMs:  1500, // Cost metric from the LLM provider
			SpendUSD:   0.08, // Cost metric calculated from tokens
		},
	}

	// 3. Set up a minimal policy that allows the template.
	policy := &Policy{
		Templates: map[string]TemplateRule{
			"test-template": {},
		},
	}

	// 4. Set up a result store to capture the Result.
	results := NewResultStore()

	// 5. Create the daemon.
	daemon := &Daemon{
		Q:        fakeQ,
		Runner:   fakeRunner,
		Policy:   policy,
		Consumer: "test-consumer",
		LeaseDur: time.Second,
		Results:  results,
	}

	// 6. Run the daemon once.
	outcome, dirID, err := daemon.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	if outcome != OutcomeDone {
		t.Errorf("outcome = %v, want %v", outcome, OutcomeDone)
	}

	if dirID != "test-directive-1" {
		t.Errorf("dirID = %q, want test-directive-1", dirID)
	}

	// 7. Assert that the result store captured the Result with cost fields.
	storedResult, ok := results.Get("test-directive-1")
	if !ok {
		t.Fatalf("Result not stored for directive test-directive-1")
	}

	// The key assertion: cost fields from the runner's Result should be on the storedResult.
	// This proves that daemon captured cost from Result and persisted it.
	if storedResult.TokensIn != 250 {
		t.Errorf("TokensIn = %d, want 250 (from runner Result)", storedResult.TokensIn)
	}
	if storedResult.TokensOut != 100 {
		t.Errorf("TokensOut = %d, want 100", storedResult.TokensOut)
	}
	if storedResult.LatencyMs != 1500 {
		t.Errorf("LatencyMs = %d, want 1500", storedResult.LatencyMs)
	}
	if absFloat(storedResult.SpendUSD-0.08) > 0.0001 {
		t.Errorf("SpendUSD = %v, want 0.08", storedResult.SpendUSD)
	}
}

// TestRunner is a test runner that returns a fixed Result.
type TestRunner struct {
	nextResult *Result
}

func (tr *TestRunner) Run(ctx context.Context, task Task) (*Result, error) {
	return tr.nextResult, nil
}

func (tr *TestRunner) Cleanup() error {
	return nil
}

// TestQueue is a test queue implementation.
type TestQueue struct {
	directives []queue.Directive
	claimed    bool
}

func NewTestQueue(d ...queue.Directive) *TestQueue {
	return &TestQueue{directives: d}
}

func (tq *TestQueue) Push(d queue.Directive) (string, error) {
	tq.directives = append(tq.directives, d)
	return d.ID, nil
}

func (tq *TestQueue) Claim(consumer string, leaseDur time.Duration) (queue.Directive, queue.Lease, error) {
	if len(tq.directives) == 0 {
		return queue.Directive{}, queue.Lease{}, queue.ErrEmpty
	}
	d := tq.directives[0]
	tq.directives = tq.directives[1:]
	tq.claimed = true
	return d, queue.Lease{}, nil
}

func (tq *TestQueue) Touch(lease queue.Lease, leaseDur time.Duration) (queue.Lease, error) {
	return lease, nil
}

func (tq *TestQueue) Done(lease queue.Lease) error {
	return nil
}

func (tq *TestQueue) Requeue(lease queue.Lease, deferTime time.Time) error {
	return nil
}

func (tq *TestQueue) DeferDirective(lease queue.Lease, deferTime time.Time) error {
	return nil
}

func (tq *TestQueue) Park(lease queue.Lease) error {
	return nil
}

func (tq *TestQueue) Reap() (int, error) {
	return 0, nil
}

func (tq *TestQueue) Peek() (queue.Directive, error) {
	if len(tq.directives) == 0 {
		return queue.Directive{}, queue.ErrEmpty
	}
	return tq.directives[0], nil
}

func (tq *TestQueue) Len() (pending, claimed int) {
	return len(tq.directives), 0
}

// absFloat returns the absolute value of a float64 (for epsilon comparisons).
func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
