package main

import "testing"

func TestNextRung(t *testing.T) {
	cases := []struct {
		attempts int
		want     Rung
	}{
		{0, RungRetrySame},      // AC-2: transient fail → retry same
		{1, RungStrongerWorker}, // AC-3: repeats → stronger worker
		{2, RungHardTier},       // AC-4: still failing → bigger/hard-tier
		{3, RungHuman},          // AC-5: authority/judgment limit → human
		{9, RungHuman},          // saturates at human
		{-1, RungRetrySame},     // defensive: treat negatives as the first rung
	}
	for _, c := range cases {
		if got := nextRung(c.attempts); got != c.want {
			t.Errorf("nextRung(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}

func TestRungString(t *testing.T) {
	want := map[Rung]string{
		RungRetrySame:      "retry-same",
		RungStrongerWorker: "stronger-worker",
		RungHardTier:       "hard-tier",
		RungHuman:          "human",
	}
	for r, s := range want {
		if r.String() != s {
			t.Errorf("Rung(%d).String() = %q, want %q", r, r.String(), s)
		}
	}
}

// Pre-approved rungs (0..2) climb autonomously; only RungHuman is the human lane.
func TestRungAutonomy(t *testing.T) {
	for _, r := range []Rung{RungRetrySame, RungStrongerWorker, RungHardTier} {
		if !r.Autonomous() {
			t.Errorf("rung %v should be autonomous (pre-approved)", r)
		}
	}
	if RungHuman.Autonomous() {
		t.Errorf("RungHuman must NOT be autonomous (requires the human lane)")
	}
}
