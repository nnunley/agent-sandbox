package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// runServeCommand runs the coordinator daemon (STORY-0007 AC-2): a single long-running
// process that drains the directive queue via the D4 loop until SIGINT/SIGTERM. This is the
// coordinator topology — one persistent daemon hosting many one-shot agents (D3), the
// systemd entrypoint for the durable micro-VM (STORY-0007 AC-1).
//
// The queue is the stub MemoryQueue; the durable laneq substrate (ITER-0006, Patrick-blocked)
// swaps in behind the same queue.Queue interface without changing this loop.
func runServeCommand(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	remote := fs.String("remote", DefaultRemote, "Incus remote for the worker backend")
	poll := fs.Duration("poll", time.Second, "poll interval when the queue is empty")
	consumer := fs.String("consumer", "coordinator", "consumer id for queue leases")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	runner, err := NewClientContainerRunner(*remote)
	if err != nil {
		log.Printf("serve: runner init failed: %v", err)
		return 1
	}
	dm := &Daemon{
		Q:        queue.NewMemoryQueue(),
		Runner:   runner,
		Policy:   &Policy{Templates: map[string]TemplateRule{}},
		Consumer: *consumer,
		LeaseDur: time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("coordinator daemon up (remote=%s, poll=%s); draining until signal", *remote, *poll)
	stats, serr := Serve(ctx, dm, ServeOptions{PollInterval: *poll})
	log.Printf("coordinator daemon stopped: claimed=%d done=%d requeued=%d escalated=%d rejected=%d",
		stats.Claimed, stats.Done, stats.Requeued, stats.Escalated, stats.Rejected)
	if serr != nil {
		log.Printf("serve error: %v", serr)
		return 1
	}
	return 0
}
