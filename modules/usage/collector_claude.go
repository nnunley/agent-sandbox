package usage

import (
	"encoding/json"
	"time"
)

// claudeRecord matches the subset of a Claude Code assistant record (transcript or stream-json)
// the meter needs. Unknown fields are ignored.
type claudeRecord struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens         int64 `json:"input_tokens"`
			CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadTokens     int64 `json:"cache_read_input_tokens"`
			OutputTokens        int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// parseClaude builds a UsageEvent from a Claude record. ts is used when the record has no
// usable timestamp (streaming). Returns ok=false for non-assistant or usage-less records.
func parseClaude(line []byte, ts time.Time) (UsageEvent, bool) {
	var r claudeRecord
	if err := json.Unmarshal(line, &r); err != nil {
		return UsageEvent{}, false
	}
	if r.Type != "assistant" || r.Message.ID == "" {
		return UsageEvent{}, false
	}
	u := r.Message.Usage
	if u.InputTokens == 0 && u.CacheCreationTokens == 0 && u.CacheReadTokens == 0 && u.OutputTokens == 0 {
		return UsageEvent{}, false
	}
	when := ts
	if r.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339, r.Timestamp); err == nil {
			when = parsed.UTC()
		}
	}
	return UsageEvent{
		Provider:            "anthropic",
		Model:               r.Message.Model,
		InputTokens:         u.InputTokens,
		CacheCreationTokens: u.CacheCreationTokens,
		CacheReadTokens:     u.CacheReadTokens,
		OutputTokens:        u.OutputTokens,
		Ts:                  when,
		Source:              SourceInteractive,
		TurnID:              r.Message.ID,
	}, true
}

// ParseClaudeTranscriptLine parses one ~/.claude/projects/**/*.jsonl line (durable record).
func ParseClaudeTranscriptLine(line []byte) (UsageEvent, bool) {
	return parseClaude(line, time.Time{})
}

// ParseClaudeStreamLine parses one Claude Code stream-json output line; now stamps it.
func ParseClaudeStreamLine(line []byte, now time.Time) (UsageEvent, bool) {
	return parseClaude(line, now)
}
