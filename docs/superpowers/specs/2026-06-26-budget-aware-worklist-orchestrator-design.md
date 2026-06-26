# Budget-Aware Worklist Orchestrator — Design

**Date:** 2026-06-26
**Status:** design (brainstormed, approved through Section 3); sub-project 1 (the meter) is the first build.

## Problem

We want to run an autonomous loop — like the iterative-development loop — that works through a
cross-project worklist on the agent-sandbox **fleet** (cheap-first workers, hidden-oracle grading,
escalation ladder, Mac-off), while **preserving enough headroom to still have interactive Claude
Code sessions, knowledge-gathering, etc.**

The blocker to "preserve headroom" is measurement. We are on **Claude Code Max ($100/mo)**, whose
real token limits are opaque and effectively arbitrary — the published numbers don't reliably predict
when you run out. The same is true for the other providers the fleet uses (Codex, ollama-cloud, …).
So we cannot enforce a budget envelope from a configured constant; we must **measure actual reported
usage and *learn* the effective ceiling**, then estimate remaining headroom honestly.

This document specifies the umbrella architecture for context, then specifies **sub-project 1 — the
cross-provider usage meter & remaining-budget estimator** — the walking skeleton everything else trusts.

## Umbrella vision (context — NOT all built now)

A budget-aware loop that drains a cross-project queue of oracle-bearing work items through the cheap
fleet, escalating to a strong-model review panel only within a reserved budget envelope.

```
worklist/roadmap ─► PRODUCER ─► laneq queue ─► FLEET (daemon: claim→run→external-grade(hidden
(any project)        (item→Directive{repo,ref,        oracle, clean checkout)→ladder; workers
                      task, grade=hidden-oracle})      call providers via llm-proxy)
                                                              │ graded / flagged
   llm-proxy ◄──────── BUDGET METER (in proxy + claude-code): per-window reserve cap;
   (all provider                       over cap → defer
    spend flows here)                          │ escalations (durable lane)
                                               ▼
                         SUPERVISOR (A: interactive skill; B: cluster service):
                         drains escalation lane; tiered review (cheap fleet review →
                         strong-model PAR panel: claude|codex|… via proxy) within budget;
                         marks done / re-queues / surfaces to operator TUI
```

**Decisions locked during brainstorming:**
- **Offload the grind to the fleet.** The interactive Claude session is a thin planner/judge/escalation
  handler. agent-sandbox already provides the execution substrate (laneq queue, daemon, external-grading,
  escalation ladder + durable `FileEscalationLane`, operator TUI, multi-level `BudgetPolicy`, llm-proxy
  with provider routing + credential broker).
- **Generic cross-project queue** of gradable items `{repo, ref, task, hidden-oracle}`. iterative-development
  is one *producer*; let-go fixes and ad-hoc tasks are others.
- **Quality gate = hidden oracle + tiered PAR.** Authoritative grade is a held-out oracle run on a clean
  checkout (worker never sees it — the existing external-grading path, STORY-0068). A cheap fleet reviewer
  is first-pass; escalate to an expensive **strong-model PAR panel (Claude, Codex, or other strong models —
  not Claude-only)** only on flagged/high-risk items.
- **Budget envelope = reserve + per-window cap, enforced at the llm-proxy** (the single chokepoint all
  provider spend flows through). Auto-handle escalations up to a per-window cap that always leaves reserved
  interactive headroom; over the cap, escalations queue durably in the lane for the next session.
- **A now, B later.** Approach A ships the supervisor as an interactive skill over the existing fleet;
  Approach B re-hosts the same producer/supervisor logic as a cluster-resident service. Components are
  defined behind clean contracts so the graft needs no rework.
- **claude agents are part of the llm-proxy infrastructure** — Claude is one brokered provider, so fleet
  Claude usage is NOT free of the Max quota; the meter must account for it alongside interactive usage.

**Sub-projects (each its own spec → plan → build):**
1. **Usage meter & remaining-budget estimator** ← THIS SPEC. The linchpin; riskiest unknown (opaque limits);
   standalone value (a trustworthy "how much have I really got left?" readout).
2. Reserve-cap enforcement at the proxy (consumes the meter's estimate; defers over-cap calls).
3. Producer (worklist/roadmap → oracle-bearing directives).
4. Supervisor skill + tiered strong-model PAR panel + escalation-lane drain.
5. (B) Cluster-resident orchestrator service re-hosting 3+4.

## Sub-project 1: Cross-provider usage meter & remaining-budget estimator

### Goal & success criteria

A durable, per-provider usage ledger fed by **both** spend sources, plus an estimator that learns each
provider's effective per-window ceiling from observed exhaustion and emits an honest remaining estimate.

**Success:** fed real fleet (proxy) + interactive (Claude Code) usage, the readout reports, per provider,
`{used, effective_ceiling_est, remaining_est, confidence, window_reset_at}`; the estimate **calibrates
toward the real (arbitrary) ceiling after an observed exhaustion event**, and is honestly labeled
"uncalibrated / wide range" until then.

### Non-goals (deferred to later sub-projects)

- Enforcing the cap / deferring calls (sub-project 2). The meter only **measures + estimates**; it does not
  block spend.
- The producer, supervisor, tiered PAR, queue draining (sub-projects 3–5).
- Predicting limits across plan changes; modeling Anthropic's exact internal window algorithm. We learn an
  *effective* ceiling empirically; we do not reverse-engineer the official one.

### Components (Go, in agent-sandbox alongside `modules/llm-proxy`)

Four small, independently testable units sharing one `UsageEvent` type.

1. **Collectors** — emit a normalized `UsageEvent` from each spend source:
   - **Proxy collector** — in the llm-proxy response path; parses the provider `usage` block (Anthropic
     `input_tokens`/`output_tokens`/cache fields; OpenAI/Codex `usage`; ollama `prompt_eval_count`/
     `eval_count`). Emits `source:"fleet"`. The proxy already brokers every fleet call, so this is a thin
     additive hook, not a new network path.
   - **Claude-Code collector** — captures interactive usage, which bypasses the proxy. **Primary source:**
     the per-turn `usage` object in Claude Code's streaming JSON output. **Durable backfill/reconciliation:**
     the per-session transcript JSONL under `~/.claude/projects/**` (token counts are recorded there).
     Emits `source:"interactive"`, `provider:"anthropic"`. Reconciliation de-dups streaming vs transcript by
     a turn/message id so the same turn is not double-counted.
2. **Usage ledger** — append-only durable JSONL of `UsageEvent`s (mirrors the existing
   `JSONLAuditLog`/`FileEscalationLane`/`JSONLDecisionLog` pattern: append + fsync, reconstruct on open).
   Source of truth for estimation and the readout; survives restarts.
3. **Estimator** — pure function over `(ledger events, observed limit-events, now)` → per
   `(provider, window)` estimate. Logic:
   - **Window model (Claude Code Max — anchored, not fixed-clock):** the 5-hour session window is **anchored
     on first-use-after-idle**, so its start floats. The estimator derives the current window's `anchor` from
     event timestamps: the first `UsageEvent` whose ts is after the prior window's expiry opens a new window;
     `window_reset_at = anchor + 5h`. Continuous use rolls within that single anchored window; going idle past
     the expiry re-anchors a fresh window at the next event. (There is also a separate, longer weekly window —
     tracked the same way with its own anchor/length.) The "2pm reset" earlier was just where one day's anchor
     happened to land, NOT a fixed boundary.
   - Sum reported usage in the current window.
   - **Effective-ceiling learner:** start from a weak published-limit **prior**; when an exhaustion/throttle
     signal is observed (HTTP 429 / provider "limit reached" / Claude Code rate-limit notice), record the
     **cumulative window usage at the moment of exhaustion** as a calibration point for that
     `(provider, window-class)` — that figure *is* the realized effective ceiling for that window. The
     ceiling estimate is a decayed/rolling value over recent calibration points (so plan or behavior changes
     are tracked rather than frozen at the first observation), tightening `confidence` as points accumulate.
     If the window resets without an exhaustion, the highest usage reached that window is a *lower bound* on
     the ceiling (weakly raises the estimate), not a calibration point.
   - `remaining_est = max(0, ceiling_est − used)`; `confidence ∈ {uncalibrated, low, med, high}` by number of
     calibration points and `window_reset_at = anchor + window_length`.
   - **Clock injected** (codebase convention) for deterministic tests.
4. **Readout / query API** — a CLI subcommand (e.g. `dispatcher budget`) printing the per-provider estimate,
   plus a Go method `Remaining(provider) Estimate` the future reserve cap (sub-project 2) and operator TUI
   consume. **Visibility-sooner (explicit requirement):** the readout must be useful from the FIRST events,
   before any ceiling calibration exists. Even at `confidence:"uncalibrated"` it surfaces the immediately-known
   facts — `window_anchor`, `window_reset_at`, time-elapsed/remaining in the window, and cumulative `used` —
   plus a best-effort `remaining_est` shown as a wide labeled range. The time/usage facts are valuable on day
   one independent of the learned ceiling. Human line (calibrated):
   `anthropic: ~123k used this window (anchored 09:00, resets 14:00, 2h12m left) · est ~57k remaining (med)`;
   uncalibrated: `… · est remaining: uncalibrated (≥0; learns the ceiling after the first limit-hit)`.

### Data flow

spend happens → collector parses reported usage → `UsageEvent` appended to the ledger (+ limit-events
appended on exhaustion) → estimator recomputes per provider on read → readout/API exposes the estimate.

### Error handling / honesty

- Provider returns no `usage` block → fall back to a local token count of the request/response, emit the
  event flagged `estimated:true` / low-confidence; never silently drop or zero.
- Cold start (no calibration point yet) → `confidence:"uncalibrated"`, `remaining_est` presented as a wide
  range, explicitly labeled — never a false-precise number.
- Streaming vs transcript double-count → de-dup by turn/message id; if ids are missing, prefer transcript
  (durable) and log the gap.
- Window detection / reset → the estimator derives the current window's `anchor` from event timestamps (first
  event after the prior window's expiry re-anchors); the injected clock decides whether the anchored window is
  still open. A new window zeroes the window-usage sum but **retains** the learned ceiling across windows. An
  idle gap that crosses the expiry must produce a fresh anchor, not extend the old window.
- Garbled/partial usage line in the ledger on reopen → skipped with a logged warning, not fatal.

### Testing

- **Estimator (unit, pure):** replay synthetic `UsageEvent`s + a forced exhaustion event; assert the ceiling
  estimate calibrates to the realized usage and `remaining_est`/`confidence` update correctly; assert a new
  window zeroes usage but keeps the ceiling. **Anchored-window tests:** continuous events stay in one window;
  an idle gap crossing the 5h expiry re-anchors a fresh window at the next event (vs. a sub-expiry gap that
  does not); assert `window_anchor`/`window_reset_at` are derived correctly (injected clock). **Visibility-
  sooner test:** with zero calibration points, the readout still returns correct `window_anchor`,
  `window_reset_at`, time-remaining, and cumulative `used`, with `confidence:"uncalibrated"`.
- **Collectors (unit):** feed captured sample provider responses (Anthropic/OpenAI/ollama `usage` shapes) and
  a sample Claude Code streaming-JSON turn + a sample transcript JSONL; assert correct `UsageEvent`s and
  streaming↔transcript de-dup.
- **Ledger (unit):** append + reopen reconstruction, fsync durability, corrupt-line skip (reuse the
  FileEscalationLane test pattern).
- **Readout (integration):** end-to-end — drive a few fleet + interactive events through real collectors →
  ledger → estimator → CLI output; assert the printed per-provider remaining estimate matches the replayed
  ledger. No proof-by-injection: the readout reads the real ledger the collectors wrote.

### What v1 proves (walking-skeleton acceptance)

A trustworthy, durable, per-provider remaining-budget readout fed by real fleet + interactive usage that
calibrates toward the real arbitrary ceiling after observed exhaustion — the measurement foundation the
reserve cap (sub-project 2) and the whole orchestrator depend on. No enforcement yet; just honest numbers.

### A→B graft

The meter is provider- and consumer-agnostic: collectors emit `UsageEvent`s, the ledger is durable, the
estimator is pure, and the readout is a query API. The interactive skill (A) and the cluster service (B)
both consume the same `Remaining(provider)` contract — no rework when the supervisor moves cluster-side.
