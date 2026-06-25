# Progress

**Phase:** **ITER-0008 core COMPLETE (13/13, two-stage PAR per task) — 2026-06-25. NEXT: per-sprint auditing-progress.**
All 13 TDD tasks delivered + wrapped (stories done:ITER-0008, roadmap done, iteration-log validated, commit 9db07a7).
Sentinels JOURNEY-0001+0002 green; full -race 514; zero core TODO(ITER-0008). The two-stage adversarial gate caught
+ fixed a real defect on essentially every production task (T1c substrate-tautology, T2a immutability hole, T2b
constant-RunID, T2c ref-collision, T3b/d toothless depth-limit, T3c artifact-linkage, T4 non-assertion+end-to-end,
T5 tautological-replay+unwired, T6 unexercised-handoff). NOW: run `auditing-progress` three-tier PAR on the core →
if clean, ITER-0008b (operator UX/governance + Mac-off capstone, closes JOURNEY-0004..0007).

**ITER-0008 core IMPLEMENTING (subagent-driven TDD, two-stage PAR per task) — 2026-06-25.** Scope review
APPROVED (5 rounds). **8/13 tasks done:** Task-0 Run-shape lock (be3e6c0/bf1522b); T1a deterministic zero-LLM drain
SCENARIO-0002 (35dfe91…); T1b static-endpoint injection SCENARIO-0011 (honest cluster-residual dnsmasq); T1c
Mac-stateless SCENARIO-0124 (rewritten: single shared substrate); T2a versioned ExecutionPolicy SCENARIO-0123
(+deep-copy immutability fix); T2b policy-driven dispatch SCENARIO-0121 (+unique RunID + AC-3 isolation); T2c typed
artifact capture SCENARIO-0122 (+collision-free refs); T3b/d recursive-delegation core SCENARIO-0019 (Message/bus/
EmitUnderPolicy/graph; +depth-monotonicity fix that genuinely bounds recursion). Every task: implementer → spec-PAR
→ code-quality-PAR, fixes applied directly, committed; the adversarial gate caught a real defect on each production
task (T1c rewrite, T2a immutability, T2b constant-RunID, T2c ref-collision, T3b/d toothless depth-limit). Suite
~490+ green. **REMAINING: T3c (one-shot/long-running modes SCENARIO-0023), T3a (Tier-2 file-feed steering STORY-0073),
T4 (child-directive provisioning SCENARIO-0027), T5 (audit SCENARIO-0125), T6 (close JOURNEY-0002).** Then
per-sprint `auditing-progress` PAR, then ITER-0008b (operator UX/governance + Mac-off capstone).

**ITER-0008 PRE-ITERATION SCOPE REVIEW (PAR) — 2026-06-25.** Sentinel baseline clean (467 + JOURNEY-0001);
citations OK (82); status reconciliation clean. Scope review loop: **R1** REVISE → split 22-story capstone into
**ITER-0008 core** (autonomous-fleet: coordinator + recursive delegation + Run/dispatch/policy/audit; closes
JOURNEY-0002) + **ITER-0008b** (operator TUI/governance + Mac-off capstone; closes JOURNEY-0004..0007), with Task-0
Run-shape lock (`c2a51a5`). **R2** REVISE → evidence seam not concrete; pinned per-story evidence plan, fixed
JOURNEY-0002, kept STORY-0011 whole (`3398a28`). **R3** REVISE (A crit: missing scenario cards) → authored
SCENARIO-0121-0125 cards + corpus rows, cleaned JOURNEY-0002 card (`9d871b4`). **R4** REVISE (A: corpus JOURNEY-0002
still had STORY-0057; 5 existing core cards still TBD) → pinned all 5 cards+corpus rows, fixed corpus row
(`af41ad5`). **R5 APPROVE/APPROVE** — scope review converged. NEXT: decompose ITER-0008 core into TDD code+evidence
tasks (Task-0 Run-shape lock first; T1 foundational → T2 Run/policy → T3 delegation → T4 child-directive → T5 audit
→ T6 close JOURNEY-0002) → dispatch `implementing-tasks`.

**AUDIT CLEAN (ITER-0007c + owed ITER-0007b three-tier PAR) — 2026-06-25.**
Two parallel adversarial auditors over both repos returned **zero findings** (Critical/Serious/Minor), high
confidence (agreement). Tier 1 every AC PASS at declared seam; live/CI split verified HONEST (SCENARIO-0056
wall-clock limitation documented, not overstated). Tier 2 interceptor additive, no regressions. Tier 3 sentinel
green: JOURNEY-0001 + full `go test -race ./...` **467 passing**; laneq 186 tests / 96% cov / 4 CI gates green.
Security review clean (replay cache bounded, fail-closed, atomic O_EXCL key, sender-constrained proof). No gap
stories; roadmap unchanged. ITER-0007c & ITER-0007b CONFIRMED DONE. Both owed audits now discharged. **All
prerequisite audits clear → run ITER-0008 (Tier-2 coordinator, recursive delegation & operator UX — capstone,
the sole remaining pending iteration) via `running-an-iteration`.**

**ITER-0007c COMPLETE (Phase 1, local) — done:2026-06-25.** Full `running-an-iteration` loop: sentinel
baseline clean → citation OK → PAR scope review (REVISE→APPROVE) → T1 GrantSource (`bf840cd`) → T2 interceptor+wiring
(`3eee665`/`4a664d8`) → T3 cross-language real-wire e2e (`0d01230`/`cd9c61e`) → T4 issuer CLI (`e1cc5b5`/`6e14400`)
→ T5 corpus → wrap. Every code task through two-stage PAR. Post-iteration sentinel green (full module `-race` +
JOURNEY-0001); zero `TODO(ITER-0007c)`. STORY-0079/0080/0081 done; STORY-0082 AC-1a done.
**AC-1b DONE (operator-authorized 2026-06-25):** external PR `selamy-labs/laneq#20` opened; auth-laneq deployed LIVE on
`agent-host` and rolled **`log-only` → `enforce`** with the Temporal worker grant-wired; live cluster verified
(authenticated worker ALLOWED, unauth `UNAUTHENTICATED`, Temporal undisturbed). Only a Mac-side grant-renewal helper
remains (720h grant). **NEXT (orchestrator):** per-sprint `auditing-progress` on ITER-0007c (+ the still-owed
ITER-0007b three-tier audit); then ITER-0008 capstone.
**⚠ Incident (2026-06-25, resolved):** during the iteration RESUME, my own mishandling of the working directory /
tree state wiped UNCOMMITTED work (NO other agents were involved) — my scope edits (re-applied + committed) and a
pre-existing edit to `specs/2026-06-24-laneq-grant-paseto-design.md`. The spec edit was RECOVERED verbatim from
conversation history and committed (`713f2a3`). Mitigation now standard: commit artifacts before dispatching work;
use explicit `git add <files>` (no checkout/restore/stash/clean/add -A); confirm absolute working directory on resume.
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
