package queue

import (
	"errors"
	"testing"
	"time"
)

// fakeClock is a controllable time source for lease / not-before tests.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestQueue() (*MemoryQueue, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)}
	q := NewMemoryQueueWithClock(clk.now)
	return q, clk
}

func TestPushAndClaim(t *testing.T) {
	q, _ := newTestQueue()
	id, err := q.Push(Directive{Intent: "fix bug", Template: "go", Importance: ImportanceNormal})
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if id == "" {
		t.Fatal("push returned empty id")
	}
	if p, c := q.Len(); p != 1 || c != 0 {
		t.Fatalf("len before claim = %d pending %d claimed, want 1/0", p, c)
	}
	d, lease, err := q.Claim("worker-1", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if d.ID != id {
		t.Fatalf("claimed id %q want %q", d.ID, id)
	}
	if lease.DirectiveID != id || lease.Token == "" {
		t.Fatalf("bad lease %+v", lease)
	}
	if p, c := q.Len(); p != 0 || c != 1 {
		t.Fatalf("len after claim = %d/%d, want 0/1", p, c)
	}
}

func TestClaimEmpty(t *testing.T) {
	q, _ := newTestQueue()
	if _, _, err := q.Claim("w", time.Minute); !errors.Is(err, ErrEmpty) {
		t.Fatalf("claim on empty = %v, want ErrEmpty", err)
	}
}

func TestClaimRespectsPriority(t *testing.T) {
	q, _ := newTestQueue()
	q.Push(Directive{Intent: "low", Importance: ImportanceLow})
	q.Push(Directive{Intent: "high", Importance: ImportanceHigh})
	q.Push(Directive{Intent: "normal", Importance: ImportanceNormal})
	d, _, _ := q.Claim("w", time.Minute)
	if d.Intent != "high" {
		t.Fatalf("first claim = %q, want high", d.Intent)
	}
	d, _, _ = q.Claim("w", time.Minute)
	if d.Intent != "normal" {
		t.Fatalf("second claim = %q, want normal", d.Intent)
	}
}

func TestClaimRespectsNotBefore(t *testing.T) {
	q, clk := newTestQueue()
	q.Push(Directive{Intent: "deferred", Importance: ImportanceHigh, NotBefore: clk.now().Add(time.Hour)})
	q.Push(Directive{Intent: "ready", Importance: ImportanceLow})
	// High-importance one is not eligible yet; the eligible low one wins.
	d, _, err := q.Claim("w", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if d.Intent != "ready" {
		t.Fatalf("claim = %q, want ready (deferred not yet eligible)", d.Intent)
	}
	// Nothing else eligible now.
	if _, _, err := q.Claim("w", time.Minute); !errors.Is(err, ErrEmpty) {
		t.Fatalf("second claim = %v, want ErrEmpty", err)
	}
	// Advance past the not-before; the deferred one becomes claimable.
	clk.advance(2 * time.Hour)
	d, _, err = q.Claim("w", time.Minute)
	if err != nil || d.Intent != "deferred" {
		t.Fatalf("after advance claim = %q err %v, want deferred", d.Intent, err)
	}
}

func TestDoneRemoves(t *testing.T) {
	q, _ := newTestQueue()
	q.Push(Directive{Intent: "x"})
	_, lease, _ := q.Claim("w", time.Minute)
	if err := q.Done(lease); err != nil {
		t.Fatalf("done: %v", err)
	}
	if p, c := q.Len(); p != 0 || c != 0 {
		t.Fatalf("len after done = %d/%d, want 0/0", p, c)
	}
	// Stale lease op now fails.
	if err := q.Done(lease); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("done on stale lease = %v, want ErrLeaseLost", err)
	}
}

func TestRequeueIncrementsAttempts(t *testing.T) {
	q, _ := newTestQueue()
	q.Push(Directive{Intent: "retry-me", Importance: ImportanceNormal})
	d, lease, _ := q.Claim("w", time.Minute)
	if d.Attempts != 0 {
		t.Fatalf("initial attempts = %d, want 0", d.Attempts)
	}
	if err := q.Requeue(lease, time.Time{}); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if p, c := q.Len(); p != 1 || c != 0 {
		t.Fatalf("len after requeue = %d/%d, want 1/0", p, c)
	}
	d2, _, err := q.Claim("w", time.Minute)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if d2.Attempts != 1 {
		t.Fatalf("attempts after requeue+claim = %d, want 1", d2.Attempts)
	}
}

func TestTouchAndReap(t *testing.T) {
	q, clk := newTestQueue()
	q.Push(Directive{Intent: "leased"})
	_, lease, _ := q.Claim("w", time.Minute)

	// Touch renews: advance 50s, touch, advance 50s — still held (not 60s since touch).
	clk.advance(50 * time.Second)
	lease, err := q.Touch(lease, time.Minute)
	if err != nil {
		t.Fatalf("touch: %v", err)
	}
	clk.advance(50 * time.Second)
	if n, _ := q.Reap(); n != 0 {
		t.Fatalf("reap reclaimed %d, want 0 (lease renewed)", n)
	}

	// Now let it expire: advance past the renewed lease.
	clk.advance(2 * time.Minute)
	n, err := q.Reap()
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if n != 1 {
		t.Fatalf("reap reclaimed %d, want 1", n)
	}
	if p, c := q.Len(); p != 1 || c != 0 {
		t.Fatalf("len after reap = %d/%d, want 1/0 (requeued)", p, c)
	}
	// The reclaimed directive's attempts incremented, and the old lease is dead.
	if _, err := q.Touch(lease, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("touch on reaped lease = %v, want ErrLeaseLost", err)
	}
}
