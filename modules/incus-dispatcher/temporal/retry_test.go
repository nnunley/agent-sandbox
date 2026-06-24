package temporal

import (
	"testing"
	"time"
)

// TestRetryBackoff validates exponential backoff schedule with a cap.
func TestRetryBackoff(t *testing.T) {
	tests := []struct {
		attempt        int
		expectedBackoff time.Duration
		description     string
	}{
		{0, 1 * time.Second, "attempt 0: base 1s"},
		{1, 2 * time.Second, "attempt 1: 2^1 = 2s"},
		{2, 4 * time.Second, "attempt 2: 2^2 = 4s"},
		{3, 8 * time.Second, "attempt 3: 2^3 = 8s"},
		{4, 16 * time.Second, "attempt 4: 2^4 = 16s"},
		{5, 32 * time.Second, "attempt 5: 2^5 = 32s"},
		{6, 1 * time.Minute, "attempt 6: 2^6 = 64s, capped at 60s"},
		{7, 1 * time.Minute, "attempt 7: capped at 60s"},
		{10, 1 * time.Minute, "attempt 10: capped at 60s"},
		{20, 1 * time.Minute, "attempt 20: capped at 60s"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			backoff := RetryBackoff(tt.attempt)
			if backoff != tt.expectedBackoff {
				t.Errorf("RetryBackoff(%d) = %v, want %v", tt.attempt, backoff, tt.expectedBackoff)
			}
			t.Logf("✓ Attempt %d: %v", tt.attempt, backoff)
		})
	}
}

// TestRetryBackoffMonotonic validates that backoff is monotonically non-decreasing.
func TestRetryBackoffMonotonic(t *testing.T) {
	const maxAttempt = 20
	var prevBackoff time.Duration

	for attempt := 0; attempt <= maxAttempt; attempt++ {
		backoff := RetryBackoff(attempt)
		if backoff < prevBackoff {
			t.Errorf("RetryBackoff(%d) = %v < previous %v; not monotonic", attempt, backoff, prevBackoff)
		}
		prevBackoff = backoff
	}
	t.Logf("✓ Backoff is monotonically non-decreasing over attempts 0-%d", maxAttempt)
}

// TestRetryBackoffCapped validates that all backoffs respect the max cap.
func TestRetryBackoffCapped(t *testing.T) {
	const maxRetryBackoff = 1 * time.Minute

	for attempt := 0; attempt <= 100; attempt++ {
		backoff := RetryBackoff(attempt)
		if backoff > maxRetryBackoff {
			t.Errorf("RetryBackoff(%d) = %v > max %v; cap not enforced", attempt, backoff, maxRetryBackoff)
		}
	}
	t.Logf("✓ All backoffs are capped at or below 60s")
}
