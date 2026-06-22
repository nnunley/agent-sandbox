# Behavior Corpus

Execution index for all behavior scenarios. Commands are TBD until the implementing iteration wires each scenario to a runnable seam.

| Scenario ID | Title | Proof seam | Run cadence | Command | Owning stories |
|---|---|---|---|---|---|
| JOURNEY-0001 | Complete one-shot lifecycle: directive to completion (walking skeleton | e2e | sentinel | `cd modules/incus-dispatcher && go test . -run TestJourney0001` | STORY-0057, STORY-0050, STORY-0051, STORY-0052, STORY-0019, STORY-0065, STORY-0066, STORY-0058, STORY-0063 |
| JOURNEY-0002 | Live steering: high-priority directive preempts current work | e2e | sentinel | TBD | STORY-0057 |
| JOURNEY-0003 | External grading reproduces 13→0 result | e2e | sentinel | AC-1 CI: `cd modules/incus-dispatcher && go test -run 'Grade\|RunGrade' .`; AC-2 cluster (refs pinned, pending): `incus-dispatcher grade --checkout <let-go@d4c36cf2d> --diff testdata/journey0003/lvl1-focused.diff` | STORY-0068 |
| JOURNEY-0004 | Mac-off: daemon claims and runs task offline | e2e | sentinel | TBD | STORY-0074 |
| JOURNEY-0005 | Mac-off: autonomous grading without human feedback | e2e | sentinel | TBD | STORY-0074 |
| JOURNEY-0006 | Mac-off: low-cost escalations proceed autonomously, privileged in esca | e2e | sentinel | TBD | STORY-0074 |
| JOURNEY-0007 | Mac-off: successor resumes via handoff without replay | e2e | sentinel | TBD | STORY-0074 |
| SCENARIO-0001 | Dispatcher recovers mid-flight after Mac host restart | e2e | iteration | TBD | STORY-0001, STORY-0006 |
| SCENARIO-0002 | Dispatcher drains queue with deterministic coordination | integration | iteration | TBD | STORY-0003 |
| SCENARIO-0003 | Worker launches from golden image without live build | integration | iteration | `bash fleet-worker/cluster-tests/run.sh golden-launch` | STORY-0005 |
| SCENARIO-0004 | Durable micro-VM stays up across multiple task executions | process-level | iteration | `bash fleet-worker/cluster-tests/run.sh durable-vm` | STORY-0007, STORY-0008 |
| SCENARIO-0005 | Trusted lane task uses Fast (namespace) isolation | integration | iteration | `bash fleet-worker/cluster-tests/run.sh nspawn-fast` | STORY-0021 |
| SCENARIO-0006 | Sensitive lane task uses Hard (hardware) isolation | integration | iteration | `bash fleet-worker/cluster-tests/run.sh hardtier` | STORY-0022 |
| SCENARIO-0007 | Multi-tenant execution isolated by VM per trust domain | e2e | iteration | `bash fleet-worker/cluster-tests/run.sh trust-boundary` | STORY-0024 |
| SCENARIO-0008 | Benchmark shows nspawn spin-up time with boot-readiness probe | process-level | spike | `cd fleet-worker/spikes && ./bench-spinup.sh nspawn 100` | STORY-0025 |
| SCENARIO-0009 | Benchmark shows per-task microVM spin-up time is not the limiting fact | process-level | spike | `cd fleet-worker/spikes && ./bench-spinup.sh microvm 20` | STORY-0025 |
| SCENARIO-0010 | Mac disconnected → fleet still claims, runs, grades, escalates; succes | e2e | iteration | TBD | STORY-0026 |
| SCENARIO-0011 | Static endpoint injection: worker receives fixed llm-proxy and queue a | integration | iteration | TBD | STORY-0009 |
| SCENARIO-0012 | [BLOCKED-ON-SUBSTRATE-DECISION] Laneq-as-cluster-service: MCP clients  | e2e | iteration | TBD | STORY-0010 |
| SCENARIO-0013 | [BLOCKED-ON-SUBSTRATE-DECISION] Network-native backend (Postgres/NATS) | integration | iteration | TBD | STORY-0010 |
| SCENARIO-0014 | [BLOCKED-ON-SUBSTRATE-DECISION] Dedicated queue host: survives worker- | process-level | iteration | TBD | STORY-0010 |
| SCENARIO-0015 | Resume work on branch with existing thread | integration | iteration | `cd modules/incus-dispatcher && go test . -run TestScenario0015` | STORY-0029, STORY-0030, STORY-0033 |
| SCENARIO-0016 | Escalate to stronger model on verification failure | integration | iteration | TBD | STORY-0035, STORY-0038, STORY-0031 |
| SCENARIO-0017 | Long-running scheduler maintains priority queue | process-level | iteration | TBD | STORY-0037, STORY-0013, STORY-0012 |
| SCENARIO-0018 | Capture and learn from repeated stumble pattern | process-level | iteration | TBD | STORY-0031, STORY-0032 |
| SCENARIO-0019 | Recursive delegation via message emission | e2e | iteration | TBD | STORY-0012, STORY-0014 |
| SCENARIO-0020 | Worker accesses provider through broker proxy without exposing credent | integration | iteration | `cd modules/llm-proxy && go test -race -run TestScenario0020` | STORY-0048 |
| SCENARIO-0021 | Operator uses TUI to create, inspect, and manage threads | app-level | iteration | TBD | STORY-0028 |
| SCENARIO-0022 | Budget enforcement prevents runaway spending | integration | iteration | TBD | STORY-0036, STORY-0032 |
| SCENARIO-0023 | One-shot worker consumes task, exits | integration | iteration | TBD | STORY-0013 |
| SCENARIO-0024 | Coordinator rejects superseding work without explicit declaration | integration | iteration | TBD | STORY-0030 |
| SCENARIO-0025 | D1: Worker directive with root flag is rejected | integration | iteration | `cd modules/incus-dispatcher && go test -race -run TestScenario0025` | STORY-0049 |
| SCENARIO-0026 | D1: Directive body contains no access_cmd or root flag | unit | iteration | `cd modules/incus-dispatcher && go test -run TestParseDirective ./queue` | STORY-0049 |
| SCENARIO-0027 | D1: Child directive from worker inherits immutable provisioning, not p | integration | iteration | TBD | STORY-0049 |
| SCENARIO-0028 | D2: Backend interface abstracts container vs. micro-VM delivery | unit | iteration | `cd modules/incus-dispatcher && go test . -run TestScenario0028` | STORY-0017 |
| SCENARIO-0029 | D2: Micro-VM boot-to-ready ≤ 5 s with closure realized | process-level | iteration | `bash fleet-worker/cluster-tests/run.sh microvm-boot` | STORY-0017 |
| SCENARIO-0030 | D3: ctx_agent diary write and read preserve progression state | integration | iteration | `cd modules/incus-dispatcher && go test . -run TestLeanCtxProvider` | STORY-0018 |
| SCENARIO-0031 | D3: Authoritative state (diff + grade) independent of lean-ctx loss | e2e | iteration | TBD | STORY-0018 |
| SCENARIO-0032 | D4: Pass grade → mark thread done (no escalation) | unit | iteration | `cd modules/incus-dispatcher && go test . -run TestRunOnce_Pass` | STORY-0055 |
| SCENARIO-0033 | D4: Fail-transient grade → retry with temporal backoff | integration | iteration | `cd modules/incus-dispatcher && go test . -run TestRunOnce_FailRequeues` | STORY-0055 |
| SCENARIO-0034 | D4: Fail-repeats grade → escalate to stronger worker model (pre-approv | process-level | iteration | `cd modules/incus-dispatcher && go test . -run TestRunOnce_LadderClimbsThenEscalates` | STORY-0055 |
| SCENARIO-0035 | D4: Fail-still grade → escalate resources/template (pre-approved hard- | process-level | iteration | `cd modules/incus-dispatcher && go test . -run TestRunOnce_LadderClimbsThenEscalates` | STORY-0055 |
| SCENARIO-0036 | D4: Authority-limit grade → escalate to human (non-blocking escalation | process-level | iteration | `cd modules/incus-dispatcher && go test . -run "TestRunOnce_LadderClimbsThenEscalates|TestRunOnce_HumanRungParksWithoutLane"` | STORY-0055 |
| SCENARIO-0037 | D4: Privileged rungs reachable only via human escalations lane | integration | iteration | `cd modules/incus-dispatcher && go test . -run "TestRunOnce_AutonomousRungDoesNotEscalate|TestJourney0001_RejectedDirectiveNeverLaunches"` | STORY-0055 |
| SCENARIO-0038 | D4: Stale human-pending escalations re-notified by Temporal (urgency r | process-level | iteration | TBD | STORY-0055 |
| SCENARIO-0039 | D5: Stop container with timeout before delete | unit | iteration | TBD | STORY-0062 |
| SCENARIO-0040 | D5: Stop timeout → out-of-band reaper (non-blocking) | process-level | iteration | TBD | STORY-0062 |
| SCENARIO-0041 | D5: Launch via incus copy from golden with fresh names (prevent collis | integration | iteration | TBD | STORY-0062 |
| SCENARIO-0042 | D6: Decision log is append-only JSONL format | unit | iteration | `cd modules/incus-dispatcher && go test . -run "DecisionLog|TestRunOnce_PassWritesReapThenDone"` | STORY-0056 |
| SCENARIO-0043 | D6: Decision log entries contain directive, grade, rule, action, times | integration | iteration | `cd modules/incus-dispatcher && go test . -run DecisionLog` | STORY-0056 |
| SCENARIO-0044 | D6: Writer interface is swappable (JSONL → tamper-evident without rear | unit | iteration | `cd modules/incus-dispatcher && go test . -run DecisionLog` | STORY-0056 |
| SCENARIO-0045 | Valid directive with all required fields accepted | unit | iteration | TBD | STORY-0064 |
| SCENARIO-0046 | Directive with access_cmd field rejected as malformed | unit | iteration | TBD | STORY-0064 |
| SCENARIO-0047 | Directive with root field rejected as malformed | unit | iteration | TBD | STORY-0064 |
| SCENARIO-0048 | Directive missing required field rejected | unit | iteration | TBD | STORY-0064 |
| SCENARIO-0049 | Directive deadline field is optional (absent => never urgent, Q4-eligi | unit | iteration | TBD | STORY-0064 |
| SCENARIO-0050 | Directive origin field is set by daemon, not author | integration | iteration | TBD | STORY-0064 |
| SCENARIO-0051 | Directive template is validated against daemon allowlist | integration | iteration | TBD | STORY-0064 |
| SCENARIO-0052 | Agents may only propose changes to directive importance/deadline; huma | integration | iteration | TBD | STORY-0064 |
| SCENARIO-0053 | Pass grading leads to done state | process-level | iteration | TBD | STORY-0058 |
| SCENARIO-0054 | Fail grading triggers retry with same worker | process-level | iteration | `cd modules/incus-dispatcher && go test . -run TestRunOnce_RequeueEmitsFreshHandoff` | STORY-0058 |
| SCENARIO-0055 | Template validation rejects unauthorized template | integration | iteration | TBD | STORY-0050 |
| SCENARIO-0056 | Q2 item promoted to Q1 as deadline nears | integration | iteration | TBD | STORY-0043, STORY-0041 |
| SCENARIO-0057 | Agent rescore beyond bound is rejected; human rescore succeeds | integration | iteration | TBD | STORY-0042 |
| SCENARIO-0058 | No-deadline low-importance item never runs while higher-tier work exis | process-level | iteration | TBD | STORY-0043, STORY-0041 |
| SCENARIO-0059 | Rescore operation is the unified gateway for all priority changes | integration | iteration | TBD | STORY-0041, STORY-0042 |
| SCENARIO-0060 | Worker PATH resolution via Go client exec | app-level | iteration | TBD | STORY-0067 |
| SCENARIO-0061 | lean-ctx bridge daemon enables shell-hook compression | integration | iteration | `bash fleet-worker/spikes/leanctx-runner-smoke.sh` (cluster smoke; needs ~/.fleet-token) | STORY-0069 |
| SCENARIO-0062 | Heartbeat projects ctx_shell as the active command, not Bash | app-level | iteration | `cd modules/incus-dispatcher && go test -run 'WorkingState|RenderHeartbeat' .` | STORY-0071 |
| SCENARIO-0063 | Worker truncation is handled by fallback result and external grader | process-level | iteration | `cd modules/incus-dispatcher && go test -run 'Grader|RunGrade' .` | STORY-0072 |
| SCENARIO-0064 | Orchestrator steers worker mid-run via file-feed | process-level | iteration | TBD | STORY-0073 |
| SCENARIO-0065 | NixOS golden is built once and reused for all tasks | integration | iteration | TBD | STORY-0075 |
| SCENARIO-0066 | NixOS golden maintains clean-room integrity (byte-identical regen) | e2e | iteration | TBD | STORY-0075 |
| SCENARIO-0067 | Provider routing allows cheap implementers with deterministic grading | integration | iteration | TBD | STORY-0076 |
| SCENARIO-0068 | Built worker image exposes the curated skill set at the discovery path | app-level | iteration | TBD | STORY-0077 |
| SCENARIO-0069 | Worker image build captures skills bundle with correct layout and filt | process-level | iteration | TBD | STORY-0077 |
| SCENARIO-0070 | Daemon claim rule: task transitions from unowned to owned | unit | iteration | `cd modules/incus-dispatcher && go test ./queue/ -run TestPark` | STORY-0059 |
| SCENARIO-0071 | Daemon lease rule: owned task extends ownership window | unit | iteration | TBD | STORY-0059 |
| SCENARIO-0072 | Daemon requeue rule: task returns to unowned queue | unit | iteration | TBD | STORY-0059 |
| SCENARIO-0073 | Daemon park rule: task enters durable hold state | unit | iteration | TBD | STORY-0059 |
| SCENARIO-0074 | Template allowlist: worker-origin privileged template denied | integration | iteration | `cd modules/incus-dispatcher && go test -race -run TestScenario0074` | STORY-0053 |
| SCENARIO-0075 | Graceful container teardown: stop-timeout routes to reaper | process-level | iteration | TBD | STORY-0060 |
| SCENARIO-0076 | Container backend interface: passes existing contract tests | integration | iteration | `cd modules/incus-dispatcher && go test . -run 'TestGenerateContainerName\|TestTaskValidation\|TestIsLocalPath\|TestRemoteFileRead\|TestContainerNameUniqueness\|TestRunTaskInContainer\|TestDeliverSourceViaClone\|TestRoundTripWithOutputArtifacts'` (integration cases self-skip when incus unreachable) | STORY-0020 |
| SCENARIO-0077 | Context handoff round-trip: validate spike unblocks feature | integration | cluster-gated (manual) | `bash fleet-worker/spikes/leanctx-handoff-spike.sh` (PASS 2026-06-21: nonce round-trips across two claude -p invocations, no data loss) | STORY-0034 |
| SCENARIO-0078 | Prioritization: deadline approaching promotes Q2 to Q1 | unit | iteration | TBD | STORY-0045 |
| SCENARIO-0079 | Prioritization: no-deadline low-importance stays Q4 (idle-only) | unit | iteration | TBD | STORY-0045 |
| SCENARIO-0080 | Laneq next: returns highest-importance eligible item only | unit | iteration | TBD | STORY-0045 |
| SCENARIO-0081 | Single-writer: only Temporal writes effective priority | integration | iteration | TBD | STORY-0046 |
| SCENARIO-0082 | Rescore authority: human can move item to any bucket | integration | iteration | TBD | STORY-0047 |
| SCENARIO-0083 | Rescore authority: agent rescore beyond bound rejected | unit | iteration | TBD | STORY-0047 |
| SCENARIO-0084 | Rescore authority: privileged rescore routed to approval | integration | iteration | TBD | STORY-0047 |
| SCENARIO-0085 | Escalation: autonomous climb through pre-approved rungs | integration | iteration | `cd modules/incus-dispatcher && go test . -run TestRunOnce_AutonomousRungDoesNotEscalate` | STORY-0061 |
| SCENARIO-0086 | Escalation: privileged escalation lands in escalations lane | integration | iteration | TBD | STORY-0061 |
| SCENARIO-0087 | Escalation: stale escalation resurfaced by rising urgency | integration | iteration | TBD | STORY-0061 |
| SCENARIO-0088 | Mac-off: human-only escalations queue durably for Mac return | e2e | iteration | TBD | STORY-0074 |
| SCENARIO-0089 | Isolation tier declared by template selects the backend (D1) | integration | iteration | `cd modules/incus-dispatcher && go test . -run TestScenario0089` | STORY-0023 |
| SCENARIO-0090 | Worker NixOS config is a single declarative source (patterns captured) | integration | iteration | `bash fleet-worker/tests/single-source.test.sh` | STORY-0017 |
