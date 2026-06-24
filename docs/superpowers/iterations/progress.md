# Progress

**Phase:** ITER-0007b — pre-iteration scope review (PAR). Sentinel baseline CLEAN. Round-1 PAR returned
REVISE (both reviewers); revisions applied to roadmap + scenarios + deploy-doc stub; round-2 PAR re-review
in flight to confirm APPROVE.
**Iterations:** 10/11 done (ITER-0000..0007). **Current: ITER-0007b** (LIVE Temporal time plane on
ndn-desktop, Nix+systemd). ITER-0008 pending (capstone).

**Sentinel corpus baseline (pre-ITER-0007b):** `go vet` clean; `go test -race ./...` **383 green**;
JOURNEY-0001 (2) + JOURNEY-0003 AC-1 (19) green; citations 78/78. Tree clean.

**ITER-0007b scope (committed live/e2e ACs):** STORY-0001 AC-1/AC-2, STORY-0041 AC-1/AC-2, STORY-0044 AC-3,
STORY-0043 AC-2, STORY-0046 AC-2, STORY-0047 AC-1, STORY-0058 AC-24, STORY-0061 AC-3, STORY-0055 AC-7,
STORY-0002 AC-2. Task 0 (BLOCKING) deploys Temporal.

**Key verified deploy facts:** upstream `services.temporal` NixOS module + `temporal` 1.29.4 in nixos-25.11
(flake pins nixos-25.11). Full temporal-server, gRPC :7233. Durability via file-backed SQLite on Incus host
volume (upstream test uses in-memory — NOT durable, must override). Build on cluster (no nix on macOS).
laneq-grpc.service confirmed ACTIVE on agent-host.

**Round-1 PAR findings (both REVISE) → all addressed in artifacts:**
1. Deploy doc → formal T0.3 deliverable + STUB committed (docs/plans/2026-06-23-iter0007b-temporal-deploy.md)
2. Aging proof decision RESOLVED: compressed real-wall-clock (SCENARIO-0056); restart-survival harness (SCENARIO-0001)
3. NEW scenarios: SCENARIO-0093 (STORY-0044 AC-3 sole-caller), SCENARIO-0094 (STORY-0047 AC-1 live rescore)
4. Sole-writer enforcement = process-level disciplined-client (non-exclusive laneq leases) — stated in roadmap + deploy doc
5. ITER-0008 GATE: STORY-0041 AC-1/AC-2 + STORY-0044 AC-3 no carries
6. Task 0 decomposed T0.1/T0.2/T0.3 + story sequencing (deploy → gate → live ACs)
7. EPIC artifact-debt note added (non-blocking)

**Implementation notes (for post-approval decompose):** temporal/ is pure-Go (no SDK dep yet); adding
go.temporal.io/sdk likely as a separate Go module to keep dispatcher lean (vendorHash rebuild). Cleanup debt:
temporal/scenario0078_test.go.bak leftover.

**Last event:** 2026-06-23 — ITER-0007b round-2 PAR scope re-review dispatched; awaiting APPROVE verdict.
