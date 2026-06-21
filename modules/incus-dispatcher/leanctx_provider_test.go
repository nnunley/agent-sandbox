package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// LeanCtxProvider is the default ContextProvider adapter (STORY-0018 AC-1/2/3): it drives the real
// lean-ctx CLI for diary, knowledge, and handoff bundles. These tests exercise the adapter's real
// logic — the lean-ctx argv it constructs and the output it parses — via an injectable command
// runner, plus a guarded end-to-end diary round-trip against a real lean-ctx (SCENARIO-0030).

// compile-time: the adapter satisfies the interface the daemon depends on.
var _ ContextProvider = (*LeanCtxProvider)(nil)

type recordedCall struct {
	dir  string
	args []string
}

type scriptedRunner struct {
	calls   []recordedCall
	respond func(args []string) (string, error)
}

func (s *scriptedRunner) run(dir string, args ...string) (string, error) {
	s.calls = append(s.calls, recordedCall{dir: dir, args: append([]string(nil), args...)})
	if s.respond != nil {
		return s.respond(args)
	}
	return "", nil
}

func TestLeanCtxProvider_WriteDiaryEmitsRememberPerNonEmptyField(t *testing.T) {
	sr := &scriptedRunner{}
	p := &LeanCtxProvider{ProjectDir: "/proj", run: sr.run}

	// Blockers is empty → it must be skipped; decisions + progress are written.
	err := p.WriteDiary("t1", DiaryEntry{
		Decisions: []string{"chose A over B"},
		Progress:  []string{"did X", "did Y"},
	})
	if err != nil {
		t.Fatalf("WriteDiary: %v", err)
	}
	if len(sr.calls) != 2 {
		t.Fatalf("want 2 remember calls (decisions, progress), got %d: %+v", len(sr.calls), sr.calls)
	}
	first := sr.calls[0]
	if first.dir != "/proj" {
		t.Fatalf("lean-ctx must run in the project dir, got %q", first.dir)
	}
	want := []string{"knowledge", "remember", `["chose A over B"]`, "--category", "diary:t1", "--key", "decisions"}
	if !reflect.DeepEqual(first.args, want) {
		t.Fatalf("decisions argv = %v, want %v", first.args, want)
	}
	if sr.calls[1].args[6] != "progress" {
		t.Fatalf("second call key = %q, want progress", sr.calls[1].args[6])
	}
}

func TestLeanCtxProvider_RecallDiaryParsesRecallOutput(t *testing.T) {
	out := `Facts [diary:t1] (showing 2/2):
  [diary:t1/decisions]: ["chose A over B","pinned ref X"] (quality: 68%, confidence: 80%, confirmed: 2026-06-21 x1)
  [diary:t1/blockers]: ["blocked on socket"] (quality: 68%, confidence: 80%, confirmed: 2026-06-21 x1)
`
	sr := &scriptedRunner{respond: func(args []string) (string, error) { return out, nil }}
	p := &LeanCtxProvider{run: sr.run}

	entries, err := p.RecallDiary("t1")
	if err != nil {
		t.Fatalf("RecallDiary: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 merged diary entry, got %d", len(entries))
	}
	e := entries[0]
	if !reflect.DeepEqual(e.Decisions, []string{"chose A over B", "pinned ref X"}) {
		t.Fatalf("decisions = %v", e.Decisions)
	}
	if !reflect.DeepEqual(e.Blockers, []string{"blocked on socket"}) {
		t.Fatalf("blockers = %v", e.Blockers)
	}
	if len(e.Progress) != 0 {
		t.Fatalf("progress should be empty, got %v", e.Progress)
	}
	// recall must be scoped to the thread's diary category.
	want := []string{"knowledge", "recall", "--category", "diary:t1"}
	if !reflect.DeepEqual(sr.calls[0].args, want) {
		t.Fatalf("recall argv = %v, want %v", sr.calls[0].args, want)
	}
}

func TestLeanCtxProvider_ShareAndReceiveKnowledge(t *testing.T) {
	sr := &scriptedRunner{}
	p := &LeanCtxProvider{run: sr.run}
	if err := p.ShareKnowledge("t1", []Fact{{Key: "build-cmd", Value: "go test ./..."}}); err != nil {
		t.Fatalf("ShareKnowledge: %v", err)
	}
	want := []string{"knowledge", "remember", "go test ./...", "--category", "fleet:t1", "--key", "build-cmd"}
	if !reflect.DeepEqual(sr.calls[0].args, want) {
		t.Fatalf("share argv = %v, want %v", sr.calls[0].args, want)
	}

	sr2 := &scriptedRunner{respond: func(args []string) (string, error) {
		return "  [fleet:t1/build-cmd]: go test ./... (quality: 68%, confidence: 80%)\n", nil
	}}
	p2 := &LeanCtxProvider{run: sr2.run}
	facts, err := p2.ReceiveKnowledge("t1")
	if err != nil {
		t.Fatalf("ReceiveKnowledge: %v", err)
	}
	if len(facts) != 1 || facts[0].Key != "build-cmd" || facts[0].Value != "go test ./..." {
		t.Fatalf("received facts = %+v", facts)
	}
}

func TestLeanCtxProvider_CreateImportHandoffRoundTrip(t *testing.T) {
	root := t.TempDir()
	sr := &scriptedRunner{respond: func(args []string) (string, error) {
		if len(args) >= 2 && args[0] == "session" && args[1] == "save" {
			return "Session 20260621-185412-967894s0 saved (v0).", nil
		}
		return "", nil
	}}
	p := &LeanCtxProvider{BundleRoot: root, run: sr.run}

	st := WorkflowState{
		CurrentBranch:    "feature-x",
		CurrentWorkspace: "github.com/x/y",
		ResumeSummary:    ResumeSummary{PriorWork: "attempt 0 failed", NextStep: "retry"},
		OpenQuestions:    []string{"is the lowering complete?"},
	}
	path, err := p.CreateHandoff("t1", "r1", st)
	if err != nil {
		t.Fatalf("CreateHandoff: %v", err)
	}
	if path != filepath.Join(root, "t1", "r1") {
		t.Fatalf("bundle path = %q", path)
	}
	if _, err := os.Stat(filepath.Join(path, "manifest.json")); err != nil {
		t.Fatalf("manifest.json not written: %v", err)
	}

	m, err := p.ImportHandoff(path)
	if err != nil {
		t.Fatalf("ImportHandoff: %v", err)
	}
	if m.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", m.SchemaVersion)
	}
	if m.ThreadID != "t1" || m.RunID != "r1" {
		t.Fatalf("ids = %q/%q", m.ThreadID, m.RunID)
	}
	// The explicit session id is REQUIRED (STORY-0034 spike: never resolve by "latest").
	if m.SessionID != "20260621-185412-967894s0" {
		t.Fatalf("session id = %q, want the explicit saved id", m.SessionID)
	}
	if m.WorkflowState.CurrentBranch != "feature-x" || m.WorkflowState.ResumeSummary.NextStep != "retry" {
		t.Fatalf("workflow_state not round-tripped: %+v", m.WorkflowState)
	}
}

// SCENARIO-0030 — genuine diary round-trip against a REAL lean-ctx, isolated in a temp project
// (lean-ctx knowledge is project-scoped by CWD, so a fresh dir is its own empty store). Skips when
// lean-ctx is unavailable so CI without it stays green; never touches the repo's own project store.
func TestLeanCtxProvider_DiaryRoundTrip_Integration(t *testing.T) {
	if _, err := exec.LookPath("lean-ctx"); err != nil {
		t.Skip("lean-ctx not on PATH — integration round-trip skipped")
	}
	proj := t.TempDir()
	p := &LeanCtxProvider{ProjectDir: proj} // real runner
	const tid = "scenario0030-roundtrip"

	want := DiaryEntry{
		Decisions: []string{"carried decision across one-shots"},
		Blockers:  []string{"none"},
	}
	if err := p.WriteDiary(tid, want); err != nil {
		t.Fatalf("WriteDiary (real lean-ctx): %v", err)
	}
	got, err := p.RecallDiary(tid)
	if err != nil {
		t.Fatalf("RecallDiary (real lean-ctx): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 recalled diary entry, got %d", len(got))
	}
	if !reflect.DeepEqual(got[0].Decisions, want.Decisions) {
		t.Fatalf("decisions not preserved: got %v want %v", got[0].Decisions, want.Decisions)
	}
	if !reflect.DeepEqual(got[0].Blockers, want.Blockers) {
		t.Fatalf("blockers not preserved: got %v want %v", got[0].Blockers, want.Blockers)
	}
	// Best-effort cleanup of the isolated temp project's facts.
	for _, k := range []string{"decisions", "blockers"} {
		_, _ = execLeanCtx(proj, "knowledge", "remove", "--category", "diary:"+tid, "--key", k)
	}
}
