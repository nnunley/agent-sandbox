# EPIC-007 — Audit & observability

**Summary:** Audit & observability
**Stories:** STORY-0054
**Primary sources:** `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`
**Status:** 0/1 done

## STORY-0054

**Epic:** EPIC-007 — Audit & observability
**Title:** Audit all runs, delegations, transitions, and mutations

**As a** operator
**I want** all system actions to be logged durably and replayed
**So that** behavior can be reconstructed and debugged

**Acceptance criteria:**
- AC-1: Every run, delegation, transition, tool action, and mutation is logged · impact:`local` · seam:`unit`
- AC-2: Logs are replayable enough to reconstruct behavior · impact:`journey` · seam:`process-level`
- AC-3: Audit log is durable and immutable · impact:`local` · seam:`unit`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:132, 315-317`

**Status:** done:ITER-0008 (T5 — durable immutable replayable audit log, daemon-wired run audit; SCENARIO-0125)