package queue

import (
	"errors"
	"testing"
	"time"
)

// AC-1: an unowned directive becomes owned by exactly one Claim.
func TestPark_AC1_ClaimIsExclusive(t *testing.T) {
	q, _ := newTestQueue()
	q.Push(Directive{Intent: "work", Importance: ImportanceNormal})

	d, lease, err := q.Claim("worker-1", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if d.Intent != "work" {
		t.Fatalf("claimed wrong directive: %q", d.Intent)
	}
	// Second claim must find nothing — the item is owned.
	if _, _, err := q.Claim("worker-2", time.Minute); !errors.Is(err, ErrEmpty) {
		t.Fatalf("second claim = %v, want ErrEmpty", err)
	}
	// Cleanup: finish the claim so the queue is empty.
	if err := q.Done(lease); err != nil {
		t.Fatalf("done: %v", err)
	}
}

// AC-2: Touch extends ownership without losing Attempts or directive state.
func TestPark_AC2_TouchPreservesState(t *testing.T) {
	q, clk := newTestQueue()
	q.Push(Directive{Intent: "stateful", Importance: ImportanceHigh})

	// Claim and immediately requeue once so Attempts > 0.
	_, lease, _ := q.Claim("w", time.Minute)
	q.Requeue(lease, time.Time{})
	d, lease, _ := q.Claim("w", time.Minute)
	if d.Attempts != 1 {
		t.Fatalf("attempts before touch = %d, want 1", d.Attempts)
	}

	// Advance near lease boundary, then Touch.
	clk.advance(55 * time.Second)
	newLease, err := q.Touch(lease, time.Minute)
	if err != nil {
		t.Fatalf("touch: %v", err)
	}
	// Renewed lease extends past the old expiry.
	if !newLease.Expiry.After(lease.Expiry) {
		t.Fatalf("touch did not extend expiry: %v → %v", lease.Expiry, newLease.Expiry)
	}

	// Advance past original expiry but not the renewed one; Reap must not reclaim.
	clk.advance(30 * time.Second) // total ~85s; original expired at 60s, renewed at ~115s
	if n, _ := q.Reap(); n != 0 {
		t.Fatalf("reap after touch reclaimed %d, want 0", n)
	}

	// Done with the renewed lease — Attempts still intact.
	if err := q.Done(newLease); err != nil {
		t.Fatalf("done after touch: %v", err)
	}
}

// AC-3: Requeue returns a claimed directive to pending (claimable again).
func TestPark_AC3_RequeueReturnsToPending(t *testing.T) {
	q, _ := newTestQueue()
	q.Push(Directive{Intent: "retry", Importance: ImportanceNormal})

	_, lease, _ := q.Claim("w", time.Minute)
	if err := q.Requeue(lease, time.Time{}); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if p, c := q.Len(); p != 1 || c != 0 {
		t.Fatalf("len after requeue = %d/%d, want 1/0", p, c)
	}
	// Claimable again.
	d, lease2, err := q.Claim("w", time.Minute)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if d.Attempts != 1 {
		t.Fatalf("attempts after requeue = %d, want 1", d.Attempts)
	}
	// Stale lease is dead.
	if err := q.Requeue(lease, time.Time{}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale requeue = %v, want ErrLeaseLost", err)
	}
	q.Done(lease2)
}

// AC-4: Park puts a claimed directive into a durable hold.
func TestPark_AC4_DurableHold(t *testing.T) {
	q, clk := newTestQueue()
	q.Push(Directive{Intent: "park-me", Importance: ImportanceNormal})

	_, lease, _ := q.Claim("w", time.Minute)

	if err := q.Park(lease); err != nil {
		t.Fatalf("park: %v", err)
	}

	// Len must exclude the parked directive from both counts.
	if p, c := q.Len(); p != 0 || c != 0 {
		t.Fatalf("len after park = %d/%d, want 0/0", p, c)
	}

	// Parked() must count it.
	if got := q.Parked(); got != 1 {
		t.Fatalf("Parked() = %d, want 1", got)
	}

	// Claim must not return it.
	if _, _, err := q.Claim("w2", time.Minute); !errors.Is(err, ErrEmpty) {
		t.Fatalf("claim after park = %v, want ErrEmpty", err)
	}

	// Peek must not return it.
	if _, err := q.Peek(); !errors.Is(err, ErrEmpty) {
		t.Fatalf("peek after park = %v, want ErrEmpty", err)
	}

	// Reap must never reclaim it, even after the original lease would have expired.
	clk.advance(2 * time.Minute)
	if n, _ := q.Reap(); n != 0 {
		t.Fatalf("reap after park reclaimed %d, want 0", n)
	}
	// Still parked.
	if got := q.Parked(); got != 1 {
		t.Fatalf("Parked() after reap = %d, want 1", got)
	}
}

// Park with an already-consumed/invalid lease returns ErrLeaseLost.
func TestPark_InvalidLeaseReturnsErrLeaseLost(t *testing.T) {
	q, _ := newTestQueue()
	q.Push(Directive{Intent: "x"})
	_, lease, _ := q.Claim("w", time.Minute)
	q.Done(lease) // consume the lease

	if err := q.Park(lease); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("park on done lease = %v, want ErrLeaseLost", err)
	}
}

// Park with an expired lease returns ErrLeaseLost.
func TestPark_ExpiredLeaseReturnsErrLeaseLost(t *testing.T) {
	q, clk := newTestQueue()
	q.Push(Directive{Intent: "expire-then-park"})
	_, lease, _ := q.Claim("w", time.Minute)

	clk.advance(2 * time.Minute) // lease expired
	q.Reap()                     // reaped → back to pending with new token

	// Original lease is now dead.
	if err := q.Park(lease); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("park on reaped lease = %v, want ErrLeaseLost", err)
	}
}

// Park with a wrong token returns ErrLeaseLost.
func TestPark_WrongTokenReturnsErrLeaseLost(t *testing.T) {
	q, _ := newTestQueue()
	q.Push(Directive{Intent: "token-mismatch"})
	_, lease, _ := q.Claim("w", time.Minute)

	badLease := Lease{DirectiveID: lease.DirectiveID, Token: "bad-token", Expiry: lease.Expiry}
	if err := q.Park(badLease); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("park with wrong token = %v, want ErrLeaseLost", err)
	}
}

// Multiple parked directives are all counted by Parked(); none visible to Claim/Peek/Reap.
func TestPark_MultipleParked(t *testing.T) {
	q, clk := newTestQueue()
	q.Push(Directive{Intent: "a"})
	q.Push(Directive{Intent: "b"})
	q.Push(Directive{Intent: "c", NotBefore: clk.now().Add(time.Hour)}) // deferred — not claimable yet

	_, la, _ := q.Claim("w", time.Minute)
	_, lb, _ := q.Claim("w", time.Minute)

	q.Park(la)
	q.Park(lb)

	// Two parked, one deferred pending.
	if got := q.Parked(); got != 2 {
		t.Fatalf("Parked() = %d, want 2", got)
	}
	// Pending has the deferred one, claimed is 0.
	if p, c := q.Len(); p != 1 || c != 0 {
		t.Fatalf("len = %d/%d, want 1/0 (deferred pending, nothing claimed)", p, c)
	}
	// Claim can't see the parked ones (deferred not eligible yet).
	if _, _, err := q.Claim("w", time.Minute); !errors.Is(err, ErrEmpty) {
		t.Fatalf("claim = %v, want ErrEmpty", err)
	}

	// Reap leaves parked items alone.
	clk.advance(2 * time.Minute)
	if n, _ := q.Reap(); n != 0 {
		t.Fatalf("reap reclaimed %d, want 0", n)
	}
	if got := q.Parked(); got != 2 {
		t.Fatalf("Parked() after reap = %d, want 2", got)
	}
}
