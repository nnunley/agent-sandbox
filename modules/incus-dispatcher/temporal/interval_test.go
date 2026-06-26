package temporal

import (
	"testing"
	"time"
)

// TestNextCheckInterval validates the durable-timer interval clamping logic.
func TestNextCheckInterval(t *testing.T) {
	tests := []struct {
		remaining   time.Duration
		expected    time.Duration
		description string
	}{
		// Very long: clamped to 6 hours
		{365 * 24 * time.Hour, 6 * time.Hour, "1 year: clamped to 6h"},
		{30 * 24 * time.Hour, 6 * time.Hour, "30 days: clamped to 6h"},
		{24 * time.Hour, 6 * time.Hour, "1 day: clamped to 6h"},

		// Moderate: 1/4 of remaining (not clamped)
		{10 * time.Hour, 2*time.Hour + 30*time.Minute, "10h: 1/4 = 2.5h"},
		{4 * time.Hour, time.Hour, "4h: 1/4 = 1h"},

		// Boundary: 1/4 = 1 minute (not clamped)
		{4 * time.Minute, time.Minute, "4m: 1/4 = 1m (at lower boundary)"},

		// Very short: clamped to 1 minute
		{3 * time.Minute, time.Minute, "3m: 1/4 = 45s, clamped to 1m"},
		{2 * time.Minute, time.Minute, "2m: 1/4 = 30s, clamped to 1m"},
		{1 * time.Minute, time.Minute, "1m: 1/4 = 15s, clamped to 1m"},
		{30 * time.Second, time.Minute, "30s: clamped to 1m"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := nextCheckInterval(tt.remaining)
			if result != tt.expected {
				t.Errorf("nextCheckInterval(%v) = %v, want %v",
					tt.remaining, result, tt.expected)
			}
			t.Logf("✓ %v → %v", tt.remaining, result)
		})
	}
}

// TestNextCheckIntervalBoundaries validates clamping at 1m and 6h boundaries.
func TestNextCheckIntervalBoundaries(t *testing.T) {
	// Lower boundary: 1/4 < 1m → clamped to 1m
	if interval := nextCheckInterval(3 * time.Minute); interval != time.Minute {
		t.Errorf("3m should clamp to 1m, got %v", interval)
	}

	// Lower boundary edge: 1/4 = 1m (no clamp)
	if interval := nextCheckInterval(4 * time.Minute); interval != time.Minute {
		t.Errorf("4m should be 1m, got %v", interval)
	}

	// Upper boundary: 1/4 > 6h → clamped to 6h
	if interval := nextCheckInterval(25 * time.Hour); interval != 6*time.Hour {
		t.Errorf("25h should clamp to 6h, got %v", interval)
	}

	// Upper boundary edge: 1/4 = 6h (no clamp)
	if interval := nextCheckInterval(24 * time.Hour); interval != 6*time.Hour {
		t.Errorf("24h should be 6h, got %v", interval)
	}

	t.Logf("✓ Boundaries validated: [1m, 6h]")
}
