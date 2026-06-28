package usage

import (
	"testing"
	"time"
)

func ev(min int) UsageEvent { // event at T0 + min minutes
	return UsageEvent{Provider: "anthropic", OutputTokens: 1, Ts: time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC).Add(time.Duration(min) * time.Minute)}
}

func TestCurrentWindow_ContinuousStaysAnchored(t *testing.T) {
	evs := []UsageEvent{ev(0), ev(60), ev(120)} // all within 5h of T0
	now := ev(150).Ts
	anchor, reset, active := CurrentWindow(evs, Window5h, now)
	if !anchor.Equal(ev(0).Ts) {
		t.Fatalf("anchor=%v want %v", anchor, ev(0).Ts)
	}
	if !reset.Equal(ev(0).Ts.Add(5 * time.Hour)) {
		t.Fatalf("reset=%v", reset)
	}
	if !active {
		t.Fatal("want active (now within window)")
	}
}

func TestCurrentWindow_IdlePastExpiryReanchors(t *testing.T) {
	// ev(0), then a gap > 5h: ev(360) is 6h after T0 → re-anchors at ev(360).
	evs := []UsageEvent{ev(0), ev(360)}
	now := ev(370).Ts
	anchor, _, active := CurrentWindow(evs, Window5h, now)
	if !anchor.Equal(ev(360).Ts) {
		t.Fatalf("anchor=%v want re-anchored %v", anchor, ev(360).Ts)
	}
	if !active {
		t.Fatal("want active (now within re-anchored window)")
	}
}

func TestCurrentWindow_SubExpiryGapDoesNotReanchor(t *testing.T) {
	// gap of 4h (< 5h) keeps the original anchor.
	evs := []UsageEvent{ev(0), ev(240)}
	now := ev(250).Ts
	anchor, _, active := CurrentWindow(evs, Window5h, now)
	if !anchor.Equal(ev(0).Ts) {
		t.Fatalf("anchor=%v want %v (no re-anchor)", anchor, ev(0).Ts)
	}
	if !active {
		t.Fatal("want active")
	}
}

func TestCurrentWindow_ExpiredWhenIdleNow(t *testing.T) {
	evs := []UsageEvent{ev(0)}
	now := ev(301).Ts // 5h1m after anchor → expired, no new event yet
	_, _, active := CurrentWindow(evs, Window5h, now)
	if active {
		t.Fatal("want NOT active (window expired, idle)")
	}
}

func TestCurrentWindow_NoEvents(t *testing.T) {
	_, _, active := CurrentWindow(nil, Window5h, ev(0).Ts)
	if active {
		t.Fatal("want NOT active with no events")
	}
}
