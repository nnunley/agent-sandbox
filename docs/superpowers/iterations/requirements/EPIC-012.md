# EPIC-012 — Worker reliability & comms

**Summary:** Worker reliability & comms
**Stories:** STORY-0067, STORY-0068, STORY-0069, STORY-0070, STORY-0071, STORY-0072, STORY-0073, STORY-0074
**Primary sources:** `docs/plans/2026-06-17-dispatcher-productization.md`, `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 0/8 done

## STORY-0067

**Epic:** EPIC-012 — Worker reliability & comms
**Title:** Go-exec PATH resolution for dispatcher client runner

**As a** dispatcher user
**I want** to execute commands via the incus Go client with access to the same PATH as interactive shell login
**So that** commands like `claude --version` resolve successfully instead of failing with exit 127

**Acceptance criteria:**
- AC-1: incus-dispatcher --cmd 'claude --version && go version && lean-ctx --version' returns exit 0 with all three tool versions printed via the Go client (not incus exec shelling) · impact:`cross-surface` · seam:`app-level` · scenario:`SCENARIO-0060`

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:41-50`

**Status:** pending

## STORY-0068

**Epic:** EPIC-012 — Worker reliability & comms
**Title:** External grading round-trip for worker diff validation

**As a** orchestrator
**I want** to grade a worker diff by applying it to a pristine checkout and regenerating artifacts, with deterministic oracle verification
**So that** the worker output is validated independent of the worker's self-report (anti-reward-hack)

**Acceptance criteria:**
- AC-1: dispatcher subcommand (or scripts/grade.sh) takes target ref + worker diff, applies source files wholesale to clean checkout, runs `make generate`, then runs oracle tests (go test -tags gogen_ir ./pkg/ir/, make check-generated, untagged, e2e), and emits structured grade JSON {passed, clusterA, check_generated, untagged_fails, e2e} · impact:`journey` · seam:`process-level` · scenario:`JOURNEY-0003`
- AC-2: grading reproduces the proven 13→0 result from the harvested /tmp/lvl1-focused.diff using the structured grade JSON · impact:`journey` · seam:`e2e` · scenario:`JOURNEY-0003`

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:52-65`

**Status:** pending

## STORY-0069

**Epic:** EPIC-012 — Worker reliability & comms
**Title:** lean-ctx full enablement with bridge daemon and shell compression

**As a** worker runner
**I want** to register lean-ctx MCP server, start the bridge daemon, and route all shell/read operations through ctx_* tools with measured token savings
**So that** the worker has verified shell-hook compression and the heartbeat can report accurate savings (bridge ON, not OFF)

**Acceptance criteria:**
- AC-1: post-run `lean-ctx gain` reports a non-zero measured savings number with 'Bridge: ON' status (not 'Bridge: OFF — proxy not reachable') · impact:`local` · seam:`unit` · scenario:`SCENARIO-0061`
- AC-2: runner invokes `lean-ctx setup` (fuller config than init) and starts bridge via `lean-ctx serve &` before launching `claude -p`; `lean-ctx status` confirms connected · impact:`local` · seam:`integration` · scenario:`SCENARIO-0061`

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:67-78`

**Status:** pending

## STORY-0070

**Epic:** EPIC-012 — Worker reliability & comms
**Title:** Canonical runner shape with fresh and continuation modes

**As a** worker orchestrator
**I want** a runner that supports both fresh task (reset + clean tree) and continuation (keep applied diff) modes
**So that** the same runner script works for independent tasks and iterative debugging

**Acceptance criteria:**
- AC-1: runner accepts --fresh (reset tree, clean state) and --continue (preserve applied diff) mode flags; both modes set PATH, run lean-ctx setup+serve, harvest worker.diff and result.json, and write lean-ctx gain output · impact:`local` · seam:`integration`

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:85-90`

**Status:** pending

## STORY-0071

**Epic:** EPIC-012 — Worker reliability & comms
**Title:** Heartbeat projection must track ctx_* tools, not just Bash

**As a** orchestrator
**I want** to see the worker's actual activity in the heartbeat by projecting from ctx_shell/ctx_read (with Bash as fallback)
**So that** the heartbeat accurately reflects work even when the worker routes all commands through lean-ctx

**Acceptance criteria:**
- AC-1: working-state projector reads events.jsonl and emits {alive, eventCount, Δsince_last, last_shell_cmd (ctx_shell|Bash), last_read, phase_guess}; phase_guess is derived from brief gate commands (go build → compile; go test ...pkg/ir → oracle; make check-generated/go test ./... → regress) · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0062`
- AC-2: heartbeat no longer shows '(no shell yet)' when worker is actively running ctx_shell commands; it accurately reports the last shell command and its timestamp · impact:`cross-surface` · seam:`app-level` · scenario:`SCENARIO-0062`

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:96-104`

**Status:** pending

## STORY-0072

**Epic:** EPIC-012 — Worker reliability & comms
**Title:** Robust result contract surviving worker truncation

**As a** orchestrator
**I want** to always receive structured result.json even if the worker runs out of turns or context before writing it
**So that** the orchestrator has a fallback result and can delegate grading to the authoritative external grader without waiting for the worker to recover

**Acceptance criteria:**
- AC-1: runner synthesizes a fallback result.json on exit if none exists: captures the last oracle command output and writes {status: UNKNOWN, harvested_diff_path} so orchestrator always has structured output · impact:`journey` · seam:`process-level` · scenario:`SCENARIO-0063`
- AC-2: external grader (#25.2) is the source of truth for pass/fail regardless of the worker's self-report; worker result.json is advisory only (anti-reward-hack) · impact:`journey` · seam:`e2e` · scenario:`SCENARIO-0063`

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:106-112`

**Status:** pending

## STORY-0073

**Epic:** EPIC-012 — Worker reliability & comms
**Title:** Tier-2 bidirectional coordinator with file-feed steering

**As a** orchestrator
**I want** to send steering messages to a running worker mid-run (e.g., 'stop, you've drifted; here's the precise pointer') via a watched file
**So that** the orchestrator can correct the worker's course without restarting it

**Acceptance criteria:**
- AC-1: orchestrator writes a steer message to a watched file in the container; worker polls the file between phase boundaries and acknowledges the message in events.jsonl within one phase boundary · impact:`journey` · seam:`process-level` · scenario:`SCENARIO-0064`

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:114-124`

**Status:** pending

## STORY-0074

**Epic:** EPIC-012 — Worker reliability & comms
**Title:** Mac-off acceptance test

**As a** operator
**I want** to run the headline acceptance test: with the Mac disconnected, the cluster claims, runs, grades, escalates, a successor resumes via handoff, and human-only escalations queue durably for the Mac's return
**So that** the system proves it can operate without human infrastructure and resume cleanly on reconnection

**Acceptance criteria:**
- AC-1: Mac offline: daemon claims a task and runs it to completion · impact:`journey` · seam:`e2e` · scenario:`JOURNEY-0004`
- AC-2: Mac offline: autonomous grading produces a result without human feedback · impact:`journey` · seam:`e2e` · scenario:`JOURNEY-0004`
- AC-3: Mac offline: low-cost escalations proceed autonomously; privileged escalations queue in escalations lane · impact:`journey` · seam:`e2e` · scenario:`JOURNEY-0004`
- AC-4: Mac offline: successor task resumes via ctx_handoff without replaying completed work · impact:`journey` · seam:`e2e` · scenario:`JOURNEY-0004`
- AC-5: Mac returns: human-only escalations queued during downtime are processed and old context is discarded · impact:`journey` · seam:`e2e` · scenario:`JOURNEY-0004`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:416-418`

**Status:** pending