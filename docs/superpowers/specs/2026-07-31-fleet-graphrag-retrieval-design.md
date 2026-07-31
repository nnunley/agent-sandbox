# Fleet Retrieval + Consolidation over graphrag (slice 1)

**Status:** design approved in brainstorming (2026-07-31); spec for review.

graphrag is a first-party tool, so its fleet-facing surface is **co-designed here as a frozen
contract** (see "Fleet ↔ graphrag contract"), implemented graphrag-side as a tracked item in
graphrag's own roadmap. **Transport is gRPC** (the repo already depends on
`google.golang.org/grpc` and laneq is fully proto/gRPC via `buf` — `queue/laneqpb/`). graphrag
runs as a **gRPC service** (Rust/`tonic` server, generated Go client here). This repo builds
against the proto contract, not against CLI text output.

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
   **fleet work**: the service decomposes synthesis into stepwise units and enqueues them on
   laneq; ephemeral workers execute the LLM steps through the llm-proxy. The service is only
   the orchestrator and sole store-writer.
3. The whole subsystem is **soft state**: best-effort, never a work source for real tasks,
   never on the diff/grade path. Disabling it entirely must not change any run's grade.

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
  retrieval (RRF) always works right away**. The graph/community layer is built by async
  stepwise fleet work and may be incomplete at any moment.
- **Corpus = fleet soft state only.** Diary, curated knowledge, handoff artifacts — the
  outputs of the existing `ContextProvider` path.
- **The service is the single store-writer.** graphrag owns the store on the shared volume;
  nothing else writes it. Workers never touch store files — they return synthesis results to
  the service by nonce, and the service merges. This removes any file-lock/concurrency race.
- **Placement.** The graphrag service runs alongside laneq and the llm-proxy (agent-host):
  the coordinator reaches it for `Ask`/`Ingest`; workers reach it (over the bridge, like the
  proxy) to submit synthesis steps. The store lives on the shared volume it owns.

## Architecture

A per-project **fleet graphrag store**, owned by the graphrag gRPC service. Three flows:

```
run completes                                                       (coordinator)
  └─ ContextProvider.WriteDiary / ShareKnowledge / CreateHandoff    (exists)
        └─ base-index hook → graphrag.Ingest(RPC)                   (vector+BM25, immediate, best-effort, no LLM)

synthesis = stepwise fleet work                                     (service orchestrates; workers compute)
  service detects un-synthesized content
    └─ decompose into stepwise units (per-chunk triples, per-community summary)
    └─ for each unit: mint NONCE, enqueue a synthesis-step directive on laneq   (service is a laneq PRODUCER, paseto grant)
  worker claims a synthesis step (laneq) ── like any other task
    └─ run LLM extraction via llm-proxy → result
    └─ graphrag.SubmitSynthesisStep(nonce, result) (RPC)
  service validates nonce → MERGES into store (single writer) → marks step done
    └─ re-submitting a nonce is a no-op → graph-stable / idempotent BY CONSTRUCTION

next run claims work                                                (coordinator)
  └─ GraphRAGProvider.Retrieve(threadID, brief, k) → graphrag.Ask(RPC)
        ├─ base hybrid (RRF)          → always available
        └─ + community expansion      → when synthesis steps have landed
        └─ []RankedFact { Text, Score, SourceRef, Via }
  └─ materialize top-k into the task's context                      (handoff bundle / context file on shared volume — existing transport)
  └─ worker launches with that context in its workspace
```

The **query** is the task brief (the story/scenario/`Cmd` already on `Task`). The
**materialization** reuses the existing handoff / shared-volume path — no new transport into
the worker VM. There is **no monolithic `consolidate` task**: synthesis is many small nonce'd
steps, each an ordinary laneq item.

All of this inherits the `ContextProvider` invariants (`context.go`): soft state only,
best-effort, never a real-work source (STORY-0018 AC-5), never affects correctness
(STORY-0018 AC-4). Synthesis steps are ungraded fleet work.

## The nonce

Minted by the service, one per stepwise synthesis unit. It is simultaneously:

- **Claim/correlation token** — carried on the laneq directive; ties a queued step to the
  worker result submitted back via `SubmitSynthesisStep(nonce, …)`.
- **Idempotency key** — the service dedupes merges by nonce, so a step delivered twice (retry,
  at-least-once queue semantics) merges once. This is where graph-stability comes from — not
  from re-running a whole synthesis pass.
- **Liveness handle** — an unclaimed/failed step's nonce simply never returns; the service can
  re-mint and re-enqueue on a timer. A dropped step means that unit is unsynthesized (base
  retrieval still covers its content), never a corrupt graph.

## Components & interfaces

- **`RetrievalProvider`** — new interface, parallels `ContextProvider`:

  ```go
  type RetrievalProvider interface {
      Retrieve(threadID, query string, k int) ([]RankedFact, error)
  }

  type RankedFact struct {
      Text      string  // retrieved passage/fact text
      Score     float64
      SourceRef string  // provenance: originating soft-state artifact; reused by later cascade-delete
      Via       string  // "base" | "community" — which retrieval path produced it (degradation signal)
  }
  ```

  `RankedFact` carries retrieved text rather than wrapping `Fact{Key,Value}`: graphrag results
  are ranked passages. `Via` lets the fleet observe whether community expansion contributed,
  i.e. whether synthesis has caught up.

- **`GraphRAGProvider`** — the adapter, a **gRPC client** of the graphrag service. Injectable
  client (a fake in tests), best-effort error handling (RPC error → `nil, nil` at call sites
  that treat it as soft state). Implements `RetrievalProvider`. No CLI, no text scraping.

- **Base-index hook** — the soft-state write path (`WriteDiary` / `ShareKnowledge`, and the
  knowledge export in `CreateHandoff`) also calls `graphrag.Ingest`. Best-effort: an ingest
  failure never fails the underlying write.

- **graphrag service** (first-party, graphrag repo) — owns the store; serves `Ingest`/`Ask`;
  orchestrates synthesis by minting nonces and **producing** synthesis-step items on laneq;
  accepts `SubmitSynthesisStep` and is the sole store-writer. Authenticates to laneq as a
  producer via a paseto grant (see `2026-06-24-laneq-grant-paseto-design.md`).

- **Synthesis-step worker** — an ephemeral fleet worker that claims a synthesis-step directive,
  runs the LLM unit via the llm-proxy, and submits the result back by nonce. Ungraded.

- **Fleet store** — per-project graphrag store on the shared volume, owned by the service.

## Error handling & degradation

| Failure | Behavior |
|---|---|
| `Ingest` RPC fails on write | Soft-state write still succeeds; that item is unsearchable until re-ingested. Run unaffected. |
| Synthesis step never claimed / worker dies | Nonce never returns; service re-mints on a timer. That unit stays unsynthesized; base hybrid still covers its content. |
| `SubmitSynthesisStep` delivered twice | Nonce-dedupe → merged once. No duplicate triples. |
| `Ask` RPC fails / service down | `Retrieve` returns empty; run proceeds with whatever other soft state it had — today's `NoopProvider` behavior. |
| Community synthesis behind | `Ask` silently falls back to base hybrid; results carry `Via:"base"`. Degradation, not error. |

**Correctness-independence invariant (mirrors STORY-0018 AC-4):** with retrieval and synthesis
fully disabled (service unreachable, `NoopRetrievalProvider`), a run grades identically. A
required test, not just a property.

## Testing

- **Unit (fleet side)** — fake graphrag gRPC client: `Retrieve` maps `Ask` responses into
  `[]RankedFact`, returns `nil` best-effort on RPC error, preserves `SourceRef`/`Via`.
- **Nonce idempotency** — submitting the same nonce twice merges once (this is the
  graph-stability guarantee; test it directly on the service's merge, and fleet-side that a
  retried step submission is safe).
- **Base-before-synthesis** — content ingested but not yet synthesized is retrievable via
  `Via:"base"`; after the relevant step lands, a relation-dependent query returns it via
  `Via:"community"`.
- **Correctness-independence scenario** — service unreachable → identical grade.
- **laneq producer** — the service enqueues a well-formed synthesis-step directive under its
  grant; an ephemeral worker can claim and complete it.

## Fleet ↔ graphrag contract

Frozen here as a **proto service**; implemented graphrag-side as a tracked roadmap item. A
**named store** is addressed by `store` + `root` (a path on the shared volume), a separate
namespace from the operator's personal brain/conversations stores.

```proto
service GraphRAG {
  // Base index, immediate, no LLM. Best-effort from the caller's view.
  rpc Ingest(IngestRequest) returns (IngestResponse);

  // Ranked hybrid retrieval. Base (RRF) always eligible; community results appear as
  // synthesis steps land. Highest score first.
  rpc Ask(AskRequest) returns (AskResponse);

  // Worker → service: return one synthesis step's result, keyed by its nonce.
  // Idempotent: a repeated nonce merges once.
  rpc SubmitSynthesisStep(SynthesisStepResult) returns (SubmitAck);
}

message StoreRef { string store = 1; string root = 2; }

message IngestRequest { StoreRef store = 1; string source_ref = 2; bytes content = 3; }
message IngestResponse {}

message AskRequest { StoreRef store = 1; string query = 2; uint32 top = 3; }
message RankedResult { string text = 1; double score = 2; string source_ref = 3; string via = 4; } // via: "base"|"community"
message AskResponse { repeated RankedResult results = 1; }

message SynthesisStepResult { string nonce = 1; bytes result = 2; } // result = extracted triples / community summary
message SubmitAck { bool merged = 1; } // false = already merged (nonce seen before)
```

The **synthesis-step directive** the service enqueues on laneq carries `{ nonce, StoreRef,
unit_input_ref }` — the worker fetches the unit input (or receives it inline), runs the LLM
step, and calls `SubmitSynthesisStep`. Trigger for synthesis is internal to the service
(on ingest / on a timer); no external "synthesize" RPC is required for slice 1.

**graphrag-side prerequisite (tracked in graphrag's roadmap, not this plan):** implement the
service against this proto, including the store-owning merge, nonce-dedupe, and laneq producer
integration. This slice builds against the proto; the two efforts meet at the `.proto`.

## Build sequence (for the implementation plan)

1. `RetrievalProvider` + `RankedFact` types; `NoopRetrievalProvider` (drops everything) as the
   default, proving correctness-independence from day one.
2. Freeze the `.proto` with the graphrag roadmap owner; generate the Go client (buf, as laneq
   does).
3. `GraphRAGProvider.Retrieve` over the `Ask` RPC + fake-client unit tests.
4. Base-index hook → `Ingest` RPC on the soft-state write path.
5. Synthesis-step worker + the service's laneq-producer/nonce-merge path (cross-repo; meets at
   the proto and a laneq directive shape). Nonce-idempotency tests.
6. Wire retrieval into work-claim → materialize top-k into task context; end-to-end scenario
   + correctness-independence scenario.
