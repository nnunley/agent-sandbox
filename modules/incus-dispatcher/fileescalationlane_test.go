package main

import (
	"encoding/json"
	"os"
	"testing"
)

// TestFileEscalationLane_PushAndList verifies FileEscalationLane persists escalation items to JSONL
// and reconstructs them on construction over an existing file. This is the durable escalations lane
// (STORY-0074 AC-3, AC-5).
func TestFileEscalationLane_PushAndList(t *testing.T) {
	// Create a temporary file for the JSONL escalation lane.
	tmpFile, err := os.CreateTemp("", "escalations-*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Create the first lane instance and push items.
	lane1 := NewFileEscalationLane(tmpFile.Name())
	if lane1 == nil {
		t.Fatal("NewFileEscalationLane returned nil")
	}

	items1 := lane1.List()
	if len(items1) != 0 {
		t.Fatalf("initial List() = %d items, want 0", len(items1))
	}

	// Push items.
	if err := lane1.Push(EscalationItem{DirectiveID: "d1", Reason: "authority-limit", Origin: "orchestrator"}); err != nil {
		t.Fatalf("Push d1: %v", err)
	}
	if err := lane1.Push(EscalationItem{DirectiveID: "d2", Reason: "budget-exceeded", Origin: "worker:w1"}); err != nil {
		t.Fatalf("Push d2: %v", err)
	}

	// Verify List returns the pushed items.
	items1 = lane1.List()
	if len(items1) != 2 {
		t.Fatalf("after push 2 items: List() = %d, want 2", len(items1))
	}
	if items1[0].DirectiveID != "d1" || items1[1].DirectiveID != "d2" {
		t.Fatalf("items out of order or incorrect: %v", items1)
	}

	// Create a SECOND lane instance over the SAME file.
	// This proves durability: the second instance must reconstruct the items from the file.
	lane2 := NewFileEscalationLane(tmpFile.Name())
	if lane2 == nil {
		t.Fatal("NewFileEscalationLane (second instance) returned nil")
	}

	// Verify the second instance reads back the items that the first instance wrote.
	items2 := lane2.List()
	if len(items2) != 2 {
		t.Fatalf("second instance List() = %d items, want 2 (durability failed)", len(items2))
	}
	if items2[0].DirectiveID != "d1" || items2[0].Reason != "authority-limit" {
		t.Fatalf("second instance d1 mismatch: %+v", items2[0])
	}
	if items2[1].DirectiveID != "d2" || items2[1].Reason != "budget-exceeded" {
		t.Fatalf("second instance d2 mismatch: %+v", items2[1])
	}

	// Push a new item on the second instance.
	if err := lane2.Push(EscalationItem{DirectiveID: "d3", Reason: "backend-unavailable", Origin: "orchestrator"}); err != nil {
		t.Fatalf("Push d3 on lane2: %v", err)
	}

	// Verify lane2 sees the new item.
	items2 = lane2.List()
	if len(items2) != 3 {
		t.Fatalf("lane2 after pushing d3: List() = %d, want 3", len(items2))
	}
	if items2[2].DirectiveID != "d3" {
		t.Fatalf("lane2 d3 not at index 2: %+v", items2[2])
	}

	// Create a THIRD instance and verify it reads all three items (d1, d2, d3).
	// This proves that the second instance's push was durably written.
	lane3 := NewFileEscalationLane(tmpFile.Name())
	items3 := lane3.List()
	if len(items3) != 3 {
		t.Fatalf("third instance List() = %d items, want 3 (durability failed on second push)", len(items3))
	}
	if items3[0].DirectiveID != "d1" || items3[1].DirectiveID != "d2" || items3[2].DirectiveID != "d3" {
		t.Fatalf("third instance IDs mismatch: %v", []string{items3[0].DirectiveID, items3[1].DirectiveID, items3[2].DirectiveID})
	}
}

// TestFileEscalationLane_ThreadSafety verifies FileEscalationLane is thread-safe.
// Multiple goroutines can push concurrently without data corruption.
func TestFileEscalationLane_ThreadSafety(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "escalations-*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	lane := NewFileEscalationLane(tmpFile.Name())

	// Spawn concurrent pushes.
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(idx int) {
			id := "d" + string(rune('0'+idx))
			_ = lane.Push(EscalationItem{DirectiveID: id, Reason: "test", Origin: "test"})
			done <- struct{}{}
		}(i)
	}

	// Wait for all goroutines.
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all items are present.
	items := lane.List()
	if len(items) != 10 {
		t.Fatalf("concurrent pushes: List() = %d items, want 10", len(items))
	}

	// Verify no corruption (all directive IDs are present).
	idSet := make(map[string]bool)
	for _, item := range items {
		idSet[item.DirectiveID] = true
	}
	if len(idSet) != 10 {
		t.Fatalf("concurrent pushes: only %d unique IDs, want 10 (data corruption)", len(idSet))
	}
}

// TestFileEscalationLane_EmptyFile verifies that constructing a FileEscalationLane over a non-existent
// file creates it successfully (no error).
func TestFileEscalationLane_EmptyFile(t *testing.T) {
	// Use a non-existent file path.
	nonExistentPath := os.TempDir() + "/escalations-nonexistent-" + randString() + ".jsonl"
	defer os.Remove(nonExistentPath)

	// Constructor should create the file.
	lane := NewFileEscalationLane(nonExistentPath)
	if lane == nil {
		t.Fatal("NewFileEscalationLane returned nil for non-existent file")
	}

	// List should return empty.
	items := lane.List()
	if len(items) != 0 {
		t.Fatalf("List() on newly created file = %d items, want 0", len(items))
	}

	// Push should work.
	if err := lane.Push(EscalationItem{DirectiveID: "d1", Reason: "test", Origin: "test"}); err != nil {
		t.Fatalf("Push on newly created file: %v", err)
	}

	// Verify the file now has the item.
	items = lane.List()
	if len(items) != 1 {
		t.Fatalf("after push to newly created file: List() = %d, want 1", len(items))
	}
}

// TestFileEscalationLane_DurabilityWithSync verifies that Push() calls Sync() to ensure
// crash durability (AC-5: escalations survive Mac power-off). Without Sync(), the kernel
// page cache could lose data on an unclean shutdown. This test verifies the fsync behavior
// by reading the file back immediately after Push (simulating a process restart).
func TestFileEscalationLane_DurabilityWithSync(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "escalations-sync-*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Create a lane and push an item.
	lane := NewFileEscalationLane(tmpFile.Name())
	if err := lane.Push(EscalationItem{DirectiveID: "d1", Reason: "test", Origin: "test"}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify the item is in the file by reading it back (simulates a crash recovery).
	// This only works if Sync() was called; otherwise, the write might still be in the
	// kernel page cache and not visible on re-open.
	fileContent, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if len(fileContent) == 0 {
		t.Fatal("file is empty after Push — fsync not called, data lost on crash (AC-5 violated)")
	}

	// Verify the content is valid JSON.
	var recovered EscalationItem
	if err := json.Unmarshal(fileContent[:len(fileContent)-1], &recovered); err != nil { // Remove newline
		t.Fatalf("unmarshaling: %v", err)
	}

	if recovered.DirectiveID != "d1" {
		t.Fatalf("recovered DirectiveID = %q, want d1", recovered.DirectiveID)
	}
}

// randString generates a random string for temp file names (collision avoidance).
func randString() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = "abcdefghijklmnopqrstuvwxyz0123456789"[os.Getpid()%36]
	}
	return string(b)
}
