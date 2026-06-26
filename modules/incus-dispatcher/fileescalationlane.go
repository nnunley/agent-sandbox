package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// FileEscalationLane is a durable, file-backed EscalationLane (STORY-0074 AC-3, AC-5).
// It writes escalation items as JSONL (one JSON object per line) to a file and reconstructs
// the lane's state on construction by reading the file. This enables durability across
// daemon restarts: a second Daemon instance constructed over the same file-backed lane
// reads all items the first instance wrote, proving AC-5 ("Mac returns → queued escalations processed").
// Thread-safe for concurrent Push/List operations.
type FileEscalationLane struct {
	path string // path to the JSONL file
	mu   sync.Mutex
	items []EscalationItem // in-memory copy (reconstructed on construction from file)
}

// NewFileEscalationLane constructs a FileEscalationLane over the given file path.
// If the file does not exist, it is created. If the file exists, all lines are parsed
// as JSON objects and reconstructed into the lane's in-memory state. Thread-safe.
func NewFileEscalationLane(path string) *FileEscalationLane {
	lane := &FileEscalationLane{path: path}

	// Attempt to open and read existing file (if it exists).
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue // Skip empty lines
			}
			var item EscalationItem
			if err := json.Unmarshal(line, &item); err != nil {
				// Log and skip corrupt lines (best-effort reconstruction).
				fmt.Fprintf(os.Stderr, "FileEscalationLane: skipping corrupt JSONL line: %v\n", err)
				continue
			}
			lane.items = append(lane.items, item)
		}
	} else if !os.IsNotExist(err) {
		// If the error is something other than "not exists" (e.g., permission), log it.
		// This is best-effort; the lane will start empty.
		fmt.Fprintf(os.Stderr, "FileEscalationLane: failed to open %s: %v\n", path, err)
	}
	// If the file doesn't exist (os.IsNotExist), it will be created on the first Push.

	return lane
}

// Push appends an EscalationItem to the file (JSONL: one JSON object per line)
// and to the in-memory state. The mutex makes Push atomic; the line is never
// interleaved with another goroutine's write or read.
func (f *FileEscalationLane) Push(item EscalationItem) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Append to in-memory copy.
	f.items = append(f.items, item)

	// Write to the file (JSONL: one JSON object per line).
	// Open the file in append mode; create it if it doesn't exist.
	file, err := os.OpenFile(f.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// If append fails, remove the item from in-memory state to maintain consistency.
		f.items = f.items[:len(f.items)-1]
		return fmt.Errorf("open file for append: %w", err)
	}
	defer file.Close()

	// Marshal the item to JSON.
	b, err := json.Marshal(item)
	if err != nil {
		// If marshaling fails, remove the item from in-memory state.
		f.items = f.items[:len(f.items)-1]
		return fmt.Errorf("marshal item: %w", err)
	}

	// Append a newline to make it JSONL.
	b = append(b, '\n')

	// Write the line (atomic write under the mutex).
	if _, err := file.Write(b); err != nil {
		// If write fails, remove the item from in-memory state.
		f.items = f.items[:len(f.items)-1]
		return fmt.Errorf("write to file: %w", err)
	}

	// Sync the file to disk (fsync). This ensures the escalation is durable against
	// power loss (AC-5: escalations survive Mac power-off). Write() alone leaves data in
	// the kernel page cache; Sync() forces it to the filesystem.
	if err := file.Sync(); err != nil {
		// If Sync fails, roll back the in-memory append and return the error.
		f.items = f.items[:len(f.items)-1]
		return fmt.Errorf("sync to disk: %w", err)
	}

	return nil
}

// List returns the lane's items in FIFO order (a defensive copy).
// Thread-safe: the mutex protects the in-memory list.
func (f *FileEscalationLane) List() []EscalationItem {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]EscalationItem, len(f.items))
	copy(out, f.items)
	return out
}
