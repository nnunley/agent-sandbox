package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// newCmdContext creates a new exec.Cmd with the given context.
func newCmdContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}

// runExternalGrading runs the oracle (typically a test suite) on a clean checkout
// with the worker's patch applied. This ensures the worker never had write access
// to the oracle and cannot tamper with grading.
//
// Flow:
// 1. Harvest the worker's diff (git format-patch).
// 2. Clone/copy cleanCheckoutPath to a pristine workspace.
// 3. Apply the worker's patch.
// 4. Run the oracle command.
// 5. Return the results.
func runExternalGrading(ctx context.Context, workerDiff []byte, cleanCheckoutPath string, oracleCmd []string) (*GradingResult, error) {
	result := &GradingResult{}
	start := time.Now()

	if cleanCheckoutPath == "" {
		return nil, fmt.Errorf("cleanCheckoutPath is required for external grading")
	}

	// Create a temporary workspace for the oracle run.
	tempDir, err := os.MkdirTemp("/tmp", "grading-")
	if err != nil {
		return nil, fmt.Errorf("create temp grading dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Clone the clean checkout to the temp dir.
	cloneCmd := newCmdContext(ctx, "git", "clone", cleanCheckoutPath, tempDir+"/src")
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		result.Duration = time.Since(start)
		return result, fmt.Errorf("clone clean checkout: %s", out)
	}

	// If we have a worker patch, try to apply it.
	if len(workerDiff) > 0 {
		patchFile := filepath.Join(tempDir, "worker.patch")
		if err := os.WriteFile(patchFile, workerDiff, 0644); err != nil {
			result.Duration = time.Since(start)
			return result, fmt.Errorf("write patch file: %w", err)
		}

		// Apply the patch.
		applyCmd := newCmdContext(ctx, "git", "-C", tempDir+"/src", "apply", "--check", patchFile)
		if out, err := applyCmd.CombinedOutput(); err != nil {
			result.PatchApplied = false
			result.ApplyError = string(out)
			// Continue anyway; we'll run the oracle on the unmodified source.
		} else {
			// Patch applies cleanly; now apply it for real.
			applyCmd = newCmdContext(ctx, "git", "-C", tempDir+"/src", "apply", patchFile)
			if out, err := applyCmd.CombinedOutput(); err != nil {
				result.PatchApplied = false
				result.ApplyError = string(out)
			} else {
				result.PatchApplied = true
			}
		}
	}

	// Run the oracle.
	var stdout, stderr bytes.Buffer
	oracleCMD := newCmdContext(ctx, oracleCmd[0], oracleCmd[1:]...)
	oracleCMD.Dir = tempDir + "/src"
	oracleCMD.Stdout = &stdout
	oracleCMD.Stderr = &stderr

	err = oracleCMD.Run()
	result.ExitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		result.ExitCode = -1
	}

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.Duration = time.Since(start)

	return result, nil
}
