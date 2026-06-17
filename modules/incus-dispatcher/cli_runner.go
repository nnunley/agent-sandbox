package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CLIContainerRunner implements Runner using Incus CLI commands.
type CLIContainerRunner struct {
	remote        string
	containerName string
}

// NewCLIContainerRunner creates a new CLI-based container runner.
// remote is the Incus remote name (e.g., "ndn-desktop").
// Returns a Runner and any error verifying the remote.
func NewCLIContainerRunner(remote string) (*CLIContainerRunner, error) {
	// Verify remote is reachable by checking if we can list containers.
	cmd := exec.Command("incus", "list", remote+":")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("incus remote %s not reachable: %w", remote, err)
	}

	return &CLIContainerRunner{
		remote: remote,
	}, nil
}

// Run executes the task inside an ephemeral Incus container.
func (cr *CLIContainerRunner) Run(ctx context.Context, task Task) (*Result, error) {
	if err := task.validate(); err != nil {
		return nil, fmt.Errorf("invalid task: %w", err)
	}

	imageName := task.ImageName
	if imageName == "" {
		imageName = DefaultImageName
	}

	timeout := task.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	// Create context with timeout for the entire task.
	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Generate unique container name.
	cr.containerName = generateContainerName(task.Name)
	result := &Result{
		ContainerName: cr.containerName,
	}

	// Phase 1: Launch container
	if err := cr.launchContainer(taskCtx, imageName, task); err != nil {
		return result, fmt.Errorf("launch container: %w", err)
	}
	defer cr.cleanup()

	// Phase 2: Deliver source (git bundle or clone)
	if task.Repo != "" {
		if err := cr.deliverSource(taskCtx, task); err != nil {
			return result, fmt.Errorf("deliver source: %w", err)
		}
	}

	// Phase 3: Run command
	start := time.Now()
	if len(task.Cmd) > 0 {
		exitCode, stdout, stderr, err := cr.execCommand(taskCtx, task.Env, task.Cmd)
		result.ExitCode = exitCode
		result.Stdout = stdout
		result.Stderr = stderr
		result.Duration = time.Since(start)
		if err != nil && !isCommandExitErr(err) {
			return result, fmt.Errorf("exec command: %w", err)
		}
	}

	// Phase 4: Harvest results
	if err := cr.harvestResults(taskCtx, result); err != nil {
		return result, fmt.Errorf("harvest results: %w", err)
	}

	return result, nil
}

// Cleanup removes the ephemeral container.
func (cr *CLIContainerRunner) Cleanup() error {
	if cr.containerName == "" {
		return nil
	}
	return cr.cleanup()
}

// launchContainer creates and starts an ephemeral container.
func (cr *CLIContainerRunner) launchContainer(ctx context.Context, imageName string, task Task) error {
	// Use incus CLI for simplicity (the Go client requires more setup for container ops).
	// If image doesn't contain a colon (remote), prepend the current remote.
	if !strings.Contains(imageName, ":") {
		imageName = cr.remote + ":" + imageName
	}

	// Build launch command with optional flags.
	args := []string{"launch", imageName, cr.containerName, "--ephemeral"}

	// Add root/privileged flag if requested.
	if task.RunAsRoot {
		args = append(args, "--config", "security.privileged=true")
	}

	// Launch ephemeral container.
	cmd := exec.CommandContext(ctx, "incus", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("incus launch failed: %s (output: %s)", err, out)
	}

	// Attach shared binary cache volume (read-only) if requested.
	// This allows workers to pull prebuilt packages from the shared cache.
	if task.SharedNixStore {
		cachePath := task.BinaryCachePath
		if cachePath == "" {
			cachePath = "/srv/nix-shared"
		}

		// Attach nix-shared volume read-only at the configured cache path
		deviceArgs := []string{"config", "device", "add", cr.remote + ":" + cr.containerName, "nix-cache",
			"disk", "pool=default", "source=nix-shared", "path=" + cachePath, "readonly=true"}
		deviceCmd := exec.CommandContext(ctx, "incus", deviceArgs...)
		if out, err := deviceCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("attach nix cache volume failed: %s (output: %s)", err, out)
		}
	}

	// Wait for container to be ready.
	if err := cr.waitReady(ctx); err != nil {
		return fmt.Errorf("container not ready: %w", err)
	}

	// Configure binary cache and start nix-daemon if using shared nix store
	if task.SharedNixStore {
		if err := cr.configureNixCache(ctx, task); err != nil {
			return fmt.Errorf("configure nix cache: %w", err)
		}
	}

	return nil
}

// waitReady waits for the container to be operational (ping with a simple command).
func (cr *CLIContainerRunner) waitReady(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Try a simple echo command.
			cmd := exec.CommandContext(ctx, "incus", "exec", cr.containerName, "--", "echo", "ok")
			if err := cmd.Run(); err == nil {
				return nil
			}
			// Not ready yet; continue polling.
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for container to be ready")
			}
		}
	}
}

// configureNixCache starts nix-daemon and configures the binary cache in the worker.
// This enables workers to pull prebuilt packages from the shared cache without rebuilding.
func (cr *CLIContainerRunner) configureNixCache(ctx context.Context, task Task) error {
	cachePath := task.BinaryCachePath
	if cachePath == "" {
		cachePath = "/srv/nix-shared"
	}

	// Start nix-daemon in the worker
	daemonCmd := exec.CommandContext(ctx, "incus", "exec", cr.containerName, "--",
		"systemctl", "start", "nix-daemon.socket", "nix-daemon.service")
	if out, err := daemonCmd.CombinedOutput(); err != nil {
		// Log but don't fail; daemon may already be running
		fmt.Printf("warning: failed to start nix-daemon: %s\n", out)
	}

	// Configure nix to use the shared cache as a substituter
	// Write nix config to enable the cache and disable signature checks
	confCmd := exec.CommandContext(ctx, "incus", "exec", cr.containerName, "--",
		"sh", "-c", fmt.Sprintf(`mkdir -p /etc/nix/nix.conf.d && cat > /etc/nix/nix.conf.d/cache.conf << 'EOFCONF'
extra-substituters = file://%s
require-sigs = false
EOFCONF
`, cachePath))

	if out, err := confCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("configure nix substituters: %s (output: %s)", err, out)
	}

	return nil
}

// deliverSource pushes the git repository into the container.
// Supports both local paths (via git bundle) and remote URLs (via shallow clone).
func (cr *CLIContainerRunner) deliverSource(ctx context.Context, task Task) error {
	repoPath := "/repo"

	if isLocalPath(task.Repo) {
		// Local path: create git bundle on host, push to container, clone.
		return cr.deliverViaBundle(ctx, task.Repo, task.Ref, repoPath)
	}

	// Remote URL: shallow clone directly in container.
	return cr.deliverViaClone(ctx, task.Repo, task.Ref, task.TargetBranch, repoPath)
}

// deliverViaBundle creates a git bundle on the host and pushes it to the container.
func (cr *CLIContainerRunner) deliverViaBundle(ctx context.Context, localPath, ref, containerPath string) error {
	bundlePath := filepath.Join("/tmp", cr.containerName+".bundle")
	defer removeFile(bundlePath)

	// Create bundle on host.
	bundleCmd := exec.CommandContext(ctx, "git", "-C", localPath, "bundle", "create", bundlePath, ref)
	if out, err := bundleCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create bundle: %s (output: %s)", err, out)
	}

	// Push bundle to container.
	pushCmd := exec.CommandContext(ctx, "incus", "file", "push", bundlePath, cr.remote+":"+cr.containerName+"/"+filepath.Join(containerPath, "repo.bundle"))
	if out, err := pushCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("push bundle: %s (output: %s)", err, out)
	}

	// Clone from bundle inside container.
	cloneCmd := exec.CommandContext(ctx, "incus", "exec", cr.containerName, "--", "bash", "-c",
		fmt.Sprintf("mkdir -p %s && git clone %s %s", containerPath, filepath.Join(containerPath, "repo.bundle"), filepath.Join(containerPath, "src")))
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clone from bundle: %s (output: %s)", err, out)
	}

	return nil
}

// deliverViaClone clones a remote repository directly in the container.
func (cr *CLIContainerRunner) deliverViaClone(ctx context.Context, repoURL, ref, targetBranch, containerPath string) error {
	cloneCmd := exec.CommandContext(ctx, "incus", "exec", cr.containerName, "--", "bash", "-c",
		fmt.Sprintf("mkdir -p %s && git clone --depth 1 --branch %s %s %s",
			containerPath, ref, repoURL, filepath.Join(containerPath, "src")))
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clone repo: %s (output: %s)", err, out)
	}

	// If target branch specified, create it.
	if targetBranch != "" {
		branchCmd := exec.CommandContext(ctx, "incus", "exec", cr.containerName, "--",
			"git", "-C", filepath.Join(containerPath, "src"), "checkout", "-b", targetBranch)
		if out, err := branchCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("create target branch: %s (output: %s)", err, out)
		}
	}

	return nil
}

// execCommand runs a command inside the container and captures output.
// For NixOS containers, ensures PATH includes /run/current-system/sw/bin.
func (cr *CLIContainerRunner) execCommand(ctx context.Context, env map[string]string, cmd []string) (int, string, string, error) {
	// Build environment for incus exec.
	// Ensure PATH is set for NixOS binaries.
	allEnv := make(map[string]string)
	if len(env) > 0 {
		for k, v := range env {
			allEnv[k] = v
		}
	}

	// Set PATH if not provided, or prepend NixOS path.
	if _, hasPath := allEnv["PATH"]; !hasPath {
		allEnv["PATH"] = "/run/current-system/sw/bin:/usr/bin:/bin"
	} else {
		// Prepend NixOS path if not already included.
		if !strings.Contains(allEnv["PATH"], "/run/current-system/sw/bin") {
			allEnv["PATH"] = "/run/current-system/sw/bin:" + allEnv["PATH"]
		}
	}

	// Build incus exec command.
	// Format: incus exec container [--env VAR=value ...] -- command args
	args := []string{"exec", cr.containerName}

	// Set all environment variables BEFORE the -- separator.
	for k, v := range allEnv {
		args = append(args, "--env", fmt.Sprintf("%s=%s", k, v))
	}

	// Now add the command separator and the command itself.
	args = append(args, "--")
	args = append(args, cmd...)

	execCmd := exec.CommandContext(ctx, "incus", args...)

	// Capture output.
	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	err := execCmd.Run()

	// Determine exit code.
	exitCode := 0
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			exitCode = e.ExitCode()
		} else if isContextErr(err) {
			exitCode = -1 // Timeout or cancelled.
		} else {
			return 0, "", "", err
		}
	}

	return exitCode, stdout.String(), stderr.String(), nil
}

// harvestResults collects git format-patch and /output artifacts from the container.
func (cr *CLIContainerRunner) harvestResults(ctx context.Context, result *Result) error {
	// Try to harvest a git patch if there were commits.
	patchData, err := cr.harvestPatch(ctx)
	if err == nil && len(patchData) > 0 {
		result.PatchData = patchData
	}

	// Harvest /output artifacts if they exist.
	artifacts, err := cr.harvestOutputArtifacts(ctx)
	if err == nil && len(artifacts) > 0 {
		result.Artifacts = artifacts
	}

	return nil
}

// harvestPatch generates git format-patch from /repo/src if it exists.
func (cr *CLIContainerRunner) harvestPatch(ctx context.Context) ([]byte, error) {
	patchPath := filepath.Join("/tmp", cr.containerName+".patch")
	defer removeFile(patchPath)

	// Generate patch (ignoring error if no repo or no commits).
	cmd := exec.CommandContext(ctx, "incus", "exec", cr.containerName, "--", "bash", "-c",
		fmt.Sprintf("cd /repo/src 2>/dev/null && git format-patch -o /tmp origin/HEAD~1..HEAD 2>/dev/null || true"))
	if err := cmd.Run(); err != nil && !isContextErr(err) {
		// Not fatal — might not have a repo.
	}

	// Pull patch from container.
	pullCmd := exec.CommandContext(ctx, "incus", "file", "pull", cr.containerName+":"+"/*.patch", patchPath)
	if err := pullCmd.Run(); err != nil {
		// No patch available — this is OK.
		return nil, nil
	}

	// Read patch file.
	data, err := readFile(patchPath)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// harvestOutputArtifacts pulls files from /output if it exists.
func (cr *CLIContainerRunner) harvestOutputArtifacts(ctx context.Context) (map[string][]byte, error) {
	artifacts := make(map[string][]byte)

	// Check if /output exists.
	checkCmd := exec.CommandContext(ctx, "incus", "exec", cr.containerName, "--", "test", "-d", "/output")
	if err := checkCmd.Run(); err != nil {
		// /output doesn't exist — nothing to harvest.
		return artifacts, nil
	}

	// List files in /output using ls since find may not support -printf on Alpine.
	listCmd := exec.CommandContext(ctx, "incus", "exec", cr.containerName, "--",
		"sh", "-c", "find /output -type f")
	var listOut bytes.Buffer
	listCmd.Stdout = &listOut
	if err := listCmd.Run(); err != nil {
		// Error listing — continue anyway.
		return artifacts, nil
	}

	// Pull each file.
	fileList := strings.TrimSpace(listOut.String())
	if fileList == "" {
		return artifacts, nil
	}

	files := strings.Split(fileList, "\n")
	for _, srcPath := range files {
		if srcPath == "" || srcPath == "/output" {
			continue
		}

		// Extract relative path from /output
		relPath := strings.TrimPrefix(srcPath, "/output/")
		dstPath := filepath.Join("/tmp", cr.containerName+"-"+relPath)

		// Create parent directory if needed
		dstDir := filepath.Dir(dstPath)
		_ = os.MkdirAll(dstDir, 0755)
		defer removeFile(dstPath)

		pullCmd := exec.CommandContext(ctx, "incus", "file", "pull", cr.remote+":"+cr.containerName+"/"+srcPath, dstPath)
		if err := pullCmd.Run(); err != nil {
			// Skip files that can't be pulled.
			continue
		}

		data, err := readFile(dstPath)
		if err != nil {
			continue
		}
		artifacts[relPath] = data
	}

	return artifacts, nil
}

// cleanup removes the container.
func (cr *CLIContainerRunner) cleanup() error {
	if cr.containerName == "" {
		return nil
	}
	// Ephemeral containers auto-cleanup on stop.
	// But we need to explicitly stop/delete if needed.
	cmd := exec.Command("incus", "delete", cr.containerName, "-f")
	_ = cmd.Run() // Ignore error if container is already gone or ephemeral.
	return nil
}

// Helper functions

func (t *Task) validate() error {
	if t.Name == "" {
		return fmt.Errorf("task name required")
	}
	if len(t.Cmd) == 0 && t.Repo == "" {
		return fmt.Errorf("task must have either Cmd or Repo")
	}
	// Default SharedNixStore to true for NixOS images.
	if strings.Contains(t.ImageName, "nixos") || t.ImageName == DefaultImageName {
		t.SharedNixStore = true
	}
	return nil
}

func generateContainerName(taskName string) string {
	// Incus container names must match [a-zA-Z0-9][-a-zA-Z0-9]*
	sanitized := strings.ToLower(taskName)
	sanitized = strings.ReplaceAll(sanitized, "_", "-")
	sanitized = strings.ReplaceAll(sanitized, ".", "-")
	return ContainerNamePrefix + sanitized + "-" + fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
}

func isLocalPath(path string) bool {
	return strings.HasPrefix(path, "/") || strings.HasPrefix(path, ".") || strings.HasPrefix(path, "~")
}

func isCommandExitErr(err error) bool {
	_, ok := err.(*exec.ExitError)
	return ok
}

func isContextErr(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

func removeFile(path string) {
	_ = exec.Command("rm", "-f", path).Run()
}

func readFile(path string) ([]byte, error) {
	cmd := exec.Command("cat", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
