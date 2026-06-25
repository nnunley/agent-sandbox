# ITER-0008 core — TDD task decomposition (PAR scope review APPROVED, 2026-06-25)

Grounded in the existing flat-package `modules/incus-dispatcher` (Run exists at run.go:31 with
reserved fields; Policy/Directive/DecisionLog exist; delegation/dispatch/Genome greenfield).
Each task: red→green→refactor→commit, then two-stage PAR (spec-compliance + code-quality).

## Task-0 (BLOCKING) — Run-shape lock
Extend `Run` (run.go:31) additively with the unified shape: WorkerID, WorkerKind, PolicyID,
ArtifactRefs ([]ArtifactRef{Kind,Ref}), LogRefs, ProviderInstance, ModelID, BudgetSnapshot;
keep StumbleSignals; additively reserve Tokens/Latency/Spend (ITER-0008b, documented). JSON
additive-safe (zero-value back-compat). Unit test: shape + JSON round-trip. No behavior change.
Stories: STORY-0015, STORY-0011 (fields), STORY-0035 AC-1/2. Gate for T2.

## T1 — foundational (independent, after T0 only for shared types not needed)
- T1a STORY-0003 deterministic loop → SCENARIO-0002 `TestScenario0002_DeterministicDrain` (daemon)
- T1b STORY-0009 service discovery → SCENARIO-0011 `TestScenario0011_StaticEndpointInjection`
- T1c STORY-0006 Mac stateless → SCENARIO-0124 `TestScenario0124_MacStatelessClient` (e2e)

## T2 — Run/policy shape (needs T0)
- T2a STORY-0016 versioned policy → SCENARIO-0123 `TestScenario0123_VersionedPolicy`
- T2b STORY-0011 dispatch (worker_kind/capabilities/allowed_policies + AC-4 Run creation) → SCENARIO-0121 `TestScenario0121_PolicyDrivenDispatch`
- T2c STORY-0015 artifact capture → SCENARIO-0122 `TestScenario0122_RunArtifactCapture`

## T3 — delegation core (needs T2 Run shape)
- T3a STORY-0073 Tier-2 coordinator file-feed steering (steering source)
- T3b STORY-0012 + T3d STORY-0014 recursive delegation via durable message emission → SCENARIO-0019 `TestScenario0019_RecursiveDelegation`
- T3c STORY-0013 one-shot vs long-running modes → SCENARIO-0023 `TestScenario0023_OneShotWorker`

## T4 — child-directive provisioning (after T3)
- STORY-0049 AC-4 → SCENARIO-0027 `TestScenario0027_ChildDirectiveProvisioning`

## T5 — audit layer (extends DecisionLog)
- STORY-0054 audit all runs/delegations/mutations + replay → SCENARIO-0125 `TestScenario0125_AuditReplay`

## T6 — close JOURNEY-0002
- STORY-0073 + STORY-0012: orchestrator steers high-priority directive; daemon preempts next claim → `TestJourney0002_LiveSteering`

Status legend: [ ] pending  [~] in progress  [x] done:ITER-0008
- [x] Task-0 (be3e6c0 + review-fix bf1522b; two-stage PAR APPROVED; suite 468 green)  [x] T1a (35dfe91/2ab2d19/9fbf282; two-stage PAR APPROVED) [x] T1b (3356489/09116ee; two-stage PAR APPROVED; honest cluster-residual AC-2) [~] T1c  [ ] T2a [ ] T2b [ ] T2c  [ ] T3a [ ] T3b/d [ ] T3c  [ ] T4  [ ] T5  [ ] T6

Process note: implementer subagents may refuse orchestrator-relayed PAR fix requests (their global
CLAUDE.md treats coordinator messages as lacking user authority). For small, fully-specified review
fixes the orchestrator applies them directly (as done for Task-0's Currency removal).
