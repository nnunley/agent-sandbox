package usage

import (
	"testing"
	"time"
)

func TestParseAnthropicUsage(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-8","usage":{"input_tokens":120,"cache_creation_input_tokens":5,"cache_read_input_tokens":7,"output_tokens":33}}`)
	now := time.Date(2026, 6, 26, 11, 0, 0, 0, time.UTC)
	ev, ok := ParseAnthropicUsage("anthropic", body, now)
	if !ok {
		t.Fatal("ok=false")
	}
	if ev.Source != SourceFleet || ev.Provider != "anthropic" || ev.Model != "claude-opus-4-8" {
		t.Fatalf("header fields wrong: %+v", ev)
	}
	if ev.InputTokens != 120 || ev.CacheCreationTokens != 5 || ev.CacheReadTokens != 7 || ev.OutputTokens != 33 {
		t.Fatalf("tokens wrong: %+v", ev)
	}
	if !ev.Ts.Equal(now) {
		t.Fatalf("ts=%v want now", ev.Ts)
	}
}

func TestParseAnthropicUsage_NoUsageIgnored(t *testing.T) {
	if _, ok := ParseAnthropicUsage("anthropic", []byte(`{"error":"x"}`), time.Now()); ok {
		t.Fatal("response without usage should be ignored")
	}
}
