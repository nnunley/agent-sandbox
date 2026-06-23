# Progress

**Phase:** ITER-0006b DONE + **AUDIT CLEAN** (PAR, 2026-06-23, both auditors CLEAN). laneq productionized
as a cluster-resident Nix service; Go adapter wire-proven over the network; Mac-off PASS-NARROW.
**Iterations:** 9/11 done (ITER-0000/0001/0002/0003/0004/0005/0005b/0005c/0006/0006b). **Next pending:
ITER-0007** (Time plane & Eisenhower prioritization — Temporal). ITER-0008 pending (capstone).
ITER-0006 Patrick-block CLEARED (substrate = laneq, deployed).

**Sentinel corpus (baseline 2026-06-23, pre-ITER-0007):** `go vet` clean; `go test -race ./...` **283
green** (incus-dispatcher + queue + llm-proxy); JOURNEY-0001 + JOURNEY-0003 AC-1 sentinels green;
citation check OK (78/78 cited stories exist). Tree clean.

**ITER-0006b delivered (commits, latest 11fd710):** T0 laneq Nix package (`fleet-worker/laneq.nix`,
in-build proto stub regen, fork checkPhase 72 grpc tests); T1 systemd `laneq-grpc` on
ndn-desktop:nix-server:9999 + host-volume SQLite `/srv/laneq` + Nix-wired `laneq-client`; T2
SCENARIO-0092 over the wire (5/5 deterministic); T3 SCENARIO-0012 Mac-off PASS-NARROW (systemd-run
detached drain, Mac uninvolved). Real-laneq divergences logged for ITER-0008 (reap return-count;
leases NOT consumer-exclusive).

**ITER-0007 SPLIT (PAR scope review 2026-06-23 — both reviewers REVISE, high agreement, applied):**
the 13-story Temporal iteration was split, mirroring 0005→0005/b/c and 0006→0006/b:
- **ITER-0007 (next, CI-provable Eisenhower LOGIC slice, pure Go + fake clock):** STORY-0040 (quadrants),
  STORY-0045 (projection determinism), STORY-0043 AC-1/AC-3 (urgency math + Q4-idle), STORY-0042/0047
  (rescore authority validation), STORY-0046 AC-1 (single-writer guard), STORY-0041 AC-3 (laneq.next,
  done ITER-0006), STORY-0001 AC-3 (single-writer design), + logic portions of split-ins (STORY-0064
  AC-15/16, 0058 AC-24, 0061 AC-3/0055 AC-7 reprojection, 0002 AC-2, 0044 AC-3 vs mock laneq).
- **ITER-0007b (cluster, LIVE Temporal):** deploy Temporal on ndn-desktop (Nix+systemd, Task 0 BLOCKING)
  + STORY-0001 AC-1/AC-2 (durable + restart-survival), STORY-0041 AC-1/AC-2 + 0044 AC-3 (live sole-writer
  over laneq gRPC), STORY-0043 AC-2 (wall-clock aging), STORY-0046 AC-2, STORY-0047 AC-1, + live split-ins.
- **Deferred → ITER-0008:** STORY-0035/0036/0037/0038/0039 (provider/budget/thread-aging/multi-repo — NOT
  time-plane; STORY-0035 Run fields MUST co-define with STORY-0011/0015 Run to avoid the colliding-Run
  lesson). Operator-experience half of SCENARIO-0087 → ITER-0008.
- **Boxing-in mitigations applied:** no `Run` struct in ITER-0007 (defer 0035); single-writer is
  process-level + documented orthogonal to laneq's non-exclusive leases (ITER-0006 finding).
- **Artifact debt (non-blocking):** EPIC-005 design-doc citations have stale line numbers (doc
  restructured) — re-anchor in a docs pass during decomposition.

**Last event:** 2026-06-23 — ITER-0006b confirmed done (audit CLEAN). Bookkeeping reconciled. Ran the
ITER-0007 pre-iteration scope PAR (2 reviewers → both REVISE); applied the split to roadmap.md
(ITER-0007 CI-logic + new ITER-0007b cluster + 5 stories → ITER-0008). Citation check OK (78/78).

**On resume:** roadmap split is applied + citation-clean. Next: optional confirming re-review PAR, then
decompose ITER-0007 (CI-logic slice) into TDD code + evidence tasks and dispatch `implementing-tasks`.
Baseline already clean (283 -race green, JOURNEY sentinels green).
