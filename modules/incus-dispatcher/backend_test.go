package main

import (
	"strings"
	"testing"
)

// STORY-0004 AC-1 / STORY-0017 AC-1: the BackendFactory abstracts tier→backend selection.
// SelectRunner returns the Runner registered for a tier (reuses the suite's fakeRunner;
// distinct instances are compared by pointer identity).
func TestStaticBackendFactory_SelectsRegisteredRunner(t *testing.T) {
	fast := &fakeRunner{}
	hard := &fakeRunner{}
	var f BackendFactory = newStaticBackendFactory(map[IsolationTier]Runner{
		TierFast: fast,
		TierHard: hard,
	})

	got, err := f.SelectRunner(TierFast)
	if err != nil {
		t.Fatalf("SelectRunner(fast) err = %v", err)
	}
	if got != Runner(fast) {
		t.Errorf("SelectRunner(fast) returned the wrong runner")
	}
	got, err = f.SelectRunner(TierHard)
	if err != nil || got != Runner(hard) {
		t.Errorf("SelectRunner(hard) = %v, %v; want the hard runner", got, err)
	}
}

// The graft point for ITER-0005b: a tier with no registered backend yields a descriptive
// error naming the tier (so a microVM/nspawn backend can be slotted in later without
// touching the interface or the daemon).
func TestStaticBackendFactory_UnregisteredTierErrors(t *testing.T) {
	f := newStaticBackendFactory(map[IsolationTier]Runner{
		TierFast: &fakeRunner{},
	})
	_, err := f.SelectRunner(TierHard)
	if err == nil {
		t.Fatal("SelectRunner(hard) with no hard backend = nil, want error")
	}
	if !strings.Contains(err.Error(), string(TierHard)) {
		t.Errorf("error %q does not name the tier %q", err.Error(), TierHard)
	}
}
