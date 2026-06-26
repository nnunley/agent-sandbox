package main

import (
	"strings"
	"testing"
	"time"
)

// stringContains reports whether s contains substr.
func stringContains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// TestGenomeDetector_FiresOnThreeTimeouts verifies that the detector fires a pattern
// when three distinct runs have timeout stumbles within the window (SCENARIO-0018 AC-4,
// design note §7).
func TestGenomeDetector_FiresOnThreeTimeouts(t *testing.T) {
	clk := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	// Create three runs, each with a timeout stumble.
	run1 := &Run{
		RunID:      "run-1",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-5 * time.Minute), RunID: "run-1"},
		},
	}
	run2 := &Run{
		RunID:      "run-2",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-3 * time.Minute), RunID: "run-2"},
		},
	}
	run3 := &Run{
		RunID:      "run-3",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-1 * time.Minute), RunID: "run-3"},
		},
	}

	cfg := DetectorConfig{
		Threshold:      3,
		Window:         time.Hour,
		WindowRunCount: 10,
	}
	openProposals := make(map[string]bool)

	patterns := DetectStumblePatterns([]*Run{run1, run2, run3}, cfg, clk, openProposals)

	// Should fire exactly one pattern: (incus-container, timeout) with 3 distinct runs.
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}

	p := patterns[0]
	if p.Domain != "incus-container" {
		t.Errorf("domain: expected incus-container, got %s", p.Domain)
	}
	if p.SignalType != StumbleTimeout {
		t.Errorf("signal_type: expected timeout, got %s", p.SignalType)
	}
	if p.Count != 3 {
		t.Errorf("count: expected 3, got %d", p.Count)
	}
	if len(p.EvidenceRunIDs) != 3 {
		t.Errorf("evidence_run_ids: expected 3 run IDs, got %d: %v", len(p.EvidenceRunIDs), p.EvidenceRunIDs)
	}

	// Check that all three run IDs are present.
	expectedIDs := map[string]bool{"run-1": true, "run-2": true, "run-3": true}
	for _, id := range p.EvidenceRunIDs {
		if !expectedIDs[id] {
			t.Errorf("unexpected run ID in evidence: %s", id)
		}
		delete(expectedIDs, id)
	}
	if len(expectedIDs) > 0 {
		t.Errorf("missing run IDs in evidence: %v", expectedIDs)
	}
}

// TestGenomeDetector_BelowThresholdNoFire verifies that no pattern fires when
// the count is below the threshold.
func TestGenomeDetector_BelowThresholdNoFire(t *testing.T) {
	clk := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	// Only two timeouts; threshold is 3.
	run1 := &Run{
		RunID:      "run-1",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-5 * time.Minute)},
		},
	}
	run2 := &Run{
		RunID:      "run-2",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-3 * time.Minute)},
		},
	}

	cfg := DetectorConfig{
		Threshold:      3,
		Window:         time.Hour,
		WindowRunCount: 10,
	}
	openProposals := make(map[string]bool)

	patterns := DetectStumblePatterns([]*Run{run1, run2}, cfg, clk, openProposals)

	if len(patterns) != 0 {
		t.Fatalf("expected 0 patterns, got %d: %v", len(patterns), patterns)
	}
}

// TestGenomeDetector_OutsideWindowNoFire verifies that runs outside the window
// do not contribute to pattern counts.
func TestGenomeDetector_OutsideWindowNoFire(t *testing.T) {
	clk := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	// Three timeouts, but one is 2 hours old (outside 1h window).
	run1 := &Run{
		RunID:      "run-1",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-2 * time.Hour)}, // Outside window
		},
	}
	run2 := &Run{
		RunID:      "run-2",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-30 * time.Minute)},
		},
	}
	run3 := &Run{
		RunID:      "run-3",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-10 * time.Minute)},
		},
	}

	cfg := DetectorConfig{
		Threshold:      3,
		Window:         time.Hour, // Only runs in last hour
		WindowRunCount: 10,
	}
	openProposals := make(map[string]bool)

	patterns := DetectStumblePatterns([]*Run{run1, run2, run3}, cfg, clk, openProposals)

	// Should NOT fire because only 2 timeouts are in-window.
	if len(patterns) != 0 {
		t.Fatalf("expected 0 patterns (2 in-window, threshold 3), got %d", len(patterns))
	}
}

// TestGenomeDetector_DistinctRunsNotSignals verifies that distinct RUNS are counted,
// not distinct SIGNALS. If one run has 3 timeout signals, it counts as 1 distinct run.
func TestGenomeDetector_DistinctRunsNotSignals(t *testing.T) {
	clk := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	// One run with 3 timeout signals.
	run1 := &Run{
		RunID:      "run-1",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-5 * time.Minute)},
			{Type: StumbleTimeout, Ts: clk.Add(-4 * time.Minute)},
			{Type: StumbleTimeout, Ts: clk.Add(-3 * time.Minute)},
		},
	}

	cfg := DetectorConfig{
		Threshold:      3,
		Window:         time.Hour,
		WindowRunCount: 10,
	}
	openProposals := make(map[string]bool)

	patterns := DetectStumblePatterns([]*Run{run1}, cfg, clk, openProposals)

	// Should NOT fire because only 1 distinct run (not 3 signals).
	if len(patterns) != 0 {
		t.Fatalf("expected 0 patterns (1 distinct run, threshold 3), got %d", len(patterns))
	}
}

// TestGenomeDetector_DedupOpenProposal verifies that a pattern does not fire
// if a proposal is already open for the same (domain, signalType).
func TestGenomeDetector_DedupOpenProposal(t *testing.T) {
	clk := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	run1 := &Run{
		RunID:      "run-1",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-5 * time.Minute)},
		},
	}
	run2 := &Run{
		RunID:      "run-2",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-3 * time.Minute)},
		},
	}
	run3 := &Run{
		RunID:      "run-3",
		WorkerKind: "incus-container",
		StumbleSignals: []StumbleSignal{
			{Type: StumbleTimeout, Ts: clk.Add(-1 * time.Minute)},
		},
	}

	// Mark the proposal as open for this (domain, signalType).
	openProposals := map[string]bool{
		"incus-container::timeout": true,
	}

	cfg := DetectorConfig{
		Threshold:      3,
		Window:         time.Hour,
		WindowRunCount: 10,
	}

	patterns := DetectStumblePatterns([]*Run{run1, run2, run3}, cfg, clk, openProposals)

	// Should NOT fire because a proposal is open.
	if len(patterns) != 0 {
		t.Fatalf("expected 0 patterns (open proposal blocking), got %d", len(patterns))
	}
}

// TestMutationProposal_RejectsProtectedTarget verifies that NewMutationProposal
// rejects protected targets and records an audit entry (AC-3).
func TestMutationProposal_RejectsProtectedTarget(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	// Try to create a proposal for a protected target (hard_budget_guardrails).
	// Use the string representation as we don't have an enum for protected targets.
	mp, err := NewMutationProposal(
		"mut-001",
		"incus-container",
		MutationTarget("hard_budget_guardrails"), // This will fail the guard
		"new content",
		"rationale",
		[]string{"run-1", "run-2"},
		audioLog,
	)

	if err == nil {
		t.Fatalf("expected error for protected target, got nil")
	}
	if mp != nil {
		t.Fatalf("expected nil proposal for protected target, got %+v", mp)
	}

	// Check that the rejection was audited.
	entries := audioLog.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].Kind != AuditKindMutation {
		t.Errorf("audit kind: expected mutation, got %s", entries[0].Kind)
	}
	if !stringContains(entries[0].Detail, "protected-invariant") {
		t.Errorf("audit detail: expected 'protected-invariant', got %s", entries[0].Detail)
	}
}

// TestMutationProposal_RejectsInvalidTarget verifies that invalid mutation targets are rejected.
func TestMutationProposal_RejectsInvalidTarget(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	mp, err := NewMutationProposal(
		"mut-001",
		"incus-container",
		MutationTarget("invalid_target"),
		"content",
		"rationale",
		[]string{},
		audioLog,
	)

	if err == nil {
		t.Fatalf("expected error for invalid target, got nil")
	}
	if mp != nil {
		t.Fatalf("expected nil proposal, got %+v", mp)
	}
}

// TestMutationProposal_ValidTargetsAccepted verifies that all six valid targets are accepted.
func TestMutationProposal_ValidTargetsAccepted(t *testing.T) {
	validTargets := []MutationTarget{
		MutationTargetPromptTweak,
		MutationTargetRoutingHeuristic,
		MutationTargetProviderModelHeuristic,
		MutationTargetBudgetEscalation,
		MutationTargetExecutionPolicy,
		MutationTargetThreadHandoffTemplate,
	}

	for _, target := range validTargets {
		audioLog := NewMemoryAuditLog()
		mp, err := NewMutationProposal(
			"mut-001",
			"incus-container",
			target,
			"content",
			"rationale",
			[]string{},
			audioLog,
		)

		if err != nil {
			t.Errorf("target %s: unexpected error: %v", target, err)
		}
		if mp == nil {
			t.Errorf("target %s: expected non-nil proposal", target)
		}
		if mp != nil && mp.Target != target {
			t.Errorf("target %s: proposal target mismatch: %s", target, mp.Target)
		}
	}
}

// TestMutationProposal_PromoteReguardsProtectedTarget verifies that Promote() re-checks
// protected targets (AC-3 dual enforcement: construction AND promotion) (ISSUE 1).
func TestMutationProposal_PromoteReguardsProtectedTarget(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	// Create a valid proposal first, then mutate its Target to a protected one
	// (simulating a post-construction mutation or adversarial input).
	mp, err := NewMutationProposal(
		"mut-001",
		"incus-container",
		MutationTargetPromptTweak,
		"content",
		"rationale",
		[]string{},
		audioLog,
	)
	if err != nil {
		t.Fatalf("proposal creation failed: %v", err)
	}

	// Simulate adversarial mutation of the target field post-construction.
	mp.Target = MutationTarget("hard_budget_guardrails")

	// Attempt to promote: should fail with protected-target guard.
	_, err = mp.Promote(2, 1, audioLog)
	if err == nil {
		t.Fatalf("promote with protected target should fail, got nil error")
	}
	if !stringContains(err.Error(), "protected") {
		t.Errorf("promote error message should mention protected: %v", err)
	}

	// Verify that the rejection was audited.
	entries := audioLog.Entries()
	foundRejection := false
	for _, entry := range entries {
		if entry.Kind == AuditKindMutation && stringContains(entry.Detail, "protected-invariant") && stringContains(entry.Detail, "promotion") {
			foundRejection = true
			break
		}
	}
	if !foundRejection {
		t.Errorf("expected audit entry for protected-target rejection at promotion, got entries: %v", entries)
	}
}

// TestMutationProposal_PromoteValidatesIncomingStatus verifies that Promote() validates
// the proposal is in candidate status before promotion (AC-4 state machine guard) (ISSUE 2).
func TestMutationProposal_PromoteValidatesIncomingStatus(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	// Create a proposal in candidate status (default).
	mp, _ := NewMutationProposal(
		"mut-001",
		"incus-container",
		MutationTargetPromptTweak,
		"content",
		"rationale",
		[]string{},
		audioLog,
	)

	// Manually mutate the status to rejected (simulating an invalid state).
	mp.Status = GenomeStatusRejected

	// Attempt to promote: should fail with status guard.
	_, err := mp.Promote(2, 1, audioLog)
	if err == nil {
		t.Fatalf("promote from rejected status should fail, got nil error")
	}
	if !stringContains(err.Error(), "candidate") {
		t.Errorf("promote error should mention candidate status: %v", err)
	}
}

// TestMutationProposal_PromoteHappyPathFromCandidate verifies that a proposal in
// candidate status CAN be promoted (happy path).
func TestMutationProposal_PromoteHappyPathFromCandidate(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	mp, _ := NewMutationProposal(
		"mut-001",
		"incus-container",
		MutationTargetPromptTweak,
		"content",
		"rationale",
		[]string{"run-1", "run-2"},
		audioLog,
	)

	// Status should be candidate by default.
	if mp.Status != GenomeStatusCandidate {
		t.Fatalf("expected candidate status, got %s", mp.Status)
	}

	// Promote should succeed.
	entry, err := mp.Promote(2, 1, audioLog)
	if err != nil {
		t.Fatalf("promote of candidate proposal failed: %v", err)
	}
	if entry.Status != GenomeStatusActive {
		t.Errorf("promoted entry status should be active, got %s", entry.Status)
	}
}

// TestMutationLifecycle_RejectValidatesIncomingStatus verifies that Reject() only acts on candidates.
func TestMutationLifecycle_RejectValidatesIncomingStatus(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	mp, _ := NewMutationProposal(
		"mut-001",
		"incus-container",
		MutationTargetPromptTweak,
		"content",
		"rationale",
		[]string{},
		audioLog,
	)

	// Manually set to promoted (invalid for reject).
	mp.Status = GenomeStatusActive

	// Reject should fail.
	err := mp.Reject(audioLog)
	if err == nil {
		t.Fatalf("reject of active proposal should fail, got nil error")
	}
	if !stringContains(err.Error(), "candidate") {
		t.Errorf("reject error should mention candidate status: %v", err)
	}
}

// TestMutationLifecycle_KeepValidatesIncomingStatus verifies that Keep() only acts on candidates.
func TestMutationLifecycle_KeepValidatesIncomingStatus(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	mp, _ := NewMutationProposal(
		"mut-001",
		"incus-container",
		MutationTargetPromptTweak,
		"content",
		"rationale",
		[]string{},
		audioLog,
	)

	// Manually set to rejected (invalid for keep).
	mp.Status = GenomeStatusRejected

	// Keep should fail.
	err := mp.Keep(audioLog)
	if err == nil {
		t.Fatalf("keep of rejected proposal should fail, got nil error")
	}
	if !stringContains(err.Error(), "candidate") {
		t.Errorf("keep error should mention candidate status: %v", err)
	}
}

// TestMutationProposal_TrialRunIDIsSet verifies that TrialRunID is wired when a trial begins.
func TestMutationProposal_TrialRunIDIsSet(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	mp, _ := NewMutationProposal(
		"mut-001",
		"incus-container",
		MutationTargetPromptTweak,
		"content",
		"rationale",
		[]string{},
		audioLog,
	)

	// Initially, TrialRunID should be empty.
	if mp.TrialRunID != "" {
		t.Errorf("TrialRunID should be empty initially, got %q", mp.TrialRunID)
	}

	// When a trial begins, set TrialRunID.
	trialRunID := "run-trial-001"
	mp.SetTrialRunID(trialRunID)

	if mp.TrialRunID != trialRunID {
		t.Errorf("TrialRunID not set correctly: expected %q, got %q", trialRunID, mp.TrialRunID)
	}
}

// TestMutationLifecycle_RevertRestoresPriorVersionAndSource verifies that Revert() restores
// the prior entry's content, version, and SOURCE (not mislabeling provenance) (ISSUE 4).
func TestMutationLifecycle_RevertRestoresPriorVersionAndSource(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	// Create a prior entry with source=bootstrap.
	priorEntry := &GenomeEntry{
		Version:      1,
		Content:      "bootstrap prompt",
		ContentHash:  ContentSHA256("bootstrap prompt"),
		Target:       MutationTargetPromptTweak,
		Domain:       "incus-container",
		Source:       GenomeSourceBootstrap,
		Status:       GenomeStatusActive,
	}

	// Create a promoted mutation.
	mp, _ := NewMutationProposal(
		"mut-001",
		"incus-container",
		MutationTargetPromptTweak,
		"promoted prompt",
		"rationale",
		[]string{},
		audioLog,
	)

	// Revert the mutation.
	revertedEntry, err := mp.Revert(priorEntry, audioLog)
	if err != nil {
		t.Fatalf("revert failed: %v", err)
	}

	// Verify the reverted entry restores the prior content and source.
	if revertedEntry.Content != priorEntry.Content {
		t.Errorf("reverted content: expected %q, got %q", priorEntry.Content, revertedEntry.Content)
	}
	if revertedEntry.ContentHash != priorEntry.ContentHash {
		t.Errorf("reverted content hash mismatch")
	}
	if revertedEntry.Source != GenomeSourceBootstrap {
		t.Errorf("reverted source: expected bootstrap, got %s", revertedEntry.Source)
	}
	if revertedEntry.Status != GenomeStatusReverted {
		t.Errorf("reverted status: expected reverted, got %s", revertedEntry.Status)
	}

	// Verify the reverted entry version is > prior but content matches prior.
	if revertedEntry.Version <= priorEntry.Version {
		t.Errorf("reverted version should be > prior: %d vs %d", revertedEntry.Version, priorEntry.Version)
	}
}

// TestMutationLifecycle_RevertFromPromotedEntry verifies that Revert can restore from
// a previously promoted entry and maintains version history.
func TestMutationLifecycle_RevertFromPromotedEntry(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	// Create two proposals: one gets promoted, then we revert it.
	mp1, _ := NewMutationProposal(
		"mut-001",
		"incus-container",
		MutationTargetPromptTweak,
		"version 1 content",
		"initial mutation",
		[]string{},
		audioLog,
	)

	promotedEntry, _ := mp1.Promote(2, 1, audioLog)

	// Now create another proposal and promote it.
	mp2, _ := NewMutationProposal(
		"mut-002",
		"incus-container",
		MutationTargetPromptTweak,
		"version 3 content",
		"second mutation",
		[]string{},
		audioLog,
	)

	mp2.Propose(audioLog)

	// Revert mp2 back to the previous promoted entry (version 2).
	revertedEntry, err := mp2.Revert(promotedEntry, audioLog)
	if err != nil {
		t.Fatalf("revert failed: %v", err)
	}

	// The reverted entry should restore version 2's content.
	if revertedEntry.Content != promotedEntry.Content {
		t.Errorf("reverted content should match prior promoted entry: expected %q, got %q",
			promotedEntry.Content, revertedEntry.Content)
	}
	if revertedEntry.ContentHash != promotedEntry.ContentHash {
		t.Errorf("reverted content hash should match prior entry")
	}
}

// TestMutationLifecycle_PromoteRejectRevert exercises the full AC-4 state machine:
// propose → trial/measure → promote/reject/revert (design note §5).
func TestMutationLifecycle_PromoteRejectRevert(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	// Create a proposal (candidate).
	mp, err := NewMutationProposal(
		"mut-001",
		"incus-container",
		MutationTargetPromptTweak,
		"improved prompt",
		"timeout pattern detected",
		[]string{"run-1", "run-2", "run-3"},
		audioLog,
	)
	if err != nil {
		t.Fatalf("proposal creation failed: %v", err)
	}

	// --- Propose (record candidate in audit) ---
	if err := mp.Propose(audioLog); err != nil {
		t.Fatalf("propose failed: %v", err)
	}
	entries := audioLog.Entries()
	if len(entries) != 1 {
		t.Fatalf("after propose: expected 1 audit entry, got %d", len(entries))
	}

	// --- Promote (active, new version) ---
	entry, err := mp.Promote(2, 1, audioLog) // version 2, prior was 1
	if err != nil {
		t.Fatalf("promote failed: %v", err)
	}
	if entry.Status != GenomeStatusActive {
		t.Errorf("promote: status should be active, got %s", entry.Status)
	}
	if entry.Source != GenomeSourcePromoted {
		t.Errorf("promote: source should be promoted, got %s", entry.Source)
	}
	if entry.Version != 2 {
		t.Errorf("promote: version should be 2, got %d", entry.Version)
	}
	if entry.PriorVersion != 1 {
		t.Errorf("promote: prior_version should be 1, got %d", entry.PriorVersion)
	}

	// Check audit trail.
	entries = audioLog.Entries()
	if len(entries) != 2 {
		t.Fatalf("after promote: expected 2 audit entries, got %d", len(entries))
	}
	if entries[1].Kind != AuditKindMutation || !stringContains(entries[1].Detail, "promote") {
		t.Errorf("promote: missing promote audit entry: %+v", entries[1])
	}

	// --- Reject (alternative path) ---
	mp2, _ := NewMutationProposal(
		"mut-002",
		"incus-container",
		MutationTargetRoutingHeuristic,
		"different heuristic",
		"another pattern",
		[]string{},
		audioLog,
	)
	if err := mp2.Reject(audioLog); err != nil {
		t.Fatalf("reject failed: %v", err)
	}
	if mp2.Status != GenomeStatusRejected {
		t.Errorf("reject: status should be rejected, got %s", mp2.Status)
	}

	// --- Revert (restore prior after regression) ---
	priorEntry := &GenomeEntry{
		Version:    1,
		Content:    "original prompt",
		ContentHash: ContentSHA256("original prompt"),
		Target:    MutationTargetPromptTweak,
		Domain:    "incus-container",
		Source:    GenomeSourceBootstrap,
		Status:    GenomeStatusActive,
	}

	mp3, _ := NewMutationProposal(
		"mut-003",
		"incus-container",
		MutationTargetPromptTweak,
		"regressed prompt",
		"mutation regressed",
		[]string{},
		audioLog,
	)
	revertedEntry, err := mp3.Revert(priorEntry, audioLog)
	if err != nil {
		t.Fatalf("revert failed: %v", err)
	}
	if revertedEntry.Status != GenomeStatusReverted {
		t.Errorf("revert: status should be reverted, got %s", revertedEntry.Status)
	}
	if revertedEntry.Version <= priorEntry.Version {
		t.Errorf("revert: new version should be > prior version: %d vs %d", revertedEntry.Version, priorEntry.Version)
	}

	// Verify full audit trail.
	allEntries := audioLog.Entries()
	if len(allEntries) < 4 {
		t.Fatalf("expected at least 4 audit entries, got %d", len(allEntries))
	}

	// Audit should have: propose, promote, reject, revert
	// (the create operations happen during NewMutationProposal with no audit on success)
}

// TestMutationLifecycle_RevertRestoresPriorContent verifies that after revert,
// the prior version's content is available in the reverted entry (AC-4).
func TestMutationLifecycle_RevertRestoresPriorContent(t *testing.T) {
	audioLog := NewMemoryAuditLog()

	priorContent := "original prompt v1"
	regressedContent := "regressed prompt v2"

	priorEntry := &GenomeEntry{
		Version:     1,
		Content:     priorContent,
		ContentHash: ContentSHA256(priorContent),
		Target:      MutationTargetPromptTweak,
		Domain:      "incus-container",
		Source:      GenomeSourceBootstrap,
		Status:      GenomeStatusActive,
	}

	regressedProposal, _ := NewMutationProposal(
		"mut-regressed",
		"incus-container",
		MutationTargetPromptTweak,
		regressedContent,
		"regression detected",
		[]string{},
		audioLog,
	)

	revertedEntry, err := regressedProposal.Revert(priorEntry, audioLog)
	if err != nil {
		t.Fatalf("revert failed: %v", err)
	}

	// Verify the reverted entry contains the PRIOR content, not the regressed content
	if revertedEntry.Content != priorContent {
		t.Errorf("reverted entry should contain prior content: expected %q, got %q", priorContent, revertedEntry.Content)
	}

	// Verify the content hash matches the prior (original) content
	if revertedEntry.ContentHash != ContentSHA256(priorContent) {
		t.Errorf("reverted entry content_hash should match prior content hash")
	}

	// Verify PriorVersion points back to the original
	if revertedEntry.PriorVersion != priorEntry.Version {
		t.Errorf("reverted entry prior_version: expected %d, got %d", priorEntry.Version, revertedEntry.PriorVersion)
	}
}
