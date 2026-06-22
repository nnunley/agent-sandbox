# Skills layout validation (STORY-0078, ITER-0005c)

**Status:** validated on the cluster 2026-06-22 (SCENARIO-0069 PASS).
**Purpose:** STORY-0078 AC-5/AC-6 deliverable — confirm the upstream skills repo's
`subdir`/`idPrefix` layout and the `filter.maxDepth` needed for correct discovery, BEFORE
STORY-0077 bakes the curated bundle into the golden. This is the pre-work gate that GATES
STORY-0077.

## Inputs (both hash-pinned via `fleet-worker/flake.lock`)

| Input | Source | Rev (2026-06-22) | Role |
|---|---|---|---|
| `agent-skills-nix` | `github:Kyure-A/agent-skills-nix` | `5ff9039` | library: `discoverCatalog` / `selectSkills` / `mkBundle` (exposed at `agent-skills-nix.lib.agent-skills`) |
| `agent-skills` | `github:selamy-labs/agent-skills` | `22ac232` | upstream skills repo — **NON-FLAKE**, consumed as `flake = false` |

`agent-skills` has **no `flake.nix`**, so it MUST be declared `flake = false`. The source
config uses `path = "${agent-skills}"` (its store path) rather than `input = "agent-skills"`,
which decouples resolution from agent-skills-nix's own `inputs` set (its lib resolves
`input` names against the inputs captured at *its* eval time, not ours).

## Upstream layout (AC-5)

```
<agent-skills>/skills/<skill-name>/SKILL.md     # flat: exactly one level under skills/
```

- 96 skills total upstream, each a directory directly under `skills/` containing `SKILL.md`.
- No nested skill grouping → `idPrefix` is unnecessary (`null`).

## Source / filter configuration (AC-6)

```nix
skillSources = {
  upstream = {
    path = "${agent-skills}";   # non-flake source root
    subdir = "skills";          # skills live under skills/
    filter.maxDepth = 1;        # flat: SKILL.md is one level under subdir
  };
};
```

`filter.maxDepth = 1` matches the current flat layout. `agent-skills-nix`'s `discoverSource`
recursively scans for `SKILL.md` directories up to `maxDepth` (null = unlimited, internally
capped at 100). If upstream later switches to a **nested** SKILL.md layout (the flat-vs-nested
change logged upstream), bump `filter.maxDepth` accordingly and re-run SCENARIO-0069 — the
bundle build will surface a count mismatch if discovery breaks.

## Curated subset (13 skills — STORY-0077 AC-2)

All confirmed present upstream and resolved by `selectSkills`:

```
using-laneq, low-level-executor-task-spec, process-aware-done,
verify-from-system-of-record, verify-real-artifact, gate-before-push,
graceful-shutdown-stateful-agents, restart-resilience, yield-on-wait,
push-over-polling, credential-proxy, context-anchored-patching,
agent-otel-trajectory
```

## Bundle build proof (SCENARIO-0069)

```
bash fleet-worker/cluster-tests/run.sh skills-discovery
# → PASS: bundle /nix/store/…-agent-skills-bundle has 13 SKILL.md
```

The bundle is built by `nix build .#agent-skills-bundle` on `nix-server` (the small
copy/rsync derivation — closure is rsync + the 13 skill source trees, **not** the golden,
toolchain, or claude-code). It needs `--no-sandbox` (nix-server is an unprivileged LXC with
no kernel-namespace build sandbox) and flakes enabled.

## Note for STORY-0077 (T2): symlink-tree → copy-tree

`mkBundle`'s default output is a **symlink-tree**: each `<skill-name>` entry under the bundle
store path is a symlink into a `--safe-links`-rsync'd source store path (those source trees
are real files in the store, so the closure is offline-complete). STORY-0077 AC-4 requires a
**copy-tree (regular files, not symlinks)** at the discovery path for an immutable, offline
image. T2 therefore materializes a copy-tree (`cp -rL` of the bundle) and places it at
`environment.etc."claude/skills".source`, and SCENARIO-0068 asserts no symlinked skill
entries remain.
