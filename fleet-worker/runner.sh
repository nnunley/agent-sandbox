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
