package temporal

import (
	"time"
)

// EscalationThreshold returns the duration before deadline at which escalation is triggered.
// Escalation window is importance-dependent:
// - Critical (Tier 3): 7 days before deadline
// - High (Tier 2): 5 days before deadline
// - Medium (Tier 1): 3 days before deadline
// - Low (Tier 0): 1 day before deadline
func EscalationThreshold(importance Importance) time.Duration {
	switch importance {
	case ImportanceCritical:
		return 7 * 24 * time.Hour
	case ImportanceHigh:
		return 5 * 24 * time.Hour
	case ImportanceMedium:
		return 3 * 24 * time.Hour
	case ImportanceLow:
		return 1 * 24 * time.Hour
	default:
		return 1 * 24 * time.Hour // Default to 1 day
	}
}

// IsEscalationTriggered checks if the escalation window has opened.
// Returns true if the time remaining until deadline is less than the threshold.
//
// If deadline is nil, returns false (no escalation for items without deadlines).
// If deadline has passed, returns true (escalation always triggered for overdue items).
func IsEscalationTriggered(importance Importance, deadline *time.Time, now time.Time) bool {
	if deadline == nil {
		return false // No escalation for items without deadlines
	}

	timeRemaining := deadline.Sub(now)
	threshold := EscalationThreshold(importance)

	// If deadline has passed or is within the threshold window
	return timeRemaining <= threshold
}

// ReprojectOnEscalation recomputes the quadrant after escalation is triggered.
// When escalation threshold is crossed, items move to higher quadrants (more urgent).
//
// Logic:
// - Item in Q2 moves to Q1 when escalation triggers (deadline approaches)
// - Item in Q3 stays in Q3 (already urgent)
// - Item in Q4 can move to Q3 if deadline becomes urgent enough
//
// Returns (newQuadrant, newPriority, error).
func ReprojectOnEscalation(importance Importance, deadline *time.Time, now time.Time) (Quadrant, int, error) {
	if deadline == nil {
		// No escalation for items without deadlines
		return ComputeQuadrant(importance, 0), ComputeEffectivePriority(importance, ComputeQuadrant(importance, 0)), nil
	}

	// Compute current urgency and quadrant
	urgency := ComputeUrgency(deadline, now)
	quadrant := ComputeQuadrant(importance, urgency)

	// Compute priority after quadrant assignment
	priority := ComputeEffectivePriority(importance, quadrant)

	return quadrant, priority, nil
}
