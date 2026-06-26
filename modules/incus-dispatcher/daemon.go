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
	Backend     BackendFactory             // tier→Runner selection (optional; nil → always use Runner)
	Log         DecisionLog                // D6 append-only decision log (optional)
	Audit       AuditLog                   // STORY-0054 system audit log (optional); every processed run is auto-logged
	Threads     *ThreadTracker             // thread-status tracking (optional)
	ThreadStore *ThreadStore               // thread data store with BudgetPolicy (optional; STORY-0036 AC-3)
	Escalations EscalationLane             // non-blocking human escalations lane (optional)
	Now         func() time.Time           // clock for decision timestamps (optional; defaults to time.Now)
	Context     ContextProvider            // soft-state provider (optional; defaults to NoopProvider). Best-effort: handoff loss never affects correctness (STORY-0018 AC-4).
	Results     *ResultStore               // result/artifact persistence (optional; stores run results for later inspection).
}

// ctx returns the configured ContextProvider, or a NoopProvider when none is set. The daemon claims
// work ONLY via dm.Q (queue.Queue); the ContextProvider is never a work source (STORY-0018 AC-5).
func (dm *Daemon) ctxProvider() ContextProvider {
	if dm.Context != nil {
		return dm.Context
	}
	return NoopProvider{}
}

func (dm *Daemon) clock() time.Time {
	if dm.Now != nil {
		return dm.Now()
	}
	return time.Now()
}

// record appends a D6 decision-log entry (no-op when no log is configured).
// An optional reason argument populates Decision.Reason (denial/explanation detail).
func (dm *Daemon) record(directiveID, grade, rule, action string, reason ...string) {
	if dm.Log != nil {
		r := ""
		if len(reason) > 0 {
			r = reason[0]
		}
		_ = dm.Log.Append(Decision{DirectiveID: directiveID, Grade: grade, Rule: rule, Action: action, Reason: r, Ts: dm.clock()})
	}
}

// audit appends a system audit entry (STORY-0054) if an audit log is wired. Nil-safe, like record().
func (dm *Daemon) audit(kind AuditKind, runID, threadID, parentRef, actor, detail string) {
	if dm.Audit != nil {
		_, _ = dm.Audit.Append(AuditEntry{
			Ts:        dm.clock(),
			Actor:     actor,
			Kind:      kind,
			ThreadID:  threadID,
			RunID:     runID,
			ParentRef: parentRef,
			Detail:    detail,
		})
	}
}

// originActor maps a directive origin to an audit actor (defaulting empty/unknown to "orchestrator").
func originActor(origin string) string {
	if origin == "" {
		return OriginOrchestrator
	}
	return origin
}

// emitRetryHandoff produces a FRESH handoff bundle for the upcoming retry (STORY-0058 AC-25),
// capturing the just-failed run's soft workflow state. Each retry gets a distinct bundle (keyed by
// the attempt count). It is best-effort: a provider error is swallowed because handoff loss must
// never affect a run's correctness — the diff + oracle grade remain authoritative (STORY-0018 AC-4).
func (dm *Daemon) emitRetryHandoff(d queue.Directive, _ *Result) {
	runID := fmt.Sprintf("%s-r%d", d.ID, d.Attempts)
	st := WorkflowState{
		CurrentBranch:    d.Ref,
		CurrentWorkspace: d.Repo,
		ResumeSummary: ResumeSummary{
			PriorWork: fmt.Sprintf("attempt %d failed grading", d.Attempts),
			NextStep:  "retry with the prior attempt's context",
		},
	}
	_, _ = dm.ctxProvider().CreateHandoff(d.ID, runID, st)
}

// setStatus records a thread-status transition (no-op when no tracker is configured).
func (dm *Daemon) setStatus(id string, s ThreadStatus) {
	if dm.Threads != nil {
		dm.Threads.Set(id, s)
	}
}

// checkBudget enforces budget guardrails at the thread level (STORY-0036 AC-3).
// It sums prior spend on the thread and checks whether the run would exceed the per-thread limit.
// Returns true if the run may proceed; false if it should escalate or be rejected.
// Also returns the BudgetEnforcement decision for auditing and logging.
func (dm *Daemon) checkBudget(d queue.Directive, run *Run) (bool, *BudgetEnforcement) {
	// If no thread store or budget policy, allow the run (no enforcement).
	if dm.ThreadStore == nil {
		return true, nil
	}

	threadRec, ok := dm.ThreadStore.Get(d.ID)
	if !ok || threadRec.BudgetPolicy == nil {
		return true, nil
	}

	// Compute current thread spend from prior runs (if the result store has them).
	var currentSpend float64
	if dm.Results != nil {
		// Load all prior runs for this thread from the result store.
		priorResults := dm.Results.ByThread(d.ID)
		for _, priorRun := range priorResults {
			if priorRun != nil && priorRun.RunID != run.RunID {
				currentSpend += priorRun.SpendUSD
			}
		}
	}

	// Enforce the budget policy.
	decision := threadRec.BudgetPolicy.EnforceRunBudget(run, currentSpend)
	if decision == nil || !decision.Allowed {
		return false, decision
	}
	return true, decision
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

	// Thread status gating: if the thread is paused or blocked, defer the directive
	// (return to pending WITHOUT incrementing Attempts) to prevent execution while held.
	// Use a small backoff (poll interval) so the gate doesn't cause a tight spin.
	if dm.Threads != nil {
		threadStatus := dm.Threads.Status(d.ID)
		if threadStatus == StatusPaused || threadStatus == StatusBlocked {
			// Backoff: use poll interval if available, else a sensible default (1 second)
			backoff := time.Second
			// Note: Daemon doesn't have a poll interval field; caller (Serve) manages polling.
			// For now, use 1s backoff; production could wire daemon.PollInterval if needed.
			deferTime := dm.clock().Add(backoff)
			_ = dm.Q.DeferDirective(lease, deferTime)
			dm.record(d.ID, "", "thread-held", "defer", fmt.Sprintf("thread status %s prevents dispatch (backed off until %s)", threadStatus, deferTime.Format("15:04:05")))
			return OutcomeRequeued, d.ID, nil
		}
	}

	dm.setStatus(d.ID, StatusActive)

	// D1: validate the PROPOSED template against the allowlist + origin before anything is
	// launched. A rejected directive is removed, not retried.
	if verr := dm.Policy.ValidateTemplate(d); verr != nil {
		log.Printf("directive %s REJECTED: %v", d.ID, verr)
		_ = dm.Q.Done(lease)
		dm.setStatus(d.ID, StatusAbandoned)
		dm.record(d.ID, "", "template-invalid", "rejected", verr.Error())
		return OutcomeRejected, d.ID, nil
	}

	// Best-effort import of prior soft state. This NEVER affects correctness: the authoritative
	// state is the diff + oracle grade computed below, not the handoff. A missing/corrupt bundle or
	// a provider error is ignored — proving STORY-0018 AC-4 (correctness independent of handoff loss).
	if d.HandoffIn != "" {
		_, _ = dm.ctxProvider().ImportHandoff(d.HandoffIn)
	}

	// Resolve the isolation tier from the VETTED template (D1 mechanism — STORY-0023 AC-1)
	// and select the backend that implements it. Selection happens OUTSIDE Runner.Run so all
	// backends share one interface; ITER-0005b's microVM/nspawn runners graft in by registering
	// with the factory. With no factory configured, fall back to the single Runner.
	runner := dm.Runner
	tier := dm.Policy.TierFor(d.Template)
	if dm.Backend != nil {
		r, serr := dm.Backend.SelectRunner(tier)
		if serr != nil {
			// No backend implements this tier yet (e.g. Hard before ITER-0005b's Firecracker).
			// Never run sensitive work on a substrate that doesn't provide its isolation: park
			// the directive (durable, recoverable) and surface it for out-of-band attention.
			_ = dm.Q.Park(lease)
			if dm.Escalations != nil {
				_ = dm.Escalations.Push(EscalationItem{DirectiveID: d.ID, Reason: "backend-unavailable", Origin: d.Origin})
			}
			dm.setStatus(d.ID, StatusBlocked)
			dm.record(d.ID, "", "backend-unavailable", "escalate-human", serr.Error())
			return OutcomeEscalated, d.ID, nil
		}
		runner = r
	}
	dm.record(d.ID, "", "tier-select", string(tier))

	task := mapFn(d)
	result, runErr := runner.Run(ctx, task)
	// STORY-0054 AC-1: every run the daemon processes is logged to the system audit log (wired here,
	// not just by tests). Worker-origin runs carry their origin as the actor; the directive id is the
	// run/thread key. A worker-authored child directive (origin "worker:<id>") is also a delegation —
	// callers that emit child directives audit the delegation at the emit seam.
	dm.audit(AuditKindRun, d.ID, d.ID, "", originActor(d.Origin), d.Intent)
	// Teardown always runs (stop-then-delete lives in the Runner's Cleanup); log the reap (D6).
	_ = runner.Cleanup()
	dm.record(d.ID, "", "teardown", "reap")

	// Store the result for later inspection (artifacts, patch, output) — optional but enables AC-3.
	// For budget enforcement (STORY-0036 AC-3), track the result under both the directive ID and thread ID.
	if dm.Results != nil && result != nil {
		dm.Results.StoreWithThread(d.ID, d.ID, result)
	}

	// STORY-0036 AC-3: Budget enforcement is checked BEFORE grading, even on successful runs.
	// A run that exceeds the budget is escalated to human review regardless of exit code.
	if dm.Results != nil && result != nil {
		// Create a minimal Run object from the Result for budget checking.
		checkRun := &Run{
			RunID:      d.ID,
			ThreadID:   d.ID,
			SpendUSD:   result.SpendUSD,
			TokensIn:   result.TokensIn,
			TokensOut:  result.TokensOut,
			LatencyMs:  result.LatencyMs,
		}
		budgetOK, budgetDecision := dm.checkBudget(d, checkRun)
		if !budgetOK && budgetDecision != nil {
			// Budget exceeded: escalate to human rung without climbing the ladder.
			_ = dm.Q.Park(lease)
			if dm.Escalations != nil {
				_ = dm.Escalations.Push(EscalationItem{DirectiveID: d.ID, Reason: "budget-exceeded", Origin: d.Origin})
			}
			dm.setStatus(d.ID, StatusBlocked)
			dm.audit(AuditKindRun, d.ID, d.ID, "", originActor(d.Origin), "budget_exceeded: "+budgetDecision.Reason)
			dm.record(d.ID, "fail", "budget-exceeded", "escalate-human", budgetDecision.Reason)
			return OutcomeEscalated, d.ID, nil
		}
	}

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
		// STORY-0058 AC-25: emit a FRESH handoff bundle reflecting the just-failed run so the
		// retry is provided with current soft state (consumed by the successor via its
		// ContextProvider — proven on a real worker in SCENARIO-0030).
		dm.emitRetryHandoff(d, result)
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
