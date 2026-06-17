package main

import (
	"context"
	"fmt"
	"time"
)

// Provider specifies the LLM provider for the task.
type Provider string

const (
	ProviderAnthropic   Provider = "anthropic"
	ProviderOpenAI      Provider = "openai"
	ProviderOllamaCloud Provider = "ollama-cloud"
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

	// ImageName is the Incus image name to launch from (NixOS-only, e.g., "images:nixos/25.11").
	// If empty, defaults to DefaultImageName (images:nixos/25.11).
	ImageName string

	// Timeout is the maximum duration for the task.
	// If zero, defaults to DefaultTimeout.
	Timeout time.Duration

	// KeepOnFailure if true keeps the container alive on task failure for debugging.
	KeepOnFailure bool

	// Env is a map of environment variables to set inside the container.
	Env map[string]string

	// Provider specifies the LLM provider (anthropic, openai, ollama-cloud).
	// If empty, defaults to ProviderAnthropic.
	Provider Provider

	// Model specifies the model name within the provider (e.g., "claude-3-5-haiku", "gpt-4o-mini").
	// Required if Provider is set and not ProviderAnthropic with default model.
	Model string

	// RunAsRoot if true launches the container with root/privileged access.
	// Allows agents to install dependencies. Default is false.
	RunAsRoot bool

	// SharedNixStore if true (default for NixOS images) attaches the shared nix volume
	// for dependency sharing via binary cache. Default: true for NixOS.
	SharedNixStore bool

	// BinaryCachePath is the path on the shared nix volume where prebuilt packages are cached.
	// If set, workers will configure nix to use this as a read-only cache.
	// Default: "/srv/nix-shared" (relative to worker /srv mount point).
	BinaryCachePath string

	// ExternalGradingCheckout is the path to a clean checkout for external grading.
	// If set, the dispatcher will run the oracle there instead of in the worker.
	// Used for verifying diffs that the worker produced.
	ExternalGradingCheckout string
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

	// ExternalGradingResult contains the results of external grading (if enabled).
	// Nil if external grading was not run.
	ExternalGradingResult *GradingResult
}

// GradingResult captures the output of running the oracle on a clean checkout.
type GradingResult struct {
	// ExitCode is the exit code from the oracle.
	ExitCode int

	// Stdout is the standard output from the oracle.
	Stdout string

	// Stderr is the standard error from the oracle.
	Stderr string

	// Duration is how long the oracle took to run.
	Duration time.Duration

	// PatchApplied is true if the diff from the worker was successfully applied.
	PatchApplied bool

	// ApplyError is non-nil if the patch failed to apply.
	ApplyError string
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
	// DefaultImageName is the default Incus image (NixOS-only).
	// NixOS is the only supported OS for reproducible, clean dependency auditing.
	DefaultImageName = "images:nixos/25.11"

	// DefaultTimeout is the default task duration limit.
	DefaultTimeout = 1 * time.Hour

	// DefaultRemote is the default Incus remote.
	DefaultRemote = "ndn-desktop"

	// ContainerNamePrefix is prepended to all ephemeral container names.
	ContainerNamePrefix = "dispatch-"
)

// ValidateProvider checks if a provider is valid and sets defaults.
func (p *Provider) ValidateProvider() error {
	if *p == "" {
		*p = ProviderAnthropic
		return nil
	}
	switch *p {
	case ProviderAnthropic, ProviderOpenAI, ProviderOllamaCloud:
		return nil
	default:
		return fmt.Errorf("invalid provider: %s", *p)
	}
}
