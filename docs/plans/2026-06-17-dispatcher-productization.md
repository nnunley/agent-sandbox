# Dispatcher Productization — grounded in the first real dogfood (2026-06-17)

Status: **active plan**. Covers tasks #25 (worker reliability + tooling), #26 (comms /
working-state), #27 (NixOS golden via `llm-agents.nix`). Builds on
`docs/2026-06-17-dispatcher-enhancements.md` (the #22 design: NixOS image, provider
routing, external grading) and `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`.

## Why this exists

On 2026-06-17 we ran the **first real end-to-end dogfood**: a general-purpose Claude
agent ran *headless on the incus cluster* (`ndn-desktop`, container `lvl1-ub`), off the
user's Mac, and produced an **oracle-verified compiler fix** for the let-go `gogen-rebase`
line — cluster-A `conj expected Collection` failures **13 → 0**, `make check-generated`
green, untagged + e2e green. It even **out-diagnosed the human audit**, tracing the
Go-`nil`-vs-`vm.NIL` leak to its origin in `pkg/vm/native_func.go` `theNativeFnType.Box`.

So the bridge works. This plan is the list of rough edges the real run exposed — the
difference between "it worked once with hand-holding" and "it's a tool."

## Evidence base (what the two runs actually showed)

- **Run 1 (first attempt):** wandered ~3.7K events, no convergence; context blew up and
  compacted. Root issue: unfocused brief + no context discipline.
- **Sharpen step:** harvested the partial diff, rewrote the brief into ordered gated
  phases with an anti-wander rule, and installed **lean-ctx** on the worker.
- **Run 2 (focused):** converged in ~48 min / ~2357 events. cluster-A → 0, all gates
  green. lean-ctx engaged — the worker routed **all** shell through `ctx_shell` (68 calls)
  and reads through `ctx_read` (27), never the raw Bash tool.
- **Under-reporting failure:** the worker ended `rc=1` with **no `result.json`** — it ran
  out of turns during the final untagged `go test ./...` *after* the work was already
  correct. The orchestrator only learned it had succeeded by running the oracle itself.
  → the result contract must be robust to truncation (see #26).

Artifacts produced this session (the canonical shapes to productize):
`fleet-worker/flake.nix`, `fleet-worker/runner.sh`, `fleet-worker/brief.level1-focused.txt`.

---

## #25 — Worker reliability + tooling

### 25.1 Go-exec `127` fix (dispatcher exec path)
**Symptom (earlier):** dispatched commands returned exit `127` (command not found) when
run via the incus Go client because the exec env didn't have the nix profile / `$PATH`
the interactive `su - worker` login has.
**Fix:** the dispatcher's `client_runner.go` exec must run the command through a login
shell that sources the worker's profile (`bash -lc` as the `worker` user), OR explicitly
prepend `$HOME/.local/bin` and the nix profile bin dirs to `PATH` in the exec `Environment`
map. Mirror exactly what `su - worker -c` resolves (that path worked all session).
**Acceptance:** `incus-dispatcher --cmd 'claude --version && go version && lean-ctx --version'`
returns 0 with all three versions, via the Go client (not `incus exec` shelling).

### 25.2 Grading round-trip proof
**Goal:** prove the external-grading guardrail (`docs/2026-06-17-dispatcher-enhancements.md`
section C) end-to-end: worker diff is applied to a **pristine checkout the worker never
touched**, regenerated, and graded by the oracle there.
**This session did this manually** and it is the pattern to encode:
1. Worker produces `worker.diff` (source files only; never the generated `.lgb`/sums).
2. Grader: clean checkout of the target ref → apply **source** files wholesale (copy, not
   `patch` — `.lg` context fragility caused 3 rejected hunks this session) → `make generate`
   → run oracle (`go test -tags gogen_ir ./pkg/ir/` cluster count, `make check-generated`,
   untagged, e2e).
3. Emit a structured grade `{passed, clusterA, check_generated, untagged_fails, e2e}`.
**Acceptance:** a dispatcher subcommand (or `scripts/grade.sh`) that takes a target ref +
a worker diff and emits the grade JSON, reproducing today's 13→0 result from the harvested
`/tmp/lvl1-focused.diff`.

### 25.3 lean-ctx FULL enablement (not just `init`)
**Finding:** `lean-ctx init --agent claude` registers the MCP server (`~/.claude.json`) +
hooks, and `claude -p` spawns the MCP server itself — so **`ctx_read`/`ctx_shell` work**.
BUT `lean-ctx gain` reported **"Bridge: OFF — proxy not reachable; savings cannot be
measured (76 tools registered)"** and `lean-ctx status` showed `last setup: (none)`. So the
**shell-hook compression + savings measurement need the bridge daemon**, which `init` does
not start.
**Fix in the runner:** before launching `claude -p`, run `lean-ctx setup` (fuller config
than `init`) and start the bridge: `lean-ctx serve &` (verify with `lean-ctx status` →
connected, and `lean-ctx gain` no longer says Bridge OFF). Capture `lean-ctx gain` at the
end of the run into the result for a measured savings number.
**Acceptance:** post-run `lean-ctx gain` shows a non-zero measured savings (bridge ON).

### 25.4 Linux `rtk` (optional)
`rtk` is the user's macOS token-killer; the brief references it as optional. Low priority —
lean-ctx subsumes most of it. Only add a Linux `rtk` to the golden if a concrete gap
remains after 25.3.

### 25.5 Canonical runner shape
Reconcile `fleet-worker/runner.sh` (resets the tree — right for a *fresh* task) with the
ad-hoc `run-focused.sh` used this session (no reset — right for *continuation*). The runner
should take a mode flag: `--fresh` (reset + clean) vs `--continue` (keep applied diff).
Both: `export PATH=$HOME/.local/bin:$PATH`, lean-ctx setup+serve, `stream-json`, harvest
`worker.diff` + `result.json`, write `lean-ctx gain`.

---

## #26 — Comms + working-state (the heartbeat is the product, not a debug aid)

### 26.1 Heartbeat must track `ctx_*`, not Bash
**Finding:** the monitoring helper projected the "last command" from `tool_use` where
`name=="Bash"` — but with lean-ctx the worker runs **everything through `ctx_shell`**, so the
heartbeat showed a misleading "(no shell yet)" for ~1500 events while the worker was very
much working. Fixed mid-session: project from `ctx_shell`/`ctx_read` (and Bash as fallback).
**Encode:** the working-state projector reads `events.jsonl` and emits
`{alive, eventCount, Δsince_last, last_shell_cmd (ctx_shell|Bash), last_read, phase_guess}`.
phase_guess from the brief's gate commands (`go build` → compile; `go test ...pkg/ir` →
oracle; `make check-generated`/`go test ./...` → regress).

### 26.2 Robust result contract (survive truncation)
**Finding:** worker `rc=1` + **no `result.json`** even though the work succeeded.
**Fix:** (a) the brief already mandates `result.json`; ALSO have the **runner** synthesize a
fallback result on exit — capture the last oracle command's output and write a best-effort
`result.json` (`status:UNKNOWN`, plus the harvested diff path) so the orchestrator always
has structured output. (b) The grader (#25.2) is the source of truth regardless — the
worker's self-report is advisory, the **external grade is authoritative** (anti-reward-hack).

### 26.3 Tier-2 bidirectional coordinator
Build on `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`. Tier-1 (one-way
heartbeat projection) is proven. Tier-2 = orchestrator can send the worker a steering
message mid-run (e.g. "stop, you've drifted; here's the precise pointer").
- **File feed now:** a watched file in the container the worker is told to poll between
  phases. Cheap, works today.
- **D-Bus** *if* the coordinator runs on the Linux host (was raised as a substrate; only
  viable host-side, not from the Mac orchestrator).
- **NATS** for multi-host / multi-worker fan-out later.
**Acceptance:** orchestrator writes a steer message; worker acknowledges it in `events.jsonl`
within one phase boundary.

---

## #27 — NixOS golden via `llm-agents.nix` (retire the Ubuntu stopgap)

### Current state
- `fleet-worker/flake.nix` already consumes `github:numtide/llm-agents.nix` and declares
  `agents.claude-code` + `agents.lean-ctx` + Go 1.26 + make/git/jq/rg/ast-grep. These are
  **binary-cached** (cache.numtide.com) — `claude-code` substitutes prebuilt, which should
  **sidestep the nix build-sandbox wall** that blocked building claude-code inside an
  unprivileged container.
- The proven dogfood ran on the **Ubuntu `lg-golden`** stopgap because of NixOS-on-incus
  frictions (below). That stopgap works but is not the target.

### Known NixOS-on-incus frictions (must be solved or documented around)
1. `claude-code` is unfree → `config.allowUnfree = true` (done in flake).
2. nix **build sandbox** fails in an unprivileged container (daemon setting, not overridable
   as non-root) → rely on **substitution from cache** (`--accept-flake-config`), never build.
3. `security.privileged=true` remapped uids and **broke a populated container**; a fresh
   privileged NixOS container never activated userspace (`bash`/`useradd` not found). →
   prefer **unprivileged + cached substitution**, not privileged builds.
4. `incus delete --force` **HANGS** on this host → never delete; always use **fresh names**
   and `incus copy` from a golden.

### Plan
1. Build a **NixOS golden** once: a container where `nix develop ./fleet-worker
   --accept-flake-config` has fully realized the closure (claude-code, lean-ctx, go, make).
   Snapshot it as the golden image; `incus copy golden <task-name>` per task (playbook shape).
2. Runner inside the golden: `nix develop --command bash runner.sh` → which does lean-ctx
   `setup` + `serve` (#25.3) → `claude -p` (#25.5).
3. Verify the **clean-room integrity gate** still holds on NixOS: byte-identical regen of
   `core_compiled.lgb` / `core_go_lowered/` / `generated.sums` (the deny-hook on generated
   artifacts from the playbook H1/H2).
**Acceptance:** a NixOS golden runs the focused Level-style brief headless with lean-ctx
**bridge ON**, produces a graded diff, with no Ubuntu fallback.

### Provider routing (ties to #22)
The golden + `llm-agents.nix` also gives `codex`/`gemini-cli`/`qwen-code` for cheap workers.
Keep the "cheap implementer + strong external grader" split: implementer can be Sonnet (or
cheaper via the proxy → Haiku/OpenAI/Ollama at `ndn.local:11434`), the **grader/oracle is
deterministic** (no model), and a strong model only reviews the final graded diff.

---

## Ordered next actions

1. **#25.1** Go-exec `127` fix (`bash -lc` / PATH in `client_runner.go`) — unblocks
   client-driven runs. Acceptance test as above.
2. **#25.2 + #26.2** Grading subcommand + authoritative external grade; reproduce 13→0 from
   the harvested diff. (Makes self-report truncation harmless.)
3. **#25.3** lean-ctx setup+serve in the runner; prove bridge ON via `gain`.
4. **#26.1** Working-state projector tracking `ctx_*` (port the fixed `lastcmd.py` logic
   into the dispatcher / a `scripts/working-state.py`).
5. **#27** NixOS golden via the flake (cached substitution); migrate off Ubuntu `lg-golden`.
6. **#26.3** Tier-2 file-feed steering; D-Bus/NATS later.

## Pointers
- Design (done): `docs/2026-06-17-dispatcher-enhancements.md`
- Coordinator: `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`
- Worker shapes: `fleet-worker/{flake.nix,runner.sh,brief.level1-focused.txt}`
- Playbook: `~/pkm/wiki/concepts/headless-claude-fleet-incus.md` +
  `self-verifying-agent-tasks.md`
- Proven artifact: harvested `/tmp/lvl1-focused.diff` (cluster-A 13→0), and let-go memory
  `cluster-A-root-cause-and-fix` (importance 9).
