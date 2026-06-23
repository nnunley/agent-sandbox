package temporal

import (
	"testing"
	"time"
)

// TestScenario0078 validates quadrant transitions and Q4 stability across an 8-day timeline.
//
// SCENARIO-0078: Quadrant Transitions and Q4 Stability
// Validates the temporal projection behavior with:
// - D1-Q1: Critical importance, 2-day deadline (always Q1)
// - D2-Q2: High importance, 8-day deadline (Q2 initially, ages to Q1 by day 5)
// - D3-Q3: Low importance, 2-day deadline (always Q3)
// - D4-Q4: Low importance, no deadline (must NEVER leave Q4)
// - D5-Q2→Q1: High importance, 6-day deadline (Q2→Q1 transition by day 2)
//
// Key invariants tested:
// - Q1: Important + Urgent (do now)
// - Q2: Important + Not Urgent (schedule)
// - Q3: Not Important + Urgent (delegate/soon)
// - Q4: Not Important + Not Urgent (idle-only, STABLE regardless of time)
// - Monotonic urgency increase as deadlines approach
// - Q4 stability: once in Q4 (no deadline + low importance), always stays Q4
func TestScenario0078(t *testing.T) {
	// Day 0: June 23, 2026 (reference point)
	day0 := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	// Define directives with their properties
	directives := map[string]struct {
		importance Importance
		deadline   *time.Time
		name       string
	}{
		"D1-Q1": {
			importance: ImportanceCritical, // 3
			deadline:   ptrTime(day0.AddDate(0, 0, 2)), // June 25
			name:       "D1-Q1: Critical + 2-day deadline",
		},
		"D2-Q2": {
			importance: ImportanceHigh, // 2
			deadline:   ptrTime(day0.AddDate(0, 0, 8)), // July 1
			name:       "D2-Q2: High + 8-day deadline",
		},
		"D3-Q3": {
			importance: ImportanceLow, // 0
			deadline:   ptrTime(day0.AddDate(0, 0, 2)), // June 25
			name:       "D3-Q3: Low + 2-day deadline",
		},
		"D4-Q4": {
			importance: ImportanceLow, // 0
			deadline:   nil, // No deadline
			name:       "D4-Q4: Low + no deadline (MUST STAY Q4)",
		},
		"D5-Q2→Q1": {
			importance: ImportanceHigh, // 2
			deadline:   ptrTime(day0.AddDate(0, 0, 6)), // June 29
			name:       "D5-Q2→Q1: High + 6-day deadline (Q2→Q1 transition)",
		},
	}

	// Test checkpoints: (dayOffset, expectedQuadrants)
	// Expectations based on actual urgency computations
	checkpoints := []struct {
		dayOffset    int
		expectations map[string]Quadrant
	}{
		{
			dayOffset: 0,
			expectations: map[string]Quadrant{
				"D1-Q1": QuadrantQ1,      // 2 days out: urgency ~0.646 (Q1)
				"D2-Q2": QuadrantQ2,      // 8 days out: urgency ~0.354 (Q2)
				"D3-Q3": QuadrantQ3,      // 2 days out: urgency ~0.646 (Q3)
				"D4-Q4": QuadrantQ4,      // nil deadline, always Q4
				"D5-Q2→Q1": QuadrantQ2,   // 6 days out: urgency ~0.450 (Q2)
			},
		},
		{
			dayOffset: 2,
			expectations: map[string]Quadrant{
				"D1-Q1": QuadrantQ1,      // deadline TODAY: urgency = 1.0 (Q1)
				"D2-Q2": QuadrantQ2,      // 6 days out: urgency ~0.45 (Q2)
				"D3-Q3": QuadrantQ3,      // deadline TODAY: urgency = 1.0 (Q3)
				"D4-Q4": QuadrantQ4,      // MUST REMAIN Q4
				"D5-Q2→Q1": QuadrantQ1,   // 4 days out: urgency ~0.55 (Q1, crosses 0.5)
			},
		},
		{
			dayOffset: 5,
			expectations: map[string]Quadrant{
				"D1-Q1": QuadrantQ1,      // 3 days PAST: urgency ~1.1 (Q1)
				"D2-Q2": QuadrantQ1,      // 3 days out: urgency ~0.599 (Q1, crosses 0.5)
				"D3-Q3": QuadrantQ3,      // 3 days PAST: urgency ~1.1 (Q3)
				"D4-Q4": QuadrantQ4,      // STILL IN Q4, NEVER MOVES
				"D5-Q2→Q1": QuadrantQ1,   // 1 day out: urgency ~0.690 (Q1)
			},
		},
		{
			dayOffset: 7,
			expectations: map[string]Quadrant{
				"D1-Q1": QuadrantQ1,      // 5 days PAST: urgency ~1.167 (Q1)
				"D2-Q2": QuadrantQ1,      // 1 day out: urgency ~0.690 (Q1)
				"D3-Q3": QuadrantQ3,      // 5 days PAST: urgency ~1.167 (Q3)
				"D4-Q4": QuadrantQ4,      // STILL Q4: NEVER PROMOTED
				"D5-Q2→Q1": QuadrantQ1,   // deadline TODAY: urgency ~1.033 (Q1)
			},
		},
		{
			dayOffset: 8,
			expectations: map[string]Quadrant{
				"D1-Q1": QuadrantQ1,      // 6 days PAST: urgency ~1.2 (Q1)
				"D2-Q2": QuadrantQ1,      // deadline PASSED: urgency = 1.0 (Q1)
				"D3-Q3": QuadrantQ3,      // 6 days PAST: urgency ~1.2 (Q3)
				"D4-Q4": QuadrantQ4,      // STILL Q4: NEVER PROMOTED, INDEFINITE STABILITY
				"D5-Q2→Q1": QuadrantQ1,   // 1 day PAST: urgency ~1.067 (Q1)
			},
		},
	}

	// Helper to track last urgency for monotonicity check
	previousUrgencies := make(map[string]Urgency)

	// Test each checkpoint
	for _, checkpoint := range checkpoints {
		currentTime := day0.AddDate(0, 0, checkpoint.dayOffset)

		t.Logf("\n=== Day %d (%s) ===", checkpoint.dayOffset, currentTime.Format("2006-01-02"))

		for directiveID, directive := range directives {
			expectation := checkpoint.expectations[directiveID]

			// Compute urgency and quadrant
			urgency := ComputeUrgency(directive.deadline, currentTime)
			quadrant := ComputeQuadrant(directive.importance, urgency)

			// Log for debugging
			deadlineStr := "nil"
			if directive.deadline != nil {
				deadlineStr = directive.deadline.Format("2006-01-02")
			}
			t.Logf("  %s (deadline: %s) → Q%v (urgency: %.3f)",
				directiveID, deadlineStr, quadrant, urgency)

			// Assert quadrant matches expectation
			if quadrant != expectation {
				t.Errorf(
					"Day %d, %s: got quadrant %v, want %v (urgency: %.3f, importance: %d)",
					checkpoint.dayOffset,
					directiveID,
					quadrant,
					expectation,
					urgency,
					directive.importance,
				)
			}

			// Assert monotonic urgency increase for deadlined items
			if directive.deadline != nil {
				if prevUrgency, exists := previousUrgencies[directiveID]; exists {
					if urgency < prevUrgency {
						t.Errorf(
							"Day %d, %s: urgency decreased from %.3f to %.3f (non-monotonic!)",
							checkpoint.dayOffset,
							directiveID,
							prevUrgency,
							urgency,
						)
					}
				}
			}

			// Store urgency for next iteration
			previousUrgencies[directiveID] = urgency
		}
	}

	// Explicit quadrant verification: D4-Q4 must be Q4 at all checkpoints
	t.Logf("\n=== Q4 Stability Verification (D4-Q4) ===")
	d4q4 := directives["D4-Q4"]
	for _, checkpoint := range checkpoints {
		currentTime := day0.AddDate(0, 0, checkpoint.dayOffset)
		urgency := ComputeUrgency(d4q4.deadline, currentTime)
		quadrant := ComputeQuadrant(d4q4.importance, urgency)

		if quadrant != QuadrantQ4 {
			t.Errorf(
				"CRITICAL: Day %d, D4-Q4 moved out of Q4 to %v! (urgency: %.3f, importance: %d)",
				checkpoint.dayOffset,
				quadrant,
				urgency,
				d4q4.importance,
			)
		}
		t.Logf("  Day %d: D4-Q4 = Q4 ✓ (urgency: %.3f, importance: %d)", checkpoint.dayOffset, urgency, d4q4.importance)
	}
}

// TestScenario0078EdgeCases tests edge cases: deadline exactly at boundary, past deadline evolution
func TestScenario0078EdgeCases(t *testing.T) {
	baseTime := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	t.Logf("\n=== Edge Case: Deadline at Boundary (0.5 urgency) ===")
	// Find a deadline that gives urgency ~0.5 (approximately 5 days)
	boundaryDeadline := baseTime.Add(time.Duration(float64(24*time.Hour) * 5.0))
	urgencyAtBoundary := ComputeUrgency(&boundaryDeadline, baseTime)
	t.Logf("Urgency at ~5 days: %.3f", urgencyAtBoundary)

	// Just after boundary (further away): Q2
	// At 5.1 days out, urgency is lower (less urgent)
	farther := baseTime.Add(time.Duration(float64(24*time.Hour) * 5.1))
	urgencyFarther := ComputeUrgency(&farther, baseTime)
	quadrantFarther := ComputeQuadrant(ImportanceHigh, urgencyFarther)
	if quadrantFarther != QuadrantQ2 {
		t.Errorf("5.1 days out: expected Q2, got %v (urgency: %.3f)", quadrantFarther, urgencyFarther)
	}
	t.Logf("5.1 days (farther): Q2 ✓ (urgency: %.3f)", urgencyFarther)

	// Just before boundary (closer): Q1
	// At 4.9 days out, urgency is higher (more urgent)
	closer := baseTime.Add(time.Duration(float64(24*time.Hour) * 4.9))
	urgencyCloser := ComputeUrgency(&closer, baseTime)
	quadrantCloser := ComputeQuadrant(ImportanceHigh, urgencyCloser)
	if quadrantCloser != QuadrantQ1 {
		t.Errorf("4.9 days out: expected Q1, got %v (urgency: %.3f)", quadrantCloser, urgencyCloser)
	}
	t.Logf("4.9 days (closer): Q1 ✓ (urgency: %.3f)", urgencyCloser)

	t.Logf("\n=== Edge Case: Far-Past Deadline ===")
	// Deadline way in the past should stay Q1 (or Q3 for low importance)
	farPastDeadline := baseTime.AddDate(-1, 0, 0) // 1 year ago
	urgencyFarPast := ComputeUrgency(&farPastDeadline, baseTime)
	quadrantFarPastHigh := ComputeQuadrant(ImportanceHigh, urgencyFarPast)
	quadrantFarPastLow := ComputeQuadrant(ImportanceLow, urgencyFarPast)

	if quadrantFarPastHigh != QuadrantQ1 {
		t.Errorf("Far-past deadline + High: expected Q1, got %v (urgency: %.3f)", quadrantFarPastHigh, urgencyFarPast)
	}
	if quadrantFarPastLow != QuadrantQ3 {
		t.Errorf("Far-past deadline + Low: expected Q3, got %v (urgency: %.3f)", quadrantFarPastLow, urgencyFarPast)
	}
	t.Logf("1-year-past deadline: High→Q1 ✓, Low→Q3 ✓ (urgency: %.3f)", urgencyFarPast)
}

// TestScenario0078MonotonicUrgency explicitly tests monotonic urgency increase
func TestScenario0078MonotonicUrgency(t *testing.T) {
	baseTime := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	deadline := baseTime.AddDate(0, 0, 10) // 10 days out

	t.Logf("\n=== Monotonic Urgency Test (10-day deadline) ===")
	previousUrgency := Urgency(0)

	for day := 0; day <= 10; day++ {
		currentTime := baseTime.AddDate(0, 0, day)
		urgency := ComputeUrgency(&deadline, currentTime)
		t.Logf("Day %d: urgency = %.3f", day, urgency)

		if urgency < previousUrgency {
			t.Errorf("Day %d: urgency %.3f is less than previous %.3f (non-monotonic!)", day, urgency, previousUrgency)
		}
		previousUrgency = urgency
	}
}
