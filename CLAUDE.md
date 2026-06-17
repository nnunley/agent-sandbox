# incus-dispatcher: Ephemeral NixOS Containers for Task Execution

**Primary Tool**: `modules/incus-dispatcher/` — CLI + Go tool for launching ephemeral NixOS containers (`images:nixos/25.11`, NixOS-only) to run tasks in clean isolation with access to a shared binary cache.

## CONVERGED DESIGN: Binary-Cache via Shared Volume

**Shared binary cache model (NixOS-only):**
- One persistent base `nix-server` container publishes prebuilt packages to `nix-shared` volume via `nix copy --to file:///srv/nix-shared`
- **Shared Incus filesystem custom volume (`nix-shared`)** mounted read-only at `/srv/nix-shared` on all workers
- **Ephemeral NixOS worker containers** pull prebuilt packages from the shared cache without rebuilding
- Workers configure nix via `/etc/nix/nix.conf.d/cache.conf`:
  ```
  extra-substituters = file:///srv/nix-shared
  require-sigs = false
  ```
- DevShell pinning (`flake.nix`) declares `git`, `go`, `gnumake`, `pkg-config`, `bash`; built once on `nix-server`, cached for all workers

**Key Design Principles:**
- Binary-cache isolation: workers pull packages from cache, no build machinery required
- No daemon socket overhead: cache is a simple read-only filesystem, no network/RPC
- Exactly ONE publisher (`nix-server`) populates cache; workers never write
- Declarative dependencies only (flake devShell); cache built from flake closure
- NixOS-only: reproducible, clean dependency auditing without OS variants

## Flags & Options

| Flag | Meaning |
|------|---------|
| `--name` | Task identifier (required) |
| `--cmd` | Command to run in container (required) |
| `--repo` | Local path or git URL (optional) |
| `--ref` | Git ref to check out (default: HEAD) |
| `--branch` | Target branch to create (optional) |
| `--root` | Launch with `security.privileged=true` (allows installing dependencies, default: false) |
| `--external-grading <path>` | Oracle: run on clean checkout with worker's patch applied |
| `--runner` | `client` (default, Go client) or `cli` (incus commands) |
| `--image` | Incus image alias (default: `images:nixos/25.11`; NixOS-only) |
| `--binary-cache-path` | Path to shared nix cache on volume (default: `/srv/nix-shared`) |
| `--remote` | Incus remote (default: `ndn-desktop` → https://192.168.86.49:8443) |
| `--timeout` | Task timeout (default: 1h; e.g., `30m`, `1h30m`) |
| `--keep-on-failure` | Keep container alive if command fails (for debugging) |
| `--output-dir` | Directory to write results JSON + patch + artifacts |
| `--provider` | LLM provider: `anthropic` (default), `openai`, `ollama-cloud` |
| `--model` | Model name (e.g., `claude-3-5-haiku`, `gpt-4o-mini`) |

## Setup: Shared Binary-Cache Volume (One-Time)

```bash
# 1. Create the shared binary cache volume
incus storage volume create default nix-shared -t filesystem

# 2. Create the persistent nix-server container (publishes packages to cache)
incus launch images:nixos/25.11 nix-server --config security.privileged=true
incus config device add nix-server nix-shared disk pool=default source=nix-shared path=/srv/nix-shared

# 3. Populate the cache by building the devShell closure on nix-server
incus exec nix-server -- nix flake update <path-to-flake>
incus exec nix-server -- nix copy --to file:///srv/nix-shared \
  $(nix eval <path-to-flake>#devShells.x86_64-linux.default --raw)

# 4. Verify cache is populated
incus exec nix-server -- ls -la /srv/nix-shared/ | grep narinfo

# 5. Create a pristine snapshot for reproducibility (optional but recommended)
incus snapshot create nix-server pristine
```

Worker containers will automatically:
- Mount nix-shared volume read-only at `/srv/nix-shared`
- Configure nix substituters to use the shared cache
- Pull prebuilt packages on demand (no rebuilds)

## Build & Test

```bash
cd modules/incus-dispatcher
go build -o dispatcher .
go test ./...
go vet ./...
```

## Example: Run a test task (uses shared nix-daemon)

```bash
./dispatcher \
  --name my-test \
  --repo https://example.com/repo \
  --ref main \
  --cmd "go test ./..." \
  --root
```

The worker container will:
1. Mount the shared `/nix` volume (read-write for store access, read-only restriction at volume level)
2. Set `NIX_REMOTE=daemon` (clients connect to the shared daemon socket)
3. Run `go test` (which pulls `go` from the shared store via the daemon)
4. Exit; container cleaned up automatically

Exit code is the command's exit code (0 = success).

---


NixOS LXC container that hosts Firecracker micro-VMs for agents. The
container runs on `ndn-desktop:agent-host` (Incus). Each agent gets a
disposable micro-VM. A small Go reverse proxy on the bridge gateway gives
those VMs access to LLM APIs without exposing host credentials.

## Layout

- `flake.nix` — single `nixosConfigurations.agent-host`.
- `host/` — host (Incus container) configuration: `configuration.nix`,
  `networking.nix`.
- `guests/` — micro-VM definitions. `base.nix` is the shared base.
- `modules/llm-proxy.nix` — NixOS module wiring up the proxy service.
- `modules/llm-proxy/` — Go source for the proxy (stdlib only).
- `scripts/agent-host` — **use this instead of raw `incus exec`.** Wraps
  build / push / activate / status / log / proxy-check verbs and squashes
  output to one or two lines. Each verb is documented in the script header.
- `scripts/deploy.sh` — initial container provisioning (init / destroy /
  full update with container restart). For incremental work, prefer
  `scripts/agent-host deploy` (live switch, no container restart).
- `scripts/smoke-test.sh`, `scripts/clean.sh` — VM lifecycle helpers.
- `docs/plans/` — design docs.

## Important behaviors

- **No `nixos-rebuild switch` inside the container.** LXC has no bootloader
  and no kernel to swap. `scripts/deploy.sh` builds the new system closure,
  sets it as the system profile via
  `nix-env -p /nix/var/nix/profiles/system --set <path>`, runs
  `<path>/bin/switch-to-configuration boot` to register it for next boot,
  then restarts the container with `incus restart --force`. A live
  `switch-to-configuration switch` would also work for most changes but the
  current script always does a full container restart for predictability.
- **Bridge is `trustedInterface`.** Anything on `br-microvm` (10.88.0.0/24)
  can reach the proxy. There is no per-VM auth or rate limit. Only run
  trusted code in the VMs until that changes.
- **Proxy reads keys from systemd credentials.** Inject via Incus:
  `incus config set ndn-desktop:agent-host
   systemd.credential.anthropic_api_key=sk-ant-...
   systemd.credential.openai_api_key=sk-...`

## Testing

- Go: `cd modules/llm-proxy && go vet ./... && go test -race ./...`. The
  test suite uses `httptest.NewServer` and exercises header injection,
  credential stripping, body size cap, streaming flush, etc. Always run
  with `-race`.
- Nix: `nix flake check` (run on a Linux/Nix host — macOS dev machine has
  no `nix` installed).
- Deploy: `scripts/deploy.sh`. It activates a new system profile and
  reruns `switch-to-configuration switch`.

## Versioning

4-digit semver in `VERSION`: `MAJOR.MINOR.PATCH.MICRO`. CHANGELOG.md is
the source of truth for what shipped at each version.

## Ship workflow notes

- This repo lives on `main` (no feature-branch flow yet). `/ship` was
  designed for app projects; the parts that apply here are: build
  verification (`go vet` + `go build` + `nix flake check`), security
  review of any change to `modules/llm-proxy/`, and CHANGELOG/VERSION
  bumps.
- Deploys happen out-of-band through `scripts/deploy.sh`, not through GH
  Actions or merge automation.

<!-- headroom:learn:start -->
## Headroom Learned Patterns
*Auto-generated by `headroom learn` on 2026-06-02 — do not edit manually*

### Commit Conventions
*~800 tokens/session saved*
- A pre-commit hook blocks commit messages containing `Claude Code`, `Generated with Claude`, `Co-Authored-By: Claude`, or `claude.com/claude-code`. Ensure commit messages avoid these phrases; use human-authored, non-AI-branded summaries.

### Environment
*~500 tokens/session saved*
- Nix is NOT installed on the local machine; all Nix builds and deployments must run on the incus container `agent-host` via `incus exec`.
- Go IS installed locally for vet, test, and build checks (`go vet`, `go test`, `go build`). The incus container does NOT have Go.
- The incus container is named `agent-host`; use `incus exec agent-host -- bash -c '...'` for remote operations.

### Build & Testing
*~300 tokens/session saved*
- Always run shell commands from the repository root (`/Users/ndn/development/agent-sandbox`). Changing directory causes relative paths like `modules/llm-proxy/` to break. Use absolute paths or `cd` only for scoped commands.

<!-- headroom:learn:end -->
