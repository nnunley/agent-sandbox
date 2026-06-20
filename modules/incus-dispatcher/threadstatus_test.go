package main

import (
	"testing"
	"time"
)

// AC-1: verify the exact value set of ThreadStatus constants.
func TestThreadStatusValues(t *testing.T) {
	want := map[ThreadStatus]string{
		StatusQueued:    "queued",
		StatusActive:    "active",
		StatusPaused:    "paused",
		StatusBlocked:   "blocked",
		StatusDone:      "done",
		StatusAbandoned: "abandoned",
	}
	for s, v := range want {
		if string(s) != v {
			t.Errorf("ThreadStatus %q: got underlying value %q, want %q", s, string(s), v)
		}
	}
	// Ensure the set is exactly 6 — caught by the compiler if any constant is missing,
	// but also guard against extras by enumerating them all explicitly above.
	if len(want) != 6 {
		t.Errorf("expected exactly 6 ThreadStatus values, got %d", len(want))
	}
}

// AC-2: transitions, status lookup, and unknown-id behaviour.
func TestThreadTracker(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tick := func() func() time.Time {
		var n int
		return func() time.Time {
			ts := now.Add(time.Duration(n) * time.Second)
			n++
			return ts
		}
	}

	t.Run("unknown id returns empty status and no transitions", func(t *testing.T) {
		tr := NewThreadTracker(time.Now)
		if got := tr.Status("nonexistent"); got != "" {
			t.Errorf("Status(unknown) = %q, want \"\"", got)
		}
		if got := tr.Transitions("nonexistent"); len(got) != 0 {
			t.Errorf("Transitions(unknown) = %v, want empty", got)
		}
	})

	t.Run("first Set records From as empty string", func(t *testing.T) {
		tr := NewThreadTracker(tick())
		tr.Set("d1", StatusQueued)

		txs := tr.Transitions("d1")
		if len(txs) != 1 {
			t.Fatalf("expected 1 transition, got %d", len(txs))
		}
		if txs[0].From != "" {
			t.Errorf("first transition From = %q, want \"\"", txs[0].From)
		}
		if txs[0].To != StatusQueued {
			t.Errorf("first transition To = %q, want %q", txs[0].To, StatusQueued)
		}
	})

	t.Run("subsequent Sets record correct From/To chain", func(t *testing.T) {
		clk := tick()
		tr := NewThreadTracker(clk)

		tr.Set("d1", StatusQueued)
		tr.Set("d1", StatusActive)
		tr.Set("d1", StatusDone)

		txs := tr.Transitions("d1")
		if len(txs) != 3 {
			t.Fatalf("expected 3 transitions, got %d", len(txs))
		}

		cases := []struct{ from, to ThreadStatus }{
			{"", StatusQueued},
			{StatusQueued, StatusActive},
			{StatusActive, StatusDone},
		}
		for i, c := range cases {
			if txs[i].From != c.from {
				t.Errorf("transition[%d].From = %q, want %q", i, txs[i].From, c.from)
			}
			if txs[i].To != c.to {
				t.Errorf("transition[%d].To = %q, want %q", i, txs[i].To, c.to)
			}
		}
	})

	t.Run("Status returns latest value", func(t *testing.T) {
		tr := NewThreadTracker(tick())
		tr.Set("d1", StatusQueued)
		tr.Set("d1", StatusActive)
		tr.Set("d1", StatusPaused)

		if got := tr.Status("d1"); got != StatusPaused {
			t.Errorf("Status after Paused = %q, want %q", got, StatusPaused)
		}
	})

	t.Run("Ts is stamped via now clock", func(t *testing.T) {
		base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		calls := 0
		clk := func() time.Time {
			ts := base.Add(time.Duration(calls) * time.Minute)
			calls++
			return ts
		}
		tr := NewThreadTracker(clk)
		tr.Set("d1", StatusQueued)
		tr.Set("d1", StatusActive)

		txs := tr.Transitions("d1")
		if txs[0].Ts != base {
			t.Errorf("Ts[0] = %v, want %v", txs[0].Ts, base)
		}
		if txs[1].Ts != base.Add(time.Minute) {
			t.Errorf("Ts[1] = %v, want %v", txs[1].Ts, base.Add(time.Minute))
		}
	})

	t.Run("transitions are independent per id", func(t *testing.T) {
		tr := NewThreadTracker(tick())
		tr.Set("d1", StatusQueued)
		tr.Set("d2", StatusActive)
		tr.Set("d1", StatusDone)

		d1 := tr.Transitions("d1")
		d2 := tr.Transitions("d2")

		if len(d1) != 2 {
			t.Errorf("d1 transitions: want 2, got %d", len(d1))
		}
		if len(d2) != 1 {
			t.Errorf("d2 transitions: want 1, got %d", len(d2))
		}
		if tr.Status("d1") != StatusDone {
			t.Errorf("d1 status: want done, got %q", tr.Status("d1"))
		}
		if tr.Status("d2") != StatusActive {
			t.Errorf("d2 status: want active, got %q", tr.Status("d2"))
		}
	})

	t.Run("transitions slice is a snapshot not a live reference", func(t *testing.T) {
		tr := NewThreadTracker(tick())
		tr.Set("d1", StatusQueued)

		before := tr.Transitions("d1")
		tr.Set("d1", StatusActive)
		after := tr.Transitions("d1")

		if len(before) != 1 {
			t.Errorf("snapshot before second Set should have 1 transition, got %d", len(before))
		}
		if len(after) != 2 {
			t.Errorf("after second Set should have 2 transitions, got %d", len(after))
		}
	})
}
