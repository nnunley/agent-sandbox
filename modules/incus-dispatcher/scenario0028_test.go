package main

import (
	"context"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// SCENARIO-0028 — D2: the backend interface abstracts container vs. micro-VM delivery.
// STORY-0017 AC-1 / STORY-0004 AC-1. Proof seam: unit.
//
// The real container backends (incus, both runner flavors) and any future micro-VM/nspawn
// backend all satisfy the SAME Runner interface, so the coordination loop drives them
// without knowing the substrate. Compile-time conformance + a behavioral proof that the
// daemon's control flow is identical across two distinct Runner implementations.

// Compile-time conformance: the shipping container backends implement Runner. The
// ITER-0005b micro-VM/nspawn runners will add their own assertions here.
var (
	_ Runner = (*CLIContainerRunner)(nil)
	_ Runner = (*ClientContainerRunner)(nil)
	_ Runner = (*fakeRunner)(nil)
	// BackendFactory selects any Runner-conforming backend by tier.
	_ BackendFactory = (*staticBackendFactory)(nil)
)

// recordingRunner is a second, structurally-different Runner implementation. The point is
// that the daemon path treats it identically to fakeRunner — it only ever calls the
// interface methods, never inspecting the concrete substrate.
type recordingRunner struct {
	ran     bool
	cleaned bool
}

func (r *recordingRunner) Run(_ context.Context, _ Task) (*Result, error) {
	r.ran = true
	return &Result{ExitCode: 0}, nil
}
func (r *recordingRunner) Cleanup() error { r.cleaned = true; return nil }

func TestScenario0028_DaemonIsSubstrateAgnostic(t *testing.T) {
	// Two unrelated Runner implementations, registered under the same tier in turn. The
	// daemon must produce the same outcome and exercise each purely via the interface.
	impls := []struct {
		name   string
		runner Runner
		ran    func() bool
	}{
		{"fakeRunner", &fakeRunner{result: &Result{ExitCode: 0}}, nil},
		{"recordingRunner", &recordingRunner{}, nil},
	}

	for _, impl := range impls {
		q := queue.NewMemoryQueue()
		dm := &Daemon{
			Q:        q,
			Backend:  newStaticBackendFactory(map[IsolationTier]Runner{TierFast: impl.runner}),
			Policy:   &Policy{Templates: map[string]TemplateRule{"fleet": {AllowWorkerOrigin: true, Tier: TierFast}}},
			Consumer: "test",
			LeaseDur: time.Minute,
			MapToTask: func(d queue.Directive) Task {
				return Task{Name: d.ID, Cmd: []string{"true"}}
			},
		}
		q.Push(queue.Directive{Template: "fleet", Origin: OriginOrchestrator, Intent: "x"})
		out, _, err := dm.RunOnce(context.Background())
		if err != nil || out != OutcomeDone {
			t.Fatalf("%s: outcome (%q,%v), want done — daemon should drive any Runner identically", impl.name, out, err)
		}
	}

	// Spot-check the recordingRunner was actually exercised through the interface.
	rr := &recordingRunner{}
	q := queue.NewMemoryQueue()
	dm := &Daemon{
		Q:        q,
		Backend:  newStaticBackendFactory(map[IsolationTier]Runner{TierFast: rr}),
		Policy:   &Policy{Templates: map[string]TemplateRule{"fleet": {AllowWorkerOrigin: true, Tier: TierFast}}},
		Consumer: "test",
		LeaseDur: time.Minute,
		MapToTask: func(d queue.Directive) Task {
			return Task{Name: d.ID, Cmd: []string{"true"}}
		},
	}
	q.Push(queue.Directive{Template: "fleet", Origin: OriginOrchestrator, Intent: "y"})
	dm.RunOnce(context.Background())
	if !rr.ran || !rr.cleaned {
		t.Errorf("recordingRunner ran=%v cleaned=%v, want both true (driven via Runner interface)", rr.ran, rr.cleaned)
	}
}
