# EPIC-011 — External grading & result contract

**Summary:** External grading & result contract
**Stories:** STORY-0065, STORY-0066
**Primary sources:** `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 0/2 done

## STORY-0065

**Epic:** EPIC-011 — External grading & result contract
**Title:** Harvest worker diff and result artifacts

**As a** fleet daemon
**I want** to extract worker.diff and result.json from the completed worker container and capture agent diary/knowledge
**So that** the orchestrator can grade outcomes and accumulate knowledge for future attempts

**Acceptance criteria:**
- AC-15: worker.diff is extracted from container · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-16: result.json is extracted from container · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-17: Agent diary/knowledge is captured and stored · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:318`

**Status:** pending

## STORY-0066

**Epic:** EPIC-011 — External grading & result contract
**Title:** Run authoritative external grading on clean checkout

**As a** fleet daemon
**I want** to run external grading on a pristine system-of-record checkout with worker patch applied via context-anchored patching
**So that** the grading result is authoritative and free from worker environment bias

**Acceptance criteria:**
- AC-18: External grading is skipped if grade field is absent · impact:`local` · seam:`unit` · scenario:`JOURNEY-0001`
- AC-19: Clean checkout is obtained from system-of-record (worker never touched) · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-20: Worker patch is applied via context-anchored-patching (not raw patch) · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-21: Authoritative grading produces pass/fail signal for coordination loop · impact:`cross-surface` · seam:`process-level` · scenario:`JOURNEY-0001`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:319-321`

**Status:** pending