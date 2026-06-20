package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

// WorkingState is a projection of worker activity derived from events.jsonl.
type WorkingState struct {
	Alive         bool          // now - LastModified <= aliveWindow
	EventCount    int           // total parsed JSONL event lines
	SinceLast     time.Duration // now - lastModified
	LastShellTool string        // "ctx_shell" | "Bash" | "" (none)
	LastShellCmd  string        // most recent shell command (ctx_shell preferred over Bash)
	LastRead      string        // most recent read target (ctx_read preferred over Read)
	PhaseGuess    string        // "compile" | "oracle" | "regress" | "" (unknown)
}

// ProjectWorkingState parses events.jsonl bytes and projects worker activity.
// lastModified is the events.jsonl file's mod time (approximates the last event time, since
// claude appends as it works); now and aliveWindow drive Alive/SinceLast.
func ProjectWorkingState(events []byte, lastModified, now time.Time, aliveWindow time.Duration) WorkingState {
	sinceLast := now.Sub(lastModified)
	ws := WorkingState{
		Alive:     sinceLast <= aliveWindow,
		SinceLast: sinceLast,
	}

	// Track preferred (ctx_*) and fallback per tool category separately.
	var (
		ctxShellCmd  string
		bashCmd      string
		hasCtxShell  bool
		hasBash      bool
		ctxReadPath  string
		readPath     string
		hasCtxRead   bool
		hasRead      bool
	)

	scanner := bufio.NewScanner(bytes.NewReader(events))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		ws.EventCount++

		// Only assistant messages carry tool_use content.
		typVal, ok := raw["type"]
		if !ok {
			continue
		}
		var typStr string
		if err := json.Unmarshal(typVal, &typStr); err != nil || typStr != "assistant" {
			continue
		}

		msgVal, ok := raw["message"]
		if !ok {
			continue
		}
		var msg struct {
			Content []struct {
				Type  string          `json:"type"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		}
		if err := json.Unmarshal(msgVal, &msg); err != nil {
			continue
		}

		for _, item := range msg.Content {
			if item.Type != "tool_use" {
				continue
			}

			normalized := normalizeToolName(item.Name)

			switch normalized {
			case "ctx_shell":
				var inp struct {
					Command string `json:"command"`
				}
				if err := json.Unmarshal(item.Input, &inp); err == nil {
					ctxShellCmd = inp.Command
					hasCtxShell = true
				}
			case "Bash":
				var inp struct {
					Command string `json:"command"`
				}
				if err := json.Unmarshal(item.Input, &inp); err == nil {
					bashCmd = inp.Command
					hasBash = true
				}
			case "ctx_read":
				var inp struct {
					FilePath string `json:"file_path"`
				}
				if err := json.Unmarshal(item.Input, &inp); err == nil {
					ctxReadPath = inp.FilePath
					hasCtxRead = true
				}
			case "Read":
				var inp struct {
					FilePath string `json:"file_path"`
				}
				if err := json.Unmarshal(item.Input, &inp); err == nil {
					readPath = inp.FilePath
					hasRead = true
				}
			}
		}
	}

	// Apply preference rules: ctx_* over plain fallback.
	if hasCtxShell {
		ws.LastShellTool = "ctx_shell"
		ws.LastShellCmd = ctxShellCmd
	} else if hasBash {
		ws.LastShellTool = "Bash"
		ws.LastShellCmd = bashCmd
	}

	if hasCtxRead {
		ws.LastRead = ctxReadPath
	} else if hasRead {
		ws.LastRead = readPath
	}

	ws.PhaseGuess = guessPhase(ws.LastShellCmd)
	return ws
}

// normalizeToolName takes the segment after the last "__" in a tool name.
// "mcp__lean-ctx__ctx_shell" → "ctx_shell"; "Bash" → "Bash".
func normalizeToolName(name string) string {
	if i := strings.LastIndex(name, "__"); i >= 0 {
		return name[i+2:]
	}
	return name
}

// guessPhase maps a shell command to a phase label. Checked in priority order.
func guessPhase(cmd string) string {
	switch {
	case strings.Contains(cmd, "go build"):
		return "compile"
	case strings.Contains(cmd, "go test") && strings.Contains(cmd, "pkg/ir"):
		return "oracle"
	case strings.Contains(cmd, "make check-generated") || strings.Contains(cmd, "go test ./..."):
		return "regress"
	default:
		return ""
	}
}
