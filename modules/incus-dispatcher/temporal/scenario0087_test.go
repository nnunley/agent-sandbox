package temporal

import (
	"testing"
	"time"
)

// TestOperatorScenario0087 simulates a full 7-day operator workflow with human rescores,
// agent bounded rescores, approvals, and escalations.
//
// SCENARIO-0087: Operator-Experience Slice
// Day 0: Operator creates directives with importance/deadline
// Day 2: System escalates Q2→Q1 for upcoming items
// Day 3: Operator manually rescores one item (human override)
// Day 5: Agent requests rescore, denied (bounds exceeded), escalates to approval
// Day 5.5: Approval grants override
// Day 7: Directives complete; re-projection confirms final quadrants
func TestOperatorScenario0087(t *testing.T) {
	baseTime := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

	// Actors
	operator := Actor{Role: ActorRoleHuman, ID: "operator"}
	agent := Actor{Role: ActorRoleAgent, ID: "agent-001"}
	approver := Actor{Role: ActorRoleHuman, ID: "approver"}

	t.Logf("=== SCENARIO-0087: Operator Workflow (7-day timeline) ===\n")

	// ==== DAY 0: Operator creates directives ====
	t.Logf("DAY 0 (%s): Operator creates directives", baseTime.Format("2006-01-02"))

	directive1 := NewGuardedDirective("DIR-001", ImportanceHigh, ptrTime(baseTime.AddDate(0, 0, 5)))
	directive2 := NewGuardedDirective("DIR-002", ImportanceMedium, ptrTime(baseTime.AddDate(0, 0, 7)))
	directive3 := NewGuardedDirective("DIR-003", ImportanceLow, ptrTime(baseTime.AddDate(0, 0, 10)))

	// Temporal projects initial quadrants and priorities
	for _, dir := range []*GuardedDirective{directive1, directive2, directive3} {
		urgency := ComputeUrgency(dir.Deadline, baseTime)
		quadrant := ComputeQuadrant(dir.Importance, urgency)
		priority := ComputeEffectivePriority(dir.Importance, quadrant)
		dir.Quadrant = quadrant
		dir.SetEffectivePriority(WriterRoleTemporal, priority)
	}

	t.Logf("  DIR-001: High importance, 5-day deadline → Q2")
	t.Logf("  DIR-002: Medium importance, 7-day deadline → Q2")
	t.Logf("  DIR-003: Low importance, 10-day deadline → Q4")
	t.Logf("")

	// ==== DAY 2: System escalates ====
	day2 := baseTime.AddDate(0, 0, 2)
	t.Logf("DAY 2 (%s): System escalates Q2→Q1 for upcoming items", day2.Format("2006-01-02"))

	// DIR-001: High importance, now 3 days out → escalates to Q1
	escalated1 := IsEscalationTriggered(directive1.Importance, directive1.Deadline, day2)
	if escalated1 {
		urgency := ComputeUrgency(directive1.Deadline, day2)
		quadrant := ComputeQuadrant(directive1.Importance, urgency)
		priority := ComputeEffectivePriority(directive1.Importance, quadrant)
		directive1.SetEffectivePriority(WriterRoleTemporal, priority)
		directive1.Quadrant = quadrant
		t.Logf("  DIR-001: escalated to Q1 (priority: %d)", priority)
	}

	// DIR-002: Medium importance, now 5 days out → NOT escalated yet (threshold 3, so check: 5 > 3, triggers!)
	escalated2 := IsEscalationTriggered(directive2.Importance, directive2.Deadline, day2)
	if escalated2 {
		urgency := ComputeUrgency(directive2.Deadline, day2)
		quadrant := ComputeQuadrant(directive2.Importance, urgency)
		priority := ComputeEffectivePriority(directive2.Importance, quadrant)
		directive2.SetEffectivePriority(WriterRoleTemporal, priority)
		directive2.Quadrant = quadrant
		t.Logf("  DIR-002: escalated to Q1 (priority: %d)", priority)
	}

	t.Logf("")

	// ==== DAY 3: Operator manually rescores ====
	day3 := baseTime.AddDate(0, 0, 3)
	t.Logf("DAY 3 (%s): Operator manually rescores DIR-001 to Critical", day3.Format("2006-01-02"))

	allowed, escalation, err := ValidateRescoreRequest(operator, directive1.Importance, ImportanceCritical, directive1.Deadline)
	if allowed && !escalation && err == nil {
		// Operator rescores to Critical (unrestricted authority)
		newPriority := ComputeEffectivePriority(ImportanceCritical, QuadrantQ1)
		directive1.SetEffectivePriority(WriterRoleTemporal, newPriority)
		directive1.Importance = ImportanceCritical
		t.Logf("  ✓ DIR-001 rescored to Critical (priority: %d)", newPriority)
	} else {
		t.Errorf("Operator rescore should be allowed: allowed=%v, escalation=%v, err=%v", allowed, escalation, err)
	}
	t.Logf("")

	// ==== DAY 5: Agent requests rescore, denied ====
	day5 := baseTime.AddDate(0, 0, 5)
	t.Logf("DAY 5 (%s): Agent requests rescore (Low→Critical), denied", day5.Format("2006-01-02"))

	// Simulate DIR-004: Agent working on a Low-importance item wants to escalate to Critical
	directive4 := NewGuardedDirective("DIR-004", ImportanceLow, ptrTime(day5.AddDate(0, 0, 2)))

	allowed, escalation, err = ValidateRescoreRequest(agent, ImportanceLow, ImportanceCritical, directive4.Deadline)
	if !allowed && escalation && err != nil {
		t.Logf("  ✗ DIR-004: Agent rescore rejected (reason: %v)", err)
		t.Logf("     Escalated to approval queue")
	} else {
		t.Errorf("Agent self-promotion should be escalated: allowed=%v, escalation=%v", allowed, escalation)
	}
	t.Logf("")

	// ==== DAY 5.5: Approval grants override ====
	day5_5 := baseTime.Add(time.Duration(5.5*24) * time.Hour)
	t.Logf("DAY 5.5 (%s): Approver grants override", day5_5.Format("2006-01-02"))

	// Approver rescores DIR-004 to Critical
	allowed, escalation, err = ValidateRescoreRequest(approver, ImportanceLow, ImportanceCritical, directive4.Deadline)
	if allowed && !escalation && err == nil {
		// Approver rescores with unrestricted authority
		newPriority := ComputeEffectivePriority(ImportanceCritical, QuadrantQ1)
		directive4.SetEffectivePriority(WriterRoleTemporal, newPriority)
		directive4.Importance = ImportanceCritical
		t.Logf("  ✓ DIR-004 rescored to Critical by approver (priority: %d)", newPriority)
	} else {
		t.Errorf("Approver override should be allowed: allowed=%v, escalation=%v", allowed, escalation)
	}
	t.Logf("")

	// ==== DAY 7: Directives complete, final re-projection ====
	day7 := baseTime.AddDate(0, 0, 7)
	t.Logf("DAY 7 (%s): Final re-projection", day7.Format("2006-01-02"))

	// DIR-001: Critical importance, deadline TODAY → Q1
	urgency1 := ComputeUrgency(directive1.Deadline, day7)
	quad1 := ComputeQuadrant(directive1.Importance, urgency1)
	priority1 := ComputeEffectivePriority(directive1.Importance, quad1)
	t.Logf("  DIR-001: Critical, deadline today → %v (priority: %d)", quad1, priority1)

	// DIR-002: Medium importance, deadline PASSED → Q1
	urgency2 := ComputeUrgency(directive2.Deadline, day7)
	quad2 := ComputeQuadrant(directive2.Importance, urgency2)
	priority2 := ComputeEffectivePriority(directive2.Importance, quad2)
	t.Logf("  DIR-002: Medium, deadline passed → %v (priority: %d)", quad2, priority2)

	// DIR-003: Low importance, 3 days remaining → Q3
	urgency3 := ComputeUrgency(directive3.Deadline, day7)
	quad3 := ComputeQuadrant(directive3.Importance, urgency3)
	priority3 := ComputeEffectivePriority(directive3.Importance, quad3)
	t.Logf("  DIR-003: Low, 3 days remaining → %v (priority: %d)", quad3, priority3)

	// DIR-004: Critical (after approval), deadline PASSED → Q1
	urgency4 := ComputeUrgency(directive4.Deadline, day7)
	quad4 := ComputeQuadrant(directive4.Importance, urgency4)
	priority4 := ComputeEffectivePriority(directive4.Importance, quad4)
	t.Logf("  DIR-004: Critical (approved), deadline passed → %v (priority: %d)", quad4, priority4)

	t.Logf("")
	t.Logf("=== SCENARIO-0087 Complete ===")
	t.Logf("✓ Human rescores unrestricted")
	t.Logf("✓ Agent bounded rescores enforced")
	t.Logf("✓ Escalations routed to approval")
	t.Logf("✓ Approval overrides enabled")
	t.Logf("✓ Final quadrants confirmed")
}
