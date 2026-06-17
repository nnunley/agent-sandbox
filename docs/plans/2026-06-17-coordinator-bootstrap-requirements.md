# Coordinator Bootstrap Requirements for agent-sandbox

**Date:** 2026-06-17
**Status:** Draft requirements
**Scope:** Broad coordination/control-plane requirements for turning `agent-sandbox` into a general durable work coordination and remote execution system.

## Purpose

Unify the strongest ideas from the existing systems into one broader platform:

- `agent-sandbox` contributes the execution substrate:
  - Incus / btrfs / NixOS host control
  - Firecracker and container workers
  - provider proxying
  - artifact capture
  - remote execution stability
- `sprout` contributes the stronger control-plane ideas:
  - recursive delegation
  - externalized agent/policy specs
  - durable pub/sub messaging
  - provider-instance model selection
  - git-backed mutable genome
  - evaluation and routing evolution
- `ralph-loop` contributes one execution-policy pattern:
  - segmented loops
  - preflight / transition / compaction discipline
  - anti-drift local continuation rules

The product boundary should be `agent-sandbox`, not `sprout` and not `ralph-loop`.

## Problem

The current multi-agent workflow has these failure modes:

- work threads are lost when agents leave branches or workspaces
- agents reinvent in-progress work rather than continuing it
- many repos and many ideas compete for attention with no anti-starvation policy
- workspace leases prevent collisions but do not preserve semantic thread state
- model/provider selection is not yet driven by explicit budget, quality, and learned routing policy
- remote workers exist, but the broader coordination layer does not yet own thread identity, prioritization, or message routing

## Goal

Build a durable coordinator that can:

- manage long-lived work threads across many repositories and non-repo topics
- dispatch work to local or remote workers
- preserve enough context for workers to resume productively
- route tasks to the best provider/model mix within explicit budget policy
- evolve prompts, routing rules, and execution policies over time
- bootstrap itself incrementally until the system can build the rest of itself

## Product Position

`agent-sandbox` should become a **general coordination and execution platform for knowledge work**, not a coding-agent product.

The system must support at least:

- coding and code review
- research and synthesis
- planning and backlog maintenance
- wiki / memory distillation
- long-lived idea incubation
- operator-driven dispatch and worker supervision

## Core Architecture

The system has five major planes.

### 1. Control plane

Owns:

- thread registry
- run registry
- prioritization and anti-starvation
- worker dispatch
- execution policy selection
- handoff state
- branch/workspace authority

### 2. Execution plane

Owns:

- local workers
- remote Incus workers
- Firecracker microVM workers
- container workers
- snapshots / copies / repo materialization
- artifact capture

### 3. Messaging plane

Owns:

- durable topics
- request/response routing
- worker status heartbeats
- context packets
- eventually side-channel conversations

### 4. Knowledge plane

Owns:

- llm-wiki integration
- graph / retrieval integration
- thread summaries
- artifact metadata
- long-lived memories
- cross-thread synthesis

### 5. Evolution plane

Owns:

- mutable genome
- prompt/spec evolution
- routing rule evolution
- provider/model policy evolution
- execution policy evolution
- evaluation and promotion/revert loops

## Non-Negotiable Invariants

- Workers must never receive raw long-lived provider secrets.
- The Mac trust root / broker remains the master secret boundary.
- Workspace claims must be checked before using an existing workspace directory.
- A branch or workspace with active work is a continuation context, not a blank slate.
- If a thread already exists, new work must continue or explicitly supersede it.
- All run, message, artifact, and mutation state must be auditable.
- Learned mutations must not silently modify kernel safety rules.

## Core Object Model

### Thread

The top-level unit the coordinator manages.

Required fields:

- `thread_id`
- `title`
- `goal`
- `kind` (`code`, `research`, `synthesis`, `ops`, `meta`, etc.)
- `repo_refs[]`
- `wiki_refs[]`
- `status` (`queued`, `active`, `paused`, `blocked`, `done`, `abandoned`)
- `priority`
- `aging_score`
- `current_branch`
- `current_workspace`
- `current_assignee`
- `execution_policy`
- `provider_policy`
- `resume_summary`
- `last_verified_state`
- `next_step`
- `artifact_refs[]`
- `supersedes` / `superseded_by`

### Run

A single execution attempt against a thread.

Required fields:

- `run_id`
- `thread_id`
- `worker_id`
- `worker_kind`
- `policy_id`
- `provider_instance`
- `model_id`
- `budget_snapshot`
- `status`
- `started_at`
- `ended_at`
- `result_summary`
- `verification_summary`
- `artifact_refs[]`
- `log_refs[]`
- `stumble_signals[]`

### Worker

An execution target, local or remote.

Required fields:

- `worker_id`
- `worker_kind` (`local`, `incus-container`, `microvm`, `research`, etc.)
- `host_id`
- `capabilities[]`
- `allowed_policies[]`
- `provider_access_policy`
- `status`
- `lease_state`
- `cost_class`

### Policy

A reusable execution strategy.

Examples:

- one-shot task execution
- ralph-style segmented loop
- research burst
- verify/fix loop
- background summarizer
- review-only worker

Required fields:

- `policy_id`
- `kind`
- `description`
- `constraints`
- `delegation_rules`
- `verification_requirements`
- `mutation_allowed`

### Artifact

A durable result object.

Examples:

- diff
- note
- synthesis
- benchmark
- verification report
- design doc
- mutation proposal

### Message

A durable bus message.

Required fields:

- `message_id`
- `topic`
- `thread_id`
- `run_id` optional
- `sender`
- `receiver` optional
- `payload`
- `context_packet_ref`
- `created_at`
- `correlation_id`

### Genome Entry

A mutable runtime spec or rule.

Examples:

- worker prompt/spec
- routing rule
- provider escalation rule
- budget heuristic
- policy tuning

Required fields:

- `entry_id`
- `kind`
- `version`
- `content_hash`
- `source` (`bootstrap`, `learned`, `promoted`, `experiment`)
- `status` (`active`, `candidate`, `rejected`, `reverted`)
- `evidence_refs[]`

## Requirements Derived from Sprout

These capabilities should be replicated, generalized, or adapted.

### Externalized specs

- Worker/orchestrator definitions must be data-driven and versioned.
- Specs must not be hardcoded in the runtime.
- Specs must support roles, limits, delegation permissions, tools/capabilities, and prompts/instructions.

### Recursive delegation

- Coordinators must be able to delegate recursively.
- Delegation must be constrained by explicit policy, not prompt text alone.
- Leaf workers must not silently gain orchestrator behavior.

### Single delegation surface

- Use one generic delegation mechanism rather than one API per worker type.
- Delegations must include:
  - target worker/policy class
  - goal
  - hints/context
  - thread/run identity

### Durable pub/sub bus

- Inter-agent communication must ride on a durable topic bus.
- Messaging must support request/response, events, and heartbeats.
- Messages must carry enough context to resume work.

### Provider-instance model

- Providers must be configured as explicit instances.
- Runtime model resolution must not rely on guessing from model names.
- Session state must store provider instance and model policy explicitly.

### Audit log

- Every run, delegation, transition, tool action, and mutation must be logged durably.
- Logs must be replayable enough to reconstruct behavior.

### Mutable git-backed genome

- Runtime specs, routing rules, and learned heuristics must live in a mutable versioned store.
- Changes must be inspectable, reviewable, revertible, and promotable.

### Evaluation-driven learning

- Repeated stumbles must generate structured learn signals.
- Mutation must be tied to measured outcomes, not just qualitative impressions.

## Requirements Derived from agent-sandbox

These capabilities are already present or clearly intended and should remain central.

### Remote execution substrate

- Incus/NixOS remote host control
- Firecracker microVM workers
- container workers
- fast repo copies / snapshots / reprovisioning
- shared artifact storage

### Provider proxy and broker integration

- Workers talk only to proxy surfaces.
- Provider auth is mediated by broker + proxy.
- Multiple provider kinds and local GPU backends are supported.

### Artifact capture

- Every remote run must produce durable logs and artifacts.
- Artifacts must be linkable back into threads and messages.

### Declarative host/guest configuration

- Worker environment should remain reproducible and Nix-driven.
- Worker profiles should be policy-addressable.

## Budget and Model Selection Requirements

This is a first-class subsystem, not a provider config footnote.

### Provider instance

The system must support explicit provider instances such as:

- `claude-code-main`
- `openai-codex-main`
- `openrouter-main`
- `ollama-cloud`
- `ollama-local`
- `vllm-local`

### Model policy

The system must support runtime model selection based on:

- task type
- worker type
- policy type
- quality tier
- latency
- cost
- context size
- historical success rate
- verification pass rate

### Budget policy

The system must support budgets at least at these levels:

- per message
- per run
- per thread
- per worker class
- per provider
- per day / time window

### Escalation rules

The system must be able to encode and learn policies such as:

- try cheap local model first
- escalate to stronger cloud model on failure or uncertainty
- reserve best models for orchestration / verification / critical synthesis
- use specialized providers for specific task classes

### Required accounting

Track at least:

- tokens / request counts
- latency
- spend per provider/model
- spend per thread/run
- success per spend
- retries per provider/model

## Genome Mutation Requirements

Genome mutation is a core requirement.

### Mutation targets

At minimum the system must be able to evolve:

- prompts/system text for worker and orchestrator specs
- routing heuristics
- provider/model choice heuristics
- budget escalation heuristics
- execution policy tuning
- thread handoff / resume templates

### Learn signals

Structured stumble signals should include:

- retries
- timeouts
- verification failures
- provider failures
- delegation loops
- workspace/thread-loss incidents
- duplicate/reinvented work
- cost blowouts
- starvation incidents

### Mutation flow

- detect repeated pattern
- propose mutation
- run trial / experiment
- measure outcome
- promote, keep experimental, reject, or revert

### Safe mutation boundary

The following must remain protected unless explicitly human-approved:

- secret-handling invariants
- lease and workspace safety rules
- audit requirements
- hard budget guardrails
- kernel safety constraints

## Messaging Requirements

### MVP topics

The first version should support at least:

- `thread.<id>.request`
- `thread.<id>.response`
- `thread.<id>.status`
- `worker.<id>.heartbeat`
- `scheduler.dispatch`
- `artifact.created`
- `run.completed`
- `mutation.proposed`

### Context packets

Every dispatched unit of work must include or reference:

- thread goal
- run objective
- current branch/workspace if relevant
- relevant repo/wiki refs
- prior summary
- verification state
- budget/policy context
- stop conditions

### Side-channel conversations

Not required for MVP, but the bus must leave room for them.

Eventually workers should be able to open subordinate conversations for:

- clarification
- research bursts
- specialist consultation
- artifact discussion

This should be modeled as additional threads or subthreads, not ad hoc hidden chat.

## Prioritization and Anti-Starvation Requirements

The coordinator must actively prevent ideas from starving.

Required capabilities:

- queue ordering by priority plus aging
- explicit paused / blocked / waiting states
- stale-thread resurfacing
- backlog review mode
- multiple queue classes (e.g. urgent, active, incubating, maintenance)
- policies for revisiting long-dormant but valuable threads

The system must not let active coding threads consume all attention indefinitely.

## Continuity Requirements

This is one of the key missing capabilities.

### Resume audit

Before resuming a thread, the system should reconstruct:

- authoritative branch/workspace
- current diff/artifacts
- last verified result
- current open questions
- next step

### Reinvention guard

If a thread already has active work:

- new runs must continue the current implementation by default
- a restart must explicitly declare why the prior path is insufficient
- superseding a thread must be modeled explicitly

### Branch/workspace authority

Threads must own authoritative branch/workspace metadata.
Leases prevent collisions; the coordinator must preserve intent.

## MVP Requirements

The MVP should be strong enough to help build the rest of the system.

### MVP operator surface

- a TUI on the Mac
- create work items / threads
- route work items into durable queues
- inspect queue and worker state
- inspect responses and artifacts
- respond, requeue, or pause threads

### MVP worker surface

- remote workers can claim queued work
- workers can receive enough context to begin useful work
- workers can send structured responses back
- runs and artifacts are durable

### MVP policy support

- at least one local policy
- at least one remote worker policy
- one segmented-loop policy inspired by `ralph`
- basic provider/model selection policy

### MVP exclusions

- no full autonomous side-channel conversations yet
- no broad automatic genome mutation rollout yet
- no complete cross-repo global optimization yet

## Bootstrap Phases

The system should be delivered as a sequence where each phase helps build the next one.

### Phase 0 — substrate hardening

Objective:

- reliable remote worker substrate
- stable provider proxy paths
- durable artifact/log capture
- fast repo copies/snapshots on remote hosts

Proves:

- remote execution is dependable enough to build on

### Phase 1 — TUI work-queue MVP

Objective:

- LLM-driven TUI on Mac
- create work items
- route to durable queues
- receive worker responses
- inspect artifacts and status

Proves:

- the operator can use the system as a real control surface

### Phase 2 — thread registry and continuity

Objective:

- durable threads as first-class objects
- resume summaries, next-step, last-verified-state
- branch/workspace authority attached to threads
- anti-reinvention guardrails

Proves:

- work continuity survives handoffs and restarts

### Phase 3 — scheduler and policy engine

Objective:

- thread prioritization and anti-starvation
- queue classes
- dispatch policy selection
- run lifecycle management

Proves:

- the control plane can manage many concurrent ideas without losing them

### Phase 4 — provider/model budget plane

Objective:

- provider instances
- model selection policy
- budget tracking and enforcement
- escalation rules

Proves:

- runs can be cost-aware and quality-aware

### Phase 5 — runtime genome and evaluation

Objective:

- mutable runtime genome store
- mutation proposals and experiments
- evaluation against stumble rate, cost, latency, and verification outcomes
- promote / revert flow

Proves:

- the system can evolve its own prompts and policies responsibly

### Phase 6 — side-channel conversations

Objective:

- workers can open subordinate conversations or subthreads
- side channels are durable and auditable
- subthreads integrate with thread registry and budgets

Proves:

- the bus and thread model support richer inter-agent collaboration without chaos

## What This Doc Intentionally Does Not Fix Yet

- exact message transport implementation choice
- exact database/storage implementation choice
- exact TUI framework choice
- exact schema for all event and artifact payloads
- exact genome promotion UX

Those belong in the next design pass.

## Immediate Next Spec

The next concrete design should define:

- the thread/run/policy schema
- the durable message schema
- the queue and scheduler state machine
- the TUI operator workflows
- the runtime genome store layout and mutation lifecycle
- the boundary between local control and remote worker execution

## Follow-Up Requirements From Queue-First Discussion

### Message-queue-first recursion

Recursive delegation should default to a durable message-queue architecture rather than a heavyweight in-memory agent tree.

Requirements:

- agents listen on one or more request topics
- agents publish on one or more response or event topics
- any agent may emit additional work onto another topic when policy allows
- recursion emerges from message flow and correlation metadata
- the coordinator remains able to reconstruct the effective delegation graph from message history

### Required message envelope metadata

Every durable work message should carry enough structure to preserve continuity and observability.

Required fields should include at least:

- `thread_id`
- `run_id`
- `parent_run_id` or `caused_by`
- `goal`
- `policy_id`
- `provider_policy_ref`
- `budget_context`
- `reply_to`
- `correlation_id`
- `depth`
- `deadline` or `ttl`
- `artifact_refs[]`
- `resume_summary_ref`

### Topic taxonomy

The architecture should distinguish at least:

- request topics
- response topics
- event/status topics
- control topics

Examples:

- `thread.<id>.request`
- `thread.<id>.response`
- `worker.<id>.heartbeat`
- `worker.<id>.progress`
- `artifact.created`
- `scheduler.dispatch`
- `run.cancel`
- `run.retry`
- `run.escalate`

### General work-state observability

The bus must support broad observability of work state, not just final responses.

Required observability includes:

- claimed / running / blocked / done state transitions
- heartbeats for long-running agents
- progress events
- provider/model selection events
- cost and token accounting events
- artifact creation events
- retry / escalation events

### Mixed runtime model

The system must support both one-shot and long-running agents.

#### One-shot agents

Properties:

- consume one durable work item
- perform bounded work
- emit structured result(s)
- exit after completion

Typical uses:

- isolated coding worker
- verifier
- reviewer
- synthesis worker
- fetch/search worker
- remote Incus or Firecracker task worker

#### Long-running agents

Properties:

- hold stable identity
- stay subscribed to one or more topics
- maintain ephemeral local caches or coordination state
- emit heartbeats and progress events

Typical uses:

- scheduler
- thread coordinator
- TUI companion/operator-facing agent
- provider/broker coordination services
- backlog and priority maintenance agents

### Runtime mode requirement

Workers/agents should declare a runtime mode such as:

- `one_shot`
- `long_running`

This mode should influence:

- subscription behavior
- heartbeat requirements
- retry semantics
- lease/liveness expectations
- cache allowances
- delegation permissions

### Cheap recursive delegation goal

The architecture should make recursive delegation cheap enough that most recursion happens through topic emission and reply handling, not through special-case orchestration code.

This should allow patterns such as:

- a research agent consuming `research.request` and emitting work to `web.fetch.request`
- a coding orchestrator consuming `code.task.request` and emitting to `review.request`
- a synthesis worker consuming `artifact.created` and emitting to `wiki.update.request`

### Design implication for next spec

The next concrete spec should emphasize:

- message envelope schema
- topic taxonomy
- thread registry mapping onto message flows
- observability/state event model
- depth, fanout, budget, and TTL guards for recursive message-driven delegation
- one-shot versus long-running runtime behavior
