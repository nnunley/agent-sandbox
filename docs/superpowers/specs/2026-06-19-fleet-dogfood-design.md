# fleet-dogfood — single-task fleet dispatch primitive (design)

**Date:** 2026-06-19
**Status:** approved to start (design); spec under review before implementation plan

## Purpose

A repeatable, single-task **dispatch primitive** for dogfooding our own fleet
infrastructure. One invocation dispatches one directive (a brief + repo + hidden
oracle) to an ephemeral worker on the cluster, harvests the worker's diff,
**authoritatively grades** it on a clean checkout, and reports pass/fail. The caller
(e.g. a hybrid ITER-0001 build, or a future iteration's implementer) sequences
multiple dispatches and decides when to apply/commit a graded diff.

This captures the manual choreography proven in ITER-0000 exit (b) — where headless
`claude` on a NixOS worker implemented `queue.Peek()` and an authoritative clean-room
grade passed 10/10 incl. 3 hidden oracle tests — as a one-command operation, so every
future iteration is dogfood-able by default.

## Scope decisions (settled in brainstorming)

- **Single-task primitive**, not a multi-task sequencer. Smallest composable unit;
  the caller owns sequencing + apply policy.
- **Standalone script of the proven loop**, not a wrapper over the `dispatcher`
  binary's generic `--cmd` flow (the binary can't yet drive the fleet-worker
  `runner.sh` choreography — that Go wiring is deferred to ITER-0003). The script
  reuses the binary's `--external-grading` for the grade step only.
- **Ephemeral worker per dispatch** (clean isolation; the "disposable unit" intent).
- **Golden snapshot is CONDITIONAL, decided by measurement** (see below). With the
  local nix cache now populated, a fresh stock worker may resolve the toolchain fast
  enough that a golden clone is unnecessary complexity.

## Dependency already satisfied: local nix cache

The toolchain (claude-code, lean-ctx, go) is published to the shared local cache
`file:///srv/nix-shared` and verified to resolve fully offline (see
`scripts/populate-nix-cache.sh`, commit 5fc3aa6). `worker-container.nix` lists the
local cache first. So a worker's `nix develop` pulls the toolchain locally in
seconds — this is what makes the golden snapshot optional.

## Components

### 1. `fleet-dogfood-prep` (occasional, idempotent)

Ensures a ready-to-clone-or-launch worker baseline exists:
- Launch a stock `images:nixos/25.11` container, apply `worker-container.nix`
  (worker user, `nix-shared` RO mount, local-first substituters, `sandbox=false`).
- **Measurement gate:** time "fresh worker ready to run a task" (launch + apply +
  first `nix develop` resolving from the local cache). If that is acceptably fast
  (target: a small fraction of a typical task's wall-clock), STOP here — no golden.
- **Only if measurement shows fresh-launch is too slow:** snapshot the configured
  worker as `fleet-worker-golden/pristine` so per-dispatch uses a btrfs-reflink
  `incus copy` clone instead of a fresh launch.

The prep step records which mode it selected (fresh-launch vs golden-clone) so
`fleet-dogfood` knows how to spin up a worker.

### 2. `fleet-dogfood` (per dispatch)

1. **Spin up** an ephemeral worker — fresh launch (default) or `incus copy` clone of
   the golden (if prep selected that mode).
2. **Deliver** repo (at `--ref`) + brief + fleet-token into the worker.
3. **Run** `nix develop ./fleet-worker --accept-flake-config --no-sandbox --command
   bash runner.sh` — `claude -p <brief>` executes; toolchain from the local cache.
4. **Harvest** `worker.diff` + `events.jsonl`.
5. **Grade (authoritative)** — apply the worker's diff to a *clean* checkout the
   worker never saw + run the hidden `--oracle`, via the dispatcher's
   `--external-grading`. Anti-reward-hack: the grade, not the worker's self-report,
   is the verdict.
6. **Teardown** — stop-then-delete the ephemeral worker (always, even on failure;
   the ITER-0000 `incus delete -f` hang fix).

## Interface

| Flag | Meaning |
|---|---|
| `--name <id>` | dispatch identifier (worker name + output dir) |
| `--brief <file>` | task description → worker's `claude -p` prompt |
| `--repo <path>` | code the worker checks out |
| `--ref <ref>` | git ref (default `HEAD`) |
| `--oracle <path>` | hidden test(s) → authoritative grade on a clean checkout |
| `--model <m>` | worker model (default `claude-sonnet-4-6`) |
| `--golden <snap>` | golden snapshot to clone (only if prep selected golden mode) |
| `--output-dir <dir>` | where `worker.diff`, `events.jsonl`, `grade.json` land |
| `--timeout <dur>` | worker wall-clock (default from runner.sh) |

## Outputs

- `worker.diff` — the change the worker produced (worker is told NOT to commit).
- `events.jsonl` — the worker's `claude` stream-json event log.
- `grade.json` — authoritative result: `{ patch_applied, exit_code, pass }`.
- **Exit code 0 iff the oracle passed.**

## Data flow

```
caller → fleet-dogfood --brief --repo --ref --oracle --name --output-dir
  → spin up ephemeral worker (fresh launch | reflink clone of golden)
  → deliver repo@ref + brief + token
  → nix develop … runner.sh   (claude -p; toolchain from file:///srv/nix-shared)
  → harvest worker.diff + events.jsonl
  → external grade: clean checkout + apply diff + run hidden oracle
  → grade.json + exit code
  → stop-then-delete worker
```

## Error handling

| Condition | Behavior |
|---|---|
| Cluster unreachable / golden missing | clear error pointing to `fleet-dogfood-prep`; non-zero exit |
| Worker wall-clock timeout | harvest whatever exists; grade runs on partial diff (likely fails) |
| Patch fails to apply on clean checkout | `grade.json.patch_applied=false` → fail |
| Oracle fails | `pass=false` → non-zero exit |
| Any failure | teardown (stop-then-delete) still runs |

## Testing

1. **Smoke:** `prep` produces a usable worker; a trivial brief + trivial oracle
   grades green end-to-end.
2. **Meta-dogfood:** re-run the ITER-0000 `queue.Peek()` task (brief + the hidden
   Peek oracle) through `fleet-dogfood`; assert the oracle passes — proving the
   skill reproduces the proven loop.

## Location

Project-scoped: `.claude/skills/fleet-dogfood/` — `SKILL.md` (when-to-use, the
prep/measurement gate, the interface, the proven-loop gotchas: the two `nix develop`
flags, non-root worker, fleet-token, authoritative grading) + the dispatch script(s).
Version-controlled with the repo so it travels with the infra and is itself
dogfood-able.

## Open questions / to verify during implementation

- **Snapshot necessity (the measurement gate):** confirm empirically whether fresh
  stock-worker launch + local-cache `nix develop` is fast enough to skip the golden
  snapshot. Default to no-golden; add it only if the numbers demand it.
- **fleet-token delivery:** how the worker's `CLAUDE_CODE_OAUTH_TOKEN` (currently
  `$HOME/.fleet-token` in runner.sh) is injected per-dispatch without baking it into
  a snapshot. Prefer agenix/secret injection over a baked token.
- **Concurrency:** v1 is one dispatch at a time; concurrent dispatches (unique worker
  names + output dirs) are a later concern.
