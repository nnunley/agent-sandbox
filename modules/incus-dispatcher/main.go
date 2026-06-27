// incus-dispatcher: CLI tool for launching ephemeral Incus containers to run tasks.
//
// Usage:
//
//	incus-dispatcher --name <task-name> --cmd "go test ./..." [--repo <path|url>] [--ref main] [--image <image>] [--timeout 1h] [--keep-on-failure]
//
// Example:
//
//	incus-dispatcher --name my-test --repo ~/myrepo --ref main --cmd "make test" --timeout 30m
//
// The dispatcher:
// 1. Launches an ephemeral Incus container from the specified image.
// 2. Delivers source via git bundle (local path) or shallow clone (URL).
// 3. Runs the command inside the container.
// 4. Harvests git format-patch and /output artifacts.
// 5. Removes the container (unless --keep-on-failure and command fails).
//
// Output format: JSON on stdout containing exit code, stdout, stderr, patch, and artifacts.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	// Subcommands are dispatched on os.Args[1] before the flag-based main path.
	// `grade` runs the authoritative external grader (STORY-0068) and prints the
	// structured grade JSON to stdout.
	if len(os.Args) > 1 && os.Args[1] == "grade" {
		os.Exit(runGradeCommand(os.Args[2:]))
	}

	// `serve` runs the coordinator as a long-running daemon (STORY-0007 AC-2): one
	// persistent process drains the directive queue via the D4 loop until signaled.
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		os.Exit(runServeCommand(os.Args[2:]))
	}

	// `tui` runs the operator TUI (STORY-0028, STORY-0027 AC-3): a command-driven
	// terminal interface for thread and worker management, including pause/block/resume.
	if len(os.Args) > 1 && os.Args[1] == "tui" {
		os.Exit(runTUICommand(os.Args[2:]))
	}

	// `usage` prints the cross-provider usage meter readout (and `usage ingest
	// <transcript>` records interactive Claude Code usage into the ledger).
	if len(os.Args) > 1 && os.Args[1] == "usage" {
		os.Exit(runUsageCommand(os.Args[2:]))
	}

	// CLI flags
	name := flag.String("name", "", "Task name (required)")
	repo := flag.String("repo", "", "Git repository path (local) or URL to deliver (optional)")
	ref := flag.String("ref", "HEAD", "Git ref to check out")
	targetBranch := flag.String("branch", "", "Target branch to create (optional)")
	cmd := flag.String("cmd", "", "Command to run inside container (required)")
	image := flag.String("image", DefaultImageName, "Incus image name (default: NixOS 25.11; NixOS-only)")
	timeout := flag.Duration("timeout", DefaultTimeout, "Task timeout")
	keepOnFailure := flag.Bool("keep-on-failure", false, "Keep container alive on task failure")
	remote := flag.String("remote", DefaultRemote, "Incus remote name")
	outputDir := flag.String("output-dir", "", "Directory to write results (optional; if set, writes JSON and artifacts)")
	runner := flag.String("runner", "client", "Runner implementation: 'client' (Go client) or 'cli' (CLI commands)")
	provider := flag.String("provider", "anthropic", "LLM provider: anthropic, openai, ollama-cloud")
	model := flag.String("model", "", "Model name (e.g., claude-3-5-haiku, gpt-4o-mini)")
	runAsRoot := flag.Bool("root", false, "Launch container with root access (allows installing dependencies)")
	binaryCachePath := flag.String("binary-cache-path", "/srv/nix-shared", "Path on shared nix volume where prebuilt packages are cached")
	externalGrading := flag.String("external-grading", "", "Path to clean checkout for external grading (oracle verification)")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `incus-dispatcher: launch ephemeral Incus containers to run tasks

Usage: incus-dispatcher [flags]

Flags:
`)
		flag.PrintDefaults()
	}

	flag.Parse()

	// Validate required flags
	if *name == "" {
		log.Fatal("--name is required")
	}
	if *cmd == "" {
		log.Fatal("--cmd is required")
	}

	// Create runner based on --runner flag
	var taskRunner Runner
	var err error

	switch *runner {
	case "client":
		taskRunner, err = NewClientContainerRunner(*remote)
		if err != nil {
			log.Fatalf("failed to create Go client runner: %v", err)
		}
	case "cli":
		taskRunner, err = NewCLIContainerRunner(*remote)
		if err != nil {
			log.Fatalf("failed to create CLI runner: %v", err)
		}
	default:
		log.Fatalf("invalid runner: %s (must be 'client' or 'cli')", *runner)
	}

	// Parse command
	cmdParts := strings.Fields(*cmd)
	if len(cmdParts) == 0 {
		log.Fatal("--cmd is empty after parsing")
	}

	// Use provided image name (NixOS-only)
	imageName := *image

	// Validate provider
	prov := Provider(*provider)
	if err := prov.ValidateProvider(); err != nil {
		log.Fatalf("invalid provider: %v", err)
	}

	// Build task
	cleanEnv, strippedKeys := SanitizeWorkerEnv(parseEnv())
	if len(strippedKeys) > 0 {
		log.Printf("stripped raw provider credentials from worker env: %v", strippedKeys)
	}
	task := Task{
		Name:                    *name,
		Repo:                    *repo,
		Ref:                     *ref,
		TargetBranch:            *targetBranch,
		Cmd:                     cmdParts,
		ImageName:               imageName,
		Timeout:                 *timeout,
		KeepOnFailure:           *keepOnFailure,
		Env:                     cleanEnv,
		Provider:                prov,
		Model:                   *model,
		RunAsRoot:               *runAsRoot,
		BinaryCachePath:         *binaryCachePath,
		ExternalGradingCheckout: *externalGrading,
	}

	// Route the implementer to the chosen provider/model (STORY-0076 AC-1): forward
	// --provider/--model into the worker env (FLEET_PROVIDER/FLEET_MODEL). The grader stays
	// deterministic (git-based, no model).
	if err := applyProviderRouting(&task); err != nil {
		log.Fatalf("provider routing: %v", err)
	}

	// Run task
	ctx := context.Background()
	result, err := taskRunner.Run(ctx, task)

	// Always cleanup unless keep-on-failure and task failed
	if !(*keepOnFailure && result.ExitCode != 0) {
		_ = taskRunner.Cleanup()
	}

	// Handle errors (non-command errors)
	if err != nil && !isCommandErr(err) {
		log.Fatalf("task failed: %v", err)
	}

	// Output results
	if *outputDir != "" {
		if err := writeResults(*outputDir, result); err != nil {
			log.Fatalf("failed to write results: %v", err)
		}
	} else {
		outputJSON(result)
	}

	// Exit with task's exit code
	os.Exit(result.ExitCode)
}

// outputJSON writes the result as JSON to stdout.
func outputJSON(result *Result) {
	data := map[string]interface{}{
		"exitCode":       result.ExitCode,
		"containerName":  result.ContainerName,
		"duration":       result.Duration.String(),
		"stdout":         result.Stdout,
		"stderr":         result.Stderr,
		"patchAvailable": len(result.PatchData) > 0,
		"artifactCount":  len(result.Artifacts),
	}

	if result.ExternalGradingResult != nil {
		data["grading"] = map[string]interface{}{
			"exitCode":     result.ExternalGradingResult.ExitCode,
			"duration":     result.ExternalGradingResult.Duration.String(),
			"stdout":       result.ExternalGradingResult.Stdout,
			"stderr":       result.ExternalGradingResult.Stderr,
			"patchApplied": result.ExternalGradingResult.PatchApplied,
			"applyError":   result.ExternalGradingResult.ApplyError,
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		log.Fatalf("failed to encode result: %v", err)
	}
}

// writeResults writes the full result (including patch and artifacts) to outputDir.
// Creates outputDir/result.json, outputDir/patch if available, outputDir/artifacts/*.
func writeResults(outputDir string, result *Result) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	// Write result JSON
	resultFile := fmt.Sprintf("%s/result.json", outputDir)
	data := map[string]interface{}{
		"exitCode":      result.ExitCode,
		"containerName": result.ContainerName,
		"duration":      result.Duration.String(),
		"stdout":        result.Stdout,
		"stderr":        result.Stderr,
		"artifactCount": len(result.Artifacts),
	}
	f, err := os.Create(resultFile)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		f.Close()
		return err
	}
	f.Close()
	log.Printf("wrote %s", resultFile)

	// Write patch if available
	if len(result.PatchData) > 0 {
		patchFile := fmt.Sprintf("%s/patch.diff", outputDir)
		if err := os.WriteFile(patchFile, result.PatchData, 0644); err != nil {
			return err
		}
		log.Printf("wrote %s", patchFile)
	}

	// Write artifacts
	if len(result.Artifacts) > 0 {
		artifactDir := fmt.Sprintf("%s/artifacts", outputDir)
		if err := os.MkdirAll(artifactDir, 0755); err != nil {
			return err
		}
		for name, data := range result.Artifacts {
			file := fmt.Sprintf("%s/%s", artifactDir, name)
			// Create subdirs as needed
			if err := os.MkdirAll(fmt.Sprintf("%s/%s", artifactDir, name), 0755); err == nil {
				// Dir created; skip file write
				continue
			}
			if err := os.WriteFile(file, data, 0644); err != nil {
				log.Printf("warning: failed to write artifact %s: %v", name, err)
			}
		}
		log.Printf("wrote artifacts to %s", artifactDir)
	}

	return nil
}

// parseEnv reads environment variables that are intended to be passed into the container.
// Convention: any var starting with CONTAINER_ is passed through (minus the prefix).
// Example: CONTAINER_DEBUG=1 becomes DEBUG=1 inside the container.
func parseEnv() map[string]string {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "CONTAINER_") {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimPrefix(parts[0], "CONTAINER_")
				env[key] = parts[1]
			}
		}
	}
	return env
}

func isCommandErr(err error) bool {
	// Check if error is a command execution error (not a framework error).
	return strings.Contains(err.Error(), "exec command")
}
