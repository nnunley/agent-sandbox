package main

import (
	"sync"
	"testing"
	"time"
)

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newClock(base time.Time) (now func() time.Time, advance func(time.Duration)) {
	t := base
	var mu sync.Mutex
	return func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return t
		}, func(d time.Duration) {
			mu.Lock()
			defer mu.Unlock()
			t = t.Add(d)
		}
}

// STORY-0033 AC-1/AC-3: free key, claim, reuse decisions.
func TestDecideReuse_FreeKey(t *testing.T) {
	now, _ := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "main"}

	if got := r.DecideReuse(key, "A"); got != ReuseFree {
		t.Fatalf("want ReuseFree, got %q", got)
	}
	_, ok := r.Claim(key, "A", "tok-a", time.Hour)
	if !ok {
		t.Fatal("Claim on free key should succeed")
	}
}

func TestDecideReuse_SameThread(t *testing.T) {
	now, _ := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "feat"}

	r.Claim(key, "A", "tok-a", time.Hour)

	if got := r.DecideReuse(key, "A"); got != ReuseContinue {
		t.Fatalf("want ReuseContinue, got %q", got)
	}
}

func TestClaim_SameThreadRenews(t *testing.T) {
	now, advance := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "feat"}

	r.Claim(key, "A", "tok-a", time.Hour)

	// Advance near expiry, then renew.
	advance(50 * time.Minute)
	_, ok := r.Claim(key, "A", "tok-a2", time.Hour)
	if !ok {
		t.Fatal("same-thread re-Claim should renew (ok=true)")
	}

	// Advance past the original expiry but within the renewed TTL.
	advance(20 * time.Minute)
	c, exists := r.ActiveClaim(key)
	if !exists {
		t.Fatal("claim should still be active after renewal")
	}
	if c.ThreadID != "A" {
		t.Fatalf("want ThreadID A, got %q", c.ThreadID)
	}
}

func TestClaim_DifferentThreadBlocked(t *testing.T) {
	now, _ := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "feat"}

	r.Claim(key, "A", "tok-a", time.Hour)

	_, ok := r.Claim(key, "B", "tok-b", time.Hour)
	if ok {
		t.Fatal("Claim by different thread over active claim should fail (ok=false)")
	}

	// State must be unchanged: A still holds the claim.
	c, exists := r.ActiveClaim(key)
	if !exists || c.ThreadID != "A" {
		t.Fatalf("claim should still belong to A, got exists=%v threadID=%q", exists, c.ThreadID)
	}
}

func TestDecideReuse_DifferentThread(t *testing.T) {
	now, _ := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "feat"}

	r.Claim(key, "A", "tok-a", time.Hour)

	if got := r.DecideReuse(key, "B"); got != ReuseSupersede {
		t.Fatalf("want ReuseSupersede, got %q", got)
	}
}

// Expiry tests.
func TestActiveClaim_Expired(t *testing.T) {
	now, advance := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "main"}

	r.Claim(key, "A", "tok-a", time.Hour)
	advance(2 * time.Hour)

	_, ok := r.ActiveClaim(key)
	if ok {
		t.Fatal("expired claim should not be returned")
	}
}

func TestDecideReuse_ExpiredBecomeFree(t *testing.T) {
	now, advance := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "main"}

	r.Claim(key, "A", "tok-a", time.Hour)
	advance(2 * time.Hour)

	if got := r.DecideReuse(key, "B"); got != ReuseFree {
		t.Fatalf("expired claim should yield ReuseFree, got %q", got)
	}
}

// Release test.
func TestRelease(t *testing.T) {
	now, _ := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "main"}

	r.Claim(key, "A", "tok-a", time.Hour)
	r.Release(key)

	_, ok := r.ActiveClaim(key)
	if ok {
		t.Fatal("after Release, ActiveClaim should return nothing")
	}
}

// STORY-0030 AC-2: Supersede with empty reason must fail.
func TestSupersede_EmptyReasonFails(t *testing.T) {
	now, _ := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "feat"}

	r.Claim(key, "A", "tok-a", time.Hour)

	_, _, ok := r.Supersede(key, "B", "tok-b", "", time.Hour)
	if ok {
		t.Fatal("Supersede with empty reason should fail")
	}

	// Claim must be unchanged.
	c, exists := r.ActiveClaim(key)
	if !exists || c.ThreadID != "A" {
		t.Fatalf("claim should still belong to A after failed Supersede")
	}
}

// STORY-0030 AC-2/AC-3: Supersede with non-empty reason.
func TestSupersede_Success(t *testing.T) {
	now, _ := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "feat"}

	r.Claim(key, "A", "tok-a", time.Hour)

	priorID, stumble, ok := r.Supersede(key, "B", "tok-b", "prior approach hit dead end", time.Hour)
	if !ok {
		t.Fatal("Supersede with reason should succeed")
	}
	if priorID != "A" {
		t.Fatalf("want priorThreadID A, got %q", priorID)
	}

	// Active claim now belongs to B.
	c, exists := r.ActiveClaim(key)
	if !exists {
		t.Fatal("ActiveClaim should exist after Supersede")
	}
	if c.ThreadID != "B" {
		t.Fatalf("want ThreadID B, got %q", c.ThreadID)
	}

	// STORY-0030 AC-3: stumble signal.
	if stumble.Type != StumbleDuplicateWork {
		t.Fatalf("want StumbleDuplicateWork, got %q", stumble.Type)
	}
	if stumble.EvidenceSummary != "prior approach hit dead end" {
		t.Fatalf("unexpected EvidenceSummary: %q", stumble.EvidenceSummary)
	}
	if stumble.Ts.IsZero() {
		t.Fatal("stumble Ts must be set")
	}
}

// STORY-0030 AC-3: Stumble captured on Run via AddStumble.
func TestSupersede_StumbleLandsOnRun(t *testing.T) {
	now, _ := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "feat"}

	r.Claim(key, "A", "tok-a", time.Hour)
	_, stumble, _ := r.Supersede(key, "B", "tok-b", "reinvention reason", time.Hour)

	run := &Run{RunID: "r", ThreadID: "B"}
	run.AddStumble(stumble)

	if len(run.StumbleSignals) != 1 {
		t.Fatalf("want 1 stumble, got %d", len(run.StumbleSignals))
	}
	sig := run.StumbleSignals[0]
	if sig.RunID != "r" {
		t.Fatalf("want RunID r, got %q", sig.RunID)
	}
	if sig.Type != StumbleDuplicateWork {
		t.Fatalf("want StumbleDuplicateWork, got %q", sig.Type)
	}
	if sig.EvidenceSummary != "reinvention reason" {
		t.Fatalf("unexpected EvidenceSummary: %q", sig.EvidenceSummary)
	}
}

// Supersede when no different-thread claim exists → ok=false.
func TestSupersede_NoActiveClaim(t *testing.T) {
	now, _ := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "feat"}

	_, _, ok := r.Supersede(key, "B", "tok-b", "some reason", time.Hour)
	if ok {
		t.Fatal("Supersede on free key should fail")
	}
}

func TestSupersede_SameThread(t *testing.T) {
	now, _ := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "feat"}

	r.Claim(key, "A", "tok-a", time.Hour)

	// Supersede by same thread (no different-thread claim) → ok=false.
	_, _, ok := r.Supersede(key, "A", "tok-a2", "reason", time.Hour)
	if ok {
		t.Fatal("Supersede by same thread should fail — no different-thread claim exists")
	}
}

// Concurrency-safe: -race test.
func TestWorkspaceRegistry_ConcurrentAccess(t *testing.T) {
	now, _ := newClock(epoch)
	r := NewWorkspaceRegistry(now)
	key := WorkspaceKey{Repo: "repo", Branch: "concurrent"}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			threadID := "A"
			if n%2 == 0 {
				threadID = "B"
			}
			r.Claim(key, threadID, "tok", time.Hour)
			r.DecideReuse(key, threadID)
			r.ActiveClaim(key)
			r.Release(key)
		}(i)
	}
	wg.Wait()
}
