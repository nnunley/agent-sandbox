package usage

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestReport_ShowsUsedAndUncalibrated(t *testing.T) {
	dir := t.TempDir()
	l, _ := OpenLedger(dir + "/usage.jsonl")
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	_ = l.Append(UsageEvent{Provider: "anthropic", OutputTokens: 100, Ts: t0, Source: SourceInteractive})

	var buf bytes.Buffer
	Report(l, Estimator{}, t0.Add(time.Hour), &buf)
	out := buf.String()
	if !strings.Contains(out, "anthropic") || !strings.Contains(out, "uncalibrated") {
		t.Fatalf("missing provider/uncalibrated label:\n%s", out)
	}
	if !strings.Contains(out, "100") { // used tokens visible immediately
		t.Fatalf("used tokens not shown:\n%s", out)
	}
}

func TestReport_ShowsRemainingWhenCalibrated(t *testing.T) {
	dir := t.TempDir()
	l, _ := OpenLedger(dir + "/usage.jsonl")
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	_ = l.Append(UsageEvent{Provider: "anthropic", OutputTokens: 800000, Ts: t0})
	_ = l.AppendLimit(LimitEvent{Provider: "anthropic", WindowClass: "5h", UsedAt: 1200000, Ts: t0.Add(-time.Hour)})

	var buf bytes.Buffer
	Report(l, Estimator{}, t0.Add(time.Hour), &buf)
	if !strings.Contains(buf.String(), "400000") { // 1.2M - 800k remaining
		t.Fatalf("remaining not shown:\n%s", buf.String())
	}
}
