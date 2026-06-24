# Progress

**Phase:** ITER-0007b — **Task 0 (deploy) COMPLETE**; **C1 (worker+Reprioritize) COMPLETE**; ready for C2 (PriorityWorkflow).
**Iterations:** 10/11 done (ITER-0000..0007). **Current: ITER-0007b** (LIVE Temporal time plane). ITER-0008 pending.

**Sentinel baseline (held green throughout):** `go vet` clean; `go test -race ./...` **387 green** (4 new Reprioritize tests); tree clean; citations 78/78.

**Scope review: 2-round PAR → APPROVE, committed `03bb3c3`.** (Round-1 both REVISE → 7 findings applied;
round-2 A=APPROVE, B=REVISE cleared by committing the deploy doc + recording the nixpkgs availability artifact.)

**Task 0 (BLOCKING) — DONE, committed `e53c71a`:** Temporal time plane LIVE on `agent-host`.
- `fleet-worker/temporal-service.nix`: hand-rolled systemd unit running `temporal-cli` 1.5.1
  `server start-dev --db-filename /srv/temporal/temporal.db --ip 0.0.0.0 --port 7233 --headless`.
  Chosen over stock `services.temporal` module because start-dev auto-bootstraps file-SQLite schema (stock
  needs temporal-sql-tool; upstream test only covers in-memory). Verified empirically (spike + deployed).
- `temporal-data` Incus host volume at `/srv/temporal` → durable across restart.
- Deployed via live switch (no container restart). Unit active, gRPC ready :7233 (boot-to-ready 22s).
- **Restart-survival proven:** namespace survived `systemctl restart temporal` (State=Registered).
- Deploy doc realized: `docs/plans/2026-06-23-iter0007b-temporal-deploy.md`.

**C1 (worker skeleton + Reprioritize wrapper) — DONE, committed `e7ca48e`:**
- `temporal/worker.go`: Temporal worker struct + NewWorker, Register, Start/Stop lifecycle.
  PriorityWorkflow stub registered (implementation in C2).
- `queue/laneq.go`: Reprioritize(id, importance) method wrapping laneq gRPC RPC.
  Called by Temporal workflows to update directive priority as urgency changes.
- `queue/laneq_test.go`: 4 Reprioritize tests (success, not found, rpc error).
- `temporal/worker_test.go`: Stub tests for NewWorker and PriorityWorkflow (integration tests deferred to C2).
- SDK v1.45.0 added to go.mod; all dependencies resolved via `go mod tidy`.

**Code phase (remaining):** C2 PriorityWorkflow + ReprojectActivity (sole-writer, CI via testsuite time-skip) → SCENARIO-0056/0093;
C3 rescore signal → SCENARIO-0094; C4 durable retry/escalation re-push → STORY-0058 AC-24/0061 AC-3/0055 AC-7;
C5 concurrent reads → SCENARIO-0081; E1 LIVE cluster harness (compressed-wall-clock aging + container-restart e2e)
→ fills SCENARIO-0001/0056/0081/0093/0094 execution commands. Wrap: corpus + story markers; rm temporal/scenario0078_test.go.bak.

**ITER-0008 GATE (recorded):** STORY-0041 AC-1/AC-2 + STORY-0044 AC-3 must pass (no carries).

**Last event:** 2026-06-24 — C1 complete (e7ca48e); worker skeleton + Reprioritize wrapper ready for C2.
**On resume:** start code phase C2 (PriorityWorkflow + ReprojectActivity) against live Temporal at agent-host:7233.
