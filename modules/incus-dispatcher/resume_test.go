package main

import (
	"bytes"
	"testing"
)

// STORY-0029 AC-4a: known thread + passing result → all soft fields copied, LastDiff==PatchData, LastGrade=="pass".
func TestReconstructResumeAudit_PassingResult(t *testing.T) {
	store := NewThreadStore()
	th := Thread{
		ID:               "thread-1",
		Status:           StatusActive,
		CurrentBranch:    "feat/auth",
		CurrentWorkspace: "ws-abc",
		ResumeSummary: ResumeSummary{
			PriorWork: "added middleware",
			NextStep:  "write tests",
		},
		LastVerifiedState: "commit:abc123",
		OpenQuestions:     []string{"should we use JWT?", "token expiry?"},
	}
	store.Put(th)

	patchData := []byte("diff --git a/foo.go b/foo.go\n+added line")
	result := &Result{
		ExitCode:  0,
		PatchData: patchData,
		ExternalGradingResult: &GradingResult{
			PatchApplied: true,
			ExitCode:     0,
		},
	}

	audit, ok := ReconstructResumeAudit(store, "thread-1", result)
	if !ok {
		t.Fatal("want ok=true for known thread")
	}

	if audit.ThreadID != "thread-1" {
		t.Errorf("ThreadID: got %q, want %q", audit.ThreadID, "thread-1")
	}
	if audit.Branch != "feat/auth" {
		t.Errorf("Branch: got %q, want %q", audit.Branch, "feat/auth")
	}
	if audit.Workspace != "ws-abc" {
		t.Errorf("Workspace: got %q, want %q", audit.Workspace, "ws-abc")
	}
	if audit.ResumeSummary != th.ResumeSummary {
		t.Errorf("ResumeSummary: got %+v, want %+v", audit.ResumeSummary, th.ResumeSummary)
	}
	if audit.LastVerified != "commit:abc123" {
		t.Errorf("LastVerified: got %q, want %q", audit.LastVerified, "commit:abc123")
	}
	if len(audit.OpenQuestions) != 2 || audit.OpenQuestions[0] != "should we use JWT?" || audit.OpenQuestions[1] != "token expiry?" {
		t.Errorf("OpenQuestions: got %v, want %v", audit.OpenQuestions, th.OpenQuestions)
	}
	if !bytes.Equal(audit.LastDiff, patchData) {
		t.Errorf("LastDiff: got %v, want %v", audit.LastDiff, patchData)
	}
	if audit.LastGrade != "pass" {
		t.Errorf("LastGrade: got %q, want \"pass\"", audit.LastGrade)
	}
}

// STORY-0029 AC-4a: failing result (ExitCode != 0) → LastGrade=="fail".
func TestReconstructResumeAudit_FailingResult(t *testing.T) {
	store := NewThreadStore()
	store.Put(Thread{ID: "thread-2", Status: StatusActive})

	result := &Result{ExitCode: 1}
	audit, ok := ReconstructResumeAudit(store, "thread-2", result)
	if !ok {
		t.Fatal("want ok=true for known thread")
	}
	if audit.LastGrade != "fail" {
		t.Errorf("LastGrade: got %q, want \"fail\"", audit.LastGrade)
	}
}

// STORY-0029 AC-4a: grading result not passed (PatchApplied false) → LastGrade=="fail".
func TestReconstructResumeAudit_GradingFailed(t *testing.T) {
	store := NewThreadStore()
	store.Put(Thread{ID: "thread-5", Status: StatusActive})

	result := &Result{
		ExitCode:  0,
		PatchData: []byte("patch"),
		ExternalGradingResult: &GradingResult{
			PatchApplied: false, // patch didn't apply
			ExitCode:     1,
		},
	}
	audit, ok := ReconstructResumeAudit(store, "thread-5", result)
	if !ok {
		t.Fatal("want ok=true for known thread")
	}
	if audit.LastGrade != "fail" {
		t.Errorf("LastGrade: got %q, want \"fail\"", audit.LastGrade)
	}
	if !bytes.Equal(audit.LastDiff, result.PatchData) {
		t.Errorf("LastDiff: got %v, want %v", audit.LastDiff, result.PatchData)
	}
}

// STORY-0029 AC-4a: nil last result → LastDiff nil, LastGrade "".
func TestReconstructResumeAudit_NilResult(t *testing.T) {
	store := NewThreadStore()
	store.Put(Thread{ID: "thread-3", Status: StatusActive})

	audit, ok := ReconstructResumeAudit(store, "thread-3", nil)
	if !ok {
		t.Fatal("want ok=true for known thread")
	}
	if audit.LastDiff != nil {
		t.Errorf("LastDiff: got %v, want nil", audit.LastDiff)
	}
	if audit.LastGrade != "" {
		t.Errorf("LastGrade: got %q, want empty string", audit.LastGrade)
	}
}

// Unknown threadID → ok=false.
func TestReconstructResumeAudit_UnknownThread(t *testing.T) {
	store := NewThreadStore()
	_, ok := ReconstructResumeAudit(store, "nonexistent", nil)
	if ok {
		t.Error("want ok=false for unknown thread")
	}
}

// STORY-0029 AC-3: ContinueRun sets Task.Ref to the thread's Branch; other fields preserved.
func TestContinueRun_SetsBranch(t *testing.T) {
	audit := ResumeAudit{
		ThreadID: "thread-1",
		Branch:   "feat/auth",
	}
	base := Task{
		Name: "test-task",
		Repo: "https://example.com/repo",
		Ref:  "main",
		Cmd:  []string{"go", "test", "./..."},
	}

	got := audit.ContinueRun(base)
	if got.Ref != "feat/auth" {
		t.Errorf("Ref: got %q, want %q", got.Ref, "feat/auth")
	}
	if got.Name != base.Name {
		t.Errorf("Name: got %q, want %q", got.Name, base.Name)
	}
	if got.Repo != base.Repo {
		t.Errorf("Repo: got %q, want %q", got.Repo, base.Repo)
	}
	if len(got.Cmd) != len(base.Cmd) || got.Cmd[0] != base.Cmd[0] {
		t.Errorf("Cmd: got %v, want %v", got.Cmd, base.Cmd)
	}
}

// STORY-0029 AC-3: empty Branch → base returned unchanged.
func TestContinueRun_EmptyBranchReturnsBase(t *testing.T) {
	audit := ResumeAudit{
		ThreadID: "thread-1",
		Branch:   "",
	}
	base := Task{
		Name: "test-task",
		Repo: "https://example.com/repo",
		Ref:  "main",
	}

	got := audit.ContinueRun(base)
	if got.Ref != "main" {
		t.Errorf("Ref: got %q, want %q (should be unchanged)", got.Ref, "main")
	}
	if got.Name != base.Name {
		t.Errorf("Name changed: got %q, want %q", got.Name, base.Name)
	}
}
