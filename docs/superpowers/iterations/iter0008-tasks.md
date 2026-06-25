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

Status legend: [ ] pending  [~] in progress  [x] done:ITER-0008  (11/13 done)
- [x] Task-0 Run-shape lock (be3e6c0/bf1522b)
- [x] T1a deterministic zero-LLM drain SCENARIO-0002 (35dfe91/2ab2d19/9fbf282)
- [x] T1b static endpoint injection SCENARIO-0011 (3356489/09116ee; honest cluster-residual dnsmasq)
- [x] T1c Mac-stateless SCENARIO-0124 (5791df2 rework 948d1af; single shared substrate)
- [x] T2a versioned ExecutionPolicy SCENARIO-0123 (996b738 + immutability fix 9d436c1)
- [x] T2b policy-driven dispatch SCENARIO-0121 (8ef171a/0e8fb13/ffb6c20; unique RunID, AC-3 isolation)
- [x] T2c typed artifact capture SCENARIO-0122 (948ad58 + collision-free refs 1e2b49d)
- [x] T3b/d recursive delegation SCENARIO-0019 (690b9f5 + depth-monotonicity 99bfc0a/c243029)
- [x] T3c runtime modes SCENARIO-0023 (d85fff6 + artifact-linkage/Valid 2dc65ff/c468780)
- [x] T3a file-feed steering SCENARIO-0064 (fc9cefc + corrupt-file test d1365bc/2bb4436)
- [x] T4 child-directive provisioning SCENARIO-0027 (49efacc + end-to-end proofs edbfc69)
- [x] T5 audit SCENARIO-0125 (0ee56ce + wiring/non-tautological-replay fix 2ab35fa; two-stage PAR APPROVED)
- [ ] T6 close JOURNEY-0002

Process note: implementer subagents may refuse orchestrator-relayed PAR fix requests (their global
CLAUDE.md treats coordinator messages as lacking user authority). For small, fully-specified review
fixes the orchestrator applies them directly (as done for Task-0's Currency removal).

Process note 2: PAR reviewer subagents (general-purpose, can write) sometimes leave scratch test
files / rebuilt the tracked binary in the working tree while "verifying". After each PAR round,
`git status` and clean up (rm untracked scratch, `git checkout` the binary) BEFORE committing.
