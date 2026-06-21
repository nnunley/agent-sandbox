# Handoff-Bundle Schema (v1)

**Status:** ITER-0004 deliverable (STORY-0018 AC-3). Defines the on-disk/shared-volume contract
for the lean-ctx-carried *soft* state that passes between one-shot worker runs. This schema is the
stable boundary ITER-0006 (queue substrate) targets when it passes `Directive.HandoffIn`, and the
shape STORY-0058 AC-25 (fresh handoff bundle on retry) produces.

## Design principles (from `docs/plans/2026-06-18-fleet-orchestration-design.md:163-198`)

1. **Soft, lossy-OK.** The bundle carries *progression hints* (diary, curated knowledge, a session
   snapshot pointer) so a successor run need not reinvent. Losing or corrupting it MUST NOT affect
   correctness — the authoritative state is the code diff + the oracle grade, which never live here
   (STORY-0018 AC-4).
2. **Not a work queue.** The bundle is state passthrough, not dispatch. The durable, leased,
   crash-safe directive queue remains the only work ledger (STORY-0018 AC-5).
3. **Versioned & forward-compatible.** A `schema_version` lets ITER-0006/0008 evolve fields without
   breaking older readers; unknown fields are ignored, not fatal.

## Bundle layout (shared volume)

A handoff bundle is a directory on the shared `handoff-store` volume, referenced by
`Directive.HandoffIn` (path). Layout:

```
<handoff-store>/<thread_id>/<run_id>/
  manifest.json            # this schema (the index)
  session/<id>.json        # opaque lean-ctx session snapshot (mechanism proven by STORY-0034)
  knowledge.jsonl          # curated facts (share_knowledge / receive_knowledge), one per line
```

## `manifest.json` (schema_version 1)

```jsonc
{
  "schema_version": 1,
  "thread_id":   "string",          // owning thread (Directive.ID lineage)
  "run_id":      "string",          // the run that PRODUCED this bundle
  "parent_run_id": "string|null",   // the run this one continued from (null on first)
  "created_ts":  "RFC3339",
  // --- workflow_state: soft progression hints (NOT authoritative) ---
  "workflow_state": {
    "resume_summary": { "prior_work": "string", "next_step": "string" },
    "open_questions": ["string"],
    "current_branch": "string",
    "current_workspace": "string"
  },
  // --- session_snapshot_ref: pointer to the opaque lean-ctx session file ---
  "session_snapshot_ref": {
    "path": "session/<id>.json",    // RELATIVE to the bundle dir
    "session_id": "string"          // explicit id — REQUIRED. Resolving by `latest` is unsafe:
                                    // bare `lean-ctx session load` (id=latest) returns "starting
                                    // fresh" though the decision IS on disk (STORY-0034 spike note).
  },
  // --- curated_knowledge: index into knowledge.jsonl ---
  "curated_knowledge": { "path": "knowledge.jsonl", "count": 0 }
}
```

### Field notes

- **`session_snapshot_ref.session_id` is REQUIRED.** Importers MUST resolve the explicit id (or rely
  on lean-ctx auto-context injection). This encodes the single hardest-won lesson from the STORY-0034
  spike — see `fleet-worker/spikes/leanctx-handoff-{spike,probe}.sh` and SCENARIO-0077.
- **`workflow_state` mirrors the Thread object** (`resume_summary`, `last_verified_state`'s
  human-facing parts, branch/workspace) so a successor can hydrate without the live daemon. The
  Thread object remains the in-process source; the bundle is its serialized, volume-resident copy.
- **Authoritative state is excluded by construction.** No code diff, no oracle grade, no work-claim
  lives in the bundle. `passed()` (daemon.go) grades from `Result.ExternalGradingResult`, never from
  any field here — the property SCENARIO-0031 proves.

## Lifecycle (ctx_handoff actions — STORY-0018 AC-3)

| Action   | Who    | Effect |
|----------|--------|--------|
| `create` | worker | materialize the bundle dir + manifest at run end |
| `export` | worker | serialize the lean-ctx session into `session/<id>.json` |
| `import` | worker | hydrate from an incoming `Directive.HandoffIn` bundle (resolve explicit session id) |
| `pull`   | daemon | on requeue, assemble a FRESH bundle from the thread store for the retry (STORY-0058 AC-25) |

## Provider abstraction (no hard lean-ctx coupling) — DECISION 2026-06-21

The context/continuity layer sits behind a **Go interface**, not a direct lean-ctx dependency.
lean-ctx is *one adapter*, the default — never a hard dependency. Rationale: avoid vendor lock-in;
lean-ctx carries a commercial-license upsell for teams/distributed operation, so swappability is a
requirement, not a nicety. This mirrors how **coordination** is already abstracted behind
`queue.Queue` (ITER-0006 swaps the substrate) — the same discipline now applies to **context**.

```go
// ContextProvider is the seam between the coordinator/runner and whatever carries SOFT state
// between one-shot runs. It is intentionally small and bundle-centric: the durable, authoritative
// state (diff + oracle grade) never flows through here (STORY-0018 AC-4).
type ContextProvider interface {
    // diary — STORY-0018 AC-1
    WriteDiary(threadID string, d DiaryEntry) error
    RecallDiary(threadID string) ([]DiaryEntry, error)
    // knowledge — STORY-0018 AC-2
    ShareKnowledge(threadID string, facts []Fact) error
    ReceiveKnowledge(threadID string) ([]Fact, error)
    // handoff bundle (this schema) — STORY-0018 AC-3
    CreateHandoff(threadID, runID string, st WorkflowState) (bundlePath string, err error)
    ImportHandoff(bundlePath string) (HandoffManifest, error)
}
```

Adapters built now (YAGNI — only what we need):
- **`LeanCtxProvider`** — the default; shells out to `lean-ctx session`/`ctx_agent`/`ctx_handoff`,
  resolving the explicit saved session id (NOT `latest`) per the STORY-0034 spike note. The session
  snapshot it writes is the only lean-ctx-specific artifact, and it is opaque to everyone else.
- **`NoopProvider`** — a test/fallback double that drops all soft state. It is also the mechanism
  that PROVES STORY-0018 AC-4: with the noop provider (handoff effectively lost), the daemon still
  grades correctly from `Result.ExternalGradingResult`. So the abstraction and the anti-reward-hack
  proof are the same lever.

Do NOT build speculative alternative adapters (Redis/S3/etc.) until a second backend is actually
needed — the interface is the insurance; extra adapters are not.

## Compatibility contract (boxing-in mitigation)

- ITER-0006 reads only `Directive.HandoffIn` (a path) — it never parses the manifest, so substrate
  swap is decoupled from the schema.
- ITER-0008 may ADD fields (e.g. delegation lineage) under new keys; readers ignore unknown keys.
- Bumping `schema_version` is reserved for breaking changes; v1 readers reject only on a higher major
  they don't understand, never on extra fields.
