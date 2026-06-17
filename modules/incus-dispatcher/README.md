# incus-dispatcher

A Go CLI tool for launching ephemeral Incus containers to run isolated tasks, with support for git repository delivery and artifact harvesting.

## Features

- **Ephemeral containers**: Automatically cleaned up after task completion
- **NixOS images**: Default to reproducible NixOS (clean, auditable closure) with optional root access
- **Git delivery**: Via local bundle (for local repos) or shallow clone (for remote URLs)
- **Output harvesting**: Automatically collects files from `/output` inside the container
- **Patch generation**: Can generate `git format-patch` output if the repo has commits
- **Environment injection**: Pass environment variables via `CONTAINER_*` convention
- **Provider routing**: Dispatch to Anthropic (Haiku), OpenAI, or Ollama Cloud via llm-proxy
- **External grading**: Verify diffs on a pristine checkout the worker never touched
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
- `--image` (optional): Incus image name (default: `images:ubuntu/24.04`); use `nixos` for NixOS
- `--root` (optional): Launch container with root/privileged access (for dependency installation)
- `--provider` (optional): LLM provider: `anthropic`, `openai`, `ollama-cloud` (default: `anthropic`)
- `--model` (optional): Model name (e.g., `claude-3-5-haiku`, `gpt-4o-mini`)
- `--external-grading` (optional): Path to clean checkout for oracle verification (anti-reward-hack)
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

Run with NixOS and root access:
```bash
incus-dispatcher \
  --name build-with-nix \
  --repo ~/myproject \
  --image nixos \
  --root \
  --cmd "nix flake check"
```

Dispatch to OpenAI via the proxy:
```bash
incus-dispatcher \
  --name gpt-test \
  --repo ~/myproject \
  --provider openai \
  --model gpt-4o-mini \
  --cmd "python test.py"
```

Verify a diff on a clean checkout (external grading):
```bash
incus-dispatcher \
  --name agent-edit \
  --repo ~/mutable-repo \
  --cmd "make edit && git format-patch -o /output" \
  --external-grading /clean-checkouts/immutable-repo \
  --output-dir ./results
```

The results will include:
```json
{
  "exitCode": 0,
  "grading": {
    "exitCode": 0,
    "patchApplied": true,
    "stdout": "PASS: all tests"
  }
}
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

## New Features (2026-06-17)

### A. NixOS Images & Root Access

Use NixOS for reproducible, auditable dependency closure:

```bash
incus-dispatcher --name task --image nixos --root --cmd "..."
```

- `--image nixos`: Uses `images:nixos/25.05` (default NixOS image)
- `--root`: Enables root/privileged access (allows `nix develop`, package installation)
- Guest includes git, build tools, and language runtimes pre-installed

### B. Provider Routing (Anthropic, OpenAI, Ollama Cloud)

Route tasks to different LLM providers via the internal proxy:

```bash
# Anthropic (default)
incus-dispatcher --name task --provider anthropic --model claude-3-5-haiku --cmd "..."

# OpenAI
incus-dispatcher --name task --provider openai --model gpt-4o-mini --cmd "..."

# Ollama Cloud
incus-dispatcher --name task --provider ollama-cloud --model neural-chat --cmd "..."
```

The proxy (`llm-proxy`) injects credentials from systemd credentials into Authorization headers.
Workers never see raw API keys; they only access the proxy at `http://10.88.0.1:12071/<provider>/*`.

### C. External Grading (Anti-Reward-Hack Guardrails)

Verify diffs on a pristine checkout the worker never touched:

```bash
# Create a clean, read-only checkout
git clone <upstream> /clean-checkouts/myrepo

# Dispatch with external grading
incus-dispatcher --name agent --repo ~/mutable --external-grading /clean-checkouts/myrepo \
  --cmd "make edit && git format-patch -o /output" \
  --output-dir ./results
```

Flow:
1. Worker produces a diff (git format-patch)
2. Dispatcher clones the clean checkout to a temp directory
3. Dispatcher applies the worker's diff to the temp clone
4. Dispatcher runs the oracle (same command) on the patched clone
5. Results include oracle exit code, stdout, stderr, and whether patch applied

The worker cannot tamper with the oracle because the oracle runs on a checkout the worker never accessed.

Result JSON includes:
```json
{
  "exitCode": <worker exit code>,
  "grading": {
    "exitCode": <oracle exit code>,
    "duration": "...",
    "stdout": "<oracle output>",
    "stderr": "",
    "patchApplied": true,
    "applyError": ""
  }
}
```

## Future Work

1. **VM Runner**: Implement `Runner` for Firecracker micro-VMs to run on the agent-host
2. **Result Caching**: Cache successful runs to skip re-execution
3. **Parallel Execution**: Fan out multiple tasks across available containers
4. **Network Isolation**: Restrict container egress (via iptables rules)
5. **Scoped Checkout Delivery**: Only deliver in-scope files to the worker (hardening)
6. **Oracle Command Separation**: Allow `--oracle-cmd` distinct from worker `--cmd`
7. **Structured Logging**: JSONL logging of task execution for auditing
