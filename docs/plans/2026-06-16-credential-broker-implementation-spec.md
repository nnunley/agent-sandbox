# Credential Broker Implementation Spec for agent-sandbox

**Date:** 2026-06-16
**Status:** Draft implementation spec
**Depends on:** `2026-06-16-credential-broker-architecture.md`

## Purpose

Turn the high-level broker architecture into an implementation plan that fits the current `agent-sandbox` repository shape.

This spec is intentionally narrow in one way: it only covers the **credential and provider-control plane**. It does **not** yet solve dynamic VM spawning, worker scheduling, or queue orchestration.

## Existing repo anchors

The design should build on the existing implementation rather than introduce a parallel control path.

Current anchors:

- `modules/llm-proxy.nix`
  - NixOS service definition for the worker-facing proxy
- `modules/llm-proxy/main.go`
  - process wiring, route table, HTTP server lifecycle
- `modules/llm-proxy/proxy.go`
  - request routing, header stripping/injection, logging
- `host/configuration.nix`
  - host service composition
- `host/networking.nix`
  - bridge and NAT
- `guests/base.nix`
  - base worker guest

The implementation should preserve the current shape:

- workers talk to a local proxy
- proxy mediates provider access
- provider credentials do not live in guests

## Target component split

Add three new layers while preserving the existing proxy.

```text
Mac
  brokerd
    vault backend(s)
    enrollment registry
    provider session adapters
    broker API server

agent-host
  llm-proxy
    provider instance config
    broker client
    in-memory delegated session cache
    upstream adapters

workers
  provider instance env only
  no secrets
```

## New modules and files

### On the Mac side

This repo does not currently manage the Mac host. For the MVP, the broker can begin as a separately launched local daemon, with Nix packaging later.

Suggested new package/repo-internal layout:

- `modules/credential-broker/`
  - `main.go`
  - `server.go`
  - `enrollment.go`
  - `vault.go`
  - `providers/`
    - `claudecode.go`
    - `openaicodex.go`
    - `openrouter.go`
    - `ollamacloud.go`
  - `types.go`
  - `*_test.go`

If you want to keep the broker in `agent-sandbox` despite the Mac runtime boundary, add it as a sibling Go package under `modules/` and treat deployment as manual in phase 1.

### On the agent-host side

Add these files or concepts:

- `modules/llm-proxy/provider_config.go`
  - loads declarative provider instance config
- `modules/llm-proxy/broker_client.go`
  - maintains authenticated control session to broker
- `modules/llm-proxy/session_cache.go`
  - memory-only session cache with TTL and eviction
- `modules/llm-proxy/providers.go`
  - provider-kind-specific upstream auth injection logic

### NixOS module changes

Add a new NixOS configuration surface, likely under a new module:

- `modules/agent-llm.nix`

This module should configure:

- provider instances
- broker endpoint
- broker enrollment key paths
- cache policy
- optional local upstream definitions for `ollama` / `vllm`

`modules/llm-proxy.nix` can either:
- stay focused on process/service wiring while consuming `services.agent-llm.*`
- or be merged into a broader `services.agent-llm` module

Recommended: keep `llm-proxy.nix` thin and introduce `agent-llm.nix` for the higher-level provider model.

## Provider configuration model

### Provider kinds

Fixed provider kinds:

- `claude-code`
- `openai-codex`
- `openrouter`
- `ollama`
- `openai-compatible`

### Provider instances

Each provider instance needs:

- `id`
- `kind`
- `route`
- `upstream`
- `credentialKind`
- `enabled`
- `policy`

### Credential kinds

- `oauth-session`
- `bearer-token`
- `no-secret`

### Example conceptual Nix shape

```nix
services.agent-llm = {
  enable = true;

  broker = {
    url = "https://broker.tailnet.example";
    enrollmentKeyPath = "/var/lib/agent-sandbox/broker/host_ed25519";
    caPath = "/var/lib/agent-sandbox/broker/ca.pem";
  };

  cache = {
    maxSessionTtl = "10m";
    refreshBeforeExpiry = "2m";
    allowProxyCache = true;
  };

  providers = {
    claude-code-main = {
      kind = "claude-code";
      route = "/claude-code-main";
      upstream = "https://claude-provider-placeholder";
      credentialKind = "bearer-token";
      enabled = true;
    };

    openai-codex-main = {
      kind = "openai-codex";
      route = "/openai-codex-main";
      upstream = "https://api.openai.com";
      credentialKind = "oauth-session";
      enabled = true;
    };

    openrouter-main = {
      kind = "openrouter";
      route = "/openrouter-main";
      upstream = "https://openrouter.ai/api/v1";
      credentialKind = "bearer-token";
      enabled = true;
    };

    ollama-cloud = {
      kind = "ollama";
      route = "/ollama-cloud";
      upstream = "https://ollama-cloud-placeholder";
      credentialKind = "bearer-token";
      enabled = true;
    };

    ollama-local = {
      kind = "ollama";
      route = "/ollama-local";
      upstream = "http://ndn.local:11434";
      credentialKind = "no-secret";
      enabled = true;
    };

    vllm-local = {
      kind = "openai-compatible";
      route = "/vllm-local";
      upstream = "http://ndn.local:8000/v1";
      credentialKind = "no-secret";
      enabled = true;
    };
  };
};
```

## Broker API

The MVP should use a simple RPC surface over a long-lived mutually authenticated channel.

### Session-oriented methods

- `AcquireSession`
- `RefreshSession`
- `RevokeSession`
- `ListAllowedProviders`

### Request shapes

`AcquireSession` request:

```json
{
  "host_id": "agent-host-01",
  "provider_instance": "openai-codex-main",
  "ttl_hint": "10m",
  "audience": "llm-proxy",
  "reason": "proxy-session"
}
```

`AcquireSession` response:

```json
{
  "session_id": "sess_123",
  "provider_instance": "openai-codex-main",
  "expires_at": "2026-06-16T18:30:00Z",
  "auth": {
    "kind": "bearer",
    "token": "...short-lived-or-delegated..."
  }
}
```

### Important constraint

Do not add a generic `GetSecret(provider)` method.

The broker API must remain capability-shaped and session-oriented.

## Enrollment model

### MVP

- On first boot, `agent-host` generates an Ed25519 keypair.
- The public key and host metadata are used for broker enrollment.
- Initial trust bootstrap can be permissive/manual.

### Stored state on agent-host

Store under a new root such as:

- `/var/lib/agent-sandbox/broker/host_ed25519`
- `/var/lib/agent-sandbox/broker/host_ed25519.pub`
- `/var/lib/agent-sandbox/broker/enrollment.json`

### Fast follow

- explicit approval command
- rotation
- revocation
- host status reporting

## Local vault backend interface

Broker-side storage should use one abstraction with pluggable backends.

### Interface responsibilities

- list provider records
- load encrypted provider record
- unlock provider record into memory
- update provider record
- optionally refresh stored OAuth material

### Backend order

1. `pass`
2. macOS Keychain integration
3. Linux Secret Service / libsecret

### Provider record schema

Common metadata:

- `providerInstance`
- `kind`
- `credentialKind`
- `label`
- `createdAt`
- `updatedAt`
- `expiresAt` if applicable

Secret payload varies by credential kind:

- `oauth-session`
  - refresh-capable session data
- `bearer-token`
  - token value plus metadata
- `no-secret`
  - no secret payload

## Proxy session cache

This is the remote equivalent of a local factotum working set.

### Requirements

- memory-only
- keyed by `providerInstance`
- TTL-aware
- renewable
- explicitly invalidatable
- never serialized to disk

### Suggested cache behavior

- cache hit allowed only if session is still inside policy TTL
- refresh when remaining lifetime is below configured threshold
- invalidate on upstream auth failure if broker says session is stale
- clear everything on process restart

## Provider-specific behavior

### `claude-code`

- credential kind: `bearer-token`
- broker stores the minted Claude Code token path/material locally
- proxy acquires short-lived delegated session material from broker
- workers do not receive the minted token directly

### `openai-codex`

- credential kind: `oauth-session`
- broker stores OAuth-backed material locally
- broker returns short-lived access material to proxy
- proxy caches renewable short-lived sessions

### `openrouter`

- credential kind: `bearer-token`
- proxy uses short-lived delegated bearer auth obtained from broker

### `ollama-cloud`

- credential kind: `bearer-token`
- same overall pattern as `openrouter`

### `ollama-local`

- credential kind: `no-secret`
- proxy still mediates access for policy and topology reasons
- no broker secret resolution required for the upstream call

### `vllm-local`

- credential kind: `no-secret`
- same as `ollama-local`, unless later wrapped with optional auth

## Worker-facing contract

Workers should receive only logical provider routing info.

Example env:

- `LLM_PROVIDER_INSTANCE=openai-codex-main`
- `LLM_BASE_URL=http://10.88.0.1:12071/openai-codex-main`

No worker should receive:

- provider secret material
- broker enrollment key
- broker endpoint auth details
- raw upstream provider credentials

## Implementation phases

### Phase A — config and proxy skeleton

- add `services.agent-llm` Nix module
- convert hardcoded proxy routes into declarative provider instances
- keep current static secret path working temporarily for compatibility

### Phase B — broker client and enrollment

- add host key generation
- add broker client in proxy
- add session cache
- add broker enrollment storage

### Phase C — first credentialed providers

- implement `claude-code`
- implement `openai-codex`

### Phase D — additional remote providers

- implement `openrouter`
- implement `ollama-cloud`

### Phase E — local provider instances

- implement `ollama-local`
- implement `vllm-local`

### Phase F — registration hardening

- explicit enrollment approval flow
- rotation and revocation
- richer audit/status surfaces

## Testing plan

### Unit tests

Broker:

- vault backend tests
- enrollment validation tests
- provider adapter tests
- session issuance / refresh / revoke tests
- policy rejection tests

Proxy:

- provider instance config parsing
- broker client retry/failure handling
- cache hit / miss / refresh behavior
- auth header injection by provider kind
- no-secret provider routing
- cache flush on restart semantics where testable

### Integration tests

- proxy starts with provider instance config and no worker secrets
- proxy can acquire `claude-code` delegated session from broker
- proxy can acquire `openai-codex` delegated session from broker
- worker can use proxy route without seeing secret material
- `ollama-local` and `vllm-local` routes function without broker secret resolution

### Security assertions

- no provider secret written to guest config or worker env
- no delegated session written to disk on `agent-host`
- no generic “dump secrets” broker API
- only enrolled host IDs can acquire provider sessions

## Non-goals for this sub-project

- full queue/orchestrator integration
- dynamic VM spawning
- worker policy lockdown inside guests beyond current substrate
- final capability-pure transport
- full Mac service packaging

## Expected repository changes

At minimum, this spec implies changes to:

- `modules/llm-proxy.nix`
- `modules/llm-proxy/main.go`
- `modules/llm-proxy/proxy.go`
- new `modules/agent-llm.nix`
- new proxy-side Go files for broker client / provider instances / cache
- new broker-side Go package or subtree
- `host/configuration.nix`
- potentially `guests/base.nix` only for worker env defaults later
