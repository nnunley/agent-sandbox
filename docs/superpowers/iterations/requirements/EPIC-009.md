# EPIC-009 — Teardown & reaper

**Summary:** Teardown & reaper
**Stories:** STORY-0062, STORY-0063
**Primary sources:** `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 2/2 done
## STORY-0062

**Epic:** EPIC-009 — Teardown & reaper
**Title:** D5: Stop first (bounded timeout), then delete; leaked instances go to out-of-band reaper

**As a** teardown orchestrator
**I want** to stop containers with timeout before deleting, and handle stop-timeout instances via out-of-band reaper
**So that** coordination loop never blocks on teardown and running instances are not force-deleted

**Acceptance criteria:**
- AC-1: Stop container with incus stop --timeout (or client UpdateInstanceState) before delete · impact:`local` · seam:`unit` · scenario:`SCENARIO-0039`
- AC-2: Stop timeout instances routed to out-of-band reaper, not hot path · impact:`cross-surface` · seam:`process-level` · scenario:`SCENARIO-0039`
- AC-3: Launch via incus copy from golden with fresh names to prevent collision on leaked instance · impact:`local` · seam:`integration` · scenario:`SCENARIO-0039`
- AC-4: Reaper sweeps timed-out instances without blocking coordination loop · impact:`cross-surface` · seam:`process-level` · scenario:`SCENARIO-0039`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:209-221`

**Status:** done:ITER-0000

## STORY-0063

**Epic:** EPIC-009 — Teardown & reaper
**Title:** Stop worker container and reap instance with decision log

**As a** fleet daemon
**I want** to stop the worker container, delete the instance, and write a decision log entry
**So that** ephemeral resources are cleaned up and all decisions are audited

**Acceptance criteria:**
- AC-26: Worker container is stopped · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-27: Worker instance is deleted by reaper · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-28: Decision log line is written per D6 specification · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:325`

**Status:** done:ITER-0001 (AC-26/27 stop+reap ITER-0000; AC-28 decision-log write on reap ITER-0001)