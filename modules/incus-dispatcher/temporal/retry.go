package temporal

import (
	"time"
)

// RetryBackoff computes the backoff duration for a retry attempt using exponential backoff.
// Formula: base * 2^attempt, capped at MaxRetryBackoff.
//
// Backoff schedule (1-second base, 1-minute cap):
//   Attempt 0: 1s
//   Attempt 1: 2s
//   Attempt 2: 4s
//   Attempt 3: 8s
//   Attempt 4: 16s
//   Attempt 5: 32s
//   Attempt 6+: 60s (capped)
//
// The backoff increases exponentially to spread retries over time,
// reducing thundering herd during transient failures.
// The cap prevents unbounded growth for many retries.
func RetryBackoff(attempt int) time.Duration {
	const (
		baseBackoff      = 1 * time.Second
		maxRetryBackoff  = 1 * time.Minute
	)

	// Exponential backoff: base * 2^attempt
	// For large attempts, this can overflow; use a loop to avoid overflow.
	backoff := baseBackoff
	for i := 0; i < attempt && backoff < maxRetryBackoff; i++ {
		backoff *= 2
		if backoff > maxRetryBackoff {
			backoff = maxRetryBackoff
		}
	}

	if backoff > maxRetryBackoff {
		return maxRetryBackoff
	}
	return backoff
}
