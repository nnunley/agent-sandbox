package usage

import "testing"

func TestFleetCap(t *testing.T) {
	cases := []struct {
		name    string
		ceiling int64
		reserve float64
		wantCap int64
	}{
		{"no ceiling -> no cap", 0, 0.30, 0},
		{"30pct reserve", 1_000_000, 0.30, 700_000},
		{"zero reserve -> full ceiling", 1_000_000, 0.0, 1_000_000},
		{"all reserved -> zero cap", 1_000_000, 1.0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := Estimate{CeilingEst: c.ceiling}
			if got := e.FleetCap(c.reserve); got != c.wantCap {
				t.Fatalf("FleetCap=%d want %d", got, c.wantCap)
			}
		})
	}
}

func TestAllowFleet(t *testing.T) {
	// Uncalibrated, no prior -> fail open regardless of Used.
	if !(Estimate{CeilingEst: 0, Used: 9_999_999}).AllowFleet(0.30) {
		t.Fatal("uncalibrated must fail open (allow)")
	}
	// ceiling 1_000_000, reserve 0.30 -> cap 700_000.
	if !(Estimate{CeilingEst: 1_000_000, Used: 699_999}).AllowFleet(0.30) {
		t.Fatal("Used below cap must allow")
	}
	if (Estimate{CeilingEst: 1_000_000, Used: 700_000}).AllowFleet(0.30) {
		t.Fatal("Used at cap must deny")
	}
	if (Estimate{CeilingEst: 1_000_000, Used: 800_000}).AllowFleet(0.30) {
		t.Fatal("Used over cap must deny")
	}
}
