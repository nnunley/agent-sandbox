#!/usr/bin/env bash
# CI test for STORY-0017 AC-2 / STORY-0075: the worker NixOS config is a SINGLE
# DECLARATIVE SOURCE and the patterns that make a worker actually run (validated
# end-to-end on ndn-desktop 2026-06-18/19 — see runner.sh:3, ITER-0000 log) are
# CAPTURED in the nix files, not left to imperative dogfood scripts. This pins them
# against silent drift so a golden COPY replicates a working worker.
#
# Pure structural assertion (grep) — no nix, no cluster, no network; runs on the Mac.
# It does NOT re-run the worker (that is the cluster/e2e seam, dated above); it proves
# the declarative source still expresses every required pattern.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FLAKE="$HERE/../flake.nix"
CONTAINER="$HERE/../worker-container.nix"
fails=0
note() { printf '%s\n' "$*"; }
have() { # have <file> <regex> <description>
  if grep -Eq "$2" "$1"; then note "ok: $3"; else note "FAIL: $3 (missing in $(basename "$1"))"; fails=$((fails+1)); fi
}

[ -f "$FLAKE" ]     || { note "FAIL: flake.nix missing — single source gone"; exit 1; }
[ -f "$CONTAINER" ] || { note "FAIL: worker-container.nix missing — single source gone"; exit 1; }

# --- flake.nix: WHAT the worker needs (the toolchain devShell, substituted from cache) ---
have "$FLAKE" 'devShells\.\$\{system\}\.default[[:space:]]*=' "flake exports the default devShell"
have "$FLAKE" 'agents\.claude-code'                            "devShell includes claude-code (headless agent)"
have "$FLAKE" 'agents\.lean-ctx'                               "devShell includes lean-ctx (context layer)"
have "$FLAKE" 'goPkg|go_1_26'                                  "devShell includes the Go toolchain"
have "$FLAKE" 'pkgs\.gnumake'                                  "devShell includes gnumake"
have "$FLAKE" 'pkgs\.git'                                      "devShell includes git"
have "$FLAKE" 'cache\.numtide\.com'                            "flake trusts the numtide agent-CLI cache"
have "$FLAKE" 'llm-agents'                                     "flake pins the llm-agents input"

# --- worker-container.nix: HOW the worker is set up (non-root, substituters, sandbox) ---
have "$CONTAINER" 'users\.users\.worker'                       "declares the non-root worker user"
have "$CONTAINER" 'isNormalUser[[:space:]]*=[[:space:]]*true' "worker user is non-root (claude-code refuses root)"
have "$CONTAINER" 'trusted-users.*worker'                      "worker is a trusted nix user (honors flake caches)"
have "$CONTAINER" 'sandbox[[:space:]]*=[[:space:]]*false'     "build sandbox disabled (unprivileged LXC)"
have "$CONTAINER" 'flakes'                                     "flakes enabled system-wide"
have "$CONTAINER" 'environment\.sessionVariables\.NIX_PATH'    "NIX_PATH set declaratively for non-login shells"

# Local-first substitution: file:///srv/nix-shared must precede the network caches so the
# worker resolves the toolchain offline (Mac-off ready). Assert ordering, not mere presence.
subline="$(grep -E '^\s*substituters\s*=' "$CONTAINER" | head -1)"
if printf '%s' "$subline" | grep -Eq 'file:///srv/nix-shared'; then
  local_pos=$(awk -v s="$subline" 'BEGIN{print index(s,"file:///srv/nix-shared")}')
  net_pos=$(awk -v s="$subline" 'BEGIN{print index(s,"cache.nixos.org")}')
  if [ "$local_pos" -gt 0 ] && { [ "$net_pos" -eq 0 ] || [ "$local_pos" -lt "$net_pos" ]; }; then
    note "ok: local cache (file:///srv/nix-shared) is listed first (offline-first)"
  else
    note "FAIL: local cache is not first in substituters (offline-first broken)"; fails=$((fails+1))
  fi
else
  note "FAIL: worker substituters do not include the local file:///srv/nix-shared cache"; fails=$((fails+1))
fi

if [ "$fails" -eq 0 ]; then
  note "PASS: single declarative source captures all required worker patterns"
  exit 0
fi
note "FAILED: $fails pattern(s) missing from the single declarative source"
exit 1
