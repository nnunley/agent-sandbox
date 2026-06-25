package main

import (
	"fmt"
	"strings"
	"sync"
)

// ArtifactStore is an in-memory artifact repository keyed by run_id.
// This is the honest seam for ITER-0008 CI. A production system would
// back this with a durable object store (S3, GCS, or disk-based).
// For now, artifacts are persisted only in the current process lifetime.
// TODO(ITER-0008b): Wire durable backing (e.g., disk store, S3).
type ArtifactStore struct {
	mu    sync.RWMutex
	store map[string]map[string][]byte // run_id → artifact_id → data
}

// NewArtifactStore creates a new in-memory artifact store.
func NewArtifactStore() *ArtifactStore {
	return &ArtifactStore{
		store: make(map[string]map[string][]byte),
	}
}

// Store saves an artifact blob under the given run_id and artifact_id.
// Returns an opaque reference that can be used to retrieve the artifact later.
// The reference has the form "run_id:artifact_id" (opaque to callers).
func (as *ArtifactStore) Store(runID, artifactID string, data []byte) string {
	as.mu.Lock()
	defer as.mu.Unlock()

	if _, ok := as.store[runID]; !ok {
		as.store[runID] = make(map[string][]byte)
	}

	// Store a copy of the data to prevent external mutation.
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	as.store[runID][artifactID] = dataCopy

	// Return opaque reference.
	return fmt.Sprintf("%s:%s", runID, artifactID)
}

// Resolve retrieves artifact data by opaque reference.
// Returns the stored bytes and true if found, or nil and false otherwise.
func (as *ArtifactStore) Resolve(ref string) ([]byte, bool) {
	as.mu.RLock()
	defer as.mu.RUnlock()

	// Parse opaque reference: "run_id:artifact_id"
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return nil, false
	}

	runID := parts[0]
	artifactID := parts[1]

	runArtifacts, ok := as.store[runID]
	if !ok {
		return nil, false
	}

	data, ok := runArtifacts[artifactID]
	if !ok {
		return nil, false
	}

	// Return a copy to prevent external mutation.
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy, true
}

// CaptureArtifacts populates Run.ArtifactRefs and Run.LogRefs by storing
// artifacts and logs in the provided ArtifactStore.
// The store must be pre-created; callers own its lifetime.
//
// Kind classification rule:
// - Files ending in .patch → ArtifactDiff
// - Files containing "synthesis" in name (case-insensitive) → ArtifactSynthesis
// - Files containing "note" in name (case-insensitive) → ArtifactNote
// - Default for other /output/* files → ArtifactNote
//
// Artifacts are stored under run_id; each artifact gets a unique artifact_id.
// Returns error only on validation failure; storage is infallible (in-memory).
func CaptureArtifacts(store *ArtifactStore, run *Run, artifacts map[string][]byte, logs []byte) error {
	if store == nil {
		return fmt.Errorf("artifact store is nil")
	}
	if run == nil {
		return fmt.Errorf("run is nil")
	}

	// Capture each artifact with its classified kind.
	for artifactName, data := range artifacts {
		kind := classifyArtifact(artifactName)
		ref := store.Store(run.RunID, artifactName, data)
		run.ArtifactRefs = append(run.ArtifactRefs, ArtifactRef{
			Kind: kind,
			Ref:  ref,
		})
	}

	// Capture logs under a reserved artifact_id "logs".
	if len(logs) > 0 {
		logRef := store.Store(run.RunID, "logs", logs)
		run.LogRefs = append(run.LogRefs, logRef)
	}

	return nil
}

// classifyArtifact determines the ArtifactKind based on the artifact filename.
// Classification rule:
// - *.patch → ArtifactDiff
// - *synthesis* (case-insensitive) → ArtifactSynthesis
// - *note* (case-insensitive) → ArtifactNote
// - default → ArtifactNote
func classifyArtifact(name string) ArtifactKind {
	nameLower := strings.ToLower(name)

	if strings.HasSuffix(nameLower, ".patch") {
		return ArtifactDiff
	}

	if strings.Contains(nameLower, "synthesis") {
		return ArtifactSynthesis
	}

	if strings.Contains(nameLower, "note") {
		return ArtifactNote
	}

	// Default for /output/* files.
	return ArtifactNote
}
