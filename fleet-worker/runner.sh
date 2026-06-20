#!/usr/bin/env bash
# Fleet worker runner (playbook-shaped). Invoke INSIDE the flake env so claude/go/make resolve.
# VERIFIED 2026-06-18 in an unprivileged stock NixOS container — both flags are REQUIRED:
#   --accept-flake-config : trust cache.numtide.com so claude-code/lean-ctx SUBSTITUTE (no build)
#   --no-sandbox          : build the tiny residual mkShell env without kernel-namespace sandbox
#                           (an unprivileged container can't provide it — plan #27.2)
#   nix develop /home/worker/fleet-worker --accept-flake-config --no-sandbox \
#     --command bash /home/worker/runner.sh [WALL_CLOCK]
# Streams one JSON event per line -> events.jsonl; harvests worker.diff. Worker runs NON-ROOT.
set -uo pipefail
WALL_CLOCK="${1:-2400}"; MAX_TURNS=200
REPO="$HOME/let-go"
cd "$REPO" || { echo "no repo at $REPO" >&2; exit 1; }

export CLAUDE_CODE_OAUTH_TOKEN="$(cat "$HOME/.fleet-token")"

# Clean baseline (playbook: reset working tree between relaunches).
git config --global --add safe.directory "$REPO" 2>/dev/null || true
git reset --hard >/dev/null 2>&1; git clean -fdq >/dev/null 2>&1

: > "$HOME/events.jsonl"
echo "=== worker start wall=${WALL_CLOCK}s sonnet $(date -u +%FT%TZ) ===" > "$HOME/worker.log"
echo "claude: $(command -v claude || echo MISSING)  go: $(go version 2>/dev/null)" >> "$HOME/worker.log"

# STORY-0069: lean-ctx FULL enablement — ctx_* MCP tools (cached reads + shell compression) AND
# the compression proxy (measured savings). FAIL-OPEN: any lean-ctx hiccup must not break the run;
# we only route claude through the proxy once it is confirmed up, else fall back to the direct API.
# (Chain proven 2026-06-20: claude OAuth Bearer → lean-ctx proxy :4444 → api.anthropic.com; the
# keyless worker→lean-ctx→fleet-llm-proxy leg is the ITER-0005 micro-VM path.)
if command -v lean-ctx >/dev/null 2>&1; then
  lean-ctx init --agent claude >> "$HOME/worker.log" 2>&1 || true
  lean-ctx setup               >> "$HOME/worker.log" 2>&1 || true
  lean-ctx serve --daemon      >> "$HOME/worker.log" 2>&1 || true   # AC-2: bridge daemon + ctx_* tools
  lean-ctx proxy enable        >> "$HOME/worker.log" 2>&1 || true
  setsid nohup lean-ctx proxy start --port 4444 > "$HOME/lean-ctx-proxy.out" 2>&1 < /dev/null &
  # Gate on the port actually accepting connections (curl proven reliable in the spike: 401 = up,
  # token-auth). `proxy status` text was too slow/unreliable mid-startup. Fail-open if curl absent (000).
  for _i in $(seq 1 15); do
    code=$(curl -sS -o /dev/null -m 2 -w '%{http_code}' http://127.0.0.1:4444/ 2>/dev/null || echo 000)
    case "$code" in
      200|401|404|405)
        export ANTHROPIC_BASE_URL="http://127.0.0.1:4444"   # route claude through the proxy (keep OAuth Bearer)
        echo "lean-ctx proxy ON (http $code) → ANTHROPIC_BASE_URL=$ANTHROPIC_BASE_URL" >> "$HOME/worker.log"
        break;;
    esac
    sleep 1
  done
  [ -n "${ANTHROPIC_BASE_URL:-}" ] || echo "lean-ctx proxy not up; claude runs direct (fail-open)" >> "$HOME/worker.log"
else
  echo "lean-ctx not found; running without compression" >> "$HOME/worker.log"
fi

timeout --signal=INT "$WALL_CLOCK" \
  claude --model claude-sonnet-4-6 \
         --dangerously-skip-permissions --max-turns "$MAX_TURNS" \
         --output-format stream-json --verbose -p "$(cat "$HOME/brief.txt")" \
  > "$HOME/events.jsonl" 2>> "$HOME/worker.log"
rc=$?
[ "$rc" = 124 ] && echo "WALL-CLOCK TIMEOUT" >> "$HOME/worker.log"
echo "worker done rc=$rc $(date -u +%FT%TZ)" >> "$HOME/worker.log"

# Harvest: source diff for review (worker is told NOT to commit).
# add -A -N (intent-to-add) so NEW files appear in the diff; --no-ext-diff in case a
# difftastic/external-diff config ever leaks in (the grade needs a real unified patch).
git -C "$REPO" add -A -N >/dev/null 2>&1 || true
git -C "$REPO" diff --no-ext-diff > "$HOME/worker.diff" 2>/dev/null
echo "diff bytes: $(wc -c < "$HOME/worker.diff")" >> "$HOME/worker.log"

# STORY-0072 AC-1: always leave a structured result.json. The worker is expected to write its own;
# if it ran out of turns/context before doing so, synthesize a fallback so the orchestrator always
# has structured output and can defer to the authoritative external grader (anti-reward-hack, AC-2).
if [ ! -s "$HOME/result.json" ]; then
  printf '{"status":"UNKNOWN","rc":%s,"harvested_diff_path":"%s"}\n' "$rc" "$HOME/worker.diff" > "$HOME/result.json"
  echo "synthesized fallback result.json (worker wrote none)" >> "$HOME/worker.log"
fi

# STORY-0069: capture measured savings. proxy status is authoritative (Requests/Compressed/Tokens
# saved); gain is the dashboard view. Best-effort — never fail the run on these.
if command -v lean-ctx >/dev/null 2>&1; then
  lean-ctx proxy status > "$HOME/lean-ctx-proxy-status.txt" 2>&1 || true
  lean-ctx gain         > "$HOME/lean-ctx-gain.txt"         2>&1 || true
fi
