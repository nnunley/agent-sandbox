#!/usr/bin/env bash
# fleet-dogfood — dispatch one task to an ephemeral worker, grade it authoritatively.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"; source "$HERE/lib.sh"

DF_NAME="" DF_BRIEF="" DF_REPO="" DF_REF="HEAD" DF_ORACLE="" DF_MODEL="claude-sonnet-4-6"
DF_GOLDEN="" DF_OUTDIR="" DF_TIMEOUT="2400"
usage() {
  cat <<'EOF'
fleet-dogfood --name ID --brief FILE --repo PATH --oracle PATH [options]
  --name ID         dispatch identifier (worker name + output dir)
  --brief FILE      task description -> worker's claude -p prompt
  --repo PATH       code the worker checks out
  --ref REF         git ref (default HEAD)
  --oracle PATH     hidden test(s); authoritative grade on a clean checkout
  --model M         worker model (default claude-sonnet-4-6)
  --golden SNAP     golden snapshot to clone (else fresh launch)
  --output-dir DIR  where worker.diff, events.jsonl, grade.json land
  --timeout SECS    worker wall-clock (default 2400)
EOF
}
while [ $# -gt 0 ]; do case "$1" in
  --name) DF_NAME="$2"; shift 2;; --brief) DF_BRIEF="$2"; shift 2;;
  --repo) DF_REPO="$2"; shift 2;; --ref) DF_REF="$2"; shift 2;;
  --oracle) DF_ORACLE="$2"; shift 2;; --model) DF_MODEL="$2"; shift 2;;
  --golden) DF_GOLDEN="$2"; shift 2;; --output-dir) DF_OUTDIR="$2"; shift 2;;
  --timeout) DF_TIMEOUT="$2"; shift 2;;
  --help|-h) usage; exit 0;; *) die "unknown arg: $1";;
esac; done
[ -n "$DF_NAME" ]   || die "--name is required"
[ -n "$DF_BRIEF" ]  || die "--brief is required"
[ -n "$DF_REPO" ]   || die "--repo is required"
[ -n "$DF_ORACLE" ] || die "--oracle is required"
[ -n "$DF_OUTDIR" ] || DF_OUTDIR="./dogfood-out/$DF_NAME"
mkdir -p "$DF_OUTDIR"
# (spin-up / deliver / run / harvest / grade / teardown added in later tasks)
log "args ok: name=$DF_NAME repo=$DF_REPO ref=$DF_REF oracle=$DF_ORACLE outdir=$DF_OUTDIR"
