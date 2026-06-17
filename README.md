# Agent Sandbox: incus-dispatcher

Ephemeral NixOS container launcher for task execution with shared read-only `/nix/store`.

## Quick Start

### Build

```bash
cd modules/incus-dispatcher
go build -o dispatcher .
```

### Run a Test Task

```bash
./dispatcher \
  --name my-task \
  --repo https://github.com/example/repo \
  --ref main \
  --cmd "go test ./..." \
  --root \
  --runner client
```

**Output** (JSON to stdout):
```json
{
  "exitCode": 0,
  "containerName": "dispatch-my-task-1234567",
  "duration": "45s",
  "stdout": "ok\texample/repo\t1.234s",
  "stderr": "",
  "patchAvailable": false,
  "artifactCount": 0
}
```

## Architecture

### Shared `/nix/store` Volume

One Incus filesystem volume named `nix-shared` is shared read-only across all workers:

```
+-------------------+
|  agent-host       |
|  (NixOS mgmt)     |
|  nix develop      |
|  nix copy ---+    |
+------|-------+    |
       |
       v
+-------------------+
|  nix-shared       |
|  (volume)         |
|  /nix/store       |
+----------|--------+
           |
    +------+------+-------+
    |      |      |       |
    v      v      v       v
[W1]   [W2]   [W3]   [W4] ...
(all mount at /nix/store, readonly=true)
```

- **Creation**: `incus storage volume create default nix-shared -t filesystem`
- **Population**: Build devShell closure on `agent-host`, realise to volume via `nix copy`
- **Worker Access**: Mounted at `/nix/store` with `readonly=true`; workers resolve tools via `nix shell`/`nix develop` (no builds)

### Worker Toolchain

Defined in `flake.nix`:

```nix
devShells.${system}.default = linuxPkgs.mkShell {
  name = "dispatcher-dev";
  description = "Dispatcher development shell with git, go, gnumake";
  buildInputs = with linuxPkgs; [
    git
    go
    gnumake
    pkg-config
    bash
  ];
};
```

This closure is built once and realised to `nix-shared`. All workers use these prebuilt tools.

### Runners

#### CLI Runner (`--runner cli`)

Uses `incus` CLI commands:
- Pro: Simple, direct, works offline
- Con: Slower for many operations (process overhead)

```go
runner, _ := NewCLIContainerRunner("ndn-desktop")
result, _ := runner.Run(ctx, task)
```

#### Go Client Runner (`--runner client`, default)

Uses `github.com/lxc/incus/v6/client`:
- Pro: Programmatic, efficient, supports streaming
- Con: Requires client certs configured

```go
runner, _ := NewClientContainerRunner("ndn-desktop")
result, _ := runner.Run(ctx, task)
```

## Key Flags

| Flag | Type | Default | Meaning |
|------|------|---------|---------|
| `--name` | string | (required) | Task identifier (used in container name) |
| `--cmd` | string | (required) | Command to run inside container |
| `--repo` | string | - | Git repository path (local) or URL; if empty, skips delivery |
| `--ref` | string | `HEAD` | Git ref to check out |
| `--branch` | string | - | Target branch to create (optional) |
| `--image` | string | `images:nixos/25.11` | Incus image alias; `nixos` or `ubuntu` are special |
| `--root` | bool | `false` | Launch with `security.privileged=true` (allows root ops, deps install) |
| `--remote` | string | `ndn-desktop` | Incus remote name |
| `--runner` | string | `client` | Runner backend: `client` or `cli` |
| `--timeout` | duration | `1h` | Max task duration (e.g., `30m`, `1h30m`) |
| `--keep-on-failure` | bool | `false` | Keep container alive if command fails (for debugging) |
| `--external-grading` | string | - | Path to clean checkout for oracle verification |
| `--output-dir` | string | - | Directory to write results (JSON + patch + artifacts); if empty, outputs JSON to stdout |
| `--provider` | string | `anthropic` | LLM provider: `anthropic`, `openai`, `ollama-cloud` |
| `--model` | string | - | Model name (e.g., `claude-3-5-haiku`, `gpt-4o-mini`) |

## Environment Variables

Environment variables prefixed with `CONTAINER_` are passed into the container, with the prefix stripped:

```bash
CONTAINER_DEBUG=1 CONTAINER_LOG_LEVEL=trace ./dispatcher \
  --name test \
  --cmd "go test -v"
  # Inside container: DEBUG=1, LOG_LEVEL=trace
```

## External Grading (Oracle Verification)

When `--external-grading /path/to/checkout` is specified:

1. **Worker runs**: Produces changes, harvests `git format-patch` output
2. **Oracle setup**: Pristine clone of `/path/to/checkout` is created
3. **Patch apply**: Worker's diff applied to the clone (checks success)
4. **Oracle exec**: User's command (e.g., test suite) runs on patched code
5. **Results**: Include exit code, stdout, stderr, and patch applicability

**Purpose**: Ensure the worker never had write access to the oracle; oracle runs in isolation.

**Example**:
```bash
./dispatcher \
  --name student-submission \
  --repo /tmp/student-work \
  --ref HEAD \
  --cmd "make test" \
  --external-grading ~/clean-oracle-checkout \
  --output-dir /tmp/grading-results
```

**Output** (in `/tmp/grading-results/result.json`):
```json
{
  "exitCode": 0,
  "containerName": "dispatch-student-submission-7890",
  "duration": "30s",
  "stdout": "✓ all tests passed",
  "stderr": "",
  "grading": {
    "exitCode": 0,
    "duration": "25s",
    "stdout": "✓ oracle passed",
    "stderr": "",
    "patchApplied": true,
    "applyError": null
  }
}
```

## Code Structure

```
modules/incus-dispatcher/
├── main.go              # CLI entry point, flag parsing, task dispatch
├── types.go             # Task, Result, GradingResult, Runner interface
├── cli_runner.go        # Incus CLI runner (incus launch/exec/file)
├── client_runner.go     # Incus Go client runner (lxc/incus/v6)
├── helpers.go           # newCmdContext, runExternalGrading, utilities
├── dispatcher           # Compiled binary (git-ignored)
└── incus-dispatcher     # Symlink to dispatcher

flake.nix               # Nix flake (nixosConfigurations.agent-host, devShells.default)
```

## Implementation Details

### Container Lifecycle

1. **Launch**: `incus launch images:nixos/25.11 dispatch-<name>-<rand> --ephemeral`
   - If `--root`: adds `--config security.privileged=true`
2. **Attach nix store**: `incus config device add ... nix-shared disk ... path=/nix/store readonly=true`
   - Only if task has `SharedNixStore=true` (auto-enabled for NixOS images)
3. **Wait ready**: Polls `incus exec ... echo ok` until container responds (500ms ticks, 30s timeout)
4. **Deliver source** (if repo specified):
   - Local path: `git bundle` on host → push to container → `git clone` from bundle
   - Remote URL: `git clone --depth 1 --branch <ref>` directly in container
5. **Run command**: `incus exec ... -- <cmd>` with environment variables, capture stdout/stderr
6. **Harvest results**:
   - `git format-patch -o /tmp`: collects any commits as a patch
   - `/output` directory: pulls files to `result.Artifacts`
7. **Cleanup**: `incus delete -f` (ephemeral, auto-cleans; explicit delete ensures cleanup)

### PATH Resolution

Both runners prepend `/run/current-system/sw/bin` to the container's `PATH`:
- If no `PATH` env provided: `PATH=/run/current-system/sw/bin:/usr/bin:/bin`
- If `PATH` exists: prepended with `/run/current-system/sw/bin:`

This ensures NixOS binaries from the shared `/nix/store` are found.

### Result Harvesting

- **Patch**: `git format-patch -o /tmp origin/HEAD~1..HEAD` (or similar range)
- **Artifacts**: Any files in `/output` directory are recursively pulled
- **Grading**: If `ExternalGradingCheckout` specified, oracle runs in a clean clone with worker's patch applied

## Build & Test

```bash
cd modules/incus-dispatcher

# Build binary
go build -o dispatcher .

# Run tests (if present)
go test ./...

# Lint
go vet ./...
```

## Conventions

1. **Declarative toolchain**: All tools come from `flake.nix` devShell, never imperative (`apt`, `nix profile`)
2. **Shared read-only store**: One `/nix/store` volume, mounted read-only on all workers
3. **Root/privileged workers**: NixOS containers run as root by default; `--root` is a no-op for security.privileged
4. **Ephemeral cleanup**: Containers are ephemeral; deleted on exit unless `--keep-on-failure` and task fails
5. **Exit code passthrough**: Dispatcher exits with the task's command exit code (or -1 on framework error)

## Troubleshooting

### Container fails to launch
```
error: instance "dispatch-..." not found
```
- Check remote is reachable: `incus list ndn-desktop:`
- Check image is available: `incus image list ndn-desktop: | grep nixos`

### Binary not found (exit 127)
```
/repo/src: line 1: go: not found
```
- Ensure `/run/current-system/sw/bin` is in `PATH`
- Runners prepend this by default; check you're not overriding `PATH` without including it

### Nix store volume not attached
```
error: attach nix volume failed: ...
```
- Check volume exists: `incus storage volume list default | grep nix-shared`
- Check volume is populated (not empty): `incus storage volume info default nix-shared`
- Verify `readonly=true` in device config

### Patch won't apply in external grading
```
"patchApplied": false,
"applyError": "error: patch does not apply"
```
- Worker's diff is based on a different code state than the oracle checkout
- This is expected if the oracle is on a different branch or post-worker commit
- Check `applyError` field for details

## Future Work

- Per-worker `/nix/var` snapshots for incremental builds
- Ollama router integration (multi-model oracle grading)
- Structured artifact export (TAR, ZIP)
- Metrics: CPU, memory, build time, storage usage
- Dry-run mode: show what would be executed without launching container
