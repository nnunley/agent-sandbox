package main

import (
	"context"
	"time"
)

// ServeOptions configures the coordinator daemon loop (STORY-0007 AC-2).
type ServeOptions struct {
	// PollInterval is how long to wait before re-checking an empty queue (long-running
	// mode). Defaults to 1s when zero. Ignored when UntilEmpty is set.
	PollInterval time.Duration

	// UntilEmpty makes Serve return as soon as the queue yields no eligible directive —
	// a bounded single drain pass (useful for tests and one-shot coordinator runs).
	// When false, Serve loops until the context is canceled (the durable-daemon mode).
	UntilEmpty bool
}

// ServeStats counts the outcomes a Serve pass produced.
type ServeStats struct {
	Claimed   int
	Done      int
	Requeued  int
	Escalated int
	Rejected  int
}

// Serve runs the coordinator as a long-running daemon: it repeatedly drains the queue via
// dm.RunOnce, one coordinator process for many one-shot tasks (D3 — the coordinator persists,
// agents do not). It returns when the context is canceled (graceful shutdown) or, in
// UntilEmpty mode, when the queue has no eligible directive. A single task failure never
// stops the loop — the outcome is counted and draining continues.
func Serve(ctx context.Context, dm *Daemon, opts ServeOptions) (ServeStats, error) {
	poll := opts.PollInterval
	if poll <= 0 {
		poll = time.Second
	}
	var stats ServeStats
	for {
		if err := ctx.Err(); err != nil {
			return stats, nil // graceful shutdown on cancel/timeout
		}
		outcome, _, err := dm.RunOnce(ctx)
		if err != nil {
			return stats, err // framework/infra error — surface it
		}
		switch outcome {
		case OutcomeEmpty:
			if opts.UntilEmpty {
				return stats, nil
			}
			// Long-running: wait PollInterval, but wake immediately on cancel.
			t := time.NewTimer(poll)
			select {
			case <-ctx.Done():
				t.Stop()
				return stats, nil
			case <-t.C:
			}
		default:
			stats.Claimed++
			switch outcome {
			case OutcomeDone:
				stats.Done++
			case OutcomeRequeued:
				stats.Requeued++
			case OutcomeEscalated:
				stats.Escalated++
			case OutcomeRejected:
				stats.Rejected++
			}
		}
	}
}
