package main

import (
	"context"
	"testing"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// tieredDaemon builds a daemon whose Backend factory maps the given tiers to fresh
// fakeRunners (returned so the test can inspect run counts), with a Policy whose templates
// declare tiers: "fleet-fast" → Fast, "fleet-hard" → Hard.
func tieredDaemon(t *testing.T, registry map[IsolationTier]Runner) (*Daemon, *queue.MemoryQueue, *MemoryDecisionLog) {
	t.Helper()
	q := queue.NewMemoryQueue()
	logmem := NewMemoryDecisionLog()
	dm := &Daemon{
		Q:       q,
		Runner:  &fakeRunner{result: &Result{ExitCode: 0}}, // fallback; should NOT be used when Backend is set
		Backend: newStaticBackendFactory(registry),
		Policy: &Policy{Templates: map[string]TemplateRule{
			"fleet-fast": {AllowWorkerOrigin: true, Tier: TierFast},
			"fleet-hard": {AllowWorkerOrigin: false, Tier: TierHard},
		}},
		Consumer:  "test",
		LeaseDur:  time.Minute,
		Log:       logmem,
		Threads:   NewThreadTracker(func() time.Time { return time.Unix(0, 0) }),
		Now:       func() time.Time { return time.Unix(0, 0) },
		MapToTask: func(d queue.Directive) Task { return Task{Name: d.ID, Cmd: []string{"true"}} },
	}
	return dm, q, logmem
}

// STORY-0023 AC-1 / STORY-0017 AC-1: the daemon resolves the tier from the validated
// template and runs the work on the backend the factory selects for that tier.
func TestRunOnce_SelectsBackendByTier(t *testing.T) {
	fast := &fakeRunner{result: &Result{ExitCode: 0}}
	hard := &fakeRunner{result: &Result{ExitCode: 0}}
	dm, q, _ := tieredDaemon(t, map[IsolationTier]Runner{TierFast: fast, TierHard: hard})

	q.Push(queue.Directive{Template: "fleet-fast", Origin: OriginOrchestrator, Intent: "x"})
	if out, _, err := dm.RunOnce(context.Background()); err != nil || out != OutcomeDone {
		t.Fatalf("fast directive → (%q,%v), want done", out, err)
	}
	if fast.runs != 1 || hard.runs != 0 {
		t.Fatalf("fast=%d hard=%d, want fast on the fast backend only", fast.runs, hard.runs)
	}

	q.Push(queue.Directive{Template: "fleet-hard", Origin: OriginOrchestrator, Intent: "y"})
	if out, _, err := dm.RunOnce(context.Background()); err != nil || out != OutcomeDone {
		t.Fatalf("hard directive → (%q,%v), want done", out, err)
	}
	if hard.runs != 1 {
		t.Fatalf("hard backend runs=%d, want 1", hard.runs)
	}
}

// The resolved tier is written to the D6 decision log so an auditor sees which substrate ran.
func TestRunOnce_RecordsResolvedTier(t *testing.T) {
	fast := &fakeRunner{result: &Result{ExitCode: 0}}
	dm, q, logmem := tieredDaemon(t, map[IsolationTier]Runner{TierFast: fast})
	q.Push(queue.Directive{Template: "fleet-fast", Origin: OriginOrchestrator, Intent: "x"})
	dm.RunOnce(context.Background())

	found := false
	for _, d := range logmem.Records() {
		if d.Rule == "tier-select" && d.Action == string(TierFast) {
			found = true
		}
	}
	if !found {
		t.Errorf("decision log has no tier-select entry for %q; got %+v", TierFast, logmem.Records())
	}
}

// Fail-safe: a tier with no registered backend (e.g. Hard before ITER-0005b's Firecracker)
// must NOT run — the directive is parked (durable, recoverable) and surfaced, never silently
// run on a weaker substrate.
func TestRunOnce_BackendUnavailableParks(t *testing.T) {
	// Only Fast registered; a Hard-tier template has no backend yet.
	dm, q, logmem := tieredDaemon(t, map[IsolationTier]Runner{TierFast: &fakeRunner{result: &Result{ExitCode: 0}}})
	q.Push(queue.Directive{Template: "fleet-hard", Origin: OriginOrchestrator, Intent: "z"})

	out, _, err := dm.RunOnce(context.Background())
	if err != nil || out != OutcomeEscalated {
		t.Fatalf("hard directive with no hard backend → (%q,%v), want escalated", out, err)
	}
	if q.Parked() != 1 {
		t.Errorf("directive was not parked (durable hold); Parked()=%d, want 1", q.Parked())
	}
	found := false
	for _, d := range logmem.Records() {
		if d.Rule == "backend-unavailable" {
			found = true
		}
	}
	if !found {
		t.Errorf("no backend-unavailable decision recorded; got %+v", logmem.Records())
	}
}
