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
| `cnf: {kid, key}` | 1 | **client public-key thumbprint** — binds the grant to the client's own keypair (sender-constraint; enables per-request proofs / anti-replay) |
| footer `kid` | 1 | issuer key id, for zero-downtime rotation |
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

## Replay resistance — sender-constrained grants + per-request proof (DPoP-style)
**Security does NOT assume the transport** (no reliance on Tailscale/TLS for auth). A bearer grant,
once captured, is replayable regardless of TTL; the only transport-independent defense is **per-request
client signing** + a server replay check. So the grant is **sender-constrained**:

- The issuer mints the grant with a **`cnf`** claim = the thumbprint of the **client's own** Ed25519
  public key (the client keypair is enrolled at the issuer; its private key never leaves the client).
- For **every** RPC the client produces a short **proof** — a PASETO v4.public token signed by the
  client key over `{aud, method, nonce, iat}` — and sends it alongside the grant (gRPC metadata
  `laneq-grant` + `laneq-proof`).
- laneq verifies, in order: (1) the **grant** (issuer-signed; `verify_grant`: sig, `exp`/`nbf`, `aud`),
  (2) extract `cnf`, (3) the **proof** signature against the `cnf` key, (4) `proof.aud` == this laneq,
  (5) `proof.method` == the actual RPC method (so a proof for `Peek` can't be replayed as `Done`),
  (6) `proof.iat` within a small skew window (default ±30 s), (7) `proof.nonce` **unseen** in a
  TTL-bounded replay cache (sized to the skew window).

Result: a captured grant is useless without the client's private key; a captured proof is valid only for
~seconds, only for that exact method/target, and only once (the nonce cache rejects the second use).
No transport trust required.

## Transport confidentiality
Tailscale (WireGuard) or gRPC-TLS still provide **confidentiality** in transit and are recommended, but
auth/replay-resistance no longer **depend** on them. The replay defense above holds even on a fully
observed/hostile channel. The remaining residual risk is a within-skew-window replay of the *same*
method by an on-path attacker who wins the race before the nonce is cached — bounded to seconds and a
single use; tighten the skew window to reduce it further.

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
- **laneq (Python) unit:** accept valid grant+proof; reject expired/bad-aud/bad-sig/bad-kid grant;
  **reject proof not signed by `cnf`, wrong-method proof, stale-iat proof, and a replayed nonce** (the
  anti-replay core); Phase 2 capability + lane enforcement + the sole-writer scenario (non-Temporal grant
  `Defer`/`Reprioritize` → `PERMISSION_DENIED`; Temporal-role grant succeeds).
- **Behavior scenarios:** `host-signed RPC accepted`; `forged/expired grant rejected`; **`replayed request
  rejected (nonce reuse)`**; **`captured grant without client key is useless (no valid proof)`**;
  `log-only allows + logs`; (Phase 2) `sole-writer capability enforced`.

## Phasing
- **Phase 1 (this spec):** token format + Mac issuer CLI + key distribution + Go client attach/renew +
  laneq interceptor (`log-only` → `enforce`), host-level `sub`/`aud`/`exp` only.
- **Phase 2 (next spec):** `cap` claims; per-op/per-lane enforcement; sole-writer enforcement; ties into
  ITER-0008 multi-consumer delegation.
- **Beyond:** the full credential broker (provider creds) remains `2026-06-16-credential-broker-architecture.md`.

## Out of scope (Phase 1)
Provider-credential brokering; capability/lane enforcement; revocation lists (jti is recorded, not yet
checked); gRPC-TLS (Tailscale covers transport); operator approval UI.
