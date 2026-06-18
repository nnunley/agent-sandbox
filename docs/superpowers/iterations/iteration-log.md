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

### Remaining ITER-0000 tasks (pending)
- Template validation (STORY-0050): allowlist + origin (security-critical, pure-Go TDD).
- Daemon claim-loop + Directive→Task mapping → existing Runner (STORY-0057/0051/0052/0019).
- Coordination outcome minimal: pass→done / fail→requeue (STORY-0058 scoped).
- Teardown stop-then-delete + reaper (STORY-0062/0063) — fixes the verified delete-hang.
- Go-exec PATH fix (STORY-0067) — fixes `127`.
- Minimal container worker image (STORY-0075 slice).
- E2E journey harness (Task 0 harness half) + grader fixture + teardown-regression assertion.
- Parallel spikes (off critical path): ctx_handoff (STORY-0034), latency (STORY-0025, partly done).
- Exit (b): real dogfood run on a cluster container.
