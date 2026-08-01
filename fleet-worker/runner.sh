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

# STORY-0070 AC-1: canonical runner with two tree modes.
#   --fresh    (default) reset working tree + clean untracked → independent task
#   --continue           preserve the already-applied diff   → iterative debugging
# Both modes set PATH, run lean-ctx setup+serve, harvest worker.diff + result.json,
# and write lean-ctx gain output — only the pre-run tree handling differs.

# parse_mode echoes the selected mode (fresh|continue) from the arg list.
parse_mode() {
  local m=fresh
  for a in "$@"; do
    case "$a" in
      --fresh)    m=fresh ;;
      --continue) m=continue ;;
    esac
  done
  printf '%s' "$m"
}

# prepare_worktree readies REPO for the run per mode. fresh = clean baseline
# (reset --hard + clean -fdq, the playbook's between-relaunch reset); continue =
# leave the applied diff in place so the worker resumes from where it left off.
prepare_worktree() {
  local mode="$1" repo="$2"
  git config --global --add safe.directory "$repo" 2>/dev/null || true
  case "$mode" in
    fresh)
      git -C "$repo" reset --hard >/dev/null 2>&1
      git -C "$repo" clean -fdq    >/dev/null 2>&1
      ;;
    continue)
      : # preserve applied diff — no reset/clean
      ;;
    *)
      echo "prepare_worktree: unknown mode '$mode'" >&2
      return 2
      ;;
  esac
}

# Library-only mode: let tests source the functions above without running a worker.
# `return` works when sourced (the intended path); fall back to `exit` if this
# guard is ever hit in an executed context so bash doesn't error on a bare return.
[ "${RUNNER_LIB_ONLY:-}" = 1 ] && { return 0 2>/dev/null || exit 0; }

# Positional WALL_CLOCK may appear with or without a mode flag.
MODE="$(parse_mode "$@")"
WALL_CLOCK=2400
for a in "$@"; do case "$a" in --fresh|--continue) ;; *) WALL_CLOCK="$a" ;; esac; done
MAX_TURNS=200
REPO="$HOME/let-go"
cd "$REPO" || { echo "no repo at $REPO" >&2; exit 1; }

# Implementer selection (STORY-0077: quota-free ollama-local floor). FLEET_PROVIDER routes the
# IMPLEMENTER; unset/anthropic keeps the existing Max-OAuth path byte-for-byte. FLEET_MODEL overrides
# the model (default per provider). The grader/oracle stays deterministic (no model) either way.
PROVIDER="${FLEET_PROVIDER:-anthropic}"
case "$PROVIDER" in
  ollama-local) MODEL_ARG="${FLEET_MODEL:-ornith:35b}" ;;
  *)            MODEL_ARG="${FLEET_MODEL:-claude-sonnet-4-6}" ;;
esac

# Prepare the working tree per the selected mode (STORY-0070 AC-1).
prepare_worktree "$MODE" "$REPO"

: > "$HOME/events.jsonl"
echo "=== worker start mode=${MODE} wall=${WALL_CLOCK}s provider=${PROVIDER} model=${MODEL_ARG} $(date -u +%FT%TZ) ===" > "$HOME/worker.log"
echo "claude: $(command -v claude || echo MISSING)  go: $(go version 2>/dev/null)" >> "$HOME/worker.log"

# Implementer routing (STORY-0077): ollama-local runs `serf` (native Ollama, no token, no ccr);
# anthropic runs claude on the Max OAuth Bearer via the lean-ctx compression proxy. Each branch
# sets up AND runs its own agent as its LAST command, so `rc=$?` after the case captures it.
# File edits land in the working tree either way; the diff is harvested below.
case "$PROVIDER" in
  ollama-local)
    # serf speaks Ollama natively. The worker is NOT on the proxy bridge, so serf reaches Ollama
    # DIRECTLY — FLEET_OLLAMA_HOST is the Ollama server host:port. No Max token, no ccr, and NO
    # lean-ctx init (its AGENTS.md/LEAN-CTX.md would pollute the harvested diff). serf writes edits
    # to the working tree with --sandbox off (the disposable container IS the sandbox).
    : "${FLEET_OLLAMA_HOST:?ollama-local requires FLEET_OLLAMA_HOST (e.g. 192.168.86.49:11434)}"
    # Use the explicit full-URL form (OLLAMA_BASE_URL, "…/v1"): serf's bare host:port handling
    # emits a scheme-less URL that fails to parse ("first path segment cannot contain colon").
    export OLLAMA_BASE_URL="http://${FLEET_OLLAMA_HOST}/v1"
    echo "serf: $(command -v serf || echo MISSING)  OLLAMA_BASE_URL=$OLLAMA_BASE_URL  model=ollama/${MODEL_ARG}" >> "$HOME/worker.log"
    timeout --signal=INT "$WALL_CLOCK" \
      serf --model "ollama/${MODEL_ARG}" \
           --dir "$REPO" --sandbox off --max-rounds "$MAX_TURNS" --verbose \
           "$(cat "$HOME/brief.txt")" \
      > "$HOME/serf.out" 2> "$HOME/events.jsonl"
    ;;
  *)
    # anthropic (default): claude on the Max OAuth Bearer via the lean-ctx compression proxy
    # :4444 → api.anthropic.com (chain proven 2026-06-20). lean-ctx ctx_* tools + proxy live here.
    export CLAUDE_CODE_OAUTH_TOKEN="$(cat "$HOME/.fleet-token")"
    if command -v lean-ctx >/dev/null 2>&1; then
      lean-ctx init --agent claude >> "$HOME/worker.log" 2>&1 || true
      lean-ctx setup               >> "$HOME/worker.log" 2>&1 || true
      lean-ctx serve --daemon      >> "$HOME/worker.log" 2>&1 || true   # AC-2: bridge daemon + ctx_* tools
      lean-ctx proxy enable        >> "$HOME/worker.log" 2>&1 || true
      setsid nohup lean-ctx proxy start --port 4444 > "$HOME/lean-ctx-proxy.out" 2>&1 < /dev/null &
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
    fi
    timeout --signal=INT "$WALL_CLOCK" \
      claude --model "$MODEL_ARG" \
             --dangerously-skip-permissions --max-turns "$MAX_TURNS" \
             --output-format stream-json --verbose -p "$(cat "$HOME/brief.txt")" \
      > "$HOME/events.jsonl" 2>> "$HOME/worker.log"
    ;;
esac
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
