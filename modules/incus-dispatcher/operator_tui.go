package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// OperatorConsole is a stdlib command interpreter for thread and worker management.
// It reads operator commands from an io.Reader (line-based) and writes rendered output to an io.Writer.
// This design is app-level testable by driving stdin→stdout strings without requiring a terminal/TTY.
type OperatorConsole struct {
	q                 queue.Queue
	threads           *ThreadTracker
	threadStore       *ThreadStore      // For enumeration (threads command)
	workers           map[string]*Worker
	audit             AuditLog
	threadToDirective map[string]queue.Directive // For requeue: thread ID → last directive
	results           *ResultStore               // For artifact inspection: real result source from Daemon
	now               func() time.Time
}

// NewOperatorConsole returns a new OperatorConsole wired with the given dependencies.
func NewOperatorConsole(q queue.Queue, threads *ThreadTracker, workers map[string]*Worker, audit AuditLog, now func() time.Time) *OperatorConsole {
	return NewOperatorConsoleWithStore(q, threads, NewThreadStore(), workers, audit, now)
}

// NewOperatorConsoleWithStore returns a new OperatorConsole with explicit threadStore.
// Used when budget policy mutations are needed.
func NewOperatorConsoleWithStore(q queue.Queue, threads *ThreadTracker, threadStore *ThreadStore, workers map[string]*Worker, audit AuditLog, now func() time.Time) *OperatorConsole {
	if now == nil {
		now = time.Now
	}
	return &OperatorConsole{
		q:                 q,
		threads:           threads,
		threadStore:       threadStore,
		workers:           workers,
		audit:             audit,
		threadToDirective: make(map[string]queue.Directive),
		results:           NewResultStore(), // Real result source from Daemon
		now:               now,
	}
}

// Run starts the console, reading commands from in and writing output to out until EOF.
func (oc *OperatorConsole) Run(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		result, err := oc.executeCommand(line)
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
		} else {
			fmt.Fprintf(out, "%s\n", result)
		}
	}
	return scanner.Err()
}

// executeCommand parses and executes a single command line.
// It handles quoted strings (e.g., "two words" stays as one argument).
func (oc *OperatorConsole) executeCommand(line string) (string, error) {
	parts := parseCommandLine(line)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "create":
		return oc.cmdCreate(args)
	case "queue":
		return oc.cmdQueue(args)
	case "workers":
		return oc.cmdWorkers(args)
	case "threads":
		return oc.cmdThreads(args)
	case "inspect":
		return oc.cmdInspect(args)
	case "pause":
		return oc.cmdPause(args)
	case "block":
		return oc.cmdBlock(args)
	case "resume":
		return oc.cmdResume(args)
	case "requeue":
		return oc.cmdRequeue(args)
	case "budget":
		return oc.cmdBudget(args)
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

// parseCommandLine parses a command line, handling quoted strings.
// For simplicity, only double quotes are supported, and escaping is not implemented.
func parseCommandLine(line string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch == '"' {
			inQuote = !inQuote
		} else if ch == ' ' && !inQuote {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// cmdCreate pushes a directive onto the queue. Usage: create <intent> <template> <repo> <ref> <task>
func (oc *OperatorConsole) cmdCreate(args []string) (string, error) {
	if len(args) < 5 {
		return "", fmt.Errorf("create requires 5 args: intent template repo ref task")
	}
	intent, template, repo, ref, task := args[0], args[1], args[2], args[3], args[4]

	d := queue.Directive{
		Intent:     intent,
		Template:   template,
		Repo:       repo,
		Ref:        ref,
		Task:       task,
		Importance: queue.ImportanceNormal,
	}
	id, err := oc.q.Push(d)
	if err != nil {
		return "", fmt.Errorf("push directive: %w", err)
	}

	// Update the directive with its assigned ID and track it for requeue
	d.ID = id
	oc.threadToDirective[id] = d

	// Initialize thread status for this directive
	if oc.threads != nil {
		oc.threads.Set(id, StatusQueued)
	}

	// Register the thread in the thread store for enumeration (AC-2)
	if oc.threadStore != nil {
		oc.threadStore.Put(Thread{
			ID:     id,
			Status: StatusQueued,
		})
	}

	// Audit the creation as a mutation
	if oc.audit != nil {
		_, _ = oc.audit.Append(AuditEntry{
			Ts:       oc.now(),
			Actor:    "operator",
			Kind:     AuditKindMutation,
			ThreadID: id,
			Detail:   fmt.Sprintf("directive created: intent=%s", intent),
		})
	}

	return fmt.Sprintf("ok: directive %s created", id), nil
}

// cmdQueue renders pending/claimed counts and per-directive summary.
func (oc *OperatorConsole) cmdQueue(args []string) (string, error) {
	pending, claimed := oc.q.Len()
	var sb strings.Builder
	fmt.Fprintf(&sb, "queue: pending=%d claimed=%d\n", pending, claimed)

	// Try to peek at the next directive
	if next, err := oc.q.Peek(); err == nil {
		fmt.Fprintf(&sb, "  next: id=%s intent=%s importance=%s\n", next.ID, next.Intent, next.Importance)
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}

// cmdWorkers renders registered workers and their status.
func (oc *OperatorConsole) cmdWorkers(args []string) (string, error) {
	if len(oc.workers) == 0 {
		return "workers: none registered", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "workers: %d registered\n", len(oc.workers))
	for id, w := range oc.workers {
		fmt.Fprintf(&sb, "  %s: kind=%s runtime=%s caps=%v\n", id, w.WorkerKind, w.RuntimeMode, w.Capabilities)
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// cmdThreads renders all known threads and their statuses.
func (oc *OperatorConsole) cmdThreads(args []string) (string, error) {
	out := &strings.Builder{}
	oc.renderThreads(out)
	return strings.TrimRight(out.String(), "\n"), nil
}

// renderThreads renders the thread list to a writer (factored for testability).
func (oc *OperatorConsole) renderThreads(out io.Writer) {
	if oc.threadStore == nil {
		fmt.Fprintf(out, "threads: no thread store configured\n")
		return
	}

	// Note: sorting by priority + aging deferred to STORY-0037;
	// this implementation lists threads in registration order.
	threads := oc.threadStore.ListAll()
	if len(threads) == 0 {
		fmt.Fprintf(out, "threads: none\n")
		return
	}

	fmt.Fprintf(out, "threads: %d\n", len(threads))
	for _, t := range threads {
		fmt.Fprintf(out, "  %s: status=%s\n", t.ID, t.Status)
	}
}

// cmdInspect renders thread status, transitions, and artifacts.
func (oc *OperatorConsole) cmdInspect(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("inspect requires thread-id or run-id argument")
	}
	id := args[0]

	out := &strings.Builder{}
	oc.renderInspect(out, id)
	return strings.TrimRight(out.String(), "\n"), nil
}

// renderInspect renders thread status, transitions, and artifacts to a writer (factored for testability).
func (oc *OperatorConsole) renderInspect(out io.Writer, id string) {
	status := oc.threads.Status(id)
	transitions := oc.threads.Transitions(id)

	fmt.Fprintf(out, "thread %s:\n", id)
	fmt.Fprintf(out, "  status: %s\n", status)
	fmt.Fprintf(out, "  transitions: %d\n", len(transitions))
	for i, t := range transitions {
		fmt.Fprintf(out, "    [%d] %s -> %s at %s\n", i, t.From, t.To, t.Ts.Format("15:04:05"))
	}

	// Render artifacts if available (from real result store)
	if oc.results != nil {
		if result, ok := oc.results.Get(id); ok {
			fmt.Fprintf(out, "  artifacts:\n")

			// Render patch data
			if len(result.PatchData) > 0 {
				fmt.Fprintf(out, "    patch:\n")
				lines := strings.Split(string(result.PatchData), "\n")
				for _, line := range lines[:minInt(len(lines), 5)] { // Show first 5 lines
					if line != "" {
						fmt.Fprintf(out, "      %s\n", line)
					}
				}
				if len(lines) > 5 {
					fmt.Fprintf(out, "      ... (%d more lines)\n", len(lines)-5)
				}
			}

			// Render artifacts map
			if len(result.Artifacts) > 0 {
				for key, content := range result.Artifacts {
					fmt.Fprintf(out, "    %s:\n", key)
					lines := strings.Split(string(content), "\n")
					for _, line := range lines[:minInt(len(lines), 3)] { // Show first 3 lines
						if line != "" {
							fmt.Fprintf(out, "      %s\n", line)
						}
					}
					if len(lines) > 3 {
						fmt.Fprintf(out, "      ... (%d more lines)\n", len(lines)-3)
					}
				}
			}
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// cmdPause transitions a thread from active to paused.
func (oc *OperatorConsole) cmdPause(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("pause requires thread-id argument")
	}
	threadID := args[0]

	if oc.threads == nil {
		return "", fmt.Errorf("thread tracker not configured")
	}

	current := oc.threads.Status(threadID)
	if current != StatusActive && current != StatusQueued {
		return "", fmt.Errorf("cannot pause thread in status %s (must be active or queued)", current)
	}

	oc.threads.Set(threadID, StatusPaused)

	// Audit the transition
	if oc.audit != nil {
		_, _ = oc.audit.Append(AuditEntry{
			Ts:       oc.now(),
			Actor:    "operator",
			Kind:     AuditKindTransition,
			ThreadID: threadID,
			Detail:   fmt.Sprintf("transition: %s -> paused", current),
		})
	}

	return fmt.Sprintf("ok: thread %s paused (was %s)", threadID, current), nil
}

// cmdBlock transitions a thread from active to blocked.
func (oc *OperatorConsole) cmdBlock(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("block requires thread-id argument")
	}
	threadID := args[0]

	if oc.threads == nil {
		return "", fmt.Errorf("thread tracker not configured")
	}

	current := oc.threads.Status(threadID)
	if current != StatusActive && current != StatusQueued {
		return "", fmt.Errorf("cannot block thread in status %s (must be active or queued)", current)
	}

	oc.threads.Set(threadID, StatusBlocked)

	// Audit the transition
	if oc.audit != nil {
		_, _ = oc.audit.Append(AuditEntry{
			Ts:       oc.now(),
			Actor:    "operator",
			Kind:     AuditKindTransition,
			ThreadID: threadID,
			Detail:   fmt.Sprintf("transition: %s -> blocked", current),
		})
	}

	return fmt.Sprintf("ok: thread %s blocked (was %s)", threadID, current), nil
}

// cmdResume transitions a thread from paused or blocked back to active.
func (oc *OperatorConsole) cmdResume(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("resume requires thread-id argument")
	}
	threadID := args[0]

	if oc.threads == nil {
		return "", fmt.Errorf("thread tracker not configured")
	}

	current := oc.threads.Status(threadID)
	if current != StatusPaused && current != StatusBlocked {
		return "", fmt.Errorf("cannot resume thread in status %s (must be paused or blocked)", current)
	}

	oc.threads.Set(threadID, StatusActive)

	// Audit the transition
	if oc.audit != nil {
		_, _ = oc.audit.Append(AuditEntry{
			Ts:       oc.now(),
			Actor:    "operator",
			Kind:     AuditKindTransition,
			ThreadID: threadID,
			Detail:   fmt.Sprintf("transition: %s -> active", current),
		})
	}

	return fmt.Sprintf("ok: thread %s resumed (was %s)", threadID, current), nil
}

// cmdRequeue re-pushes a directive back onto the queue and sets thread status to queued.
func (oc *OperatorConsole) cmdRequeue(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("requeue requires thread-id argument")
	}
	threadID := args[0]

	// Look up the directive from our thread→directive index
	d, ok := oc.threadToDirective[threadID]
	if !ok {
		return "", fmt.Errorf("thread %s has no directive in index (requeue requires create or previous tracking)", threadID)
	}

	// Re-push the directive onto the queue
	newID, err := oc.q.Push(d)
	if err != nil {
		return "", fmt.Errorf("re-push directive: %w", err)
	}

	// Set thread status back to queued
	if oc.threads != nil {
		oc.threads.Set(threadID, StatusQueued)
	}

	// Audit the requeue as a mutation
	if oc.audit != nil {
		_, _ = oc.audit.Append(AuditEntry{
			Ts:       oc.now(),
			Actor:    "operator",
			Kind:     AuditKindMutation,
			ThreadID: threadID,
			Detail:   fmt.Sprintf("requeue: directive re-emitted (old=%s, new=%s)", threadID, newID),
		})
	}

	return fmt.Sprintf("ok: requeue thread %s (directive %s re-emitted)", threadID, newID), nil
}

// cmdBudget updates a thread's budget policy with explicit operator approval.
// Syntax: budget <thread-id> <field> <value>
// Example: budget thr-1 per_thread_hard_ceiling 20.0
// Field names: per_message_hard_ceiling, per_run_hard_ceiling, per_thread_hard_ceiling, etc.
func (oc *OperatorConsole) cmdBudget(args []string) (string, error) {
	if len(args) != 3 {
		return "", fmt.Errorf("budget: expected 3 arguments (thread-id, field, value), got %d", len(args))
	}
	threadID := args[0]
	fieldName := args[1]
	valueStr := args[2]

	// Parse the value.
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return "", fmt.Errorf("budget: invalid value %q: %v", valueStr, err)
	}

	// Retrieve the thread and its budget policy.
	if oc.threadStore == nil {
		return "", fmt.Errorf("budget: thread store not available")
	}

	thread, ok := oc.threadStore.Get(threadID)
	if !ok {
		return "", fmt.Errorf("budget: thread %q not found", threadID)
	}

	if thread.BudgetPolicy == nil {
		return "", fmt.Errorf("budget: thread %q has no budget policy", threadID)
	}

	// Apply the operator-approved mutation.
	oldValue, err := thread.BudgetPolicy.ApplyOperatorMutation(fieldName, value, "operator")
	if err != nil {
		return "", fmt.Errorf("budget: mutation failed: %v", err)
	}

	// Update the thread in the store.
	oc.threadStore.Put(thread)

	// Audit the budget change.
	if oc.audit != nil {
		_, _ = oc.audit.Append(AuditEntry{
			Ts:       oc.now(),
			Actor:    "operator",
			Kind:     AuditKindMutation,
			ThreadID: threadID,
			Detail: fmt.Sprintf(
				"budget_update: field=%s, old=%.3f, new=%.3f",
				fieldName, oldValue, value,
			),
		})
	}

	return fmt.Sprintf("ok: updated thread %s budget %s from %.3f to %.3f", threadID, fieldName, oldValue, value), nil
}
