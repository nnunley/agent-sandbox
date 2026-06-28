package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLedger_AppendReopenDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ts := time.Unix(1000, 0).UTC()
	if err := l.Append(UsageEvent{Provider: "anthropic", OutputTokens: 5, Ts: ts, Source: SourceInteractive, TurnID: "a"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := l.AppendLimit(LimitEvent{Provider: "anthropic", WindowClass: "5h", UsedAt: 999, Ts: ts}); err != nil {
		t.Fatalf("appendLimit: %v", err)
	}

	// Reopen a SECOND ledger over the same file: it must reconstruct both records (durability).
	l2, err := OpenLedger(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if ev := l2.Events(); len(ev) != 1 || ev[0].TurnID != "a" {
		t.Fatalf("events after reopen = %+v, want 1 with TurnID a", ev)
	}
	if lim := l2.Limits(); len(lim) != 1 || lim[0].UsedAt != 999 {
		t.Fatalf("limits after reopen = %+v, want 1 with UsedAt 999", lim)
	}
}

func TestLedger_SkipsCorruptLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	l, _ := OpenLedger(path)
	_ = l.Append(UsageEvent{Provider: "anthropic", OutputTokens: 1, Ts: time.Unix(1, 0).UTC()})
	// Append a garbage line directly, then a good one.
	appendRawLine(t, path, "{not json")
	l3, _ := OpenLedger(path)
	if err := l3.Append(UsageEvent{Provider: "openai", OutputTokens: 2, Ts: time.Unix(2, 0).UTC()}); err != nil {
		t.Fatalf("append after corrupt: %v", err)
	}
	l4, err := OpenLedger(path)
	if err != nil {
		t.Fatalf("reopen after corrupt: %v", err) // corrupt line must NOT be fatal
	}
	if got := len(l4.Events()); got != 2 {
		t.Fatalf("events = %d, want 2 (corrupt line skipped)", got)
	}
}

func appendRawLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("write raw: %v", err)
	}
}
