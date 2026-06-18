package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// DirectiveOutcome is the result of processing one directive through the loop.
type DirectiveOutcome string

const (
	OutcomeDone     DirectiveOutcome = "done"     // pass → thread done
	OutcomeRequeued DirectiveOutcome = "requeued" // fail → retry (under max attempts)
	OutcomeParked   DirectiveOutcome = "parked"   // fail → exhausted attempts (ITER-0001 adds the escalations lane)
	OutcomeRejected DirectiveOutcome = "rejected" // template/origin validation failed (D1)
	OutcomeEmpty    DirectiveOutcome = ""         // queue had no eligible directive
)

// defaultMaxAttempts when a directive does not specify one.
const defaultMaxAttempts = 3

// Daemon drains the queue and runs each directive through the one-shot lifecycle
// using the existing execution Runner. ITER-0000 scope: pass→done / fail→requeue
// / park; the full escalation ladder + Temporal-backed retry + decision-log are
// deferred (ITER-0001/0007).
type Daemon struct {
	Q        queue.Queue
	Runner   Runner
	Policy   *Policy
	Consumer string
	LeaseDur time.Duration

	// MapToTask converts a validated directive into a Task for the Runner.
	// Injectable for testing; defaults to DefaultMapToTask.
	MapToTask func(queue.Directive) Task
}

// RunOnce claims one eligible directive and processes it. Returns the outcome
// and the directive ID (OutcomeEmpty + "" when the queue has nothing eligible).
func (dm *Daemon) RunOnce(ctx context.Context) (DirectiveOutcome, string, error) {
	mapFn := dm.MapToTask
	if mapFn == nil {
		mapFn = DefaultMapToTask
	}

	d, lease, err := dm.Q.Claim(dm.Consumer, dm.LeaseDur)
	if errors.Is(err, queue.ErrEmpty) {
		return OutcomeEmpty, "", nil
	}
	if err != nil {
		return OutcomeEmpty, "", fmt.Errorf("claim: %w", err)
	}

	// D1: validate the PROPOSED template against the allowlist + origin before
	// anything is launched. A rejected directive is removed, not retried.
	if verr := dm.Policy.ValidateTemplate(d); verr != nil {
		log.Printf("directive %s REJECTED: %v", d.ID, verr)
		_ = dm.Q.Done(lease)
		return OutcomeRejected, d.ID, nil
	}

	task := mapFn(d)
	result, runErr := dm.Runner.Run(ctx, task)
	// Teardown always runs (the stop-then-delete fix lives in the Runner's Cleanup).
	_ = dm.Runner.Cleanup()

	if passed(result, runErr) {
		_ = dm.Q.Done(lease)
		return OutcomeDone, d.ID, nil
	}

	// Fail → minimal outcome handling.
	maxAttempts := d.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	if d.Attempts+1 >= maxAttempts {
		log.Printf("directive %s PARKED after %d attempt(s)", d.ID, d.Attempts+1)
		_ = dm.Q.Done(lease) // ITER-0000: park = remove + log; escalations lane is ITER-0001
		return OutcomeParked, d.ID, nil
	}
	_ = dm.Q.Requeue(lease, time.Time{})
	return OutcomeRequeued, d.ID, nil
}

// passed decides the authoritative outcome of a run. A run passes only when the
// framework call succeeded, the command exited 0, AND — if an external grade was
// run — the oracle also passed (the grade is authoritative, anti-reward-hack).
func passed(result *Result, runErr error) bool {
	if runErr != nil && !isCommandErr(runErr) {
		return false // framework/infra error
	}
	if result == nil || result.ExitCode != 0 {
		return false
	}
	if g := result.ExternalGradingResult; g != nil {
		return g.PatchApplied && g.ExitCode == 0
	}
	return true
}

// DefaultMapToTask maps a directive to a Task. The template name selects the
// image (and, later, the runner); ITER-0000 uses a minimal convention and leaves
// the richer template→runner registry to ITER-0005.
func DefaultMapToTask(d queue.Directive) Task {
	t := Task{
		Name:      sanitizeName(d.ID),
		Repo:      d.Repo,
		Ref:       d.Ref,
		ImageName: DefaultImageName,
		Timeout:   DefaultTimeout,
		Provider:  ProviderAnthropic,
	}
	if d.Task != "" {
		// The brief is delivered to the worker; the template's runner consumes it.
		t.Cmd = []string{"bash", "-lc", d.Task}
	}
	if d.Grade != nil && d.Grade.OracleRef != "" {
		// Presence of a grade spec routes the run through external grading.
		t.ExternalGradingCheckout = d.Grade.OracleRef
	}
	return t
}

func sanitizeName(id string) string {
	return strings.ToLower(strings.NewReplacer("/", "-", "_", "-", ":", "-").Replace(id))
}
