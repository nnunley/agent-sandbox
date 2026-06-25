# EPIC-014 — laneq grant signing (PASETO host-to-host auth)

**Summary:** Sign and verify laneq gRPC calls with PASETO grants so laneq is safe across non-local (Tailscale) networks; grant mechanism lives inside laneq. Phase 1 = host-level signing; Phase 2 (deferred) = per-consumer/op/lane capabilities + sole-writer enforcement.
**Stories:** STORY-0079, STORY-0080, STORY-0081, STORY-0082
**Primary sources:** `docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md`, `docs/plans/2026-06-16-credential-broker-architecture.md`
**Status:** pending (spec approved 2026-06-24; target ITER-0007c). Phase 2 capability/sole-writer enforcement → separate spec/iteration.

## STORY-0079

**Epic:** EPIC-014 — laneq grant signing (PASETO host-to-host auth)
**Title:** PASETO v4.public grant token format + Mac issuer

**As a** cluster trust root on the user's Mac
**I want** to mint short-lived PASETO v4.public (Ed25519) grant tokens for laneq callers, holding the private key only on the Mac
**So that** laneq callers can present a signed, audience-bound, expiring grant without any shared secret leaving the Mac

**Acceptance criteria:**
- AC-1: Grant token format defined (claims `iss`/`sub`/`aud`/`iat`/`nbf`/`exp`/`jti` + footer `kid`); Go encode + Ed25519 sign and decode + verify round-trip; tampered token fails verification · impact:`local` · seam:`unit` · scenario:`SCENARIO-0120`
- AC-2: Mac issuer `laneq-grant mint --sub <id> --aud laneq://<host>:<port> --ttl <dur>` produces a valid token; Ed25519 private key is generated and held on the Mac (Keychain / secstore vault) and is never exported · impact:`local` · seam:`integration` · scenario:`SCENARIO-0117`
- AC-3: Key rotation supported — issuer can mint under a new `kid`; the format carries `kid` in the footer so verifiers can trust current + next public keys · impact:`local` · seam:`unit` · scenario:`SCENARIO-0120`

**Sources:**
- `docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md:24-40`

**Status:** pending (ITER-0007c). Trust root seeds the `brokerd` of `2026-06-16-credential-broker-architecture.md`.

## STORY-0080

**Epic:** EPIC-014 — laneq grant signing (PASETO host-to-host auth)
**Title:** Go client grant attachment to laneq RPCs

**As a** laneq Go consumer (daemon / Temporal worker)
**I want** to attach my current PASETO grant to every laneq gRPC call and renew it before expiry
**So that** my RPCs are authenticated across non-local networks without changing the laneq RPC shapes

**Acceptance criteria:**
- AC-1: `GrantSource` interface + file-backed implementation loads the current token, caches it in memory, and reloads on file change / before `exp` · impact:`local` · seam:`unit` · scenario:`SCENARIO-0117`
- AC-2: `LaneqQueue` attaches the grant via a gRPC unary+stream client interceptor (metadata `authorization: Bearer v4.public…`); an absent/nil `GrantSource` preserves today's behavior (no RPC-shape change, nothing breaks pre-rollout) · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0117`

**Sources:**
- `docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md:48-58`

**Status:** pending (ITER-0007c). Additive to `modules/incus-dispatcher/queue/laneq.go`.

## STORY-0081

**Epic:** EPIC-014 — laneq grant signing (PASETO host-to-host auth)
**Title:** laneq server-side grant verification (Python gRPC interceptor)

**As a** laneq gRPC server (`nnunley/laneq`)
**I want** to verify the PASETO grant on every RPC and reject invalid ones, with a configurable enforcement mode
**So that** only granted callers can issue laneq RPCs and the security story is addressed inside laneq

**Acceptance criteria:**
- AC-1: gRPC `ServerInterceptor` extracts the grant, verifies the v4.public signature against the configured public key(s), checks `exp`/`nbf`/`aud`; rejects an invalid/expired/wrong-audience/forged grant with `UNAUTHENTICATED` · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0118`
- AC-2: Enforcement mode `off | log-only | enforce` is configurable; in `log-only` the interceptor verifies and logs failures but ALLOWS the RPC (safe rollout against the live cluster) · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0119`
- AC-3: Public key set is keyed by `kid`; the verifier trusts current + next keys for zero-downtime rotation · impact:`local` · seam:`integration` · scenario:`SCENARIO-0120`

**Sources:**
- `docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md:60-66`

**Status:** pending (ITER-0007c). Cross-repo: implemented in `nnunley/laneq`; format defined in agent-sandbox first.

## STORY-0082

**Epic:** EPIC-014 — laneq grant signing (PASETO host-to-host auth)
**Title:** Token delivery, rollout & transport

**As a** cluster operator
**I want** the grant pushed to agent-host and rolled out log-only→enforce without disrupting the live cluster
**So that** laneq becomes authenticated across non-local networks safely, with confidentiality carried by Tailscale

**Acceptance criteria:**
- AC-1: The Mac issuer pushes the minted token to `agent-host` via the Incus systemd-credential / file mechanism; a Mac-side renewal helper re-mints + re-pushes before expiry; the cluster transitions `log-only` → `enforce` with no legitimate RPC rejected · impact:`journey` · seam:`e2e` · scenario:`SCENARIO-0119`
- AC-2: Transport confidentiality across non-local networks is provided by Tailscale (WireGuard); the residual bearer-replay-within-TTL risk is documented and bounded by short TTL + `aud` binding · impact:`none` · seam:`integration`

**Sources:**
- `docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md:67-79`

**Status:** pending (ITER-0007c). gRPC-TLS and a pull/fetch broker RPC are deferred (Phase 2 / broker doc).
