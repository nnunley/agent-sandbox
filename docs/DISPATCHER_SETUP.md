# incus-dispatcher: Complete Setup & Operations Guide

**Status**: Converged design, implementation complete (June 17, 2026)

## Architecture Overview

The dispatcher launches ephemeral NixOS containers to run tasks in isolation. All workers share a single `/nix` filesystem volume and connect to one shared `nix-daemon` for deterministic, efficient dependency resolution.

```
┌────────────────────┐
│   nix-server       │
│  (privileged)      │
│  ┌──────────────┐  │
│  │ nix-daemon   │  │
│  │ (single)     │  │
│  └──────────────┘  │
│  Socket: /nix/var/ │
│   nix/daemon-      │
│   socket/socket    │
└────────────────────┘
         │
         │ (shared /nix volume, read-write)
         │
    ┌────┴─────┬─────────┬────────┐
    │           │         │        │
    v           v         v        v
┌─────────┐ ┌──────┐ ┌──────┐ ┌──────┐
│ Worker1 │ │Work2 │ │Work3 │ │Work4 │
│ (ephm)  │ │(ephm)│ │(ephm)│ │(ephm)│
│ NIX_    │ │NIX_  │ │NIX_  │ │NIX_  │
│ REMOTE  │ │REMOTE│ │REMOTE│ │REMOTE│
│=daemon  │ │daemon│ │daemon│ │daemon│
└─────────┘ └──────┘ └──────┘ └──────┘
```

## One-Time Setup

### Step 1: Verify Incus Remote

```bash
incus list ndn-desktop: | head -3
# Should show containers with working connection
```

### Step 2: Create Shared nix Volume

```bash
incus storage volume create ndn-desktop:default nix-shared -t filesystem
incus storage volume info ndn-desktop:default nix-shared
# NAME: nix-shared
# TYPE: custom
# CONTENT type: filesystem
# ...
```

### Step 3: Create Persistent nix-server Container

```bash
# Launch privileged NixOS container to host the daemon
incus launch ndn-desktop:images:nixos/25.11 nix-server \
  --config security.privileged=true \
  --device nix-shared,type=disk,pool=default,source=nix-shared,path=/nix

# Verify container started
incus list ndn-desktop: | grep nix-server
# | nix-server  | RUNNING | ... | CONTAINER | ...
```

### Step 4: Verify nix-daemon Socket

```bash
# The NixOS daemon should start automatically
incus exec ndn-desktop:nix-server -- \
  test -S /nix/var/nix/daemon-socket/socket && echo "Socket OK"
# Output: Socket OK

# Alternatively, check daemon status
incus exec ndn-desktop:nix-server -- \
  systemctl status nix-daemon
# ● nix-daemon.service - Nix Daemon
#   Loaded: loaded (/etc/systemd/system/nix-daemon.service; enabled; preset: enabled)
#   Active: active (running) since ...
```

### Step 5: Populate Store with DevShell Closure

```bash
# On the incus host or locally (if nix is installed):
cd /Users/ndn/development/agent-sandbox

# Build the devShell closure
nix build .#devShells.x86_64-linux.default --out-link /tmp/devshell

# Copy it into the shared store (via nix-server)
# Method A: Via shell (if nix is available locally)
nix copy /tmp/devshell \
  --to "ssh-ng://ndn-desktop?remote-program=nix-daemon" \
  --all

# Method B: Manual (build inside nix-server, then copy paths)
incus exec ndn-desktop:nix-server -- \
  nix build --impure \
    --expr "import ./flake.nix" \
    devShells.x86_64-linux.default
```

### Step 6: Create nix-server Snapshot (Optional but Recommended)

```bash
# Checkpoint the pristine state
incus snapshot create ndn-desktop:nix-server clean

# Later, restore if needed
incus restore ndn-desktop:nix-server clean
```

## Daily Operations

### Running a Task

```bash
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher

# Build the dispatcher (one-time)
go build -o dispatcher .

# Run a simple test
./dispatcher \
  --name my-test \
  --cmd "echo 'Hello from worker'" \
  --root \
  --timeout 30s

# Output (JSON):
# {
#   "exitCode": 0,
#   "containerName": "dispatch-my-test-1234567",
#   "duration": "5.234s",
#   "stdout": "Hello from worker\n",
#   "stderr": "",
#   "patchAvailable": false,
#   "artifactCount": 0
# }
```

### Running with Git Repo

```bash
./dispatcher \
  --name test-example \
  --repo https://github.com/example/repo \
  --ref main \
  --cmd "go test ./..." \
  --timeout 5m \
  --root

# Alternatively, local path:
./dispatcher \
  --name test-local \
  --repo /Users/ndn/development/my-project \
  --ref HEAD \
  --cmd "make test" \
  --timeout 5m
```

### External Grading (Oracle Verification)

```bash
# Test a worker's changes against a clean oracle
./dispatcher \
  --name student-work \
  --repo /tmp/student-submission \
  --ref HEAD \
  --cmd "make build" \
  --external-grading /clean/oracle/checkout \
  --output-dir /tmp/results \
  --timeout 10m

# Results include:
# - Worker stdout/stderr
# - Patch applicability (did the diff apply cleanly?)
# - Oracle exit code (did tests pass on the patched version?)
```

### Passing Environment Variables

```bash
# Convention: CONTAINER_* env vars are passed through (minus prefix)
CONTAINER_DEBUG=1 CONTAINER_LOG_LEVEL=trace ./dispatcher \
  --name test-with-env \
  --cmd "echo $DEBUG $LOG_LEVEL"

# Inside container: DEBUG=1, LOG_LEVEL=trace
```

## Debugging

### Container Timeouts

If a worker container times out (default 1h):

```bash
# Use --keep-on-failure to keep the container alive
./dispatcher \
  --name slow-task \
  --cmd "sleep 2h" \
  --timeout 30m \
  --keep-on-failure

# The container will remain on ndn-desktop for inspection
incus list ndn-desktop: | grep dispatch-slow-task

# SSH in or exec to debug
incus exec ndn-desktop:dispatch-slow-task-12345 -- bash

# When done, clean up manually
incus delete -f ndn-desktop:dispatch-slow-task-12345
```

### Worker Cannot Find Tools (exit 127)

**Symptom**: Command not found (go, git, make, etc.)

**Cause**: NIX_REMOTE not set, or shared /nix volume not mounted, or daemon not running

**Diagnosis**:
```bash
# Check PATH inside worker
./dispatcher --name check-path --cmd "echo \$PATH"

# Check /nix is mounted
./dispatcher --name check-nix --cmd "ls -la /nix" --root

# Check nix-daemon socket
./dispatcher --name check-socket --cmd "test -S /nix/var/nix/daemon-socket/socket && echo OK || echo FAIL" --root
```

**Fix**:
1. Ensure nix-server is running: `incus list ndn-desktop: | grep nix-server`
2. Verify socket exists: `incus exec ndn-desktop:nix-server -- test -S /nix/var/nix/daemon-socket/socket`
3. Verify volume is mounted: Check `/nix/var/nix/daemon-socket` exists
4. If needed, restart nix-daemon: `incus exec ndn-desktop:nix-server -- systemctl restart nix-daemon`

### Shared Store Not Populated

**Symptom**: `error: cannot find required packages`

**Cause**: DevShell closure not built/copied into shared /nix

**Fix**:
```bash
# Rebuild and copy devShell
cd /Users/ndn/development/agent-sandbox
nix flake update
nix build .#devShells.x86_64-linux.default --out-link /tmp/devshell-new

# Copy to shared store (adjust method based on your nix setup)
# Use the Method A or B from Step 5 of one-time setup
```

## Runner Options

### CLI Runner (`--runner cli`)
- Uses `incus` CLI commands
- Simpler, no client setup required
- Slower (process overhead per operation)

```bash
./dispatcher --name test --cmd "go version" --runner cli
```

### Go Client Runner (`--runner client`, default)
- Uses `github.com/lxc/incus/v6/client`
- More efficient, programmatic
- Requires client certs configured at `~/.config/incus/client.{crt,key}`

```bash
./dispatcher --name test --cmd "go version" --runner client
```

## Build & Tests

```bash
cd modules/incus-dispatcher

# Build
go build -o dispatcher .

# Test (if tests exist)
go test ./...

# Lint
go vet ./...
```

## Key Conventions

1. **Declarative toolchain**: All tools come from `flake.nix` devShell, never from apt/apk/`nix profile`
2. **Shared read-only store**: Workers mount `/nix` read-only (enforced at volume level)
3. **Single daemon**: One nix-daemon in nix-server, all workers are clients
4. **Ephemeral workers**: Containers are cleaned up automatically (unless --keep-on-failure)
5. **Immutable sessions**: Workers cannot write to store; builds are idempotent

## Troubleshooting Checklist

- [ ] Incus remote accessible: `incus list ndn-desktop:`
- [ ] Volume exists: `incus storage volume list ndn-desktop:default | grep nix-shared`
- [ ] nix-server running: `incus list ndn-desktop: | grep nix-server`
- [ ] Daemon socket ready: `incus exec ndn-desktop:nix-server -- test -S /nix/var/nix/daemon-socket/socket`
- [ ] DevShell closure in store: `incus exec ndn-desktop:nix-server -- nix store ls /nix/store | grep git`
- [ ] Dispatcher builds: `cd modules/incus-dispatcher && go build -o dispatcher .`
- [ ] Simple test passes: `./dispatcher --name smoke-test --cmd "echo OK" --root`

## Future Enhancements

- Per-worker `/nix/var` snapshots for incremental builds
- Ollama router integration for multi-model oracle grading
- Structured artifact export (TAR/ZIP)
- Metrics collection (CPU, memory, storage)
- Dry-run mode for execution planning
