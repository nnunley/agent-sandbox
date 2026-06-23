package temporal

import (
	"testing"
	"time"
)

// TestUrgencyNil tests that nil deadline returns 0.0 urgency
func TestUrgencyNil(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	urgency := ComputeUrgency(nil, now)
	if urgency != 0.0 {
		t.Errorf("ComputeUrgency(nil, now) = %v, want 0.0", urgency)
	}
}

// TestUrgencyFarFuture tests that far-future deadlines return low urgency
func TestUrgencyFarFuture(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	deadline := time.Date(2030, 12, 31, 23, 59, 59, 0, time.UTC)
	urgency := ComputeUrgency(&deadline, now)
	if urgency >= 0.1 {
		t.Errorf("ComputeUrgency(far future) = %v, want < 0.1", urgency)
	}
}

// TestUrgencyPast tests that past deadlines return >= 1.0 urgency
func TestUrgencyPast(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	deadline := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	urgency := ComputeUrgency(&deadline, now)
	if urgency < 1.0 {
		t.Errorf("ComputeUrgency(past deadline) = %v, want >= 1.0", urgency)
	}
}

// TestQuadrantQ1 tests important + urgent items go to Q1
func TestQuadrantQ1(t *testing.T) {
	tests := []struct {
		name       string
		importance Importance
		urgency    Urgency
	}{
		{"Critical + high urgency", ImportanceCritical, 0.8},
		{"High + high urgency", ImportanceHigh, 0.8},
		{"High + deadline passed", ImportanceHigh, 1.0},
	}

	for _, tt := range tests {
		q := ComputeQuadrant(tt.importance, tt.urgency)
		if q != QuadrantQ1 {
			t.Errorf("%s: got %v, want Q1", tt.name, q)
		}
	}
}

// TestQuadrantQ2 tests important + not urgent items go to Q2
func TestQuadrantQ2(t *testing.T) {
	tests := []struct {
		name       string
		importance Importance
		urgency    Urgency
	}{
		{"Critical + low urgency", ImportanceCritical, 0.1},
		{"High + low urgency", ImportanceHigh, 0.3},
		{"High + no urgency", ImportanceHigh, 0.0},
	}

	for _, tt := range tests {
		q := ComputeQuadrant(tt.importance, tt.urgency)
		if q != QuadrantQ2 {
			t.Errorf("%s: got %v, want Q2", tt.name, q)
		}
	}
}

// TestQuadrantQ3 tests not important + urgent items go to Q3
func TestQuadrantQ3(t *testing.T) {
	tests := []struct {
		name       string
		importance Importance
		urgency    Urgency
	}{
		{"Low + high urgency", ImportanceLow, 0.8},
		{"Medium + urgent", ImportanceMedium, 0.6},
		{"Low + deadline passed", ImportanceLow, 1.0},
	}

	for _, tt := range tests {
		q := ComputeQuadrant(tt.importance, tt.urgency)
		if q != QuadrantQ3 {
			t.Errorf("%s: got %v, want Q3", tt.name, q)
		}
	}
}

// TestQuadrantQ4 tests not important + not urgent items go to Q4
func TestQuadrantQ4(t *testing.T) {
	tests := []struct {
		name       string
		importance Importance
		urgency    Urgency
	}{
		{"Low + no urgency", ImportanceLow, 0.0},
		{"Low + low urgency", ImportanceLow, 0.2},
		{"Medium + low urgency", ImportanceMedium, 0.3},
	}

	for _, tt := range tests {
		q := ComputeQuadrant(tt.importance, tt.urgency)
		if q != QuadrantQ4 {
			t.Errorf("%s: got %v, want Q4", tt.name, q)
		}
	}
}

// TestDeadlinePromotion tests Q2 -> Q1 transition as deadline approaches
func TestDeadlinePromotion(t *testing.T) {
	baseTime := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	deadline := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	now1 := baseTime
	urgency1 := ComputeUrgency(&deadline, now1)
	q1 := ComputeQuadrant(ImportanceHigh, urgency1)
	if q1 != QuadrantQ2 {
		t.Errorf("7 days before deadline: got %v, want Q2 (urgency: %v)", q1, urgency1)
	}

	now2 := baseTime.AddDate(0, 0, 6)
	urgency2 := ComputeUrgency(&deadline, now2)
	q2 := ComputeQuadrant(ImportanceHigh, urgency2)
	if q2 != QuadrantQ1 {
		t.Errorf("1 day before deadline: got %v, want Q1 (urgency: %v)", q2, urgency2)
	}
}

// TestQ4Stability tests that Q4 (no deadline, low importance) stays in Q4
func TestQ4Stability(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	urgency := ComputeUrgency(nil, now)
	q := ComputeQuadrant(ImportanceLow, urgency)
	if q != QuadrantQ4 {
		t.Errorf("No deadline, low importance: got %v, want Q4", q)
	}
}

// TestEffectivePriority tests priority ordering: Q1 > Q2 > Q3 > Q4
func TestEffectivePriority(t *testing.T) {
	q1Priority := ComputeEffectivePriority(ImportanceHigh, QuadrantQ1)
	q2Priority := ComputeEffectivePriority(ImportanceHigh, QuadrantQ2)
	q3Priority := ComputeEffectivePriority(ImportanceHigh, QuadrantQ3)
	q4Priority := ComputeEffectivePriority(ImportanceHigh, QuadrantQ4)

	if q1Priority <= q2Priority {
		t.Errorf("Q1 priority %d should be > Q2 priority %d", q1Priority, q2Priority)
	}
	if q2Priority <= q3Priority {
		t.Errorf("Q2 priority %d should be > Q3 priority %d", q2Priority, q3Priority)
	}
	if q3Priority <= q4Priority {
		t.Errorf("Q3 priority %d should be > Q4 priority %d", q3Priority, q4Priority)
	}
}

// TestEffectivePriorityImportanceWins tests that higher importance wins within quadrant
func TestEffectivePriorityImportanceWins(t *testing.T) {
	criticalQ4 := ComputeEffectivePriority(ImportanceCritical, QuadrantQ4)
	lowQ4 := ComputeEffectivePriority(ImportanceLow, QuadrantQ4)

	if criticalQ4 <= lowQ4 {
		t.Errorf("Critical Q4 %d should be > Low Q4 %d", criticalQ4, lowQ4)
	}
}

// TestDirectiveProjection tests the full projection on a Directive
func TestDirectiveProjection(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		importance Importance
		deadline   *time.Time
		expectQ    Quadrant
	}{
		{
			"Critical with deadline in 1 day",
			ImportanceCritical,
			ptrTime(now.AddDate(0, 0, 1)),
			QuadrantQ1,
		},
		{
			"Critical with deadline in 10 days",
			ImportanceCritical,
			ptrTime(now.AddDate(0, 0, 10)),
			QuadrantQ2,
		},
		{
			"Low with no deadline",
			ImportanceLow,
			nil,
			QuadrantQ4,
		},
		{
			"Low with deadline in 1 day",
			ImportanceLow,
			ptrTime(now.AddDate(0, 0, 1)),
			QuadrantQ3,
		},
	}

	for _, tt := range tests {
		urgency := ComputeUrgency(tt.deadline, now)
		q := ComputeQuadrant(tt.importance, urgency)
		if q != tt.expectQ {
			t.Errorf("%s: got %v, want %v (urgency: %v)", tt.name, q, tt.expectQ, urgency)
		}
	}
}

// TestQuadrantString tests string representation
func TestQuadrantString(t *testing.T) {
	tests := []struct {
		q    Quadrant
		want string
	}{
		{QuadrantQ1, "Q1"},
		{QuadrantQ2, "Q2"},
		{QuadrantQ3, "Q3"},
		{QuadrantQ4, "Q4"},
	}

	for _, tt := range tests {
		if tt.q.String() != tt.want {
			t.Errorf("Quadrant.String() = %q, want %q", tt.q.String(), tt.want)
		}
	}
}

// TestUrgencyRange tests that urgency stays non-negative
func TestUrgencyRange(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		deadline *time.Time
	}{
		{"nil", nil},
		{"far future", ptrTime(time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC))},
		{"1 day ahead", ptrTime(now.AddDate(0, 0, 1))},
		{"just passed", ptrTime(now.Add(-time.Hour))},
		{"far past", ptrTime(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))},
	}

	for _, tt := range tests {
		urgency := ComputeUrgency(tt.deadline, now)
		if urgency < 0 {
			t.Errorf("%s: urgency %v is negative", tt.name, urgency)
		}
	}
}

// TestMultipleDeadlines tests multiple items with varying deadlines
func TestMultipleDeadlines(t *testing.T) {
	now := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	items := []struct {
		name   string
		imprt  Importance
		ddl    *time.Time
		wantQ  Quadrant
	}{
		{"Item A", ImportanceCritical, ptrTime(now.AddDate(0, 0, 1)), QuadrantQ1},
		{"Item B", ImportanceHigh, ptrTime(now.AddDate(0, 0, 7)), QuadrantQ2},
		{"Item C", ImportanceMedium, ptrTime(now.AddDate(0, 0, 1)), QuadrantQ3},
		{"Item D", ImportanceLow, nil, QuadrantQ4},
	}

	for _, item := range items {
		urgency := ComputeUrgency(item.ddl, now)
		q := ComputeQuadrant(item.imprt, urgency)
		if q != item.wantQ {
			t.Errorf("%s: got %v, want %v (urgency: %v)", item.name, q, item.wantQ, urgency)
		}
	}
}

// TestImportanceStringToTierConversion tests the conversion from queue.Importance strings to temporal.Importance tiers
func TestImportanceStringToTierConversion(t *testing.T) {
	tests := []struct {
		name        string
		importance  string
		expectTier  Importance
		expectError bool
	}{
		{"high maps to tier 2", "high", ImportanceHigh, false},
		{"normal maps to tier 1", "normal", ImportanceMedium, false},
		{"low maps to tier 0", "low", ImportanceLow, false},
		{"critical maps to tier 3", "critical", ImportanceCritical, false},
		{"unknown string returns error", "unknown", -1, true},
		{"empty string returns error", "", -1, true},
	}

	for _, tt := range tests {
		tier, err := ImportanceStringToTier(tt.importance)
		if tt.expectError {
			if err == nil {
				t.Errorf("%s: expected error, got nil", tt.name)
			}
		} else {
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tt.name, err)
			}
			if tier != tt.expectTier {
				t.Errorf("%s: got %v, want %v", tt.name, tier, tt.expectTier)
			}
		}
	}
}

// TestImportanceStringToTierDeterministic tests that conversion is deterministic
func TestImportanceStringToTierDeterministic(t *testing.T) {
	importance := "high"
	tier1, _ := ImportanceStringToTier(importance)
	tier2, _ := ImportanceStringToTier(importance)
	if tier1 != tier2 {
		t.Errorf("Conversion not deterministic: %v != %v", tier1, tier2)
	}
}

// TestUrgencyBoundary05 tests the urgency=0.5 boundary where Q2 -> Q1 transition occurs
func TestUrgencyBoundary05(t *testing.T) {
	baseTime := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	// Approximately 5 days remaining should give urgency ~0.5
	fiveDaysDeadline := baseTime.AddDate(0, 0, 5)
	fourPointNineDeadline := baseTime.Add(time.Duration(float64(24*time.Hour) * 4.9))
	fivePointOneDeadline := baseTime.Add(time.Duration(float64(24*time.Hour) * 5.1))

	// Test at approximately 5 days: urgency should be near 0.5
	urgency5days := ComputeUrgency(&fiveDaysDeadline, baseTime)
	if urgency5days < 0.45 || urgency5days > 0.55 {
		t.Logf("WARNING: At 5 days, urgency = %v (expected ~0.50 for boundary)", urgency5days)
	}

	// Test at 4.9 days: urgency should be > 0.5, promoting Q2 to Q1 (with importance >= 2)
	urgency4_9days := ComputeUrgency(&fourPointNineDeadline, baseTime)
	if urgency4_9days <= 0.5 {
		t.Errorf("At 4.9 days, urgency = %v, want > 0.5", urgency4_9days)
	}
	q4_9 := ComputeQuadrant(ImportanceHigh, urgency4_9days)
	if q4_9 != QuadrantQ1 {
		t.Errorf("At 4.9 days with High importance, got %v, want Q1", q4_9)
	}

	// Test at 5.1 days: urgency should be < 0.5, keeping Q2 (with importance >= 2)
	urgency5_1days := ComputeUrgency(&fivePointOneDeadline, baseTime)
	if urgency5_1days >= 0.5 {
		t.Errorf("At 5.1 days, urgency = %v, want < 0.5", urgency5_1days)
	}
	q5_1 := ComputeQuadrant(ImportanceHigh, urgency5_1days)
	if q5_1 != QuadrantQ2 {
		t.Errorf("At 5.1 days with High importance, got %v, want Q2", q5_1)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
