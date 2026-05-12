# CLAUDE.md — agent-sandbox

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
