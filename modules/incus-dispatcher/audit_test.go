package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestAuditKindEnum verifies AuditKind enum values.
func TestAuditKindEnum(t *testing.T) {
	kinds := []AuditKind{
		AuditKindRun,
		AuditKindDelegation,
		AuditKindTransition,
		AuditKindToolAction,
		AuditKindMutation,
	}
	for _, k := range kinds {
		if k == "" {
			t.Errorf("empty AuditKind value")
		}
	}
}

// TestMemoryAuditLog_Append verifies basic Append functionality.
func TestMemoryAuditLog_Append(t *testing.T) {
	log := NewMemoryAuditLog()

	// Append an entry without ID.
	entry := AuditEntry{
		Ts:       time.Now(),
		Actor:    "test",
		Kind:     AuditKindRun,
		ThreadID: "T1",
		RunID:    "R1",
		Detail:   "test entry",
	}

	saved, err := log.Append(entry)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Verify ID was assigned.
	if saved.ID == "" {
		t.Fatal("ID not assigned")
	}
	if !strings.HasPrefix(saved.ID, "audit-") {
		t.Errorf("ID does not have expected prefix: %s", saved.ID)
	}

	// Verify other fields are preserved.
	if saved.Actor != "test" {
		t.Errorf("Actor mismatch: %s", saved.Actor)
	}
	if saved.Kind != AuditKindRun {
		t.Errorf("Kind mismatch: %v", saved.Kind)
	}
}

// TestMemoryAuditLog_ImmutabilityOfEntries verifies that Entries() returns a copy.
func TestMemoryAuditLog_ImmutabilityOfEntries(t *testing.T) {
	log := NewMemoryAuditLog()

	entry := AuditEntry{
		Ts:       time.Now(),
		Actor:    "test",
		Kind:     AuditKindRun,
		ThreadID: "T1",
		RunID:    "R1",
		Detail:   "original",
	}
	log.Append(entry)

	// Get entries and mutate.
	entries := log.Entries()
	if len(entries) > 0 {
		entries[0].Detail = "MUTATED"
	}

	// Re-query and verify original is intact.
	fresh := log.Entries()
	if len(fresh) > 0 && fresh[0].Detail != "original" {
		t.Errorf("mutation of Entries() slice affected the log: %s", fresh[0].Detail)
	}
}

// TestMemoryAuditLog_ByThread verifies thread-based filtering.
func TestMemoryAuditLog_ByThread(t *testing.T) {
	log := NewMemoryAuditLog()

	log.Append(AuditEntry{Ts: time.Now(), ThreadID: "T1", RunID: "R1", Kind: AuditKindRun})
	log.Append(AuditEntry{Ts: time.Now(), ThreadID: "T2", RunID: "R2", Kind: AuditKindRun})
	log.Append(AuditEntry{Ts: time.Now(), ThreadID: "T1", RunID: "R3", Kind: AuditKindDelegation})

	t1Entries := log.ByThread("T1")
	if len(t1Entries) != 2 {
		t.Errorf("ByThread(T1) returned %d entries, expected 2", len(t1Entries))
	}
	for _, e := range t1Entries {
		if e.ThreadID != "T1" {
			t.Errorf("ByThread returned entry with wrong thread: %s", e.ThreadID)
		}
	}
}

// TestMemoryAuditLog_ByRun verifies run-based filtering.
func TestMemoryAuditLog_ByRun(t *testing.T) {
	log := NewMemoryAuditLog()

	log.Append(AuditEntry{Ts: time.Now(), ThreadID: "T1", RunID: "R1", Kind: AuditKindRun})
	log.Append(AuditEntry{Ts: time.Now(), ThreadID: "T1", RunID: "R2", Kind: AuditKindDelegation})
	log.Append(AuditEntry{Ts: time.Now(), ThreadID: "T1", RunID: "R1", Kind: AuditKindMutation})

	r1Entries := log.ByRun("R1")
	if len(r1Entries) != 2 {
		t.Errorf("ByRun(R1) returned %d entries, expected 2", len(r1Entries))
	}
	for _, e := range r1Entries {
		if e.RunID != "R1" {
			t.Errorf("ByRun returned entry with wrong run: %s", e.RunID)
		}
	}
}

// TestMemoryAuditLog_Replay verifies causal-order reconstruction.
func TestMemoryAuditLog_Replay(t *testing.T) {
	log := NewMemoryAuditLog()

	// Create entries with parent refs: run → delegation → mutation.
	runEntry, _ := log.Append(AuditEntry{
		Ts:       time.Now(),
		ThreadID: "T1",
		RunID:    "R1",
		Kind:     AuditKindRun,
		Detail:   "run",
	})

	delEntry, _ := log.Append(AuditEntry{
		Ts:        time.Now().Add(1 * time.Millisecond),
		ThreadID:  "T1",
		RunID:     "R2",
		ParentRef: runEntry.ID,
		Kind:      AuditKindDelegation,
		Detail:    "delegation",
	})

	log.Append(AuditEntry{
		Ts:        time.Now().Add(2 * time.Millisecond),
		ThreadID:  "T1",
		RunID:     "R2",
		ParentRef: delEntry.ID,
		Kind:      AuditKindMutation,
		Detail:    "mutation",
	})

	// Replay should reconstruct in causal order: run → delegation → mutation.
	replayed := log.Replay()
	if len(replayed) != 3 {
		t.Errorf("Replay returned %d entries, expected 3", len(replayed))
	}

	if len(replayed) > 0 && replayed[0].Kind != AuditKindRun {
		t.Errorf("replayed[0] kind = %v, expected AuditKindRun", replayed[0].Kind)
	}
	if len(replayed) > 1 && replayed[1].Kind != AuditKindDelegation {
		t.Errorf("replayed[1] kind = %v, expected AuditKindDelegation", replayed[1].Kind)
	}
	if len(replayed) > 2 && replayed[2].Kind != AuditKindMutation {
		t.Errorf("replayed[2] kind = %v, expected AuditKindMutation", replayed[2].Kind)
	}
}

// TestJSONLAuditLog_WritesJSONL verifies JSONL output format.
func TestJSONLAuditLog_WritesJSONL(t *testing.T) {
	var buf bytes.Buffer
	log := NewJSONLAuditLog(&buf, time.Now)

	entry := AuditEntry{
		Ts:       time.Now(),
		Actor:    "test",
		Kind:     AuditKindRun,
		ThreadID: "T1",
		RunID:    "R1",
		Detail:   "test",
	}

	log.Append(entry)

	// Verify output is valid JSONL (one JSON object per line).
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(lines))
	}

	// Verify the line is valid JSON.
	var decoded AuditEntry
	err := json.Unmarshal([]byte(lines[0]), &decoded)
	if err != nil {
		t.Fatalf("JSONL output is not valid JSON: %v", err)
	}

	// Verify decoded content matches.
	if decoded.Actor != "test" {
		t.Errorf("decoded.Actor = %s, expected test", decoded.Actor)
	}
	if decoded.Kind != AuditKindRun {
		t.Errorf("decoded.Kind = %v, expected AuditKindRun", decoded.Kind)
	}
}

// TestJSONLAuditLog_ImmutabilityOfEntries verifies that JSONL Entries() returns a copy.
func TestJSONLAuditLog_ImmutabilityOfEntries(t *testing.T) {
	var buf bytes.Buffer
	log := NewJSONLAuditLog(&buf, time.Now)

	entry := AuditEntry{
		Ts:       time.Now(),
		Actor:    "test",
		Kind:     AuditKindRun,
		ThreadID: "T1",
		RunID:    "R1",
		Detail:   "original",
	}
	log.Append(entry)

	// Get entries and mutate.
	entries := log.Entries()
	if len(entries) > 0 {
		entries[0].Detail = "MUTATED"
	}

	// Re-query and verify original is intact.
	fresh := log.Entries()
	if len(fresh) > 0 && fresh[0].Detail != "original" {
		t.Errorf("mutation of Entries() slice affected the log: %s", fresh[0].Detail)
	}
}

// TestAuditLog_StableIDAssignment verifies IDs are stable and unique.
func TestAuditLog_StableIDAssignment(t *testing.T) {
	log := NewMemoryAuditLog()

	// Append multiple entries and verify IDs are stable and unique.
	ids := make(map[string]bool)
	for i := 0; i < 5; i++ {
		saved, _ := log.Append(AuditEntry{
			Ts:       time.Now(),
			ThreadID: "T1",
			RunID:    "R1",
			Kind:     AuditKindRun,
		})
		if ids[saved.ID] {
			t.Fatalf("duplicate ID assigned: %s", saved.ID)
		}
		ids[saved.ID] = true
	}

	// Verify ID format.
	entries := log.Entries()
	for i, e := range entries {
		if !strings.HasPrefix(e.ID, "audit-") {
			t.Errorf("entry %d ID does not have expected prefix: %s", i, e.ID)
		}
	}
}
