package main

import (
	"testing"
)

// TestScenario0122_RunArtifactCapture is the behavior evidence for SCENARIO-0122:
// Run captures artifact_refs and log_refs across artifact types (STORY-0015 AC-1/2/3).
// This integration test proves:
// - Run.ArtifactRefs is populated with TYPED references (not inline blobs).
// - Run.LogRefs is populated with log references.
// - Artifacts are stored durably (in-memory now; durable backing is ITER-0008b residual).
// - Every artifact and log ref resolves back to exact original bytes (no content loss).
// - run_id correctly links artifacts in the store.
func TestScenario0122_RunArtifactCapture(t *testing.T) {
	// Create a run with a known RunID.
	runID := "run-scenario-0122-test"
	run := &Run{RunID: runID}

	// Build fake artifacts: 1 diff (patch) + 1 synthesis note.
	artifacts := map[string][]byte{
		"worker.patch": []byte("diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@"),
		"synthesis.md": []byte("# Synthesis\n\nThis is a synthesis artifact describing the work done."),
	}

	// Build fake logs.
	logs := []byte("task started\nclone succeeded\nbuild succeeded\ntask completed")

	// Create artifact store.
	store := NewArtifactStore()

	// Capture artifacts and logs into the run.
	err := CaptureArtifacts(store, run, artifacts, logs)
	if err != nil {
		t.Fatalf("CaptureArtifacts failed: %v", err)
	}

	// AC-1: Run.ArtifactRefs must be populated with TYPED references.
	if len(run.ArtifactRefs) != 2 {
		t.Fatalf("expected 2 artifact refs, got %d: %+v", len(run.ArtifactRefs), run.ArtifactRefs)
	}

	// Verify artifact kinds are correct.
	// We expect: worker.patch → ArtifactDiff, synthesis.md → ArtifactSynthesis.
	foundDiff := false
	foundSynthesis := false
	for _, ref := range run.ArtifactRefs {
		if ref.Kind == ArtifactDiff {
			foundDiff = true
		}
		if ref.Kind == ArtifactSynthesis {
			foundSynthesis = true
		}
	}
	if !foundDiff {
		t.Fatalf("expected ArtifactDiff kind in artifact_refs, got: %+v", run.ArtifactRefs)
	}
	if !foundSynthesis {
		t.Fatalf("expected ArtifactSynthesis kind in artifact_refs, got: %+v", run.ArtifactRefs)
	}

	// AC-1: Run.LogRefs must be populated.
	if len(run.LogRefs) != 1 {
		t.Fatalf("expected 1 log ref, got %d: %+v", len(run.LogRefs), run.LogRefs)
	}

	// Typed-reference discipline (AC-1): each Ref must be an OPAQUE key, not the inline blob and not
	// the bare filename. Assert the ref does not equal any artifact's content nor any artifact name.
	for _, ref := range run.ArtifactRefs {
		for name, content := range artifacts {
			if ref.Ref == string(content) {
				t.Fatalf("artifact ref %q equals inline content — refs must be opaque, not the blob", ref.Ref)
			}
			if ref.Ref == name {
				t.Fatalf("artifact ref %q equals the bare filename — ref must be an opaque store key", ref.Ref)
			}
		}
	}

	// AC-2: Artifacts must be resolvable back to exact original bytes (no content loss).
	// Also verify run_id correctly links them.
	for _, ref := range run.ArtifactRefs {
		data, ok := store.Resolve(ref.Ref)
		if !ok {
			t.Fatalf("failed to resolve artifact ref %q", ref.Ref)
		}

		// Find which original artifact this corresponds to (by content match, not key).
		var expectedData []byte
		var kindMatched string
		if ref.Kind == ArtifactDiff {
			expectedData = artifacts["worker.patch"]
			kindMatched = "worker.patch"
		} else if ref.Kind == ArtifactSynthesis {
			expectedData = artifacts["synthesis.md"]
			kindMatched = "synthesis.md"
		}

		if string(data) != string(expectedData) {
			t.Fatalf("artifact %q resolved data mismatch:\n  expected: %q\n  got: %q",
				kindMatched, string(expectedData), string(data))
		}
	}

	// Verify log ref resolves to exact original logs.
	logRef := run.LogRefs[0]
	logData, ok := store.Resolve(logRef)
	if !ok {
		t.Fatalf("failed to resolve log ref %q", logRef)
	}
	if string(logData) != string(logs) {
		t.Fatalf("log resolved data mismatch:\n  expected: %q\n  got: %q",
			string(logs), string(logData))
	}

	// AC-2 run_id linkage: every ref the Run recorded must be discoverable from the store BY run_id.
	runRefs := store.RefsForRun(runID)
	if len(runRefs) != 3 { // 2 artifacts + 1 log
		t.Fatalf("RefsForRun(%q) = %d refs, want 3 (2 artifacts + 1 log): %v", runID, len(runRefs), runRefs)
	}
	indexed := make(map[string]bool, len(runRefs))
	for _, r := range runRefs {
		indexed[r] = true
	}
	for _, ref := range run.ArtifactRefs {
		if !indexed[ref.Ref] {
			t.Fatalf("artifact ref %q not linked to run_id %q in the store index", ref.Ref, runID)
		}
	}
	if !indexed[run.LogRefs[0]] {
		t.Fatalf("log ref %q not linked to run_id %q in the store index", run.LogRefs[0], runID)
	}
}

// TestArtifactStore_MutationResistance proves the store deep-copies on BOTH Store (input) and Resolve
// (output), so a caller cannot corrupt the stored "durable" bytes by mutating an aliased slice in
// either direction (the same defensive-copy discipline the policy store needed).
func TestArtifactStore_MutationResistance(t *testing.T) {
	store := NewArtifactStore()

	input := []byte("original")
	ref := store.Store("run-mut", "art", input)

	// Mutating the caller's input slice after Store must not change the stored bytes.
	input[0] = 'X'
	got, ok := store.Resolve(ref)
	if !ok || string(got) != "original" {
		t.Fatalf("store corrupted by input mutation: got %q, want %q", string(got), "original")
	}

	// Mutating a Resolve result must not change the stored bytes.
	got[0] = 'Y'
	got2, _ := store.Resolve(ref)
	if string(got2) != "original" {
		t.Fatalf("store corrupted by output mutation: got %q, want %q", string(got2), "original")
	}
}
