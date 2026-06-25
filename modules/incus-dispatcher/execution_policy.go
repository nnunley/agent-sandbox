package main

import (
	"fmt"
	"sync"
)

// PolicyKind enumerates the execution policy kinds (STORY-0016 AC-3).
type PolicyKind string

const (
	PolicyKindOneShot      PolicyKind = "one-shot"       // Single directive execution
	PolicyKindRalphLoop    PolicyKind = "ralph-loop"     // Ralph-style iterative loop
	PolicyKindResearchBurst PolicyKind = "research-burst" // Burst of research directives
	PolicyKindVerifyFix    PolicyKind = "verify-fix"     // Verification-followed-by-fix loop
	PolicyKindSummarizer   PolicyKind = "background-summarizer" // Background summarization
	PolicyKindReviewOnly   PolicyKind = "review-only"    // Read-only review, no mutations
)

// ExecutionPolicy defines the versioned execution policy (STORY-0016 AC-2).
// It is immutable once created and stored. All fields are required for well-formed policies.
type ExecutionPolicy struct {
	// Kind specifies the policy type (one-shot, loop, etc.)
	Kind PolicyKind `json:"kind"`

	// Constraints is a map of named constraints (e.g., "timeout": "1h", "max_retries": "3")
	Constraints map[string]string `json:"constraints"`

	// DelegationRules is a list of delegation rule names (e.g., "allow-child-directives")
	DelegationRules []string `json:"delegation_rules"`

	// VerificationRequirements is a list of verification rule names (e.g., "external-grade-required")
	VerificationRequirements []string `json:"verification_requirements"`

	// MutationAllowed controls whether this policy permits mutations (STORY-0016 AC-2).
	// Future work (richer shape): could expand to per-artifact-type or per-operation mutation controls.
	MutationAllowed bool `json:"mutation_allowed"`
}

// ExecutionPolicyStore is the durable versioned policy store (STORY-0016 AC-1, AC-4).
// It maintains an immutable history of versions keyed by (policy name, version).
// Each Save() appends a new version; prior versions are never mutated.
// This is a simple in-memory store; production would use a durable backing (e.g., database).
type ExecutionPolicyStore struct {
	mu       sync.RWMutex
	policies map[string]map[int]ExecutionPolicy // policies[name][version_number] = ExecutionPolicy
	versions map[string][]int                    // versions[name] = [v1, v2, ...] sorted ascending
	nextVer  map[string]int                      // nextVer[name] = next version number to assign
}

// NewExecutionPolicyStore creates a new empty policy store.
func NewExecutionPolicyStore() *ExecutionPolicyStore {
	return &ExecutionPolicyStore{
		policies: make(map[string]map[int]ExecutionPolicy),
		versions: make(map[string][]int),
		nextVer:  make(map[string]int),
	}
}

// Save stores a new version of a policy and returns its versioned ID (STORY-0016 AC-1).
// The ID encodes both the policy name and version (e.g., "policy-1@v1").
// v1 is version number 1 (not 0); versions are 1-indexed.
// Immutability is enforced: saving the same policy object again creates a new version.
func (s *ExecutionPolicyStore) Save(name string, policy ExecutionPolicy) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure the policy name has been initialized
	if _, ok := s.policies[name]; !ok {
		s.policies[name] = make(map[int]ExecutionPolicy)
		s.versions[name] = []int{}
		s.nextVer[name] = 1
	}

	// Assign the next version number
	ver := s.nextVer[name]
	s.nextVer[name]++

	// Store the immutable policy value
	s.policies[name][ver] = policy

	// Append version to the ordered list
	s.versions[name] = append(s.versions[name], ver)

	// Return versioned ID
	return fmt.Sprintf("%s@v%d", name, ver)
}

// Get retrieves a policy by its versioned ID (STORY-0016 AC-1, AC-4).
// The ID format is "<name>@v<N>" (e.g., "policy-1@v1").
// Returns (policy, ok) where ok=false if the ID is not found.
func (s *ExecutionPolicyStore) Get(versionedID string) (*ExecutionPolicy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name, ver, ok := parseVersionedID(versionedID)
	if !ok {
		return nil, false
	}

	policies, ok := s.policies[name]
	if !ok {
		return nil, false
	}

	policy, ok := policies[ver]
	if !ok {
		return nil, false
	}

	return &policy, true
}

// ListVersions returns all version IDs for a given policy name in order (STORY-0016 AC-4).
// Returns a list of versioned IDs like ["policy-1@v1", "policy-1@v2", ...].
func (s *ExecutionPolicyStore) ListVersions(name string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verList, ok := s.versions[name]
	if !ok {
		return []string{}
	}

	result := make([]string, len(verList))
	for i, ver := range verList {
		result[i] = fmt.Sprintf("%s@v%d", name, ver)
	}
	return result
}

// Revert recovers an earlier version's policy content (STORY-0016 AC-4).
// Given a versioned ID, it returns a copy of the policy object at that version.
// Returns nil if the ID is not found or invalid.
func (s *ExecutionPolicyStore) Revert(versionedID string) *ExecutionPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name, ver, ok := parseVersionedID(versionedID)
	if !ok {
		return nil
	}

	policies, ok := s.policies[name]
	if !ok {
		return nil
	}

	policy, ok := policies[ver]
	if !ok {
		return nil
	}

	// Return a copy so the caller cannot mutate the stored version
	return &policy
}

// parseVersionedID extracts (name, version) from a versioned ID like "policy-1@v1".
// Returns (name, version, ok) where ok=false if the format is invalid.
func parseVersionedID(versionedID string) (string, int, bool) {
	// Find the @v prefix to split name and version
	atIdx := -1
	for i := len(versionedID) - 1; i >= 0; i-- {
		if versionedID[i] == '@' {
			atIdx = i
			break
		}
	}
	if atIdx < 1 {
		return "", 0, false
	}

	name := versionedID[:atIdx]
	versionStr := versionedID[atIdx+1:]

	var ver int
	n, err := fmt.Sscanf(versionStr, "v%d", &ver)
	if err != nil || n != 1 {
		return "", 0, false
	}

	return name, ver, true
}
