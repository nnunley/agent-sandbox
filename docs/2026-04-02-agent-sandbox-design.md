# Agent Sandbox: Firecracker Micro-VMs on Incus via NixOS

**Date**: 2026-04-02
**Status**: Draft
**Repo**: `~/development/agent-sandbox/`

## Problem

Running coding agents (Sprout, serf, etc.) on local machines offers no isolation, no auditability, and no way to fan out eval runs. Stockyard solves this with Firecracker micro-VMs but requires a specific stack (containerd, Flintlock, ZFS) that doesn't match our existing infrastructure: an Incus remote on Ubuntu with btrfs storage.

## Solution

A standalone Nix flake that:

1. Defines a NixOS container for an existing Incus remote (no host bootstrapping)
2. Uses microvm.nix to manage Firecracker micro-VMs inside that container
3. Builds generic guest images via Nix, with composable overlays for specific agents
4. Provides scripts for deployment, day-to-day agent runs, and eval fan-out

## Architecture

```
Mac (local) ──incus CLI──→ Incus remote (Ubuntu, btrfs)
  │                              └── agent-host (NixOS container)
  │                                    ├── microvm.nix host module
  │                                    ├── br-microvm (bridge)
  │                                    ├── llm-proxy :12071
  │                                    └── Firecracker micro-VMs
  │                                          ├── eval-task-1
  │                                          ├── eval-task-2
  │                                          └── ...
  │
  └── nix build → guest rootfs + kernel
```

### Key properties

- **No host bootstrapping**: the Incus remote is already running. We create a container on it via `incus` CLI, never SSH to the host.
- **KVM passthrough**: `/dev/kvm` is passed through to the NixOS container. This gives bare-metal KVM performance — no nested virtualization. `/dev/vhost-vsock` passthrough is deferred — serial console and network are sufficient for the initial implementation.
- **Declarative convergence**: `nixos-rebuild switch` on the container reconverges everything. State lives in shared storage, not container config.
- **Fast micro-VM boot**: ~1-2s boot-to-ready (125ms Firecracker + NixOS systemd init).

## Components

### 1. Flake Structure

```
agent-sandbox/
├── flake.nix              — inputs: nixpkgs, microvm.nix
├── flake.lock
├── host/
│   └── configuration.nix  — NixOS config for the Incus container
├── guests/
│   ├── base.nix           — generic coding agent guest
│   ├── sprout.nix         — base + bun + sprout (fast follow)
│   └── serf.nix           — base + go + serf (fast follow)
├── modules/
│   ├── llm-proxy.nix      — LLM API traffic logging service
│   └── orchestrator.nix   — micro-VM lifecycle management
└── scripts/
    ├── deploy.sh           — create/update NixOS container on remote
    ├── run.sh              — launch a single micro-VM (day-to-day use)
    ├── run-eval.sh         — fan out tasks across micro-VMs
    ├── smoke-test.sh       — boot one VM, run a trivial command, tear down
    └── clean.sh            — list and destroy all micro-VMs on the host
```

### 2. Host Container (NixOS on Incus)

The `agent-host` NixOS container provides:

- **microvm.nix host module**: manages Firecracker VM lifecycle as systemd services (`microvm@<name>.service`)
- **KVM access**: via Incus device passthrough (configured by deploy script, not NixOS)
- **Bridge networking**: `br-microvm` internal bridge (192.168.100.0/24) with NAT to container's `eth0`
- **DHCP + DNS**: dnsmasq on `br-microvm` serving IP addresses and DNS to micro-VMs
- **NAT**: iptables masquerade rules + `net.ipv4.ip_forward=1` for outbound traffic
- **TAP interfaces**: microvm.nix generates systemd dependencies for per-VM TAP setup
- **LLM proxy**: reverse proxy on `br-microvm:12071`, routes LLM requests with key injection
- **Shared storage**: `/var/lib/agent-sandbox/` for eval artifacts, ATIF trajectories, results

**Container privileges**: the Incus container requires `security.nesting=true` for iptables/NAT and the ability to set sysctls like `net.ipv4.ip_forward`. Deploy script configures this.

Networking model:

```
Internet ← Incus bridge ← agent-host container
                              ├── br-microvm (internal bridge, e.g. 192.168.100.0/24)
                              │     ├── tap-vm1 → micro-VM 1
                              │     ├── tap-vm2 → micro-VM 2
                              │     └── ...
                              ├── llm-proxy :12071 (on br-microvm)
                              └── NAT (br-microvm → eth0)
```

Micro-VMs are configured with LLM provider base URLs pointing at the proxy (reverse proxy model — see LLM Proxy section). The proxy handles TLS to upstream APIs and injects API keys, so VMs never hold credentials.

### 3. Guest Image (Base)

Minimal NixOS system built as erofs by microvm.nix:

**Included:**
- Core tools: git, curl, wget, jq, ripgrep, fd, tree
- Build essentials: gcc, make, pkg-config
- Shell: bash with minimal profile
- User: unprivileged `agent` user, passwordless sudo
- Networking: DHCP on virtio-net, LLM provider base URLs pointing at host proxy
- Output mount: `/output` → writable volume mapped to host shared storage

**Excluded:**
- No language runtimes (come from overlays)
- No agent binaries (come from overlays)
- No SSH (serial console or vsock for access)
- No X11, docs, or other bloat

**Minimization:**
```nix
documentation.enable = false;
environment.noXlibs = true;
```

**Overlay pattern:**
```nix
# guests/sprout.nix
{ imports = [ ./base.nix ];
  environment.systemPackages = [ pkgs.bun ];
  # sprout binary via volume or built into image
}

# guests/serf.nix
{ imports = [ ./base.nix ];
  environment.systemPackages = [ pkgs.go ];
}
```

### 4. Deployment

**First-time setup** (`deploy.sh init`):

1. `nix build .#nixosConfigurations.host.config.system.build.toplevel`
2. `incus launch images:nixos/unstable agent-host` (container, not VM)
3. `incus config device add agent-host kvm unix-char path=/dev/kvm`
4. `incus config set agent-host security.nesting=true`
5. Export NixOS closure: `nix store export --recursive $(readlink result)` → pipe to `incus exec agent-host -- nix-store --import`
6. Activate: `incus exec agent-host -- result/bin/switch-to-configuration switch`

**Updates** (`deploy.sh update`):

1. Build new closure
2. Push delta to container via `nix store export` → `incus exec` → `nix-store --import`
3. `incus exec agent-host -- nixos-rebuild switch`

### 5. Run Workflows

**Day-to-day:**

```bash
# Start a micro-VM, drop into it
./scripts/run.sh --guest base

# Run sprout with a task
./scripts/run.sh --guest sprout -- sprout -p "Fix the flaky test"
```

The script:
1. Starts a micro-VM via `microvm@<name>.service` on the host container
2. If a command is provided, runs it inside the VM and collects output
3. If no command, provides interactive access via serial console
4. Cleans up VM on exit

**Eval fan-out:**

```bash
./scripts/run-eval.sh \
  --guest sprout \
  --tasks ./eval/tasks/ \
  --concurrency 4 \
  --output ./results/
```

The script:
1. For each task file: start a micro-VM from the guest image
2. Run the agent with `--eval-mode --log-atif /output/trajectory.json`
3. Respect concurrency limit (track PIDs, `wait -n` to backfill slots)
4. Collect `/output` from each VM into `results/<task-name>/`
5. Destroy VMs after artifact collection
6. Pull results from host container to local machine via `incus file pull`

Each micro-VM gets its own `/output` volume mapped to `$SHARED_STORAGE/<run-id>/<vm-id>/` on the host.

**Smoke test** (`smoke-test.sh`):

1. Boot one micro-VM from the base image
2. Verify it gets a DHCP address on `br-microvm`
3. Verify it can reach the LLM proxy (`curl http://192.168.100.1:12071/health`)
4. Verify it can reach the internet via allowed hosts (`curl https://github.com`)
5. Write a test file to `/output` and verify it appears in host shared storage
6. Tear down the VM
7. Exit 0 on success, non-zero with diagnostics on failure

## Closure Delivery

Getting the NixOS closure into the Incus container is the hardest deployment step. The mechanism:

1. Build the closure locally: `nix build .#nixosConfigurations.host.config.system.build.toplevel`
2. Export as a NAR archive: `nix store export --recursive $(readlink result)` → pipe
3. Push into container: pipe to `incus exec agent-host -- nix-store --import`
4. Activate: `incus exec agent-host -- result/bin/switch-to-configuration switch`

For updates, `nix copy --to ssh-ng://agent-host` is the ideal path. Since we avoid SSH to the *host*, we use `incus exec` as the transport. A wrapper script translates `nix copy` into `incus exec` + `nix-store --import` for the delta.

Alternative considered: building a custom NixOS image with the full closure baked in and importing it via `incus image import`. Rejected because it means rebuilding the entire image on every config change instead of pushing deltas.

## Repo Delivery into Micro-VMs

Target repositories (the code the agent works on) get into micro-VMs via network clone:

- **Eval mode**: the task file specifies a git URL + ref. The VM boots, clones the repo, runs the agent.
- **Day-to-day**: the user specifies a repo URL or the run script mounts a pre-cloned block device volume.

Git clone is the simplest approach and works with Firecracker's network-only constraint. For large repos where clone time matters, a pre-warmed block device volume containing the repo can be attached to the VM at launch.

## Resource Defaults

| Resource | Default | Notes |
|----------|---------|-------|
| VM memory | 4096 MiB | Sufficient for most agent + compilation workloads |
| VM vCPUs | 2 | Matches Stockyard's default |
| `/output` volume | 2 GiB | ATIF trajectories + logs + artifacts |
| Host container memory | `concurrency × VM memory + 1 GiB` overhead | Set via Incus config |
| Host container CPUs | `concurrency × VM vCPUs + 1` | Set via Incus config |
| Task timeout | 3600s (1 hour) | Eval tasks killed after this |

All configurable via CLI flags or a config file.

## LLM Proxy

Uses a **reverse proxy** model — not a forward proxy. Agents inside micro-VMs send requests directly to the proxy's HTTP endpoint, and the proxy handles TLS to upstream APIs, injects API keys, and logs traffic.

- Proxy listens on `http://192.168.100.1:12071`
- Agents are configured with provider base URLs pointing at the proxy (e.g., `ANTHROPIC_BASE_URL=http://192.168.100.1:12071/anthropic/`, `OPENAI_BASE_URL=http://192.168.100.1:12071/openai/`)
- The proxy routes by path prefix, adds `Authorization` headers from host-side key storage, and forwards to the real API over HTTPS
- Log format: JSONL with timestamp, provider, model, token counts, latency, request/response bodies (configurable — can redact bodies for cost-only audit)

This means micro-VMs never see API keys and never make direct HTTPS connections to LLM providers. All LLM usage is centrally audited. This is the same model Stockyard's LLM proxy on port 12071 uses.

Implementation: either prime-radiant-inc's `llm-proxy` (if it supports this routing mode) or a lightweight custom Go/Nix service. The interface is simple — path-prefix routing + header injection + JSONL logging.

## Network Security

Micro-VM egress is restricted via iptables on the host container:

- **Allowed**: traffic to `br-microvm` gateway (proxy), DNS
- **Allowed**: traffic to a configurable allowlist (git hosts: github.com, package mirrors)
- **Blocked**: all other outbound traffic
- **Eval mode**: stricter — only proxy access, no direct internet (repos pre-cloned or cloned via allowed git hosts)

This prevents untrusted agent code from exfiltrating data to arbitrary endpoints.

## Error Handling and Failure Modes

| Failure | Handling |
|---------|----------|
| VM fails to boot | Log error, skip task, continue eval fan-out. Exit with partial results. |
| VM hangs (exceeds timeout) | `systemctl stop microvm@<name>`, collect partial artifacts, log timeout. |
| OOM inside VM | Firecracker kills the VM. Orchestrator detects service exit, logs OOM, continues. |
| Host container unreachable | Scripts fail fast with clear error. No partial state to clean up (VMs are inside the container). |
| Incus CLI connectivity lost mid-eval | Running VMs continue inside the container. Reconnect and resume artifact collection. VMs that finish write to shared storage regardless. |
| Zombie VMs | Orchestrator tracks all started VMs. Cleanup phase on script exit (including SIGINT/SIGTERM) stops all VMs in the current run. A `clean.sh` script lists and destroys all micro-VMs. |

## Boot Time Expectations

Firecracker boots in ~125ms. A minimal NixOS guest with systemd adds ~500ms-1s for service initialization. Expect **~1-2s total boot-to-ready** for the base image. This is still an order of magnitude faster than an Incus VM (~5-15s) and faster than cold-starting an Incus container (~1-2s).

## Constraints

- **Incus remote must be reachable** via `incus` CLI from the local machine
- **Ubuntu host must have KVM support** (should already — standard for Incus hosts)
- **No host-level changes required** beyond the Incus container itself
- **microvm.nix limitation**: no 9p/virtiofs filesystem shares with Firecracker — data passes through block device volumes or network
- **Firecracker limitation**: TAP networking only, no SLiRP

## Future Work

- **Snapshot/restore**: leverage Firecracker's snapshot mechanism for instant resume of pre-warmed VMs
- **btrfs integration**: use btrfs snapshots at the Incus layer for fast guest image cloning
- **Sprout genome persistence**: mount genome directory as a block device volume across micro-VM restarts
- **Result analysis**: integrate with terminal-bench-analysis for eval scoring
- **Multi-host fan-out**: create agent-host containers on multiple Incus remotes for higher concurrency
