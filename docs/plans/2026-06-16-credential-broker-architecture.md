# Credential Broker Architecture for agent-sandbox

**Date:** 2026-06-16
**Status:** Draft notes
**Scope:** High-level design for brokered provider credentials in `agent-sandbox`

## Goal

Extend `agent-sandbox` so remote workers never receive raw long-lived secrets, while still supporting the provider paths I actively use:

- `claude-code`
- `openai-codex`
- `openrouter`
- `ollama-cloud`
- `ollama-local`
- `vllm-local`

The system should preserve the useful shape of `secstore + factotum`:

- one durable encrypted trust root
- one in-memory credential broker
- one worker-facing capability surface
- no raw secret exposure to sandboxed workers

## Security model

### Hard invariants

- The master secret / root unlock path stays on the Mac.
- Remote `agent-host` must never receive the master key.
- Remote workers must never receive raw provider secrets.
- Workers only talk to the local proxy surface on `agent-host`.
- Only the remote proxy may hold delegated provider auth material, and only in memory.
- Delegated provider auth is short-lived and renewable.
- Delegated provider auth is policy-scoped by provider instance, remote host, and TTL.
- Remote caches are flushed on proxy restart.

### Trust layers

1. **Local trust root on Mac**
   - encrypted local vault
   - master unlock path
   - broker issuer root
2. **Independent host enrollment**
   - not tied to Tailscale identity
   - `agent-host` generates its own host keypair on first boot
   - broker recognizes enrolled hosts using broker-specific credentials
3. **Network transport**
   - Tailscale provides private connectivity and encrypted transport only
4. **Delegated provider sessions**
   - issued by broker
   - cached only at the remote proxy
5. **Worker-facing capability surface**
   - proxy-only
   - no direct credential broker access from workers

## secstore / factotum mapping

Modernized mapping:

- **secstore** -> encrypted local vault on the Mac
- **factotum** -> broker on the Mac plus a short-lived operational cache in the remote proxy
- **keyring/tool surface** -> the remote LLM proxy and any future operator/admin tools

Important behavioral translation:

- durable encrypted store is centralized
- decrypted working set is ephemeral and memory-only
- consumers operate on capability, not raw secret material

## Deployment topology

### Current substrate

- Mac orchestrator / trust root
- remote x86_64 Incus host
- `agent-host` NixOS container on Incus
- Firecracker microVM workers inside `agent-host`
- LLM reverse proxy already present inside `agent-host`

### Proposed topology

```text
Mac
  brokerd
    - local vault backend
    - provider credential/session logic
    - host enrollment registry
    - delegated session issuance
  |
  |  mutually authenticated control channel over Tailscale
  v
agent-host (NixOS container on Incus)
  llm-proxy
    - worker-facing provider routes
    - broker client
    - short-lived in-memory session cache
  |
  +-- Firecracker workers / future container workers
        - no raw secrets
        - provider access only through llm-proxy
```

## Connection model

### MVP transport

Keep transport simple first:

- a mutually authenticated long-lived control channel between `agent-host` and the Mac broker
- Tailscale carries the transport
- broker trust is independent of Tailscale identity

### Desired future direction

Transport can later become more capability-pure:

- pre-opened capability channels
- forwarded sockets / vsock / fd-like semantics
- same broker semantics, different transport

The interface should therefore be capability-shaped from the start, even if the first transport is plain RPC.

## Provider model

### Provider kinds

Use a small fixed set of semantic provider kinds:

- `claude-code`
- `openai-codex`
- `openrouter`
- `ollama`
- `openai-compatible`

### Provider instances

Instances carry endpoint, auth, and policy:

- `claude-code-main`
- `openai-codex-main`
- `openrouter-main`
- `ollama-cloud`
- `ollama-local`
- `vllm-local`

### Credential kinds

Three credential families cover the current use cases:

1. `oauth-session`
   - `openai-codex`
2. `bearer-token`
   - `claude-code`
   - `openrouter`
   - `ollama-cloud`
3. `no-secret`
   - `ollama-local`
   - `vllm-local`

## Local vault design

The local vault should support multiple storage backends behind one broker API.

### Initial backend strategy

- macOS: Keychain-backed unlock path and/or secure local storage
- portable fallback: `pass`
- Linux later: `pass` and/or Secret Service / libsecret equivalent

### Vault properties

- a master unlock path gates access to all provider records
- provider records are encrypted at rest
- decrypted records live only in broker memory
- provider records are separated by instance

Example conceptual records:

- `providers/openai-codex-main/oauth`
- `providers/claude-code-main/token`
- `providers/openrouter-main/api-key`
- `providers/ollama-cloud/api-key`

## Host enrollment

### MVP

- `agent-host` auto-generates a host keypair on first boot
- broker stores enrollment state for that host
- this enrollment is independent of Tailscale machine identity

### Fast follow

- explicit registration / approval flow
- revocation
- rotation
- re-enrollment after rebuild or compromise

## Broker API semantics

Do not expose a “give me the secret database” style API.

Use capability-shaped broker methods instead.

### Core methods

- `AcquireSession(providerInstance, hostId, ttlHint, audience)`
- `RefreshSession(sessionId)`
- `RevokeSession(sessionId)`
- `ListAllowedProviders(hostId)`

### Provider-specific adapters behind the API

- `claude-code`
  - broker returns short-lived usable token/session material derived from the stored minted token path
- `openai-codex`
  - broker returns short-lived access material from stored OAuth-backed credentials
- `openrouter` / `ollama-cloud`
  - broker returns short-lived delegated bearer capability or directly supplies request auth data to proxy
- `ollama-local` / `vllm-local`
  - broker may be bypassed for raw credentials, but policy still flows through provider instance configuration

## Remote proxy behavior

The remote proxy is the only host-side service allowed to hold delegated provider auth.

### Proxy responsibilities

- expose worker-facing provider routes
- authenticate itself to broker
- acquire delegated provider sessions from broker
- cache delegated provider sessions in memory
- inject provider auth into upstream calls
- flush cache on restart
- never write delegated auth to disk

### Cache policy

Remote proxy cache policy should be a configurable high-level knob, not hardcoded.

Suggested policy fields:

- `maxSessionTtl`
- `refreshBeforeExpiry`
- `allowProxyCache`
- `audience`
- `allowedHosts`
- `revocationMode`

Recommended default shape:

- memory-only cache
- short TTL
- renewable
- host-bound audience
- immediate loss on restart

This is intentionally LOAS-like in spirit:

- strong workload identity
- centralized issuance
- short-lived delegated authority
- local ephemeral cache
- no widespread distribution of long-lived secrets

## Worker contract

Workers receive only provider route information, never real upstream credentials.

Conceptual worker env:

- `LLM_PROVIDER_INSTANCE=openai-codex-main`
- `LLM_BASE_URL=http://10.88.0.1:12071/openai-codex-main`

or:

- `LLM_PROVIDER_INSTANCE=ollama-local`
- `LLM_BASE_URL=http://10.88.0.1:12071/ollama-local`

Workers should not know:

- upstream provider secrets
- broker addresses
- broker enrollment material
- raw provider endpoints unless explicitly necessary for diagnostics

## Routing shape

The current proxy is hardcoded and should evolve toward declarative provider instances.

Conceptual route patterns:

- `/claude-code-main/*`
- `/openai-codex-main/*`
- `/openrouter-main/*`
- `/ollama-cloud/*`
- `/ollama-local/*`
- `/vllm-local/*`

## Local GPU and remote GPU support

The design should support both:

- external GPU box such as `ndn.local`
- dedicated Incus GPU instance with one or more GPUs attached

This is an instance concern, not a provider-kind concern.

Examples:

- `ollama-local` -> `http://ndn.local:11434`
- `vllm-local` -> `http://ndn.local:8000/v1`
- future `ollama-gpu-incus` -> dedicated NixOS GPU appliance on Incus

## Phasing

This should be delivered in phases even though the spec covers the full provider set.

### Phase 1

- broker skeleton on Mac
- host enrollment key generation
- remote proxy broker client
- provider instance schema
- proxy-only delegated auth invariant enforced

### Phase 2

- `claude-code`
- `openai-codex`

### Phase 3

- `openrouter`
- `ollama-cloud`

### Phase 4

- `ollama-local`
- `vllm-local`

### Phase 5

- explicit registration / approval flow
- rotation and revocation
- more capability-pure transport

## Why this belongs in agent-sandbox

`agent-sandbox` already has the right structural anchors:

- NixOS host/container control point
- Firecracker guest substrate
- existing LLM proxy choke point
- credential-isolating architecture intent

What is missing today is:

- a Mac-hosted broker
- independent host enrollment
- provider-instance abstraction
- delegated session cache semantics
- removal of long-lived host-side static provider credentials from proxy startup config

That makes `agent-sandbox` the correct place to implement this architecture.
