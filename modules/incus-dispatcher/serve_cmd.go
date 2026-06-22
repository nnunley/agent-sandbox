package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/agent-sandbox/incus-dispatcher/queue"
	"github.com/agent-sandbox/incus-dispatcher/queue/laneqpb"
)

// runServeCommand runs the coordinator daemon (STORY-0007 AC-2): a single long-running
// process that drains the directive queue via the D4 loop until SIGINT/SIGTERM. This is the
// coordinator topology — one persistent daemon hosting many one-shot agents (D3), the
// systemd entrypoint for the durable micro-VM (STORY-0007 AC-1).
//
// The queue backend is selectable via --queue flag: memory (default, ITER-0000 stub) or
// laneq (ITER-0006 cluster substrate). Both implement queue.Queue; the daemon logic is
// unchanged (STORY-0002 AC-2).
func runServeCommand(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	remote := fs.String("remote", DefaultRemote, "Incus remote for the worker backend")
	poll := fs.Duration("poll", time.Second, "poll interval when the queue is empty")
	consumer := fs.String("consumer", "coordinator", "consumer id for queue leases")
	queueType := fs.String("queue", "memory", "queue backend: 'memory' (default) or 'laneq'")
	laneqAddr := fs.String("laneq-addr", "localhost:50051", "laneq gRPC server address (used if --queue=laneq)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	runner, err := NewClientContainerRunner(*remote)
	if err != nil {
		log.Printf("serve: runner init failed: %v", err)
		return 1
	}
	// Isolation-tier backends (ITER-0005b): Fast → in-guest nspawn --ephemeral disposable
	// units (STORY-0021); Hard → per-task Firecracker microVM (STORY-0022). The daemon
	// resolves the tier from the vetted template (Policy.TierFor) and selects here; an
	// unregistered tier fails safe (park + escalate) rather than running on a weaker substrate.
	backend := newStaticBackendFactory(map[IsolationTier]Runner{
		TierFast: NewNspawnRunner(*remote),
		TierHard: NewFirecrackerRunner(*remote),
	})

	// ITER-0006 T5: Backend selector (STORY-0002 AC-2).
	q, err := buildQueue(*queueType, *laneqAddr)
	if err != nil {
		log.Printf("serve: queue init failed: %v", err)
		return 1
	}

	dm := &Daemon{
		Q:        q,
		Runner:   runner, // fallback for directives whose template declares no tier
		Backend:  backend,
		Policy:   &Policy{Templates: map[string]TemplateRule{}},
		Consumer: *consumer,
		LeaseDur: time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("coordinator daemon up (remote=%s, queue=%s, poll=%s); draining until signal", *remote, *queueType, *poll)
	stats, serr := Serve(ctx, dm, ServeOptions{PollInterval: *poll})
	log.Printf("coordinator daemon stopped: claimed=%d done=%d requeued=%d escalated=%d rejected=%d",
		stats.Claimed, stats.Done, stats.Requeued, stats.Escalated, stats.Rejected)
	if serr != nil {
		log.Printf("serve error: %v", serr)
		return 1
	}
	return 0
}

// buildQueue constructs the requested queue backend.
// Supported types: "memory" (default, ITER-0000 stub) and "laneq" (ITER-0006 cluster substrate).
// For laneq, laneqAddr must be a valid gRPC server address (e.g., "localhost:50051").
//
// ITER-0006 T5 / STORY-0002 AC-2: The Temporal-sole-writer seam.
// The daemon's claim path (Claim/Touch/Done/Requeue) only READS the scheduling fields
// (priority, not_before_unix). These fields are written ONLY via laneq's gRPC Defer and
// Reprioritize ops. In ITER-0007, Temporal becomes the sole caller of those gRPC ops,
// enabling external urgency resurfacing and backoff management. The daemon remains a
// read-only consumer of the scheduling state; Temporal drives the scheduling decision.
func buildQueue(queueType, laneqAddr string) (queue.Queue, error) {
	switch queueType {
	case "memory":
		return queue.NewMemoryQueue(), nil

	case "laneq":
		if laneqAddr == "" {
			return nil, fmt.Errorf("laneq backend requires --laneq-addr (e.g., localhost:50051)")
		}

		// Dial the laneq gRPC server with insecure transport (no TLS for now; ITER-0007 adds cert-based auth).
		conn, err := grpc.NewClient(laneqAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, fmt.Errorf("laneq dial %q: %w", laneqAddr, err)
		}

		// TODO(ITER-0006/0007): Conn lifecycle: The gRPC conn is held by the LaneqQueue
		// and lives for the lifetime of the daemon. On daemon shutdown (ctx cancellation
		// in runServeCommand), the conn should be closed. For now, we rely on process
		// exit cleanup; a future refactor will thread the conn through to a cleanup function.

		client := laneqpb.NewLaneqClient(conn)
		return queue.NewLaneqQueue(client, "default"), nil

	default:
		return nil, fmt.Errorf("unknown queue backend %q (must be 'memory' or 'laneq')", queueType)
	}
}
