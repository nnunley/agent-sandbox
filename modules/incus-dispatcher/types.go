package main

import (
	"context"
	"time"
)

// Task describes a workload to run inside a container.
type Task struct {
	// Name is a unique identifier for this task (used in container naming).
	Name string

	// Repo is the git repository path (local) or URL to deliver.
	// If it starts with / or contains /, it's a local path; otherwise a URL.
	Repo string

	// Ref is the git ref (branch/commit/tag) to check out.
	Ref string

	// TargetBranch is the branch to create for the work (optional).
	// If empty, work is done on the ref directly.
	TargetBranch string

	// Cmd is the command to run inside the container.
	Cmd []string

	// ImageName is the Incus image name to launch from (e.g., "ubuntu/24.04").
	// If empty, defaults to DefaultImageName.
	ImageName string

	// Timeout is the maximum duration for the task.
	// If zero, defaults to DefaultTimeout.
	Timeout time.Duration

	// KeepOnFailure if true keeps the container alive on task failure for debugging.
	KeepOnFailure bool

	// Env is a map of environment variables to set inside the container.
	Env map[string]string
}

// Result captures the output and status of a completed task.
type Result struct {
	// ExitCode is the exit code of the command.
	ExitCode int

	// Stdout is the standard output from the container command.
	Stdout string

	// Stderr is the standard error from the container command.
	Stderr string

	// ContainerName is the ephemeral container that ran the task.
	// Useful for debugging if KeepOnFailure was set.
	ContainerName string

	// Duration is how long the task took to run.
	Duration time.Duration

	// PatchData contains git format-patch output if available.
	PatchData []byte

	// Artifacts is a map of files/directories harvested from /output.
	Artifacts map[string][]byte
}

// Runner abstracts the execution backend.
// Implementations can be container-based, VM-based, etc.
type Runner interface {
	// Run executes the task and returns the result.
	// ctx can be used for timeouts and cancellation.
	Run(ctx context.Context, task Task) (*Result, error)

	// Cleanup removes ephemeral resources (containers, VMs, etc.).
	// It's a no-op if there's nothing to clean up.
	Cleanup() error
}

const (
	// DefaultImageName is the default Incus image.
	DefaultImageName = "images:ubuntu/24.04"

	// DefaultTimeout is the default task duration limit.
	DefaultTimeout = 1 * time.Hour

	// DefaultRemote is the default Incus remote.
	DefaultRemote = "ndn-desktop"

	// ContainerNamePrefix is prepended to all ephemeral container names.
	ContainerNamePrefix = "dispatch-"
)
