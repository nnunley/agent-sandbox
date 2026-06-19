---
name: fleet-dogfood
description: Dispatch one task (brief + repo + hidden oracle) to an ephemeral cluster worker, harvest the diff, and authoritatively grade it on a clean checkout. Use when dogfooding the fleet — having the fleet build/verify a change end-to-end with an oracle-graded result.
---

# fleet-dogfood

Single-task dispatch primitive. One call → one ephemeral worker → one oracle-graded diff.
Captures the proven ITER-0000 dogfood loop (a real `claude` worker implements a change,
graded on a clean checkout it never saw) as a one-command operation.

## When to use
- Dogfooding the fleet: have a real worker implement a change and prove it with a hidden oracle.
- Per-task dispatch inside a larger build (the caller sequences + applies graded diffs).

## One-time prep
```
bash .claude/skills/fleet-dogfood/fleet-dogfood-prep.sh
```
Configures a base worker (`fleet-dogfood-base`) from `fleet-worker/worker-container.nix`,
measures local-cache `nix develop` readiness, and selects **fresh-launch** (default) or a
**golden snapshot** (written to `.mode`). Re-run after the `fleet-worker` flake changes.
Prereq: the local nix cache must be populated — `scripts/populate-nix-cache.sh`.

## Dispatch
```
FLEET_TOKEN="$(cat ~/.fleet-token)" bash .claude/skills/fleet-dogfood/fleet-dogfood.sh \
  --name ID --brief FILE --repo PATH --oracle PATH \
  [--ref REF --model M --output-dir DIR --timeout SECS --golden SNAP]
```
Outputs in `--output-dir` (default `./dogfood-out/ID`): `worker.diff`, `events.jsonl`,
`grade.json`. **Exit 0 iff the oracle passed.**

The `--oracle` is a script run on a CLEAN checkout with the worker's diff applied. For a
hidden go-test oracle, have the script WRITE the test file then `go test` (so the worker
never sees it — anti-reward-hack). See `test/meta-dogfood.sh`.

## Gotchas (each cost a debugging cycle — proven the hard way)
- Worker is NON-root (claude refuses root); the run is uid 1000. chown the repo to the
  worker AFTER cloning (clone runs as root; else claude's `Write` fails on a root-owned tree).
- `nix develop` needs BOTH `--accept-flake-config` AND `--no-sandbox` (unprivileged LXC).
- Toolchain resolves from `file:///srv/nix-shared` (local cache) — populate it first. See
  the `nixos-incus-worker` skill for the worker traps.
- **Golden snapshot must be taken on a STOPPED container** — a running-container snapshot
  clones into an instance that won't boot systemd as PID 1 (nix-daemon never starts).
- `runner.sh` harvests with `git add -A -N` + `git diff --no-ext-diff` so NEW files are
  captured and the patch is a clean unified diff.
- Teardown is stop-then-delete and ALWAYS runs (never `delete --force` a running container).
- The grade runs on a CLEAN checkout the worker never saw — authoritative.
- `FLEET_TOKEN` injects the worker's `CLAUDE_CODE_OAUTH_TOKEN`; never bake it into a snapshot.

## Tests
- `test/test-args.sh`, `test/test-grade.sh` — pure-local (arg validation, grading).
- `test/test-prep.sh` — cluster: prep + measurement gate.
- `test/test-dispatch-smoke.sh` — cluster: real dispatch, trivial brief + grep oracle.
- `test/meta-dogfood.sh` — cluster: real `Queue.Pending()` task + hidden go-test oracle.
