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
	OutcomeDone      DirectiveOutcome = "done"      // pass → thread done
	OutcomeRequeued  DirectiveOutcome = "requeued"  // fail → autonomous rung (retry / stronger-worker / hard-tier)
	OutcomeEscalated DirectiveOutcome = "escalated" // fail → human rung: parked + pushed to the escalations lane
	OutcomeRejected  DirectiveOutcome = "rejected"  // template/origin validation failed (D1)
	OutcomeEmpty     DirectiveOutcome = ""          // queue had no eligible directive
)

// Daemon drains the queue and drives each directive through the D4 deterministic
// coordination loop: claim → validate (D1) → run → authoritative grade → apply the
// graduated escalation ladder — tracking thread status and writing a D6 decision-log entry
// for every transition. Log / Threads / Escalations / Now are OPTIONAL (nil → no-op) so the
// minimal ITER-0000 construction still works. The ladder is SYNCHRONOUS; Temporal-backed
// backoff (STORY-0058 AC-24) and urgency resurfacing (STORY-0061 AC-3) are deferred to ITER-0007.
type Daemon struct {
	Q        queue.Queue
	Runner   Runner
	Policy   *Policy
	Consumer string
	LeaseDur time.Duration

	MapToTask   func(queue.Directive) Task // converts a directive to a Task; defaults to DefaultMapToTask
	Log         DecisionLog                // D6 append-only decision log (optional)
	Threads     *ThreadTracker             // thread-status tracking (optional)
	Escalations EscalationLane             // non-blocking human escalations lane (optional)
	Now         func() time.Time           // clock for decision timestamps (optional; defaults to time.Now)
}

func (dm *Daemon) clock() time.Time {
	if dm.Now != nil {
		return dm.Now()
	}
	return time.Now()
}

// record appends a D6 decision-log entry (no-op when no log is configured).
func (dm *Daemon) record(directiveID, grade, rule, action string) {
	if dm.Log != nil {
		_ = dm.Log.Append(Decision{DirectiveID: directiveID, Grade: grade, Rule: rule, Action: action, Ts: dm.clock()})
	}
}

// setStatus records a thread-status transition (no-op when no tracker is configured).
func (dm *Daemon) setStatus(id string, s ThreadStatus) {
	if dm.Threads != nil {
		dm.Threads.Set(id, s)
	}
}

// RunOnce claims one eligible directive and drives it through the D4 loop. Returns the
// outcome and the directive ID (OutcomeEmpty + "" when the queue has nothing eligible).
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
	dm.setStatus(d.ID, StatusActive)

	// D1: validate the PROPOSED template against the allowlist + origin before anything is
	// launched. A rejected directive is removed, not retried.
	if verr := dm.Policy.ValidateTemplate(d); verr != nil {
		log.Printf("directive %s REJECTED: %v", d.ID, verr)
		_ = dm.Q.Done(lease)
		dm.setStatus(d.ID, StatusAbandoned)
		dm.record(d.ID, "", "template-invalid", "rejected")
		return OutcomeRejected, d.ID, nil
	}

	task := mapFn(d)
	result, runErr := dm.Runner.Run(ctx, task)
	// Teardown always runs (stop-then-delete lives in the Runner's Cleanup); log the reap (D6).
	_ = dm.Runner.Cleanup()
	dm.record(d.ID, "", "teardown", "reap")

	if passed(result, runErr) {
		_ = dm.Q.Done(lease)
		dm.setStatus(d.ID, StatusDone)
		dm.record(d.ID, "pass", "grade-pass", "done")
		return OutcomeDone, d.ID, nil
	}

	// Fail → climb the graduated escalation ladder by prior-attempt count (D4).
	rung := nextRung(d.Attempts)
	if rung.Autonomous() {
		// Pre-approved rungs (retry-same / stronger-worker / hard-tier) requeue autonomously.
		_ = dm.Q.Requeue(lease, time.Time{})
		dm.setStatus(d.ID, StatusQueued)
		dm.record(d.ID, "fail", rung.String(), "requeue")
		return OutcomeRequeued, d.ID, nil
	}

	// Human rung — authority/judgment limit. Park the directive into a durable hold and push
	// it to the NON-BLOCKING escalations lane; the loop keeps draining other directives.
	// The Park + the "escalate-human" D6 decision below ALWAYS happen, so even when no lane is
	// configured the directive is not lost — it is durably parked (recoverable via Q.Parked())
	// and recorded in the decision log. The lane is the convenience surface a human polls.
	_ = dm.Q.Park(lease)
	if dm.Escalations != nil {
		_ = dm.Escalations.Push(EscalationItem{DirectiveID: d.ID, Reason: "authority-limit", Origin: d.Origin})
	}
	dm.setStatus(d.ID, StatusBlocked)
	dm.record(d.ID, "fail", rung.String(), "escalate-human")
	return OutcomeEscalated, d.ID, nil
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
