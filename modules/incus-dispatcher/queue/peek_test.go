package queue

import (
	"errors"
	"testing"
	"time"
)

// HIDDEN ORACLE for the dogfood task. Applied to a CLEAN checkout + the worker's
// diff, then run with `go test ./queue/`. The worker never sees this file.

func oracleClock() (*MemoryQueue, time.Time) {
	t := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	return NewMemoryQueueWithClock(func() time.Time { return t }), t
}

func TestPeek_ReturnsHighestWithoutClaiming(t *testing.T) {
	q, _ := oracleClock()
	q.Push(Directive{Intent: "low", Importance: ImportanceLow})
	q.Push(Directive{Intent: "high", Importance: ImportanceHigh})

	d, err := q.Peek()
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if d.Intent != "high" {
		t.Fatalf("peek = %q, want high", d.Intent)
	}
	// Peek must NOT claim: nothing leased, both still pending.
	if p, c := q.Len(); p != 2 || c != 0 {
		t.Fatalf("after peek len = %d/%d, want 2/0 (peek must not claim)", p, c)
	}
	// The same directive is still claimable afterward.
	cd, _, err := q.Claim("w", time.Minute)
	if err != nil || cd.Intent != "high" {
		t.Fatalf("claim after peek = %q err %v, want high", cd.Intent, err)
	}
}

func TestPeek_RespectsNotBefore(t *testing.T) {
	q, now := oracleClock()
	q.Push(Directive{Intent: "deferred", Importance: ImportanceHigh, NotBefore: now.Add(time.Hour)})
	q.Push(Directive{Intent: "ready", Importance: ImportanceLow})
	d, err := q.Peek()
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if d.Intent != "ready" {
		t.Fatalf("peek = %q, want ready (deferred not yet eligible)", d.Intent)
	}
}

func TestPeek_EmptyReturnsErrEmpty(t *testing.T) {
	q, _ := oracleClock()
	if _, err := q.Peek(); !errors.Is(err, ErrEmpty) {
		t.Fatalf("peek on empty = %v, want ErrEmpty", err)
	}
	// Only deferred items present → still empty to Peek.
	_, now := oracleClock()
	q.Push(Directive{Intent: "later", NotBefore: now.Add(time.Hour)})
	if _, err := q.Peek(); !errors.Is(err, ErrEmpty) {
		t.Fatalf("peek with only-deferred = %v, want ErrEmpty", err)
	}
}
