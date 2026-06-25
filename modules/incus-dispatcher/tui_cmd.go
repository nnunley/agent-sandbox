package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// runTUICommand runs the operator TUI (STORY-0028 + STORY-0027 AC-3): a command-driven
// terminal interface for thread and worker management.
//
// The TUI reads commands from stdin and writes output to stdout. It allows operators to:
// - Create work items (AC-1)
// - View queue and worker state (AC-2)
// - Inspect thread responses and artifacts (AC-3)
// - Pause/block/resume threads (AC-4, STORY-0027 AC-3)
// - Requeue directives (AC-4)
//
// Usage:
//
//	incus-dispatcher tui [--queue memory|laneq] [--laneq-addr localhost:50051]
func runTUICommand(args []string) int {
	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	queueType := fs.String("queue", "memory", "queue backend: 'memory' (default) or 'laneq'")
	laneqAddr := fs.String("laneq-addr", "localhost:50051", "laneq gRPC server address (used if --queue=laneq)")
	laneqGrantFile := fs.String("laneq-grant-file", "", "path to laneq PASETO grant file (enables gRPC auth if set)")
	laneqClientKey := fs.String("laneq-client-key", "", "path to laneq client Ed25519 private key PEM (enables gRPC auth if set)")
	laneqAud := fs.String("laneq-aud", "", "laneq audience for grants, e.g. laneq://agent-host:9999 (enables gRPC auth if set)")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Build queue backend
	q, err := buildQueue(*queueType, *laneqAddr, *laneqGrantFile, *laneqClientKey, *laneqAud)
	if err != nil {
		log.Printf("tui: queue init failed: %v", err)
		return 1
	}

	// Clean up queue on shutdown
	defer func() {
		if c, ok := q.(interface{ Close() error }); ok {
			if err := c.Close(); err != nil {
				log.Printf("tui: queue close failed: %v", err)
			}
		}
	}()

	// Initialize coordinator components
	now := time.Now
	threads := NewThreadTracker(now)
	workers := make(map[string]*Worker)
	audit := NewMemoryAuditLog()

	// Create and run the console
	console := NewOperatorConsole(q, threads, workers, audit, now)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run the TUI with a goroutine for graceful shutdown on signal
	go func() {
		if err := console.Run(os.Stdin, os.Stdout); err != nil {
			log.Printf("tui: console error: %v", err)
		}
		cancel()
	}()

	// Wait for context cancellation (via signal or EOF)
	<-ctx.Done()
	return 0
}
