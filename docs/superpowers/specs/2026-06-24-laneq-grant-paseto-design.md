# laneq Grant Signing (PASETO host-to-host auth) — Design

**Date:** 2026-06-24
**Status:** Approved (brainstorming) — Phase 1 spec
**Scope:** Sign and verify laneq gRPC calls with PASETO grants so laneq is safe across
non-local (Tailscale) networks. Phase 1 = host-level signing end-to-end. Phase 2 (separate
spec) = per-consumer/per-operation/per-lane capabilities, incl. enforcing the sole-writer rule.
**Relates to:** `docs/plans/2026-06-16-credential-broker-architecture.md` (this seeds its `brokerd`
trust root), and the ITER-0007b sole-writer seam (Phase 2 upgrades it from process discipline to
enforced authz).

## Problem

laneq is a gRPC server (`nnunley/laneq`) currently reachable with **no authentication** — fine on the
trusted `10.88.0.0/24` bridge, unsafe the moment its RPC crosses non-local networks. We need
host-to-host signing so only enrolled, granted callers can issue laneq RPCs, with the trust root
(signing key + service passwords) staying on the user's Mac. laneq is the user's own project, and its
maintainers want a "security story," so verification + the grant mechanism live **inside laneq**.

## Hard invariants

- The Ed25519 **private** (issuer) key never leaves the Mac. laneq and cluster hosts hold only public key(s).
- Every laneq RPC across a non-local network carries a valid PASETO grant; laneq verifies before serving.
- Grants are short-lived and renewable; audience-bound to a specific laneq instance.
- Rollout never breaks the live cluster: laneq verifies in `log-only` before `enforce`.

## The grant: PASETO v4.public (Ed25519)

Asymmetric (`v4.public`) so the remote side holds only the public key. Claims:

| Claim | Phase | Meaning |
|---|---|---|
| `iss` | 1 | Mac issuer id |
| `sub` | 1 | caller identity — Phase 1: host (`agent-host`); Phase 2: role (`temporal-writer`, `daemon-consumer`) |
| `aud` | 1 | target laneq instance, e.g. `laneq://agent-host:9999` — prevents cross-target replay |
| `iat`/`nbf`/`exp` | 1 | short TTL (default ~30m), renewable |
| `jti` | 1 | token id (audit / optional revocation) |
| footer `kid` | 1 | key id, for zero-downtime rotation |
| `cap: {ops:[...], lanes:[...]}` | 2 | per-operation/lane capability; the Temporal-role grant is the ONLY one with `defer`/`reprioritize` |

## Components

### 1. Mac issuer — `laneq-grant` (trust-root seed)
Go CLI/daemon on the Mac holding the Ed25519 private key (macOS Keychain / secstore vault). Mints
tokens: `laneq-grant mint --sub agent-host --aud laneq://agent-host:9999 --ttl 30m`. Phase 1 = CLI +
a renewal helper; deliberately the kernel of the broker doc's `brokerd`. Private key generated here,
never exported.

### 2. Go client — `agent-sandbox`
- `GrantSource` interface + default impl: loads the current token from a configured path, caches it in
  memory, and reloads when the file changes / before `exp`.
- **Phase 1 token delivery (explicit):** the Mac issuer mints the token and **pushes** it to `agent-host`
  via the existing Incus systemd-credential / file mechanism already used for API keys (CLAUDE.md:
  `incus config set … systemd.credential.…`). A Mac-side renewal helper (launchd/cron) re-mints and
  re-pushes before expiry; with the issuer on the Mac, Phase 1 may use a longer TTL (e.g. a few hours)
  to keep renewal cadence low. A laneq-side **pull/fetch RPC** to the broker is deferred to Phase 2.
- `LaneqQueue` attaches it via a gRPC unary+stream **client interceptor** (per-RPC credentials),
  metadata `authorization: Bearer v4.public.…`. Purely additive to `queue/laneq.go`; an absent
  `GrantSource` == today's behavior (nothing breaks pre-rollout).

### 3. laneq interceptor — Python, `nnunley/laneq`
gRPC `ServerInterceptor`: extract token → verify `v4.public` signature against configured public
key(s) → check `exp`/`nbf`/`aud` → (Phase 2) enforce `cap` against the RPC method + lane. Reject with
`UNAUTHENTICATED` (Phase 2: `PERMISSION_DENIED`). Config: public key set (with `kid`), expected
audience, and **enforcement mode `off | log-only | enforce`**.

## Key management & rotation
Ed25519 keypair on the Mac; public key(s) distributed to laneq via Nix config / systemd credential
(no secret in the repo). `kid` in the footer lets laneq trust **current + next** keys → zero-downtime
rotation.

## Transport confidentiality
Across non-local networks, Tailscale (WireGuard) provides encryption-in-transit (per the broker doc);
PASETO provides authn/authz + integrity + replay-resistance (short TTL + `aud` binding + optional
`jti`). gRPC-TLS may layer on later. **Residual risk:** bearer replay *within* the TTL on an
already-compromised Tailscale path — mitigated by short TTL + audience binding.

## Rollout (safe against the live cluster)
laneq is live (ITER-0007b deploys against it):
1. Ship interceptor in **`log-only`** (verify + log failures, allow all).
2. Confirm the Go client attaches valid tokens; nothing legitimate is rejected.
3. Flip to **`enforce`**.
The Go `GrantSource` ships dark (off) until issuer + keys are in place.

## Testing & behavior evidence
- **Go unit:** client interceptor attaches token; `GrantSource` cache + renew-before-expiry; absent grant = legacy passthrough.
- **Go ↔ laneq real-wire** (extend `queue/run-laneq-wire.sh` + `laneq_realwire_lifecycle_test.go`):
  valid signed token round-trips all RPCs; expired / wrong-`aud` / forged-sig rejected (`UNAUTHENTICATED`);
  `log-only` mode logs-but-allows a bad token (proves the rollout gate).
- **laneq (Python) unit:** accept valid; reject expired/bad-aud/bad-sig/bad-kid; Phase 2 capability + lane
  enforcement + the sole-writer scenario (non-Temporal grant `Defer`/`Reprioritize` → `PERMISSION_DENIED`;
  Temporal-role grant succeeds).
- **Behavior scenarios:** `host-signed RPC accepted`; `forged/expired rejected`; `log-only allows + logs`;
  (Phase 2) `sole-writer capability enforced`.

## Phasing
- **Phase 1 (this spec):** token format + Mac issuer CLI + key distribution + Go client attach/renew +
  laneq interceptor (`log-only` → `enforce`), host-level `sub`/`aud`/`exp` only.
- **Phase 2 (next spec):** `cap` claims; per-op/per-lane enforcement; sole-writer enforcement; ties into
  ITER-0008 multi-consumer delegation.
- **Beyond:** the full credential broker (provider creds) remains `2026-06-16-credential-broker-architecture.md`.

## Out of scope (Phase 1)
Provider-credential brokering; capability/lane enforcement; revocation lists (jti is recorded, not yet
checked); gRPC-TLS (Tailscale covers transport); operator approval UI.
