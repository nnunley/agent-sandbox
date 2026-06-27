package usage

import (
	"testing"
	"time"
)

func TestEstimate_UsedAndWindowVisibleUncalibrated(t *testing.T) {
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	evs := []UsageEvent{
		{Provider: "anthropic", OutputTokens: 100, Ts: t0, Source: SourceInteractive},
		{Provider: "anthropic", InputTokens: 50, Ts: t0.Add(30 * time.Minute), Source: SourceFleet},
		{Provider: "openai", OutputTokens: 999, Ts: t0, Source: SourceFleet}, // other provider ignored
	}
	now := t0.Add(time.Hour)
	est := Estimator{}.Estimate(evs, nil, "anthropic", Window5h, now)

	if est.Used != 150 {
		t.Fatalf("Used=%d want 150 (only anthropic in window)", est.Used)
	}
	if !est.WindowAnchor.Equal(t0) || !est.WindowReset.Equal(t0.Add(5*time.Hour)) || !est.WindowActive {
		t.Fatalf("window facts wrong: %+v", est)
	}
	if est.Confidence != ConfUncalibrated {
		t.Fatalf("Confidence=%s want uncalibrated (no limits)", est.Confidence)
	}
	if est.CeilingEst != 0 {
		t.Fatalf("CeilingEst=%d want 0 when uncalibrated", est.CeilingEst)
	}
}

func TestEstimate_ExpiredWindowUsedZero(t *testing.T) {
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	evs := []UsageEvent{{Provider: "anthropic", OutputTokens: 100, Ts: t0}}
	now := t0.Add(6 * time.Hour) // idle past expiry
	est := Estimator{}.Estimate(evs, nil, "anthropic", Window5h, now)
	if est.WindowActive {
		t.Fatal("want window not active")
	}
	if est.Used != 0 {
		t.Fatalf("Used=%d want 0 (no active window)", est.Used)
	}
}
