package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	incus "github.com/lxc/incus/v6/client"
	"github.com/lxc/incus/v6/shared/api"
)

// ClientContainerRunner implements Runner using the Incus Go client.
type ClientContainerRunner struct {
	client        incus.InstanceServer
	remote        string
	containerName string
	project       string
}

// NewClientContainerRunner creates a new Incus Go client-based runner.
// remote is the Incus remote name (e.g., "ndn-desktop").
// Returns a Runner and any error connecting to the remote.
func NewClientContainerRunner(remote string) (*ClientContainerRunner, error) {
	// For ndn-desktop (remote), connect using the configured remote address and certs.
	// Read from incus config to find the remote address and certificate paths.
	remoteAddr, clientCert, clientKey, serverCert, err := loadRemoteConfig(remote)
	if err != nil {
		return nil, fmt.Errorf("load remote config: %w", err)
	}

	// Connect to the remote Incus daemon.
	var client incus.InstanceServer

	if remoteAddr == "unix://" || strings.HasPrefix(remoteAddr, "unix://") {
		// Local Unix socket (e.g., local daemon).
		client, err = incus.ConnectIncusUnix("", nil)
	} else {
		// Remote over HTTPS.
		args := &incus.ConnectionArgs{
			TLSClientCert: clientCert,
			TLSClientKey:  clientKey,
			TLSServerCert: serverCert,
			SkipGetServer: false,
		}
		client, err = incus.ConnectIncus(remoteAddr, args)
	}

	if err != nil {
		return nil, fmt.Errorf("connect to remote %s: %w", remote, err)
	}

	// Verify the connection by getting server info.
	_, _, err = client.GetServer()
	if err != nil {
		client.Disconnect()
		return nil, fmt.Errorf("verify remote %s: %w", remote, err)
	}

	return &ClientContainerRunner{
		client:  client,
		remote:  remote,
		project: "default",
	}, nil
}

// Run executes the task inside an ephemeral Incus container using the Go client.
func (cr *ClientContainerRunner) Run(ctx context.Context, task Task) (*Result, error) {
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

	// Create context with timeout for entire task.
	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Generate unique container name.
	cr.containerName = generateContainerName(task.Name)
	result := &Result{
		ContainerName: cr.containerName,
	}

	// Phase 1: Launch container
	if err := cr.launchContainer(taskCtx, imageName); err != nil {
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
	if len(task.Cmd) > 0 {
		start := time.Now()
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

	// Phase 5: External grading (if configured)
	if task.ExternalGradingCheckout != "" {
		gradingResult, err := runExternalGrading(taskCtx, result.PatchData, task.ExternalGradingCheckout, task.Cmd)
		if err != nil {
			// Log the error but don't fail the entire task
			return result, nil // return what we have
		}
		result.ExternalGradingResult = gradingResult
	}

	return result, nil
}

// Cleanup removes the ephemeral container.
func (cr *ClientContainerRunner) Cleanup() error {
	if cr.containerName == "" {
		return nil
	}
	return cr.cleanup()
}

// launchContainer creates and starts an ephemeral container using the Go client.
func (cr *ClientContainerRunner) launchContainer(ctx context.Context, imageName string) error {
	// Resolve image name with remote if needed.
	if !strings.Contains(imageName, ":") {
		imageName = "images:" + imageName
	}

	// Create instance request.
	// If image starts with "images:", use the public linuxcontainers images server.
	// Otherwise use the remote's existing images.
	req := api.InstancesPost{
		Name: cr.containerName,
		Type: "container",
	}

	if strings.HasPrefix(imageName, "images:") {
		req.Source = api.InstanceSource{
			Type:     "image",
			Alias:    strings.TrimPrefix(imageName, "images:"),
			Server:   "https://images.linuxcontainers.org",
			Protocol: "simplestreams",
		}
	} else {
		// Assume image alias is available locally on the remote
		req.Source = api.InstanceSource{
			Type:  "image",
			Alias: imageName,
		}
	}

	// Create the instance.
	op, err := cr.client.CreateInstance(req)
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}

	// Wait for creation to complete.
	err = op.Wait()
	if err != nil {
		return fmt.Errorf("create instance wait: %w", err)
	}

	// Start the instance.
	startReq := api.InstanceStatePut{
		Action:  "start",
		Timeout: -1,
	}

	op, err = cr.client.UpdateInstanceState(cr.containerName, startReq, "")
	if err != nil {
		return fmt.Errorf("start instance: %w", err)
	}

	err = op.Wait()
	if err != nil {
		return fmt.Errorf("start instance wait: %w", err)
	}

	// Wait for container to be ready.
	if err := cr.waitReady(ctx); err != nil {
		return fmt.Errorf("container not ready: %w", err)
	}

	return nil
}

// waitReady waits for container to be operational.
func (cr *ClientContainerRunner) waitReady(ctx context.Context) error {
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
			// Try a simple exec to see if container responds.
			req := api.InstanceExecPost{
				Command:     []string{"true"},
				WaitForWS:   true,
				Interactive: false,
			}

			op, err := cr.client.ExecInstance(cr.containerName, req, nil)
			if err == nil && op != nil {
				// Wait for the operation to complete.
				_ = op.Wait()
				return nil
			}

			if time.Now().After(deadline) {
				return fmt.Errorf("container not ready after deadline")
			}
		}
	}
}

// deliverSource pushes git repository into container.
func (cr *ClientContainerRunner) deliverSource(ctx context.Context, task Task) error {
	repoPath := "/repo"

	if isLocalPath(task.Repo) {
		return cr.deliverViaBundle(ctx, task.Repo, task.Ref, repoPath)
	}

	return cr.deliverViaClone(ctx, task.Repo, task.Ref, task.TargetBranch, repoPath)
}

// deliverViaBundle creates a git bundle on host and pushes it to container.
func (cr *ClientContainerRunner) deliverViaBundle(ctx context.Context, localPath, ref, containerPath string) error {
	// Create bundle on host.
	bundlePath := filepath.Join("/tmp", cr.containerName+".bundle")
	defer removeFile(bundlePath)

	bundleCmd := newCmdContext(ctx, "git", "-C", localPath, "bundle", "create", bundlePath, ref)
	if out, err := bundleCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create bundle: %s (output: %s)", err, out)
	}

	// Read bundle file.
	bundleData, err := readFile(bundlePath)
	if err != nil {
		return fmt.Errorf("read bundle: %w", err)
	}

	// Push bundle to container.
	args := incus.InstanceFileArgs{
		Content: bytes.NewReader(bundleData),
	}

	bundleTargetPath := filepath.Join(containerPath, "repo.bundle")
	err = cr.client.CreateInstanceFile(cr.containerName, bundleTargetPath, args)
	if err != nil {
		return fmt.Errorf("push bundle: %w", err)
	}

	// Clone from bundle inside container.
	cloneCmd := fmt.Sprintf("mkdir -p %s && git clone %s %s",
		containerPath,
		filepath.Join(containerPath, "repo.bundle"),
		filepath.Join(containerPath, "src"))

	req := api.InstanceExecPost{
		Command:     []string{"sh", "-c", cloneCmd},
		WaitForWS:   true,
		Interactive: false,
	}

	op, err := cr.client.ExecInstance(cr.containerName, req, nil)
	if err != nil {
		return fmt.Errorf("clone from bundle: %w", err)
	}

	err = op.Wait()
	if err != nil {
		return fmt.Errorf("clone from bundle wait: %w", err)
	}

	return nil
}

// deliverViaClone performs a shallow clone of a remote repo inside container.
func (cr *ClientContainerRunner) deliverViaClone(ctx context.Context, repoURL, ref, targetBranch, containerPath string) error {
	// Shallow clone inside container.
	cloneCmd := fmt.Sprintf("mkdir -p %s && git clone --depth 1 --branch %s %s %s",
		containerPath, ref, repoURL, filepath.Join(containerPath, "src"))

	req := api.InstanceExecPost{
		Command:     []string{"sh", "-c", cloneCmd},
		WaitForWS:   true,
		Interactive: false,
	}

	op, err := cr.client.ExecInstance(cr.containerName, req, nil)
	if err != nil {
		return fmt.Errorf("shallow clone: %w", err)
	}

	err = op.Wait()
	if err != nil {
		return fmt.Errorf("shallow clone wait: %w", err)
	}

	// Create target branch if specified.
	if targetBranch != "" {
		branchCmd := fmt.Sprintf("git -C %s checkout -b %s",
			filepath.Join(containerPath, "src"), targetBranch)

		req := api.InstanceExecPost{
			Command:     []string{"bash", "-c", branchCmd},
			WaitForWS:   true,
			Interactive: false,
		}

		op, err := cr.client.ExecInstance(cr.containerName, req, nil)
		if err != nil {
			return fmt.Errorf("create target branch: %w", err)
		}

		err = op.Wait()
		if err != nil {
			return fmt.Errorf("create target branch wait: %w", err)
		}
	}

	return nil
}

// execCommand runs a command inside container and captures output.
func (cr *ClientContainerRunner) execCommand(ctx context.Context, env map[string]string, cmd []string) (int, string, string, error) {
	// Build environment list for exec request.
	var environment map[string]string
	if len(env) > 0 {
		environment = env
	}

	// Capture stdout and stderr using pipes.
	var stdout, stderr bytes.Buffer

	args := &incus.InstanceExecArgs{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// Build exec request.
	req := api.InstanceExecPost{
		Command:     cmd,
		WaitForWS:   true,
		Interactive: false,
		Environment: environment,
	}

	// Execute the command with output capture.
	op, err := cr.client.ExecInstance(cr.containerName, req, args)
	if err != nil {
		return -1, "", "", fmt.Errorf("exec: %w", err)
	}

	if op == nil {
		return -1, stdout.String(), stderr.String(), fmt.Errorf("exec returned nil operation")
	}

	// Wait for command to complete using operation context.
	err = op.Wait()

	// Extract exit code from operation metadata.
	exitCode := 0
	opAPI := op.Get()
	if opAPI.Metadata != nil {
		if code, ok := opAPI.Metadata["return"].(float64); ok {
			exitCode = int(code)
		}
	}

	// Return results even if there's an error (could be command exit code).
	return exitCode, stdout.String(), stderr.String(), err
}

// harvestResults collects git format-patch and /output artifacts from container.
func (cr *ClientContainerRunner) harvestResults(ctx context.Context, result *Result) error {
	// Try to harvest git patch.
	patchData, err := cr.harvestPatch(ctx)
	if err == nil && len(patchData) > 0 {
		result.PatchData = patchData
	}

	// Harvest /output artifacts.
	artifacts, err := cr.harvestOutputArtifacts(ctx)
	if err == nil && len(artifacts) > 0 {
		result.Artifacts = artifacts
	}

	return nil
}

// harvestPatch generates and retrieves git format-patch output if available.
func (cr *ClientContainerRunner) harvestPatch(ctx context.Context) ([]byte, error) {
	// Generate patches.
	patchCmd := "cd /repo/src 2>/dev/null && git format-patch -o /tmp origin/HEAD~1..HEAD 2>/dev/null || true"

	req := api.InstanceExecPost{
		Command:      []string{"bash", "-c", patchCmd},
		WaitForWS:    true,
		Interactive:  false,
		RecordOutput: true,
	}

	op, err := cr.client.ExecInstance(cr.containerName, req, nil)
	if err != nil {
		return nil, nil
	}

	_ = op.Wait()

	// Try to pull patch files from /tmp.
	patchPath := "/tmp"
	resp, _, err := cr.client.GetInstanceFile(cr.containerName, patchPath)
	if err != nil || resp == nil {
		return nil, nil
	}

	// Read first matching patch file if directory listing is available.
	// For now, return empty (would need to list directory and read specific file).
	return nil, nil
}

// harvestOutputArtifacts pulls files from /output if it exists.
func (cr *ClientContainerRunner) harvestOutputArtifacts(ctx context.Context) (map[string][]byte, error) {
	artifacts := make(map[string][]byte)

	// Check if /output exists.
	_, _, err := cr.client.GetInstanceFile(cr.containerName, "/output")
	if err != nil {
		// /output doesn't exist or error reading it.
		return artifacts, nil
	}

	// List files in /output.
	filesCmd := "find /output -type f 2>/dev/null"

	req := api.InstanceExecPost{
		Command:     []string{"bash", "-c", filesCmd},
		WaitForWS:   true,
		Interactive: false,
	}

	var listOut bytes.Buffer
	args := &incus.InstanceExecArgs{
		Stdout: &listOut,
	}

	op, err := cr.client.ExecInstance(cr.containerName, req, args)
	if err != nil {
		return artifacts, nil
	}

	_ = op.Wait()

	// Parse file list.
	fileList := strings.TrimSpace(listOut.String())
	if fileList == "" {
		return artifacts, nil
	}

	// Pull each file.
	for _, file := range strings.Split(fileList, "\n") {
		if file == "" {
			continue
		}

		content, _, err := cr.client.GetInstanceFile(cr.containerName, file)
		if err != nil || content == nil {
			continue
		}

		defer content.Close()

		data, err := io.ReadAll(content)
		if err != nil {
			continue
		}

		// Store with relative path from /output.
		relPath := strings.TrimPrefix(file, "/output/")
		artifacts[relPath] = data
	}

	return artifacts, nil
}

// cleanup removes the container.
func (cr *ClientContainerRunner) cleanup() error {
	if cr.containerName == "" {
		return nil
	}

	// Delete the instance (ephemeral or not).
	op, err := cr.client.DeleteInstance(cr.containerName)
	if err != nil {
		// Instance may already be gone.
		return nil
	}

	// Wait for deletion to complete.
	_ = op.Wait()
	return nil
}

// loadRemoteConfig reads Incus remote configuration from ~/.config/incus/config.yml
func loadRemoteConfig(remoteName string) (addr, clientCert, clientKey, serverCert string, err error) {
	// For now, use simple hardcoded paths and let Incus handle the config.
	// A full implementation would parse the YAML config file.
	// For ndn-desktop, we can rely on standard Incus client paths.

	configDir := filepath.Join(os.Getenv("HOME"), ".config", "incus")
	certPath := filepath.Join(configDir, "client.crt")
	keyPath := filepath.Join(configDir, "client.key")

	// Read certificate and key files.
	clientCertData, err := readFile(certPath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("read client cert: %w", err)
	}

	clientKeyData, err := readFile(keyPath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("read client key: %w", err)
	}

	// For the remote address, we use the configured address or fallback to default.
	// This is a simplification; a full parser would read the config.yml.
	remoteAddr := "https://192.168.86.49:8443"
	if remoteName == "local" {
		remoteAddr = "unix://"
	}

	// Read the server certificate (ca.crt) if it exists.
	// For ndn-desktop with custom CA, we may need this.
	serverCertPath := filepath.Join(configDir, "servercerts")
	serverCertDir := filepath.Join(serverCertPath, "ndn-desktop.crt")
	serverCertData, _ := readFile(serverCertDir)

	return remoteAddr, string(clientCertData), string(clientKeyData), string(serverCertData), nil
}
