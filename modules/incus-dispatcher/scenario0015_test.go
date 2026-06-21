package main

import (
	"testing"
	"time"
)

// TestScenario0015_ResumeOnBranch is the behavior evidence for SCENARIO-0015 (Resume work on a
// branch with an existing thread). It integrates the ITER-0004 continuity pieces — ThreadStore
// (STORY-0029), WorkspaceRegistry (STORY-0033), ReconstructResumeAudit + ContinueRun (STORY-0029
// AC-3/AC-4a) — to prove a new work request on an actively-claimed branch CONTINUES the prior
// thread with its reconstructed context instead of reinventing it.
//
// Owning stories: STORY-0029, STORY-0030, STORY-0033. Seam: integration (in-process, fake backend).
func TestScenario0015_ResumeOnBranch(t *testing.T) {
	clk := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return clk }

	threads := NewThreadStore()
	workspaces := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "/repo", Branch: "feature-x"}

	// --- Run 1 (thread A) establishes work on feature-x ---
	const threadA = "thr-A"
	if _, ok := workspaces.Claim(key, threadA, "lease-A", time.Hour); !ok {
		t.Fatalf("thread A should claim the free workspace")
	}
	// On run completion the daemon records the thread's soft state + last verified result.
	threads.Put(Thread{
		ID:                threadA,
		Status:            StatusActive,
		CurrentBranch:     "feature-x",
		CurrentWorkspace:  "/repo@feature-x",
		ResumeSummary:     ResumeSummary{PriorWork: "scaffolded the parser", NextStep: "wire the evaluator"},
		LastVerifiedState: "unit suite green @c0ffee",
		OpenQuestions:     []string{"should eval be lazy?"},
	})
	lastResult := &Result{
		ExitCode:              0,
		PatchData:             []byte("diff --git a/parser.go b/parser.go"),
		ExternalGradingResult: &GradingResult{PatchApplied: true, ExitCode: 0},
	}

	// --- A NEW work request arrives for the SAME (repo, branch) ---
	// SCENARIO-0015: the coordinator must detect the existing thread, NOT treat the branch as blank.
	if d := workspaces.DecideReuse(key, threadA); d != ReuseContinue {
		t.Fatalf("same-thread new work on active branch must CONTINUE, got %q", d)
	}
	claim, ok := workspaces.ActiveClaim(key)
	if !ok || claim.ThreadID != threadA {
		t.Fatalf("lease must be valid and owned by thread A, got ok=%v owner=%q", ok, claim.ThreadID)
	}

	// Reconstruct the authoritative resume context before continuing (AC-4a).
	audit, ok := ReconstructResumeAudit(threads, threadA, lastResult)
	if !ok {
		t.Fatalf("resume audit must reconstruct for the known thread")
	}
	// Observable: resume_summary + last verified state are read back; diff/grade are authoritative.
	if audit.ResumeSummary.NextStep != "wire the evaluator" {
		t.Fatalf("resume_summary not carried: %+v", audit.ResumeSummary)
	}
	if audit.LastVerified != "unit suite green @c0ffee" {
		t.Fatalf("last_verified not available: %q", audit.LastVerified)
	}
	if string(audit.LastDiff) != "diff --git a/parser.go b/parser.go" || audit.LastGrade != "pass" {
		t.Fatalf("authoritative diff/grade wrong: diff=%q grade=%q", audit.LastDiff, audit.LastGrade)
	}
	if len(audit.OpenQuestions) != 1 {
		t.Fatalf("open questions not reconstructed: %+v", audit.OpenQuestions)
	}

	// Observable: the continuing run inherits the branch/workspace — no reinvention, no reset.
	base := Task{Name: "run-2", Repo: "/repo", Ref: "main", Cmd: []string{"make", "test"}}
	cont := audit.ContinueRun(base)
	if cont.Ref != "feature-x" {
		t.Fatalf("continuing run must stay on feature-x, not reset to %q", cont.Ref)
	}
	if audit.Workspace != "/repo@feature-x" {
		t.Fatalf("workspace must not be reset: %q", audit.Workspace)
	}
}

// TestScenario0015_SupersedeRequiresDeclaration is the anti-reinvention edge of SCENARIO-0015 /
// STORY-0030 AC-2/AC-3: a DIFFERENT thread cannot silently take an actively-claimed workspace — it
// must explicitly supersede with a reason, and the reinvention is captured as a structured stumble.
func TestScenario0015_SupersedeRequiresDeclaration(t *testing.T) {
	clk := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	workspaces := NewWorkspaceRegistry(func() time.Time { return clk })
	key := WorkspaceKey{Repo: "/repo", Branch: "feature-y"}

	workspaces.Claim(key, "thr-A", "lease-A", time.Hour)

	// A different thread sees it must supersede, not blindly claim.
	if d := workspaces.DecideReuse(key, "thr-B"); d != ReuseSupersede {
		t.Fatalf("different-thread new work must require SUPERSEDE, got %q", d)
	}
	if _, ok := workspaces.Claim(key, "thr-B", "lease-B", time.Hour); ok {
		t.Fatalf("blind claim over an active different-thread claim must fail")
	}
	// Superseding without a declared reason is rejected (AC-2).
	if _, _, ok := workspaces.Supersede(key, "thr-B", "lease-B", "", time.Hour); ok {
		t.Fatalf("supersede without a reason must be rejected")
	}
	// With a declared reason it succeeds and yields a duplicate-work stumble (AC-3).
	prior, stumble, ok := workspaces.Supersede(key, "thr-B", "lease-B", "thr-A stalled 3 days", time.Hour)
	if !ok || prior != "thr-A" || stumble.Type != StumbleDuplicateWork {
		t.Fatalf("declared supersede wrong: ok=%v prior=%q stumble=%+v", ok, prior, stumble)
	}
	// The reinvention signal lands on the new run's stumble_signals.
	run := &Run{RunID: "run-B"}
	run.AddStumble(stumble)
	if len(run.StumbleSignals) != 1 || run.StumbleSignals[0].RunID != "run-B" {
		t.Fatalf("reinvention stumble not captured on the run: %+v", run.StumbleSignals)
	}
}
