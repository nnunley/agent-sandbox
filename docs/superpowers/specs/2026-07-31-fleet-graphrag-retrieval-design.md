# Fleet Retrieval + Consolidation over graphrag (slice 1)

**Status:** design approved in brainstorming (2026-07-31). **Revised 2026-08-03** — reconciled
against `graphrag-rs`'s actual architecture and an empirical extraction spike (see "Revision"
below). Spec for review.

## Revision note (2026-08-03) — what changed and why

The 2026-07-31 draft designed a bespoke **graphrag gRPC service** (tonic/proto/buf) whose
synthesis was **stepwise fleet work**: the service minted a nonce per unit, produced
synthesis-step directives on **laneq**, and ephemeral workers ran each LLM step. Two discoveries
supersede that:

1. **graphrag-rs already is the tool.** It implements chunking, embeddings, hierarchical Leiden,
   HNSW, BM25, RRF hybrid fusion, SQLite persistence, a CLI, a **14-tool MCP server**, LLM
   entity/relation extraction, and an **`enrich` orchestrator** (extract → Leiden → summarize).
   There is no bespoke gRPC service to build, no proto to freeze, no nonce/laneq-producer
   machinery. The fleet **drives graphrag-rs**, it does not reimplement it.
2. **Synthesis is in-process, not container-dispatched.** `enrichment-engine-design.md:62`:
   *"Concurrency is graphrag-rs's own bounded tokio semaphore — **not** container/fleet dispatch
   (a container per sub-second call is the wrong tool)."* So synthesis = one `graphrag enrich`
   process making concurrent cheap-model calls through the **llm-proxy**, idempotent, resume-by-
   rerun. The nonce/stepwise/laneq-producer design is **dropped**.

laneq + container dispatch survive **only** for the coding-*worker* tier (running actual coding
tasks), which is a separate concern and **not on graphrag's critical path**.

**Empirically grounded (2026-08-03 spike, see "Extraction config"):** local models that *fail as
serf coding agents* (qwen3.6-hermes, gemma4:31b) do **structured extraction flawlessly**. The
synthesis layer is therefore viable on the cheap local floor, decoupled from the coding-agent
problem.

## Origin

Critical read of the MOMENTUM paper (jeanlabs.ai): it bundles retrieval + assembly + synthesis
into one ADK monolith with marketing-domain glue; this repo already has better, separated
primitives (graphrag-rs, lean-ctx, laneq). The one transferable idea is the *composition* — a
fleet run's opening context assembled and **relevance-ranked** from accumulated soft state. This
spec is that composition, over the fleet's own soft state only.

## Goals

1. A fleet run's opening context is **retrieved by relevance to its task brief** from accumulated
   fleet soft state, instead of the current all-facts-for-a-thread dump (`ReceiveKnowledge`
   returns everything, unranked).
2. The knowledge-graph **synthesis** (entity/relation extraction + Leiden communities + community
   summaries) is produced by **`graphrag enrich`** running concurrent cheap-model calls through
   the llm-proxy — in-process, idempotent, resume-by-rerun. Not fleet-dispatched.
3. The whole subsystem is **soft state**: best-effort, never a work source for real tasks, never
   on the diff/grade path. Disabling it entirely must not change any run's grade.

## Non-goals (slice 1)

- Context-window token budgeting / multi-source merge (top-k is enough) — **slice 2**.
- Explicit "project soul" surfacing — falls out of community summaries; **slice 3** if needed.
- Multi-tenant / cross-thread store scoping — single per-project store for now.
- Retrieving from the operator's personal graphrag (brain/conversations) stores — privacy line.
- The coding-*worker* tier (serf/lace on local models). Separate track; not on this path.

## Key premises (load-bearing)

- **Base index is total and immediate; only downstream synthesis lags.** Chunk + embed (HNSW) +
  BM25 covers *all* soft state as it lands (cheap, non-LLM), so **hybrid base retrieval (RRF)
  works right away**. The graph/community layer is produced by `enrich` and may lag.
- **Synthesis is `enrich`, in-process.** Concurrent bounded (`--concurrency`) `complete()` calls
  through the llm-proxy → a cheap model; idempotent (`extracted_at` column + non-empty
  `summary`); a mid-run failure is recovered by re-running. No nonce/laneq/stepwise machinery.
- **graphrag-rs owns the store** (it is the tool; single writer by construction). The fleet feeds
  it soft state and drives `enrich`/`ask`; it never hand-writes store files.
- **Corpus = fleet soft state only** — diary, curated knowledge, handoff artifacts (the existing
  `ContextProvider` outputs).
- **Placement.** The fleet store + `enrich`/`ask` invocations run where they can reach the
  llm-proxy and the store: coordinator-side, or one long-lived job — *not* a container per call.

## Architecture

A per-project **fleet graphrag-rs store**. Three flows, all driven through graphrag-rs (CLI
`--json` / its MCP tools — structured, no text scraping):

```
run completes                                                    (coordinator)
  └─ ContextProvider.WriteDiary / ShareKnowledge / CreateHandoff (exists)
        └─ ingest hook → graphrag store (note/add_document)       (chunk+embed+BM25, immediate, best-effort, no LLM)

synthesis (periodic / triggered; in-process, NOT fleet-dispatched)
  └─ graphrag enrich --extract --summarize --provider-url <llm-proxy /v1> --model <cheap>
        ├─ extract   chunks → concurrent complete() → entity/relation triples   (validated config below)
        ├─ Leiden    graph → hierarchical communities
        └─ summarize communities leaf→root → concurrent complete() → summaries   (sandwiched)
     idempotent (extracted_at + non-empty summary); resume-by-rerun

next run claims work                                             (coordinator)
  └─ RetrievalProvider.Retrieve(threadID, brief, k) → graphrag ask --json (or MCP query)
        ├─ base hybrid (RRF)      → always available
        └─ + community expansion  → when enrich has run
        └─ []RankedFact { Text, Score, SourceRef, Via }
  └─ materialize top-k into the task's context                   (handoff bundle / context file on shared volume — existing transport)
  └─ worker launches with that context in its workspace
```

The **query** is the task brief (story/scenario/`Cmd` on `Task`). **Materialization** reuses the
existing handoff / shared-volume path — no new worker-VM transport. All of it inherits the
`ContextProvider` invariants (`context.go`): soft state only, best-effort, never a real-work
source (STORY-0018 AC-5), never affects correctness (STORY-0018 AC-4).

## Extraction config (empirically validated — 2026-08-03 spike)

Factorial spike `{JSON, triple-lines} × {single-block, sandwich} × {nemotron3:33b,
qwen3.6-hermes, gemma4:31b}`, 8 chunks/arm, `think:false` + temp 0, mechanical metrics + a
deepseek-v4-pro correctness judge (`scratchpad/extract-experiment/{harness,judge}.py`). Results:

- **Single-block, no sandwich, for extraction.** The closing "bread" is neutral-to-harmful on
  short chunks — worst cell in the sweep was nemotron3+JSON+sandwich (precision 0.83→**0.44**,
  38% malformed). The sandwich's rationale (recency after a *long* span) does not apply to
  context-constrained extraction chunks. **Keep the sandwich only for summarization** (long
  community payloads — leaf→root).
- **Output format is model-specific** (the correctness judge overturned "triples always wins"
  that mechanical yield suggested):
  - `qwen3.6-hermes` → **triple-lines** (`head (Type) -[relation]-> tail (Type)`): prec 0.82,
    **4.6 correct triples/chunk** — best cell overall.
  - `gemma4:31b` → **triple-lines** (0.81, 3.9/chunk).
  - `nemotron3:33b` → **JSON** (0.83, 3.4) — its triple-line output is noisier (prec 0.60).
- `think:false` (13–22× speedup) + temp 0. Precision ceiling ~0.80–0.83 (~1 in 5 imperfect) —
  absorbed by graphrag-rs's existing **reflection/continuation passes** (max 2) plus later dedup
  and community summarization.
- **Best default pairing: `qwen3.6-hermes` + triple-lines** (highest correctness-weighted yield,
  ~1.2s/call). Notably qwen3.6-hermes *fails* as a serf agent yet is the *best* extractor.

### graphrag-rs changes this implies (extraction prompt-builder)

- Make the extraction **output format a knob** (`json` | `triples`) resolved **per model** —
  default `triples`, `json` for nemotron-class. graphrag-rs already parses `EntityTriple`
  (`head, head_type, relation, tail, tail_type`) and already has an arrow string form
  (`format_triple`); extend it with types as the wire format and add a line parser (skip+count
  malformed lines — degrades gracefully, unlike a broken JSON blob that loses the whole chunk).
- Make the **sandwich a per-stage flag**: **off for extract**, **on for summarize**.
- Keep `think:false` + temp 0 on the local path; mirror `required` fields in the prompt body
  (Ollama drops schema `required` enforcement under `think:false`).

## Components & interfaces (fleet side)

- **`RetrievalProvider`** — new interface, parallels `ContextProvider`:
  ```go
  type RetrievalProvider interface {
      Retrieve(threadID, query string, k int) ([]RankedFact, error)
      Ingest(sourceRef string, content []byte) error
  }
  type RankedFact struct {
      Text      string  // ranked passage/fact
      Score     float64
      SourceRef string  // provenance: originating soft-state artifact
      Via       string  // "base" | "community" — retrieval path (degradation signal)
  }
  ```
- **`GraphRAGProvider`** — the adapter. Backs onto **graphrag-rs** via its structured surface
  (`graphrag ask --json` / the MCP query tools) — an injectable runner (fake in tests),
  best-effort (error → `nil,nil`). Structured output, no text scraping. Implements
  `RetrievalProvider`. (This replaces the dropped bespoke-gRPC-client design.)
- **`NoopRetrievalProvider`** — default; drops all reads/writes. Guarantees correctness-
  independence from day one.
- **Ingest hook** — the soft-state write path (`WriteDiary`/`ShareKnowledge`, `CreateHandoff`
  knowledge export) also feeds the fleet store. Best-effort; a failure never fails the write.
- **`enrich` job** — invokes `graphrag enrich --extract --summarize --provider-url <proxy>
  --model <cheap>` periodically/triggered. In-process concurrency; ungraded; idempotent.
- **Materialization** — top-k `RankedFact`s → a context file on the shared volume + a `Task.Env`
  pointer, reusing the handoff transport.

## Error handling & degradation

| Failure | Behavior |
|---|---|
| Ingest fails on write | Soft-state write still succeeds; item unsearchable until re-ingested. Run unaffected. |
| `enrich` run fails partway | Idempotent: re-run resumes (`extracted_at`/`summary` skip). Base retrieval still works meanwhile. Ungraded → no correctness impact. |
| Malformed model output (a triple line / JSON) | Skip + count (line-level for triples — partial extraction preserved); reflection/continuation passes; still bad → skip. |
| `ask` fails / store missing | `Retrieve` returns empty; run proceeds — today's `NoopProvider` behavior. |
| Community synthesis behind | `ask` falls back to base hybrid; results carry `Via:"base"`. Degradation, not error. |

**Correctness-independence invariant (mirrors STORY-0018 AC-4):** with retrieval + synthesis
disabled (`NoopRetrievalProvider`), a run grades identically. Required test.

## Testing

- **Unit (fleet side)** — fake graphrag runner: `Retrieve` maps `ask --json` output into
  `[]RankedFact`, `nil` best-effort on error, preserves `SourceRef`/`Via`.
- **Base-before-synthesis** — content ingested but not yet enriched is retrievable via
  `Via:"base"`; after `enrich`, a relation-dependent query returns it via `Via:"community"`.
- **Idempotent enrich** — re-running `enrich` over an already-enriched store is a fast no-op (no
  duplicate entities/relations/summaries).
- **Correctness-independence scenario** — provider disabled → identical grade.
- **Extraction config** (graphrag-rs side) — prompt-builder emits single-block for extract /
  sandwiched for summarize; per-model format resolution; triple-line parser skips malformed lines.

## Build sequence (for the implementation plan)

1. `RetrievalProvider` + `RankedFact` + `NoopRetrievalProvider` (default; proves correctness-
   independence).
2. `GraphRAGProvider.Retrieve` over `graphrag ask --json` (or MCP) + fake-runner unit tests.
3. Ingest hook on the soft-state write path → fleet graphrag store.
4. `enrich` job wiring: invoke `graphrag enrich … --provider-url <proxy> --model <cheap>`;
   idempotency test.
5. graphrag-rs extraction prompt-builder changes (per-model format, per-stage sandwich, triple-
   line parser) — tracked in graphrag-rs's roadmap; meets the fleet at the `enrich` CLI/config.
6. Wire retrieval into work-claim → materialize top-k into task context; end-to-end + correctness-
   independence scenarios.

## Superseded (kept for provenance)

The 2026-07-31 design's **gRPC service + `.proto` contract + nonce + laneq-producer + synthesis-
step-worker** are **dropped** (see Revision note): graphrag-rs is the tool, and synthesis is
in-process `enrich`, not container-dispatched stepwise work. The `RankedFact` shape and the
retrieval read-path/materialization survive; the transport is graphrag-rs's structured CLI/MCP,
not a bespoke service.
