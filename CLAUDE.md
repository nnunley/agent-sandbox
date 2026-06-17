# incus-dispatcher: Ephemeral NixOS Containers for Task Execution

**Primary Tool**: `modules/incus-dispatcher/` — CLI + Go tool for launching ephemeral NixOS containers (`images:nixos/25.11`) to run tasks in clean isolation with shared read-only `/nix/store`.

## Key Design

- **Workers**: Root/privileged NixOS containers; image is `images:nixos/25.11`
- **Shared Store**: One filesystem volume `nix-shared` (Incus) mounted at `/nix/store` read-only across all workers
- **Population**: DevShell closure (from `flake.nix`) built once on `agent-host`, realised into the volume via `nix copy`
- **Toolchain**: `git`, `go`, `gnumake`, `pkg-config`, `bash` — all from flake devShell, prebuilt in store
- **Runners**: Both CLI (`incus` commands) and Go client (`lxc/incus/v6`) backends
- **External Grading**: Optional oracle verification — worker's patch applied to clean checkout, oracle runs on patched version

## Flags & Options

| Flag | Meaning |
|------|---------|
| `--name` | Task identifier (required) |
| `--cmd` | Command to run in container (required) |
| `--repo` | Local path or git URL (optional) |
| `--ref` | Git ref to check out (default: HEAD) |
| `--root` | Launch with `security.privileged=true` (default: false) |
| `--external-grading <path>` | Oracle: run on clean checkout with worker's patch applied |
| `--runner` | `client` (default, Go client) or `cli` (incus commands) |
| `--image` | Incus image alias (default: `images:nixos/25.11`) |
| `--remote` | Incus remote (default: `ndn-desktop` → https://192.168.86.49:8443) |

## Declarative Dependencies (Hard Rule)

- ✅ All worker tools come from **`flake.nix` → `devShells.default`**
- ✅ Toolchain closure is built once, realised to the shared `/nix/store` volume
- ❌ NO `apt`, `apk add`, `nix profile install` inside containers
- ❌ NO throwaway Ubuntu instances or temporary build containers
- ❌ Each worker has its own `/nix/var` (not shared)

## Setup: Shared Volume (One-Time)

```bash
# Create the volume
incus storage volume create default nix-shared -t filesystem

# Populate it (on agent-host or manually):
# - Build devShell closure
# - nix copy to the volume
# (Exact command depends on mounting strategy; see docs/plans/)
```

## Build & Test

```bash
cd modules/incus-dispatcher
go build -o dispatcher .
go test ./...
go vet ./...
```

## Example: Run a test task

```bash
./dispatcher \
  --name my-test \
  --repo https://example.com/repo \
  --ref main \
  --cmd "go test ./..." \
  --root
```

Exit code is 0 (task success) or match the command's exit code.

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
