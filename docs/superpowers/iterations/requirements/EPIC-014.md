# EPIC-014 — laneq grant signing (PASETO host-to-host auth)

**Summary:** Sign and verify laneq gRPC calls with PASETO grants so laneq is safe across **non-local, untrusted** networks (transport NOT assumed); grant verification lives inside laneq. Design is **sender-constrained / DPoP-style** (2026-06-24 user directive): the grant carries `cnf`=client-key thumbprint, the client signs a per-request **proof** {aud, method, nonce, iat}, laneq verifies proof-vs-cnf + freshness window + **nonce replay cache**. Phase 1 = host-level signing; Phase 2 (deferred) = per-consumer/op/lane capabilities + sole-writer enforcement.
**Stories:** STORY-0079, STORY-0080, STORY-0081, STORY-0082
**Primary sources:** `docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md`, `docs/plans/2026-06-16-credential-broker-architecture.md`
**Status:** IN PROGRESS:ITER-0007c (hands-on TDD, not the formal PAR loop — code-review owed). **laneq verification side DONE** (verify_grant + verify_proof + ReplayCache + interceptor + serve wiring; 37 tests, gates green; branch `nnunley/laneq:paseto-auth`). **Go signing core DONE** (`grantauth` MintGrant/SignProof; cross-language interop with pyseto PROVEN). Remaining: Go client interceptor + GrantSource + wiring, issuer CLI, local e2e, PR + rollout. Phase 2 → separate spec.

## STORY-0079

**Epic:** EPIC-014 — laneq grant signing (PASETO host-to-host auth)
**Title:** PASETO v4.public grant token format + Mac issuer

**As a** cluster trust root on the user's Mac
**I want** to mint short-lived PASETO v4.public (Ed25519) grant tokens for laneq callers, holding the private key only on the Mac
**So that** laneq callers can present a signed, audience-bound, expiring grant without any shared secret leaving the Mac

**Acceptance criteria:**
- AC-1: Grant token format defined (claims `iss`/`sub`/`aud`/`iat`/`nbf`/`exp`/`jti` + **`cnf`=client-key thumbprint (sender-constraint)** + footer `kid`; unix-int timestamps for cross-impl parity); Go encode + Ed25519 sign + cross-language verify round-trip (pyseto); tampered token fails verification · impact:`local` · seam:`unit` · scenario:`SCENARIO-0120`
- AC-2: Mac issuer `laneq-grant mint --sub <id> --aud laneq://<host>:<port> --ttl <dur>` produces a valid grant binding the client key into `cnf`; Ed25519 private key is generated and held on the Mac (Keychain / secstore vault) and is never exported · impact:`local` · seam:`integration` · scenario:`SCENARIO-0117`
- AC-3: Key rotation supported — issuer can mint under a new `kid`; the format carries `kid` in the footer so verifiers can trust current + next public keys · impact:`local` · seam:`unit` · scenario:`SCENARIO-0120`

**Sources:**
- `docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md:24-46`

**Status:** partial:ITER-0007c — **AC-1 done** (Go `grantauth.MintGrant` mints the cnf-bound grant; interop with pyseto PROVEN, `e99a28b`); AC-2/AC-3 = issuer **CLI** (`laneq-grant`) + Keychain/rotation **pending**. Trust root seeds the `brokerd` of `2026-06-16-credential-broker-architecture.md`.

## STORY-0080

**Epic:** EPIC-014 — laneq grant signing (PASETO host-to-host auth)
**Title:** Go client grant attachment to laneq RPCs

**As a** laneq Go consumer (daemon / Temporal worker)
**I want** to attach my current PASETO grant to every laneq gRPC call and renew it before expiry
**So that** my RPCs are authenticated across non-local networks without changing the laneq RPC shapes

**Acceptance criteria:**
- AC-1: Client holds its own Ed25519 keypair and signs a per-request **proof** (PASETO v4.public over {aud, method, nonce, iat}) — sender-constraint so a captured grant is useless · impact:`local` · seam:`unit` · scenario:`SCENARIO-0117`
- AC-2: `GrantSource` interface + file-backed implementation loads the current grant, caches it in memory, reloads on file change / before `exp` · impact:`local` · seam:`unit` · scenario:`SCENARIO-0117`
- AC-3: `LaneqQueue` attaches grant+proof via a gRPC unary client interceptor (metadata `laneq-grant` + `laneq-proof`, proof bound to the actual method); an absent/nil grant source preserves today's behavior (no RPC-shape change, nothing breaks pre-rollout) · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0117`

**Sources:**
- `docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md:48-66`

**Status:** partial:ITER-0007c — **AC-1 done** (Go `grantauth.SignProof`, `e99a28b`); AC-2 (`GrantSource`) + AC-3 (client interceptor + `serve_cmd.go` wiring) **pending**. Additive to `modules/incus-dispatcher/queue/laneq.go` + `grantauth`.

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
- AC-4: **Per-request proof verification (anti-replay):** the interceptor extracts `cnf` from the grant, verifies the per-request proof signed by that client key, binds it to the actual method + a freshness (skew) window, and rejects a reused nonce via a TTL replay cache — so neither a captured grant nor a captured proof is replayable · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0118`

**Sources:**
- `docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md:48-79`

**Status:** **done:ITER-0007c (laneq verification side)** — AC-1..AC-4 implemented in `nnunley/laneq:paseto-auth`: `auth.verify_grant`, `auth.verify_proof`+`ReplayCache`, `grpc_auth.GrantAuthInterceptor` (off/log-only/enforce) + `serve()` wiring + `build_interceptor_from_env`. 37 tests; ruff format/check + pytest + coverage(96%) green. Live `enforce` rollout on the cluster → STORY-0082. (Implemented via hands-on TDD; PAR/code-review owed before the laneq PR.)

## STORY-0082

**Epic:** EPIC-014 — laneq grant signing (PASETO host-to-host auth)
**Title:** Token delivery, rollout & transport

**As a** cluster operator
**I want** the grant pushed to agent-host and rolled out log-only→enforce without disrupting the live cluster
**So that** laneq becomes authenticated across non-local networks safely, with confidentiality carried by Tailscale

**Acceptance criteria:**
- AC-1: The Mac issuer pushes the minted token to `agent-host` via the Incus systemd-credential / file mechanism; a Mac-side renewal helper re-mints + re-pushes before expiry; the cluster transitions `log-only` → `enforce` with no legitimate RPC rejected · impact:`journey` · seam:`e2e` · scenario:`SCENARIO-0119`
- AC-2: Transport confidentiality (Tailscale/WireGuard or gRPC-TLS) is recommended but auth/replay-resistance does NOT depend on it (sender-constrained proof + nonce cache hold on an observed channel); the residual within-skew-window same-method race is documented and bounded by the freshness window + nonce cache · impact:`none` · seam:`integration`

**Sources:**
- `docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md:67-79`

**Status:** pending (ITER-0007c). gRPC-TLS and a pull/fetch broker RPC are deferred (Phase 2 / broker doc).
