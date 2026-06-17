# incus-dispatcher

A Go CLI tool for launching ephemeral Incus containers to run isolated tasks, with support for git repository delivery and artifact harvesting.

## Features

- **Ephemeral containers**: Automatically cleaned up after task completion
- **Git delivery**: Via local bundle (for local repos) or shallow clone (for remote URLs)
- **Output harvesting**: Automatically collects files from `/output` inside the container
- **Patch generation**: Can generate `git format-patch` output if the repo has commits
- **Environment injection**: Pass environment variables via `CONTAINER_*` convention
- **Configurable timeouts**: Per-task execution limits
- **Debug mode**: Keep failing containers alive with `--keep-on-failure`

## Usage

```bash
incus-dispatcher [flags]
```

### Flags

- `--name` (required): Unique task identifier used in container naming
- `--cmd` (required): Command to run inside the container
- `--repo` (optional): Git repository path (local) or URL to deliver
  - Local path: `/path/to/repo`, `~/repo`, `./repo` (delivered via git bundle)
  - Remote URL: `https://github.com/user/repo.git` (shallow cloned)
- `--ref` (optional): Git reference to check out (default: `HEAD`)
- `--branch` (optional): Target branch to create for the work
- `--image` (optional): Incus image name (default: `images:ubuntu/24.04`)
- `--remote` (optional): Incus remote name (default: `ndn-desktop`)
- `--timeout` (optional): Task execution timeout (default: `1h`)
- `--keep-on-failure` (optional): Keep container alive if command fails (for debugging)
- `--output-dir` (optional): Directory to write results (JSON + artifacts)

### Examples

Run a simple command:
```bash
incus-dispatcher --name my-test --cmd "echo hello"
```

Run tests in a local repo:
```bash
incus-dispatcher \
  --name test-suite \
  --repo ~/myproject \
  --ref main \
  --cmd "make test" \
  --timeout 30m
```

Run tests and harvest artifacts:
```bash
incus-dispatcher \
  --name build-and-test \
  --repo https://github.com/user/project.git \
  --ref develop \
  --cmd "go test -v -coverprofile /output/coverage.txt ./..." \
  --output-dir ./results
```

Run a command with environment variables:
```bash
CONTAINER_DEBUG=1 CONTAINER_LOG_LEVEL=trace incus-dispatcher \
  --name debug-run \
  --cmd "my-script"
```

### Output Format

By default, results are printed as JSON to stdout:
```json
{
  "exitCode": 0,
  "containerName": "dispatch-test-task-123456",
  "duration": "5.234s",
  "stdout": "...",
  "stderr": "",
  "patchAvailable": false,
  "artifactCount": 2
}
```

With `--output-dir`, results are written to files:
- `result.json`: JSON summary
- `patch.diff`: Git format-patch output (if available)
- `artifacts/`: Directory containing harvested files from `/output`

## Architecture

### Runner Interface

The tool uses a `Runner` interface for pluggable execution backends:

```go
type Runner interface {
    Run(ctx context.Context, task Task) (*Result, error)
    Cleanup() error
}
```

Currently implemented:
- **ContainerRunner**: Incus ephemeral containers (CLI-based)

Future runners:
- **VMRunner**: Firecracker micro-VMs (for the agent-host environment)
- **CloudRunner**: Remote execution (e.g., cloud functions)

### Git Delivery Modes

**Local Bundle** (for local paths):
1. Create `git bundle` on host
2. Push bundle to container via `incus file push`
3. Clone inside container: `git clone <bundle> <target>`

**Shallow Clone** (for remote URLs):
1. Shallow clone directly in container: `git clone --depth 1 --branch <ref> <url>`
2. Optionally create target branch with `git checkout -b <branch>`

### Result Harvesting

After task execution:
1. **Git Patch** (if `/repo/src` exists and has commits):
   - Runs `git format-patch` to generate a patch
   - Pulls patch file from container

2. **Output Artifacts** (from `/output`):
   - Lists files in `/output` recursively
   - Pulls each file from container to local output directory
   - Preserves directory structure

## Environment Variables

### Runtime Configuration

- `CONTAINER_*`: Variables prefixed with `CONTAINER_` are passed into the container
  - Example: `CONTAINER_DEBUG=1` becomes `DEBUG=1` inside the container
  - Useful for configuration without modifying the command

### Development

No special environment variables needed for the CLI. For integration testing:
- Tests requiring a live incus remote can be run with `go test ./...`
- Tests are skipped gracefully if the remote is unreachable
- Use `-short` flag to skip integration tests

## Testing

```bash
# Unit tests (no remote required)
go test -short ./...

# All tests (requires live incus remote ndn-desktop)
go test ./...

# Specific test
go test -run TestRunTaskInContainer ./...
```

Tests use the `ndn-desktop` incus remote by default. If unavailable, integration tests are skipped automatically.

## Design Notes

### Why Incus CLI Instead of Go Client?

The current implementation uses `incus` CLI commands rather than `github.com/lxc/incus/client` Go bindings because:

1. **Simplicity**: File operations (`incus file push/pull`) are simpler via CLI
2. **Stability**: CLI is stable; Go client API changes more frequently
3. **No authentication overhead**: CLI uses local socket; Go client requires more setup
4. **Deployment flexibility**: Incus CLI is already available on the host

Future: Can migrate to Go client if performance or integration requirements change.

### Ephemeral Container Cleanup

Ephemeral containers are automatically deleted when stopped. The tool:
1. Launches with `--ephemeral` flag
2. Harvests results before cleanup
3. Explicitly deletes the container to ensure cleanup (even if already gone)

If `--keep-on-failure` is set and the command exits non-zero, the container is left running for debugging.

### Timeout Semantics

Task timeout is enforced via `context.WithTimeout()`. If a task exceeds the timeout:
1. The context is cancelled
2. Any running `incus exec` command is interrupted
3. Results so far (partial stdout/stderr) are returned
4. Container is cleaned up
5. Exit code is set to -1 to indicate timeout

## Future Work

1. **VM Runner**: Implement `Runner` for Firecracker micro-VMs to run on the agent-host
2. **Result Caching**: Cache successful runs to skip re-execution
3. **Parallel Execution**: Fan out multiple tasks across available containers
4. **Network Isolation**: Restrict container egress (via iptables rules)
5. **Credential Handling**: Inject secrets safely (API keys, OAuth tokens, etc.)
6. **Structured Logging**: JSONL logging of task execution for auditing
