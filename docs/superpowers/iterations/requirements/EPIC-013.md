# EPIC-013 — Worker image & skills

**Summary:** Worker image & skills
**Stories:** STORY-0075, STORY-0076, STORY-0077, STORY-0078
**Primary sources:** `docs/plans/2026-06-17-dispatcher-productization.md`, `docs/plans/2026-06-18-fleet-orchestration-design.md`
**Status:** 3/4 fully done (ITER-0005c). STORY-0076/0077/0078 done:ITER-0005c. STORY-0075 PARTIAL:
AC-1 (FULL golden build/snapshot/copy + realized toolchain + skills) done:ITER-0005c; AC-2/AC-3
(clean-room byte-identical regen + bridge-ON graded run) CARRIED — blocked by an upstream let-go
native-Go-lowering codegen bug (non-compiling regenerated test pkg), reproduced on the pinned
toolchain (same blocker as STORY-0068 AC-2 / JOURNEY-0003).
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
(STORY-0069 landed the bridge in ITER-0003; the golden-run-with-bridge graded proof is ITER-0005c).

**ITER-0005c AC-ordering + carry-allowance (PAR 2026-06-22 — A "not split-worthy" vs B "split":
resolved by task-level ordering, no renumber):** the 3 ACs have heterogeneous dependency profiles, so
they are decomposed into ordered tasks with separate scenario evidence rather than renumbered into
0075a/b/c (avoids requirements-index churn):
- **AC-1** (build-once + snapshot + `incus copy` per task) · seam `integration` · SCENARIO-0065 —
  the must-pass core; harness-gated; depends only on the ITER-0005b golden substrate (done).
- **AC-2** (clean-room byte-identical regen of `core_compiled.lgb`/`core_go_lowered/`/`generated.sums`)
  · seam `e2e` · SCENARIO-0066 — a toolchain-sensitive GATING proof; needs the real `let-go` repo +
  nix-pinned toolchain. **This is the same toolchain-sensitivity ITER-0003 STORY-0068 AC-2 hit.**
- **AC-3** (bridge-ON headless graded run, no Ubuntu fallback) · seam `e2e` · SCENARIO-0066 (extended:
  the graded run MUST execute with the lean-ctx bridge active — resolves A-S2/B "bridge proof
  unspecified") — composes AC-1 + AC-2 + STORY-0069 bridge.
**Carry-allowance (precedent: ITER-0003 STORY-0068 AC-2):** AC-1 must pass this iteration. If AC-2/AC-3
hit the let-go-toolchain wall on the cluster within the iteration, STORY-0075 is marked PARTIAL (AC-1
done; AC-2/AC-3 carried with an explicit toolchain reason + a captured fixture) rather than blocking
the whole image track. The skills/provider work (0077/0076/0078) does NOT depend on AC-2/AC-3.

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

**Status:** done:ITER-0005c (AC-1 — golden exports codex/gemini/qwen + dispatcher --provider/--model
passthrough + deterministic grader; SCENARIO-0067 cluster PASS + TestScenario0067 CI). **AC-1 scope
clarification (PAR 2026-06-22, B-SERIOUS resolved):**
AC-1 decomposes into two already-distinct concerns, neither needing a new story: (a) the **golden
exports** `codex`/`gemini-cli`/`qwen-code` from `llm-agents.nix` (nix-level: uncomment the commented
line in `fleet-worker/flake.nix:55`), and (b) the **dispatcher routing** of `--provider`/`--model`
already exists (`modules/incus-dispatcher` flags + `modules/llm-proxy/proxy.go` per-provider routes) —
proven by a contract test that the dispatcher passes the flags through and the grader stays
deterministic (the oracle is git-based, no LLM). SCENARIO-0067 asserts (a) export presence + (b)
flag-passthrough/grader-determinism.

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

**Status:** done:ITER-0005c (AC-1..AC-4 — agent-skills-nix flake input hash-pinned; selectSkills/mkBundle curate the 13-skill subset; baked at environment.etc."claude/skills" via copy-tree real files; SCENARIO-0068 cluster PASS: 13 SKILL.md, 0 symlinks).

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

**Status:** done:ITER-0005c (AC-5/AC-6 — layout validated + bundle builds with all 13 skills, SCENARIO-0069 cluster PASS). **GATES STORY-0077 (PAR 2026-06-21, A):** this is pre-work
DISCOVERY — confirm the upstream agent-skills-nix subdir/idPrefix layout + `filter.maxDepth` BEFORE
building the bundle (STORY-0077). AC-5/AC-6 proof = a validated layout doc + the resolved bundle
exhibiting the expected discovery paths (folded into SCENARIO-0068/0069 evidence; no separate
scenario). Run FIRST in ITER-0005c so a broken/changed upstream layout surfaces before bundle config.

**ITER-0005c proof clarification (PAR 2026-06-22 — A-S3 + B-C2 "proof undefined / timing paradox"
resolved):** STORY-0078's standalone gate is the **bundle BUILD itself** —
`nix build .#agent-skills-bundle` (the `mkBundle` of the `selectSkills` of the 13 ids). This needs
only the small bundle derivation, NOT the golden image, so it runs BEFORE STORY-0077 bakes the bundle
into the golden — no timing paradox. The build succeeding with all 13 expected skill ids present at
the discovery layout IS the AC-5/AC-6 proof (SCENARIO-0069), accompanied by the layout-validation doc
`docs/plans/2026-06-22-skills-layout-validation.md`. **Discovery already executed on the cluster
(2026-06-22):** upstream `github:selamy-labs/agent-skills` (rev 22ac232) is NON-FLAKE → `flake = false`
input; flat layout `skills/<name>/SKILL.md` (96 skills) → source cfg `subdir = "skills"`,
`idPrefix = null`, `filter.maxDepth = 1`; agent-skills-nix `github:Kyure-A/agent-skills-nix` (rev
5ff9039) exposes `lib.selectSkills` + `lib.mkBundle`; all 13 curated skills confirmed present.
Seam note (B-minor): AC-5/AC-6 are nix-eval/build proofs (need nix, not the microVM substrate).