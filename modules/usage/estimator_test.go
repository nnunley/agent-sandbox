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

func TestEstimate_CalibratesFromExhaustion(t *testing.T) {
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	evs := []UsageEvent{{Provider: "anthropic", OutputTokens: 800_000, Ts: t0}}
	// One exhaustion at 1.2M → that IS the realized ceiling for the 5h window.
	limits := []LimitEvent{{Provider: "anthropic", WindowClass: "5h", UsedAt: 1_200_000, Ts: t0.Add(-time.Hour)}}
	now := t0.Add(time.Hour)
	est := Estimator{}.Estimate(evs, limits, "anthropic", Window5h, now)

	if est.CeilingEst != 1_200_000 {
		t.Fatalf("CeilingEst=%d want 1200000", est.CeilingEst)
	}
	if est.RemainingEst != 1_200_000-800_000 {
		t.Fatalf("RemainingEst=%d want 400000", est.RemainingEst)
	}
	if est.Confidence != ConfLow {
		t.Fatalf("Confidence=%s want low (1 point)", est.Confidence)
	}
}

func TestEstimate_MostRecentCalibrationWins(t *testing.T) {
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	limits := []LimitEvent{
		{Provider: "anthropic", WindowClass: "5h", UsedAt: 1_000_000, Ts: t0.Add(-48 * time.Hour)},
		{Provider: "anthropic", WindowClass: "5h", UsedAt: 1_500_000, Ts: t0.Add(-24 * time.Hour)}, // newer
	}
	est := Estimator{}.Estimate(nil, limits, "anthropic", Window5h, t0)
	if est.CeilingEst != 1_500_000 {
		t.Fatalf("CeilingEst=%d want most-recent 1500000", est.CeilingEst)
	}
	if est.Confidence != ConfLow { // 2 points → still low
		t.Fatalf("Confidence=%s want low", est.Confidence)
	}
}

func TestEstimate_PublishedPriorWhenNoCalibration(t *testing.T) {
	t0 := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	evs := []UsageEvent{{Provider: "anthropic", OutputTokens: 100, Ts: t0}}
	e := Estimator{PublishedPrior: map[string]int64{"anthropic": 2_000_000}}
	est := e.Estimate(evs, nil, "anthropic", Window5h, t0.Add(time.Minute))
	if est.CeilingEst != 2_000_000 || est.RemainingEst != 2_000_000-100 {
		t.Fatalf("prior not applied: %+v", est)
	}
	if est.Confidence != ConfLow { // prior → low, not uncalibrated
		t.Fatalf("Confidence=%s want low (prior)", est.Confidence)
	}
}
