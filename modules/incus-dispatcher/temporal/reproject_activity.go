package temporal

import (
	"context"
	"fmt"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// Reprojector is the minimal sole-writer interface for writing scheduling fields to laneq.
// It defines the ONLY two RPC paths that Temporal uses to mutate laneq scheduling state:
// Reprioritize (update priority via importance mapping) and Defer (update not-before eligibility).
//
// This interface is the core of the sole-writer seam contract (STORY-0044 AC-3, SCENARIO-0093):
// Only the Temporal workflow role calls these methods. The interface enables testing via
// injection of a fake Reprojector while the real implementation is *queue.LaneqQueue.
type Reprojector interface {
	// Reprioritize changes the priority of a directive by ID.
	// Used to update directive priority based on urgency changes.
	Reprioritize(id string, importance queue.Importance) error

	// Defer sets the not-before eligibility time for a directive by ID.
	// Defers the directive until the specified time (UTC).
	Defer(id string, notBefore time.Time) error
}

// Activities holds the dependencies for Temporal activities.
// It is structured to allow dependency injection (real *queue.LaneqQueue in production,
// a fake Reprojector in tests).
type Activities struct {
	Queue Reprojector
}

// ReprojectActivity is the sole-writer activity that persists priority/urgency changes
// from the PriorityWorkflow into laneq.
//
// This is the ONLY path that calls laneq Reprioritize/Defer. The activity enforces
// the sole-writer discipline: the workflow computes projections, but ONLY the activity
// (registered with the Temporal worker) can write to laneq.
//
// Parameters:
//   - ctx: activity context (for timeouts, cancellation)
//   - req: ReprojectRequest containing the directive ID, computed quadrant, priority, and notBefore time
//
// Returns:
//   - nil if both Reprioritize and Defer succeed
//   - error if either RPC fails
//
// Invariants:
// - This activity MUST be the only code that calls Queue.Reprioritize and Queue.Defer.
// - The workflow must NOT call these methods directly (enforced by testsuite assertion).
// - Reprioritize is called with the importance tier corresponding to the computed quadrant.
// - Defer is called with notBefore set to the eligibility time (now for Q1, etc.).
func (a *Activities) ReprojectActivity(ctx context.Context, req ReprojectRequest) error {
	if a.Queue == nil {
		return fmt.Errorf("reproject activity: queue dependency is nil")
	}

	// Map the computed quadrant/importance to a queue.Importance for Reprioritize.
	// This bridge converts temporal.Importance tier (0-3) back to queue.Importance strings.
	// The exact mapping depends on the quadrant and importance level.
	// For now, use a simple strategy: pass through the importance tier.
	// In production, this could be refined based on the quadrant and strategic priority rules.
	queueImportance := tierToQueueImportance(req.Importance)

	// Call Reprioritize to update the directive's priority in laneq
	if err := a.Queue.Reprioritize(req.DirectiveID, queueImportance); err != nil {
		return fmt.Errorf("reproject: reprioritize failed for %s: %w", req.DirectiveID, err)
	}

	// Call Defer to set the not-before eligibility time
	if err := a.Queue.Defer(req.DirectiveID, req.NotBefore); err != nil {
		return fmt.Errorf("reproject: defer failed for %s: %w", req.DirectiveID, err)
	}

	return nil
}

// tierToQueueImportance converts a temporal Importance tier (0-3) to a queue.Importance string.
// This is the inverse of ImportanceStringToTier.
func tierToQueueImportance(tier Importance) queue.Importance {
	switch tier {
	case ImportanceCritical:
		return queue.ImportanceHigh // Map critical → high (queue has no critical)
	case ImportanceHigh:
		return queue.ImportanceHigh
	case ImportanceMedium:
		return queue.ImportanceNormal
	case ImportanceLow:
		return queue.ImportanceLow
	default:
		return queue.ImportanceNormal
	}
}
