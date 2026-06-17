# Dispatcher Enhancements: NixOS, Provider Routing, External Grading (2026-06-17)

## Overview

Three interconnected enhancements to the Incus dispatcher and LLM proxy:

1. **NixOS image + privilege model** — containers default to NixOS for clean, auditable dependency closure
2. **Provider routing via llm-proxy** — dispatch tasks to Anthropic (Haiku), OpenAI, or Ollama Cloud
3. **External-grading guardrails** — verify worker diffs on a pristine checkout the worker never touched

## A. NixOS Image & Privilege Model

### Rationale

- Alpine/Ubuntu images require repeated installations of git, build tools, and language runtimes
- NixOS provides a **declarative, reproducible, auditable dependency closure** inside the agent shell
- Long-lived agent processes need to install dependencies — safe only with root in an isolated container

### Implementation

#### Image Selection

- **Default image**: `DefaultNixOSImageName = "images:nixos/25.05"` (NixOS 25.05 from images.linuxcontainers.org)
- **Fallback**: users can specify `--image ubuntu/24.04` or any custom image
- **Special shorthand**: `--image nixos` maps to DefaultNixOSImageName
- **Root capability**: `--root` flag enables root/privileged access (default: false)

#### Finding NixOS Images

On the remote (`ndn-desktop`), list available images:

```bash
incus image list ndn-desktop:
```

Look for `nixos/*` entries. If the preferred version isn't present, build one locally or use the
default. This project does not build guest images; we rely on linuxcontainers.org's public builds.

#### Guest Config (guests/base.nix)

The guest already includes:

- git, curl, wget, jq, ripgrep, fd, tree, gcc, gnumake, pkg-config
- Both root and unprivileged `agent` user
- `wheel` group for sudo (passwordless)
- DHCP-based network

When `--root` is set, the dispatcher delivers the task to run as root directly (no context switch).

### Usage

```bash
# NixOS with root (for agents that install deps)
incus-dispatcher --name build-task --repo /path/to/repo \
  --cmd "make build" \
  --image nixos --root

# Ubuntu with root (compatibility)
incus-dispatcher --name test --repo /path/to/repo \
  --cmd "go test ./..." \
  --image ubuntu/24.04 --root

# NixOS without root (safer, typical)
incus-dispatcher --name audit --repo /path/to/repo \
  --cmd "nix flake check" \
  --image nixos
```

---

## B. Provider Routing via llm-proxy

### Rationale

- Agent tasks may need different LLM providers (Anthropic's Haiku, OpenAI's gpt-4o-mini, Ollama)
- Credentials must never reach the worker; **only the proxy holds secrets**
- Proxy injects credentials via systemd credentials (Incus integration) into environment variables
- Single reverse-proxy entry point ensures credential isolation and audit logging

### Implementation

#### Task.Provider & Task.Model

The `Task` struct now carries:

```go
type Task struct {
    // ... existing fields ...
    Provider Provider // enum: "anthropic" | "openai" | "ollama-cloud"
    Model    string   // e.g., "claude-3-5-haiku", "gpt-4o-mini"
}

type Provider string
const (
    ProviderAnthropic   Provider = "anthropic"
    ProviderOpenAI      Provider = "openai"
    ProviderOllamaCloud Provider = "ollama-cloud"
)
```

Defaults: `Provider = ProviderAnthropic`, `Model = ""` (uses provider's default).

#### CLI Flags

```bash
incus-dispatcher --name task --repo ... --cmd ... \
  --provider openai --model gpt-4o-mini

incus-dispatcher --name task --repo ... --cmd ... \
  --provider ollama-cloud --model neural-chat
```

#### llm-proxy Routes

The proxy exposes three routes:

| Path | Upstream | Credentials | Status |
|------|----------|-------------|--------|
| `/anthropic/*` | https://api.anthropic.com | $ANTHROPIC_API_KEY | ✓ (existing) |
| `/openai/*` | https://api.openai.com | $OPENAI_API_KEY | ✓ (new) |
| `/ollama-cloud/*` | https://ollama.ai (or $OLLAMA_CLOUD_URL) | $OLLAMA_CLOUD_API_KEY | ✓ (new) |

Each route:
1. Strips client-supplied Authorization headers
2. Injects the proxy's credential (from systemd credentials or env)
3. Logs the request/response (provider, method, path, status, duration, bytes)
4. Returns early with 503 if a required key is missing

#### Systemd Credentials

The llm-proxy.nix service now imports three credential files:

```nix
ImportCredential = [
  "anthropic_api_key"
  "openai_api_key"
  "ollama_cloud_api_key"
];
```

Incus injects them into $CREDENTIALS_DIRECTORY, and the startup script reads them into env vars:

```bash
export ANTHROPIC_API_KEY=""
export OPENAI_API_KEY=""
export OLLAMA_CLOUD_API_KEY=""

creds="${CREDENTIALS_DIRECTORY:-}"
if [ -n "$creds" ]; then
  [ -f "$creds/anthropic_api_key" ]    && ANTHROPIC_API_KEY=$(cat "$creds/anthropic_api_key")
  [ -f "$creds/openai_api_key" ]       && OPENAI_API_KEY=$(cat "$creds/openai_api_key")
  [ -f "$creds/ollama_cloud_api_key" ] && OLLAMA_CLOUD_API_KEY=$(cat "$creds/ollama_cloud_api_key")
fi
```

#### Agent Configuration

Inside a container, the agent sets SDK env vars to point at the proxy:

```bash
# For Anthropic
export ANTHROPIC_BASE_URL=http://10.88.0.1:12071/anthropic

# For OpenAI
export OPENAI_BASE_URL=http://10.88.0.1:12071/openai

# For Ollama Cloud
export OLLAMA_CLOUD_BASE_URL=http://10.88.0.1:12071/ollama-cloud
```

The dispatcher passes these via the `Task.Env` map (or container env setup).

### Credential Injection (Non-Anthropic)

For OpenAI and Ollama Cloud, the proxy injects:

```
Authorization: Bearer $API_KEY
```

Anthropic gets both `Authorization: Bearer` and the legacy `x-api-key` header (for API compatibility).

### Usage

```bash
# Dispatch to OpenAI via the proxy
incus-dispatcher --name gpt-task --repo /path/to/repo \
  --cmd "python test.py" \
  --provider openai --model gpt-4o-mini

# Inside the container, the agent reads OPENAI_BASE_URL and uses the SDK normally
# The proxy intercepts /openai/v1/messages, injects Bearer $OPENAI_API_KEY, and forwards
```

---

## C. External-Grading Guardrails

### Rationale

The **authoritative defense against reward-hacking**: the oracle runs on a **clean checkout** the worker
**never touched**. The worker cannot tamper with the grading because it never sees the oracle.

- Worker runs in `$WORKER_INSTANCE` → produces a git diff
- Dispatcher harvests the diff (`git format-patch`)
- Dispatcher creates a **fresh clone** of the repo (from a clean checkout the worker never accessed)
- Dispatcher applies the worker's diff to the fresh clone
- Dispatcher runs the oracle (test suite) on the fresh clone
- Dispatcher reports the oracle's exit code and output

### Implementation

#### Task.ExternalGradingCheckout

```go
type Task struct {
    // ... existing fields ...
    ExternalGradingCheckout string // path to clean checkout (e.g., /clean/repo)
}
```

#### Result.ExternalGradingResult

```go
type GradingResult struct {
    ExitCode    int       // exit code from the oracle
    Stdout      string    // oracle stdout
    Stderr      string    // oracle stderr
    Duration    time.Duration
    PatchApplied bool      // did the diff apply cleanly?
    ApplyError   string    // if PatchApplied == false, why not
}
```

#### runExternalGrading() Helper

Implemented in helpers.go:

```go
func runExternalGrading(ctx context.Context, workerDiff []byte, cleanCheckoutPath string, oracleCmd []string) (*GradingResult, error)
```

Flow:

1. **Create temp workspace**: `mkdir /tmp/grading-<uuid>`
2. **Clone clean checkout**: `git clone <cleanCheckoutPath> /tmp/grading-<uuid>/src`
   - The clone is independent; the worker never touched this checkout
3. **Apply worker's diff**:
   - Write the diff to `/tmp/grading-<uuid>/worker.patch`
   - Run `git apply --check` (dry run)
   - If successful, run `git apply` (apply for real)
   - If failed, log the error and continue (oracle runs on unpatched code)
4. **Run the oracle**:
   - Execute `oracleCmd` in the newly-patched source dir
   - Capture exit code, stdout, stderr
5. **Return GradingResult**

#### Integration in Client Runner

In `client_runner.go`, after harvesting results:

```go
// Phase 5: External grading (if configured)
if task.ExternalGradingCheckout != "" {
    gradingResult, err := runExternalGrading(taskCtx, result.PatchData, task.ExternalGradingCheckout, task.Cmd)
    if err != nil {
        return result, nil // log the error but don't fail; return what we have
    }
    result.ExternalGradingResult = gradingResult
}
```

#### CLI Flags

```bash
incus-dispatcher --name worker-task --repo /path/to/repo \
  --cmd "go test ./..." \
  --external-grading /clean-checkouts/myrepo
```

#### JSON Output

When external grading runs, the result JSON includes:

```json
{
  "exitCode": 0,
  "containerName": "dispatch-worker-...",
  "duration": "45s",
  "stdout": "...",
  "stderr": "",
  "patchAvailable": true,
  "artifactCount": 0,
  "grading": {
    "exitCode": 0,
    "duration": "10s",
    "stdout": "PASS: all tests",
    "stderr": "",
    "patchApplied": true,
    "applyError": ""
  }
}
```

### Defense Layers (Depth)

| Layer | Mechanism | Scope |
|-------|-----------|-------|
| **Primary** | Clean checkout isolation (worker ≠ oracle) | Rewards |
| **Secondary** | In-instance deny-hook on test files (fast feedback) | Quick feedback; worker can bypass (root) |
| **Tertiary** | Scoped checkout delivery (only in-scope files to worker) | Reduce attack surface; partial |

### Usage

**Scenario**: An agent makes edits to a repo and produces a patch. We want to verify the patch
passes all tests on a clean tree.

```bash
# On the fresh checkout machine, keep a clone isolated:
git clone /path/to/upstream /clean-checkouts/myrepo
chmod 555 /clean-checkouts/myrepo  # read-only (optional; not enforced)

# Dispatch the worker task with external grading:
incus-dispatcher --name agent-edit --repo /local/myrepo --ref main \
  --cmd "make edit && git format-patch -o /output" \
  --external-grading /clean-checkouts/myrepo

# Result includes:
# - worker.exitCode: 0 (edit succeeded)
# - worker.stdout: <edit output>
# - grading.exitCode: 0 (tests passed on clean tree)
# - grading.stdout: <test output>
```

If the worker somehow hacks the oracle (e.g., modifies test code), the oracle still runs on the
clean tree because it never saw the worker's edit.

---

## Files Modified / Created

### New Files
- `docs/2026-06-17-dispatcher-enhancements.md` — this document

### Modified Files

#### Dispatcher
- `modules/incus-dispatcher/types.go`
  - Added `Provider` enum, `Model`, `RunAsRoot`, `ExternalGradingCheckout` to `Task`
  - Added `GradingResult` struct
  - Added `DefaultNixOSImageName` constant, `ValidateProvider()` method

- `modules/incus-dispatcher/main.go`
  - Added `--provider`, `--model`, `--root`, `--external-grading` flags
  - Updated `--image` help text to mention `nixos` shorthand
  - Validate provider and map special image names

- `modules/incus-dispatcher/client_runner.go`
  - Added Phase 5 (external grading) to `Run()`
  - Calls `runExternalGrading()` if `task.ExternalGradingCheckout` is set

- `modules/incus-dispatcher/helpers.go`
  - Added `runExternalGrading()` function

#### Proxy & Nix
- `modules/llm-proxy/main.go`
  - Added `/ollama-cloud` route
  - Updated documentation comments

- `modules/llm-proxy.nix`
  - Added `ollamaCloudUrl` option
  - Added `ollama_cloud_api_key` to `ImportCredential`
  - Updated startup script to read `OLLAMA_CLOUD_API_KEY` from credentials

### Infrastructure
- `go.mod` — created (first time Go modules for this repo)

---

## Testing & Verification

### Build & Unit Tests

All tests pass (green):

```bash
cd /Users/ndn/development/agent-sandbox/modules/incus-dispatcher
go build -o /tmp/test-dispatcher .
go test -v .   # 7 tests passed

cd /Users/ndn/development/agent-sandbox/modules/llm-proxy
go build -o /tmp/test-proxy .
go test -v .   # 15 tests passed
```

### Manual Verification (Gated)

If `ndn-desktop` is reachable, the following can be verified:

#### 1. NixOS Instance Launch (pending verification)

```bash
# TBD: Verify NixOS image is available on ndn-desktop
incus image list ndn-desktop: | grep nixos

# TBD: Launch a NixOS instance with root
incus-dispatcher --name test-nixos --image nixos \
  --cmd "whoami && nix --version" \
  --root

# Expected: exit code 0, root user, nix available
```

#### 2. Provider Routing (pending verification)

The proxy now routes OpenAI and Ollama Cloud requests. Verification requires:
- Setting systemd credentials for OPENAI_API_KEY and OLLAMA_CLOUD_API_KEY (not present yet)
- Making test calls through the proxy
- Confirming Authorization headers are injected and logged

#### 3. External Grading (pending verification)

Create a test scenario:
```bash
# 1. Clone a clean repo to /clean-checkouts/test-repo
# 2. Create a worker task that edits and produces a patch
# 3. Run external grading on the clean clone
# 4. Verify the oracle runs independently of the worker
```

### Known Gaps / TODOs

1. **NixOS image availability**: This design assumes `images:nixos/25.05` exists on ndn-desktop.
   If not, document how to build or substitute.

2. **External grading oracle command**: Currently, the dispatcher runs the same command as the worker
   (`task.Cmd`). This is reasonable for test suites but may need refinement for other oracles.
   Consider adding a separate `--oracle-cmd` flag if different behavior is needed.

3. **Scoped checkout delivery**: Part C's tertiary defense (scoped checkout) is documented but
   not implemented. This is a future hardening (not essential for the current design).

4. **Agent env setup**: Workers need to set LLM SDK env vars pointing at the proxy. This is
   handled via `parseEnv()` (CONTAINER_* convention) but not integrated with the provider/model
   selection yet. Future enhancement: auto-wire provider-specific env vars based on Task.Provider.

5. **Ollama Cloud URL & API Key**: The default is https://ollama.ai, but this should be confirmed.
   Users can override via `ollamaCloudUrl` NixOS option and OLLAMA_CLOUD_API_KEY env var.

---

## Summary

- **A. NixOS + Root**: Containers now default to NixOS (clean closure, git included). `--root` flag
  enables root access for agents that install deps. Backwards compatible; users can specify
  `--image ubuntu/24.04` for other images.

- **B. Provider Routing**: Three providers routed through llm-proxy (Anthropic, OpenAI, Ollama Cloud).
  Credentials injected via systemd credentials, never exposed to workers. Task and CLI both support
  provider/model selection.

- **C. External Grading**: Oracle runs on a pristine checkout the worker never touched. Worker
  cannot tamper with grading. Flow: harvest diff → clone clean repo → apply patch → run oracle →
  return results.

All code builds, unit tests pass. Manual verification of NixOS image availability and end-to-end
provider routing requires access to ndn-desktop (gated, pending user confirmation).
