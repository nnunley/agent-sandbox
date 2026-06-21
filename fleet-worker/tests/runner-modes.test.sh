#!/usr/bin/env bash
# Local CI test for STORY-0070 AC-1: runner --fresh vs --continue tree handling.
# Sources runner.sh in library-only mode (RUNNER_LIB_ONLY=1) so prepare_worktree
# can be exercised against a throwaway git repo WITHOUT launching claude or a
# container. No cluster, no nix, no network — runnable on the Mac.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNNER="$HERE/../runner.sh"
fails=0
note() { printf '%s\n' "$*"; }
check() { if [ "$1" != "$2" ]; then note "FAIL: $3 (want '$2' got '$1')"; fails=$((fails+1)); else note "ok: $3"; fi; }

# Load the runner's functions without running the worker flow.
RUNNER_LIB_ONLY=1 source "$RUNNER" || { note "FAIL: could not source runner in lib-only mode"; exit 1; }
type prepare_worktree >/dev/null 2>&1 || { note "FAIL: prepare_worktree not defined"; exit 1; }
type parse_mode      >/dev/null 2>&1 || { note "FAIL: parse_mode not defined";      exit 1; }

# parse_mode: default + explicit flags.
check "$(parse_mode)"            "fresh"    "default mode is fresh"
check "$(parse_mode --fresh)"    "fresh"    "--fresh selects fresh"
check "$(parse_mode --continue)" "continue" "--continue selects continue"

make_repo() {
  local d; d="$(mktemp -d)"
  git -C "$d" init -q
  git -C "$d" -c user.email=t@t -c user.name=t commit -q --allow-empty -m init
  printf 'tracked\n' > "$d/tracked.txt"; git -C "$d" add tracked.txt
  git -C "$d" -c user.email=t@t -c user.name=t commit -q -m base
  # simulate an applied worker change: modify tracked + add untracked
  printf 'tracked CHANGED\n' > "$d/tracked.txt"
  printf 'new\n' > "$d/untracked.txt"
  printf '%s' "$d"
}

# --fresh wipes the applied change back to a clean tree.
R1="$(make_repo)"
prepare_worktree fresh "$R1"
check "$(git -C "$R1" status --porcelain | wc -l | tr -d ' ')" "0" "fresh: clean tree"
check "$(cat "$R1/tracked.txt")" "tracked" "fresh: tracked file reset"
check "$([ -e "$R1/untracked.txt" ] && echo yes || echo no)" "no" "fresh: untracked removed"
rm -rf "$R1"

# --continue preserves the applied diff (both modified + untracked).
R2="$(make_repo)"
prepare_worktree continue "$R2"
check "$(cat "$R2/tracked.txt")" "tracked CHANGED" "continue: tracked change preserved"
check "$([ -e "$R2/untracked.txt" ] && echo yes || echo no)" "yes" "continue: untracked preserved"
rm -rf "$R2"

if [ "$fails" -ne 0 ]; then note "=== $fails check(s) FAILED ==="; exit 1; fi
note "=== all runner-mode checks passed ==="
