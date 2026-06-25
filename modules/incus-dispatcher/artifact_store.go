package main

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// artifactSeq backs the globally-unique portion of every artifact reference. Using a process-global
// counter (not a string derived from run_id + artifact name) means a reference can never collide and
// never needs to be PARSED to resolve — so artifact names or run ids containing any character
// (including the old "run_id:artifact_id" delimiter, or "/", or a path separator) are handled safely.
var artifactSeq atomic.Uint64

// ArtifactStore is an in-memory artifact repository. References are opaque keys resolved by a direct
// map lookup; a separate index links each run_id to the references stored under it (STORY-0015 AC-2
// "linked to run_id"). This is the honest seam for ITER-0008 CI. A production system would back this
// with a durable object store (S3, GCS, or disk-based); artifacts here live only for the process
// lifetime. TODO(ITER-0008b): wire durable backing.
type ArtifactStore struct {
	mu    sync.RWMutex
	blobs map[string][]byte   // opaque ref → data (resolution is a direct lookup; refs are never parsed)
	byRun map[string][]string // run_id → its artifact refs (run-linkage)
}

// NewArtifactStore creates a new in-memory artifact store.
func NewArtifactStore() *ArtifactStore {
	return &ArtifactStore{
		blobs: make(map[string][]byte),
		byRun: make(map[string][]string),
	}
}

// Store saves an artifact blob under the given run_id and artifact_id, returning an opaque reference.
// The reference embeds the run_id and artifact name for human readability plus a globally-unique
// suffix for collision-freedom, but callers MUST treat it as opaque — Resolve never parses it.
func (as *ArtifactStore) Store(runID, artifactID string, data []byte) string {
	as.mu.Lock()
	defer as.mu.Unlock()

	ref := fmt.Sprintf("%s/%s#a%d", runID, artifactID, artifactSeq.Add(1))

	// Store a copy of the data to prevent external mutation of the stored (durable) bytes.
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	as.blobs[ref] = dataCopy
	as.byRun[runID] = append(as.byRun[runID], ref)

	return ref
}

// Resolve retrieves artifact data by opaque reference via a direct map lookup (no parsing).
// Returns the stored bytes and true if found, or nil and false otherwise.
func (as *ArtifactStore) Resolve(ref string) ([]byte, bool) {
	as.mu.RLock()
	defer as.mu.RUnlock()

	data, ok := as.blobs[ref]
	if !ok {
		return nil, false
	}

	// Return a copy to prevent external mutation of the stored bytes.
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy, true
}

// RefsForRun returns all artifact references stored under a run_id (STORY-0015 AC-2: artifacts are
// linked to run_id and discoverable from it). The returned slice is a copy.
func (as *ArtifactStore) RefsForRun(runID string) []string {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return append([]string(nil), as.byRun[runID]...)
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
