# Changelog

All notable changes to agent-sandbox are recorded here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.0.0] - 2026-05-09

First named cutpoint. Establishes the baseline state of the agent-sandbox
host: a NixOS LXC container that runs Firecracker micro-VMs for agents and
bridges them to cloud + local LLMs through a credential-injecting proxy.

### Added
- Initial flake with `nixpkgs/nixos-25.11` and `microvm.nix` inputs.
- Host NixOS config: bridge networking (`br-microvm` on 10.88.0.1/24),
  dnsmasq for DHCP/DNS, NAT to the container's eth0, systemd-networkd.
- Base micro-VM definition using Firecracker as the hypervisor.
- LLM reverse proxy (`modules/llm-proxy/`): path-prefix routing for
  Anthropic, OpenAI, and two local llama.cpp endpoints; API keys loaded
  from systemd credentials; JSONL request logging to stdout. Hardened
  systemd unit (DynamicUser, ProtectSystem=strict, etc.).
- LXC container module fixes for systemd-networkd compatibility.
- Deploy script using `nix-env --profile-set` + restart instead of
  `nixos-rebuild switch` (works inside LXC where bootloader/kernel
  rebuilds are not appropriate).
- Smoke-test and clean scripts for VM lifecycle management.
- Initial design spec at `docs/plans/2026-04-02-agent-sandbox-design.md`.

### Security
- Proxy strips `Authorization` and `x-api-key` from incoming requests
  before injecting its own, so a compromised micro-VM cannot smuggle
  alternate credentials to upstream APIs.
- Routes that require an API key fail-fast with HTTP 503 if the key is
  missing, instead of forwarding unauthenticated and producing a
  confusing upstream 401.
- Hop-by-hop headers (RFC 7230 §6.1) are stripped before forwarding.
- `path.Clean` is applied to upstream paths to neutralize `../` segments.

### Known gaps
- No Go unit tests yet.
- No per-VM auth or quota tracking on the proxy — every VM on the
  bridge can spend the host's API budget.
- No request body size cap.
- `LOCAL_FAST_URL` / `LOCAL_LARGE_URL` defaults bake in a specific LAN IP.
