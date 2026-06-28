package usage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUsageEvent_TotalAndJSON(t *testing.T) {
	ts := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	e := UsageEvent{
		Provider: "anthropic", Model: "claude-opus-4-8",
		InputTokens: 20260, CacheCreationTokens: 14744, CacheReadTokens: 15874, OutputTokens: 383,
		Ts: ts, Source: SourceInteractive, TurnID: "msg_01Wk",
	}
	if got, want := e.Total(), int64(20260+14744+15874+383); got != want {
		t.Fatalf("Total()=%d want %d", got, want)
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back UsageEvent
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Total() != e.Total() || back.TurnID != e.TurnID || back.Source != SourceInteractive {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
}

func TestLimitEvent_JSON(t *testing.T) {
	le := LimitEvent{Provider: "anthropic", WindowClass: "5h", UsedAt: 1_200_000, Ts: time.Unix(100, 0).UTC()}
	b, _ := json.Marshal(le)
	var back LimitEvent
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.UsedAt != le.UsedAt || back.WindowClass != "5h" {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
}
