package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// SteerMessage represents a steering command from the orchestrator to a worker.
// MessageID uniquely identifies this steer message (computed by orchestrator).
// Action is the requested action (e.g., "revert", "retry", "halt").
// Details carries action-specific information (e.g., commit hash, reason).
type SteerMessage struct {
	MessageID string `json:"message_id"`
	Action    string `json:"action"`
	Details   string `json:"details"`
}

// SteerChannel manages file-based steering between orchestrator and worker.
// The orchestrator writes steer messages; the worker polls the file at phase boundaries.
// Consume-once semantics: a steer message is processed at one boundary, then consumed
// (deleted or marked), so it is not repeatedly processed.
type SteerChannel struct {
	path string
}

// NewSteerChannel creates a SteerChannel watching the given file path.
func NewSteerChannel(path string) *SteerChannel {
	return &SteerChannel{path: path}
}

// WriteSteer writes a steer message to the watched file (orchestrator side).
// The message is JSON-encoded; the file is created if it doesn't exist.
// If called multiple times, the latest message overwrites previous ones.
func (sc *SteerChannel) WriteSteer(msg SteerMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal steer message: %w", err)
	}
	if err := os.WriteFile(sc.path, data, 0o644); err != nil {
		return fmt.Errorf("write steer file: %w", err)
	}
	return nil
}

// PollSteer reads and consumes the steer message from the watched file (worker side).
// Returns (msg, ok, error) where:
// - ok=true if a steer message was found and consumed
// - ok=false if no steer message is pending (file doesn't exist or is empty)
// - error if reading fails (e.g., permission denied)
// Consume-once semantics: the file is deleted after successful parse.
func (sc *SteerChannel) PollSteer() (SteerMessage, bool, error) {
	data, err := os.ReadFile(sc.path)
	if err != nil {
		// File not found or not yet written — no steer pending.
		if os.IsNotExist(err) {
			return SteerMessage{}, false, nil
		}
		return SteerMessage{}, false, err
	}

	// File exists but is empty — no steer pending.
	if len(data) == 0 {
		return SteerMessage{}, false, nil
	}

	// Parse the steer message.
	var msg SteerMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return SteerMessage{}, false, fmt.Errorf("parse steer message: %w", err)
	}

	// Consume: delete the file so the same steer is not processed again. If delete fails we return
	// ok=false (consume failed) so the caller does NOT act on the steer; the message is not lost — a
	// later poll re-reads the file and retries the delete.
	if err := os.Remove(sc.path); err != nil {
		return SteerMessage{}, false, fmt.Errorf("consume steer (delete file): %w", err)
	}

	return msg, true, nil
}

// EventLog manages structured event logging (JSONL format).
// Events are appended one per line, with timestamps injected by the log.
type EventLog struct {
	path string
	now  func() time.Time
}

// NewEventLog creates an EventLog writing to the given path.
// now is a clock function (injected for testing).
func NewEventLog(path string, now func() time.Time) *EventLog {
	return &EventLog{path: path, now: now}
}

// AppendSteerAck appends a steer_ack event to the log with the current timestamp.
// A steer_ack event acknowledges that the worker received and acted on a steer message.
func (el *EventLog) AppendSteerAck(messageID, action string) error {
	event := struct {
		Type      string    `json:"type"`
		MessageID string    `json:"message_id"`
		Action    string    `json:"action"`
		Ts        time.Time `json:"ts"`
	}{
		Type:      "steer_ack",
		MessageID: messageID,
		Action:    action,
		Ts:        el.now(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal steer_ack event: %w", err)
	}

	// Append to JSONL file (create if not exists, append if exists).
	f, err := os.OpenFile(el.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open events.jsonl: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write steer_ack to events.jsonl: %w", err)
	}

	return nil
}
