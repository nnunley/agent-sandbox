# Fleet Retrieval + Consolidation over graphrag (slice 1)

**Status:** design approved in brainstorming (2026-07-31); spec for review.

Adds a retrieval/synthesis layer over the fleet's own soft state. graphrag becomes the
hybrid-retrieval engine and knowledge-graph builder over the diary / curated-knowledge /
handoff artifacts that runs already emit — never over the operator's personal brain. Origin:
critical read of the MOMENTUM paper (jeanlabs.ai). MOMENTUM bundles retrieval + assembly +
synthesis into one ADK monolith with marketing-domain glue; this repo already has better,
separated primitives (graphrag, lean-ctx, laneq). The one genuinely transferable idea is the
*composition* MOMENTUM performs that this fleet does not yet: relevance-ranked context
assembled from accumulated soft state. This spec is that composition, slice 1.

## Goals

1. A fleet run's opening context is **retrieved by relevance to its task brief** from
   accumulated fleet soft state, instead of the current all-facts-for-a-thread dump
   (`ReceiveKnowledge` today returns everything, unranked).
2. graphrag's knowledge-graph **synthesis** (triples/relations + Leiden communities) runs as
   **fleet work** — the fleet is the LLM compute substrate; consolidating its own soft state
   into a graph is an ordinary (ungraded) fleet task.
3. The whole subsystem is **soft state**: best-effort, never a work source, never on the
   diff/grade path. Disabling it entirely must not change any run's grade.

## Non-goals (slice 1)

- Context-window token budgeting / multi-source merge (top-k is enough for now) — **slice 2**.
- Explicit "project soul" summary surfacing — mostly falls out of community summaries once
  synthesis runs; revisit as **slice 3** only if community output proves insufficient.
- Multi-tenant / cross-thread store scoping — single per-project store for now.
- Retrieving from the operator's personal graphrag (brain/conversations) stores. Out of
  bounds by design (privacy boundary).

## Key premises (from brainstorming, load-bearing)

- **Indexing ≠ a passive reindex; downstream indexing *is* synthesis.** graphrag's triple
  extraction, consolidation/dedupe, and Leiden community summarization are LLM work. So
  "retrieval adapter" and "synthesis" are the same subsystem viewed from read vs. write —
  they cannot be phased apart.
- **Base indexing is total and immediate; only downstream synthesis lags.** Chunk + embed
  (HNSW) + BM25 covers *all* soft state as it lands (cheap, non-LLM), so **hybrid base
  retrieval (RRF) always works right away**. The graph/community layer is an async batch job
  that increments as content is added and may be incomplete at any moment.
- **Corpus = fleet soft state only.** Diary, curated knowledge, handoff artifacts — the
  outputs of the existing `ContextProvider` path.
- **Placement.** graphrag/lean-ctx run coordinator-side; workers are ephemeral isolated VMs.
  The store lives on the shared volume (as `BundleRoot` does) so consolidation workers and
  the coordinator-side reader both reach it.

## Architecture

A per-project **fleet graphrag store** on the shared volume, with two layers over it:

```
run completes
  └─ ContextProvider.WriteDiary / ShareKnowledge / CreateHandoff      (exists)
        └─ base-index hook → fleet graphrag store                     (vector+BM25, immediate, best-effort, no LLM)

async, decoupled
  └─ consolidate task (laneq) → worker runs graphrag downstream synthesis
        └─ extract triples/relations, merge/dedupe, update Leiden communities   (LLM, lags freely, ungraded)

next run claims work (coordinator)
  └─ GraphRAGProvider.Retrieve(threadID, brief, k)
        ├─ base hybrid (RRF)          → always available
        └─ + community expansion      → when synthesis has caught up
        └─ []RankedFact { Fact, Score, SourceRef }
  └─ materialize top-k into the task's context                        (handoff bundle / context file on shared volume — existing transport)
  └─ worker launches with that context in its workspace
```

The **query** is the task brief (the story/scenario/`Cmd` already on `Task`). The
**materialization** reuses the existing handoff / shared-volume path — no new transport into
the worker VM.

All arrows inherit the `ContextProvider` invariants (`context.go`): soft state only,
best-effort, never a work source (STORY-0018 AC-5), never affects correctness
(STORY-0018 AC-4).

## Components & interfaces

- **`RetrievalProvider`** — new interface, parallels `ContextProvider`:

  ```go
  type RetrievalProvider interface {
      Retrieve(threadID, query string, k int) ([]RankedFact, error)
  }

  type RankedFact struct {
      Fact      Fact    // reuses existing Fact{Key,Value}
      Score     float64
      SourceRef string  // originating soft-state artifact; provenance, reused by later cascade-delete
  }
  ```

- **`GraphRAGProvider`** — the adapter. Shells the `graphrag` CLI exactly as
  `LeanCtxProvider` shells `lean-ctx`: an injectable runner (`nil` → real exec) so tests
  inject a recorder, best-effort error handling (non-zero exit → `nil, nil` at call sites
  that treat it as soft state). Implements `RetrievalProvider`.

- **Base-index hook** — the soft-state write path (`WriteDiary` / `ShareKnowledge`, and the
  knowledge export in `CreateHandoff`) also base-indexes the written content into the fleet
  store. Best-effort: an index failure never fails the underlying write.

- **`consolidate` fleet task** — a new task type dispatched through laneq like any other
  work. Runs graphrag downstream synthesis over the store, incremental over content new since
  the last consolidation. Ungraded. Idempotent / graph-stable (re-running over the same input
  adds no duplicate triples).

- **Fleet store** — per-project graphrag store rooted on the shared volume.

## Error handling & degradation

| Failure | Behavior |
|---|---|
| Base-index hook fails on write | Soft-state write still succeeds; that item is unsearchable until re-indexed. Run unaffected. |
| `consolidate` task fails | Graph stays as-is (stale, not corrupt). Base hybrid retrieval still works. Ungraded → no correctness impact. |
| `Retrieve` fails / store missing | Returns empty; run proceeds with whatever other soft state it had — today's `NoopProvider` behavior. |
| Community synthesis behind | Retrieval silently falls back to base hybrid. Degradation, not error. |

**Correctness-independence invariant (mirrors STORY-0018 AC-4):** with retrieval and
consolidation fully disabled, a run grades identically. This is a required test, not just a
property.

## Testing

- **Unit** — inject a fake `graphrag` runner (same pattern as `leanCtxRunner`): `Retrieve`
  parses ranked output into `[]RankedFact`, returns `nil` best-effort on non-zero exit,
  preserves `SourceRef` provenance.
- **Consolidation** — `consolidate` is idempotent: running twice over the same soft state is
  graph-stable (no duplicate triples); an incremental run processes only new content.
- **Scenario** —
  - soft state in → base retrieval returns it ranked *before* synthesis runs;
  - after `consolidate`, a relation-dependent query returns a fact reachable only via
    community expansion;
  - correctness-independence: retrieval + consolidation disabled → identical grade.

## Interface assumptions to verify before implementation

The exact `graphrag` CLI surface for a **named, non-personal store** is not yet confirmed and
must be pinned before coding (do not guess):

1. Create / target a named store separate from the personal brain/conversations stores.
2. Ingest / base-index arbitrary content into that store (the write-hook path).
3. Query a specific store — `ask --store <name>` or equivalent — and a parseable ranked
   output format (score + source, for `RankedFact`).
4. Drive / await downstream community synthesis as a discrete step (the `consolidate` task's
   command + flags), and whether it is incremental or full re-synthesis per invocation.

If any of these is unavailable in the current `graphrag` CLI, that gap becomes a prerequisite
task (graphrag-side) before this slice can land.

## Build sequence (for the implementation plan)

1. `RetrievalProvider` + `RankedFact` types; `NoopRetrievalProvider` (drops everything) as the
   default, proving correctness-independence from day one.
2. Confirm the graphrag CLI surface (assumptions above); pin commands.
3. `GraphRAGProvider.Retrieve` over a pre-populated store + fake-runner unit tests.
4. Base-index write hook on the soft-state path.
5. `consolidate` fleet task (laneq) + idempotency/incremental tests.
6. Wire retrieval into work-claim → materialize top-k into task context; end-to-end scenario
   + correctness-independence scenario.
