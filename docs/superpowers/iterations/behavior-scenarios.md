# Behavior Scenarios

## Journey Scenarios

## JOURNEY-0001 — Complete one-shot lifecycle: directive to completion (walking skeleton)

**Kind:** journey
**Proof seam:** e2e
**Owning stories:** STORY-0057, STORY-0050, STORY-0051, STORY-0052, STORY-0019, STORY-0065, STORY-0066, STORY-0058, STORY-0063

**Preconditions:**
- Daemon is running and connected to queue
- Golden container image exists and is accessible
- Shared nix cache volume is mounted and populated
- Target repository is accessible
- Template runner is valid and authorized
- External grader is configured

**Steps:**
1. Daemon claims next directive with priority lease
   → Directive is atomically reserved
   → No other worker can claim same directive
   → Lease is held for directive lifetime
2. Daemon validates template against allowlist and origin
   → Template identity matches allowlist entry
   → Template origin passes verification
   → Validation completes without error
3. Daemon copies golden image to fresh-<name> container
   → New ephemeral container created
   → Container is in stopped state
   → Container name is unique
4. Daemon attaches shared volumes (nix cache, handoff store)
   → Nix cache volume mounted read-only at /srv/nix-shared
   → Handoff store volume attached
   → Volumes are accessible inside container
5. Daemon starts container and delivers repository
   → Container enters running state
   → Repository is cloned or bundled into container
   → Repository is at correct ref/branch
6. Daemon imports handoff context if handoff_in is present
   → ctx_handoff import executes on worker
   → Prior context is restored
   → No errors during import
7. Worker executes template runner (setup + serve + claude -p)
   → Lean-context setup completes
   → Lean-context serve starts successfully
   → Claude agent invoked with template prompt
   → Agent completes and produces output
8. Daemon harvests worker.diff and result.json artifacts
   → worker.diff is extracted and stored
   → result.json is extracted and stored
   → Agent diary/knowledge is captured
   → All artifacts are readable and valid
9. Daemon runs authoritative external grade on clean checkout
   → Fresh system-of-record checkout obtained
   → Worker patch applied via context-anchored-patching
   → External grader produces pass/fail signal
   → Grade result is authoritative
10. Coordination loop evaluates grade result
   → If pass: directive moves to done state
   → If fail: escalation ladder is triggered
   → Decision is logged
11. Daemon stops container and reaps instance
   → Container is stopped gracefully
   → Instance is deleted by reaper
   → Shared volumes remain intact
   → Decision log entry is written

**Final observables:** (✅ = asserted by the ITER-0000 harness; ⏳ = deferred subsystem, not yet asserted)
- ✅ Directive state is done or escalated-to-<level>
- ⏳ Decision log contains complete audit trail — DEFERRED: the D6 decision log is STORY-0063 AC-28 → ITER-0001 (ITER-0000 emits a plain stderr line). No harness assertion yet.
- ✅ Worker instance no longer exists
- ⏳ Shared volumes are clean and ready for next directive — DEFERRED: real-backend property (volume attach/detach lifecycle) → ITER-0005; the fake backend has no volumes to assert.
- ✅ Result artifacts (diff, result.json, knowledge) are persisted (worker.diff + result.json; "knowledge" capture deferred with the lean-ctx bridge → ITER-0003)

**Automation status:** automated (fake backend, CI) + manually validated E2E on cluster
**Execution command:** `cd modules/incus-dispatcher && go test . -run TestJourney0001` (nested go.mod — must run inside the module dir)

**Evidence:**
- Automated harness: `modules/incus-dispatcher/journey_test.go` — `TestJourney0001_OneShotLifecycle`
  drives the real `Daemon` + `DefaultMapToTask` against a recording fake backend and asserts the
  daemon-seam final observables (done outcome, queue drained, instance reaped exactly once, authoritative
  grade present+passing, worker.diff + result.json harvested) plus the contracted phase order with
  teardown strictly last. The two ⏳ observables above are intentionally NOT asserted (deferred subsystems).
  `TestJourney0001_RejectedDirectiveNeverLaunches` proves the D1 gate (step 2 blocks step 3: a worker
  proposing a privileged template never launches the backend and is never reaped).
- Mutation coverage for the authoritative-grade rule: `daemon_test.go` —
  `TestRunOnce_GradePatchNotAppliedIsFail` (patch-not-applied ⇒ fail) and
  `TestRunOnce_FrameworkErrorIsFail` (framework/infra error ⇒ fail) pin `passed()`.
- Manual E2E (EXIT b, 2026-06-18): headless claude on a NixOS worker implemented `queue.Peek()`;
  authoritative clean-room grade passed 10/10 incl. 3 hidden oracle tests. See iteration-log.md ITER-0000.
- Real-backend wiring of `DefaultMapToTask` to the proven `nix develop ./fleet-worker
  --accept-flake-config --no-sandbox --command bash runner.sh` invocation is tracked as a
  cluster-evidence-gated follow-up — see roadmap ITER-0003 canonical runner modes.

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:310-325`

## JOURNEY-0002 — Live steering: high-priority directive preempts current work

**Kind:** journey
**Proof seam:** e2e
**Owning stories:** STORY-0057

**Preconditions:**
- A directive is currently being processed by a worker
- A new high-priority directive is pushed to the queue by orchestrator
- Current fast-start cycle is between one-shots

**Steps:**
1. Orchestrator pushes high-priority directive into queue
   → High-priority directive is inserted ahead of current work
   → Queue order is updated
2. Next fast-start one-shot cycle begins
   → Daemon checks queue and finds high-priority directive
   → High-priority directive is claimed instead of lower-priority work
3. High-priority directive is processed with prior handoff applied
   → Prior work's handoff context is available
   → High-priority directive runs to completion
   → Lower-priority work remains in queue

**Final observables:**
- High-priority directive completes
- Prior context from earlier attempts is preserved in handoff
- Lower-priority directives remain queued

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:327-328`

## JOURNEY-0003 — External grading reproduces 13→0 result

**Kind:** journey
**Proof seam:** e2e
**Owning stories:** STORY-0068

**Preconditions:**
- harvested /tmp/lvl1-focused.diff contains the proven worker fix
- dispatcher grading subcommand is implemented
- target ref is known (the let-go commit where the bug was introduced)

**Steps:**
1. invoke grader with --ref <target-ref> --diff /tmp/lvl1-focused.diff
   → grader clones target ref to a pristine checkout
   → grader applies worker diff (source files only) to checkout
   → grader runs `make generate`
2. grader runs oracle: go test -tags gogen_ir ./pkg/ir/ cluster count
   → cluster-A test output shows 0 failures (down from 13)
3. grader runs make check-generated
   → exit 0
   → generated artifacts pass integrity check
4. grader runs untagged go test ./...
   → exit 0
   → all tests pass
5. grader emits grade JSON
   → grade JSON contains {passed: true, clusterA: 0, check_generated: true, untagged_fails: 0, e2e: true}

**Final observables:**
- structured grade JSON is produced with clusterA: 0 (the canonical 13→0 result)
- all oracle gates (check_generated, untagged, e2e) show success
- grader output is deterministic (same ref + same diff always produces the same grade)

**Automation status:** partial. AC-1 (the grader mechanism + grade-JSON shape {passed,clusterA,
check_generated,untagged_fails,e2e}, including generated-artifact exclusion and the 13-failure cluster-A
parsing) is AUTOMATED IN CI against synthetic fixtures. AC-2 (the let-go 13→0 reproduction) is PENDING on
the cluster: refs are PINNED (fix #249=23bfd87f1, pre-fix target=parent d4c36cf2d; see
testdata/journey0003/README.md). **Finding (2026-06-20):** applying the captured FOCUSED diff to the
parent ref + local go1.26.4 `make generate` regenerates a lowered `core_go_lowered/test/test.go` that fails
to compile (generator fallbacks for register-test!/use-fixtures) — local repro is toolchain/diff-scope
sensitive, so AC-2's green must run on the nix-pinned cluster worker (its declared e2e seam), not the Mac.
**Execution command (CI, AC-1):** `cd modules/incus-dispatcher && go test -run 'Grade|RunGrade' .`
**Repro command (cluster, AC-2):** `incus-dispatcher grade --checkout <clean let-go @ d4c36cf2d> --diff modules/incus-dispatcher/testdata/journey0003/lvl1-focused.diff`

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:52-65`

## JOURNEY-0004 — Mac-off: daemon claims and runs task offline

**Kind:** journey
**Proof seam:** e2e
**Owning stories:** STORY-0074

**Preconditions:**
- Mac is disconnected (no operator, no interactive decisions)
- queue has tasks waiting
- daemon container is running on remote cluster

**Steps:**
1. daemon scans queue for claimable tasks
   → finds a task independent of human input
2. daemon claims task and begins execution
   → task state transitions to running
   → execution proceeds autonomously
3. task completes (command exits)
   → exit code is captured
   → result is stored

**Final observables:**
- task.state is running or completed
- execution proceeded without Mac input

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:416-418`

## JOURNEY-0005 — Mac-off: autonomous grading without human feedback

**Kind:** journey
**Proof seam:** e2e
**Owning stories:** STORY-0074

**Preconditions:**
- Mac is disconnected
- task has completed execution
- grading system is configured for autonomous mode

**Steps:**
1. grading system evaluates task output
   → grading logic runs without waiting for human feedback
   → grade is assigned (pass/fail/inconclusive)
2. grading result is stored
   → result is persisted durably
   → no human confirmation required

**Final observables:**
- task has a grade
- grade was assigned autonomously
- no human-in-loop delay occurred

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:416-418`

## JOURNEY-0006 — Mac-off: low-cost escalations proceed autonomously, privileged in escalations lane

**Kind:** journey
**Proof seam:** e2e
**Owning stories:** STORY-0074

**Preconditions:**
- Mac is disconnected
- task result triggers escalation requirement
- escalation ladder has mixed costs (low, high)

**Steps:**
1. escalation system evaluates rung cost and policy
   → low-cost rung (e.g., retry) is pre-approved
2. low-cost escalation executes autonomously
   → execution proceeds without human involvement
3. privileged rung (e.g., resource override) is encountered
   → rung is routed to escalations lane
   → task does not proceed autonomously past this rung
4. escalation queues in escalations lane for human review on Mac return
   → escalation is durable and survives disconnection

**Final observables:**
- low-cost escalation was executed
- privileged escalation is queued, not blocked
- no human on Mac prevented either action

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:416-418`

## JOURNEY-0007 — Mac-off: successor resumes via handoff without replay

**Kind:** journey
**Proof seam:** e2e
**Owning stories:** STORY-0074

**Preconditions:**
- Mac is disconnected
- task is in progress or has completed
- predecessor context includes handoff metadata (decisions, state)

**Steps:**
1. task completes or is interrupted
   → handoff context (decisions made, work completed) is serialized
2. next task instance launches (same or related task)
   → handoff context is deserialized
   → predecessor decisions are available
3. successor task runs with knowledge of predecessor
   → completed work is not replayed
   → successor resumes from appropriate point

**Final observables:**
- successor task launched
- handoff context was used
- no replay of completed work observed

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:416-418`

## Surface Scenarios

## SCENARIO-0001 — Dispatcher recovers mid-flight after Mac host restart

**Kind:** failure-recovery
**Proof seam:** e2e
**Owning stories:** STORY-0001, STORY-0006

**Preconditions:**
- Temporal plane holds durable scheduling state on ndn-desktop
- Coordinator daemon is running on live micro-VM
- Directive queue has pending work

**Action:**
- Mac host goes offline (sleep or restart)
- Mac host comes back online

**Expected observables:**
- Temporal state persists on ndn-desktop (cluster-resident)
- Coordinator daemon resumes draining queue
- Deferred work in Temporal becomes eligible and re-enters queue
- Retry backoff logic continues from checkpoint
- Work resumes without manual intervention
- No work is lost or duplicated
- Decision log shows continuous timeline

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:31-37,60-61`

## SCENARIO-0002 — Dispatcher drains queue with deterministic coordination

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0003

**Preconditions:**
- dispatcher serve --queue daemon is running on ndn-desktop
- Directive queue has actionable directives with priority and leasing
- Runner backend is initialized (container or micro-VM)

**Action:**
- Dispatcher drains next directive from queue
- Dispatcher runs coordination loop

**Expected observables:**
- Directive is resolved to launch template
- Runner interface is called with template
- Zero LLM calls in hot path
- Coordination logic is deterministic (same input → same output)
- Decision-log line is written
- Task is launched via appropriate backend
- Audit trail shows all coordination decisions
- No non-deterministic delays

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:45-50`

## SCENARIO-0003 — Worker launches from golden image without live build

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0005

**Preconditions:**
- Golden worker image exists (fleet-worker/flake.nix)
- Incus copy infrastructure is available
- Directive specifies task to run

**Action:**
- Dispatcher resolves directive to worker launch
- incus copy copies golden to fresh container with unique name
- Task runs inside worker

**Expected observables:**
- Runner backend selects golden image (no live build)
- Container has immutable root filesystem
- Container has writable /workspace and /tmp
- Copy completes quickly (no build)
- Task can read-only access golden packages
- Task can write to /workspace and /tmp
- Worker is one-shot (runs once, then cleaned up)
- No live Nix builds during worker launch
- Worker state is isolated from other tasks

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:55-58`

## SCENARIO-0004 — Durable micro-VM stays up across multiple task executions

**Kind:** surface
**Proof seam:** process-level
**Owning stories:** STORY-0007, STORY-0008

**Preconditions:**
- Live micro-VM is running inside agent-host incus container
- Coordinator daemon and queue client are running inside VM
- Warm /nix store is populated

**Action:**
- Dispatcher launches task unit A inside live VM
- Unit A is torn down (killed, not VM deleted)
- Dispatcher launches task unit B inside same live VM

**Expected observables:**
- Unit A runs and completes
- VM stays up
- Warm /nix store is still present
- Coordinator daemon is still running
- Queue client can immediately fetch next work
- Unit B spins up in sub-seconds (warm store)
- No re-initialization of coordinator or daemon
- VM uptime persists across multiple units
- No incus delete calls in hot path (D5 hang resolved)
- Spin-up time for second and subsequent tasks is sub-second

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:65-79`

## SCENARIO-0005 — Trusted lane task uses Fast (namespace) isolation

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0021

**Preconditions:**
- Template declares isolation tier = Fast
- Task is in a trusted lane (e.g., internal CI)
- Live VM with warm /nix store is running

**Action:**
- Dispatcher resolves directive with Fast isolation
- nspawn container is created with shared VM kernel namespace
- Task runs inside Fast container

**Expected observables:**
- Backend selects nspawn --ephemeral container
- Container has namespace isolation (not hardware)
- Container pulls packages from warm /nix store
- Spin-up completes in sub-seconds
- Task runs under namespaces (pid, net, ipc, mnt)
- Task shares VM kernel with other Fast containers
- Container is cost-efficient for cheap iterations
- Isolation is sufficient for trusted code
- Teardown is quick (kill container process)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:81-87`

## SCENARIO-0006 — Sensitive lane task uses Hard (hardware) isolation

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0022

**Preconditions:**
- Template declares isolation tier = Hard
- Task is in sensitive lane (e.g., trading-platform domain)
- Firecracker and microvm.nix infrastructure is available

**Action:**
- Dispatcher resolves directive with Hard isolation
- Per-task microVM is created
- Task runs inside Hard VM

**Expected observables:**
- Backend selects per-task Firecracker microVM
- VM gets its own kernel and scheduler
- Optional: NixOS container wraps the microVM unit
- Boot-readiness probe completes (hundreds of ms)
- Task cannot escape hardware boundary
- Task cannot interfere with other VMs
- VM provides hardware-level isolation
- Spin-up cost (hundreds ms) is amortized over task lifetime
- Trust domain (e.g., trading) is protected from other tasks

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:81-87`

## SCENARIO-0007 — Multi-tenant execution isolated by VM per trust domain

**Kind:** surface
**Proof seam:** e2e
**Owning stories:** STORY-0024

**Preconditions:**
- Multiple trust domains are present (e.g., platform A, platform B)
- Each domain gets its own live micro-VM on ndn-desktop
- Directive queue routes tasks to appropriate domain VM

**Action:**
- Domain A directive launches task inside Domain A VM
- Domain B directive launches task inside Domain B VM

**Expected observables:**
- Domain A disposable unit runs inside Domain A's durable VM
- Warm /nix store is isolated per VM
- Domain B disposable unit runs inside Domain B's durable VM
- No cross-VM resource sharing
- Domain A and Domain B tasks cannot interfere with each other
- Each domain has its own coordinator daemon (one per VM)
- Multi-tenancy is enforced by VM isolation topology

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:88-89`

## SCENARIO-0008 — Benchmark shows nspawn spin-up time with boot-readiness probe

**Kind:** surface
**Proof seam:** process-level
**Owning stories:** STORY-0025

**Preconditions:**
- Live VM with warm /nix store is running
- nspawn and boot-readiness probe infrastructure is available
- Benchmark harness is prepared

**Action:**
- Benchmark spawns nspawn --ephemeral container 100 times
- Measure wall-clock time from spawn to readiness

**Expected observables:**
- Each container starts with warm /nix store
- Boot-readiness probe checks when container is fully ready (not just started)
- Time is recorded per-run
- Statistics (mean, p50, p99) are computed
- nspawn spin-up time is sub-second (e.g., <500ms p99)
- Variance is low (warm store is reliable)
- Result informs decision: Fast tier is viable for trusted lanes

**Automation status:** measured (spike, cluster/manual) — nspawn `--ephemeral` boot-to-marker in a
nesting-enabled Incus NixOS container, warm /nix bind-ro. N=100: mean 76 ms, p50 76, p99 97, stddev
7.8 ms — sub-second confirmed, low variance. Results: `fleet-worker/spikes/STORY-0025-benchmark-results.md`.
**Execution command:** `cd fleet-worker/spikes && ./bench-spinup.sh nspawn 100` (needs an Incus NixOS container with `security.nesting=true` on the ndn-desktop host)

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:91-94`

## SCENARIO-0009 — Benchmark shows per-task microVM spin-up time is not the limiting factor

**Kind:** surface
**Proof seam:** process-level
**Owning stories:** STORY-0025

**Preconditions:**
- Per-task Firecracker microVM infrastructure is available
- Boot-readiness probe infrastructure is available
- Benchmark harness is prepared

**Action:**
- Benchmark spawns per-task microVM 10 times
- Measure wall-clock time from spawn to readiness
- Analyze cost in context of task lifetime

**Expected observables:**
- Each VM gets its own kernel
- Boot-readiness probe checks when VM is fully ready (network, services)
- Time is recorded per-run
- Statistics are computed
- VM boot cost (hundreds ms) is compared against typical task duration
- Amortization factor is computed
- Per-task microVM spin-up is hundreds of milliseconds (e.g., 200-800ms)
- Boot cost is amortized over task lifetime (is not the limiting factor for Hard tier)
- nspawn spin-up time (not VM boot) is the signal for substrate selection

**Automation status:** measured (spike, cluster/manual) — Firecracker microVM stop→start→ready cycles
on agent-host. N=20: mean 1861 ms, p50 1811, p99 2134, stddev 139 ms. Amortization <0.7% for a 5–10 min
task — boot is NOT the limiting factor; nspawn (76 ms) is the substrate-selection signal. Results:
`fleet-worker/spikes/STORY-0025-benchmark-results.md`.
**Execution command:** `cd fleet-worker/spikes && ./bench-spinup.sh microvm 20` (on the ndn-desktop agent-host)

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:91-94`

## SCENARIO-0010 — Mac disconnected → fleet still claims, runs, grades, escalates; successor resumes via handoff

**Kind:** failure-recovery
**Proof seam:** e2e
**Owning stories:** STORY-0026

**Preconditions:**
- Queue, coordinator, and handoff store are running on ndn-desktop cluster
- One or more active work items are in queue (claimed but not yet finished)
- Mac client is online and connected

**Action:**
- User powers off the Mac (simulated by network isolation or shutdown)
- Cluster workers execute tasks against handoff store (no Mac interaction needed)
- Worker completes task and writes result + context to handoff store
- User powers Mac back on (or new client connects)

**Expected observables:**
- Queue continues serving incoming work requests from cluster-resident clients
- Coordinator continues managing active leases without interruption
- Cluster can still claim new tasks and assign them to workers
- Grading and result collection happen entirely on cluster
- State is persisted to cluster storage, not Mac storage
- Result is visible in queue; escalation path is defined (e.g., human review, next agent)
- Client can immediately resume work from lean-ctx handoff store
- No state loss; all completed and in-progress tasks are recoverable
- Queue event log shows no interruption during Mac downtime
- Coordinator lease table shows continuous tracking (no leases expired spuriously)
- Handoff store contains all task results from the downtime period
- Successor client reconnects and resumes from last known checkpoint

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:14-26`

## SCENARIO-0011 — Static endpoint injection: worker receives fixed llm-proxy and queue addresses from task spec

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0009

**Preconditions:**
- low-level-executor-task-spec template is populated with llm-proxy and queue endpoints
- Worker container is launched from that spec

**Action:**
- Executor reads task spec and extracts static service endpoints
- Worker process queries dnsmasq on br-microvm bridge for local name resolution
- Worker connects to queue to pull next work item

**Expected observables:**
- llm-proxy address is available (e.g., 10.88.0.1:4000)
- Queue address is available (e.g., 10.88.0.1:5000)
- dnsmasq resolves static hostnames (e.g., 'queue', 'llm-proxy') to injected IPs
- No external DNS or service discovery daemon is consulted
- Connection succeeds using static endpoint from task spec
- No discovery lookup fails or times out
- Worker's /etc/hosts or dnsmasq local zone contains injected service names
- Network trace shows queries only to br-microvm dnsmasq (10.88.0.1:53), not external resolvers
- Worker successfully pulls tasks from queue without any discovery-phase latency

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:354-363`

## SCENARIO-0012 — [BLOCKED-ON-SUBSTRATE-DECISION] Laneq-as-cluster-service: MCP clients reconnect after Mac downtime, queue state intact

**Kind:** failure-recovery
**Proof seam:** e2e
**Owning stories:** STORY-0010

**Preconditions:**
- laneq + MCP server runs on ndn-desktop with DB on host volume
- Mac and worker containers are remote MCP clients
- One or more tasks are in queue

**Action:**
- Mac MCP client is disconnected (Mac powered off or network isolated)
- Worker MCP client claims and processes tasks while Mac is offline
- Mac client reconnects

**Expected observables:**
- Server-side laneq process continues running; DB is not affected
- Queue state is fully persisted on host volume
- Queue leases are issued and tracked on ndn-desktop
- Tasks are dequeued and executed without Mac involvement
- MCP server re-establishes connection; no state loss
- Client can query queue state and observe completed/in-progress tasks
- laneq process uptime is continuous (no restart during Mac downtime)
- DB transaction log shows all queue operations persisted
- Mac client successfully resumes MPC communication with no missing commands

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:366-389`

## SCENARIO-0013 — [BLOCKED-ON-SUBSTRATE-DECISION] Network-native backend (Postgres/NATS): cluster coordinates independently; Temporal uses same DB

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0010

**Preconditions:**
- Postgres or NATS JetStream instance runs on ndn-desktop cluster
- Queue service is backed by this persistent store (not laneq)
- Temporal also uses Postgres for state

**Action:**
- Cluster-resident queue service queries backend for pending tasks
- Temporal needs to schedule or query workflow state
- Mac client connects and queries queue state

**Expected observables:**
- No network round-trip to Mac; all queries are local to cluster
- Multi-client HA is supported (e.g., multiple queue replicas can connect)
- Temporal connects to same Postgres instance; no separate DB overhead
- Queues and workflows coexist in one persistence layer
- Query succeeds; client sees consistent state
- Client has no special privileges or recovery overhead vs. workers
- Single Postgres instance (or NATS JetStream cluster) serves both queue and Temporal
- Network trace shows no external API calls outside ndn-desktop for coordination
- Queue CLI/MCP surface emulates laneq commands; skills remain usable unchanged or as patterns

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:366-389`

## SCENARIO-0014 — [BLOCKED-ON-SUBSTRATE-DECISION] Dedicated queue host: survives worker-capacity rebuilds, isolation from compute tier

**Kind:** surface
**Proof seam:** process-level
**Owning stories:** STORY-0010

**Preconditions:**
- Queue runs on its own persistent Incus container (separate from worker hosts)
- Worker host can be destroyed and rebuilt without affecting queue

**Action:**
- Worker-capacity container is marked for rebuild or destroyed
- Queue host is restarted (planned maintenance or failure recovery)
- Worker clients reconnect after capacity rebuild

**Expected observables:**
- Queue host process continues running unchanged
- Queue state is not affected
- State is recovered from persistent storage
- Worker clients can reconnect and resume claiming tasks
- Queue is immediately available; no grace period or backfill needed
- Leases from pre-rebuild tasks are still valid or cleanly expired
- Queue host has its own lifecycle independent of worker host
- Uptime metrics for queue and workers can be tracked separately
- Failure of workers does not cascade to queue

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:366-389`

## SCENARIO-0015 — Resume work on branch with existing thread

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0029, STORY-0030, STORY-0033

**Preconditions:**
- Thread exists with status=active and current_branch=feature-x
- Resume summary captures prior work and next step
- Last verified state documents successful test run

**Action:**
- Coordinator receives new work request for branch feature-x
- Coordinator checks workspace claim on feature-x
- Coordinator reconstructs context: diff, artifacts, verified result, open questions
- New run is created with current_branch=feature-x and thread_id pointing to existing thread
- Worker receives context packet with thread goal, prior work, and next step

**Expected observables:**
- Coordinator queries thread registry for existing thread on feature-x
- Lease is valid and owned by thread
- resume_summary is read from thread
- last_verified_state is available
- Run inherits thread context, does not treat branch as blank slate
- Worker can continue implementation without reinventing
- run.thread_id matches existing thread.thread_id
- run's input context includes resume_summary
- workspace is not reset
- no duplicate reinvention occurred

**Automation status:** passing (ITER-0004, integration seam, in-process fake backend)
**Execution command:** `cd modules/incus-dispatcher && go test . -run TestScenario0015`
(TestScenario0015_ResumeOnBranch + TestScenario0015_SupersedeRequiresDeclaration — composes ThreadStore +
WorkspaceRegistry + ReconstructResumeAudit + ContinueRun)

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:521-546`

## SCENARIO-0016 — Escalate to stronger model on verification failure

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0035, STORY-0038, STORY-0031

**Preconditions:**
- Run initialized with model=ollama-local, budget=cheap
- Model policy includes escalation rule: try cheap local first
- Verification requirements are defined

**Action:**
- Worker executes task with ollama-local model
- Verification fails (e.g., test suite returns non-zero exit code)
- Coordinator detects stumble signal and evaluates escalation rule
- New run is created with provider_instance=claude-code-main, model_id=claude-3-5-haiku
- Worker retries with stronger model

**Expected observables:**
- Run.provider_instance=ollama-local, model_id=ollama
- run.stumble_signals includes verification_failure
- Escalation rule matches: uncertainty or failure → stronger model
- Budget context carries forward, accounting includes both runs
- run.run_id is new, parent_run_id references prior run
- First run has stumble_signals=[verification_failure]
- Second run has parent_run_id=<first run id>
- Provider escalation is audited in message history
- Both runs contribute to accounting and success metrics

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:358-416, 433-445`

## SCENARIO-0017 — Long-running scheduler maintains priority queue

**Kind:** surface
**Proof seam:** process-level
**Owning stories:** STORY-0037, STORY-0013, STORY-0012

**Preconditions:**
- Scheduler agent runs in long_running mode
- Thread registry contains: urgent thread (priority=10), active thread (priority=5), incubating thread (priority=2)
- Incubating thread has not been scheduled in 7 days

**Action:**
- Scheduler subscribes to scheduler.dispatch topic
- Scheduler computes queue order: priority + aging_score
- After urgent and active threads progress, aging_score of incubating thread reaches threshold
- Scheduler emits work to thread.<id>.request topic for highest-priority thread
- Worker claims work, emits progress and completion

**Expected observables:**
- Scheduler emits heartbeat
- Urgent thread queued first
- active thread queued second
- Incubating thread is surfaced to prevent starvation
- Message includes thread_id, priority, aging_score
- Scheduler receives response, updates thread status
- Queue ordering reflects priority + aging
- No thread starves indefinitely
- Stale-thread resurfacing occurs
- Work distribution spans all queue classes

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:506-519, 767-820`

## SCENARIO-0018 — Capture and learn from repeated stumble pattern

**Kind:** failure-recovery
**Proof seam:** process-level
**Owning stories:** STORY-0031, STORY-0032

**Preconditions:**
- Three recent runs all failed with timeout after 30 seconds
- Genome includes bootstrap prompt for coding worker
- Mutation is in candidate status

**Action:**
- First run times out; stumble_signals=[timeout]
- Second run times out; stumble_signals=[timeout]
- Third run times out; stumble_signals=[timeout]
- Coordinator proposes mutation: increase worker prompt guidance or task timeout
- Next run uses mutated prompt in trial experiment
- Trial run completes successfully
- Mutation is promoted to status=active after human review or automated threshold

**Expected observables:**
- Run.status=failed, stumble_signals recorded
- Pattern detection identifies repeated timeout
- Threshold exceeded; mutation proposal is generated
- Mutation kind=prompt_tweak, source=learned, status=candidate
- Mutation evidence_refs links to prior run IDs
- Outcome measured: success_rate improves
- New runs use promoted prompt by default
- Genome entry has version, content_hash, source=learned, status=active
- Mutation is auditable and revertible
- Evidence trail links mutation to stumble signals
- Protected invariants (budget guardrails, secret handling) were not mutated

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:433-463`

## SCENARIO-0019 — Recursive delegation via message emission

**Kind:** surface
**Proof seam:** e2e
**Owning stories:** STORY-0012, STORY-0014

**Preconditions:**
- Research agent subscribed to research.request topic
- Web fetch worker subscribed to web.fetch.request topic
- Policy allows research agent to emit to web.fetch.request

**Action:**
- Coordinator emits work to research.request with goal='find recent trends in LLMs'
- Research agent receives message, determines it needs external search
- Research agent emits work to web.fetch.request with parent_run_id=research-1, goal='fetch LLM trend articles'
- Web fetch worker receives request and performs searches
- Web fetch worker emits response to research.request with correlation_id=corr-123
- Research agent synthesizes results and emits to research.response with correlation_id=corr-123

**Expected observables:**
- Message includes thread_id=research-1, correlation_id=corr-123
- Agent checks policy allows delegation
- Message includes depth=1, correlation_id=corr-123 (same), thread_id=research-1
- Worker has parent_run_id context
- Research agent correlates response to original request
- Coordinator receives synthesized result
- Message chain is auditable via correlation_id
- Delegation graph is reconstructible from parent_run_id
- Depth field prevents unbounded recursion
- No heavyweight in-memory orchestration was needed

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:699-839`

## SCENARIO-0020 — Worker accesses provider through broker proxy without exposing credentials

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0048

**Preconditions:**
- Worker is remote Firecracker microVM
- Provider API key is stored on Mac trust root
- Proxy service runs on host and brokers requests

**Action:**
- Coordinator creates run and sends context packet to worker
- Worker receives run context and needs to call provider API
- Proxy authenticates request and injects credential from Mac keystore
- Proxy forwards to real provider API
- Proxy returns response to worker

**Expected observables:**
- Context packet includes provider_instance=claude-code-main, NO raw API key
- Worker makes request to proxy surface (e.g., http://proxy:8080/models/list)
- Proxy adds Authorization header with real API key
- Worker never sees raw credential
- Response is streamed back
- Worker received no raw API key
- Proxy is the sole credential holder
- All requests are audited at proxy level
- Credential compromise is bounded to proxy host

**Automation status:** automated (ITER-0002, rescoped to container/proxy integration seam;
microVM host-socket isolation → ITER-0005)
**Execution command:** `cd modules/llm-proxy && go test -race -run TestScenario0020`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:127-128, 342-346`

## SCENARIO-0021 — Operator uses TUI to create, inspect, and manage threads

**Kind:** surface
**Proof seam:** app-level
**Owning stories:** STORY-0028

**Preconditions:**
- TUI is running on Mac
- Thread registry is populated with 5 threads
- Worker status is available

**Action:**
- Operator opens TUI
- Operator views thread details: goal, status, current branch, next step
- Operator creates new work item with title, goal, kind=coding
- Operator inspects worker state: worker_id, status, last heartbeat
- Operator reviews artifact from completed run
- Operator pauses a thread
- Operator requeues a thread

**Expected observables:**
- Queue view shows threads sorted by priority + aging
- resume_summary and last_verified_state are displayed
- New thread is created with status=queued
- Long-running workers show recent heartbeats
- Artifact metadata and content are displayed
- Thread status changes to paused, no new work is dispatched
- Thread status changes to queued, work is re-emitted
- TUI is responsive and displays real-time state
- All required operations succeed
- Operator has visibility into queue, workers, and artifacts

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:552-559`

## SCENARIO-0022 — Budget enforcement prevents runaway spending

**Kind:** failure-recovery
**Proof seam:** integration
**Owning stories:** STORY-0036, STORY-0032

**Preconditions:**
- Thread has budget_policy with per-thread limit of $10
- First run consumed $8
- Second run is about to be dispatched

**Action:**
- First run completes with spend=$8, recorded in run.budget_snapshot
- Coordinator considers second run for same thread
- Coordinator estimates second run cost: $5 (would exceed $10 limit)
- Coordinator rejects or escalates second run
- Operator reviews run and increases thread budget to $20
- Coordinator resumes second run

**Expected observables:**
- Run accounting is audited
- Coordinator sums prior spend on thread: $8
- Budget enforcement checks per-thread limit
- Second run is paused, status=blocked, reason=budget_exceeded
- Budget policy is updated
- Run proceeds with new budget context
- Spending is tracked at multiple levels: per-run, per-thread, per-provider
- Budget guardrails are enforced
- No run exceeds its budget without explicit approval
- Hard budget guardrails remain protected from mutation

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:387-416, 455-463`

## SCENARIO-0023 — One-shot worker consumes task, exits

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0013

**Preconditions:**
- Isolated coding worker is configured with runtime_mode=one_shot
- Task is queued on code.task.request topic
- Worker is ready to claim work

**Action:**
- Worker subscribes to code.task.request and claims one task
- Worker performs bounded work: implement feature, run tests, emit result
- Worker emits structured result to code.response topic
- Worker exits
- Coordinator receives response and updates thread

**Expected observables:**
- Message includes thread_id, run_id, goal, context packet
- Worker does not wait for responses or retry in-process
- Message includes correlation_id, artifact_refs, run_id
- Exit code reflects run success/failure
- Thread status is updated, artifacts are linked
- Worker claimed exactly one task
- Worker did not re-subscribe after completion
- Worker exited after emitting result
- No ephemeral cache or coordination state persisted

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:767-820`

## SCENARIO-0024 — Coordinator rejects superseding work without explicit declaration

**Kind:** failure-recovery
**Proof seam:** integration
**Owning stories:** STORY-0030

**Preconditions:**
- Thread exists with status=active, current_branch=feature-x, next_step='run tests'
- New work request arrives for same branch without supersedes field

**Action:**
- Coordinator receives new work request for feature-x
- New request does not include supersedes field or explicit restart reason
- Coordinator rejects the new request with error message
- Operator corrects request: either omit supersedes (to continue) or add supersedes with reason
- If omitting supersedes: new run continues from next_step
- If adding supersedes with reason: new thread is created, old thread superseded_by is set

**Expected observables:**
- Coordinator queries thread registry, finds active thread
- Coordinator detects potential reinvention
- Error includes: 'Thread thread-x is active on feature-x. To continue, omit supersedes. To restart, set supersedes=<thread-id> with reason.'
- Request is re-submitted
- Work proceeds on existing implementation
- Intent is explicit and auditable
- Silent reinvention is prevented
- Continuity intent is explicit in data model
- Thread relationships are auditable via supersedes/superseded_by

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:536-542`

## SCENARIO-0025 — D1: Worker directive with root flag is rejected

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0049

**Preconditions:**
- Worker identity W1 is known to daemon
- Template allowlist contains T1 (unprivileged) but not T1-root
- Directive from W1 proposes T1-root

**Action:**
- Daemon receives directive(origin=W1, proposed_template=T1-root, intent=...)

**Expected observables:**
- Daemon rejects directive
- Error logged indicating template not in allowlist or origin not permitted
- Directive rejected, no container launched
- W1 remains unprivileged

**Automation status:** automated (ITER-0002)
**Execution command:** `cd modules/incus-dispatcher && go test -race -run TestScenario0025`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:99-110`

## SCENARIO-0026 — D1: Directive body contains no access_cmd or root flag

**Kind:** surface
**Proof seam:** unit
**Owning stories:** STORY-0049

**Preconditions:**
- Directive schema is enforced

**Action:**
- Parse directive JSON payload

**Expected observables:**
- Payload fields: origin, intent, proposed_template (no access_cmd, no root flag)
- Validation succeeds if and only if these fields present
- Directive schema rejects payloads containing access_cmd or root

**Automation status:** automated (ITER-0002; unit seam — queue.ParseDirective)
**Execution command:** `cd modules/incus-dispatcher && go test -run TestParseDirective ./queue`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:99-105`

## SCENARIO-0027 — D1: Child directive from worker inherits immutable provisioning, not privileged

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0049

**Preconditions:**
- Parent directive from user to W1 with template T1
- W1 generates child directive with intent and task content
- W1 does not set root or privileged flag in child

**Action:**
- Daemon processes child directive from W1

**Expected observables:**
- Child inherits parent's template T1 (immutable)
- No escalation to privileged template permitted
- Task content preserved
- Child container launched with parent provisioning
- Child cannot elevate privileges beyond parent template capability

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:104-105`

## SCENARIO-0028 — D2: Backend interface abstracts container vs. micro-VM delivery

**Kind:** surface
**Proof seam:** unit
**Owning stories:** STORY-0017

**Preconditions:**
- Backend interface defined with Launch, Stop, Delete, GetStatus methods
- Container backend implementation (incus) written
- Micro-VM backend stub exists (placeholders for Firecracker)

**Action:**
- Call backend.Launch(template, args) via interface

**Expected observables:**
- Container backend launches incus container
- Micro-VM backend would launch Firecracker VM (gated by benchmark)
- Return same Instance handle from both
- Coordination loop invokes backend methods without knowing substrate
- Container and micro-VM backends pass same interface contract

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:117-128`

## SCENARIO-0029 — D2: Micro-VM boot-to-ready ≤ 5 s with closure realized

**Kind:** surface
**Proof seam:** process-level
**Owning stories:** STORY-0017

**Preconditions:**
- Firecracker VM running fleet-worker NixOS config
- Closure (dependencies) already realized on disk
- Boot-to-ready sentinel (e.g., multi-user.target) defined

**Action:**
- Power on Firecracker VM and measure to boot-to-ready

**Expected observables:**
- Startup completes in ≤ 5 seconds (measured: ~4.8 s)
- multi-user.target reached, systemd ready
- Boot latency acceptable for per-task hard-tier cost (not per-task fast-tier)
- Micro-VM teardown via systemctl stop is clean (no hang)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:140-158`

## SCENARIO-0030 — D3: ctx_agent diary write and read preserve progression state

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0018

**Preconditions:**
- Run 1 creates ctx_agent diary entry: decisions=[...], blockers=[...], progress=[...]
- Run 1 completes, lean-ctx persists handoff to shared volume

**Action:**
- Run 2 calls ctx_agent action=recall_diary

**Expected observables:**
- Diary returned with Run 1 decisions/blockers/progress intact
- Run 2 can read and continue from Run 1 state
- Soft state successfully carried between one-shots
- Loss of diary does not break correctness (code diff + grade still authoritative)

**Automation status:** automated (adapter seam — `LeanCtxProvider` argv/parse + a genuine diary
round-trip against a real `lean-ctx` in an isolated temp project; skips if `lean-ctx` is absent. The
session round-trip across one-shots was independently proven on a real cluster worker by the
STORY-0034 spike — SCENARIO-0077.)
**Execution command:** `cd modules/incus-dispatcher && go test . -run TestLeanCtxProvider`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:169-175`

## SCENARIO-0031 — D3: Authoritative state (diff + grade) independent of lean-ctx loss

**Kind:** failure-recovery
**Proof seam:** e2e
**Owning stories:** STORY-0018

**Preconditions:**
- Run 1 generates worker.diff (authoritative code change)
- Run 1 generates oracle JSON grade (authoritative execution result)
- lean-ctx handoff is lost or corrupted

**Action:**
- Run 2 begins without lean-ctx handoff (stale/missing)

**Expected observables:**
- Run 2 reads diff and grade from own artifacts
- Run 2 correctness is not affected by missing handoff state
- Run 2 produces same or better grade without handoff
- Anti-reward-hack: loss of soft state does not degrade execution correctness

**Automation status:** pending
**Execution command:** TBD — **ITER-0004 primary seam is CI unit/integration** (daemon-loop test, fake backend,
handoff bundle absent/corrupt → `passed()` still grades from `Result.ExternalGradingResult`). This keeps STORY-0018
AC-4 a *testable* AC and avoids an ITER-0003-style cluster carry-item. A cluster e2e run is optional enrichment, not
gating evidence.

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:177-182`

## SCENARIO-0032 — D4: Pass grade → mark thread done (no escalation)

**Kind:** surface
**Proof seam:** unit
**Owning stories:** STORY-0055

**Preconditions:**
- Grade returned from oracle: pass
- Coordination loop applies decision rules

**Action:**
- Daemon receives pass grade

**Expected observables:**
- Rule fires: pass → mark thread done
- No retry, no escalation, no human lane
- Thread state = done
- Decision logged to D6 log (directive id, pass, rule=pass→done, ts)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:191-192`

## SCENARIO-0033 — D4: Fail-transient grade → retry with temporal backoff

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0055

**Preconditions:**
- Grade returned: fail (transient, e.g., network timeout)
- Thread retry_count < max_retries

**Action:**
- Daemon receives fail (transient) grade

**Expected observables:**
- Rule fires: fail (transient) → retry same with backoff
- New directive requeued with exponential backoff delay
- No escalation
- Thread remains in queue with incremented retry_count
- Next attempt scheduled after backoff window
- Decision logged: directive id, fail (transient), rule=retry_with_backoff, ts

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:193`

## SCENARIO-0034 — D4: Fail-repeats grade → escalate to stronger worker model (pre-approved)

**Kind:** surface
**Proof seam:** process-level
**Owning stories:** STORY-0055

**Preconditions:**
- Grade returned: fail (repeats, e.g., same error after 3+ retries)
- Escalation ladder for workers defined: cheap_model → medium_model → strong_model
- New directive ready for stronger worker

**Action:**
- Daemon detects fail (repeats) after threshold

**Expected observables:**
- Rule fires: fail (repeats) → escalate worker to pre-approved rung (e.g., medium → strong)
- New directive queued with escalated worker model
- Thread transitioned to escalation rung (worker level)
- Decision logged: directive id, fail (repeats), rule=escalate_worker→strong_model, ts
- New directive retried with stronger model (pre-approved, no human approval)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:194-195`

## SCENARIO-0035 — D4: Fail-still grade → escalate resources/template (pre-approved hard-tier)

**Kind:** surface
**Proof seam:** process-level
**Owning stories:** STORY-0055

**Preconditions:**
- Grade returned: fail (still, e.g., strong model exhausted)
- Template escalation ladder defined: standard → hard-tier
- hard-tier template is pre-approved

**Action:**
- Daemon detects fail (still) after worker escalation exhausted

**Expected observables:**
- Rule fires: fail (still) → escalate resources to hard-tier template
- New directive queued with hard-tier template (micro-VM or larger container)
- Thread transitioned to resource escalation rung
- Decision logged: directive id, fail (still), rule=escalate_template→hard-tier, ts
- New directive retried with hard-tier resources (pre-approved)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:196-197`

## SCENARIO-0036 — D4: Authority-limit grade → escalate to human (non-blocking escalations lane)

**Kind:** failure-recovery
**Proof seam:** process-level
**Owning stories:** STORY-0055

**Preconditions:**
- Grade returned: authority/judgment limit (e.g., privileged rung needed, human decision required)
- Escalations lane exists (distinct durable state)
- Thread origin known and traceable

**Action:**
- Daemon detects authority-limit after all autonomous rungs exhausted

**Expected observables:**
- Rule fires: authority-limit → push to escalations lane
- Escalation entry created (threaded to origin, non-blocking)
- Main queue continues draining other lanes
- Thread moved to escalations lane (distinct durable state)
- Decision logged: directive id, authority-limit, rule=escalate_to_human, ts
- Fleet continues processing other work (non-blocking)
- Human can drain escalations lane on return (Mac-off-safe)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:198-202`

## SCENARIO-0037 — D4: Privileged rungs reachable only via human escalations lane

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0055

**Preconditions:**
- Template ladder includes root/sensitive templates
- Autonomous escalation rungs do not include root

**Action:**
- Daemon processes escalation ladder: transient → worker → resources → human

**Expected observables:**
- root template never directly escalated by autonomous rules
- root template only assignable when escalation reaches human lane
- Privileged rungs (root, sensitive) require human approval (D1 + D4 enforcement)
- Autonomous escalation ladder is capped at pre-approved hard-tier (no privilege)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:201-202`

## SCENARIO-0038 — D4: Stale human-pending escalations re-notified by Temporal (urgency rises)

**Kind:** failure-recovery
**Proof seam:** process-level
**Owning stories:** STORY-0055

**Preconditions:**
- Escalation in human lane created at T0
- Human has not cleared it by T0 + threshold
- Temporal timer fires to re-notify

**Action:**
- Temporal re-triggers notification at T0 + threshold, T0 + 2×threshold, ...

**Expected observables:**
- Escalation re-surfaced to human (urgency rises)
- Existing escalation preserved, no duplicate creation
- Human receives re-notification of pending escalation
- Escalation state remains durable until cleared
- Fleet continues processing (non-blocking)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:206`

## SCENARIO-0039 — D5: Stop container with timeout before delete

**Kind:** surface
**Proof seam:** unit
**Owning stories:** STORY-0062

**Preconditions:**
- Running container instance exists
- Stop timeout configured (e.g., 30 s)

**Action:**
- Teardown: call backend.Stop(instance, timeout=30s)
- After Stop succeeds: call backend.Delete(instance)

**Expected observables:**
- incus stop --timeout 30 instance OR client UpdateInstanceState(State=STOPPED, Timeout=30s)
- Container transitions to STOPPED state
- Container deleted without force (-f flag)
- No hang observed
- Orderly teardown: stop → wait → delete (no force-delete)
- Container cleanup completes without blocking coordination loop

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:214-215`

## SCENARIO-0040 — D5: Stop timeout → out-of-band reaper (non-blocking)

**Kind:** failure-recovery
**Proof seam:** process-level
**Owning stories:** STORY-0062

**Preconditions:**
- Stop command issued with timeout T
- Container does not transition to STOPPED within T

**Action:**
- Stop timeout expires; coordination loop detects timeout
- Reaper periodically attempts delete on timed-out instances

**Expected observables:**
- Instance moved to reaper queue (out-of-band process)
- Coordination loop does NOT block on teardown
- Reaper sweeps instances, cleans up eventual runaways
- Reaper runs asynchronously (does not block loop)
- Stop-timeout instance handled by reaper, loop unblocked
- Eventually reaped even if initial stop hangs

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:216-217`

## SCENARIO-0041 — D5: Launch via incus copy from golden with fresh names (prevent collision)

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0062

**Preconditions:**
- Golden snapshot prepared from fleet-worker NixOS config
- Instance name generation produces unique names (e.g., uuid or timestamp)

**Action:**
- Launch new container: incus copy golden <fresh_name>; incus start <fresh_name>

**Expected observables:**
- New container started from golden snapshot
- Name is unique, no collision with leaked instances
- Each launch gets unique instance name
- Leaked instances (if any) do not collide with fresh launches
- Reproducible state from golden snapshot

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:218-219`

## SCENARIO-0042 — D6: Decision log is append-only JSONL format

**Kind:** surface
**Proof seam:** unit
**Owning stories:** STORY-0056

**Preconditions:**
- Decision log writer interface defined
- Log backend = JSONL file on cluster storage

**Action:**
- Log coordination decision: directive_id, grade_summary, rule_fired, action, ts

**Expected observables:**
- Entry appended to log as single JSON line
- No in-place edits, no deletes (append-only)
- Log contains immutable, time-ordered coordination decisions
- Audit trail is tamper-obvious in v1 (future: cryptographic)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:223-224`

## SCENARIO-0043 — D6: Decision log entries contain directive, grade, rule, action, timestamp

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0056

**Preconditions:**
- Coordination loop fires a rule (e.g., pass, escalate_worker, human_escalation)

**Action:**
- Log entry created after rule decision

**Expected observables:**
- Entry includes: directive_id, grade_summary (pass|fail-transient|fail-repeats|fail-still|authority-limit), rule_name, action_taken, unix_timestamp
- D6 audit log fully describes coordination decisions
- All D4 ladder transitions logged (pass, retry, escalate_worker, escalate_template, escalate_human)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:223-227`

## SCENARIO-0044 — D6: Writer interface is swappable (JSONL → tamper-evident without rearchitect)

**Kind:** surface
**Proof seam:** unit
**Owning stories:** STORY-0056

**Preconditions:**
- Writer interface defined as trait/interface (Log(entry))
- JSONL backend implements interface (v1)
- Tamper-evident backend skeleton available

**Action:**
- Coordination loop calls writer.Log(decision_entry)

**Expected observables:**
- Call agnostic to backend (JSONL or HMAC-chained)
- Future: swap backend without loop logic change
- Log writer is pluggable interface (not hardcoded JSONL)
- Future tamper-evident upgrade requires only new backend, no loop rewrite

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:225-226`

## SCENARIO-0045 — Valid directive with all required fields accepted

**Kind:** contract
**Proof seam:** unit
**Owning stories:** STORY-0064

**Preconditions:**
- directive JSON is well-formed
- intent is a non-empty string
- template is in daemon allowlist
- origin is orchestrator or worker:<id>
- importance is valid enum
- lane, repo, ref, task are non-empty strings
- max_attempts is a positive integer

**Action:**
- daemon receives directive JSON
- daemon validates template against allowlist
- daemon validates deadline (if present)

**Expected observables:**
- directive is parsed successfully
- all required fields are present
- template is found in allowlist
- origin has permission to use this template
- deadline is ISO 8601 timestamp or absent
- directive is accepted and queued
- no validation errors returned

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:282-301`

## SCENARIO-0046 — Directive with access_cmd field rejected as malformed

**Kind:** failure-recovery
**Proof seam:** unit
**Owning stories:** STORY-0064

**Preconditions:**
- directive JSON includes access_cmd field
- all other fields are valid

**Action:**
- daemon receives directive with access_cmd field
- daemon validates directive schema

**Expected observables:**
- field is detected as prohibited
- schema validation fails
- error indicates access_cmd is not allowed
- directive is rejected
- error message states access_cmd is not permitted in directive (template defines execution)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:303`

## SCENARIO-0047 — Directive with root field rejected as malformed

**Kind:** failure-recovery
**Proof seam:** unit
**Owning stories:** STORY-0064

**Preconditions:**
- directive JSON includes root field
- all other fields are valid

**Action:**
- daemon receives directive with root field
- daemon validates directive schema

**Expected observables:**
- field is detected as prohibited
- schema validation fails
- error indicates root is not allowed
- directive is rejected
- error message states root is not permitted in directive (template defines execution)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:303`

## SCENARIO-0048 — Directive missing required field rejected

**Kind:** failure-recovery
**Proof seam:** unit
**Owning stories:** STORY-0064

**Preconditions:**
- directive JSON is missing one or more required fields (intent, template, origin, importance, lane, repo, ref, task, max_attempts)

**Action:**
- daemon receives incomplete directive
- daemon validates directive schema

**Expected observables:**
- required field is missing
- schema validation fails
- error identifies which required field(s) are missing
- directive is rejected
- error message lists missing required fields

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:276-301`

## SCENARIO-0049 — Directive deadline field is optional (absent => never urgent, Q4-eligible)

**Kind:** contract
**Proof seam:** unit
**Owning stories:** STORY-0064

**Preconditions:**
- directive JSON is valid except deadline field is absent
- all required fields are present and valid

**Action:**
- daemon receives directive without deadline field
- daemon processes deadline semantics

**Expected observables:**
- directive is parsed successfully
- absence of deadline means task is never urgent
- task is Q4-eligible (lower priority)
- directive is accepted
- deadline semantics are applied: never urgent, Q4-eligible

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:288`

## SCENARIO-0050 — Directive origin field is set by daemon, not author

**Kind:** contract
**Proof seam:** integration
**Owning stories:** STORY-0064

**Preconditions:**
- directive JSON is received from an author/client
- directive includes an origin field

**Action:**
- daemon receives directive with origin field from author
- daemon validates origin semantics

**Expected observables:**
- daemon detects origin field was provided by author
- daemon overwrites origin with authoritative value (orchestrator or worker:<id>)
- author-provided origin is ignored or triggers warning
- directive is processed with daemon-set origin
- origin reflects true source: orchestrator or worker:<id>

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:286`

## SCENARIO-0051 — Directive template is validated against daemon allowlist

**Kind:** contract
**Proof seam:** integration
**Owning stories:** STORY-0064

**Preconditions:**
- directive JSON includes a template field
- daemon has a configured allowlist of valid templates
- origin is provided (orchestrator or worker:<id>)

**Action:**
- daemon receives directive with template field
- daemon checks template against allowlist

**Expected observables:**
- template field is extracted
- template is found in allowlist
- origin has permission to use the template
- directive is accepted if template is in allowlist
- directive is rejected if template is not in allowlist or origin lacks permission

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:285`

## SCENARIO-0052 — Agents may only propose changes to directive importance/deadline; humans set freely

**Kind:** contract
**Proof seam:** integration
**Owning stories:** STORY-0064

**Preconditions:**
- directive has been created and queued
- importance or deadline fields need adjustment

**Action:**
- agent proposes modification to importance or deadline
- daemon evaluates agent proposal
- human operator modifies importance or deadline

**Expected observables:**
- modification request is submitted to daemon
- agent proposals are treated as suggestions
- daemon does not auto-apply agent proposals
- human has full authority to set values
- modification is applied without restrictions
- agent proposals cannot override directive fields
- humans can set importance/deadline without restrictions
- audit trail shows origin of each modification

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:304-306`

## SCENARIO-0053 — Pass grading leads to done state

**Kind:** surface
**Proof seam:** process-level
**Owning stories:** STORY-0058

**Preconditions:**
- External grading has been performed
- Grading result is pass

**Action:**
- Coordination loop receives pass signal from external grader
- Directive state transitions to done

**Expected observables:**
- Pass result is confirmed valid
- No escalation is triggered
- Directive no longer in active queue
- Result artifacts are archived
- Worker cleanup proceeds
- Directive state = done
- Decision log marks outcome as success
- Handoff is not re-pushed

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:322-324`

## SCENARIO-0054 — Fail grading triggers retry with same worker

**Kind:** failure-recovery
**Proof seam:** process-level
**Owning stories:** STORY-0058

**Preconditions:**
- External grading has been performed
- Grading result is fail
- Temporal workflow engine is available

**Action:**
- Coordination loop receives fail signal from external grader
- First escalation tier: retry-same pushes directive with backoff
- New worker claims retried directive

**Expected observables:**
- Fail result is confirmed valid
- Escalation ladder is invoked
- Directive is re-pushed to queue by Temporal
- Backoff delay is applied
- Fresh handoff bundle is prepared
- Retry is processed with same worker tier
- Prior handoff is available in fresh bundle
- New attempt begins
- Directive state = escalated-to-retry-same
- Temporal backoff is scheduled
- Fresh handoff bundle is enqueued with retry

**Automation status:** automated (AC-25 portion — daemon seam, fake backend; AC-24 Temporal portion → ITER-0007)
**Execution command:** `cd modules/incus-dispatcher && go test . -run TestRunOnce_RequeueEmitsFreshHandoff`

**ITER-0004 scope note (PAR round-2):** this scenario is the home for **STORY-0058 AC-25** ("a fresh handoff
bundle accompanies each retry"). ITER-0004 proves the *fresh-bundle-on-requeue* observables ("Fresh handoff bundle
is prepared / enqueued with retry / prior handoff available in fresh bundle") at the daemon seam with the fake
backend — no Temporal needed. The "Temporal backoff is scheduled / re-pushed by Temporal" observables remain
**STORY-0058 AC-24 → ITER-0007**. (Earlier roadmap text mislabeled this as a new "SCENARIO-0078"; that ID belongs
to deadline-prioritization/STORY-0045 — corrected to SCENARIO-0054.)

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:322-324`

## SCENARIO-0055 — Template validation rejects unauthorized template

**Kind:** failure-recovery
**Proof seam:** integration
**Owning stories:** STORY-0050

**Preconditions:**
- Allowlist is configured with authorized templates
- Directive contains unauthorized template

**Action:**
- Daemon retrieves directive with unauthorized template
- Template validation checks against allowlist
- Directive is rejected with error

**Expected observables:**
- Directive is claimed from queue
- Template identity does not match any allowlist entry
- Validation fails
- Clear error reason is provided
- No worker is launched
- Directive is moved to error queue
- Directive is not processed
- Error log contains rejection reason
- Security boundary is maintained

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:313`

## SCENARIO-0056 — Q2 item promoted to Q1 as deadline nears

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0043, STORY-0041

**Preconditions:**
- Directive has importance=high (Q2 tier)
- Directive has deadline 2 days in future
- Directive is in laneq but not-before is in future
- laneq.next currently returns a different Q1 item

**Action:**
- Time advances 1.5 days
- Call laneq.next

**Expected observables:**
- Temporal re-evaluates urgency and updates directive effective priority to Q1
- Temporal updates not-before to current time (now eligible)
- Directive is now eligible (not-before passed)
- Directive is returned as next item if importance >= current top item
- Directive moved from Q2 to Q1 by deadline aging
- No human intervention required
- Item is now eligible for provisioning

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:245, 266-268`

## SCENARIO-0057 — Agent rescore beyond bound is rejected; human rescore succeeds

**Kind:** failure-recovery
**Proof seam:** integration
**Owning stories:** STORY-0042

**Preconditions:**
- Directive has importance=Q4 (low)
- Agent has bounded rescore permission: may move at most 1 quadrant
- Human has unrestricted rescore permission

**Action:**
- Agent proposes rescore from Q4 to Q1 (jump of 3 quadrants)
- Human rescores same directive from Q4 to Q1

**Expected observables:**
- Rescore is rejected because jump exceeds bounded limit
- Directive remains at Q4
- Temporal does NOT re-project
- Rescore is accepted immediately
- Directive importance updated to Q1
- Temporal re-projects effective priority and not-before
- Agent self-promotion is bounded and rejected
- Human override authority is retained
- Temporal re-projection is triggered only by successful rescores

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:256-263`

## SCENARIO-0058 — No-deadline low-importance item never runs while higher-tier work exists

**Kind:** surface
**Proof seam:** process-level
**Owning stories:** STORY-0043, STORY-0041

**Preconditions:**
- Directive A: importance=Q4 (low), no deadline
- Directive B: importance=Q2 (high), deadline in 3 days
- Both directives in laneq

**Action:**
- Call laneq.next repeatedly over 1 week
- Clear all Q1, Q2, Q3 directives from laneq

**Expected observables:**
- Directive A never becomes eligible for provisioning
- Directive B becomes eligible as deadline nears
- Directive A remains in Q4 (idle-only by design)
- laneq.next now returns Directive A (only work remaining)
- A is provisioned only because no other work exists
- Q4 items are idle-only and never preempt higher-tier work
- No-deadline, low-importance work has lowest priority indefinitely
- Starvation by design is enforced

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:246, 266-268`

## SCENARIO-0059 — Rescore operation is the unified gateway for all priority changes

**Kind:** contract
**Proof seam:** integration
**Owning stories:** STORY-0041, STORY-0042

**Preconditions:**
- Directive has current (importance, deadline)
- Multiple actors (human, agent, scheduled) may initiate priority changes
- Temporal is the single writer of effective priority and not-before

**Action:**
- Human issues manual rescore: change importance from Q3 to Q1
- Agent proposes deadline extension via rescore
- Scheduled rescore (e.g., end-of-week bump) changes importance

**Expected observables:**
- Directive inputs are updated (importance field)
- Temporal re-projects effective priority and not-before
- laneq.next respects new priority
- Directive deadline field is updated if approved
- Temporal re-projects urgency based on new deadline
- Effective priority may shift (Q2→Q1 or vice versa)
- Directive inputs are updated
- Temporal re-projects; all downstream consumers (laneq, provisioner) see new effective priority
- All priority changes flow through unified rescore operation
- Temporal re-projection is triggered consistently
- No actor bypasses Temporal projection logic

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:256-264`

## SCENARIO-0060 — Worker PATH resolution via Go client exec

**Kind:** surface
**Proof seam:** app-level
**Owning stories:** STORY-0067

**Preconditions:**
- dispatcher is invoked with Go client runner (--runner client, the default)
- incus container is running with worker user home at /home/worker
- worker profile sources nix store paths and ~/.local/bin

**Action:**
- invoke dispatcher with --cmd 'which claude && claude --version'
- invoke dispatcher with --cmd 'go version'
- invoke dispatcher with --cmd 'lean-ctx --version'

**Expected observables:**
- exit 0
- output includes full path to claude binary
- output includes claude version string
- exit 0
- output includes Go version (e.g., go version go1.26)
- exit 0
- output includes lean-ctx version
- all three commands (claude, go, lean-ctx) resolve and execute via Go client, not shelling out to incus exec
- exit code is 0 for all invocations
- no exit 127 (command not found) errors

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:41-50`

## SCENARIO-0061 — lean-ctx bridge daemon enables shell-hook compression

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0069

**Preconditions:**
- runner is executing on a fresh container
- lean-ctx binary is available in PATH

**Action:**
- runner invokes `lean-ctx init --agent claude` (registers MCP server)
- runner invokes `lean-ctx setup` (fuller config than init)
- runner starts bridge daemon: `lean-ctx serve &`
- verify bridge status: `lean-ctx status`
- runner launches `claude -p` (spawns MCP server)
- worker executes shell commands (e.g., `git status`, `go build`)
- post-run: invoke `lean-ctx gain`

**Expected observables:**
- ~/.claude.json contains lean-ctx MCP server entry
- lean-ctx hooks are registered
- lean-ctx configuration is persisted
- status reports setup complete
- lean-ctx serve process runs in background
- bridge socket/endpoint is accessible
- output shows 'connected: true' or 'Bridge: ON'
- does NOT show 'Bridge: OFF — proxy not reachable'
- claude MCP server connects to lean-ctx bridge
- shell-hook compression is active
- commands are routed through ctx_shell (not raw Bash)
- compression is applied
- output shows non-zero measured savings number
- status is 'Bridge: ON' (not OFF)
- reports tools compressed through lean-ctx
- lean-ctx gain reports measured token savings (e.g., '68 ctx_shell calls, 27 ctx_read calls')
- bridge is ON throughout the run
- worker routed all shell and read operations through lean-ctx

**Automation status:** smoke (cluster). Bridge ON + measured savings proven on a real worker
(STORY-0069 landed e6b847e: lean-ctx init+setup+serve --daemon + compression proxy on :4444 routed
via ANTHROPIC_BASE_URL; OAuth Bearer forwards transparently; "Tokens saved 376", no "Bridge: OFF").
Seam corrected unit→integration. Reusable regression harness in-repo.
**Execution command:** `bash fleet-worker/spikes/leanctx-runner-smoke.sh` (needs ~/.fleet-token +
the cluster remote; see fleet-worker/spikes/README.md for the proven recipe).

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:67-78`

## SCENARIO-0062 — Heartbeat projects ctx_shell as the active command, not Bash

**Kind:** surface
**Proof seam:** app-level
**Owning stories:** STORY-0071

**Preconditions:**
- events.jsonl is being written by the worker
- working-state projector is running (scripts/working-state.py or dispatcher module)
- worker is using lean-ctx (ctx_shell calls dominate)

**Action:**
- working-state projector reads events.jsonl
- worker executes ~1500 events with ctx_shell commands (the prior run's observation)
- projector emits heartbeat with last_shell_cmd
- check heartbeat does NOT show '(no shell yet)' while worker is running
- projector derives phase_guess from brief gate commands

**Expected observables:**
- parses tool_use events where name == 'ctx_shell' or name == 'Bash'
- prioritizes ctx_shell over Bash
- events.jsonl contains many tool_use entries with name == 'ctx_shell'
- few or no Bash tool_use entries
- last_shell_cmd reflects the most recent ctx_shell invocation (not Bash)
- includes the actual shell command (e.g., 'go build ./...')
- includes timestamp of the command
- heartbeat accurately reflects work-in-progress
- last_shell_cmd is recent (within last few seconds/events)
- phase_guess == 'compile' when last_shell_cmd matches 'go build' pattern
- phase_guess == 'oracle' when last_shell_cmd matches 'go test.*pkg/ir' pattern
- phase_guess == 'regress' when last_shell_cmd matches 'make check-generated' or 'go test \./\.\.\.' pattern
- heartbeat accurately shows the worker's last shell command via ctx_shell (not stale or missing)
- phase_guess correctly tracks the brief's execution phases
- eventCount, Δsince_last, and alive status are all accurate

**Automation status:** automated (CI) — projector (AC-1) + heartbeat renderer (AC-2). AC-2 live
heartbeat-print during a real worker run is the cluster integration confirmation (leanctx-runner-smoke).
**Execution command:** `cd modules/incus-dispatcher && go test -run 'WorkingState|RenderHeartbeat' .`
(workingstate_test.go: projector parses ctx_shell/ctx_read, ctx_* preferred over Bash, phase_guess;
heartbeat_test.go: RenderHeartbeat surfaces the last ctx_shell cmd + age and never falsely
'(no shell yet)' while active).

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:96-104`

## SCENARIO-0063 — Worker truncation is handled by fallback result and external grader

**Kind:** failure-recovery
**Proof seam:** process-level
**Owning stories:** STORY-0072

**Preconditions:**
- worker runs out of turns during final test phase (after work is correct)
- worker does NOT write result.json before terminating

**Action:**
- worker process terminates with rc=1 or timeout
- runner detects missing result.json on exit
- runner synthesizes fallback result.json
- orchestrator receives the fallback result.json
- orchestrator delegates grading to external grader
- grader produces authoritative grade JSON (even though worker's result was UNKNOWN)

**Expected observables:**
- no result.json is written by the worker
- runner captures output from the last oracle command (e.g., `go test ./...` output)
- result.json is written with {status: 'UNKNOWN', harvested_diff_path: '<path-to-worker.diff>'}
- orchestrator has structured output (not an error or null)
- grader runs independently of the worker's self-report
- grader applies the harvested diff and runs oracle gates
- grade JSON shows pass/fail independent of worker's truncation
- orchestrator uses grader's result, not worker's UNKNOWN result
- orchestrator does not fail or retry due to missing result.json
- external grader is the source of truth (not the worker's self-report)
- anti-reward-hack: worker truncation does not cause false negatives or block grading

**Automation status:** automated (CI) for both ACs. AC-1 fallback synthesis is in runner.sh
(`result.json` written with {status:UNKNOWN, rc, harvested_diff_path} when the worker wrote none),
smoke-validated on a real worker (leanctx-runner-smoke). AC-2 grader-is-truth is locked in CI:
TestGraderIgnoresWorkerSelfReport — a lying worker result.json claiming success still grades
Passed=false because RunGrade computes the verdict solely from its own gate runs.
**Execution command:** `cd modules/incus-dispatcher && go test -run 'Grader|RunGrade' .`
(grader-is-truth property); fallback synth covered by `fleet-worker/spikes/leanctx-runner-smoke.sh`.

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:106-112`

## SCENARIO-0064 — Orchestrator steers worker mid-run via file-feed

**Kind:** failure-recovery
**Proof seam:** process-level
**Owning stories:** STORY-0073

**Preconditions:**
- worker is running and polling a watched file between phase boundaries
- orchestrator can write to the watched file in the container

**Action:**
- worker reaches end of a phase (e.g., after `go build` gate)
- orchestrator detects worker has drifted (e.g., via heartbeat analysis)
- orchestrator writes steer message to watched file in container
- worker reads and processes the steer message at next phase boundary
- worker acknowledges steer message in events.jsonl

**Expected observables:**
- worker checks the watched steer file (e.g., /tmp/steer.json)
- orchestrator computes a correction message (e.g., 'revert to commit X and retry')
- steer message is a JSON object with {action: '...', details: '...'}
- worker parses the JSON
- worker takes the requested action (e.g., git reset, re-run)
- events.jsonl contains an entry like {type: 'steer_ack', message_id: '...', action: '...'}
- timestamp is within one phase boundary of the orchestrator's write
- orchestrator and worker communicate mid-run without restarting
- steer message is acknowledged in events.jsonl within one phase boundary
- worker corrects course based on orchestrator feedback

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:114-124`

## SCENARIO-0065 — NixOS golden is built once and reused for all tasks

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0075

**Preconditions:**
- incus is running on the host
- nix is available on the host (or in a builder container)
- fleet-worker/flake.nix exists and references llm-agents.nix

**Action:**
- build golden once: `nix develop ./fleet-worker --accept-flake-config -c echo 'closure realized'`
- launch NixOS container with the realized flake closure
- snapshot the golden container
- for task N, copy the golden
- inside task-N, run runner: `nix develop --command bash runner.sh`

**Expected observables:**
- nix fetches and caches claude-code, lean-ctx, go, make from cache.numtide.com (no local builds)
- closure is fully downloaded (substitution only, no build sandbox failures)
- echo succeeds
- incus launch images:nixos/25.11 nix-golden
- container boots without nix build-sandbox errors
- incus snapshot create nix-golden pristine
- incus copy nix-golden task-N
- task-N is a fresh clone of the golden (no rebuild)
- runner.sh executes (no re-fetching or building)
- lean-ctx setup+serve runs
- claude -p runs
- NixOS golden is built exactly once
- all tasks reuse the golden via incus copy (zero rebuild per task)
- no nix build-sandbox failures in unprivileged containers

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:149-159`

## SCENARIO-0066 — NixOS golden maintains clean-room integrity (byte-identical regen)

**Kind:** contract
**Proof seam:** e2e
**Owning stories:** STORY-0075

**Preconditions:**
- NixOS golden has been built and a task has run to completion
- worker diff is harvested
- oracle verification is ready

**Action:**
- grader starts with a pristine NixOS container (copy from golden)
- grader applies worker diff (source files only)
- grader runs `make generate` inside the NixOS golden
- grader compares regenerated artifacts to the harvested originals
- grader runs oracle gates (go test -tags gogen_ir, make check-generated, untagged, e2e)

**Expected observables:**
- fresh copy of the golden is created
- source files are copied wholesale (not patched)
- generated artifacts (core_compiled.lgb, core_go_lowered, generated.sums) are NOT copied
- regenerate exits 0
- core_compiled.lgb and core_go_lowered are freshly built
- core_compiled.lgb is byte-identical (clean-room proof)
- core_go_lowered artifacts are byte-identical
- generated.sums matches
- all gates pass
- exit codes are 0
- clean-room integrity gate holds: byte-identical regen of generated artifacts
- no Ubuntu stopgap fallback is needed
- NixOS golden is sufficient for end-to-end task execution and verification

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:149-159`

## SCENARIO-0067 — Provider routing allows cheap implementers with deterministic grading

**Kind:** surface
**Proof seam:** integration
**Owning stories:** STORY-0076

**Preconditions:**
- NixOS golden exports codex, gemini-cli, qwen-code from llm-agents.nix
- dispatcher accepts --provider and --model flags
- orchestrator can choose implementer vs grader models independently

**Action:**
- dispatcher is invoked with --provider openai --model gpt-4o-mini
- worker applies --provider ollama-cloud --model qwen-coder
- grader runs with --provider anthropic (deterministic oracle, no model)
- strong model (e.g., Claude 3.5 Sonnet) reviews the final graded diff

**Expected observables:**
- worker uses gpt-4o-mini for implementation
- worker routes to ndn.local:11434 via the proxy
- uses cheap Qwen coder for implementation
- grader is pure git-based verification (make generate + oracle tests)
- no LLM call for grading
- strong model is only used for final review, not for the entire task
- implementer can be cheap (Haiku, OpenAI, Ollama)
- grader remains deterministic (oracle is the source of truth)
- cost is minimized while rigor is preserved

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:161-165`

## SCENARIO-0068 — Built worker image exposes the curated skill set at the discovery path

**Kind:** surface
**Proof seam:** app-level
**Owning stories:** STORY-0077

**Preconditions:**
- worker image has been built with agent-skills-nix flake input
- bundle contains the ~13-skill subset (using-laneq, low-level-executor-task-spec, etc.)
- environment.etc."claude/skills" is configured to point to the bundle

**Action:**
- Launch worker container from built image
- Execute 'claude -p' inside container to discover skills path
- Verify skill files exist at discovery path
- Verify skills are immutable and from copy-tree (not symlinks)

**Expected observables:**
- Container starts successfully with NixOS system profile applied
- Skills discovery path resolves to /etc/claude/skills or equivalent system location
- All 13 skill SKILL.md files are present and readable at the discovery path
- Each skill directory matches expected naming (using-laneq, process-aware-done, etc.)
- Skills are regular files/directories, not symlinks
- Skills are owned by root with read-only permissions for container runtime
- claude -p output includes the skill discovery path pointing to bundled skills
- All 13 curated skills are accessible to Claude agents running in the worker container
- Skills are offline-available (no network fetch required for discovery)

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:332-347`

## SCENARIO-0069 — Worker image build captures skills bundle with correct layout and filter configuration

**Kind:** process-level
**Proof seam:** process-level
**Owning stories:** STORY-0077

**Preconditions:**
- agent-skills-nix is available as a flake input
- Upstream skills repo is hash-pinned in flake.lock
- subdir/idPrefix layout is documented

**Action:**
- Run nix flake check on worker configuration
- Invoke selectSkills/mkBundle to curate the 13-skill subset
- Build worker image system closure with bundled skills
- Verify copy-tree application (non-symlink copy)

**Expected observables:**
- Flake evaluation succeeds with agent-skills-nix input available
- Bundle derivation completes without evaluation errors
- Bundle output contains only the specified skills
- nix build produces a system closure with the skills bundle included
- environment.etc."claude/skills" paths are correctly materialized
- Built image closure shows copied files, not symlinks to flake inputs
- Closure is fully offline (all dependencies in /nix/store)
- nix flake check passes
- selectSkills/mkBundle completes successfully
- Built system closure includes immutable skills at /etc/claude/skills
- Image is reproducible and offline-available

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:332-347`

## SCENARIO-0070 — Daemon claim rule: task transitions from unowned to owned

**Kind:** contract
**Proof seam:** unit
**Owning stories:** STORY-0059

**Preconditions:**
- task exists in queue with owner=null
- temp queue DB initialized and accessible

**Action:**
- daemon instance executes claim for task

**Expected observables:**
- task.owner set to daemon_id
- task.owned_at timestamp recorded
- no other instance has same task owner
- task.owner equals daemon_id
- task is not claimable by other daemons

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:395`

## SCENARIO-0071 — Daemon lease rule: owned task extends ownership window

**Kind:** contract
**Proof seam:** unit
**Owning stories:** STORY-0059

**Preconditions:**
- task is owned by current daemon
- lease window is approaching expiration

**Action:**
- daemon executes lease renewal

**Expected observables:**
- task.leased_until extended by TTL
- intermediate state (in-progress flag, partial results) preserved
- no ownership transfer occurs
- task.leased_until > now + TTL
- task intermediate state unchanged

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:395`

## SCENARIO-0072 — Daemon requeue rule: task returns to unowned queue

**Kind:** contract
**Proof seam:** unit
**Owning stories:** STORY-0059

**Preconditions:**
- task is owned by daemon
- daemon releases task (e.g., error, timeout, normal completion)

**Action:**
- daemon executes requeue

**Expected observables:**
- task.owner set to null
- task.leased_until cleared
- task re-enters unowned queue
- retry count incremented if applicable
- task.owner is null
- task is now claimable by any daemon
- retry count reflects the requeue

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:395`

## SCENARIO-0073 — Daemon park rule: task enters durable hold state

**Kind:** contract
**Proof seam:** unit
**Owning stories:** STORY-0059

**Preconditions:**
- task requires manual intervention or meets park criteria

**Action:**
- daemon executes park

**Expected observables:**
- task.state set to parked
- task.owner cleared
- task no longer appears in active queue
- park reason recorded in metadata
- task.state equals parked
- task does not re-enter queue until manually unparked
- park reason is queryable

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:395`

## SCENARIO-0074 — Template allowlist: worker-origin privileged template denied

**Kind:** contract
**Proof seam:** integration
**Owning stories:** STORY-0053

**Preconditions:**
- allowlist is configured with worker-origin restrictions
- worker submits proposal with privileged-template intent

**Action:**
- daemon evaluates worker-origin privileged-template proposal

**Expected observables:**
- proposal.origin validation detects worker origin
- privileged-template access is denied
- denial reason logged with proposal ID
- proposal.state is rejected
- rejection reason includes 'worker-origin not allowed for privileged templates'
- audit log contains denial event

**Automation status:** automated (ITER-0002)
**Execution command:** `cd modules/incus-dispatcher && go test -race -run TestScenario0074`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:396-397`

## SCENARIO-0075 — Graceful container teardown: stop-timeout routes to reaper

**Kind:** failure-recovery
**Proof seam:** process-level
**Owning stories:** STORY-0060

**Preconditions:**
- container is running and daemon owns it
- stop command is issued with timeout T

**Action:**
- daemon sends stop signal to container
- container does not stop within timeout
- reaper dequeues stalled stop and forcefully kills container

**Expected observables:**
- stop command begins with timeout T
- daemon immediately routes task to reaper queue
- daemon loop continues processing other tasks
- no blocking wait
- container is eventually terminated
- daemon loop was never blocked
- daemon processed >= 1 other task while reaper handled stalled stop
- container is terminated
- no delete-hang regression observed

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:398-399`

## SCENARIO-0076 — Container backend interface: passes existing contract tests

**Kind:** contract
**Proof seam:** integration
**Owning stories:** STORY-0020

**Preconditions:**
- backend implementation is complete
- container_runner_test.go is available

**Action:**
- run container_runner_test.go against backend

**Expected observables:**
- all tests pass
- no interface violations detected
- test output shows 100% pass rate
- backend is confirmed to match interface contract

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:401`

## SCENARIO-0077 — Context handoff round-trip: validate spike unblocks feature

**Kind:** contract
**Proof seam:** integration
**Owning stories:** STORY-0034

**Preconditions:**
- ctx_handoff implementation is available
- two claude -p invocations can be chained on worker
- lean-ctx compression is enabled
- bridge communication is enabled

**Action:**
- invoke claude -p (iteration 1) with context and decision task
- serialize handoff context and pass to iteration 2
- invoke claude -p (iteration 2) with handoff context

**Expected observables:**
- produces a decision (e.g., choice of next action)
- decision is serialized into handoff context
- context is encoded without loss of decision state
- claude can retrieve the original decision from context
- decision matches iteration 1 output exactly
- round-trip produces identical decision state
- no data loss in compression/decompression cycle
- spike result gates feature for dogfood rollout

**Automation status:** passing (cluster spike, 2026-06-21) — manual/cluster-gated, not a CI sentinel
**Execution command:** `bash fleet-worker/spikes/leanctx-handoff-spike.sh` (clones golden, runs two
real `claude -p` invocations on a worker; probe: `fleet-worker/spikes/leanctx-handoff-probe.sh`)

**Evidence (2026-06-21):** VERDICT=PASS airtight. A 48-bit random nonce `HANDOFF_NONCE=cd3fbfee57b0`
injected into iteration-1 only was recorded via `lean-ctx session decision` + `session save`,
serialized to disk (`~/.local/share/lean-ctx/sessions/<id>.json`, independently confirmed to contain
the nonce), and recovered EXACTLY by iteration-2 — a separate `claude -p` process whose prompt never
contained the nonce. Guess probability ~2⁻⁴⁸. Compression+bridge enabled (proxy on :4444 forwarding the
OAuth Bearer transparently); large-payload compression itself is proven by the STORY-0069 chain spike.
**Implementation note for ITER-0004:** bare `lean-ctx session load` (id=`latest`) returns "starting
fresh"; recovery must resolve the explicit saved session id (or rely on auto-context injection), not
`load latest`. The decision persists on disk regardless.

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:402-404`

## SCENARIO-0078 — Prioritization: deadline approaching promotes Q2 to Q1

**Kind:** contract
**Proof seam:** unit
**Owning stories:** STORY-0045

**Preconditions:**
- task in Q2 with deadline T
- current time is approaching T (within promotion threshold)

**Action:**
- scheduler projects (importance, deadline) to effective priority
- scheduler assigns task to Q1

**Expected observables:**
- effective priority calculated based on deadline delta
- delta below threshold triggers Q2->Q1 promotion
- not-before gate allows task to compete for execution
- task.queue is Q1
- task is eligible for immediate execution

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:405-409`

## SCENARIO-0079 — Prioritization: no-deadline low-importance stays Q4 (idle-only)

**Kind:** contract
**Proof seam:** unit
**Owning stories:** STORY-0045

**Preconditions:**
- task has no deadline (deadline=null)
- task importance is low

**Action:**
- scheduler projects (importance=low, deadline=null) to effective priority
- scheduler assigns task to Q4

**Expected observables:**
- effective priority is lowest
- not-before gate is set to require idle queue
- task only runs when no higher-priority work available
- task.queue is Q4
- task.not_before requires idle state

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:405-409`

## SCENARIO-0080 — Laneq next: returns highest-importance eligible item only

**Kind:** contract
**Proof seam:** unit
**Owning stories:** STORY-0045

**Preconditions:**
- queue contains multiple tasks with varying importance and eligibility
- laneq.next() is called

**Action:**
- laneq.next() filters eligible tasks and ranks by importance

**Expected observables:**
- ineligible tasks (not-before gate not met) are skipped
- eligible tasks ranked by effective priority descending
- top item returned
- returned task has highest effective_priority among eligible items
- ineligible items are not considered

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:405-409`

## SCENARIO-0081 — Single-writer: only Temporal writes effective priority

**Kind:** contract
**Proof seam:** integration
**Owning stories:** STORY-0046

**Preconditions:**
- Temporal service is running
- multiple daemon instances are active
- task priority is being updated

**Action:**
- Temporal updates task.effective_priority
- non-Temporal actor attempts to update task.effective_priority
- multiple daemons read task.effective_priority concurrently

**Expected observables:**
- write succeeds with Temporal authorization
- write is rejected with authorization error
- all readers observe the same value
- no stale or torn reads
- no unauthorized writes to effective_priority detected
- concurrent reads are consistent

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:409`

## SCENARIO-0082 — Rescore authority: human can move item to any bucket

**Kind:** contract
**Proof seam:** integration
**Owning stories:** STORY-0047

**Preconditions:**
- task exists in current queue (e.g., Q2)
- human operator issues rescore command to move to target queue (e.g., Q1)

**Action:**
- human submits rescore request
- orchestrator evaluates human authority
- task is moved to target queue

**Expected observables:**
- request includes human credential/authorization
- human authority is confirmed
- no bounds check applied (human can move anywhere)
- task.queue = target
- not-before gate is updated for target queue
- task is in target queue
- no approval request was created
- rescore is immediately effective

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:410-412`

## SCENARIO-0083 — Rescore authority: agent rescore beyond bound rejected

**Kind:** contract
**Proof seam:** unit
**Owning stories:** STORY-0047

**Preconditions:**
- task is in current queue (e.g., Q3)
- agent is authorized to promote up to 1 queue level
- agent attempts to move task 2 levels up (beyond bound)

**Action:**
- agent submits rescore request to move from Q3 to Q1 (delta=2)
- orchestrator evaluates agent authority bound
- request is rejected

**Expected observables:**
- request includes agent credential and proposed delta
- agent bound is 1 level
- proposed delta=2 exceeds bound
- rejection decision is made
- task.queue unchanged
- rejection reason logged with agent ID and bound
- task remains in Q3
- no rescore was applied
- audit log includes agent/bound/delta/reason

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:410-412`

## SCENARIO-0084 — Rescore authority: privileged rescore routed to approval

**Kind:** contract
**Proof seam:** integration
**Owning stories:** STORY-0047

**Preconditions:**
- task is in current queue
- agent proposes rescore with privileged implication (e.g., bypass security gate)

**Action:**
- agent submits rescore request with privileged intent
- request is routed to approval queue
- human reviewer approves or denies

**Expected observables:**
- orchestrator detects privileged implication
- task.state = pending-approval
- approval request is enqueued with agent identity
- task does not move until approved
- approval result is logged with reviewer identity
- request was routed to human approval, not auto-rejected
- approval decision is visible to operator

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:410-412`

## SCENARIO-0085 — Escalation: autonomous climb through pre-approved rungs

**Kind:** contract
**Proof seam:** integration
**Owning stories:** STORY-0061

**Preconditions:**
- task has escalation state
- first rung is pre-approved (e.g., retry with backoff)

**Action:**
- escalation system evaluates first rung (low-cost, pre-approved)
- system climbs rung autonomously

**Expected observables:**
- rung cost is below autonomy threshold
- rung is pre-approved in policy
- escalation action executed without human intervention
- state transitions to next rung
- escalation rung was executed
- no approval request was created
- task state reflects rung climb

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:413-415`

## SCENARIO-0086 — Escalation: privileged escalation lands in escalations lane

**Kind:** failure-recovery
**Proof seam:** integration
**Owning stories:** STORY-0061

**Preconditions:**
- task requires privileged or judgment-based escalation
- multiple workflow lanes are active (work, approvals, escalations)

**Action:**
- escalation system detects privileged rung requirement
- escalation is enqueued in escalations lane
- human reviews escalation asynchronously

**Expected observables:**
- escalation is not autonomously executed
- task is moved to escalations lane
- other lanes (work, approvals) continue processing unblocked
- escalation review does not block other tasks
- task is in escalations lane
- other lanes show no processing delays
- escalation is queued for human review

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:413-415`

## SCENARIO-0087 — Escalation: stale escalation resurfaced by rising urgency

**Kind:** failure-recovery
**Proof seam:** integration
**Owning stories:** STORY-0061

**Preconditions:**
- escalation has been pending (old, not yet processed)
- task urgency increases (deadline approaching, new high-priority trigger)

**Action:**
- urgency metric rises for task
- old escalation is re-queued in escalations lane with new urgency rank

**Expected observables:**
- system detects rising urgency threshold breach
- escalation moves higher in priority order
- human reviewer sees it sooner
- stale escalation was resurfaced
- it ranks higher in escalations lane by urgency
- human reviewer will see it before new low-urgency escalations

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:413-415`

## SCENARIO-0088 — Mac-off: human-only escalations queue durably for Mac return

**Kind:** failure-recovery
**Proof seam:** e2e
**Owning stories:** STORY-0074

**Preconditions:**
- Mac is disconnected
- task escalates to a human-only rung
- escalations lane persists across container restarts

**Action:**
- human-only escalation is triggered (e.g., policy override)
- cluster runs independently for time period T
- Mac reconnects
- human reviews and resolves escalation

**Expected observables:**
- escalation is enqueued in durable escalations lane
- no autonomous action is taken
- escalation remains queued, unseen (Mac is offline)
- no error or hang occurs
- escalation is still present in queue
- human can review it
- resolution is applied to task
- human-only escalations survived disconnection
- they were processed after reconnection
- no escalations were lost or auto-resolved

**Automation status:** pending
**Execution command:** TBD

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:416-418`
