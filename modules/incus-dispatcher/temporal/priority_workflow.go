package temporal

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// workflowActivityOptions sets sensible defaults for activities in the priority workflow.
// Start-to-close timeout of 30s is sufficient for gRPC calls to laneq.
// RetryPolicy ensures transient laneq gRPC errors are retried, making scheduling writes durable.
var workflowActivityOptions = workflow.ActivityOptions{
	StartToCloseTimeout: 30 * time.Second,
	RetryPolicy: &temporal.RetryPolicy{
		InitialInterval:    1 * time.Second,
		BackoffCoefficient: 2.0,
		MaximumInterval:    1 * time.Minute,
		// MaximumAttempts = 0 (default) means unlimited retries with exponential backoff.
		// This ensures sole-writer Reprioritize/Defer RPCs survive transient laneq unavailability.
	},
}

// PriorityWorkflowInput is the input to the PriorityWorkflow.
type PriorityWorkflowInput struct {
	DirectiveID string
	Importance  Importance
	Deadline    *time.Time
}

// ReprojectRequest is the input to the ReprojectActivity.
type ReprojectRequest struct {
	DirectiveID      string
	Importance       Importance
	Quadrant         Quadrant
	EffectivePriority int
	NotBefore        time.Time
}

// PriorityWorkflow is a durable Temporal workflow that ages a directive's priority
// over time and writes the result into laneq through the ReprojectActivity sole-writer seam.
//
// The workflow:
// 1. Takes serializable input (DirectiveID, Importance, Deadline).
// 2. Uses workflow.Now(ctx) for ALL time reads (never time.Now()).
// 3. Uses the pure-Go projection functions (ComputeUrgency, ComputeQuadrant, ComputeEffectivePriority).
// 4. Loops: compute the current quadrant; sleep on a DURABLE Temporal timer until the next
//    re-projection point (a sensible interval before the deadline, or until the deadline);
//    when the timer fires, recompute urgency→quadrant→effective-priority; if the projection
//    changed, invoke ReprojectActivity to persist it.
// 5. Terminate the loop once the item reaches Q1 or the deadline has passed.
//
// Linked to STORY-0043 (urgency + quadrant aging) and STORY-0044 (sole-writer seam).
func PriorityWorkflow(ctx workflow.Context, input PriorityWorkflowInput) error {
	// Set activity options with a reasonable timeout for gRPC calls to laneq
	ctx = workflow.WithActivityOptions(ctx, workflowActivityOptions)
	// Get the current workflow time (deterministic, not time.Now())
	now := workflow.Now(ctx)

	// If no deadline, there's nothing to age. Q4 items (no deadline) remain idle-only forever.
	if input.Deadline == nil {
		return nil
	}

	// Compute the initial quadrant at workflow start time.
	// This is the directive's starting projection; we track changes from this point.
	initialUrgency := ComputeUrgency(input.Deadline, now)
	lastQuadrant := ComputeQuadrant(input.Importance, initialUrgency)
	lastEffectivePriority := ComputeEffectivePriority(input.Importance, lastQuadrant)

	// Allocate the activity reference once (reused for all activity invocations in the loop).
	activities := &Activities{}

	for {
		// Recompute urgency and quadrant at the current workflow time
		urgency := ComputeUrgency(input.Deadline, now)
		quadrant := ComputeQuadrant(input.Importance, urgency)
		effectivePriority := ComputeEffectivePriority(input.Importance, quadrant)

		// If the projection changed (quadrant or priority), invoke the ReprojectActivity
		if quadrant != lastQuadrant || effectivePriority != lastEffectivePriority {
			// Set notBefore to the current workflow time (make item eligible now).
			notBefore := now

			// Invoke the sole-writer activity to persist the projection.
			// Use workflow.ExecuteActivity with the activity struct method reference.
			// The activity is registered as Activities.ReprojectActivity.
			req := ReprojectRequest{
				DirectiveID:       input.DirectiveID,
				Importance:        input.Importance,
				Quadrant:          quadrant,
				EffectivePriority: effectivePriority,
				NotBefore:         notBefore,
			}

			err := workflow.ExecuteActivity(ctx, activities.ReprojectActivity, req).Get(ctx, nil)
			if err != nil {
				return fmt.Errorf("reprojection failed: %w", err)
			}

			lastQuadrant = quadrant
			lastEffectivePriority = effectivePriority
		}

		// Exit if we've reached Q1 and the deadline is still in the future
		// (items in Q1 are ready to run; we don't need to reschedule them further)
		if quadrant == QuadrantQ1 {
			// Item is now in the top quadrant; stop aging
			return nil
		}

		// Exit if deadline has passed
		if now.After(*input.Deadline) || now.Equal(*input.Deadline) {
			return nil
		}

		// Calculate the next re-projection point
		// Strategy: check again at 1/4 of the time remaining, bounded between 1 minute and 6 hours.
		// In production with live Temporal, the 6-hour cap is reasonable; testsuite auto-advances
		// timers so the duration doesn't affect test runtime.
		timeRemaining := input.Deadline.Sub(now)
		nextCheckDuration := timeRemaining / 4
		if nextCheckDuration < time.Minute {
			nextCheckDuration = time.Minute
		}
		if nextCheckDuration > 6*time.Hour {
			// For very far-out deadlines, check every 6 hours
			nextCheckDuration = 6 * time.Hour
		}

		// Sleep on a durable Temporal timer (deterministic, auto-advanced in tests)
		err := workflow.Sleep(ctx, nextCheckDuration)
		if err != nil {
			return fmt.Errorf("timer failed: %w", err)
		}

		// Advance the workflow's "now" for the next iteration
		now = workflow.Now(ctx)
	}
}
