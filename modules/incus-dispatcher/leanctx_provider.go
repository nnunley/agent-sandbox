package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

// LeanCtxProvider is the DEFAULT ContextProvider adapter (STORY-0018 AC-1/2/3). It carries soft state
// between one-shot runs by driving the real lean-ctx CLI: diary + curated knowledge map onto
// project-scoped `lean-ctx knowledge` facts, and a handoff bundle is materialized on the shared
// volume per docs/plans/2026-06-21-handoff-bundle-schema.md. lean-ctx is ONE adapter, never a hard
// dependency — the daemon/runner depend only on ContextProvider (mirrors queue.Queue). Nothing here
// is authoritative: a failed lean-ctx call must never change a run's correctness (STORY-0018 AC-4),
// so callers treat provider errors as best-effort.
type LeanCtxProvider struct {
	// BundleRoot is the shared handoff-store volume root; bundles live at <BundleRoot>/<thread>/<run>/.
	BundleRoot string
	// ProjectDir is the working directory lean-ctx runs in (its knowledge store is project-scoped by
	// CWD). Empty = the current process dir.
	ProjectDir string
	// run is the injectable command runner (nil → execLeanCtx). Tests inject a recorder.
	run leanCtxRunner
	// Now supplies the bundle timestamp (nil → time.Now).
	Now func() time.Time
}

// leanCtxRunner executes `lean-ctx <args...>` in dir and returns combined output.
type leanCtxRunner func(dir string, args ...string) (string, error)

func execLeanCtx(dir string, args ...string) (string, error) {
	cmd := exec.Command("lean-ctx", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (p *LeanCtxProvider) runner() leanCtxRunner {
	if p.run != nil {
		return p.run
	}
	return execLeanCtx
}

func (p *LeanCtxProvider) clock() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// diaryCategory / knowledgeCategory namespace facts per thread so recall can scope to one thread.
func diaryCategory(threadID string) string     { return "diary:" + threadID }
func knowledgeCategory(threadID string) string { return "fleet:" + threadID }

// factLine parses one `lean-ctx knowledge recall` output line:
//
//	[category/key]: value (quality: 68%, ...)
//
// capturing category, key, and the verbatim value (up to the trailing " (quality:|confidence:" stats).
var factLine = regexp.MustCompile(`(?m)^\s*\[(.+)/([^/\]]+)\]:\s*(.*?)\s*\((?:quality|confidence):`)

// WriteDiary records the run's progression notes as project-scoped knowledge facts (AC-1). Each
// non-empty field (decisions/blockers/progress) becomes one fact under the thread's diary category;
// empty fields are skipped. Order is fixed for deterministic behavior.
func (p *LeanCtxProvider) WriteDiary(threadID string, d DiaryEntry) error {
	cat := diaryCategory(threadID)
	fields := []struct {
		key  string
		vals []string
	}{
		{"decisions", d.Decisions},
		{"blockers", d.Blockers},
		{"progress", d.Progress},
	}
	for _, f := range fields {
		if len(f.vals) == 0 {
			continue
		}
		b, err := json.Marshal(f.vals)
		if err != nil {
			return fmt.Errorf("marshal diary %s: %w", f.key, err)
		}
		if _, err := p.runner()(p.ProjectDir, "knowledge", "remember", string(b), "--category", cat, "--key", f.key); err != nil {
			return fmt.Errorf("write diary %s: %w", f.key, err)
		}
	}
	return nil
}

// RecallDiary reconstructs the diary for a thread from its knowledge facts (AC-1). Returns a single
// merged DiaryEntry, or nil when the thread has no diary.
func (p *LeanCtxProvider) RecallDiary(threadID string) ([]DiaryEntry, error) {
	cat := diaryCategory(threadID)
	out, err := p.runner()(p.ProjectDir, "knowledge", "recall", "--category", cat)
	if err != nil {
		return nil, fmt.Errorf("recall diary: %w", err)
	}
	var e DiaryEntry
	found := false
	for _, m := range factLine.FindAllStringSubmatch(out, -1) {
		if m[1] != cat {
			continue
		}
		var arr []string
		if err := json.Unmarshal([]byte(m[3]), &arr); err != nil {
			continue // a malformed/non-array value is soft state — skip, never fail correctness
		}
		switch m[2] {
		case "decisions":
			e.Decisions = arr
			found = true
		case "blockers":
			e.Blockers = arr
			found = true
		case "progress":
			e.Progress = arr
			found = true
		}
	}
	if !found {
		return nil, nil
	}
	return []DiaryEntry{e}, nil
}

// ShareKnowledge stores curated facts for cross-one-shot exchange (AC-2), namespaced to the thread.
func (p *LeanCtxProvider) ShareKnowledge(threadID string, facts []Fact) error {
	cat := knowledgeCategory(threadID)
	for _, f := range facts {
		if _, err := p.runner()(p.ProjectDir, "knowledge", "remember", f.Value, "--category", cat, "--key", f.Key); err != nil {
			return fmt.Errorf("share knowledge %s: %w", f.Key, err)
		}
	}
	return nil
}

// ReceiveKnowledge retrieves the thread's curated facts (AC-2).
func (p *LeanCtxProvider) ReceiveKnowledge(threadID string) ([]Fact, error) {
	cat := knowledgeCategory(threadID)
	out, err := p.runner()(p.ProjectDir, "knowledge", "recall", "--category", cat)
	if err != nil {
		return nil, fmt.Errorf("receive knowledge: %w", err)
	}
	var facts []Fact
	for _, m := range factLine.FindAllStringSubmatch(out, -1) {
		if m[1] != cat {
			continue
		}
		facts = append(facts, Fact{Key: m[2], Value: m[3]})
	}
	return facts, nil
}

// sessionSaveID parses the explicit saved id from `lean-ctx session save` output:
//
//	Session 20260621-185412-967894s0 saved (v0).
var sessionSaveID = regexp.MustCompile(`Session\s+(\S+)\s+saved`)

// bundleManifest is the on-disk manifest.json (schema_version 1) — the exact shape documented in
// docs/plans/2026-06-21-handoff-bundle-schema.md. It is serialized separately from the in-memory
// HandoffManifest the interface returns, so the on-disk contract and the Go API can evolve apart.
type bundleManifest struct {
	SchemaVersion      int           `json:"schema_version"`
	ThreadID           string        `json:"thread_id"`
	RunID              string        `json:"run_id"`
	ParentRunID        string        `json:"parent_run_id,omitempty"`
	CreatedTs          time.Time     `json:"created_ts"`
	WorkflowState      WorkflowState `json:"workflow_state"`
	SessionSnapshotRef struct {
		Path      string `json:"path"`
		SessionID string `json:"session_id"`
	} `json:"session_snapshot_ref"`
	CuratedKnowledge struct {
		Path  string `json:"path"`
		Count int    `json:"count"`
	} `json:"curated_knowledge"`
}

// CreateHandoff materializes a fresh handoff bundle for (threadID, runID) on the shared volume and
// returns its directory path (AC-3). It saves the lean-ctx session to capture the EXPLICIT session
// id (never "latest" — the STORY-0034 spike lesson), exports curated knowledge into the bundle, and
// writes manifest.json. Soft state only: no diff/grade ever enters the bundle (AC-4).
func (p *LeanCtxProvider) CreateHandoff(threadID, runID string, st WorkflowState) (string, error) {
	dir := filepath.Join(p.BundleRoot, threadID, runID)
	if err := os.MkdirAll(filepath.Join(dir, "session"), 0o755); err != nil {
		return "", fmt.Errorf("create bundle dir: %w", err)
	}

	// Capture an explicit session id (resolving by "latest" is unsafe — spike note).
	saveOut, err := p.runner()(p.ProjectDir, "session", "save")
	if err != nil {
		return "", fmt.Errorf("session save: %w", err)
	}
	sessionID := ""
	if m := sessionSaveID.FindStringSubmatch(saveOut); m != nil {
		sessionID = m[1]
	}

	// Export curated knowledge into the bundle (best-effort; an empty store is fine).
	knowledgePath := "knowledge.jsonl"
	_, _ = p.runner()(p.ProjectDir, "knowledge", "export", "--format", "jsonl", "--output", filepath.Join(dir, knowledgePath))

	man := bundleManifest{
		SchemaVersion: 1,
		ThreadID:      threadID,
		RunID:         runID,
		CreatedTs:     p.clock(),
		WorkflowState: st,
	}
	man.SessionSnapshotRef.Path = filepath.Join("session", sessionID+".json")
	man.SessionSnapshotRef.SessionID = sessionID
	man.CuratedKnowledge.Path = knowledgePath

	b, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}
	return dir, nil
}

// ImportHandoff reads a bundle's manifest and returns it as the interface's HandoffManifest (AC-3),
// flattening the explicit session id. It does not hydrate lean-ctx state here — the daemon import is
// best-effort and the bundle is soft state; a successor run resolves the explicit session id from
// SessionID (never "latest").
func (p *LeanCtxProvider) ImportHandoff(bundlePath string) (HandoffManifest, error) {
	b, err := os.ReadFile(filepath.Join(bundlePath, "manifest.json"))
	if err != nil {
		return HandoffManifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var man bundleManifest
	if err := json.Unmarshal(b, &man); err != nil {
		return HandoffManifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	return HandoffManifest{
		SchemaVersion: man.SchemaVersion,
		ThreadID:      man.ThreadID,
		RunID:         man.RunID,
		ParentRunID:   man.ParentRunID,
		WorkflowState: man.WorkflowState,
		SessionID:     man.SessionSnapshotRef.SessionID,
	}, nil
}
