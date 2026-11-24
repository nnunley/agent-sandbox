#!/usr/bin/env bash
# Local test: the anthropic implementer reaches the model only through llm-proxy and holds no
# credential. Sources runner.sh in library-only mode; no cluster, no nix, no network.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNNER="$HERE/../runner.sh"
fails=0
note() { printf '%s\n' "$*"; }
check() { if [ "$1" != "$2" ]; then note "FAIL: $3 (want '$2' got '$1')"; fails=$((fails+1)); else note "ok: $3"; fi; }

RUNNER_LIB_ONLY=1 source "$RUNNER" || { note "FAIL: could not source runner in lib-only mode"; exit 1; }
type claude_proxy_env >/dev/null 2>&1 || { note "FAIL: claude_proxy_env not defined"; exit 1; }

# A token left in the environment by the launcher must not survive into claude's environment.
export CLAUDE_CODE_OAUTH_TOKEN="leaked-token"
claude_proxy_env "agent-host.incus:12071"
check "$?" "0" "claude_proxy_env succeeds with an address"
check "$ANTHROPIC_BASE_URL" "http://agent-host.incus:12071/anthropic" "claude calls llm-proxy's fleet route"
check "$ANTHROPIC_AUTH_TOKEN" "llm-proxy" "claude holds only the placeholder credential"
check "${CLAUDE_CODE_OAUTH_TOKEN-unset}" "unset" "no OAuth token in claude's environment"
check "${ANTHROPIC_API_KEY-unset}" "unset" "no API key in claude's environment"
check "$LEAN_CTX_ANTHROPIC_UPSTREAM" "http://agent-host.incus:12071/anthropic" "lean-ctx forwards to llm-proxy"
check "$LEAN_CTX_ALLOW_INSECURE_HTTP_UPSTREAM" "1" "lean-ctx may use the plain-HTTP upstream"

( claude_proxy_env "" ) >/dev/null 2>&1
check "$?" "1" "claude_proxy_env refuses an empty address"

check "$(grep -c 'fleet-token' "$RUNNER")" "0" "runner reads no token file"

[ "$fails" -eq 0 ] && note "=== all runner proxy-env checks passed ===" || { note "=== $fails failure(s) ==="; exit 1; }
