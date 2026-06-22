package main

import "fmt"

// BackendFactory resolves an IsolationTier to the Runner that implements it. The daemon
// resolves the tier from the validated template (Policy.TierFor) and asks the factory for
// the matching Runner, then drives it through the unchanged Runner interface. Selecting the
// backend OUTSIDE Runner.Run is deliberate: every backend keeps the same signature, so
// ITER-0005b's microVM (Hard) and nspawn (Fast) runners graft in by registering here —
// no interface or daemon change. See docs/plans/2026-06-21-iter0005-backend-tier-design.md.
type BackendFactory interface {
	// SelectRunner returns the Runner registered for tier, or an error naming the tier
	// when no backend is registered for it (the ITER-0005b graft point).
	SelectRunner(tier IsolationTier) (Runner, error)
}

// staticBackendFactory maps tiers to pre-constructed runners.
//
// ITER-0005b registers the real isolation-tier backends in the serve entrypoint:
// TierFast → NspawnRunner (in-guest nspawn --ephemeral) and TierHard → FirecrackerRunner
// (per-task microVM). New backends slot in by registering here — no change to this type
// or the daemon.
type staticBackendFactory struct {
	byTier map[IsolationTier]Runner
}

// newStaticBackendFactory builds a factory from a tier→runner registry. The map is copied
// so later mutation of the caller's map cannot change selection.
func newStaticBackendFactory(byTier map[IsolationTier]Runner) *staticBackendFactory {
	cp := make(map[IsolationTier]Runner, len(byTier))
	for tier, r := range byTier {
		cp[tier] = r
	}
	return &staticBackendFactory{byTier: cp}
}

// SelectRunner implements BackendFactory.
func (f *staticBackendFactory) SelectRunner(tier IsolationTier) (Runner, error) {
	r, ok := f.byTier[tier]
	if !ok || r == nil {
		return nil, fmt.Errorf("no backend registered for isolation tier %q", tier)
	}
	return r, nil
}
