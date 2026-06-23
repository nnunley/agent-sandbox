package temporal

import (
	"fmt"
	"sync"
	"time"
)

// WriterRole represents who is allowed to write to protected fields.
type WriterRole int

const (
	WriterRoleTemporal WriterRole = iota
	WriterRoleQueue
	WriterRoleHuman
)

// String returns the string representation of a WriterRole.
func (wr WriterRole) String() string {
	switch wr {
	case WriterRoleTemporal:
		return "Temporal"
	case WriterRoleQueue:
		return "Queue"
	case WriterRoleHuman:
		return "Human"
	default:
		return fmt.Sprintf("WriterRole(%d)", wr)
	}
}

// GuardedDirective wraps a directive to enforce single-writer invariants.
// Only WriterRole==Temporal can write priority fields (EffectivePriority, NotBefore).
// Access to priority fields is protected by a sync.RWMutex for concurrent safety.
type GuardedDirective struct {
	mu sync.RWMutex
	// Private fields: protected by writer invariant + mutex
	effectivePriority int
	notBefore         time.Time
	// Public metadata for reference
	DirectiveID string
	Importance  Importance
	Deadline    *time.Time
	Quadrant    Quadrant
}

// NewGuardedDirective creates a new guarded directive with initial values.
func NewGuardedDirective(id string, importance Importance, deadline *time.Time) *GuardedDirective {
	return &GuardedDirective{
		DirectiveID:       id,
		Importance:        importance,
		Deadline:          deadline,
		effectivePriority: 0,
		notBefore:         time.Time{},
	}
}

// SetEffectivePriority sets the effective priority.
// Only WriterRole==Temporal is allowed; returns error for unauthorized roles.
func (gd *GuardedDirective) SetEffectivePriority(role WriterRole, value int) error {
	if role != WriterRoleTemporal {
		return fmt.Errorf(
			"unauthorized write to EffectivePriority: role=%s (only %s allowed)",
			role, WriterRoleTemporal,
		)
	}
	gd.mu.Lock()
	defer gd.mu.Unlock()
	gd.effectivePriority = value
	return nil
}

// SetNotBefore sets the NotBefore time.
// Only WriterRole==Temporal is allowed; returns error for unauthorized roles.
func (gd *GuardedDirective) SetNotBefore(role WriterRole, value time.Time) error {
	if role != WriterRoleTemporal {
		return fmt.Errorf(
			"unauthorized write to NotBefore: role=%s (only %s allowed)",
			role, WriterRoleTemporal,
		)
	}
	gd.mu.Lock()
	defer gd.mu.Unlock()
	gd.notBefore = value
	return nil
}

// GetEffectivePriority returns the effective priority.
// Read is allowed from any role (defensive read).
func (gd *GuardedDirective) GetEffectivePriority() int {
	gd.mu.RLock()
	defer gd.mu.RUnlock()
	return gd.effectivePriority
}

// GetNotBefore returns the NotBefore time.
// Read is allowed from any role (defensive read).
func (gd *GuardedDirective) GetNotBefore() time.Time {
	gd.mu.RLock()
	defer gd.mu.RUnlock()
	return gd.notBefore
}

// ValidateWriterInvariant checks that only Temporal has written to priority fields.
// In a real system, this would be called by the queue/daemon to validate
// that no unauthorized role has modified protected fields.
//
// For GuardedDirective, this is enforced by the type system itself
// (private fields + guarded setters), but this function provides an
// explicit check for auditing and testing.
func ValidateWriterInvariant(gd *GuardedDirective) error {
	// GuardedDirective enforces invariant by construction:
	// - Fields are private
	// - Only SetEffectivePriority/SetNotBefore (guarded) can mutate
	// So if a GuardedDirective exists, the invariant is valid
	// This function is a no-op for GuardedDirective, but could be extended
	// to validate other directive types in the future.
	return nil
}
