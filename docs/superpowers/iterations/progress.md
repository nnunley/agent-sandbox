# Progress

**Phase:** **ITER-0007c — formal `running-an-iteration` (resumed 2026-06-25).** Sentinel baseline CLEAN; citation OK;
**PAR scope review APPROVED** (after one REVISE cycle — STORY-0082 AC-1 split AC-1a/AC-1b, mandatory Go real-wire
evidence, ITER-0008 per-role-issuer precondition documented). **Task progress:** T1 GrantSource **DONE** (`e0f4a5d`,
in two-stage PAR now); T2 client interceptor+wiring, T3 real-wire e2e evidence, T4 issuer CLI, T5 corpus — pending.
**Gated handoff:** STORY-0082 AC-1b (external laneq PR + live log-only→enforce rollout) deferred for operator authorization.
**⚠ Incident (2026-06-25):** the T1 implementer subagent cleaned the working tree before committing, wiping all
UNCOMMITTED changes — re-applied my scope edits (now committed); the pre-existing uncommitted edit to
`specs/2026-06-24-laneq-grant-paseto-design.md` (dirty at session start) was lost (no stash/reflog/blob). Mitigation:
artifact edits are committed before each implementer dispatch; implementers restricted to `git add <their files>` only.
ITER-0007b COMPLETE (done:2026-06-24). ITER-0008 pending (capstone).
**Iterations:** 11/12 base done; **ITER-0007c active**.

**⚠ Process note (ITER-0007c):** implemented via **direct hands-on TDD during an interactive design session**, NOT
the formal `running-an-iteration` PAR loop (no per-task scope/spec/quality PAR like ITER-0007b). Code is tested +
cross-language-interop-proven but has NOT had per-task adversarial review — a `requesting-code-review` / PAR pass is
owed before the laneq PR + before the eventual ITER-0007c audit. Also: ITER-0007b's own `auditing-progress`
(three-tier) was never run — still owed.

**Design change mid-flight (2026-06-24):** user directed "no Tailscale assumption; make replay impossible/harder",
so the grant became **sender-constrained (DPoP-style)**: grant carries `cnf`=client-key thumbprint; client signs a
per-request **proof** over {aud, method, nonce, iat}; laneq verifies proof vs cnf + freshness window + **nonce
replay cache**. Design doc updated; EPIC-014 stories/scenarios updated to match.

**ITER-0007c done so far (committed; gates green both repos):**
- **laneq (Python)** — branch `nnunley/laneq:paseto-auth` off `selamy-labs/laneq:main` (PR #19 merged), PR-ready:
  `auth.verify_grant` (`b711ce5`), `auth.verify_proof`+`ReplayCache` (`f0b1ce9`), `grpc_auth.GrantAuthInterceptor`
  (off/log-only/enforce) + `serve()` wiring + `build_interceptor_from_env` (`d8b5cbe`). 37 tests; all 4 laneq CI
  gates green (ruff format/check, pytest, coverage 96% ≥95). Clone at `/Users/ndn/development/laneq`.
- **agent-sandbox (Go)** — `main`: `grantauth` signing core (issuer `MintGrant`, client `SignProof`, PEM) `e99a28b`.
  Vet clean; module suite green; go-paseto v1.6.0 added.
- **🎯 Cross-language interop PROVEN:** Go-minted grant + Go-signed proof verify in laneq `pyseto` (cnf-bound,
  method-bound, int-timestamps) — the riskiest contract risk, de-risked by a real round-trip.

**ITER-0007c remaining:** (1) Go gRPC **client interceptor** (per-call nonce+proof+metadata) + `GrantSource` +
`serve_cmd.go` wiring; (2) Mac **issuer CLI** (`laneq-grant`); (3) **local end-to-end** proof (laneq enforce +
issuer key → Go client authenticates, unauth/replayed rejected) — extend `run-laneq-wire.sh`; (4) corpus exec
commands wired to tests; (5) PAR/code-review pass; (6) laneq PR + live `log-only→enforce` rollout. Phase 2
(per-op/lane capabilities + sole-writer enforcement at laneq) = separate spec.

**Sentinel corpus:** `go vet ./...` clean; `go test -race ./...` **429 green** (387→429, +42 across C2–C5);
tree clean; zero `TODO(ITER-0007b)` markers (re-tagged → ITER-0008). Live tests env-gated (`TEMPORAL_LIVE=1`),
excluded from default CI.

**ITER-0007b delivered (all code tasks two-stage PAR; E1 orchestrator-executed & verified):**
- **C2** — PriorityWorkflow + sole-writer ReprojectActivity (Reprioritize+Defer) + lease-free `LaneqQueue.Defer`. `4bc32ae`.
- **C3** — rescore signal (human-unrestricted / agent-bounded) + `currentImportance` query handler. `958f6ca`.
- **C4** — RetryWorkflow (exp backoff re-push) + EscalationWorkflow (time-driven stale re-raise) + DeferWorkflow
  (hold-until-eligible), all via the sole-writer seam; `nextCheckInterval` helper; dead `ReprojectRequest` fields removed. `e58ea72`.
- **C5** — concurrent-read consistency, both guarded fields, `-race`. `401abc0`.
- **E1** — worker-driven live cluster harness (`temporal/temporal_live_test.go` + `run-temporal-live.sh`, gated).
  **SCENARIO-0094 LIVE-PROVEN** (human rescore flips laneq P1→P0, real worker executing). **SCENARIO-0001 LIVE-PROVEN**
  (DeferWorkflow survives a REAL Temporal restart PID 6976→7066; same runID Running→Completed; directive fired —
  STORY-0001 AC-2 durable timer, not laneq natural expiry). 0081/0093 live = honest (concurrent Peek / process-level
  sole-caller; value-consistency & sole-writer enforcement CI-PROVEN in C5/C2). Latest E1 commit `c55a35d`.

**ITER-0008 GATE: MET (no carries).** STORY-0041 AC-1/AC-2 (live sole writer) + STORY-0044 AC-3 (live sole caller).

**ITER-0008 design notes (recorded):** PriorityWorkflow early-exit-at-Q1 → post-Q1 operator pause/block/resume
needs a separate control plane; live wall-clock Q2→Q1 not compressible to seconds (urgency calibrated in days —
needs a knob or multi-day runner; logic is CI-PROVEN); rejected agent rescores → operator approval queue;
laneq `Stats()/Len()` observability deferred.

**Last event:** 2026-06-25 — ITER-0007c: laneq verification side + Go signing core committed; cross-language interop
proven. **On resume:** continue ITER-0007c remaining (Go client interceptor → local e2e → issuer CLI → PAR/review →
laneq PR). Still owed from before: ITER-0007b `auditing-progress` (three-tier).
