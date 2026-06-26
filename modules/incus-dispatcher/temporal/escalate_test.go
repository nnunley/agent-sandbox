package temporal

import (
	"testing"
	"time"
)

// TestEscalationThreshold validates importance-based escalation windows.
func TestEscalationThreshold(t *testing.T) {
	tests := []struct {
		name       string
		importance Importance
		wantDays   float64
	}{
		{"Critical (Tier 3)", ImportanceCritical, 7},
		{"High (Tier 2)", ImportanceHigh, 5},
		{"Medium (Tier 1)", ImportanceMedium, 3},
		{"Low (Tier 0)", ImportanceLow, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration := EscalationThreshold(tt.importance)
			expectedDuration := time.Duration(int64(tt.wantDays*24)) * time.Hour
			if duration != expectedDuration {
				t.Errorf("EscalationThreshold: got %v (%f days), want %v (%f days)",
					duration, duration.Hours()/24, expectedDuration, tt.wantDays)
			}
			t.Logf("  %s: %d days", tt.name, int(tt.wantDays))
		})
	}
}

// TestIsEscalationTriggered validates escalation triggering logic.
func TestIsEscalationTriggered(t *testing.T) {
	baseTime := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		importance        Importance
		daysUntilDeadline float64
		wantTriggered     bool
	}{
		// Critical (7-day threshold)
		{"Critical, 8 days out (no trigger)", ImportanceCritical, 8, false},
		{"Critical, 7 days out (trigger)", ImportanceCritical, 7, true},
		{"Critical, 6 days out (trigger)", ImportanceCritical, 6, true},
		{"Critical, deadline passed (trigger)", ImportanceCritical, -1, true},

		// High (5-day threshold)
		{"High, 6 days out (no trigger)", ImportanceHigh, 6, false},
		{"High, 5 days out (trigger)", ImportanceHigh, 5, true},
		{"High, 1 day out (trigger)", ImportanceHigh, 1, true},

		// Medium (3-day threshold)
		{"Medium, 4 days out (no trigger)", ImportanceMedium, 4, false},
		{"Medium, 3 days out (trigger)", ImportanceMedium, 3, true},

		// Low (1-day threshold)
		{"Low, 2 days out (no trigger)", ImportanceLow, 2, false},
		{"Low, 1 day out (trigger)", ImportanceLow, 1, true},
		{"Low, 0 days (deadline today, trigger)", ImportanceLow, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deadline := baseTime.Add(time.Duration(int64(tt.daysUntilDeadline*24)) * time.Hour)
			triggered := IsEscalationTriggered(tt.importance, &deadline, baseTime)
			if triggered != tt.wantTriggered {
				t.Errorf("IsEscalationTriggered: got %v, want %v", triggered, tt.wantTriggered)
			}
		})
	}
}

// TestIsEscalationTriggeredNilDeadline validates that items without deadlines never escalate.
func TestIsEscalationTriggeredNilDeadline(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	for _, importance := range []Importance{ImportanceCritical, ImportanceHigh, ImportanceMedium, ImportanceLow} {
		triggered := IsEscalationTriggered(importance, nil, now)
		if triggered {
			t.Errorf("Nil deadline, %v: escalation triggered, want false", importance)
		}
	}
	t.Logf("✓ Nil deadlines never trigger escalation")
}

// TestReprojectOnEscalation validates quadrant transitions on escalation.
// Note: Medium (tier 1) and Low (tier 0) are NOT important (importance >= 2 is important).
// So Medium + urgency < 0.5 → Q4, >= 0.5 → Q3
// High (tier 2) and Critical (tier 3) are important.
// So High + urgency < 0.5 → Q2, >= 0.5 → Q1
// Urgency curve: at 5 days ≈ 0.50, at 4 days ≈ 0.55, at 2 days ≈ 0.60
func TestReprojectOnEscalation(t *testing.T) {
	baseTime := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		importance        Importance
		daysUntilDeadline float64
		expectQuadrant    Quadrant
	}{
		// High importance (tier 2, IS important)
		{"High, 8 days (Q2)", ImportanceHigh, 8, QuadrantQ2},
		{"High, 4 days (Q1)", ImportanceHigh, 4, QuadrantQ1},
		{"High, deadline today (Q1)", ImportanceHigh, 0, QuadrantQ1},

		// Low importance (tier 0, NOT important)
		{"Low, 2 days (Q3)", ImportanceLow, 2, QuadrantQ3},
		{"Low, 1 day (Q3)", ImportanceLow, 1, QuadrantQ3},

		// Medium importance (tier 1, NOT important)
		// At 4 days: urgency ≈ 0.55, Q3 (not important + urgent)
		// At 2 days: urgency ≈ 0.603, Q3 (not important + urgent)
		{"Medium, 4 days (Q3)", ImportanceMedium, 4, QuadrantQ3},
		{"Medium, 2 days (Q3)", ImportanceMedium, 2, QuadrantQ3},

		// Critical importance (tier 3, IS important)
		// At 10 days: urgency ~0.016, Q2 (important + not urgent)
		// At 1 day: urgency ~0.690, Q1 (important + urgent)
		{"Critical, 10 days (Q2)", ImportanceCritical, 10, QuadrantQ2},
		{"Critical, 1 day (Q1)", ImportanceCritical, 1, QuadrantQ1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deadline := baseTime.Add(time.Duration(int64(tt.daysUntilDeadline*24)) * time.Hour)
			quadrant, priority, err := ReprojectOnEscalation(tt.importance, &deadline, baseTime)
			if err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if quadrant != tt.expectQuadrant {
				t.Errorf("quadrant = %v, want %v (priority: %d)", quadrant, tt.expectQuadrant, priority)
			}
		})
	}
}

// TestReprojectOnEscalationNilDeadline validates reprojection with no deadline.
func TestReprojectOnEscalationNilDeadline(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	// Items without deadlines stay in their natural quadrant
	// Low importance, no deadline → Q4
	quadrant, priority, err := ReprojectOnEscalation(ImportanceLow, nil, now)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if quadrant != QuadrantQ4 {
		t.Errorf("Low, nil deadline: got %v, want Q4", quadrant)
	}
	t.Logf("✓ Low+nil deadline: Q4 (priority: %d)", priority)

	// High importance, no deadline → Q2
	quadrant, priority, err = ReprojectOnEscalation(ImportanceHigh, nil, now)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if quadrant != QuadrantQ2 {
		t.Errorf("High, nil deadline: got %v, want Q2", quadrant)
	}
	t.Logf("✓ High+nil deadline: Q2 (priority: %d)", priority)
}

// TestEscalationChain simulates a full escalation chain over time.
func TestEscalationChain(t *testing.T) {
	baseTime := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	deadline := baseTime.AddDate(0, 0, 5) // 5 days out (High importance threshold = 5 days)

	t.Logf("=== Escalation Chain (High importance, 5-day deadline) ===")

	// High importance, 5-day threshold
	// Day 0: 5 days out, timeRemaining <= 5 days (equal), escalation TRIGGERED
	// Day 1: 4 days out, timeRemaining <= 5 days, escalation TRIGGERED
	checkpoints := []struct {
		day            int
		expectEsc      bool
		expectQuadrant Quadrant
	}{
		{0, true, QuadrantQ1}, // 5 days out, timeRemaining==threshold, escalated to Q1
		{1, true, QuadrantQ1}, // 4 days out, timeRemaining < threshold, escalated to Q1
		{2, true, QuadrantQ1}, // 3 days out, escalated to Q1
		{3, true, QuadrantQ1}, // 2 days out, stays Q1
		{4, true, QuadrantQ1}, // 1 day out, stays Q1
		{5, true, QuadrantQ1}, // deadline passed, stays Q1
	}

	for _, cp := range checkpoints {
		now := baseTime.AddDate(0, 0, cp.day)
		escalated := IsEscalationTriggered(ImportanceHigh, &deadline, now)
		quadrant, _, _ := ReprojectOnEscalation(ImportanceHigh, &deadline, now)

		if escalated != cp.expectEsc {
			t.Errorf("Day %d: escalated=%v, want %v", cp.day, escalated, cp.expectEsc)
		}
		if quadrant != cp.expectQuadrant {
			t.Errorf("Day %d: quadrant=%v, want %v", cp.day, quadrant, cp.expectQuadrant)
		}
		t.Logf("Day %d: escalated=%v, quadrant=%v ✓", cp.day, escalated, quadrant)
	}
}

// TestEscalationByImportance shows how escalation windows vary by importance.
func TestEscalationByImportance(t *testing.T) {
	baseTime := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	// Same deadline, different importance levels (10 days out)
	deadline := baseTime.AddDate(0, 0, 10)

	t.Logf("=== Escalation by Importance (10-day deadline, at day 0) ===")

	tests := []struct {
		importance   Importance
		threshold    int  // days
		willEscalate bool // at 10 days out
	}{
		// At day 0 (10 days remaining): all are > threshold, so none escalated
		{ImportanceCritical, 7, false}, // 10 > 7: NOT escalated
		{ImportanceHigh, 5, false},     // 10 > 5: NOT escalated
		{ImportanceMedium, 3, false},   // 10 > 3: NOT escalated
		{ImportanceLow, 1, false},      // 10 > 1: NOT escalated
	}

	for _, tt := range tests {
		escalated := IsEscalationTriggered(tt.importance, &deadline, baseTime)
		if escalated != tt.willEscalate {
			t.Errorf("%v: escalated=%v, want %v", tt.importance, escalated, tt.willEscalate)
		}
		t.Logf("%v (threshold: %d days): escalated=%v", tt.importance, tt.threshold, escalated)
	}
}
