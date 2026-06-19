# fleet-dogfood Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A project-scoped `fleet-dogfood` skill that dispatches one task (brief + repo + hidden oracle) to an ephemeral cluster worker, harvests the diff, authoritatively grades it on a clean checkout, and reports pass/fail — capturing the proven ITER-0000 dogfood loop as one command.

**Architecture:** Two bash entry points over a shared lib: `fleet-dogfood-prep.sh` (configure a worker baseline + a measurement gate that decides whether a golden snapshot is needed) and `fleet-dogfood.sh` (per-dispatch: spin up ephemeral worker → deliver repo/brief/token → `nix develop … runner.sh` → harvest → grade inline on a clean checkout → stop-then-delete). All cluster ops go through `incus` via the `rtk proxy incus` convention. Grading mirrors the dispatcher's `--external-grading` logic inline (clean checkout + `git apply` + run oracle) so the skill is self-contained.

**Tech Stack:** bash, incus CLI (remote `ndn-desktop`), nix (on workers only), the existing `fleet-worker/` flake + `runner.sh` + `worker-container.nix`, the populated `file:///srv/nix-shared` cache.

## Global Constraints

- Skill location: `.claude/skills/fleet-dogfood/` (project-scoped, version-controlled). Verbatim.
- All `incus` calls go through `${INCUS:-rtk proxy incus}` (quiet, remote-aware). Verbatim.
- No nix on the dev machine — every nix/build op runs **inside a worker** via `incus exec`.
- Worker is **non-root** (`claude --dangerously-skip-permissions` refuses root); run as uid 1000 / `worker`.
- `nix develop` needs BOTH `--accept-flake-config` AND `--no-sandbox` in an unprivileged container.
- Teardown is **stop-then-delete** (`incus stop --timeout N --force` then `incus delete`); never `delete --force` a running container (hang). Teardown ALWAYS runs.
- The grade is **authoritative**: run on a clean checkout the worker never saw. Exit 0 iff the oracle passed.
- Commit messages must NOT contain "Claude", "Generated with Claude", "Co-Authored-By: Claude", or "claude.com" (pre-commit hook blocks them).
- Bash: `set -euo pipefail` in every script; quote expansions.

---

### Task 1: Shared lib + dispatch arg parsing + teardown trap

**Files:**
- Create: `.claude/skills/fleet-dogfood/lib.sh`
- Create: `.claude/skills/fleet-dogfood/fleet-dogfood.sh`
- Test: `.claude/skills/fleet-dogfood/test/test-args.sh`

**Interfaces:**
- Produces: `lib.sh` exporting `INCUS`, `log <msg>`, `die <msg>` (stderr + exit 1), `incus_x <args…>` (wraps `$INCUS`), `register_teardown <container>` + `do_teardown` (stop-then-delete, idempotent). `fleet-dogfood.sh` parses `--name --brief --repo --ref --oracle --model --golden --output-dir --timeout` into vars `DF_NAME DF_BRIEF …` and validates required (`--name --brief --repo --oracle`).

- [ ] **Step 1: Write the failing test**

```bash
# .claude/skills/fleet-dogfood/test/test-args.sh
#!/usr/bin/env bash
set -uo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"
fail=0
check() { if eval "$1"; then echo "ok: $2"; else echo "FAIL: $2"; fail=1; fi; }

# Missing required args → non-zero exit + a clear message naming the missing flag.
out=$(bash "$HERE/fleet-dogfood.sh" --name x --repo /tmp --oracle /tmp/o 2>&1); rc=$?
check '[ "$rc" -ne 0 ]' "missing --brief exits non-zero"
check 'echo "$out" | grep -qi "brief"' "error names the missing --brief"

# --help exits 0 and lists the flags.
out=$(bash "$HERE/fleet-dogfood.sh" --help 2>&1); rc=$?
check '[ "$rc" -eq 0 ]' "--help exits 0"
check 'echo "$out" | grep -q -- "--oracle"' "--help lists --oracle"

exit $fail
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash .claude/skills/fleet-dogfood/test/test-args.sh`
Expected: FAIL (fleet-dogfood.sh / lib.sh do not exist yet → "No such file").

- [ ] **Step 3: Write `lib.sh`**

```bash
# .claude/skills/fleet-dogfood/lib.sh
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
```

- [ ] **Step 4: Write `fleet-dogfood.sh` arg parsing skeleton**

```bash
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
```

- [ ] **Step 5: Run test to verify it passes**

Run: `chmod +x .claude/skills/fleet-dogfood/*.sh && bash .claude/skills/fleet-dogfood/test/test-args.sh`
Expected: all `ok:` lines, exit 0.

- [ ] **Step 6: Commit**

```bash
git add .claude/skills/fleet-dogfood/
git commit -m "feat(fleet-dogfood): shared lib + dispatch arg parsing + teardown trap"
```

---

### Task 2: `fleet-dogfood-prep.sh` — worker baseline + measurement gate

**Files:**
- Create: `.claude/skills/fleet-dogfood/fleet-dogfood-prep.sh`
- Create: `.claude/skills/fleet-dogfood/test/test-prep.sh`

**Interfaces:**
- Consumes: `lib.sh`.
- Produces: a configured base worker container `fleet-dogfood-base`; a recorded mode file `.claude/skills/fleet-dogfood/.mode` containing `fresh` or `golden:<snapshot>`; on `golden` mode a snapshot `fleet-dogfood-base/pristine`. Prints the measured "fresh worker ready" seconds.

- [ ] **Step 1: Write the failing test**

```bash
# .claude/skills/fleet-dogfood/test/test-prep.sh
#!/usr/bin/env bash
set -uo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"; fail=0
check() { if eval "$1"; then echo "ok: $2"; else echo "FAIL: $2"; fail=1; fi; }
bash "$HERE/fleet-dogfood-prep.sh" --threshold 120 2>&1 | tee /tmp/df-prep.log
check 'grep -qE "ready in [0-9]+s" /tmp/df-prep.log' "prep reports a measured readiness time"
check 'test -f "$HERE/.mode"' ".mode file written"
check 'grep -qE "^(fresh|golden:)" "$HERE/.mode"' ".mode is fresh or golden:<snap>"
exit $fail
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash .claude/skills/fleet-dogfood/test/test-prep.sh`
Expected: FAIL (`fleet-dogfood-prep.sh` missing).

- [ ] **Step 3: Write `fleet-dogfood-prep.sh`**

```bash
#!/usr/bin/env bash
# fleet-dogfood-prep — configure a base worker, measure fresh-launch readiness, and
# choose fresh-launch vs golden-snapshot. Idempotent.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"; source "$HERE/lib.sh"
REPO_ROOT="$(cd "$HERE/../../.." && pwd)"   # .claude/skills/fleet-dogfood -> repo root
BASE="fleet-dogfood-base"; THRESHOLD=120
while [ $# -gt 0 ]; do case "$1" in --threshold) THRESHOLD="$2"; shift 2;; *) die "unknown arg: $1";; esac; done

# Recreate the base worker from the declarative config.
$INCUS delete "$BASE" --force >/dev/null 2>&1 || true
log "launch + configure base worker ($BASE) from worker-container.nix"
$INCUS launch images:nixos/25.11 "$BASE" >/dev/null
for i in $(seq 1 20); do st=$($INCUS exec "$BASE" -- systemctl is-system-running 2>/dev/null || true); case "$st" in running|degraded) break;; esac; done
$INCUS file push "$REPO_ROOT/fleet-worker/worker-container.nix" "$BASE/etc/nixos/configuration.nix"
# nix-shared RO mount so the local cache is visible during the rebuild + nix develop.
$INCUS config device add "$BASE" nix-shared disk pool=default source=nix-shared path=/srv/nix-shared readonly=true >/dev/null 2>&1 || true
$INCUS exec "$BASE" -- bash -lc 'export NIX_CONFIG="sandbox = false"; nix-channel --update && nixos-rebuild switch' >/dev/null

# Measure: time the worker resolving the toolchain from the local cache (the slow bit a golden would skip).
log "measuring fresh-worker toolchain readiness (local-cache nix develop)…"
$INCUS file push -r "$REPO_ROOT/fleet-worker" "$BASE/home/worker/" >/dev/null
$INCUS exec "$BASE" --user 1000 --env HOME=/home/worker -- chown -R worker /home/worker/fleet-worker >/dev/null 2>&1 || true
start=$(date +%s)
$INCUS exec "$BASE" --user 1000 --env HOME=/home/worker -- bash -lc \
  'nix develop /home/worker/fleet-worker --accept-flake-config --no-sandbox --command claude --version' >/dev/null
elapsed=$(( $(date +%s) - start ))
log "fresh worker ready in ${elapsed}s (threshold ${THRESHOLD}s)"

if [ "$elapsed" -le "$THRESHOLD" ]; then
  echo "fresh" > "$HERE/.mode"
  log "mode=fresh (local cache fast enough; no golden snapshot needed)"
else
  $INCUS snapshot create "$BASE" pristine >/dev/null
  echo "golden:$BASE/pristine" > "$HERE/.mode"
  log "mode=golden ($BASE/pristine) — fresh launch exceeded threshold"
fi
```

- [ ] **Step 4: Run test to verify it passes**

Run: `chmod +x .claude/skills/fleet-dogfood/fleet-dogfood-prep.sh && bash .claude/skills/fleet-dogfood/test/test-prep.sh`
Expected: `ok:` for readiness time, `.mode` exists, `.mode` is `fresh` or `golden:`. (Requires the cluster + populated cache.)

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/fleet-dogfood/
git commit -m "feat(fleet-dogfood): prep with measurement gate for fresh-vs-golden"
```

---

### Task 3: Worker spin-up + deliver + run (per-dispatch core)

**Files:**
- Modify: `.claude/skills/fleet-dogfood/fleet-dogfood.sh` (append spin-up/deliver/run after arg parsing)
- Create: `.claude/skills/fleet-dogfood/test/test-dispatch-smoke.sh`

**Interfaces:**
- Consumes: `lib.sh`, the `.mode` file from Task 2, `DF_*` vars.
- Produces: a running ephemeral worker `df-<name>`; `$DF_OUTDIR/worker.diff` and `$DF_OUTDIR/events.jsonl` harvested after the run.

- [ ] **Step 1: Write the failing test (trivial brief, real cluster)**

```bash
# .claude/skills/fleet-dogfood/test/test-dispatch-smoke.sh
#!/usr/bin/env bash
set -uo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"; ROOT="$(cd "$HERE/../../.." && pwd)"; fail=0
check() { if eval "$1"; then echo "ok: $2"; else echo "FAIL: $2"; fail=1; fi; }
printf 'In the repo, append a line "dogfood smoke" to a new file SMOKE.txt at the repo root. Do not commit.\n' > /tmp/df-brief.txt
bash "$HERE/fleet-dogfood.sh" --name smoke --brief /tmp/df-brief.txt \
  --repo "$ROOT" --ref HEAD --oracle /tmp/df-oracle.sh --output-dir /tmp/df-smoke 2>&1 | tail -20
check 'test -f /tmp/df-smoke/worker.diff' "worker.diff harvested"
check 'test -f /tmp/df-smoke/events.jsonl' "events.jsonl harvested"
exit $fail
```

- [ ] **Step 2: Run test to verify it fails**

Run: `: > /tmp/df-oracle.sh && bash .claude/skills/fleet-dogfood/test/test-dispatch-smoke.sh`
Expected: FAIL (no spin-up/harvest yet → no `worker.diff`).

- [ ] **Step 3: Append spin-up + deliver + run to `fleet-dogfood.sh`**

```bash
# --- spin up ephemeral worker (fresh launch or golden clone per .mode) ---
WORKER="df-$DF_NAME"
MODE="$(cat "$HERE/.mode" 2>/dev/null || echo fresh)"
$INCUS delete "$WORKER" --force >/dev/null 2>&1 || true
register_teardown "$WORKER"
if [ "${DF_GOLDEN:-}" ] || [ "${MODE#golden:}" != "$MODE" ]; then
  SNAP="${DF_GOLDEN:-${MODE#golden:}}"
  log "spin up $WORKER from golden $SNAP (reflink clone)"
  $INCUS copy "$SNAP" "$WORKER" >/dev/null
  $INCUS start "$WORKER" >/dev/null
else
  log "spin up $WORKER (fresh launch + worker-container.nix)"
  $INCUS launch images:nixos/25.11 "$WORKER" >/dev/null
  for i in $(seq 1 20); do st=$($INCUS exec "$WORKER" -- systemctl is-system-running 2>/dev/null || true); case "$st" in running|degraded) break;; esac; done
  $INCUS file push "$REPO_ROOT_GUESS/fleet-worker/worker-container.nix" "$WORKER/etc/nixos/configuration.nix" 2>/dev/null \
    || $INCUS file push "$(cd "$HERE/../../.." && pwd)/fleet-worker/worker-container.nix" "$WORKER/etc/nixos/configuration.nix"
  $INCUS config device add "$WORKER" nix-shared disk pool=default source=nix-shared path=/srv/nix-shared readonly=true >/dev/null 2>&1 || true
  $INCUS exec "$WORKER" -- bash -lc 'export NIX_CONFIG="sandbox = false"; nix-channel --update && nixos-rebuild switch' >/dev/null
fi
ROOT_DIR="$(cd "$HERE/../../.." && pwd)"

# --- deliver repo + brief + token + fleet-worker flake ---
log "deliver repo@$DF_REF + brief + token"
( cd "$DF_REPO" && git bundle create /tmp/df-$DF_NAME.bundle "$DF_REF" >/dev/null )
$INCUS file push /tmp/df-$DF_NAME.bundle "$WORKER/home/worker/repo.bundle"
$INCUS file push -r "$ROOT_DIR/fleet-worker" "$WORKER/home/worker/" >/dev/null
$INCUS file push "$DF_BRIEF" "$WORKER/home/worker/brief.txt"
# fleet-token: prefer an env-injected secret over baking it; require it present.
[ -n "${FLEET_TOKEN:-}" ] || die "FLEET_TOKEN env not set (worker needs CLAUDE_CODE_OAUTH_TOKEN)"
printf '%s' "$FLEET_TOKEN" | $INCUS exec "$WORKER" --user 1000 --env HOME=/home/worker -- tee /home/worker/.fleet-token >/dev/null
$INCUS exec "$WORKER" -- bash -lc 'chown -R worker /home/worker && rm -rf /home/worker/let-go && git clone -q /home/worker/repo.bundle /home/worker/let-go' >/dev/null

# --- run the worker (runner.sh inside the flake env; toolchain from local cache) ---
log "run worker (nix develop … runner.sh, model=$DF_MODEL, timeout=${DF_TIMEOUT}s)"
$INCUS exec "$WORKER" --user 1000 --env HOME=/home/worker -- bash -lc \
  "nix develop /home/worker/fleet-worker --accept-flake-config --no-sandbox --command bash /home/worker/runner.sh $DF_TIMEOUT" || true

# --- harvest ---
log "harvest worker.diff + events.jsonl -> $DF_OUTDIR"
$INCUS file pull "$WORKER/home/worker/worker.diff" "$DF_OUTDIR/worker.diff" 2>/dev/null || : > "$DF_OUTDIR/worker.diff"
$INCUS file pull "$WORKER/home/worker/events.jsonl" "$DF_OUTDIR/events.jsonl" 2>/dev/null || : > "$DF_OUTDIR/events.jsonl"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `FLEET_TOKEN="$(cat ~/.fleet-token 2>/dev/null)" bash .claude/skills/fleet-dogfood/test/test-dispatch-smoke.sh`
Expected: `ok: worker.diff harvested`, `ok: events.jsonl harvested`. Worker auto-torn-down by the EXIT trap.

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/fleet-dogfood/
git commit -m "feat(fleet-dogfood): per-dispatch spin-up, deliver, run, harvest"
```

---

### Task 4: Authoritative grading (clean checkout + apply + oracle)

**Files:**
- Modify: `.claude/skills/fleet-dogfood/fleet-dogfood.sh` (append grade after harvest)
- Create: `.claude/skills/fleet-dogfood/test/test-grade.sh`

**Interfaces:**
- Consumes: `$DF_OUTDIR/worker.diff`, `$DF_REPO`, `$DF_REF`, `$DF_ORACLE`.
- Produces: `$DF_OUTDIR/grade.json` `{ "patch_applied": bool, "exit_code": int, "pass": bool }`; the script's own exit code is 0 iff `pass`.

- [ ] **Step 1: Write the failing test (grading is pure-local; no cluster)**

```bash
# .claude/skills/fleet-dogfood/test/test-grade.sh
#!/usr/bin/env bash
set -uo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"; fail=0
check() { if eval "$1"; then echo "ok: $2"; else echo "FAIL: $2"; fail=1; fi; }
# Build a tiny git repo + a diff that adds PASS marker; oracle greps for it.
tmp=$(mktemp -d); ( cd "$tmp" && git init -q && echo base > f.txt && git add . && git commit -qm base )
( cd "$tmp" && echo MARKER >> f.txt && git diff > /tmp/df-grade.diff && git checkout -q -- f.txt )
printf '#!/usr/bin/env bash\ngrep -q MARKER f.txt\n' > /tmp/df-grade-oracle.sh; chmod +x /tmp/df-grade-oracle.sh
out=$(DF_OUTDIR=/tmp/df-grade-out DF_REPO="$tmp" DF_REF=HEAD DF_ORACLE=/tmp/df-grade-oracle.sh \
  bash -c 'mkdir -p $DF_OUTDIR; cp /tmp/df-grade.diff $DF_OUTDIR/worker.diff; source '"$HERE"'/grade.sh'; echo "rc=$?")
check 'grep -q "\"pass\": *true" /tmp/df-grade-out/grade.json' "passing diff grades pass=true"
check 'echo "$out" | grep -q "rc=0"' "exit 0 on pass"
exit $fail
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash .claude/skills/fleet-dogfood/test/test-grade.sh`
Expected: FAIL (`grade.sh` missing).

- [ ] **Step 3: Write `grade.sh` (sourced helper) + call it from `fleet-dogfood.sh`**

```bash
# .claude/skills/fleet-dogfood/grade.sh — authoritative grade on a clean checkout.
# Expects DF_OUTDIR, DF_REPO, DF_REF, DF_ORACLE in env. Sets exit code via 'pass'.
_grade() {
  local clean; clean=$(mktemp -d)
  git clone -q "$DF_REPO" "$clean"; ( cd "$clean" && git checkout -q "$DF_REF" )
  local applied=false ec=1
  if ( cd "$clean" && git apply --whitespace=nowarn "$DF_OUTDIR/worker.diff" 2>/dev/null ); then
    applied=true
    cp "$DF_ORACLE" "$clean/.oracle"; chmod +x "$clean/.oracle"
    ( cd "$clean" && ./.oracle ); ec=$?
  fi
  local pass=false; [ "$applied" = true ] && [ "$ec" -eq 0 ] && pass=true
  printf '{ "patch_applied": %s, "exit_code": %d, "pass": %s }\n' "$applied" "$ec" "$pass" > "$DF_OUTDIR/grade.json"
  rm -rf "$clean"
  [ "$pass" = true ]
}
_grade
```

Append to `fleet-dogfood.sh`:

```bash
# --- authoritative grade ---
log "grade on clean checkout (apply worker.diff + run hidden oracle)"
if source "$HERE/grade.sh"; then GRADE_RC=0; else GRADE_RC=1; fi
log "grade: $(cat "$DF_OUTDIR/grade.json")"
do_teardown
trap - EXIT
exit "$GRADE_RC"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bash .claude/skills/fleet-dogfood/test/test-grade.sh`
Expected: `ok: passing diff grades pass=true`, `ok: exit 0 on pass`.

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/fleet-dogfood/
git commit -m "feat(fleet-dogfood): authoritative grade on clean checkout + exit code"
```

---

### Task 5: SKILL.md + meta-dogfood verification

**Files:**
- Create: `.claude/skills/fleet-dogfood/SKILL.md`
- Create: `.claude/skills/fleet-dogfood/test/meta-dogfood-peek.sh`

**Interfaces:**
- Consumes: the full skill (Tasks 1–4).
- Produces: `SKILL.md` (frontmatter `name` + `description`, when-to-use, prep/measurement, interface, the proven-loop gotchas); a meta-dogfood test re-running the ITER-0000 `queue.Peek()` task.

- [ ] **Step 1: Write the meta-dogfood test (the real proof)**

```bash
# .claude/skills/fleet-dogfood/test/meta-dogfood-peek.sh
# Re-run the ITER-0000 Peek task through the skill; the hidden oracle must pass.
#!/usr/bin/env bash
set -uo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"; ROOT="$(cd "$HERE/../../.." && pwd)"
cat > /tmp/peek-brief.txt <<'EOF'
In modules/incus-dispatcher/queue, implement Queue.Peek() per the interface doc in
queue.go: return the directive Claim would return next (highest-priority eligible
pending) WITHOUT claiming it; no lease, no mutation; ErrEmpty when none. Do not commit.
EOF
cat > /tmp/peek-oracle.sh <<'EOF'
#!/usr/bin/env bash
cd modules/incus-dispatcher && go test ./queue/ -run Peek
EOF
chmod +x /tmp/peek-oracle.sh
FLEET_TOKEN="${FLEET_TOKEN:-$(cat ~/.fleet-token 2>/dev/null)}" \
bash "$HERE/fleet-dogfood.sh" --name peek-meta --brief /tmp/peek-brief.txt \
  --repo "$ROOT" --ref HEAD --oracle /tmp/peek-oracle.sh --output-dir /tmp/df-peek
rc=$?
echo "meta-dogfood exit=$rc"; cat /tmp/df-peek/grade.json
exit $rc
```

- [ ] **Step 2: Run it to verify the whole loop (expect oracle pass)**

Run: `FLEET_TOKEN="$(cat ~/.fleet-token)" bash .claude/skills/fleet-dogfood/test/meta-dogfood-peek.sh`
Expected: `grade.json` shows `"pass": true`, exit 0. (If the repo already has Peek, the brief is a no-op refactor; the oracle still passes — the loop is what's under test.)

- [ ] **Step 3: Write `SKILL.md`**

```markdown
---
name: fleet-dogfood
description: Dispatch one task (brief + repo + hidden oracle) to an ephemeral cluster worker, harvest the diff, and authoritatively grade it on a clean checkout. Use when dogfooding the fleet — having the fleet build/verify a change end-to-end with an oracle-graded result.
---

# fleet-dogfood

Single-task dispatch primitive. One call → one ephemeral worker → one oracle-graded diff.

## When to use
- Dogfooding the fleet: have a real worker implement a change and prove it with a hidden oracle.
- Per-task dispatch inside a larger build (the caller sequences + applies graded diffs).

## One-time prep
`bash .claude/skills/fleet-dogfood/fleet-dogfood-prep.sh` — configures a base worker,
measures local-cache readiness, and selects fresh-launch (default) or a golden snapshot.

## Dispatch
`FLEET_TOKEN=… bash .claude/skills/fleet-dogfood/fleet-dogfood.sh --name ID --brief FILE \
  --repo PATH --oracle PATH [--ref REF --model M --output-dir DIR --timeout SECS]`
Outputs `worker.diff`, `events.jsonl`, `grade.json`; **exit 0 iff the oracle passed**.

## Gotchas (proven the hard way)
- Worker is NON-root (claude refuses root); runs as uid 1000.
- `nix develop` needs BOTH `--accept-flake-config` AND `--no-sandbox` (unprivileged LXC).
- Toolchain resolves from `file:///srv/nix-shared` (local cache) — populate via
  `scripts/populate-nix-cache.sh`. See `nixos-incus-worker` skill for the worker traps.
- Teardown is stop-then-delete and ALWAYS runs (never `delete --force` a running container).
- The grade runs on a CLEAN checkout the worker never saw — authoritative, anti-reward-hack.
- `FLEET_TOKEN` injects the worker's `CLAUDE_CODE_OAUTH_TOKEN`; never bake it into a snapshot.
```

- [ ] **Step 4: Commit**

```bash
git add .claude/skills/fleet-dogfood/
git commit -m "feat(fleet-dogfood): SKILL.md + meta-dogfood Peek verification"
```

---

## Self-Review

**Spec coverage:** purpose → Tasks 1–5; single-task primitive → Task 1/3; standalone script → all; ephemeral worker → Task 3; conditional golden via measurement → Task 2; interface table → Task 1 usage; outputs (worker.diff/events.jsonl/grade.json, exit 0 iff pass) → Tasks 3–4; data flow → Tasks 3–4; error handling (cluster/golden/timeout/patch-apply/oracle/always-teardown) → lib teardown + grade.sh + die guards; testing (smoke + meta-dogfood) → Tasks 3 & 5; location → Global Constraints; fleet-token open-q → resolved as `FLEET_TOKEN` env injection (Task 3). All covered.

**Placeholder scan:** no TBD/TODO; every code step shows real content. The measurement threshold is a real default (120s) with a `--threshold` override.

**Type consistency:** `.mode` format `fresh|golden:<snap>` written in Task 2, read in Task 3; `grade.json` keys `patch_applied/exit_code/pass` produced in Task 4, asserted in Task 4 test & Task 5; `register_teardown/do_teardown` defined in Task 1, used in Tasks 3–4; `DF_*` vars consistent throughout.
