# Reserve-Cap Enforcement at the Proxy — Design

**Status:** design (brainstormed + approved 2026-06-27). Sub-project 2 of the
[budget-aware worklist orchestrator](2026-06-26-budget-aware-worklist-orchestrator-design.md).
Sub-project 1 (the usage meter & remaining-budget estimator) is built and shipped; this
consumes it.

## Problem

The meter measures and estimates per-window provider usage, but nothing yet *enforces* a
budget. We are on Claude Code Max, whose per-window ceiling is opaque and shared between
interactive work and any fleet/escalation spend brokered through the `llm-proxy`. Without a
cap, a burst of fleet calls can consume the window and leave no headroom for the interactive
session. We need the proxy — the single chokepoint all brokered provider spend flows through —
to **reserve interactive headroom by bounding fleet spend**, deferring fleet calls that would
cross the cap. Measurement-only (sub-project 1) becomes measurement + enforcement here.

## Goal & success criteria

- The `llm-proxy` enforces a per-provider, per-window **reserve cap** on **fleet** traffic:
  once estimated fleet usage in the active window would cross `cap = ceiling × (1 − reservePct)`,
  further fleet calls are **refused without being forwarded upstream** (HTTP 429 + `Retry-After`
  + a `budget-deferred` marker). The fleet daemon treats this like any call failure and requeues
  the directive durably for a later window.
- **Interactive traffic is never capped.** When interactive Claude Code is routed through the
  proxy (optional), its calls always forward; the proxy meters them but never defers them.
- The proxy **records calibration**: on an upstream `429`/rate-limit it appends a `LimitEvent`
  to the ledger at the current window usage — wiring the 429-recording that sub-project 1
  explicitly deferred to "alongside enforcement."
- Both modules build, vet, and test green under `-race`; the security-sensitive proxy's
  dependency graph stays minimal.

## Non-goals (deferred to later sub-projects or explicitly out of v1)

- The producer, supervisor, tiered strong-model PAR panel, queue draining (sub-projects 3–5).
- Per-request hard serialization to fully close the cap TOCTOU (see Error handling). v1 is a
  soft cap; the reserve absorbs the slack.
- Non-Anthropic streaming-usage extraction beyond a documented stub. OpenAI/ollama
  non-streaming responses are parsed; their streaming usage shapes are a later refinement.
- A numeric confidence band on the readout (sub-project 1 carried this as polish; unchanged).

## Decisions locked during brainstorming

- **Two request classes by route prefix.** Fleet uses the existing routes (`/anthropic`,
  `/openai`, …). Interactive uses new parallel prefixes (`/interactive/anthropic`, …). Point
  Claude Code's `ANTHROPIC_BASE_URL` at the interactive prefix to route it through the proxy.
  **Fleet-only mode** = simply don't configure interactive routes (today's reality).
- **Enforcement is a pre-flight gate**, evaluated before the upstream call. It reads only the
  ledger's already-accumulated window usage, so it is **fully streaming-agnostic** — it never
  needs the current response.
- **Refuse fast; caller requeues.** The proxy stays stateless about deferral: it rejects the
  over-cap fleet call and the fleet daemon's existing requeue path handles durability.
- **Cap = ceiling × (1 − reservePct)**, per provider per 5h window. `reservePct` is configured
  (a default plus optional per-provider override).
- **Uncalibrated → enforce against the configured prior.** Use the meter's `PublishedPrior` as
  the working ceiling (low confidence) and cap from day one; the cap auto-sharpens once a real
  429 calibrates the true ceiling. **Fail open only if no prior is configured.**
- **Promote `usage` to its own stdlib-only module.** Move `modules/incus-dispatcher/usage` →
  `modules/usage` with its own `go.mod` (no third-party deps). Both `incus-dispatcher` and
  `llm-proxy` depend on it via a `replace` directive, so the proxy pulls only the tiny meter
  module — not incus-dispatcher's gRPC/protobuf/temporal/grantauth graph. This retroactively
  unblocks the sub-project-1 Task 9 proxy-wiring deferral.

## Architecture

```
                 ┌─────────────────────── llm-proxy ───────────────────────┐
 fleet worker ──►│ /anthropic        (class=fleet)       ┌── budgetGate ──┐ │
   (microVM)     │   pre-flight gate ──── over cap? ──────┤ Estimate(now)  │ │──► refuse 429
                 │        │ allow                          │ cap=ceil×(1−p) │ │    (+ marker,
 Claude Code  ──►│ /interactive/anthropic (class=interactive, never capped) │    no upstream)
 (optional)      │        ▼ forward upstream                                │
                 │   response ──► parse usage ──► append UsageEvent(Source) │
                 │   upstream 429 ──► append LimitEvent (calibration)       │
                 └──────────────────────────┬──────────────────────────────┘
                                             ▼
                                    modules/usage  (shared, stdlib-only)
                                    Ledger (fsync JSONL) · Estimator · ReserveCap
```

## Components

### `modules/usage` (promoted standalone module)

- The existing meter, moved verbatim: `event`, `ledger`, `window`, `estimator`,
  `collector_claude`, `collector_proxy` (`ParseAnthropicUsage`), `ingest`, `report`. Import path
  becomes `github.com/agent-sandbox/usage` (final path confirmed at plan time against the repo's
  module-naming convention).
- **New, pure (clock-injected, no I/O):** a reserve-cap calculation over an `Estimate`:
  - `FleetCap(reservePct float64) int64` on `Estimate` → `ceiling × (1 − reservePct)`, `0` when
    `CeilingEst == 0` (no working ceiling).
  - `AllowFleet(reservePct float64) bool` → `CeilingEst == 0` ⇒ fail open (true); else
    `Used < FleetCap(reservePct)`. This is exactly the locked "prior → enforce, no prior →
    fail open" rule: the `Estimator` already folds a configured `PublishedPrior` *into*
    `CeilingEst` (at low confidence), so a present prior yields a non-zero ceiling and enforces,
    while a truly uncalibrated provider with no prior leaves `CeilingEst == 0` and fails open.
  - `reservePct` is validated to `[0,1)` by the caller; the pure function assumes a valid input
    and is total over it.

### `llm-proxy` additions

- **Route classification.** `route` gains a `class` field (`fleet` | `interactive`). Interactive
  routes are registered from the same spec list with the `/interactive` prefix and
  `class=interactive`; existing routes are `class=fleet`. Classification is purely the route the
  request arrived on — no client cooperation, no ambiguous default.
- **`budgetGate`** — splits a pure decision from its I/O:
  - I/O layer: open the shared `usage.Ledger` (path from `FLEET_USAGE_LEDGER`, mirroring the
    sub-project-1 default), snapshot `Events()`/`Limits()`, build the per-provider `Estimate` at
    `time.Now()` (the clock is read only here, at the edge).
  - Pure layer: `Estimate.AllowFleet(reservePct)`.
  - Returns allow/defer; on a ledger-read error it **fails open** and logs (never strand the
    fleet on a meter bug).
- **Response metering.** After the upstream response, parse the `usage` block and append a
  `UsageEvent` with `Source` set from the request class (`SourceInteractive` for interactive
  routes, `SourceFleet` otherwise), reusing `usage.ParseAnthropicUsage`. Interactive events carry
  the message id as `TurnID`, so they **de-dup naturally** against transcript ingest via the
  meter's existing `(Source,TurnID)` key.
- **Streaming usage extractor.** Anthropic streaming responses carry usage in SSE events
  (`message_start` → input/cache tokens; `message_delta` → cumulative output). `streamCopy` is
  extended to **tee**: it forwards bytes to the client unchanged and un-delayed while scanning
  line-buffered for the usage fields, emitting one `UsageEvent` at stream end. This unit is
  isolated and independently tested. It is **load-bearing**: fleet calls have no transcript
  fallback, so without stream metering the gate would never see fleet usage accumulate.
- **Calibration on 429.** When the upstream status is `429` (or a provider rate-limit signal),
  append a `LimitEvent{provider, window_class:"5h", used_at: currentWindowUsed, ts: now}` so the
  meter learns the realized ceiling.
- **Config (env).** `LLM_PROXY_RESERVE_PCT` (default, e.g. `0.30`) with optional per-provider
  overrides (e.g. `LLM_PROXY_RESERVE_PCT_ANTHROPIC`); prior ceilings (e.g.
  `LLM_PROXY_CEILING_PRIOR_ANTHROPIC`) feeding `Estimator.PublishedPrior`; `FLEET_USAGE_LEDGER`
  for the ledger path. Exact env names finalized at plan time.

## Data flow

1. Request arrives; classify by route (`fleet` vs `interactive`).
2. **Fleet:** the gate opens the ledger, computes the provider's `Estimate` at now, derives
   `cap = ceiling × (1 − reservePct)`. If `Used ≥ cap` → refuse `429` + `Retry-After` +
   `budget-deferred` marker, log the decision, **do not call upstream**. Else forward.
3. **Interactive:** always forward.
4. On the upstream response: parse usage (non-streaming JSON directly, or via the SSE tee for
   streaming) → append a `UsageEvent` with the class's `Source`. If the upstream status is `429`
   → also append a `LimitEvent` at the current window usage.
5. The meter sharpens; the next gate decision uses the learned ceiling.

## Error handling / honesty

- **Metering and calibration are best-effort and never block brokering.** A ledger-append
  failure is logged; the proxied call proceeds and returns normally.
- **The gate fails open on ledger-read errors.** A meter bug must not strand the fleet; an
  unreadable ledger means "allow," logged.
- **TOCTOU (known, accepted for v1).** Two concurrent near-cap fleet calls can both pass the
  gate before either records usage, so the cap is *soft* — it can be modestly overshot under
  concurrency. The reserve percentage absorbs this slack. Hard serialization is a deferred
  refinement, not a v1 requirement; documented here so it is a known limit, not a silent bug.
- **Streaming integrity is inviolable.** The SSE tee must never buffer the full body, reorder,
  or delay client bytes; if usage cannot be extracted from a given stream, that call is simply
  un-metered (logged) rather than broken.

## Testing (stdlib only, `-race`)

- **Pure cap logic:** table tests for `FleetCap` and `AllowFleet` — uncalibrated-no-prior
  (fail open), uncalibrated-with-prior (cap against prior), calibrated under/at/over cap,
  `reservePct` boundaries.
- **`budgetGate`** with an injected clock + temp ledger: fleet under cap → allow; fleet over cap
  → defer; interactive → always allow; ledger-read error → fail open.
- **Proxy integration (`httptest`):** fleet over-cap returns `429` + marker **and the upstream
  is never hit** (assert via a counting stub upstream); interactive over the same usage is still
  forwarded; a normal response appends a `UsageEvent` with the correct `Source`; an upstream
  `429` appends a `LimitEvent`.
- **SSE tee:** a streamed Anthropic-shaped response is forwarded byte-identical and without added
  latency to the client **and** yields exactly one `UsageEvent` with the right token fields.
- **Cross-module:** `modules/usage`, `modules/incus-dispatcher`, and `modules/llm-proxy` each
  build, vet, and `go test -race` green after the move + wiring.

## What v1 proves (acceptance)

Pointing a fleet worker at the proxy with a configured prior ceiling and `reservePct`, a run that
crosses the cap is refused with `429 + budget-deferred` and never reaches upstream, while an
interactive call through `/interactive/...` in the same window still succeeds — and the ledger
shows fleet usage accumulating from proxied calls plus a `LimitEvent` recorded on a real upstream
429. Interactive headroom is protected by bounding fleet spend, with honest, self-sharpening
numbers underneath.

## A→B graft

The gate, cap calculation, and metering live behind the proxy's route layer and the shared
`usage` module, so the later cluster-resident orchestrator (Approach B) reuses them unchanged;
only the deployment of the proxy + ledger location changes.
