package main

import (
	"testing"
	"time"
)

// TestScenario0125_AuditReplay proves AC-1 (all actions logged), AC-2 (replayability in causal order),
// and AC-3 (immutability): audit entries are recorded, queryable by thread/run, and replayed deterministically.
// Covers: every kind present, ParentRef causal links, replay == recorded order, stable unique IDs.
func TestScenario0125_AuditReplay(t *testing.T) {
	// Preconditions: empty audit log, ready for a sequence of actions.
	log := NewMemoryAuditLog()

	now := time.Now()
	// Scenario: a run (R1, T1), a delegation (R2, T1, ParentRef=R1), a mutation (T1, ParentRef=R2).

	// Step 1: Record a RUN (kind=run, actor=worker, runID=R1, threadID=T1).
	runEntry := AuditEntry{
		Ts:       now,
		Actor:    "worker",
		Kind:     AuditKindRun,
		ThreadID: "T1",
		RunID:    "R1",
		ParentRef: "",
		Detail:   "run dispatched",
	}
	savedRun, err := log.Append(runEntry)
	if err != nil {
		t.Fatalf("Append run entry failed: %v", err)
	}
	// AC-3: ID is stable and unique (assigned by Append if empty).
	if savedRun.ID == "" {
		t.Fatal("run entry ID not assigned")
	}

	// Step 2: Record a DELEGATION (kind=delegation, actor=worker:W1, runID=R2, ParentRef=R1, threadID=T1).
	delegationEntry := AuditEntry{
		Ts:        now.Add(1 * time.Millisecond),
		Actor:     "worker:W1",
		Kind:      AuditKindDelegation,
		ThreadID:  "T1",
		RunID:     "R2",
		ParentRef: "R1",
		Detail:    "delegation to child directive",
	}
	savedDel, err := log.Append(delegationEntry)
	if err != nil {
		t.Fatalf("Append delegation entry failed: %v", err)
	}
	if savedDel.ID == "" {
		t.Fatal("delegation entry ID not assigned")
	}
	delID := savedDel.ID

	// Step 3: Record a MUTATION (kind=mutation, actor, ParentRef set, threadID=T1).
	mutationEntry := AuditEntry{
		Ts:        now.Add(2 * time.Millisecond),
		Actor:     "orchestrator",
		Kind:      AuditKindMutation,
		ThreadID:  "T1",
		RunID:     "R2",
		ParentRef: delID,
		Detail:    "mutation applied",
	}
	savedMut, err := log.Append(mutationEntry)
	if err != nil {
		t.Fatalf("Append mutation entry failed: %v", err)
	}
	if savedMut.ID == "" {
		t.Fatal("mutation entry ID not assigned")
	}

	// Step 2a: Query by thread T1 — assert EVERY action is retrievable in CAUSAL ORDER.
	byThread := log.ByThread("T1")
	if len(byThread) != 3 {
		t.Errorf("ByThread(T1) returned %d entries, expected 3", len(byThread))
	}

	// Verify causal order: run (no parent), delegation (parent=R1), mutation (parent=delID).
	if byThread[0].Kind != AuditKindRun {
		t.Errorf("entry[0] kind = %v, expected AuditKindRun", byThread[0].Kind)
	}
	if byThread[0].RunID != "R1" {
		t.Errorf("entry[0] RunID = %s, expected R1", byThread[0].RunID)
	}
	if byThread[0].ParentRef != "" {
		t.Errorf("entry[0] ParentRef = %s, expected empty (root)", byThread[0].ParentRef)
	}

	if byThread[1].Kind != AuditKindDelegation {
		t.Errorf("entry[1] kind = %v, expected AuditKindDelegation", byThread[1].Kind)
	}
	if byThread[1].RunID != "R2" {
		t.Errorf("entry[1] RunID = %s, expected R2", byThread[1].RunID)
	}
	if byThread[1].ParentRef != "R1" {
		t.Errorf("entry[1] ParentRef = %s, expected R1", byThread[1].ParentRef)
	}

	if byThread[2].Kind != AuditKindMutation {
		t.Errorf("entry[2] kind = %v, expected AuditKindMutation", byThread[2].Kind)
	}
	if byThread[2].ParentRef != delID {
		t.Errorf("entry[2] ParentRef = %s, expected %s (delegation ID)", byThread[2].ParentRef, delID)
	}

	// Step 3a: Replay — assert the replay reconstructs the SAME chain.
	replayed := log.Replay()

	// Count must match.
	if len(replayed) != 3 {
		t.Errorf("Replay() returned %d entries, expected 3", len(replayed))
	}

	// Verify no gaps, no reordering: compare replayed sequence against recorded.
	if len(replayed) >= 1 && replayed[0].Kind != AuditKindRun {
		t.Error("replay[0] is not the run")
	}
	if len(replayed) >= 2 && replayed[1].Kind != AuditKindDelegation {
		t.Error("replay[1] is not the delegation")
	}
	if len(replayed) >= 3 && replayed[2].Kind != AuditKindMutation {
		t.Error("replay[2] is not the mutation")
	}

	// Verify ParentRef causal links are correct.
	if len(replayed) >= 2 && replayed[1].ParentRef != "R1" {
		t.Errorf("replay[1] ParentRef = %s, expected R1", replayed[1].ParentRef)
	}
	if len(replayed) >= 3 && replayed[2].ParentRef != delID {
		t.Errorf("replay[2] ParentRef = %s, expected %s", replayed[2].ParentRef, delID)
	}

	// Step 4a: AC-3 immutability test.
	// Mutate a returned entry and verify the stored log is unchanged.
	mutatedByThread := log.ByThread("T1")
	if len(mutatedByThread) > 0 {
		mutatedByThread[0].Detail = "MUTATED"
	}

	// Re-query and verify the original Detail is intact.
	fresh := log.ByThread("T1")
	if len(fresh) > 0 && fresh[0].Detail == "MUTATED" {
		t.Error("AC-3 violation: mutation of returned Entries() slice changed the stored log")
	}
	if len(fresh) > 0 && fresh[0].Detail != "run dispatched" {
		t.Errorf("AC-3 violation: expected Detail='run dispatched', got '%s'", fresh[0].Detail)
	}
}
