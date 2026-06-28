# `dispatcher bench` — Fleet Model-Quality Evaluator — Design

**Status:** design (brainstormed + approved 2026-06-28). Feeds `pickImplementer` in the
fleet-native iterative-development skill ([design](2026-06-28-iterative-development-fleet-design.md)).
Reuses the `fleet-dogfood` primitive + the dispatcher's external grader.

## Problem

Fleet routing (`pickImplementer`) and the PAR panel need to know which models are actually strong
enough to use as implementers/reviewers — e.g. "is `ornith:35b` stronger than `qwen3.6`?" — but we
have only hunches. We need a reproducible evaluator that ranks candidate models (local ollama pool +
codex) by capability on fleet-shaped tasks, with cost visibility. Models and tooling evolve, so the
task suite must evolve too: a static target is wrong long-term.

## Goal & success criteria

- A new `dispatcher bench` subcommand evaluates `candidates × suite`: for each pair it dispatches the
  task to an ephemeral worker running that model/provider (reusing the fleet-dogfood path), harvests
  the diff, and **oracle-grades on a clean checkout**. It aggregates a **scorecard** (ranked table +
  machine-readable JSON).
- Primary metric: **oracle pass-rate** (deterministic). Optional **LLM-judge** pass adds quality
  dimensions as a tie-break among passing models.
- The suite is **versioned and extensible**; every scorecard records the suite's name + version +
  content-hash so pass-rates are only ever compared within an identical suite version.
- Per-candidate **token cost is reported per provider** (from the usage meter) so "strong but
  expensive" is visible.
- Acceptance run compares the local pool + codex + `ornith:35b` on a v1 curated suite.

## Decisions locked during brainstorming

- **Form:** a first-class `dispatcher bench` Go subcommand (not a skill/script) — durable,
  scriptable, emits structured JSON.
- **Scoring:** oracle pass-rate primary + optional LLM-judge for quality tie-breaks. The judge model
  is configurable and defaults to a **strong** provider (judging with a weak local model is flagged
  unreliable and is not the default).
- **Evolving suite:** versioned manifest directories; additive growth bumps the version; external
  benchmark suites (HumanEval/SWE-bench-style) integrate via a thin adapter mapping their tasks into
  the `{brief, repo/ref, hidden-oracle}` shape. Scorecards are comparable only within a suite version.
- **Candidates:** the installed local ollama pool + codex, plus `ornith:35b` (pulled first).

## Architecture

```
dispatcher bench --suite <name@version> --candidates <list> [--judge <model>] [--out scorecard.json]
   │
   for each candidate × task:
   ├─ dispatch task to ephemeral worker (provider=candidate)   ── reuses fleet-dogfood / Runner
   ├─ harvest diff
   ├─ ORACLE external-grade on clean checkout  ── reuses runGradeCommand / --external-grading
   └─ [optional] LLM-judge the diff (quality dims)  ── via llm-proxy, judge model
   │
   aggregate ─► scorecard{ suite:{name,version,hash}, candidates:[{model, passRate, perTask[],
                 judgeScore?, wallMs, tokensByProvider}], ranking[] }  ─► table + JSON
```

## Components

- **Suite loader** — reads `suites/<name>/<version>/`; each task dir carries a brief, a repo/ref, and
  a hidden-oracle (script or holdout). Computes a content-hash. An **external-suite adapter** seam maps
  third-party eval tasks into the same task struct (seam defined in v1; concrete adapters are later).
- **Bench runner** — iterates `candidates × tasks`, dispatching each via the existing fleet path with
  `provider/model` set to the candidate; bounded concurrency; per-task timeout; isolated worker per run.
- **Oracle gate** — the existing external-grading path; authoritative pass/fail.
- **Judge (optional)** — sends the diff + task to a configurable strong judge model via the llm-proxy;
  returns a quality score; used only to break ties among passing candidates.
- **Scorecard aggregator** — emits ranked table + JSON, tagged with suite name/version/hash, per-task
  results, wall-time, and per-provider token cost pulled from the usage meter.

## Data flow / error handling

1. Load suite (fail fast on a malformed manifest; record the content-hash).
2. For each candidate × task: dispatch → harvest → oracle-grade; a worker crash / timeout = task
   `fail` for that candidate (recorded, never aborts the whole run).
3. Optional judge pass over passing diffs.
4. Aggregate + write scorecard (table to stdout, JSON to `--out`).
5. A candidate that is unreachable / not installed is recorded as `skipped` with a reason — the run
   still completes for the rest. ollama-local candidates are free/unmetered; quota-bearing candidates'
   spend is recorded and a candidate over its reserve cap is skipped (logged).

## Acceptance

`dispatcher bench --suite fleet-core@v1 --candidates <local pool>,codex,ornith:35b` produces a ranked
scorecard with oracle pass-rates + per-provider cost; rerunning on the same suite-hash yields a
comparable result; `ornith:35b` and `qwen3.6` appear directly compared.

## Out of scope (v1)

- Concrete external-benchmark adapters (define the seam + ship one curated `fleet-core@v1` suite).
- Auto-updating `pickImplementer` from scorecards (emit the scorecard; wiring routing is a follow-up).
- Judge-model bias calibration; multi-judge panels.

## Build constraints (for the implementer)

- Go, in `modules/incus-dispatcher/`; stdlib + existing internal packages only; no new third-party deps.
- Reuse `fleet-dogfood`/`Runner` dispatch and `runGradeCommand` rather than reimplementing dispatch
  or grading. New code: suite loader, bench runner loop, scorecard aggregator, the `bench` subcommand
  wiring in `main.go` (same `if os.Args[1]=="bench"` pattern as `grade`/`serve`/`tui`/`usage`).
- Tests under `-race`; `go vet ./...` clean. Commit messages must avoid the hook-blocked phrases
  (`Claude`, `Generated with`, `Co-Authored-By: Claude`, `claude.com/claude-code`).
- The suite v1 may start with a tiny set (3–5 small tasks) reusing the dogfood example task shape.
