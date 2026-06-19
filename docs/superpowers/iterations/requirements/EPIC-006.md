# EPIC-006 — Provisioning & template security

**Summary:** Provisioning & template security
**Stories:** STORY-0048, STORY-0049, STORY-0050, STORY-0051, STORY-0052, STORY-0053
**Primary sources:** `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md`, `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 1/6 done
## STORY-0048

**Epic:** EPIC-006 — Provisioning & template security
**Title:** Broker provider secrets and prevent raw exposure to workers

**As a** coordinator
**I want** workers to never receive raw long-lived provider secrets
**So that** credential compromise is bounded

**Acceptance criteria:**
- AC-1: Workers never receive raw provider API keys · impact:`local` · seam:`unit` · scenario:`SCENARIO-0020`
- AC-2: Worker accesses providers through broker proxy surface · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0020`
- AC-3: Mac trust root / broker remains master secret boundary · impact:`local` · seam:`unit` · scenario:`SCENARIO-0020`

**Sources:**
- `docs/plans/2026-06-17-coordinator-bootstrap-requirements.md:127-128, 342-346`

**Status:** pending

## STORY-0049

**Epic:** EPIC-006 — Provisioning & template security
**Title:** D1: Directives carry intent + proposed template, never direct commands

**As a** daemon operator
**I want** to validate directive origin + template allowlist before launching any container
**So that** a compromised worker cannot escalate to root or run arbitrary privileged commands

**Acceptance criteria:**
- AC-1: Directive payload contains only intent and proposed_template name, never access_cmd or root flag · impact:`local` · seam:`unit` · scenario:`SCENARIO-0025`
- AC-2: Daemon rejects directive if proposed_template is not in allowlist · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0025`
- AC-3: Daemon rejects directive if origin (worker identity) is not permitted for proposed_template · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0025`
- AC-4: Worker-authored child directives carry task content only; provisioning is inherited and never privileged · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0025`
- AC-5: Launched template is immutable root with writable scratch (/workspace, /tmp tmpfs/overlay) · impact:`local` · seam:`app-level` · scenario:`SCENARIO-0025`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:99-116`

**Status:** pending

## STORY-0050

**Epic:** EPIC-006 — Provisioning & template security
**Title:** Validate template against allowlist and origin

**As a** fleet daemon
**I want** to validate the proposed template against an allowlist and verify its origin before execution
**So that** only authorized templates from trusted sources execute on workers

**Acceptance criteria:**
- AC-3: Template identity is matched against configured allowlist · impact:`local` · seam:`unit` · scenario:`JOURNEY-0001`
- AC-4: Template origin is verified per D1 specification · impact:`local` · seam:`unit` · scenario:`JOURNEY-0001`
- AC-5: Invalid templates are rejected with clear error reason · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:313`

**Status:** done:ITER-0000

## STORY-0051

**Epic:** EPIC-006 — Provisioning & template security
**Title:** Launch worker container from golden image with shared volumes

**As a** fleet daemon
**I want** to copy the golden container image to create a fresh ephemeral instance and attach shared volumes
**So that** each directive runs in clean isolation with access to shared nix cache and handoff storage

**Acceptance criteria:**
- AC-6: Golden image is copied to fresh container with unique name (golden → fresh-name) · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-7: Shared nix cache volume is attached to fresh container · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-8: Lean-context handoff store volume is attached to fresh container · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:314-315`

**Status:** pending

## STORY-0052

**Epic:** EPIC-006 — Provisioning & template security
**Title:** Deliver repository to worker via bundle/clone and import handoff

**As a** fleet daemon
**I want** to deliver the target repository to the worker container and import any prior handoff context
**So that** the worker has a complete working copy with all accumulated context from prior attempts

**Acceptance criteria:**
- AC-9: Repository is delivered via bundle or clone to worker container · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-10: If handoff_in is present, ctx_handoff import is executed on worker · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`
- AC-11: Handoff import correctly restores prior context state · impact:`local` · seam:`integration` · scenario:`JOURNEY-0001`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:316`

**Status:** pending

## STORY-0053

**Epic:** EPIC-006 — Provisioning & template security
**Title:** Intent/template provisioning with security

**As a** platform operator
**I want** to enforce origin-based template restrictions so that worker-origin proposals for privileged templates are denied
**So that** untrusted actors cannot inject elevated operations into the workflow

**Acceptance criteria:**
- AC-1: a worker-origin privileged-template proposal is rejected with denial reason logged · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0074`
- AC-2: allowlist evaluation is deterministic across concurrent daemon instances · impact:`local` · seam:`unit` · scenario:`SCENARIO-0074`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:396-397`

**Status:** pending