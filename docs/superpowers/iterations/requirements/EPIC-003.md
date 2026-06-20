# EPIC-003 — Mac-off governance & constraints

**Summary:** Mac-off governance & constraints
**Stories:** STORY-0026, STORY-0027, STORY-0028
**Primary sources:** `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`, `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 0/3 done
## STORY-0026

**Epic:** EPIC-003 — Mac-off governance & constraints
**Title:** Mac-off SPOF constraint: fleet autonomy with Mac powered down

**As a** fleet operator
**I want** the cluster to claim, execute, grade, and hand off work while the Mac is powered off
**So that** the Mac is a thin optional client, never a bottleneck or single point of failure

**Acceptance criteria:**
- AC-1: Coordination plane (queue) runs on cluster infrastructure (ndn-desktop), not on the user's Mac · impact:`cross-surface` · seam:`process-level` · scenario:`SCENARIO-0010`
- AC-2: Provisioner/coordinator daemon runs on the cluster (ndn-desktop), not on the Mac · impact:`cross-surface` · seam:`process-level` · scenario:`SCENARIO-0010`
- AC-3: State-passthrough store (lean-ctx handoff) persists on cluster storage, accessible when Mac is off · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0010`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:14-26`

**Status:** pending

## STORY-0027

**Epic:** EPIC-003 — Mac-off governance & constraints
**Title:** Track thread status through lifecycle states

**As a** coordinator
**I want** threads to move through explicit states: queued, active, paused, blocked, done, abandoned
**So that** thread lifecycle is observable and can be managed

**Acceptance criteria:**
- AC-1: Thread object includes status field with values: queued, active, paused, blocked, done, abandoned · impact:`local` · seam:`unit`
- AC-2: System emits status events when thread transitions between states · impact:`cross-surface` · seam:`integration`
- AC-3: Operator can pause, block, or resume threads from TUI · impact:`local` · seam:`integration`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:149, 513-514`

**Status:** partial:ITER-0001 (AC-1 status field + AC-2 transitions done); AC-3 TUI pause/block/resume→ITER-0008

## STORY-0028

**Epic:** EPIC-003 — Mac-off governance & constraints
**Title:** Provide operator TUI for thread and worker management

**As a** operator
**I want** a TUI to create work items, inspect queue/worker state, and manage responses
**So that** I can supervise and direct the system

**Acceptance criteria:**
- AC-1: TUI runs on Mac and allows creating work items · impact:`local` · seam:`app-level` · scenario:`SCENARIO-0021`
- AC-2: TUI displays queue and worker state · impact:`local` · seam:`app-level` · scenario:`SCENARIO-0021`
- AC-3: TUI allows inspecting responses and artifacts · impact:`local` · seam:`app-level` · scenario:`SCENARIO-0021`
- AC-4: TUI allows responding, requeuing, or pausing threads · impact:`local` · seam:`app-level` · scenario:`SCENARIO-0021`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:552-559`

**Status:** pending