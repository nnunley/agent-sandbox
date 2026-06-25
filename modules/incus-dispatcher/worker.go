package main

// WorkerKind enumerates the worker types available for dispatch (STORY-0011 AC-1).
type WorkerKind string

const (
	WorkerKindLocal          WorkerKind = "local"            // Local machine / embedded
	WorkerKindIncusContainer WorkerKind = "incus-container"  // Incus-managed container
	WorkerKindMicroVM        WorkerKind = "microvm"          // Firecracker micro-VM
	WorkerKindResearch       WorkerKind = "research"         // Research-dedicated worker
)

// Worker represents a dispatching target — a registered worker capable of executing
// directives under selected policies (STORY-0011 AC-1/2/3).
//
// WorkerID is the unique identifier for this worker.
// WorkerKind classifies the worker type (local, incus-container, microvm, research).
// Capabilities lists the tools/features this worker can provide (e.g., "code-review", "deployment").
// AllowedPolicies lists the policy IDs (including version, e.g., "policy-1@v1") that are permitted
// to dispatch to this worker. If a policy is NOT in this list, dispatch MUST reject it, preventing
// unauthorized or inappropriate task delegation.
// RuntimeMode specifies the worker's execution mode — one_shot or long_running (STORY-0013 AC-1).
// The zero value (empty string) defaults to one_shot for backwards compatibility.
type Worker struct {
	WorkerID         string      // Unique identifier for this worker
	WorkerKind       WorkerKind  // Class of worker (local, incus-container, microvm, research)
	Capabilities     []string    // Available tools/features (e.g., ["code-review", "deployment"])
	AllowedPolicies  []string    // Allowed policy IDs (e.g., ["policy-1@v1", "policy-2@v2"])
	RuntimeMode      RuntimeMode // Runtime mode: one_shot or long_running (STORY-0013 AC-1)
}

// HasCapability reports whether this worker offers the given capability.
func (w *Worker) HasCapability(cap string) bool {
	for _, c := range w.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// IsPolicyAllowed reports whether this worker is allowed to execute under the given policy ID.
func (w *Worker) IsPolicyAllowed(policyID string) bool {
	for _, p := range w.AllowedPolicies {
		if p == policyID {
			return true
		}
	}
	return false
}
