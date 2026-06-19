# Shared helpers for the fleet-dogfood skill. Source, don't execute.
set -euo pipefail
INCUS="${INCUS:-rtk proxy incus}"
log() { printf '[fleet-dogfood] %s\n' "$*" >&2; }
die() { printf '[fleet-dogfood] ERROR: %s\n' "$*" >&2; exit 1; }
incus_x() { $INCUS "$@"; }

_TEARDOWN_TARGET=""
register_teardown() { _TEARDOWN_TARGET="$1"; trap do_teardown EXIT; }
do_teardown() {
  [ -n "$_TEARDOWN_TARGET" ] || return 0
  local c="$_TEARDOWN_TARGET"; _TEARDOWN_TARGET=""
  log "teardown: stop-then-delete $c"
  $INCUS stop "$c" --timeout 30 --force >/dev/null 2>&1 || true
  $INCUS delete "$c" --force >/dev/null 2>&1 || true
}
