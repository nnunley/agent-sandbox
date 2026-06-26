package main

import (
	"time"
)

// DetectorConfig holds configuration for stumble-pattern detection (STORY-0032 design note §1).
type DetectorConfig struct {
	Threshold   int           // Minimum count of distinct runs with the same stumble type to fire (default 3)
	Window      time.Duration // Look-back bound by wall-clock time (alternative: WindowRunCount)
	WindowRunCount int        // Look-back bound by run count; if both set, whichever is tighter
}

// StumblePattern is one detected pattern: a recurrence of the same StumbleType across
// distinct recent runs within a bounded window (STORY-0032 design note §3, AC-4).
type StumblePattern struct {
	Domain          string   `json:"domain"`      // worker_kind the pattern applies to (e.g., "incus-container")
	SignalType      StumbleType `json:"signal_type"` // The stumble type being counted
	Count           int      `json:"count"`       // Number of distinct runs with this type
	Window          string   `json:"window"`      // Human-readable window description
	EvidenceRunIDs  []string `json:"evidence_run_ids"` // The run IDs that satisfied the pattern (AC-4 trail)
}

// DetectStumblePatterns is a pure function that identifies recurrence patterns in recent runs.
// It returns zero or more StumblePattern structs, one per (domain, signalType) pair that fires.
// The detector is unit-testable without I/O: the caller supplies the run history.
//
// Inputs:
//   - runs: recent Run slice, assumed ordered newest-first (or in any order; we filter by window)
//   - cfg: DetectorConfig with Threshold (>= count to fire) and Window constraints
//   - now: injected clock (for window boundary calculation)
//   - openProposals: map keyed by (domain, signalType) string; if present, pattern does NOT fire (de-dup)
//
// Returns: slice of StumblePattern, one per firing (domain, signalType) pair.
// EvidenceRunIDs are the run IDs that contributed to the pattern.
//
// Firing rule (design note §1): a pattern fires when count(distinct runs in window with signalType) >= threshold.
func DetectStumblePatterns(
	runs []*Run,
	cfg DetectorConfig,
	now time.Time,
	openProposals map[string]bool,
) []StumblePattern {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 3 // Default threshold
	}
	if cfg.Window <= 0 && cfg.WindowRunCount <= 0 {
		cfg.Window = time.Hour // Default 1h
		cfg.WindowRunCount = 10 // Default 10 runs
	}

	// Group runs by domain, then by signal type.
	// Map: domain -> signalType -> (count, evidenceRunIDs)
	type signalTypeKey struct {
		domain     string
		signalType StumbleType
	}
	counts := make(map[signalTypeKey]struct {
		count         int
		evidenceRunIDs []string
	})

	// Filter runs into window. We use the timestamps from stumble signals to determine window membership.
	windowStart := now.Add(-cfg.Window)
	var runCount int
	for _, run := range runs {
		// Skip runs with no stumble signals.
		if len(run.StumbleSignals) == 0 {
			continue
		}

		// Check if this run is within the window (using any of its signal timestamps).
		inWindow := false
		if cfg.Window <= 0 {
			inWindow = true // No time window constraint
		} else {
			for _, sig := range run.StumbleSignals {
				if sig.Ts.After(windowStart) {
					inWindow = true
					break
				}
			}
		}

		if !inWindow {
			continue // Run is outside time window
		}

		// Check run-count window.
		runCount++
		if cfg.WindowRunCount > 0 && runCount > cfg.WindowRunCount {
			break // Beyond run-count window
		}

		// Extract domain. v1 uses worker_kind; empty domain is wildcard (for tests/operators).
		domain := run.WorkerKind

		// For each signal type in this run's stumble signals, record it (distinct per type).
		seenTypes := make(map[StumbleType]bool)
		for _, sig := range run.StumbleSignals {
			if !seenTypes[sig.Type] {
				key := signalTypeKey{domain, sig.Type}
				v := counts[key]
				v.count++
				if !stringInSlice(v.evidenceRunIDs, run.RunID) {
					v.evidenceRunIDs = append(v.evidenceRunIDs, run.RunID)
				}
				counts[key] = v
				seenTypes[sig.Type] = true
			}
		}
	}

	// Fire patterns where count >= threshold and no open proposal exists.
	var patterns []StumblePattern
	for key, v := range counts {
		if v.count >= cfg.Threshold {
			// De-dup: check if a proposal is already open for this (domain, signalType).
			proposalKey := proposeKey(key.domain, key.signalType)
			if openProposals[proposalKey] {
				continue // Proposal already open; do not fire again
			}

			patterns = append(patterns, StumblePattern{
				Domain:         key.domain,
				SignalType:     key.signalType,
				Count:          v.count,
				Window:         "last 10 runs or 1h, whichever tighter",
				EvidenceRunIDs: v.evidenceRunIDs,
			})
		}
	}

	return patterns
}

// proposeKey returns a map key for a (domain, signalType) proposal pair (for de-dup).
func proposeKey(domain string, signalType StumbleType) string {
	return domain + "::" + string(signalType)
}

// stringInSlice reports whether needle is in haystack.
func stringInSlice(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
