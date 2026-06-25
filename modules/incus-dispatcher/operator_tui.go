package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// OperatorConsole is a stdlib command interpreter for thread and worker management.
// It reads operator commands from an io.Reader (line-based) and writes rendered output to an io.Writer.
// This design is app-level testable by driving stdin→stdout strings without requiring a terminal/TTY.
type OperatorConsole struct {
	q       queue.Queue
	threads *ThreadTracker
	workers map[string]*Worker
	audit   AuditLog
	now     func() time.Time
}

// NewOperatorConsole returns a new OperatorConsole wired with the given dependencies.
func NewOperatorConsole(q queue.Queue, threads *ThreadTracker, workers map[string]*Worker, audit AuditLog, now func() time.Time) *OperatorConsole {
	if now == nil {
		now = time.Now
	}
	return &OperatorConsole{
		q:       q,
		threads: threads,
		workers: workers,
		audit:   audit,
		now:     now,
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
		Intent:    intent,
		Template:  template,
		Repo:      repo,
		Ref:       ref,
		Task:      task,
		Importance: queue.ImportanceNormal,
	}
	id, err := oc.q.Push(d)
	if err != nil {
		return "", fmt.Errorf("push directive: %w", err)
	}

	// Initialize thread status for this directive
	if oc.threads != nil {
		oc.threads.Set(id, StatusQueued)
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
	if oc.threads == nil {
		return "threads: no tracker configured", nil
	}

	var sb strings.Builder
	sb.WriteString("threads:\n")

	// Since ThreadTracker doesn't expose all thread IDs, we can't enumerate all threads.
	// This is a limitation of the current design. For now, we show a message about this.
	// In practice, the daemon would maintain a separate ThreadStore for enumeration.
	sb.WriteString("  (use 'inspect <thread-id>' to view specific thread status)")

	return sb.String(), nil
}

// cmdInspect renders thread status and any associated responses/artifacts.
func (oc *OperatorConsole) cmdInspect(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("inspect requires thread-id or run-id argument")
	}
	id := args[0]

	var sb strings.Builder
	status := oc.threads.Status(id)
	transitions := oc.threads.Transitions(id)

	fmt.Fprintf(&sb, "thread %s:\n", id)
	fmt.Fprintf(&sb, "  status: %s\n", status)
	fmt.Fprintf(&sb, "  transitions: %d\n", len(transitions))
	for i, t := range transitions {
		fmt.Fprintf(&sb, "    [%d] %s -> %s at %s\n", i, t.From, t.To, t.Ts.Format("15:04:05"))
	}

	return strings.TrimRight(sb.String(), "\n"), nil
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

// cmdRequeue re-pushes a directive back onto the queue.
func (oc *OperatorConsole) cmdRequeue(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("requeue requires directive-id argument")
	}
	_ = args[0] // directive ID

	// For a full requeue, we'd need the original directive from the queue.
	// Since the queue interface doesn't expose it, we return an error for now.
	// In production, this would require storing directives in a separate index.
	return "", fmt.Errorf("requeue not yet implemented (requires directive index)")
}
