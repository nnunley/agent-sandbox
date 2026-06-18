# Fleet Orchestration — queue-driven, Mac-off-safe agent dispatch (design)

Status: **design, one open decision (substrate)**. Date: 2026-06-18.

Builds on `docs/plans/2026-06-17-dispatcher-productization.md` (#25/#26/#27) and
`docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`. Extends the
existing `modules/incus-dispatcher` (one-shot CLI) into a queue-driven fleet.

This doc was produced by a brainstorming pass followed by an adversarial
pushback review. Each major decision below carries the issue that forced it.

---

## Governing constraint

> **The agent cluster must EXECUTE and COORDINATE with the user's Mac powered
> off. The Mac is a thin, optional client — never a single point of failure.**

Acceptance test for every substrate/topology choice: *close the Mac — can the
fleet still claim, run, grade, and hand off work?*

Consequences (not negotiable):
- Coordination plane (the queue) lives on the cluster, not the laptop.
- The provisioner/coordinator daemon runs on the cluster (`ndn-desktop`).
- The state-passthrough store (lean-ctx handoff) lives on cluster storage.

---

## Architecture — three planes

**Coordination plane**
- **Directive queue** (substrate OPEN — see below). Source of truth for *what
  work exists* and *who holds it*: priority, lanes, threading, leasing.
  Durable, cluster-resident.

**Control plane (our new code — extends `modules/incus-dispatcher`)**
- **`dispatcher serve --queue`** — long-running daemon on `ndn-desktop`. Drains
  the queue, resolves each directive to a launch, and runs a **deterministic,
  zero-LLM coordination loop** over results (see Coordination loop). Reuses the
  existing `Runner` launch/harvest/grade code paths. Writes a decision-log line
  per coordination action.
- **Execution backend interface** — `launch(template) → handle`. Two
  implementations: container-backed (ship first, proven) and micro-VM-backed
  (benchmark-gated track).

**Execution plane**
- **Worker instances** — NixOS golden (`fleet-worker/flake.nix`), immutable
  root with writable `/workspace` + `/tmp`. One-shot per directive in v1.
  Launched by `incus copy` from a golden with a fresh name (never built live).

The Mac is a stateless client: it authors top-level directives and reviews
parked threads. It holds no fleet state.

---

## Decisions (with the pushback issue that forced each)

### D1 — Intent + proposed template, never commands (Issue 1, security/critical)
Directives carry **intent and a *proposed* client-template name**, never a
literal `access_cmd` or a `--root` flag. The daemon **resolves** intent → an
immutable, pre-vetted template and **disposes** on the proposal: a template is
launched only if (a) it is in the allowlist and (b) the directive's **origin**
is permitted that template. Worker-authored child directives may carry task
content only; their provisioning is inherited/fixed and never privileged.

This closes the escalation path created by the trusted, unauthenticated bridge:
a compromised or drifting worker cannot push a `root: true` + arbitrary-command
directive and get a privileged container on the host.

Sub-constraints:
- Immutable root **with writable scratch** (`/workspace`, `/tmp` tmpfs/overlay)
  — the worker still checks out a repo, writes `worker.diff`, runs builds.
- Adding a capability = building a new immutable template, not `--root install`.
  Slower iteration; pairs with the NixOS golden + cached closure.

### D2 — Substrate-agnostic backend; container first, micro-VM benchmark-gated (Issue 2, feasibility)
The only proven end-to-end path is an incus **container** (the 13→0 dogfood,
even that on an Ubuntu stopgap). Micro-VMs are the north star (lower latency in
theory, and they eliminate the teardown hang — see D5) but unproven for this
workload. So:
- Define the backend as an interface; ship **container-backed one-shot first**
  on the proven path.
- Add a **micro-VM backend as a parallel track**, gated by a benchmark that
  measures (a) startup latency *with the closure realized* and (b) clean
  kill-to-teardown vs the container delete-hang.
- The queue/intent/template design is backend-agnostic.

### D3 — Durable agents deferred; lean-ctx carries state passthrough (Issue 3)
Durable agents fit neither the one-shot `Runner` interface nor the
immutable/ephemeral/fast-start direction (if startup is cheap, the
re-provisioning argument for durable evaporates). v1 = **one-shot + fast restart
+ requeue-based steering**.

State passthrough between one-shots uses **lean-ctx memory primitives**:
- `ctx_agent action=diary` (decisions/blockers/progress) — write; `recall_diary`
  / `diaries` — read.
- `ctx_agent action=share_knowledge` / `receive_knowledge` — facts.
- `ctx_handoff action=create|export|import|pull` — a deterministic bundle
  (workflow state + session snapshot + curated knowledge) on the shared volume,
  `import`ed by the successor one-shot.

**Authoritative state never lives in lean-ctx.** The code (`worker.diff`) and the
grade (oracle JSON) are authoritative and stay in our own artifacts. lean-ctx
carries only soft, lossy-OK state; if its handoff drops something, the diff +
external grade still make the run correct (anti-reward-hack,
`verify-from-system-of-record`).

**lean-ctx's message bus is NOT the queue.** Its `ctx_agent post/read` bus is
soft agent-to-agent chatter (broadcast/direct, read-once, no lease, no atomic
claim, no requeue). The directive queue keeps the durable, leased, crash-safe
work ledger. Don't replace it with the bus; don't bridge two buses.

### D4 — Deterministic, cluster-resident coordination loop (Issue 4)
"Coordinate with the Mac off" requires the decision loop on the cluster, not the
laptop — but not an unsupervised LLM. The daemon applies **fixed rules** on each
grade:
- pass → mark the thread `done`;
- fail → requeue with the grade attached, up to **N** attempts;
- persistent fail → **park** for human review.

No decomposition, no model in the loop (bernstein's zero-LLM-scheduler idea).
The human, via the Mac when on, authors top-level directives and clears parked
threads.

### D5 — Orderly teardown + reaper; root cause was a missing stop (Issue 5)
Current code never stops before deleting: the CLI runner force-deletes a running
container (`incus delete -f` — the documented hang trigger), and the Go-client
runner calls `DeleteInstance` and swallows the error (leaks running instances).
Fix:
- **Stop first, bounded timeout** (`incus stop --timeout` / client
  `UpdateInstanceState`), *then* delete — out of the hot path.
- Stop-timeout instances go to an **out-of-band reaper**; the coordination loop
  never blocks on teardown.
- Launch via `incus copy` from golden + fresh names, so a leaked instance never
  collides with the next run.
- Micro-VM backend (D2) sidesteps this entirely (kill the VM process).

### D6 — Append-only decision log, swappable to tamper-evident later (Issue 6)
v1 audit = **plain append-only JSONL** on cluster storage, one line per
coordination decision (directive id, grade summary, rule fired, action, ts) — no
crypto. **Behind a writer interface** so it can be swapped to an HMAC-chained
tamper-evident log without rearchitecting. Driver for the eventual upgrade: a
separate trading-platform project with compliance-grade audit needs.

---

## Directive contract (intent-shaped, per D1)

A directive body (JSON the daemon interprets; the queue stays generic). Shaped
by `low-level-executor-task-spec` — nothing the executor cannot infer is left
implicit.

```json
{
  "intent": "fix cluster-A conj-expected-Collection failures to 0",
  "template": "fleet-golden-go",        // PROPOSED; daemon validates vs allowlist + origin
  "origin": "orchestrator",             // orchestrator | worker:<id> (set by the daemon, not the author)
  "priority": "P1",
  "lane": "let-go",
  "repo": "git-bundle:///srv/feed/let-go.bundle",
  "ref": "main",
  "task": "…literal brief / pointer to brief file in the template…",
  "handoff_in": "handoffs/<ts>-<md5>.json",   // optional: lean-ctx bundle to import
  "grade": {                            // optional; presence ⇒ authoritative external grade
    "oracle_ref": "main",
    "cmd": "make check-generated && go test -tags gogen_ir ./pkg/ir/",
    "expect": {"clusterA": 0}
  },
  "max_attempts": 3
}
```

No `access_cmd`, no `root`. The template defines how the work runs.

---

## One-shot lifecycle (v1)

1. Daemon claims the next directive (priority, lease).
2. Validate the proposed `template` against allowlist + `origin` (D1).
3. `incus copy golden → fresh-name` (D5); attach shared volumes (nix cache,
   lean-ctx handoff store).
4. Deliver repo (bundle/clone); if `handoff_in`, `ctx_handoff import`.
5. Run the template's runner (lean-ctx `setup` + `serve`, then `claude -p`).
6. Harvest `worker.diff` + `result.json`; agent writes diary/knowledge.
7. If `grade` present, run the **authoritative external grade** on a clean
   checkout the worker never touched (`verify-from-system-of-record`,
   `context-anchored-patching` — copy source files, don't `patch`).
8. Coordination loop (D4): pass→done; fail→requeue (≤ `max_attempts`) with grade
   + a fresh `handoff` bundle; persistent fail→park.
9. Stop → (reaper) delete (D5). Write decision-log line (D6).

Live steering = the orchestrator pushes a higher-priority directive into the
lane; the next fast-start one-shot picks it up with the prior handoff applied.

---

## Skills (vendored, per the port decision)

Vendor the relevant subset of `selamy-labs/agent-skills` into the worker golden
(`fleet-worker/skills/`), baked into the image (offline, pinned):

`using-laneq`, `low-level-executor-task-spec`, `process-aware-done`,
`verify-from-system-of-record`, `verify-real-artifact`, `gate-before-push`,
`graceful-shutdown-stateful-agents`, `restart-resilience`, `yield-on-wait`,
`push-over-polling`, `credential-proxy`, `context-anchored-patching`,
`agent-otel-trajectory`.

---

## OPEN DECISION — queue substrate

Must satisfy the governing constraint (Mac off → fleet still coordinates).
Candidates:

1. **laneq-as-cluster-service** — run laneq + its MCP server on `ndn-desktop`
   (DB on a host volume); Mac and workers are remote MCP clients. Keeps the
   vendored skills, threading, leasing, ecosystem. Against laneq's "no network"
   grain; single-cluster SPOF; WAL handles a handful of fleet clients.
2. **Network-native durable backend** — Postgres or NATS JetStream. True
   multi-client/HA, survives host recycle. Loses laneq's local-SQLite simplicity;
   reimplement its CLI/MCP surface (keep the skills as patterns).
3. **Dedicated queue host/container** — queue on its own persistent container,
   separate from the worker host, so it survives worker-capacity rebuilds.
   Cleaner failure isolation; one more thing to run.

Decision pending.

---

## Testing

- **Daemon** — against a temp queue DB: claim/lease/requeue/park rule coverage;
  template allowlist + origin validation (a `worker`-origin privileged-template
  proposal must be denied).
- **Teardown** — stop-then-delete with a bounded timeout; a stop-timeout routes
  to the reaper and never blocks the loop (regression for the delete-hang).
- **Backend interface** — container backend against the existing
  `container_runner_test.go`; micro-VM backend behind the benchmark gate.
- **State passthrough** — `ctx_handoff` round-trips decisions across two
  separate `claude -p` invocations on the worker (this is unproven today — the
  dogfood ran lean-ctx compression only, bridge OFF; treat as a gating spike).
- **Mac-off** — the headline acceptance test: with the Mac disconnected, the
  cluster claims, runs, grades, requeues/parks, and a successor resumes via
  handoff.

---

## Pointers

- Productization plan: `docs/plans/2026-06-17-dispatcher-productization.md`
- Coordinator bootstrap: `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`
- Existing dispatcher: `modules/incus-dispatcher/` (`Runner`, `cleanup()`)
- Worker golden: `fleet-worker/{flake.nix,runner.sh,brief.level1-focused.txt}`
- Reference designs: laneq (`selamy-labs/laneq`), bernstein
  (`sipyourdrink-ltd/bernstein`), lean-ctx (`yvgude/lean-ctx`)
