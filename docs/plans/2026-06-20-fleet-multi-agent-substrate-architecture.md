# Fleet as a Multi-Agent Substrate — Architecture Note

**Date:** 2026-06-20
**Status:** Vision / roadmap synthesis. One buildable-now sub-project (SP1) has its own spec
(graphrag-rs `docs/2026-06-20-enrichment-engine-design.md`); SP4/SP5 are gated on spikes.
**Scope:** how context compression, shared caching, GraphRAG, lightweight dispatch, fleet
fan-out, context handoff, and inter-agent messaging compose into one system.

## Motivation (the forcing function)

Local agent fan-out via in-process subagents does not fit a 16 GB workstation. Forensics from
2026-06-20: three macOS **Jetsam (OOM) events** (23:19, 11:50, 12:20), peak footprint ~30 GB.
At the 12:20 event the top consumer was a **single Claude Code session at 4.46 GB** (a heavy,
long-lived orchestration session — accumulated context + giant tool outputs), compounded by
version-sprawled duplicate sessions, `graphrag-mcp` (~1 GB), the Headroom proxy (~1 GB), ~1.8 GB
of Ghostty scrollback, and a browser. The repeated `mcpbridge` crashes were Jetsam *victims*,
not the cause.

Lesson: **fan-out and large context must move off the coordinator box.** The fleet
(incus-dispatcher on `ndn-desktop`) already exists to run isolated agent tasks remotely — it is
the natural substrate. The pieces to make that cheap and general are mostly already on the
roadmap; this note records how they fit.

## Target architecture

> The local machine is a **thin coordinator**. Agents are **disposable microVMs** launched from
> a **warm golden image**, that **carry context via lean-ctx `ctx_handoff` bundles**, **share
> knowledge via the graphrag knowledge graph**, **communicate over a durable message bus**, with
> **token/KV caching** keeping it cheap — all **audited via the D6 decision log**.

**lean-ctx is the connective tissue**: it is simultaneously the *compression* layer (token
reduction), the *context-transport* format (handoff bundles), and the *cacheable-unit* format.

## Dispatch policy — three workloads, three destinations

| Workload | Examples | Destination | Rationale |
|---|---|---|---|
| Heavy / independent agent tasks | TDD implementation, PAR review of a large diff, deep multi-file analysis, long research | **Fleet (container now → microVM)** | Minutes-long, isolatable; returns a compact artifact (diff/grade/report); memory off-box |
| High-volume tiny LLM calls | graphrag entity extraction, community summarization | **Cheap-model concurrency via proxy** (no container) | Sub-second; a container per call is absurd |
| Short, context-coupled lookups | quick locate/explore needing live state | **Inline (local), used sparingly** — and shrinking (see SP4) | Latency-sensitive, tiny result |

Once microVM ms-spin-up + `ctx_handoff` are proven (see gates), the third row collapses toward
the fleet too: **fleet by default; inline only for genuine human-in-the-loop steering.**

## The caching/compression design tension (SP3)

Shared caching and token reduction are two literatures that **fight each other** and must be
co-designed:
- **Shared caching** = KV/prefix caching (Prompt Cache, RadixAttention/SGLang, vLLM automatic
  prefix caching) + semantic caching (GPTCache and successors). Rewards a **byte-stable prefix**.
- **Token reduction** = prompt compression (LLMLingua family) + lossless tool-output compression
  (lean-ctx / Headroom / RTK / sqz). **Mutates** content → busts the prefix cache.

Resolution: structure every prompt as **`[stable cacheable prefix] + [compressed volatile
suffix]`** — keep the prefix (system/tooling/spec, community summaries) byte-identical to
maximize shared KV-cache hits; compress/dedup only the volatile tail. Community summaries and
`ctx_handoff` bundles are themselves stable, reusable, cacheable units.

Key references: Prompt Cache (arXiv 2311.04934, MLSys'24), SGLang/RadixAttention (2312.07104),
"Don't Break the Cache" (agentic prompt-caching eval), LLMLingua/-2/Long- (microsoft/LLMLingua),
Asteria (cross-region agentic tool caching). Tool-output compressors (lean-ctx/Headroom/sqz/RTK)
are engineering, not research; the only portable trick is the ~13-token reference for a repeated
read.

## Sub-projects

| SP | What | Status | Gate |
|---|---|---|---|
| **SP1** | GraphRAG enrichment engine: provider-pluggable cheap-model dispatch + concurrent extraction + automatic community summarization (graphrag-rs) | **Buildable now — spec written, approved** | none |
| **SP2** | Context-layer integration: fleet workers query the KG for compact graph-ranked context instead of raw file reads | Pending | SP1 |
| **SP3** | Caching architecture: stable-prefix + compressed-suffix + KV/prefix caching; community summaries / handoff bundles as cache units | Buildable, mostly independent | none |
| **SP4** | Fleet/microVM as the **default fan-out substrate** (the dispatch policy above) | Pending | **STORY-0025** (ms spin-up) + **STORY-0034** (ctx_handoff round-trip) |
| **SP5** | Inter-agent **durable message bus**: recursive delegation + mid-run steering over the queue | Pending | **ITER-0006** (laneq substrate) |

## Mapping to the existing roadmap (not new invention)

- **microVM fan-out (SP4):** STORY-0025 (spin-up benchmark spike), ITER-0005 (micro-VM backend),
  STORY-0075 (warm golden image). Firecracker is ~125 ms cold boot; snapshot-restore targets
  single-digit ms — to be *measured* by STORY-0025 on real hardware (SCENARIO-0008/0009). Today
  the fleet is still Incus containers (golden copy + start); ms-class numbers are the target,
  not yet a fact.
- **Context handoff:** `Directive.HandoffIn` ("optional lean-ctx bundle"), STORY-0052 AC-10
  (`ctx_handoff` import on worker), STORY-0034 (round-trip spike), ITER-0004 (continuity build-out).
  A handoff bundle carries *externalized working context* (working-set, ctx_agent diary,
  decisions, repo state) — a compact briefing, **not** the coordinator's full LLM transcript.
  This is also the fix for the orchestrator-context OOM: it forces compact externalized state.
- **Inter-agent messaging (SP5):** "durable message-queue-first recursive delegation" —
  STORY-0014 (delegation via message emission), SCENARIO-0019, STORY-0073 / SCENARIO-0064
  (mid-run steering), worker-authored child directives (STORY-0049 AC-4). A message ≈ a
  generalized directive on the bus; **reuse the existing claim/lease/requeue/park semantics**
  for addressing + at-least-once delivery + durability. Needs the cluster-resident laneq
  (ITER-0006, post-Patrick) — the in-memory stub cannot carry cross-process messaging.

## Governance (required once fan-out is a graph)

Free cross-agent messaging + recursive delegation reintroduces the runaway-cost/OOM risk,
distributed. Guardrails already in the design must gate it: **delegation depth + budget limits**
(SCENARIO-0022), the **escalation ladder**, and the **D6 decision log** for audit/replay of who
delegated/messaged whom. Messaging rides on these, never bypasses them.

## Two flavors of messaging (don't conflate)

1. **Durable, addressable, persisted** (delegation, steering, results, escalations) → the laneq
   bus; survives Mac-off. This is ~90% of the need and matches the existing model.
2. **Low-latency streaming** (partial-progress streaming, tight back-and-forth) → a lighter
   channel/pub-sub. Keep YAGNI until a concrete use case forces it.

## Immediate next steps

1. **SP1** → implementation plan (the only unblocked build).
2. **SP4/SP5** stay gated; the two spikes (STORY-0025 spin-up, STORY-0034 handoff round-trip)
   are the de-risking prerequisites — run them before committing to the substrate design.
3. **SP3** (caching) can proceed in parallel with SP1 if desired; otherwise sequence after SP2.
