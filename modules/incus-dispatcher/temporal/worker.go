package temporal

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
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
	client client.Client
	worker worker.Worker
	config WorkerConfig
}

// NewWorker creates a new Temporal worker with the given configuration.
func NewWorker(ctx context.Context, cfg WorkerConfig) (*Worker, error) {
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

	return &Worker{
		client: c,
		worker: w,
		config: cfg,
	}, nil
}

// Register registers workflow and activity definitions with the worker.
func (w *Worker) Register() {
	w.worker.RegisterWorkflow(PriorityWorkflow)
	// Activities will be registered here as they are defined.
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

// PriorityWorkflow is a stub workflow that will be implemented in C2.
// For now, it's a placeholder that the worker skeleton can register.
func PriorityWorkflow(ctx workflow.Context) error {
	// TODO: Implement PriorityWorkflow in C2.
	// This will schedule timers based on urgency/deadline and re-project
	// priority into laneq via Reprioritize.
	return nil
}
