package temporal

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"
)

// RetryBackoff computes the backoff duration for a retry attempt using exponential backoff.
// Formula: base * 2^attempt, capped at MaxRetryBackoff.
//
// Backoff schedule (1-second base, 1-minute cap):
//
//	Attempt 0: 1s
//	Attempt 1: 2s
//	Attempt 2: 4s
//	Attempt 3: 8s
//	Attempt 4: 16s
//	Attempt 5: 32s
//	Attempt 6+: 60s (capped)
//
// The backoff increases exponentially to spread retries over time,
// reducing thundering herd during transient failures.
// The cap prevents unbounded growth for many retries.
func RetryBackoff(attempt int) time.Duration {
	const (
		baseBackoff     = 1 * time.Second
		maxRetryBackoff = 1 * time.Minute
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

// RetryWorkflowInput is the input to the RetryWorkflow.
// It specifies a directive to retry with exponential backoff.
type RetryWorkflowInput struct {
	DirectiveID string
	Importance  Importance
	Attempt     int // Current attempt number (0-indexed)
	MaxAttempts int // Maximum retries allowed (0 = infinite)
}

// RetryWorkflow is a durable Temporal workflow that implements retry logic
// with exponential backoff via the sole-writer seam.
//
// The workflow:
// 1. Takes a directive, its current attempt count, and max attempts.
// 2. For each attempt from current to maxAttempts:
//   - Computes notBefore = now + RetryBackoff(attempt) using the pure helper
//   - Invokes ReprojectActivity.Defer to re-push the directive to laneq
//   - Logs the re-push event via workflow.GetLogger (deterministic, for D6)
//   - Sleeps until notBefore, then advances to next attempt
//
// 3. Exits when maxAttempts is reached (further escalation is ITER-0008 work)
//
// This workflow proves AC-A (STORY-0058 AC-24): durable re-push with exponential backoff
// via the sole-writer seam. Each retry is deferred (not immediately eligible), preventing
// thundering herd and giving transient failures time to resolve.
//
// Linked to STORY-0058 AC-24 (retry backoff re-push), JOURNEY-0001 (complete one-shot
// lifecycle with retry seam), and SCENARIO-0115 (durable retry re-push with exponential backoff).
func RetryWorkflow(ctx workflow.Context, input RetryWorkflowInput) error {
	// Set activity options with a reasonable timeout for gRPC calls to laneq
	ctx = workflow.WithActivityOptions(ctx, workflowActivityOptions)

	// Get the current workflow time (deterministic, not time.Now())
	now := workflow.Now(ctx)

	// Allocate the activity reference once (reused for all activity invocations in the loop).
	activities := &Activities{}

	// Log workflow start
	workflow.GetLogger(ctx).Info("retry workflow started",
		"directiveID", input.DirectiveID,
		"currentAttempt", input.Attempt,
		"maxAttempts", input.MaxAttempts)

	// Retry loop: from current attempt up to maxAttempts
	for attempt := input.Attempt; input.MaxAttempts == 0 || attempt < input.MaxAttempts; attempt++ {
		// Compute the backoff duration for this attempt
		backoff := RetryBackoff(attempt)
		notBefore := now.Add(backoff)

		// Log the re-push (deterministic, for D6 decision log)
		workflow.GetLogger(ctx).Info("retry re-push with backoff",
			"directiveID", input.DirectiveID,
			"attempt", attempt,
			"backoffDuration", backoff,
			"notBefore", notBefore.Format(time.RFC3339),
			"now", now.Format(time.RFC3339))

		// Invoke the sole-writer activity to defer the directive
		req := ReprojectRequest{
			DirectiveID: input.DirectiveID,
			Importance:  input.Importance,
			NotBefore:   notBefore, // Defer until backoff elapses
		}

		err := workflow.ExecuteActivity(ctx, activities.ReprojectActivity, req).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("retry re-push activity failed at attempt %d: %w", attempt, err)
		}

		// Sleep until the backoff duration expires (durable timer, workflow.Now() only)
		err = workflow.Sleep(ctx, backoff)
		if err != nil {
			return fmt.Errorf("retry sleep failed at attempt %d: %w", attempt, err)
		}

		// Advance the workflow's "now" for the next iteration
		now = workflow.Now(ctx)

		// Log the retry wake-up and advance to next attempt
		workflow.GetLogger(ctx).Info("retry backoff elapsed, advancing",
			"directiveID", input.DirectiveID,
			"attemptCompleted", attempt,
			"nextAttempt", attempt+1,
			"now", now.Format(time.RFC3339))
	}

	// Max attempts reached
	workflow.GetLogger(ctx).Warn("retry workflow exhausted max attempts",
		"directiveID", input.DirectiveID,
		"maxAttempts", input.MaxAttempts)

	// TODO(ITER-0008b): on retry exhaustion, hand off to the escalation ladder
	// (STORY-0058 AC-3 stronger-worker rung) via the coordinator.
	// Current behavior: the directive remains in laneq and becomes eligible again when its
	// not-before passes (the daemon/coordinator re-claims it). It is not silently held forever.

	return nil
}
