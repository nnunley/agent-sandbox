#!/usr/bin/env bash
# CI test for STORY-0005 AC-1 (unit seam) + STORY-0049 AC-5: the IMMUTABLE GOLDEN worker
# image is DEFINED declaratively (immutable root + writable /workspace,/tmp scratch) so a
# disposable copy replicates a known-good worker without a live build. Pins golden.nix's
# contract against silent drift.
#
# Pure structural assertion (grep) — no nix, no cluster, no network; runs on the Mac.
# It does NOT build the golden (that is the cluster/e2e seam: SCENARIO-0003 golden-launch);
# it proves the declarative definition still expresses every required property.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOLDEN="$HERE/../golden.nix"
fails=0
note() { printf '%s\n' "$*"; }
have() { # have <file> <regex> <description>
  if grep -Eq "$2" "$1"; then note "ok: $3"; else note "FAIL: $3 (missing in $(basename "$1"))"; fails=$((fails+1)); fi
}

[ -f "$GOLDEN" ] || { note "FAIL: golden.nix missing — golden image definition gone"; exit 1; }

# Builds ON the single declarative worker source (not a fork).
have "$GOLDEN" 'imports[[:space:]]*=.*worker-container\.nix'   "golden extends worker-container.nix (single source)"

# WRITABLE SCRATCH — only /workspace and /tmp, as tmpfs (disposable copy starts clean).
have "$GOLDEN" 'fileSystems\."/workspace"'                     "declares writable /workspace scratch"
have "$GOLDEN" 'fileSystems\."/tmp"'                           "declares writable /tmp scratch"
have "$GOLDEN" '/workspace".*|/workspace"[[:space:]]*=[[:space:]]*\{' "/workspace is a dedicated mount"
have "$GOLDEN" 'fsType[[:space:]]*=[[:space:]]*"tmpfs"'        "scratch is tmpfs (no residue across copies)"

# IMMUTABLE ROOT — read-only nix store; golden image is the immutable source.
have "$GOLDEN" 'boot\.readOnlyNixStore[[:space:]]*=[[:space:]]*true' "immutable root: read-only nix store"

# Golden marker so a launched copy is provably a fleet-golden clone (no live build, AC-2).
have "$GOLDEN" 'fleet-golden-version'                          "golden carries a version marker for copy-launch proof"

if [ "$fails" -eq 0 ]; then
  note "PASS: golden image definition (STORY-0005 AC-1 / STORY-0049 AC-5)"
  exit 0
fi
note "FAILED: $fails assertion(s)"
exit 1
