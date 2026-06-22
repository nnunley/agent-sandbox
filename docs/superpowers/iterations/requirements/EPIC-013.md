# EPIC-013 — Worker image & skills

**Summary:** Worker image & skills
**Stories:** STORY-0075, STORY-0076, STORY-0077, STORY-0078
**Primary sources:** `docs/plans/2026-06-17-dispatcher-productization.md`, `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 0/4 fully done (STORY-0075 PARTIAL — minimal worker-image slice done:ITER-0000; FULL
golden + STORY-0076/0077/0078 → ITER-0005b)
## STORY-0075

**Epic:** EPIC-013 — Worker image & skills
**Title:** NixOS golden image with cached substitution (retire Ubuntu stopgap)

**As a** worker executor
**I want** to run a NixOS golden container built once with nix develop ./fleet-worker fully realized, then copy and use it for each task without rebuilding
**So that** I avoid nix build-sandbox failures in unprivileged containers and rely entirely on binary substitution from cache.numtide.com

**Acceptance criteria:**
- AC-1: NixOS golden is built once (`nix develop ./fleet-worker --accept-flake-config` fully realized with claude-code, lean-ctx, go, make); snapshot as golden image; incus copy golden <task-name> per task works · impact:`local` · seam:`integration` · scenario:`SCENARIO-0065`
- AC-2: runner inside golden executes `nix develop --command bash runner.sh` which does lean-ctx setup+serve, then `claude -p`; clean-room integrity gate still holds (byte-identical regen of core_compiled.lgb, core_go_lowered/, generated.sums) · impact:`journey` · seam:`e2e` · scenario:`SCENARIO-0065`
- AC-3: NixOS golden runs focused Level-style brief headless with lean-ctx bridge ON, produces graded diff, with no Ubuntu fallback required · impact:`journey` · seam:`e2e` · scenario:`SCENARIO-0065`

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:149-159`

**Status:** partial — **minimal worker-image slice done:ITER-0000** (stock NixOS + non-root `worker` +
`nix develop ./fleet-worker` substituted from cache; real dogfood ran headless + produced a 10/10
oracle-graded diff, no Ubuntu fallback — substance of AC-3 minus the bridge). **FULL golden →
ITER-0005b:** AC-1 (build once + snapshot as golden + `incus copy` per task), AC-2 (clean-room
integrity gate: byte-identical regen of generated artifacts), AC-3 with the lean-ctx bridge ON
(STORY-0069 landed the bridge in ITER-0003; the golden-run-with-bridge graded proof is ITER-0005b).

## STORY-0076

**Epic:** EPIC-013 — Worker image & skills
**Title:** Provider routing with llm-agents.nix binaries (cheap implementer + strong grader)

**As a** cost optimizer
**I want** to use cheap model implementers (Sonnet via proxy → Haiku/OpenAI/Ollama at ndn.local:11434) while keeping the grader/oracle deterministic (no model)
**So that** I minimize cost while preserving grading rigor

**Acceptance criteria:**
- AC-1: golden exports codex/gemini-cli/qwen-code from llm-agents.nix; dispatcher routes implementer via --provider (anthropic|openai|ollama-cloud) and --model flags; grader remains deterministic (oracle is git-based, not LLM-based) · impact:`local` · seam:`integration` · scenario:`SCENARIO-0067`

**Sources:**
- `docs/plans/2026-06-17-dispatcher-productization.md:161-165`

**Status:** pending

## STORY-0077

**Epic:** EPIC-013 — Worker image & skills
**Title:** Declaratively vendor curated skills subset via agent-skills-nix

**As a** fleet orchestration system
**I want** to bring the ~13-skill subset into worker config declaratively (not by copying files) using Kyure-A/agent-skills-nix as a hash-pinned flake input
**So that** skills are reproducibly vendored, offline-available, and immutably baked into the worker image without file duplication

**Acceptance criteria:**
- AC-1: agent-skills-nix flake input is added to worker config with hash-pinned upstream skills repo reference · impact:`local` · seam:`integration` · scenario:`SCENARIO-0068`
- AC-2: selectSkills/mkBundle is used to curate the subset: using-laneq, low-level-executor-task-spec, process-aware-done, verify-from-system-of-record, verify-real-artifact, gate-before-push, graceful-shutdown-stateful-agents, restart-resilience, yield-on-wait, push-over-polling, credential-proxy, context-anchored-patching, agent-otel-trajectory · impact:`local` · seam:`integration` · scenario:`SCENARIO-0068`
- AC-3: Skill bundle is placed at the path claude -p discovers: environment.etc."claude/skills".source = bundle · impact:`cross-surface` · seam:`integration` · scenario:`SCENARIO-0068`
- AC-4: copy-tree (not symlink) is used for immutable, offline-accessible image · impact:`local` · seam:`process-level` · scenario:`SCENARIO-0068`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:332-347`

**Status:** pending

## STORY-0078

**Epic:** EPIC-013 — Worker image & skills
**Title:** Resolve upstream skills layout and bundle filter configuration

**As a** worker image build system
**I want** to confirm the upstream skills' subdir layout (subdir/idPrefix) and filter.maxDepth for flat-vs-nested SKILL.md discovery
**So that** the bundle correctly represents all skill metadata and discovery paths align with upstream changes

**Acceptance criteria:**
- AC-5: Upstream skills subdir layout (subdir/idPrefix) is documented and validated against agent-skills-nix expectations · impact:`local` · seam:`integration`
- AC-6: filter.maxDepth configuration is set to handle flat-vs-nested SKILL.md changes logged upstream · impact:`local` · seam:`integration`

**Sources:**
- `docs/plans/2026-06-18-fleet-orchestration-design.md:349-350`

**Status:** pending — **ITER-0005c, GATES STORY-0077 (PAR 2026-06-21, A):** this is pre-work
DISCOVERY — confirm the upstream agent-skills-nix subdir/idPrefix layout + `filter.maxDepth` BEFORE
building the bundle (STORY-0077). AC-5/AC-6 proof = a validated layout doc + the resolved bundle
exhibiting the expected discovery paths (folded into SCENARIO-0068/0069 evidence; no separate
scenario). Run FIRST in ITER-0005c so a broken/changed upstream layout surfaces before bundle config.