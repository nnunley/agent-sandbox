# EPIC-001 — Execution backend & topology

**Summary:** Execution backend & topology
**Stories:** STORY-0001, STORY-0002, STORY-0003, STORY-0004, STORY-0005, STORY-0006, STORY-0007, STORY-0008, STORY-0009, STORY-0010, STORY-0011, STORY-0012, STORY-0013, STORY-0014, STORY-0015, STORY-0016, STORY-0017, STORY-0018, STORY-0019, STORY-0020
**Primary sources:** `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`, `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 9/20 done (STORY-0018 done:ITER-0004; STORY-0004/0017/0020 done:ITER-0005 — in-scope
interface ACs; STORY-0007/0005/0008 done:ITER-0005b — durable coordinator VM (SCENARIO-0004),
immutable golden + incus-copy launch (SCENARIO-0003), disposable units inside the VM (SCENARIO-0004);
STORY-0001/0002 done:ITER-0007b — live Temporal time plane. Deferred microVM ACs (STORY-0004 AC-3,
STORY-0017 AC-3/4, STORY-0020 AC-2) proven by the ITER-0005b substrate harness — see iteration-log)

## STORY-0001

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Time plane holds durable, Mac-off-resistant scheduling state

**As a** fleet coordinator
**I want** scheduling and timer state to survive host restarts
**So that** work resumes mid-flight even after the Mac goes offline

**Acceptance criteria:**
- AC-1: Temporal plane (cluster-resident, durable) owns periodic Schedules, durable timers for deferred/future work, and retry backoff · impact:`cross-surface` · seam:`process-level` · scenario:`SCENARIO-0001`
- AC-2: Server and workers run on ndn-desktop; state survives host restarts natively · impact:`journey` · seam:`e2e` · scenario:`SCENARIO-0001`
- AC-3: Single writer constraint: only Temporal writes laneq scheduling fields (effective priority + not-before) · impact:`local` · seam:`integration` · scenario:`SCENARIO-0001`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:31-37`

**Status:** done:ITER-0007b — AC-3 (single-writer constraint: only Temporal writes laneq scheduling fields) proven at the design/logic level by the `GuardedDirective` writer-role guard (`modules/incus-dispatcher/temporal/writer.go`), evidence SCENARIO-0081 (mock-Temporal). **AC-1 + AC-2 done:ITER-0007b (E1 LIVE: DeferWorkflow survived a real Temporal restart, same runID Running→Completed, directive fired — durable timer, not laneq natural expiry)**.

## STORY-0002

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Directive queue is cluster-resident source of truth for work

**As a** dispatcher daemon
**I want** one authoritative queue that holds actionable directives with priority and leasing info
**So that** I can drain work deterministically without coordination conflicts

**Acceptance criteria:**
- AC-1: Coordination plane substrate implements durable directive queue: priority, lanes, threading, leasing · impact:`cross-surface` · seam:`integration`
- AC-2: Only currently-actionable directives live in queue; deferred/future work lives in Temporal until eligible · impact:`local` · seam:`integration`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:39-43`

**Status:** done:ITER-0007b — AC-1 done:ITER-0006 (laneq is the durable cluster-resident queue: priority/lanes/threading/leasing, proven via the gRPC adapter + SCENARIO-0091 fake CI gate + SCENARIO-0092 real-wire @nnunley/laneq 2d1b59e). AC-2 done:ITER-0007b (DeferWorkflow, C4 testsuite + E1 live) — the Temporal-holds-deferred-until-eligible contract proven against mock Temporal/laneq seam (`temporal/projection.go` not-before gating logic) in ITER-0007; live durable hold in deployed Temporal until eligible confirmed ITER-0007b.

## STORY-0003

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Control plane runs deterministic zero-LLM coordination loop

**As a** dispatcher daemon on ndn-desktop
**I want** to drain queue directives and run deterministic coordination over results
**So that** work is scheduled without per-task LLM decisions in the hot path

**Acceptance criteria:**
- AC-1: dispatcher serve --queue daemon drains queue, resolves each directive to a launch via Runner interface · impact:`local` · seam:`integration` · scenario:`SCENARIO-0002`
- AC-2: Coordination loop is deterministic with zero LLM calls per coordination action · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0002`
- AC-3: Each coordination action writes a decision-log line for auditability · impact:`local` · seam:`unit` · scenario:`SCENARIO-0002`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:45-50`

**Status:** done:ITER-0008 (T1a — deterministic zero-LLM coordination drain; SCENARIO-0002)

## STORY-0004

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Execution backend interface supports container and micro-VM implementations

**As a** dispatcher coordinator
**I want** pluggable launch(template) → handle backend interface
**So that** execution can use either proven container-backed or benchmarked micro-VM-backed runners

**Acceptance criteria:**
- AC-1: Runner interface supports launch(template) → handle abstraction · impact:`local` · seam:`unit`
- AC-2: Container-backed implementation ships first and is proven · impact:`none` · seam:`integration`
- AC-3: Micro-VM-backed implementation is benchmark-gated (deferred track) · impact:`none` · seam:`integration`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:51-53`

**Status:** done:ITER-0005 — AC-1 (Runner `launch→handle` abstraction; `BackendFactory.SelectRunner`
seam, `types.go`/`backend.go`) + AC-2 (container backend proven via `container_runner_test.go`,
SCENARIO-0076; daemon drives it through the factory, SCENARIO-0028). **AC-3 (microVM backend) →
ITER-0005b** (Firecracker, cluster-only; the factory graft point is documented `TODO(ITER-0005b)`).

## STORY-0005

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Worker instances launch as immutable golden copies

**As a** dispatcher backend
**I want** to launch workers from a golden NixOS image without live builds
**So that** worker spin-up is fast and reproducible

**Acceptance criteria:**
- AC-1: Golden worker image defined in fleet-worker/flake.nix with immutable root and writable /workspace + /tmp · impact:`local` · seam:`unit` · scenario:`SCENARIO-0003`
- AC-2: Launch via incus copy from golden with fresh name, never built live (one-shot per directive in v1) · impact:`local` · seam:`integration` · scenario:`SCENARIO-0003`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:55-58`

**Status:** done:ITER-0005b — AC-1: golden image DEFINED declaratively (`fleet-worker/golden.nix`):
read-only nix store (immutable root) + tmpfs /workspace,/tmp writable scratch (STORY-0049 AC-5),
pinned by `fleet-worker/tests/golden-image.test.sh`. AC-2: a real `fleet-golden` incus image was
published on the cluster; SCENARIO-0003 (golden-launch) MEASURED PASS — a fresh copy launches via
btrfs CoW in ~2.9–3.3s with the golden marker present (proves NO live build), /workspace + /tmp
writable, clean stop-then-delete teardown. FULL golden (skills/provider routing baked, byte-identical
clean-room regen) is STORY-0075 / ITER-0005c.

## STORY-0006

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Mac is stateless client authoring directives

**As a** Mac host developer
**I want** to author top-level directives and review parked threads
**So that** I hold no fleet state and can recover cleanly if offline

**Acceptance criteria:**
- AC-1: Mac holds no fleet state; it authors directives and reviews results only · impact:`journey` · seam:`e2e` · scenario:`SCENARIO-0001`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:60-61`

**Status:** done:ITER-0008 (T1c — Mac stateless client, no replay on reconnect; SCENARIO-0124)

## STORY-0007

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Durable micro-VM layer hosts coordinator daemon and warm Nix store

**As a** fleet execution layer
**I want** one live Firecracker micro-VM per trust domain running the coordinator daemon and storing warm /nix closure
**So that** disposable task units spin up in sub-seconds with zero initialization overhead

**Acceptance criteria:**
- AC-1: Live micro-VM (inside agent-host incus container) is durable: stays up across tasks, hosts coordinator + queue client + warm /nix store · impact:`cross-surface` · seam:`process-level` · scenario:`SCENARIO-0004`
- AC-2: Durable layer resolves D3 (agent lifecycle) cleanly: coordinator is daemon, not per-agent · impact:`local` · seam:`integration` · scenario:`SCENARIO-0004`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:65-79`

**Status:** done:ITER-0005b — AC-1 (durable micro-VM stays up + warm /nix store): `guests/coordinator-vm.nix`
(`microvm.vms.fleet-coord`, static 10.88.0.2, `writableStoreOverlay` warm store, nspawn-capable real
kernel) deployed to agent-host; **SCENARIO-0004 MEASURED PASS** — 0 restarts across 10 task cycles
(boot_id stable) + in-guest unit spin-up 17ms mean / 20ms p99 (gate ≤1000ms). AC-2 (coordinator is a
daemon, not per-agent): `Serve()` loop + `dispatcher serve` entrypoint (serve.go/serve_cmd.go),
TDD-tested. **Follow-up (noted, not blocking AC-1/AC-2):** baking the `dispatcher` binary into the VM
as a LIVE systemd service is deferred — it needs the incus-client-heavy binary Nix-packaged (vendorHash)
AND a real queue to drain (laneq, ITER-0006); the daemon loop + entrypoint are proven at the Go seam now.

## STORY-0008

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Disposable task units execute inside live micro-VM

**As a** task executor
**I want** each task to run as a one-shot disposable unit inside the durable VM
**So that** teardown is fast (kill unit, not incus delete) and warm store enables sub-second spin-up

**Acceptance criteria:**
- AC-1: Per-task disposable units run inside live micro-VM with immutable root · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0004`
- AC-2: Teardown never uses incus delete in hot path; only unit kill (resolves D5 hang) · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0004`
- AC-3: Warm /nix store enables sub-second unit spin-up · impact:`local` · seam:`integration` · scenario:`SCENARIO-0004`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:65-79`

**Status:** done:ITER-0005b — AC-1: per-task disposable units run inside the live coord VM via
`systemd-nspawn --ephemeral` (`fleet-worker/unit/fleet-unit.sh`) with an immutable read-only /nix
root. AC-2: teardown is unit-kill + ephemeral COW discard — the hot path is structurally incus-free
(incus is unreachable from inside the guest), SCENARIO-0008 AC-2 MEASURED PASS (111ms unit-kill, gate
≤5000ms, asserted incus-free). AC-3: warm /nix store → sub-second spin-up, MEASURED 16ms mean / 19ms
p99 in-guest unit spin-up (SCENARIO-0004 durable-vm). Evidence: cluster harness nspawn-fast + teardown
+ durable-vm.

## STORY-0009

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Service discovery v1: static endpoints + dnsmasq, no coredns

**As a** infrastructure architect
**I want** workers to reach fixed services (llm-proxy, queue/coordinator) via static template-injected endpoints and basic dnsmasq name resolution
**So that** we minimize DNS discovery surface, keep attack surface small, and defer multi-host discovery patterns (coredns/Consul/NATS) to v2+

**Acceptance criteria:**
- AC-1: Disposable units receive fixed service endpoints injected by low-level-executor-task-spec template discipline · impact:`local` · seam:`unit` · scenario:`SCENARIO-0011`
- AC-2: dnsmasq runs on br-microvm bridge for DHCP and basic name resolution only; no service discovery daemon · impact:`local` · seam:`integration` · scenario:`SCENARIO-0011`
- AC-3: Workers are launched (not discovered); coordination is queue-mediated (pull) + lean-ctx; liveness is tracked at app layer via queue leases + lean-ctx agent registry · impact:`local` · seam:`integration` · scenario:`SCENARIO-0011`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:354-363`

**Status:** done:ITER-0008 (T1b — static endpoint injection, no dynamic discovery; SCENARIO-0011; AC-2 dnsmasq cluster-residual via host/networking.nix)

## STORY-0010

**Epic:** EPIC-001 — Execution backend & topology
**Title:** [BLOCKED-ON-SUBSTRATE-DECISION] Queue substrate selection: must pass Mac-off acceptance test

**As a** fleet coordinator
**I want** to evaluate queue substrate candidates (laneq-as-cluster-service, network-native durable backend, dedicated queue host) against the governing constraint
**So that** the chosen substrate supports cluster autonomy with Mac powered off and handles prioritization (not-before) + temporal persistence requirements

**Acceptance criteria:**
- AC-1: [BLOCKED-ON-SUBSTRATE-DECISION] Candidate 1 (laneq-as-cluster-service): close Mac → fleet still coordinates via laneq MCP server on ndn-desktop; DB on host volume survives Mac downtime · impact:`cross-surface` · seam:`e2e` · scenario:`SCENARIO-0012`
- AC-2: [BLOCKED-ON-SUBSTRATE-DECISION] Candidate 2 (network-native durable backend, e.g., Postgres/NATS JetStream): close Mac → fleet still coordinates via persistent backend; multi-client HA + host-recycle survival supported · impact:`cross-surface` · seam:`e2e` · scenario:`SCENARIO-0012`
- AC-3: [BLOCKED-ON-SUBSTRATE-DECISION] Candidate 3 (dedicated queue host/container): close Mac → fleet still coordinates via separate persistent queue container; queue survives worker-capacity rebuilds · impact:`cross-surface` · seam:`e2e` · scenario:`SCENARIO-0012`
- AC-4: [BLOCKED-ON-SUBSTRATE-DECISION] Chosen substrate implements eligibility gate (not-before) for prioritization regardless of winner · impact:`local` · seam:`unit` · scenario:`SCENARIO-0012`
- AC-5: [BLOCKED-ON-SUBSTRATE-DECISION] If network-native backend chosen: Postgres (or equivalent) instance also serves Temporal persistence needs, making queue infrastructure 'free' · impact:`local` · seam:`integration` · scenario:`SCENARIO-0012`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:366-389`

**Status:** done:ITER-0006b — AC-4 done:ITER-0006 (not-before eligibility); AC-1 done:ITER-0006b
(Mac-off acceptance, NARROW substrate proof — SCENARIO-0012: the deployed laneq coordinates with a
cluster-resident consumer draining autonomously via a `systemd-run` detached unit while the Mac is
uninvolved; host-volume DB persists). AC-2/3/5 = not-chosen decision outcomes (laneq chosen). The FULL
sustained operator/fleet Mac-off (real dispatcher daemon) is tracked separately as STORY-0074/ITER-0008,
not an unmet STORY-0010 AC. (Original PAR-revised plan note below.) — substrate decision
RESOLVED = **laneq** (Python; gRPC binding; Norman is a contributor). AC-4 (not-before eligibility gate)
done:ITER-0006 via SCENARIO-0091/0092. AC-2/AC-3/AC-5 are **not-chosen decision outcomes** (network-native
Postgres/NATS / dedicated-host candidates — closed BY the laneq decision, not unmet ACs). The
Go↔**real laneq** gRPC wire is proven in ITER-0006 (SCENARIO-0092 via uvx). **AC-1 (Mac-off cluster
e2e, SCENARIO-0012) carried → ITER-0006b** (Nix package + cluster deploy + Mac-off acceptance). Story =
PARTIAL after ITER-0006; closes in ITER-0006b.

## STORY-0011

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Dispatch work to correct worker based on task type and policy

**As a** coordinator
**I want** to route tasks to local or remote workers with matching capabilities
**So that** work executes on appropriate infrastructure

**Acceptance criteria:**
- AC-1: Worker object includes worker_kind field (local, incus-container, microvm, research, etc.) · impact:`local` · seam:`unit`
- AC-2: Worker object includes capabilities array describing tools and features available · impact:`local` · seam:`unit`
- AC-3: Policy object includes allowed_policies array constraining which policies can dispatch to this worker · impact:`local` · seam:`unit`
- AC-4: Run is created with worker_id, worker_kind, and policy_id matching dispatch decision · impact:`cross-surface` · seam:`integration`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:73-80, 186-201, 202-223`

**Status:** done:ITER-0008 (T2b — Worker registry + capability/allowed-policies dispatch -> Run; SCENARIO-0121)

## STORY-0012

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Support durable message-queue-first recursive delegation

**As a** coordinator
**I want** agents to emit work onto durable topics rather than calling each other in-memory
**So that** delegation is observable, resumable, and survives coordinator restarts

**Acceptance criteria:**
- AC-1: Message object includes thread_id, run_id, parent_run_id, and depth fields for delegation chain tracking · impact:`local` · seam:`unit` · scenario:`SCENARIO-0017`
- AC-2: Message object includes correlation_id for request/response pairing · impact:`local` · seam:`unit` · scenario:`SCENARIO-0017`
- AC-3: System supports at least request/response topics and event/status topics for work emission · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0017`
- AC-4: Coordinator can reconstruct effective delegation graph from message history using parent_run_id and correlation metadata · impact:`journey` · seam:`process-level` · scenario:`SCENARIO-0017`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:699-730`

**Status:** done:ITER-0008 (T3b/d — durable message-queue recursive delegation; SCENARIO-0019)

## STORY-0013

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Support one-shot and long-running agent runtime modes

**As a** coordinator
**I want** agents to declare whether they run once or maintain stable identity
**So that** subscription, heartbeat, and retry behavior can be tuned correctly

**Acceptance criteria:**
- AC-1: Worker/Agent declares runtime mode: one_shot or long_running · impact:`local` · seam:`unit` · scenario:`SCENARIO-0017`
- AC-2: one_shot mode: worker consumes one item, performs bounded work, emits result, exits · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0017`
- AC-3: long_running mode: worker holds stable identity, stays subscribed, emits heartbeats · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0017`
- AC-4: Runtime mode influences subscription, heartbeat requirements, retry semantics, cache allowances · impact:`local` · seam:`unit` · scenario:`SCENARIO-0017`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:767-820`

**Status:** done:ITER-0008 (T3c — one-shot vs long-running runtime modes; SCENARIO-0023)

## STORY-0014

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Make recursive delegation cheap via message emission

**As a** agent
**I want** to emit work onto topics instead of orchestrating sub-agents
**So that** coordination scales without heavyweight in-memory delegation

**Acceptance criteria:**
- AC-1: Agent can emit work to any request topic when policy allows · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0019`
- AC-2: Message routing preserves thread_id, run_id, correlation_id for tracking · impact:`local` · seam:`unit` · scenario:`SCENARIO-0019`
- AC-3: Depth field on message prevents unbounded recursion · impact:`local` · seam:`unit` · scenario:`SCENARIO-0019`
- AC-4: Research agent can emit web.fetch.request, coding orchestrator can emit review.request, synthesis worker can emit wiki.update.request · impact:`journey` · seam:`e2e` · scenario:`SCENARIO-0019`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:821-839`

**Status:** done:ITER-0008 (T3b/d — cheap delegation via depth-bounded message emission; SCENARIO-0019)

## STORY-0015

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Capture artifacts from every remote run

**As a** coordinator
**I want** every run to produce durable logs and artifacts
**So that** results are linkable back to threads and messages

**Acceptance criteria:**
- AC-1: Run object includes artifact_refs and log_refs arrays · impact:`local` · seam:`unit`
- AC-2: Artifacts and logs are stored durably and linked to run_id · impact:`local` · seam:`unit`
- AC-3: Artifact types include: diff, note, synthesis, benchmark, verification report, design doc, mutation proposal · impact:`local` · seam:`unit`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:348-351, 160, 182, 225-237`

**Status:** done:ITER-0008 (T2c — typed artifact capture into Run.ArtifactRefs/LogRefs; SCENARIO-0122)

## STORY-0016

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Support externalized and versioned execution policies

**As a** coordinator
**I want** policies to be data-driven and versioned, not hardcoded
**So that** execution strategies can be evolved and reviewed

**Acceptance criteria:**
- AC-1: Policy object is versioned and stored durably · impact:`local` · seam:`unit`
- AC-2: Policy includes: kind, constraints, delegation_rules, verification_requirements, mutation_allowed · impact:`local` · seam:`unit`
- AC-3: Policy types include: one-shot task, ralph-style loop, research burst, verify-fix loop, background summarizer, review-only · impact:`local` · seam:`unit`
- AC-4: Policy changes are inspectable, reviewable, revertible · impact:`journey` · seam:`process-level`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:282-286, 202-223`

**Status:** done:ITER-0008 (T2a — versioned immutable ExecutionPolicy store; SCENARIO-0123)

## STORY-0017

**Epic:** EPIC-001 — Execution backend & topology
**Title:** D2: Backend-agnostic interface; container-backed one-shot first, micro-VM gated by benchmark

**As a** platform engineer
**I want** to define backend as an interface and ship container-backed one-shot first
**So that** micro-VMs can be added as parallel track when startup latency and teardown are benchmarked and validated

**Acceptance criteria:**
- AC-1: Backend interface separates intent/template/queue logic from container vs. micro-VM delivery · impact:`local` · seam:`unit` · scenario:`SCENARIO-0028`
- AC-2: Worker NixOS config is single declarative source (fleet-worker) delivered as incus container or Firecracker guest · impact:`local` · seam:`integration` · scenario:`SCENARIO-0028`
- AC-3: Micro-VM backend startup latency with closure realized ≤ 5 s (measured on test-vm with boot-to-ready sentinel) · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0028`
- AC-4: Micro-VM clean kill-to-teardown completes without hang (systemctl stop success, no incus delete required) · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0028`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:117-162`

**Status:** done:ITER-0005 — AC-1 (backend interface separates intent/template/queue from delivery
— `Runner`/`BackendFactory`; daemon is substrate-agnostic, SCENARIO-0028). AC-2 (worker NixOS config
single declarative source = `fleet-worker`): the "delivered as incus container" half is DONE and was
**cluster-validated end-to-end on ndn-desktop/agent-host 2026-06-18/19** (real dogfood ran via
`nix develop ./fleet-worker`; `worker-container.nix` applied via `nixos-rebuild switch`; runner.sh:3,
ITER-0000 log). ITER-0005 PINS the single-source patterns against drift with a CI test (SCENARIO-0090,
`fleet-worker/tests/single-source.test.sh`) — resolving the audit finding that AC-2 was prose-only.
**The "delivered as Firecracker guest" half + golden-copy replication (incus copy from golden) +
immutable-root/writable-scratch (STORY-0005 AC-1 / STORY-0049 AC-5) → ITER-0005b.** AC-3 (microVM
startup ≤5s measured) + AC-4 (microVM clean teardown) → ITER-0005b (cluster/Firecracker, no CI seam).

## STORY-0018

**Epic:** EPIC-001 — Execution backend & topology
**Title:** D3: Durable agents deferred; lean-ctx carries state passthrough, not authoritative state

**As a** workflow coordinator
**I want** to carry soft, lossy-OK state between one-shot runs via lean-ctx without making it authoritative
**So that** state loss does not break correctness (code diff + oracle grade remain source of truth)

**Acceptance criteria:**
- AC-1: ctx_agent diary (decisions/blockers/progress) is accessible via action=diary (write) and recall_diary/diaries (read) · impact:`local` · seam:`integration` · scenario:`SCENARIO-0030`
- AC-2: ctx_agent share_knowledge/receive_knowledge primitives enable fact exchange between one-shots · impact:`local` · seam:`integration` · scenario:`SCENARIO-0030`
- AC-3: ctx_handoff action=create|export|import|pull bundles workflow state + session snapshot + curated knowledge on shared volume · impact:`local` · seam:`integration` · scenario:`SCENARIO-0030`
- AC-4: Authoritative state (code diff, oracle grade) never lives in lean-ctx; correctness is independent of handoff loss · impact:`journey` · seam:`e2e` · scenario:`SCENARIO-0030`
- AC-5: lean-ctx message bus is not used as work queue; directive queue remains durable, leased, crash-safe ledger · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0030`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:163-187`

**Status:** done:ITER-0004 — AC-1/AC-2/AC-3 (diary, knowledge, ctx_handoff bundle) via the `LeanCtxProvider` adapter (T6) behind the `ContextProvider` interface; AC-3's versioned handoff-bundle schema doc shipped (T0). AC-4 (correctness independent of handoff loss) + AC-5 (provider is never the work queue) via `NoopProvider` + daemon guard test (T7). Evidence SCENARIO-0030 (adapter + real-lean-ctx round-trip) + SCENARIO-0031 (CI anti-reward-hack). **ITER-0004 scope (PAR round-2 rescoping):**
- AC-1/AC-2/AC-3 IN, AC-3 carries an explicit **documentation deliverable**: the formal versioned handoff-bundle
  schema → `docs/plans/2026-06-21-handoff-bundle-schema.md` (so ITER-0006 can pass `Directive.HandoffIn`).
- **AC-4** proof seam moved e2e→**CI unit/integration primary** (daemon loop, fake backend, handoff absent/corrupt
  → `passed()` still grades from `Result.ExternalGradingResult`); SCENARIO-0031 cluster e2e is optional enrichment,
  not gating. It stays a *testable* AC, not just a principle.
- **AC-5** proof = architecture/guard test + code review (daemon claims work only via the durable `queue.Queue`
  ledger, never a lean-ctx message bus).

## STORY-0019

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Run template runner with lean-context setup and agent invocation

**As a** fleet worker
**I want** to execute the template's runner which sets up lean-context and invokes claude with the template prompt
**So that** the agent processes the task using all available context and produces a diff and result

**Acceptance criteria:**
- AC-12: Lean-context setup runs to prepare isolated execution environment · impact:`local` · seam:`process-level` · scenario:`JOURNEY-0001`
- AC-13: Lean-context serve starts the context layer · impact:`local` · seam:`process-level` · scenario:`JOURNEY-0001`
- AC-14: Claude agent is invoked with template prompt (-p flag) · impact:`local` · seam:`process-level` · scenario:`JOURNEY-0001`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:317`

**Status:** pending

## STORY-0020

**Epic:** EPIC-001 — Execution backend & topology
**Title:** Container backend interface abstraction

**As a** platform developer
**I want** to verify container backend implementation against the existing container_runner_test.go contract
**So that** backends are interchangeable and new backends meet the interface guarantee

**Acceptance criteria:**
- AC-1: container backend passes all tests in container_runner_test.go · impact:`local` · seam:`integration` · scenario:`SCENARIO-0076`
- AC-2: micro-VM backend is gated behind benchmark flag and passes same contract · impact:`local` · seam:`integration` · scenario:`SCENARIO-0076`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:401`

**Status:** done:ITER-0005 — AC-1 (container backend passes `container_runner_test.go`; CI subset
green, integration cases self-skip when incus unreachable — SCENARIO-0076). **AC-2 (microVM backend
passes same contract) → ITER-0005b** (the contract is the graft target for the Firecracker runner).