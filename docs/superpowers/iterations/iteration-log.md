# Iteration Log

## ITER-0000 — Dogfood milestone (IN PROGRESS)

**Started:** 2026-06-18

### Task 0 — stub Queue contract (DONE)
- `modules/incus-dispatcher/queue/`: `Directive` (full field set + `NotBefore`),
  `Queue` interface (atomic Claim + Lease/Touch + Done + Requeue + Reap), and
  `MemoryQueue` stub with importance→priority projection + not-before eligibility.
- Models laneq's contract so the ITER-0006 substrate swap is drop-in (PAR boxing-in fix).
- Evidence: `go test ./queue/` → 7 passing; `go vet` clean.
- Stories advanced: STORY-0057 (claim/lease substrate), STORY-0044 (not-before, stub form).

### Tasks 1–5 — daemon path (DONE, commit 5817399)
- Template/origin validation (STORY-0050): `policy.go` — allowlist + D1 authority split
  (worker proposing a privileged template is DENIED; fail-closed on unknown origin). 6 tests.
- Daemon claim-loop + Directive→Task mapping + minimal outcome (STORY-0057/0058): `daemon.go`
  — pass→done / fail→requeue / park-after-max / reject-invalid-template; external grade is
  authoritative (grade-fail ⇒ fail even if cmd exited 0). 7 tests w/ fake Runner.
- Teardown stop-then-delete (STORY-0062/0063): both runners now stop (bounded) BEFORE delete —
  fixes the verified `incus delete -f` hang.
- Go-exec PATH fix (STORY-0067): `workerToolPath` prepends worker nix-profile + ~/.local/bin
  so agent tools resolve (fixes exit 127). 2 tests.
- Total: 29 tests green, `go build`/`go vet` clean.

### Minimal worker image — flavor 1 (STORY-0075 slice) — VALIDATED on cluster 2026-06-18
- Stock `images:nixos/25.11` (unprivileged) + `nix develop ./fleet-worker --accept-flake-config
  --no-sandbox` → claude 2.1.181, lean-ctx 3.8.8, go 1.26.4, git, jq all resolve (exit 0).
  claude-code/lean-ctx SUBSTITUTE from cache.numtide.com — no build, no baked image, no
  nix-server republish. The two flags are required (cache trust + unprivileged-sandbox).
- `fleet-worker/flake.nix` gained the `nixConfig` substituter (commit a1ab0e0);
  `runner.sh` header documents the two flags.
- Teardown fix (STORY-0062/0063) VALIDATED on a real container: stop-then-delete took 2s,
  no `incus delete -f` hang.

### Remaining ITER-0000 tasks
- Wire DefaultMapToTask / the runner invocation to `nix develop … --accept-flake-config
  --no-sandbox --command bash runner.sh` (template→runner mapping; thin for ITER-0000).
- E2E journey harness (Task 0 harness half) + grader fixture.
- Parallel spikes (off critical path): ctx_handoff (STORY-0034), latency (STORY-0025, partly done).
- **Exit (b): real dogfood run — a claude -p task on a worker → oracle-graded diff (needs a
  target task/brief + API token in the container).**
