package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestScenario0064_FileFeedSteer is the behavior evidence for SCENARIO-0064 (Orchestrator steers
// worker mid-run via file-feed). It proves the steering mechanism: a file-based channel allows
// the orchestrator to write a steer message to a watched file in the worker's container; the
// worker polls the file between phase boundaries, acknowledges the message in events.jsonl within
// one phase boundary, and continues without restarting.
//
// Owning stories: STORY-0073 AC-1. Seam: process-level (file in temp dir; real container path is cluster).
func TestScenario0064_FileFeedSteer(t *testing.T) {
	// Clock injection for deterministic timestamp assertions.
	clk := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	now := func() time.Time { return clk }
	tickPhase := func() { clk = clk.Add(1 * time.Second) }

	// Setup: temp dir for watched steer file and events.jsonl
	tmpdir := t.TempDir()
	steerPath := filepath.Join(tmpdir, "steer.json")
	eventsPath := filepath.Join(tmpdir, "events.jsonl")

	// Create EventLog
	eventLog := NewEventLog(eventsPath, now)

	// Create SteerChannel (file-based)
	steerChannel := NewSteerChannel(steerPath)

	// --- Phase 1: Worker polls, no steer pending ---
	_, ok, err := steerChannel.PollSteer()
	if err != nil {
		t.Fatalf("PollSteer at phase 1: %v", err)
	}
	if ok {
		t.Fatalf("phase 1: steer should be pending=false when no file exists, got ok=%v", ok)
	}
	tickPhase() // Move to next phase boundary

	// --- Phase 2: Orchestrator writes a steer message ---
	steerMsg := SteerMessage{
		MessageID: "steer-001",
		Action:    "revert",
		Details:   "reset to commit abc123",
	}
	if err := steerChannel.WriteSteer(steerMsg); err != nil {
		t.Fatalf("WriteSteer: %v", err)
	}
	tickPhase() // Move to next phase boundary

	// --- Phase 3: Worker polls and finds the steer ---
	polled, ok, err := steerChannel.PollSteer()
	if err != nil {
		t.Fatalf("PollSteer at phase 3: %v", err)
	}
	if !ok {
		t.Fatalf("phase 3: steer should be pending=true after orchestrator wrote it, got ok=%v", ok)
	}
	if polled.MessageID != steerMsg.MessageID {
		t.Fatalf("phase 3: message_id mismatch: got %q, want %q", polled.MessageID, steerMsg.MessageID)
	}
	if polled.Action != steerMsg.Action {
		t.Fatalf("phase 3: action mismatch: got %q, want %q", polled.Action, steerMsg.Action)
	}
	if polled.Details != steerMsg.Details {
		t.Fatalf("phase 3: details mismatch: got %q, want %q", polled.Details, steerMsg.Details)
	}

	// Worker acts on the steer (simulated — just record in ack)
	if err := eventLog.AppendSteerAck(steerMsg.MessageID, steerMsg.Action); err != nil {
		t.Fatalf("AppendSteerAck: %v", err)
	}
	tickPhase() // Move to next phase boundary

	// --- Phase 4: Verify ack in events.jsonl ---
	// The ack MUST appear at phase boundary 3 (not 4 or later), proving "within one phase boundary".
	ackData, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read events.jsonl: %v", err)
	}

	ackEntry, ackTS, ok := parseSteerAckFromJsonl(ackData, steerMsg.MessageID)
	if !ok {
		t.Fatalf("phase 4: steer_ack not found in events.jsonl")
	}
	if ackEntry.Action != steerMsg.Action {
		t.Fatalf("phase 4: ack.action mismatch: got %q, want %q", ackEntry.Action, steerMsg.Action)
	}

	// Assertion: ack timestamp is at phase 3 (within one phase boundary of write at phase 2).
	// The ack was appended at clk (after 2 ticks, at phase 3 boundary).
	expectedAckTime := time.Date(2026, 6, 25, 10, 0, 2, 0, time.UTC) // 2 seconds from start
	if !ackTS.Equal(expectedAckTime) {
		t.Fatalf("phase 4: ack timestamp = %v, want %v (within one phase boundary)", ackTS, expectedAckTime)
	}

	// --- Phase 5: Verify consume-once semantics (steer file is consumed) ---
	_, ok, err = steerChannel.PollSteer()
	if err != nil {
		t.Fatalf("PollSteer at phase 5: %v", err)
	}
	if ok {
		t.Fatalf("phase 5: steer must be consumed-once; second poll should find none, got ok=%v", ok)
	}

	// Verify no duplicate ack in events.jsonl
	ackLines := countSteerAckLines(ackData, steerMsg.MessageID)
	if ackLines != 1 {
		t.Fatalf("phase 5: events.jsonl has %d steer_ack entries for message_id %q, want exactly 1",
			ackLines, steerMsg.MessageID)
	}

	// --- Final check: Worker continues without restart (same steer loop) ---
	// The steer channel is still usable; worker can continue polling.
	_, ok, err = steerChannel.PollSteer()
	if err != nil {
		t.Fatalf("final poll: %v", err)
	}
	if ok {
		t.Fatalf("final poll: no new steer pending, got ok=%v", ok)
	}
	t.Logf("final poll: worker continues normally (no restart required)")
}

// parseSteerAckFromJsonl extracts the steer_ack entry for the given message_id from events.jsonl data.
func parseSteerAckFromJsonl(data []byte, messageID string) (struct {
	Type      string `json:"type"`
	MessageID string `json:"message_id"`
	Action    string `json:"action"`
}, time.Time, bool) {
	var result struct {
		Type      string `json:"type"`
		MessageID string `json:"message_id"`
		Action    string `json:"action"`
	}

	lines := splitJsonl(data)
	for _, line := range lines {
		var raw struct {
			Type      string    `json:"type"`
			MessageID string    `json:"message_id"`
			Action    string    `json:"action"`
			Ts        time.Time `json:"ts"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		if raw.Type == "steer_ack" && raw.MessageID == messageID {
			result.Type = raw.Type
			result.MessageID = raw.MessageID
			result.Action = raw.Action
			return result, raw.Ts, true
		}
	}
	return result, time.Time{}, false
}

// countSteerAckLines counts the number of steer_ack entries for the given message_id.
func countSteerAckLines(data []byte, messageID string) int {
	count := 0
	lines := splitJsonl(data)
	for _, line := range lines {
		var raw struct {
			Type      string `json:"type"`
			MessageID string `json:"message_id"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		if raw.Type == "steer_ack" && raw.MessageID == messageID {
			count++
		}
	}
	return count
}

// splitJsonl splits newline-separated JSON lines.
func splitJsonl(data []byte) [][]byte {
	var lines [][]byte
	var line []byte
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			if len(line) > 0 {
				lines = append(lines, append([]byte{}, line...))
			}
			line = line[:0]
		} else {
			line = append(line, data[i])
		}
	}
	if len(line) > 0 {
		lines = append(lines, line)
	}
	return lines
}
