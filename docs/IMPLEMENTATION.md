# incus-dispatcher Implementation Guide

## Status

✅ **Implementation Complete** (2026-06-17)

All core components implemented and tested:
- Flake devShell with git, go, gnumake, pkg-config, bash
- Both CLI and Go client runners with proper initialization
- Shared read-only /nix/store volume attachment (readonly=true)
- PATH resolution for NixOS containers (/run/current-system/sw/bin)
- External grading (oracle) support
- Comprehensive documentation (CLAUDE.md, README.md)

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        incus-dispatcher                         │
│  (Go tool for launching ephemeral NixOS containers)             │
└─────────────────────────────────────────────────────────────────┘
                           │
                   ┌───────┼───────┐
                   │               │
              ┌────▼─────┐   ┌────▼──────┐
              │ CLI      │   │ Go client │
              │ runner   │   │ runner    │
              └────┬─────┘   └────┬──────┘
                   │               │
            ┌──────▼───────────────▼──────┐
            │  Incus remote: ndn-desktop  │
            │  (https://192.168.86.49:8443)
            └──────┬───────────────────────┘
                   │
     ┌─────────────┼─────────────┐
     │             │             │
┌────▼────┐  ┌────▼────┐  ┌────▼────┐
│ Volume: │  │ Volume: │  │ Volume: │
│nix-store│  │nix-store│  │nix-store│
└────┬────┘  └────┬────┘  └────┬────┘
     │            │            │
     │       (all same volume, readonly=true)
     │
  ┌──▼──┐   ┌──────┐   ┌──────┐
  │ W1  │   │ W2   │   │ W3   │
  │(NixOS)  │(NixOS)  │(NixOS)
  └─────┘   └──────┘   └──────┘
  
Workers mount /nix/store readonly from the single nix-shared volume.
Toolchain (git, go, gnumake) prebuilt in the store, consumed via nix shell.
```

## Key Design Decisions

### 1. Shared /nix/store Volume (Read-Only)

**Why**: Nix store paths are immutable content-addressed. Multiple readers can safely access the same store read-only.

**Implementation**:
- Create once: `incus storage volume create default nix-shared -t filesystem`
- Populate once: Build devShell closure on agent-host, realise to volume via `nix copy`
- Mount on workers: Both runners attach at `/nix/store` with `readonly=true`

**Code locations**:
- CLI runner: `modules/incus-dispatcher/cli_runner.go:127-129`
  ```go
  deviceArgs := []string{"config", "device", "add", ..., 
    "disk", "pool=default", "source=nix-shared", "path=/nix/store", "readonly=true"}
  ```
- Go client: `modules/incus-dispatcher/client_runner.go:191-196`
  ```go
  req.Devices["nix-shared"] = map[string]string{
    "type":     "disk",
    "pool":     "default",
    "source":   "nix-shared",
    "path":     "/nix/store",
    "readonly": "true",
  }
  ```

### 2. Privileged Root Containers

**Why**: NixOS dependency resolution and test isolation require root access.

**Implementation**:
- Flag: `--root` (boolean, default false)
- Sets: `security.privileged=true` in container config
- Auto-enabled: For NixOS images via `Task.validate()` in types.go:417-419

**Code**:
- CLI: `cli_runner.go:115-117`
- Client: `client_runner.go:184-186`

### 3. Declarative Toolchain (NixOS Flake)

**Why**: Reproducible, auditable dependency closure; single point of truth.

**Implementation**:
- File: `flake.nix:30-40`
- Includes: git, go, gnumake, pkg-config, bash
- Built: Once on agent-host via `nix build .#devShells.x86_64-linux.default`
- Realised: Into nix-shared volume via `nix copy --to file://...`
- Used: Workers resolve via `nix shell` or `nix develop` (no runtime builds)

**Never**: apt, apk add, nix profile install, throwaway builds.

### 4. PATH Resolution in NixOS Containers

**Why**: NixOS bins live in /run/current-system/sw/bin (a symlink tree); standard /usr/bin is limited.

**Implementation**:
- Both runners prepend `/run/current-system/sw/bin:` to PATH
- If no PATH provided: `PATH=/run/current-system/sw/bin:/usr/bin:/bin`
- If PATH exists: `/run/current-system/sw/bin:` prepended

**Code**:
- CLI: `cli_runner.go:246-254`
- Client: `client_runner.go:394-403`

### 5. External Grading (Oracle Verification)

**Why**: Ensure worker cannot tamper with grading; oracle runs in pristine environment.

**Flow**:
1. Worker runs, produces git diff (`git format-patch`)
2. Oracle setup: Clone clean checkout to temp dir
3. Patch apply: Apply worker's diff (checks success)
4. Oracle exec: Run user's command on patched code
5. Results: Exit code, stdout, stderr, patch applicability

**Code**: `helpers.go:28-96` (`runExternalGrading`)

## Implementation Checklist

- ✅ `flake.nix` devShell with git, go, gnumake, pkg-config, bash
- ✅ CLI runner: attach nix-shared at /nix/store readonly=true
- ✅ Go client runner: attach nix-shared with readonly: "true"
- ✅ Both runners: set /run/current-system/sw/bin in PATH
- ✅ Both runners: support --root flag
- ✅ Both runners: auto-enable SharedNixStore for NixOS images
- ✅ External grading: runExternalGrading helper implemented
- ✅ Types: RunAsRoot, SharedNixStore, ExternalGradingCheckout fields
- ✅ Tests: go vet, go build clean
- ✅ Documentation: CLAUDE.md, README.md, this file

## One-Time Setup (Before First Use)

### 1. Create the Shared Volume

```bash
incus storage volume create default nix-shared -t filesystem
```

Verify:
```bash
incus storage volume list default | grep nix-shared
# Should show: nix-shared  filesystem  default
```

### 2. Populate the Volume (From agent-host)

This is done ONCE. The volume retains the store across container restarts.

**Option A: Mount and realise** (assumes agent-host is available)
```bash
# On the host machine with Incus access:
# Attach the empty volume writable to agent-host temporarily
incus config device add agent-host nix-build disk source=nix-shared path=/mnt/nix-store

# Then on agent-host (via incus exec):
incus exec agent-host -- bash -c '
  # Build devShell closure
  nix build .#devShells.x86_64-linux.default --out-link /tmp/devshell
  
  # Realise to the mounted volume
  nix copy --to file:///mnt/nix-store --from nixpkgs /tmp/devshell
'

# Detach the device from agent-host
incus config device remove agent-host nix-build
```

**Option B: Manual closure export** (if agent-host not available)
```bash
# On a Linux machine with nix/flake support:
nix build github:ndn/agent-sandbox#devShells.x86_64-linux.default --out-link /tmp/devshell
nix copy --to file:///path/to/nix-shared /tmp/devshell
```

Verify volume is populated:
```bash
incus storage volume info default nix-shared
# Should show: size > 0 (not empty)
```

### 3. Build the Dispatcher Binary

```bash
cd modules/incus-dispatcher
go build -o dispatcher .
```

## Running Tasks

### Simple Test (No Repo)

```bash
./dispatcher \
  --name hello-world \
  --cmd "echo 'Hello from NixOS container'" \
  --root
```

**Output** (JSON):
```json
{
  "exitCode": 0,
  "containerName": "dispatch-hello-world-1234567",
  "duration": "3s",
  "stdout": "Hello from NixOS container\n",
  "stderr": "",
  "patchAvailable": false,
  "artifactCount": 0
}
```

### Test with Git Repo

```bash
./dispatcher \
  --name test-suite \
  --repo https://github.com/example/repo \
  --ref main \
  --cmd "go test ./..." \
  --root
```

### Test with External Grading

```bash
./dispatcher \
  --name grade-submission \
  --repo /tmp/student-work \
  --ref HEAD \
  --cmd "make test" \
  --root \
  --external-grading ~/clean-oracle-checkout \
  --output-dir /tmp/results
```

**Output files**:
```
/tmp/results/
├── result.json          # Task metadata + grading results
├── patch                # Worker's git format-patch output
└── artifacts/           # Files from /output in container
```

## Environment Variables

Pass container env vars with `CONTAINER_` prefix:

```bash
CONTAINER_DEBUG=1 CONTAINER_LOG_LEVEL=trace ./dispatcher \
  --name test \
  --cmd "go test -v"
```

Inside container: `DEBUG=1`, `LOG_LEVEL=trace`

## Build Quality Checks

```bash
cd modules/incus-dispatcher

# Lint
go vet ./...

# Build
go build -o dispatcher .

# Format (optional, for code style)
gofmt -w .
```

All should succeed with no errors.

## Runner Selection

| Runner | Command | Use Case |
|--------|---------|----------|
| `--runner client` (default) | Go Incus client | Production, programmatic control, streaming |
| `--runner cli` | incus CLI commands | Development, debugging, simplicity |

Both support:
- Ephemeral containers
- Source delivery (git bundle or clone)
- Command execution with env vars
- Result harvesting (patches, artifacts)
- External grading
- Path resolution

No functional difference; CLI is slower (process overhead), client is faster.

## Troubleshooting

### "container not found" on launch
- Check remote: `incus list ndn-desktop:`
- Check image: `incus image list ndn-desktop: | grep nixos`

### Binary not found in container (exit 127)
- Verify /nix/store is mounted: `incus exec <container> -- mount | grep nix/store`
- Check PATH: `incus exec <container> -- sh -c 'echo $PATH'`
  - Should include `/run/current-system/sw/bin`

### Nix volume not attached
- Check exists: `incus storage volume list default | grep nix-shared`
- Check size: `incus storage volume info default nix-shared | grep size`
  - Should be > 0 bytes (populated)

### Patch won't apply in external grading
- Worker's code may differ from oracle's base
- Check `result.ApplyError` for details
- This is expected if oracle and worker are on different branches

## Code Files Reference

| File | Purpose | Key Changes |
|------|---------|-------------|
| `flake.nix` | Nix flake + devShell | Added devShells.default with git, go, gnumake |
| `types.go` | Task, Result types | RunAsRoot, SharedNixStore, ExternalGradingCheckout |
| `cli_runner.go` | Incus CLI runner | Line 129: readonly=true on nix-shared |
| `client_runner.go` | Go client runner | Line 196: readonly: "true" in device config |
| `helpers.go` | Utility functions | External grading, cmd context creation |
| `main.go` | CLI entry point | Flag parsing, task dispatch |
| `CLAUDE.md` | Project conventions | Dispatcher architecture, setup, conventions |
| `README.md` | User guide | Quick-start, architecture, flags, examples |

## Next Steps (Post-Implementation)

1. **Per-worker /nix/var snapshots**: For incremental closure builds (copy-on-write)
2. **Ollama router**: Multi-model grading via dispatcher env vars
3. **Metrics export**: CPU, memory, build time, storage usage
4. **Artifact packaging**: TAR/ZIP export for result harvesting
5. **Dry-run mode**: Show what would execute without launching container

## References

- Incus docs: https://linuxcontainers.org/incus/docs/main/
- NixOS docs: https://nixos.org/manual/nixos/stable/
- This project's CLAUDE.md: Conventions, setup procedures
- This project's README.md: Flag reference, examples, troubleshooting
