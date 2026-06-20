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
ROOT_DIR="$(cd "$HERE/../../.." && pwd)"
WORKER="df-$DF_NAME"
MODE="$(cat "$HERE/.mode" 2>/dev/null || echo fresh)"
[ -n "${FLEET_TOKEN:-}" ] || die "FLEET_TOKEN env not set (worker needs CLAUDE_CODE_OAUTH_TOKEN)"
log "dispatch $WORKER (mode=$MODE) repo=$DF_REPO ref=$DF_REF -> $DF_OUTDIR"

$INCUS delete "$WORKER" --force >/dev/null 2>&1 || true
register_teardown "$WORKER"

# --- spin up: golden clone (btrfs reflink) or fresh launch + rebuild ---
if [ -n "$DF_GOLDEN" ] || [ "${MODE#golden:}" != "$MODE" ]; then
  SNAP="${DF_GOLDEN:-${MODE#golden:}}"
  log "spin up from golden $SNAP (reflink clone)"
  $INCUS copy "$SNAP" "$WORKER" >/dev/null
  $INCUS config device add "$WORKER" nix-shared disk pool=default source=nix-shared path=/srv/nix-shared readonly=true >/dev/null 2>&1 || true
  $INCUS start "$WORKER" >/dev/null
else
  log "spin up fresh (launch + worker-container.nix rebuild)"
  $INCUS launch images:nixos/25.11 "$WORKER" >/dev/null
  for i in $(seq 1 20); do st=$($INCUS exec "$WORKER" -- systemctl is-system-running 2>/dev/null || true); case "$st" in running|degraded) break;; esac; done
  $INCUS file push "$ROOT_DIR/fleet-worker/worker-container.nix" "$WORKER/etc/nixos/configuration.nix"
  $INCUS config device add "$WORKER" nix-shared disk pool=default source=nix-shared path=/srv/nix-shared readonly=true >/dev/null 2>&1 || true
  $INCUS exec "$WORKER" -- bash -lc 'export NIX_CONFIG="sandbox = false"; nix-channel --update && nixos-rebuild switch' >/dev/null
fi
for i in $(seq 1 20); do st=$($INCUS exec "$WORKER" -- systemctl is-system-running 2>/dev/null || true); case "$st" in running|degraded) break;; esac; done

# Wait for the nix-daemon socket to actually accept connections. systemd reporting
# "degraded" can precede nix-daemon.socket being up, so concurrent golden clones race here
# and `nix develop` dies with "cannot connect to socket … Connection refused". Nudge the
# socket each iteration and confirm with a real client ping before proceeding.
for i in $(seq 1 30); do
  if $INCUS exec "$WORKER" -- bash -lc 'nix store ping --store daemon >/dev/null 2>&1'; then break; fi
  $INCUS exec "$WORKER" -- systemctl start nix-daemon.socket nix-daemon.service >/dev/null 2>&1 || true
  sleep 1
done

# --- deliver repo@ref + brief + token + fleet-worker flake ---
log "deliver repo@$DF_REF + brief + token"
# Deliver + run AS ROOT in /root. The disposable container IS the sandbox, so claude runs
# as root via IS_SANDBOX=1 (verified) — no non-root worker, no uid/gid, no ownership dance:
# root owns/writes everything (repo, flake.lock, claude's edits). Much simpler than the
# non-root path. (Flake inputs/fetchGit only yield READ-ONLY store paths — see SKILL.md —
# so the editable work-tree is still a real clone, just root-owned now.)
( cd "$DF_REPO" && git bundle create "/tmp/df-$DF_NAME.bundle" "$DF_REF" >/dev/null 2>&1 )
$INCUS file push "/tmp/df-$DF_NAME.bundle" "$WORKER/root/repo.bundle"
$INCUS file push -r "$ROOT_DIR/fleet-worker" "$WORKER/root/" >/dev/null
$INCUS file push "$DF_BRIEF" "$WORKER/root/brief.txt"
printf '%s' "$FLEET_TOKEN" | $INCUS exec "$WORKER" -- tee /root/.fleet-token >/dev/null
$INCUS exec "$WORKER" -- bash -lc 'rm -rf /root/let-go && git clone -q /root/repo.bundle /root/let-go' >/dev/null

# --- run the worker as root (runner.sh inside the flake env; IS_SANDBOX=1 lets claude run
# as root; toolchain from the local cache) ---
log "run worker (nix develop … runner.sh as root, timeout=${DF_TIMEOUT}s)"
$INCUS exec "$WORKER" --env HOME=/root --env IS_SANDBOX=1 -- bash -lc \
  "nix develop /root/fleet-worker --accept-flake-config --no-sandbox --command bash /root/fleet-worker/runner.sh $DF_TIMEOUT" || true

# --- harvest ---
log "harvest worker.diff + events.jsonl -> $DF_OUTDIR"
$INCUS file pull "$WORKER/root/worker.diff" "$DF_OUTDIR/worker.diff" 2>/dev/null || : > "$DF_OUTDIR/worker.diff"
$INCUS file pull "$WORKER/root/events.jsonl" "$DF_OUTDIR/events.jsonl" 2>/dev/null || : > "$DF_OUTDIR/events.jsonl"

# --- authoritative grade (clean checkout + apply + hidden oracle) ---
log "grade on clean checkout"
if source "$HERE/grade.sh"; then GRADE_RC=0; else GRADE_RC=1; fi
log "grade: $(cat "$DF_OUTDIR/grade.json" 2>/dev/null || echo '{}')"
do_teardown; trap - EXIT
exit "$GRADE_RC"
