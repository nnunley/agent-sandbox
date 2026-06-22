package main

import (
	"context"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// SCENARIO-0089 — Isolation tier declared by template selects the backend (D1).
// STORY-0023 AC-1. Proof seam: integration (daemon + policy + factory).
//
// Proves the card's observables: a Fast template resolves to the fast backend, a Hard
// template to the hard backend, an unset template tier defaults to Hard (fail-safe), the
// tier is fixed by the vetted TemplateRule (origin-invariant — a worker cannot downgrade
// isolation), and the resolved tier is written to the D6 decision log.
func TestScenario0089_TierSelectsBackend(t *testing.T) {
	newRunner := func() *fakeRunner { return &fakeRunner{result: &Result{ExitCode: 0}} }
	fast, hard := newRunner(), newRunner()

	q := queue.NewMemoryQueue()
	logmem := NewMemoryDecisionLog()
	dm := &Daemon{
		Q:       q,
		Runner:  newRunner(), // fallback — must NOT run when Backend is set
		Backend: newStaticBackendFactory(map[IsolationTier]Runner{TierFast: fast, TierHard: hard}),
		Policy: &Policy{Templates: map[string]TemplateRule{
			"fleet-fast":    {AllowWorkerOrigin: true, Tier: TierFast},  // trusted lane, worker-proposable
			"fleet-trading": {AllowWorkerOrigin: false, Tier: TierHard}, // sensitive lane, orchestrator-only
			"fleet-default": {AllowWorkerOrigin: true},                  // tier unset → Hard (fail-safe)
		}},
		Consumer:  "test",
		LeaseDur:  time.Minute,
		Log:       logmem,
		Threads:   NewThreadTracker(func() time.Time { return time.Unix(0, 0) }),
		Now:       func() time.Time { return time.Unix(0, 0) },
		MapToTask: func(d queue.Directive) Task { return Task{Name: d.ID, Cmd: []string{"true"}} },
	}

	mustRun := func(d queue.Directive, want DirectiveOutcome) {
		t.Helper()
		q.Push(d)
		out, _, err := dm.RunOnce(context.Background())
		if err != nil || out != want {
			t.Fatalf("template %q origin %q → (%q,%v), want %q", d.Template, d.Origin, out, err, want)
		}
	}

	// (a) Fast template → fast backend.
	mustRun(queue.Directive{Template: "fleet-fast", Origin: OriginOrchestrator, Intent: "a"}, OutcomeDone)
	// (b) Hard template (orchestrator) → hard backend.
	mustRun(queue.Directive{Template: "fleet-trading", Origin: OriginOrchestrator, Intent: "b"}, OutcomeDone)
	// (c) Unset-tier template → defaults to Hard.
	mustRun(queue.Directive{Template: "fleet-default", Origin: OriginOrchestrator, Intent: "c"}, OutcomeDone)
	// (d) Worker-origin on the Fast template still resolves Fast (origin-invariant tier:
	//     the worker cannot pick a different substrate — the template fixes it).
	mustRun(queue.Directive{Template: "fleet-fast", Origin: "worker:w1", Intent: "d"}, OutcomeDone)

	// Fast ran exactly twice (a + d); Hard ran twice (b + the unset-default c). Fallback never ran.
	if fast.runs != 2 {
		t.Errorf("fast backend runs=%d, want 2 (cases a,d)", fast.runs)
	}
	if hard.runs != 2 {
		t.Errorf("hard backend runs=%d, want 2 (cases b,c — c defaults to Hard)", hard.runs)
	}

	// (e) The resolved tier is recorded in the D6 decision log for each run.
	var tierEntries []string
	for _, d := range logmem.Records() {
		if d.Rule == "tier-select" {
			tierEntries = append(tierEntries, d.Action)
		}
	}
	want := []string{"fast", "hard", "hard", "fast"}
	if len(tierEntries) != len(want) {
		t.Fatalf("tier-select entries = %v, want %v", tierEntries, want)
	}
	for i, w := range want {
		if tierEntries[i] != w {
			t.Errorf("tier-select[%d] = %q, want %q (entries=%v)", i, tierEntries[i], w, tierEntries)
		}
	}
}

// D1 escalation-prevention at the tier boundary: a worker proposing the sensitive Hard
// template is denied BEFORE tier resolution (it never reaches a backend), so a worker can
// neither run a Hard template nor influence which tier runs.
func TestScenario0089_WorkerCannotProposeHardTemplate(t *testing.T) {
	hard := &fakeRunner{result: &Result{ExitCode: 0}}
	q := queue.NewMemoryQueue()
	dm := &Daemon{
		Q:       q,
		Runner:  &fakeRunner{result: &Result{ExitCode: 0}},
		Backend: newStaticBackendFactory(map[IsolationTier]Runner{TierHard: hard}),
		Policy: &Policy{Templates: map[string]TemplateRule{
			"fleet-trading": {AllowWorkerOrigin: false, Tier: TierHard},
		}},
		Consumer:  "test",
		LeaseDur:  time.Minute,
		MapToTask: func(d queue.Directive) Task { return Task{Name: d.ID, Cmd: []string{"true"}} },
	}
	q.Push(queue.Directive{Template: "fleet-trading", Origin: "worker:evil", Intent: "x"})
	out, _, _ := dm.RunOnce(context.Background())
	if out != OutcomeRejected {
		t.Fatalf("worker proposing Hard template → %q, want rejected (D1)", out)
	}
	if hard.runs != 0 {
		t.Errorf("hard backend ran %d times for a denied worker directive, want 0", hard.runs)
	}
}
