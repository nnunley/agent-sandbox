package temporal

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/agent-sandbox/incus-dispatcher/queue"
)

// WorkerConfig holds configuration for the Temporal worker.
type WorkerConfig struct {
	// TemporalAddress is the Temporal server address (e.g., "127.0.0.1:7233").
	TemporalAddress string
	// TaskQueue is the Temporal task queue name (e.g., "priority-workflow").
	TaskQueue string
	// Namespace is the Temporal namespace (e.g., "default").
	Namespace string
}

// Worker encapsulates the Temporal worker and client.
type Worker struct {
	client     client.Client
	worker     worker.Worker
	config     WorkerConfig
	activities *Activities
}

// NewWorker creates a new Temporal worker with the given configuration.
// If q is nil, a default Activities struct with nil Queue is used (tests must inject a fake).
func NewWorker(ctx context.Context, cfg WorkerConfig, q queue.Queue) (*Worker, error) {
	// Validate queue early: if a non-nil queue is provided, it must implement Reprojector.
	// This fails fast instead of silently leaving Queue=nil (which would fail at activity runtime).
	var reprojector Reprojector
	if q != nil {
		// *queue.LaneqQueue implements Reprojector directly.
		var ok bool
		reprojector, ok = q.(Reprojector)
		if !ok {
			return nil, fmt.Errorf("queue %T does not implement Reprojector (expected *queue.LaneqQueue)", q)
		}
	}

	if cfg.TemporalAddress == "" {
		cfg.TemporalAddress = "127.0.0.1:7233"
	}
	if cfg.TaskQueue == "" {
		cfg.TaskQueue = "priority-workflow"
	}
	if cfg.Namespace == "" {
		cfg.Namespace = "default"
	}

	// Create Temporal client.
	c, err := client.Dial(client.Options{
		HostPort:  cfg.TemporalAddress,
		Namespace: cfg.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Temporal client: %w", err)
	}

	// Create worker.
	w := worker.New(c, cfg.TaskQueue, worker.Options{})

	activities := &Activities{
		Queue: reprojector,
	}

	return &Worker{
		client:     c,
		worker:     w,
		config:     cfg,
		activities: activities,
	}, nil
}

// Register registers workflow and activity definitions with the worker.
func (w *Worker) Register() {
	w.worker.RegisterWorkflow(PriorityWorkflow)
	w.worker.RegisterActivity(w.activities.ReprojectActivity)
}

// Start starts the worker.
func (w *Worker) Start(ctx context.Context) error {
	return w.worker.Start()
}

// Stop stops the worker and closes the client.
func (w *Worker) Stop(ctx context.Context) error {
	w.worker.Stop()
	w.client.Close()
	return nil
}

// Client returns the underlying Temporal client.
func (w *Worker) Client() client.Client {
	return w.client
}

