package usage

import (
	"testing"
	"time"
)

const sampleTranscriptLine = `{"type":"assistant","timestamp":"2026-06-25T22:44:04.198Z","uuid":"u1","message":{"id":"msg_01Wk","model":"claude-opus-4-8","usage":{"input_tokens":20260,"cache_creation_input_tokens":14744,"cache_read_input_tokens":15874,"output_tokens":383}}}`

func TestParseClaudeTranscriptLine(t *testing.T) {
	ev, ok := ParseClaudeTranscriptLine([]byte(sampleTranscriptLine))
	if !ok {
		t.Fatal("ok=false, want a usage event")
	}
	if ev.Provider != "anthropic" || ev.Source != SourceInteractive {
		t.Fatalf("provider/source wrong: %+v", ev)
	}
	if ev.TurnID != "msg_01Wk" || ev.Model != "claude-opus-4-8" {
		t.Fatalf("id/model wrong: %+v", ev)
	}
	if ev.InputTokens != 20260 || ev.CacheCreationTokens != 14744 || ev.CacheReadTokens != 15874 || ev.OutputTokens != 383 {
		t.Fatalf("tokens wrong: %+v", ev)
	}
	want := time.Date(2026, 6, 25, 22, 44, 4, 198000000, time.UTC)
	if !ev.Ts.Equal(want) {
		t.Fatalf("ts=%v want %v", ev.Ts, want)
	}
}

func TestParseClaude_NonAssistantIgnored(t *testing.T) {
	if _, ok := ParseClaudeTranscriptLine([]byte(`{"type":"user","message":{"content":"hi"}}`)); ok {
		t.Fatal("user line should be ignored")
	}
	if _, ok := ParseClaudeTranscriptLine([]byte(`{bad json`)); ok {
		t.Fatal("garbage line should be ignored, not panic")
	}
}

func TestParseClaudeStreamLine_UsesNow(t *testing.T) {
	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	line := `{"type":"assistant","message":{"id":"msg_stream","model":"claude-opus-4-8","usage":{"input_tokens":5,"output_tokens":7}}}`
	ev, ok := ParseClaudeStreamLine([]byte(line), now)
	if !ok {
		t.Fatal("ok=false")
	}
	if !ev.Ts.Equal(now) || ev.TurnID != "msg_stream" || ev.OutputTokens != 7 {
		t.Fatalf("stream parse wrong: %+v", ev)
	}
}
