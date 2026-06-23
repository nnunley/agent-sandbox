package temporal

import (
	"fmt"
	"time"
)

// ActorRole represents the type of actor making a request.
type ActorRole int

const (
	ActorRoleHuman ActorRole = iota
	ActorRoleAgent
)

// String returns the string representation of an ActorRole.
func (ar ActorRole) String() string {
	switch ar {
	case ActorRoleHuman:
		return "Human"
	case ActorRoleAgent:
		return "Agent"
	default:
		return fmt.Sprintf("ActorRole(%d)", ar)
	}
}

// Actor represents who is making a rescore request.
type Actor struct {
	Role ActorRole
	ID   string // Human name or Agent identifier
}

// IsHumanUnrestricted returns true if the actor is human (unrestricted rescore authority).
// Humans can rescore to any quadrant/importance level.
func IsHumanUnrestricted(actor Actor) bool {
	return actor.Role == ActorRoleHuman
}

// IsAgentBounded validates whether an agent rescore request is within bounded change limits.
// Returns (allowed, error) where allowed=true if the rescore is within bounds.
//
// Agent bounds:
// - Cannot jump more than 1 tier at a time (e.g., Low to Medium is OK, Low to Critical is not)
// - Cannot self-promote to Critical (Tier 3) without explicit override approval
//
// Returns (false, reason) if the rescore exceeds bounds.
func IsAgentBounded(actor Actor, currentImportance Importance, proposedImportance Importance) (bool, error) {
	if actor.Role != ActorRoleAgent {
		// Only agents are bounded; humans are unrestricted
		return true, nil
	}

	// Check if jump is more than 1 tier
	tierDiff := int(proposedImportance) - int(currentImportance)
	if tierDiff > 1 || tierDiff < -1 {
		return false, fmt.Errorf(
			"agent %s: tier jump from %d to %d exceeds bound (max 1 tier)",
			actor.ID, currentImportance, proposedImportance,
		)
	}

	// Check if agent is trying to self-promote to Critical (Tier 3)
	if proposedImportance == ImportanceCritical && tierDiff > 0 {
		// Upward jump TO Critical is not allowed without override
		return false, fmt.Errorf(
			"agent %s: cannot self-promote to Critical (Tier 3) without approval override",
			actor.ID,
		)
	}

	return true, nil
}

// ValidateRescoreRequest performs full validation of a rescore request.
// Returns (allowed, escalationRequired, error).
//
// - allowed: true if the rescore is allowed to proceed immediately
// - escalationRequired: true if the rescore exceeds bounds and must route to approval queue
// - error: reason if validation failed (human-readable)
//
// Workflow:
// - Humans: always return (true, false, nil) — no bounds, no escalation
// - Agents within bounds: return (true, false, nil) — allowed
// - Agents out of bounds (e.g., self-promoting to Critical): return (false, true, reason) — escalate to approval
func ValidateRescoreRequest(actor Actor, currentImportance Importance, proposedImportance Importance, deadline *time.Time) (allowed bool, escalationRequired bool, err error) {
	if IsHumanUnrestricted(actor) {
		// Humans can rescore anything; no escalation needed
		return true, false, nil
	}

	// Agent: check bounds
	withinBounds, boundErr := IsAgentBounded(actor, currentImportance, proposedImportance)
	if withinBounds {
		// Within bounds: allowed to proceed immediately
		return true, false, nil
	}

	// Out of bounds: route to approval queue
	return false, true, boundErr
}
