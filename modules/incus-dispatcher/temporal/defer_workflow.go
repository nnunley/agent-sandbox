package temporal

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"
)

// DeferWorkflowInput is the input to the DeferWorkflow.
// It specifies a directive to defer until a specific time (future-work holding).
type DeferWorkflowInput struct {
	DirectiveID string
	NotBefore   time.Time // The time at which the directive becomes eligible
	Importance  Importance
}

// DeferWorkflow is a minimal durable Temporal workflow that holds a directive's eligibility
// until a specified time arrives.
//
// The workflow:
// 1. Takes a directive ID and a not-before time.
// 2. Uses a durable Temporal timer to wait until that time (or until the timer fires).
// 3. When the timer fires (and notBefore <= now), invokes ReprojectActivity with Defer
//    to mark the directive as eligible (notBefore <= current time).
// 4. Exits, allowing the directive to be claimed once eligible.
//
// This workflow proves AC-C (STORY-0002 AC-2): deferred work is held in Temporal until eligible,
// not prematurely eligible in laneq.
//
// Linked to STORY-0002 (durable defer-until-eligible) and SCENARIO-0115 (future-work holding).
func DeferWorkflow(ctx workflow.Context, input DeferWorkflowInput) error {
	// Set activity options with a reasonable timeout for gRPC calls to laneq
	ctx = workflow.WithActivityOptions(ctx, workflowActivityOptions)

	// Get the current workflow time (deterministic, not time.Now())
	now := workflow.Now(ctx)

	// Calculate how long to sleep until the not-before time
	timeUntilEligible := input.NotBefore.Sub(now)

	// If already eligible (or very close), mark eligible immediately and return
	if timeUntilEligible <= 0 {
		// Already eligible; invoke the activity to persist the current time as notBefore
		activities := &Activities{}
		req := ReprojectRequest{
			DirectiveID: input.DirectiveID,
			Importance:  input.Importance,
			NotBefore:   now,
		}
		err := workflow.ExecuteActivity(ctx, activities.ReprojectActivity, req).Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("defer activity failed: %w", err)
		}
		return nil
	}

	// Sleep until the not-before time arrives
	// NOTE(ITER-0008): For far-future not-before times, the single long-lived workflow's history
	// is a live-Temporal concern. The production mitigation is workflow.NewContinueAsNewError
	// when GetContinueAsNewSuggested() — an E1/live concern, not implemented here.
	err := workflow.Sleep(ctx, timeUntilEligible)
	if err != nil {
		return fmt.Errorf("defer sleep failed: %w", err)
	}

	// Advance the workflow's "now" for the activity invocation
	now = workflow.Now(ctx)

	// Invoke the sole-writer activity to mark the directive as eligible at the current time
	activities := &Activities{}
	req := ReprojectRequest{
		DirectiveID: input.DirectiveID,
		Importance:  input.Importance,
		NotBefore:   now, // Set notBefore to current time (now eligible)
	}

	err = workflow.ExecuteActivity(ctx, activities.ReprojectActivity, req).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("defer activity failed: %w", err)
	}

	return nil
}
