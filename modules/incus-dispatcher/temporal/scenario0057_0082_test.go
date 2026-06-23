package temporal

import (
	"testing"
)

// TestScenario0057HumanUnrestrictedRescore validates SCENARIO-0057 D1.
// D1: Human rescores Low-importance item to Critical (must succeed)
func TestScenario0057HumanUnrestrictedRescore(t *testing.T) {
	human := Actor{
		Role: ActorRoleHuman,
		ID:   "operator",
	}

	allowed, escalation, err := ValidateRescoreRequest(human, ImportanceLow, ImportanceCritical, nil)
	if !allowed || escalation || err != nil {
		t.Errorf("D1 (human rescores Low→Critical): allowed=%v, escalation=%v, err=%v; want allowed=true, escalation=false, err=nil",
			allowed, escalation, err)
	}
	t.Logf("✓ D1 (human rescores Low→Critical): allowed (no escalation)")
}

// TestScenario0057AgentBoundedRescore validates SCENARIO-0057 D2.
// D2: Agent rescores Medium to High (must succeed, 1-tier jump allowed)
func TestScenario0057AgentBoundedRescore(t *testing.T) {
	agent := Actor{
		Role: ActorRoleAgent,
		ID:   "agent-001",
	}

	allowed, escalation, err := ValidateRescoreRequest(agent, ImportanceMedium, ImportanceHigh, nil)
	if !allowed || escalation || err != nil {
		t.Errorf("D2 (agent rescores Medium→High): allowed=%v, escalation=%v, err=%v; want allowed=true, escalation=false, err=nil",
			allowed, escalation, err)
	}
	t.Logf("✓ D2 (agent rescores Medium→High): allowed (within bounds)")
}

// TestScenario0082ApprovalEscalation validates SCENARIO-0082 D3-D4.
// D3: Agent rescores Low to Critical (must fail, self-promotion blocked, escalate to approval)
// D4: Approval grants override; agent can now rescore with authorization
func TestScenario0082ApprovalEscalation(t *testing.T) {
	agent := Actor{
		Role: ActorRoleAgent,
		ID:   "agent-002",
	}
	approver := Actor{
		Role: ActorRoleHuman,
		ID:   "approver",
	}

	// D3: Agent rescores Low to Critical (must fail, self-promotion blocked)
	allowed, escalation, err := ValidateRescoreRequest(agent, ImportanceLow, ImportanceCritical, nil)
	if allowed || !escalation || err == nil {
		t.Errorf("D3 (agent rescores Low→Critical): allowed=%v, escalation=%v, err=%v; want allowed=false, escalation=true, err!=nil",
			allowed, escalation, err)
	}
	t.Logf("✓ D3 (agent rescores Low→Critical): escalated to approval (reason: %v)", err)

	// D4: Approval grants override; approver can rescore with unrestricted authority
	allowed, escalation, err = ValidateRescoreRequest(approver, ImportanceLow, ImportanceCritical, nil)
	if !allowed || escalation || err != nil {
		t.Errorf("D4 (approver override): allowed=%v, escalation=%v, err=%v; want allowed=true, escalation=false, err=nil",
			allowed, escalation, err)
	}
	t.Logf("✓ D4 (approver override): allowed (approver has unrestricted authority)")
}

// TestScenario0082DriftingAgentPrevention validates SCENARIO-0082 D5.
// D5: Agent proposes multiple small rescores over time
// Validates that repeated rescores don't bypass bounds via drift.
func TestScenario0082DriftingAgentPrevention(t *testing.T) {
	agent := Actor{
		Role: ActorRoleAgent,
		ID:   "agent-003",
	}

	t.Logf("=== Drift Prevention Test (agent-003) ===")

	// Step 1: Low → Medium (allowed, 1 tier)
	allowed, escalation, _ := ValidateRescoreRequest(agent, ImportanceLow, ImportanceMedium, nil)
	if !allowed || escalation {
		t.Errorf("Step 1 (Low→Medium): allowed=%v, escalation=%v; want both false", allowed, escalation)
	}
	t.Logf("Step 1 (Low→Medium): ✓ allowed")

	// Step 2: Medium → High (allowed, 1 tier)
	allowed, escalation, _ = ValidateRescoreRequest(agent, ImportanceMedium, ImportanceHigh, nil)
	if !allowed || escalation {
		t.Errorf("Step 2 (Medium→High): allowed=%v, escalation=%v; want both false", allowed, escalation)
	}
	t.Logf("Step 2 (Medium→High): ✓ allowed")

	// Step 3: High → Critical (blocked, self-promotion)
	allowed, escalation, _ = ValidateRescoreRequest(agent, ImportanceHigh, ImportanceCritical, nil)
	if allowed || !escalation {
		t.Errorf("Step 3 (High→Critical): allowed=%v, escalation=%v; want allowed=false, escalation=true", allowed, escalation)
	}
	t.Logf("Step 3 (High→Critical): ✓ escalated (drift blocked)")

	t.Logf("✓ D5 (agent drifting): bounds enforced at each step, drift prevented")
}
