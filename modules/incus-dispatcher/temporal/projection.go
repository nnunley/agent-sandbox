package temporal

import (
	"fmt"
	"math"
	"time"
)

// Importance represents the importance tier of a directive (0-3).
// 0=Low, 1=Medium, 2=High, 3=Critical
type Importance int

const (
	ImportanceLow      Importance = 0
	ImportanceMedium   Importance = 1
	ImportanceHigh     Importance = 2
	ImportanceCritical Importance = 3
)

// Urgency represents the urgency of a directive (0.0 to ~1.0+).
// 0.0 = not urgent, 1.0+ = deadline passed
type Urgency float64

// Quadrant represents the Eisenhower matrix quadrant.
// Q1: Important + Urgent (do now)
// Q2: Important + Not Urgent (schedule)
// Q3: Not Important + Urgent (delegate/soon)
// Q4: Not Important + Not Urgent (idle-only)
type Quadrant int

const (
	QuadrantQ1 Quadrant = iota + 1
	QuadrantQ2
	QuadrantQ3
	QuadrantQ4
)

// String returns the string representation of a Quadrant.
func (q Quadrant) String() string {
	switch q {
	case QuadrantQ1:
		return "Q1"
	case QuadrantQ2:
		return "Q2"
	case QuadrantQ3:
		return "Q3"
	case QuadrantQ4:
		return "Q4"
	default:
		return fmt.Sprintf("Q%d", q)
	}
}

// ImportanceStringToTier converts queue.Importance string values ("high", "normal", "low")
// to temporal.Importance tier values (0-3).
// This bridges the queue package's string-based importance with the temporal package's
// tier-based importance used for Eisenhower quadrant mapping (STORY-0040 AC-1).
//
// Mapping:
// - "critical" -> ImportanceCritical (3)
// - "high"     -> ImportanceHigh (2)
// - "normal"   -> ImportanceMedium (1)
// - "low"      -> ImportanceLow (0)
//
// Returns an error for unknown or empty strings.
func ImportanceStringToTier(importance string) (Importance, error) {
	switch importance {
	case "critical":
		return ImportanceCritical, nil
	case "high":
		return ImportanceHigh, nil
	case "normal":
		return ImportanceMedium, nil
	case "low":
		return ImportanceLow, nil
	default:
		return -1, fmt.Errorf("unknown importance string: %q", importance)
	}
}

// ComputeUrgency determines urgency from a deadline and current time.
// Returns 0.0 if deadline is nil or far in the future.
// Increases monotonically as deadline approaches.
// Returns >= 1.0 if deadline has passed.
//
// Urgency formula: uses a sigmoid curve that:
// - returns 0 for nil deadline
// - ramps from 0 to 1 as deadline approaches (with steeper curve near deadline)
// - returns >= 1 when deadline has passed
//
// Sigmoid parameters (a=0.2, b=1.0):
// - Calibrated to reach urgency ~0.5 at approximately 5 days remaining
// - Ensures smooth, monotonic urgency increase as deadlines approach
// - Urgency thresholds: 0.0 (far future), 0.5 (soon, ~5 days), 1.0+ (deadline passed)
// - Linked to STORY-0043 AC-1 requirement: urgency must be monotonic
func ComputeUrgency(deadline *time.Time, now time.Time) Urgency {
	if deadline == nil {
		return 0.0
	}

	// Time remaining until deadline
	timeRemaining := deadline.Sub(now)

	// If deadline has passed, return >= 1.0
	if timeRemaining <= 0 {
		// Scale past-deadline duration to keep it somewhat bounded
		daysPast := math.Abs(timeRemaining.Hours() / 24)
		// Even far-past deadlines don't exceed 2.0
		return Urgency(math.Min(1.0+daysPast/30, 2.0))
	}

	// For future deadlines, use a sigmoid curve that increases as deadline approaches
	// At 30 days out, urgency is ~0.01
	// At 14 days out, urgency is ~0.14
	// At 7 days out, urgency is ~0.40
	// At 5 days out, urgency is ~0.50 (boundary for Q2 -> Q1 transition)
	// At 3 days out, urgency is ~0.60
	// At 2 days out, urgency is ~0.65
	// At 1 day out, urgency is ~0.69
	// At 0 hours, urgency is ~0.73

	daysRemaining := timeRemaining.Hours() / 24.0

	// Sigmoid formula: urgency = 1 / (1 + e^(a*daysRemaining - b))
	// This sigmoid increases urgency as daysRemaining decreases
	// We use a=0.2 and b=1.0 to get the desired curve
	// Sigmoid guarantees output in (0, 1) for finite daysRemaining,
	// so no clamping is needed for future deadlines.
	urgency := 1.0 / (1.0 + math.Exp(0.2*daysRemaining-1.0))

	return Urgency(urgency)
}

// ComputeQuadrant determines the Eisenhower quadrant from importance and urgency.
// Q1: importance >= 2 (High or Critical) AND urgency >= 0.5
// Q2: importance >= 2 (High or Critical) AND urgency < 0.5
// Q3: importance < 2 (Low or Medium) AND urgency >= 0.5
// Q4: importance < 2 (Low or Medium) AND urgency < 0.5
func ComputeQuadrant(importance Importance, urgency Urgency) Quadrant {
	isImportant := int(importance) >= 2
	isUrgent := float64(urgency) >= 0.5

	switch {
	case isImportant && isUrgent:
		return QuadrantQ1
	case isImportant && !isUrgent:
		return QuadrantQ2
	case !isImportant && isUrgent:
		return QuadrantQ3
	default:
		return QuadrantQ4
	}
}

// ComputeEffectivePriority maps (importance, quadrant) to a priority score.
// Higher scores = higher priority.
// Priority formula: quadrant_score * 1000 + importance_score
// This ensures quadrant is the primary sort key, importance is secondary.
func ComputeEffectivePriority(importance Importance, quadrant Quadrant) int {
	// Quadrant scores: Q1=4, Q2=3, Q3=2, Q4=1
	quadrantScore := 5 - int(quadrant)

	// Importance scores: 0-3
	importanceScore := int(importance)

	// Combined score: quadrant dominates, importance is a tiebreaker
	// Formula: quadrantScore * 1000 + importanceScore
	// This ensures Q1 importance=0 > Q2 importance=3
	return quadrantScore*1000 + importanceScore
}
