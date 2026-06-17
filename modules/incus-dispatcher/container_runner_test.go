package main

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestGenerateContainerName verifies container name generation and sanitization.
func TestGenerateContainerName(t *testing.T) {
	tests := []struct {
		taskName string
		check    func(string) bool
	}{
		{
			taskName: "my-task",
			check: func(name string) bool {
				return len(name) > 0 && name[:len(ContainerNamePrefix)] == ContainerNamePrefix
			},
		},
		{
			taskName: "my_task",
			check: func(name string) bool {
				// Underscores converted to hyphens
				return !contains(name, "_")
			},
		},
		{
			taskName: "My.Task",
			check: func(name string) bool {
				// Dots converted to hyphens, uppercased to lowercase
				return !contains(name, ".") && !contains(name, "MY")
			},
		},
	}

	for _, tt := range tests {
		name := generateContainerName(tt.taskName)
		if !tt.check(name) {
			t.Errorf("generateContainerName(%q) = %q, failed validation", tt.taskName, name)
		}
	}
}

// TestTaskValidation verifies task validation logic.
func TestTaskValidation(t *testing.T) {
	tests := []struct {
		task      Task
		shouldErr bool
	}{
		{
			task:      Task{Name: "", Repo: "/tmp/repo", Cmd: []string{"echo", "hi"}},
			shouldErr: true, // Missing name
		},
		{
			task:      Task{Name: "test", Repo: "", Cmd: []string{}},
			shouldErr: true, // No cmd and no repo
		},
		{
			task:      Task{Name: "test", Repo: "/tmp/repo", Cmd: []string{}},
			shouldErr: false, // Repo is enough
		},
		{
			task:      Task{Name: "test", Repo: "", Cmd: []string{"echo", "hi"}},
			shouldErr: false, // Cmd is enough
		},
	}

	for i, tt := range tests {
		err := tt.task.validate()
		if (err != nil) != tt.shouldErr {
			t.Errorf("test %d: validate() error = %v, shouldErr = %v", i, err, tt.shouldErr)
		}
	}
}

// TestIsLocalPath verifies local path detection.
func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/tmp/repo", true},
		{"~/repo", true},
		{"./repo", true},
		{"../repo", true},
		{"https://github.com/user/repo.git", false},
		{"git@github.com:user/repo.git", false},
		{"repo", false},
	}

	for _, tt := range tests {
		result := isLocalPath(tt.path)
		if result != tt.expected {
			t.Errorf("isLocalPath(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

// TestRemoteFileRead tests reading a file via cat command.
func TestRemoteFileRead(t *testing.T) {
	// Create a temporary file
	tmpfile, err := os.CreateTemp(t.TempDir(), "test-*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpfile.WriteString("hello world")
	tmpfile.Close()

	data, err := readFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}

	if string(data) != "hello world" {
		t.Errorf("readFile() = %q, want %q", string(data), "hello world")
	}
}

// TestContainerNameUniqueness verifies that generated names are unique across calls.
func TestContainerNameUniqueness(t *testing.T) {
	name1 := generateContainerName("task")
	time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	name2 := generateContainerName("task")

	if name1 == name2 {
		t.Errorf("generated container names are not unique: %s == %s", name1, name2)
	}
}

// IntegrationTest: TestRunTaskInContainer
// This test requires a live Incus remote (ndn-desktop) and is skipped if unavailable.
// Run with: go test -tags=integration ./...
// Or skip with: go test -short ./...
func TestRunTaskInContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if incus remote is reachable
	remote := "ndn-desktop"
	if !incusRemoteReachable(remote) {
		t.Skipf("incus remote %s not reachable; skipping integration test", remote)
	}

	runner, err := NewCLIContainerRunner(remote)
	if err != nil {
		t.Fatalf("NewContainerRunner: %v", err)
	}
	defer runner.Cleanup()

	// Simple task: echo command
	task := Task{
		Name:      "test-echo",
		Cmd:       []string{"echo", "Hello from incus"},
		ImageName: "ndn-desktop:shesha-sandbox",
		Timeout:   30 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), task.Timeout)
	defer cancel()

	result, err := runner.Run(ctx, task)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0; stdout=%s, stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
	}

	if !contains(result.Stdout, "Hello from incus") {
		t.Errorf("Stdout = %q, want to contain 'Hello from incus'", result.Stdout)
	}

	// Cleanup
	if err := runner.Cleanup(); err != nil {
		t.Errorf("Cleanup: %v", err)
	}
}

// IntegrationTest: TestDeliverSourceViaClone
// Clones a real repo (shallow) and verifies it inside the container.
// Requires incus remote to be reachable and git to be available in the image.
func TestDeliverSourceViaClone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	remote := "ndn-desktop"
	if !incusRemoteReachable(remote) {
		t.Skipf("incus remote %s not reachable; skipping integration test", remote)
	}

	// Check if git is available in the image
	if !hasToolInImage(remote, "shesha-sandbox", "git") {
		t.Skip("git not available in test image; skipping clone test")
	}

	runner, err := NewCLIContainerRunner(remote)
	if err != nil {
		t.Fatalf("NewContainerRunner: %v", err)
	}
	defer runner.Cleanup()

	task := Task{
		Name:      "test-clone",
		Repo:      "https://github.com/golang/example.git",
		Ref:       "master",
		Cmd:       []string{"ls", "-la", "/repo/src"},
		ImageName: "ndn-desktop:shesha-sandbox",
		Timeout:   60 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), task.Timeout)
	defer cancel()

	result, err := runner.Run(ctx, task)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0; stderr=%s", result.ExitCode, result.Stderr)
	}

	// Verify directory exists (ls output contains "hello" or similar)
	if result.Stdout == "" && result.Stderr == "" {
		t.Errorf("no output from ls; something went wrong")
	}
}

// IntegrationTest: TestRoundTripWithOutputArtifacts
// Creates a file in /output and verifies it's harvested.
func TestRoundTripWithOutputArtifacts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	remote := "ndn-desktop"
	if !incusRemoteReachable(remote) {
		t.Skipf("incus remote %s not reachable; skipping integration test", remote)
	}

	runner, err := NewCLIContainerRunner(remote)
	if err != nil {
		t.Fatalf("NewContainerRunner: %v", err)
	}
	defer runner.Cleanup()

	task := Task{
		Name:      "test-output",
		Cmd:       []string{"sh", "-c", "mkdir -p /output && echo 'test artifact' > /output/result.txt"},
		ImageName: "ndn-desktop:shesha-sandbox",
		Timeout:   30 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), task.Timeout)
	defer cancel()

	result, err := runner.Run(ctx, task)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0; stderr=%s", result.ExitCode, result.Stderr)
	}

	// Debug: Check if /output actually exists
	checkCmd := exec.Command("incus", "exec", result.ContainerName, "--", "ls", "-la", "/output")
	checkOut, _ := checkCmd.CombinedOutput()
	t.Logf("Container %s /output contents: %s", result.ContainerName, string(checkOut))

	if len(result.Artifacts) == 0 {
		t.Errorf("no artifacts harvested; expected result.txt. Container was %s", result.ContainerName)
	}

	if len(result.Artifacts) > 0 {
		if data, ok := result.Artifacts["result.txt"]; !ok {
			t.Errorf("expected artifact 'result.txt' not found")
		} else if !contains(string(data), "test artifact") {
			t.Errorf("artifact content = %q, want to contain 'test artifact'", string(data))
		}
	}
}

// Helper: Check if incus remote is reachable.
func incusRemoteReachable(remote string) bool {
	// Try to run 'incus list <remote>:' and see if it succeeds.
	cmd := exec.Command("incus", "list", remote+":")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// Helper: Check if a tool is available in a container image by launching a temporary instance.
func hasToolInImage(remote, imageAlias, tool string) bool {
	tempName := "test-tool-check-" + tool
	launchCmd := exec.Command("incus", "launch", remote+":"+imageAlias, tempName, "--ephemeral")
	if err := launchCmd.Run(); err != nil {
		return false
	}

	checkCmd := exec.Command("incus", "exec", tempName, "--", "which", tool)
	checkCmd.Stdout = nil
	checkCmd.Stderr = nil

	result := checkCmd.Run() == nil
	// Try to clean up
	_ = exec.Command("incus", "delete", tempName, "-f").Run()
	return result
}

// Helper: Check if string contains substring.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
