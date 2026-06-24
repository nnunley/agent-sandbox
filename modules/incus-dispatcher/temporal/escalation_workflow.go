package temporal

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"
)

// EscalationWorkflowInput is the input to the EscalationWorkflow.
// It holds a directive that is currently in an escalation state (e.g., pending approval)
// and monitors its urgency over time. When the urgency crosses the escalation threshold,
// the workflow re-raises the directive's effective priority and logs the transition.
type EscalationWorkflowInput struct {
	DirectiveID string
	Importance  Importance
	Deadline    *time.Time
}

// EscalationWorkflow is a durable Temporal workflow that holds a pending escalation
// and re-raises its priority as urgency increases over time.
//
// The workflow:
// 1. Takes a directive that has a deadline and a current importance tier.
// 2. Periodically checks if the escalation threshold has been crossed (IsEscalationTriggered).
// 3. When the threshold crosses, recomputes the quadrant (ReprojectOnEscalation),
//    invokes the ReprojectActivity to persist the updated priority to laneq,
//    and logs the transition using workflow.GetLogger (deterministic, auditable).
// 4. Exits after re-raising OR when the deadline has passed.
//
// This workflow proves AC-B (STORY-0061 AC-3 / STORY-0055 AC-7):
// - Stale escalations are re-raised as urgency rises (autonomous, durable).
// - Ladder transitions are logged for D6 decision log consumption.
// - No human action required; Temporal's durable timers drive the re-raise.
//
// Linked to STORY-0061 (escalation re-raise), STORY-0055 (ladder logging),
// and SCENARIO-0087 (operator workflow / stale escalation resurfaced by rising urgency).
func EscalationWorkflow(ctx workflow.Context, input EscalationWorkflowInput) error {
	// Set activity options with a reasonable timeout for gRPC calls to laneq
	ctx = workflow.WithActivityOptions(ctx, workflowActivityOptions)

	// Get the current workflow time (deterministic, not time.Now())
	now := workflow.Now(ctx)

	// If no deadline, there's nothing to age. No escalation window.
	if input.Deadline == nil {
		return nil
	}

	// Initialize the last-known quadrant to detect transitions.
	lastQuadrant, _, err := ReprojectOnEscalation(input.Importance, input.Deadline, now)
	if err != nil {
		return fmt.Errorf("initial escalation reprojection failed: %w", err)
	}

	// Allocate the activity reference once (reused for all activity invocations in the loop).
	activities := &Activities{}

	// Log the initial state
	workflow.GetLogger(ctx).Info("escalation workflow started",
		"directiveID", input.DirectiveID,
		"importance", input.Importance,
		"deadline", input.Deadline.Format(time.RFC3339),
		"initialQuadrant", lastQuadrant)

	for {
		// Recompute urgency and quadrant at the current workflow time
		quadrant, priority, err := ReprojectOnEscalation(input.Importance, input.Deadline, now)
		if err != nil {
			return fmt.Errorf("escalation reprojection failed: %w", err)
		}

		// If the quadrant changed (escalation triggered and raised urgency), invoke the activity
		if quadrant != lastQuadrant {
			// NOTE(ITER-0008): This re-raise is AUTONOMOUS + time-driven (driven by urgency crossing threshold,
			// logged via workflow.GetLogger). The coordinator audit log should distinguish these autonomous
			// re-raises from operator-triggered (TUI) re-raises.
			// Log the ladder transition (deterministic, for D6 decision log)
			workflow.GetLogger(ctx).Warn("escalation re-raised: quadrant transition",
				"directiveID", input.DirectiveID,
				"from", lastQuadrant,
				"to", quadrant,
				"priority", priority,
				"now", now.Format(time.RFC3339),
				"deadline", input.Deadline.Format(time.RFC3339))

			// Set notBefore to the current workflow time (make item eligible now at new priority)
			notBefore := now

			// Invoke the sole-writer activity to persist the re-raised priority
			req := ReprojectRequest{
				DirectiveID: input.DirectiveID,
				Importance:  input.Importance,
				NotBefore:   notBefore,
			}

			err := workflow.ExecuteActivity(ctx, activities.ReprojectActivity, req).Get(ctx, nil)
			if err != nil {
				return fmt.Errorf("escalation reprojection activity failed: %w", err)
			}

			lastQuadrant = quadrant
		}

		// Exit if deadline has passed or if we've reached Q1
		// (Q1 means the escalation is now urgent enough to run; stop re-raising)
		if now.After(*input.Deadline) || now.Equal(*input.Deadline) || quadrant == QuadrantQ1 {
			workflow.GetLogger(ctx).Info("escalation workflow exiting",
				"directiveID", input.DirectiveID,
				"finalQuadrant", lastQuadrant,
				"deadlinePassed", now.After(*input.Deadline) || now.Equal(*input.Deadline))
			return nil
		}

		// Calculate the next re-check point
		timeRemaining := input.Deadline.Sub(now)
		nextCheckDuration := nextCheckInterval(timeRemaining)

		// Sleep until the next check point
		err = workflow.Sleep(ctx, nextCheckDuration)
		if err != nil {
			return fmt.Errorf("escalation workflow sleep failed: %w", err)
		}

		// Advance the workflow's "now" for the next iteration
		now = workflow.Now(ctx)
	}
}
