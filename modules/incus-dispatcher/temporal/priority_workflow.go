package temporal

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// RescoreSignalName is the name of the rescore signal that can be sent to a PriorityWorkflow.
const RescoreSignalName = "rescore"

// CurrentImportanceQuery is the name of the query that exposes the workflow's live currentImportance.
// Used by operators (tests, E1 cluster) to verify that a rescore signal was processed and accepted/rejected.
// Query handler returns the current Importance value (updated by rescore signals or aging).
const CurrentImportanceQuery = "currentImportance"

// RescoreSignal is the payload for a rescore request sent to a PriorityWorkflow.
// It contains the actor making the request and the proposed new importance level.
type RescoreSignal struct {
	Actor              Actor
	ProposedImportance Importance
}

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

// nextCheckInterval returns the durable-timer interval for the next aging/escalation re-check:
// a quarter of the time remaining, clamped to [1 minute, 6 hours].
// Used by PriorityWorkflow and EscalationWorkflow to determine when to recompute projections.
func nextCheckInterval(remaining time.Duration) time.Duration {
	interval := remaining / 4
	if interval < time.Minute {
		return time.Minute
	}
	if interval > 6*time.Hour {
		return 6 * time.Hour
	}
	return interval
}

// PriorityWorkflowInput is the input to the PriorityWorkflow.
type PriorityWorkflowInput struct {
	DirectiveID string
	Importance  Importance
	Deadline    *time.Time
}

// ReprojectRequest is the input to the ReprojectActivity.
// Only DirectiveID, Importance, and NotBefore are used by ReprojectActivity.Defer/Reprioritize.
type ReprojectRequest struct {
	DirectiveID string
	Importance  Importance
	NotBefore   time.Time
}

// PriorityWorkflow is a durable Temporal workflow that ages a directive's priority
// over time and writes the result into laneq through the ReprojectActivity sole-writer seam.
// The workflow ALSO handles rescore signals from external actors (human or agent).
//
// The workflow:
// 1. Takes serializable input (DirectiveID, Importance, Deadline).
// 2. Uses workflow.Now(ctx) for ALL time reads (never time.Now()).
// 3. Uses the pure-Go projection functions (ComputeUrgency, ComputeQuadrant, ComputeEffectivePriority).
// 4. Maintains a mutable currentImportance (initialized from input.Importance; updated on successful rescore).
// 5. Loops: compute the current quadrant using currentImportance; use a Selector to wait on BOTH:
//   - an aging timer (next re-projection point), or
//   - an incoming rescore signal.
//     On timer: recompute urgency→quadrant→effective-priority; if the projection changed,
//     invoke ReprojectActivity to persist it.
//     On signal: validate the rescore with ValidateRescoreRequest. If allowed (human or agent within bounds),
//     update currentImportance, recompute projection, and invoke ReprojectActivity (same sole-writer seam).
//     If NOT allowed (agent out of bounds): reject silently (no write); escalation routing is deferred to C4.
//
// 6. Terminate the loop once the item reaches Q1 or the deadline has passed.
//
// Linked to STORY-0043 (urgency + quadrant aging), STORY-0044 (sole-writer seam),
// STORY-0047 (human rescore), SCENARIO-0057/0082 (agent-bounded rescore),
// and SCENARIO-0094 (rescore signal path).
func PriorityWorkflow(ctx workflow.Context, input PriorityWorkflowInput) error {
	// Set activity options with a reasonable timeout for gRPC calls to laneq
	ctx = workflow.WithActivityOptions(ctx, workflowActivityOptions)
	// Get the current workflow time (deterministic, not time.Now())
	now := workflow.Now(ctx)

	// If no deadline, there's nothing to age. Q4 items (no deadline) remain idle-only forever.
	if input.Deadline == nil {
		return nil
	}

	// Initialize currentImportance from input; this is MUTABLE and updated by rescore signals.
	currentImportance := input.Importance

	// Compute the initial quadrant at workflow start time.
	// This is the directive's starting projection; we track changes from this point.
	initialUrgency := ComputeUrgency(input.Deadline, now)
	lastQuadrant := ComputeQuadrant(currentImportance, initialUrgency)
	lastEffectivePriority := ComputeEffectivePriority(currentImportance, lastQuadrant)

	// Allocate the activity reference once (reused for all activity invocations in the loop).
	activities := &Activities{}

	// Set up the rescore signal channel (non-buffered; we'll consume it in the loop).
	rescoreSignalChannel := workflow.GetSignalChannel(ctx, RescoreSignalName)

	// Register a query handler to expose the live currentImportance.
	// This allows external actors (operators, E1 cluster) to verify that a rescore signal
	// was processed and whether it was accepted (importance changed) or rejected (stayed same).
	// The handler closes over the mutable currentImportance variable.
	if err := workflow.SetQueryHandler(ctx, CurrentImportanceQuery, func() (Importance, error) {
		return currentImportance, nil
	}); err != nil {
		return err
	}

	for {
		// Recompute urgency and quadrant at the current workflow time
		urgency := ComputeUrgency(input.Deadline, now)
		quadrant := ComputeQuadrant(currentImportance, urgency)
		effectivePriority := ComputeEffectivePriority(currentImportance, quadrant)

		// If the projection changed (quadrant or priority), invoke the ReprojectActivity
		if quadrant != lastQuadrant || effectivePriority != lastEffectivePriority {
			// Set notBefore to the current workflow time (make item eligible now).
			notBefore := now

			// Invoke the sole-writer activity to persist the projection.
			req := ReprojectRequest{
				DirectiveID: input.DirectiveID,
				Importance:  currentImportance,
				NotBefore:   notBefore,
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
		timeRemaining := input.Deadline.Sub(now)
		nextCheckDuration := nextCheckInterval(timeRemaining)

		// Create a cancellable timer context for the next aging check.
		// If a rescore signal fires first, we'll cancel the timer to prevent
		// unbounded history accumulation in long-lived workflows.
		timerCtx, cancelTimer := workflow.WithCancel(ctx)
		timerFuture := workflow.NewTimer(timerCtx, nextCheckDuration)

		// Use a Selector to wait on BOTH the aging timer and incoming rescore signals.
		// The selector fires when EITHER the timer expires OR a signal arrives.
		// We'll loop the selector until one of these happens, then process the result.
		selector := workflow.NewSelector(ctx)
		selector.AddFuture(timerFuture, func(f workflow.Future) {
			// Timer fired; we'll advance the workflow's "now" and continue the loop.
		})

		// Add the rescore signal channel to the selector. When a signal arrives,
		// we'll process the rescore request.
		selector.AddReceive(rescoreSignalChannel, func(c workflow.ReceiveChannel, more bool) {
			if !more {
				// Channel closed (unlikely in normal operation, but handle it gracefully).
				return
			}

			// Receive the signal (non-blocking because the selector triggered).
			var signal RescoreSignal
			c.Receive(ctx, &signal)

			// Validate the rescore request with the current importance and deadline.
			allowed, escalationRequired, err := ValidateRescoreRequest(
				signal.Actor, currentImportance, signal.ProposedImportance, input.Deadline,
			)

			if allowed {
				// Rescore is allowed; update currentImportance and the workflow will
				// recompute the projection on the next loop iteration.
				currentImportance = signal.ProposedImportance
				// NOTE: The re-projection happens at the top of the loop.
				// No immediate activity invocation here; we let the loop detect the change.
			} else if escalationRequired {
				// Rescore was out of bounds and requires approval/escalation.
				// Log the reason for C4's escalation handler and live operators to see.
				workflow.GetLogger(ctx).Warn("rescore rejected: escalation required",
					"directiveID", input.DirectiveID, "actor", signal.Actor.ID,
					"current", currentImportance, "proposed", signal.ProposedImportance, "reason", err)
				// TODO(ITER-0008b): route the rejected agent rescore to the operator approval
				// queue / escalation lane (STORY-0047 AC-3) — operator/TUI scope. ITER-0007b's
				// time-plane escalation (stale re-raise + retry backoff) is the EscalationWorkflow
				// / RetryWorkflow; this approval-routing belongs to the ITER-0008 coordinator.
			} else {
				// Other validation errors; treat as rejection (no write).
			}
			// Loop will continue and check for projection changes on next iteration.
		})

		// Wait for either the timer or a signal (whichever comes first).
		selector.Select(ctx)

		// Cancel the aging timer if it hasn't fired yet.
		// No-op if the timer already fired (selector won due to timer completion).
		// Prevents unbounded history accumulation when signals frequently win the race.
		// NOTE: For very-long-lived directives with high signal volume, the production mitigation
		// is workflow.ContinueAsNew when workflow.GetInfo(ctx).GetContinueAsNewSuggested() is true.
		// That's an E1/live concern — not in C3 scope.
		cancelTimer()

		// Advance the workflow's "now" for the next iteration.
		now = workflow.Now(ctx)
	}
}
