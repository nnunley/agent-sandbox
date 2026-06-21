# fleet-worker spikes — lean-ctx bridge/proxy diagnostics & runner regression

Cluster spike + smoke harnesses for the worker's lean-ctx integration (STORY-0069). Each clones the
golden snapshot (`fleet-dogfood-base/pristine`), runs inside `nix develop`, and reaps the worker.
Run from the repo root; needs `~/.fleet-token` and incus default remote = the cluster (`ndn-desktop`).

```
bash fleet-worker/spikes/leanctx-runner-smoke.sh    # end-to-end: runs the REAL runner.sh, checks artifacts
bash fleet-worker/spikes/leanctx-chain-spike.sh     # proves Claude → lean-ctx proxy → Anthropic measures savings
bash fleet-worker/spikes/leanctx-doctor-spike.sh    # lean-ctx doctor + config/env diagnosis
```

## Proven recipe (2026-06-20) — what makes `lean-ctx gain` report Bridge ON

`gain`'s "Bridge: OFF — proxy not reachable" is **not** the serve daemon (that's healthy: `doctor`
29/29, daemon running). It needs lean-ctx's **separate compression proxy**:

1. `lean-ctx init --agent claude` + `lean-ctx setup` + `lean-ctx serve --daemon` → ctx_* MCP tools + bridge (AC-2).
2. `lean-ctx proxy enable` + `setsid nohup lean-ctx proxy start --port 4444 &` → the API proxy that
   "compresses tool_results before the LLM API" (the source of `gain`'s numbers).
3. `export ANTHROPIC_BASE_URL=http://127.0.0.1:4444` so Claude routes through the proxy.

Key findings:
- The fleet token is an **OAuth (sk-ant-oat) Bearer** token; `lean-ctx proxy enable` *declines* to
  auto-rewrite `ANTHROPIC_BASE_URL` for subscription tokens, but the proxy **forwards the Bearer
  transparently** to `api.anthropic.com` — so manually setting the base URL works (spike-proven:
  Claude rc=0, proxy Requests/Tokens-saved > 0, no "Bridge: OFF").
- Start the proxy with **`setsid nohup`** — a plain `&`/subshell didn't survive headless.
- Gate on a **curl healthcheck** (`401` = up, token-auth), not `proxy status` text (too slow mid-startup).
- **Fail-open**: only set `ANTHROPIC_BASE_URL` once the proxy answers; else Claude runs direct so the
  dogfood loop never breaks. (All wired into `fleet-worker/runner.sh`.)
- savings scale with tool_result size (trivial `echo` → 0; large `cat` → measurable). Authoritative
  measure = `lean-ctx proxy status` (Requests/Compressed/Tokens saved); `gain` is the dashboard.
- Dogfood chain = worker → lean-ctx proxy → Anthropic. The keyless worker → lean-ctx → **fleet
  llm-proxy** → Anthropic leg is the ITER-0005 micro-VM path (broker injects the key).

## Files
- `leanctx-runner-smoke.sh` — spins a worker, delivers a repo+brief, runs the real `runner.sh`,
  inspects new artifacts (result.json fallback, proxy status savings, gain Bridge state). **Reusable
  regression** for runner lean-ctx changes. Pass `--env IS_SANDBOX=1` is handled internally.
- `leanctx-chain-spike.sh` + `leanctx-chain-probe.sh` — minimal proof of the Claude→proxy→Anthropic
  chain with a real (small) Claude call; prints proxy status + gain.
- `leanctx-doctor-spike.sh` + `leanctx-proxy-probe.sh` — `lean-ctx doctor`/`proxy` interface diagnosis.
