# Changelog

All notable changes to agent-sandbox are recorded here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.1.1.0] - 2026-05-09

Closes the open review findings from the v0.1.0.0 cutpoint and adds the
first Go test suite for the proxy.

### Added
- 15 Go tests for `llm-proxy` covering header injection, client-credential
  stripping (including the no-key case), missing-required-key 503,
  path-traversal cleaning, prefix boundary, hop-by-hop stripping, body
  size cap, accurate `bytes_in` for chunked uploads, streaming flush,
  upstream-failure 502, `/health`, and concurrent log-line integrity.
  Suite passes under `-race`.
- `services.llm-proxy.localFastUrl` and `localLargeUrl` Nix options so
  the LAN IPs aren't baked into the system closure.
- `services.llm-proxy.maxBodyBytes` Nix option (default 32 MiB),
  passed to the proxy via `LLM_PROXY_MAX_BODY`.

### Changed
- Replaced `http.ListenAndServe` with an explicit `http.Server` carrying
  `ReadHeaderTimeout=30s` and `IdleTimeout=2m`. Long-running streaming
  reads/writes still have no per-request deadline.
- Logs go through a buffered, mutex-protected writer to stdout, ending
  per-line interleaving under concurrent load. Each entry ends in `\n`.
- Streaming responses now flush per chunk via `http.Flusher`. SSE chunks
  reach the agent as they arrive instead of being buffered.
- `bytes_in` is computed from a counting wrapper around the request body,
  so chunked uploads (no Content-Length) report accurate sizes.
- Request bodies above `maxBodyBytes` are truncated by `http.MaxBytesReader`.
- `newRoute` now returns `(route, error)` and rejects URLs missing a
  scheme or host at startup, so the proxy fails fast instead of producing
  502s at runtime.
- Refactored: routing/forwarding logic moved to `proxy.go`; `main.go` is
  now thin wiring (env, server lifecycle).

### Fixed
- CLAUDE.md description of `scripts/deploy.sh` corrected — it uses
  `switch-to-configuration boot` followed by an Incus container restart,
  not `switch`.

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

(All four were addressed in v0.1.1.0.)
