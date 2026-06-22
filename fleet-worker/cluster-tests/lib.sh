#!/usr/bin/env bash
# Cluster-verification harness — shared library (ITER-0005b Task 0).
#
# Pure measurement + acceptance-gate logic (TDD-tested locally in tests/lib.test.sh)
# plus cluster-facing readiness pollers used by run.sh. The cluster pollers require a
# reachable `agent-host` Incus remote; run.sh self-skips when it is unreachable so the
# corpus commands are safe to invoke in CI (no Mac Nix/cluster).
#
# Acceptance gates live in gates.env (declarative); see README.md for the readiness
# sentinel definitions per scenario.

REMOTE="${FLEET_REMOTE:-ndn-desktop}"
GUEST="${FLEET_GUEST:-agent-host}"   # the LXC container hosting the Firecracker microVM(s)
LIBDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Repo artifact: the in-guest FAST-tier disposable-unit launcher (STORY-0021/0008),
# pushed into the durable coord VM on demand by coord_push_unit.
FLEET_UNIT_SRC="${FLEET_UNIT_SRC:-$LIBDIR/../unit/fleet-unit.sh}"

# --- pure logic (Mac-runnable, tested) -------------------------------------------------

# compute_stats reads whitespace/newline-separated integers on stdin and echoes a single
# line: "N=<n> mean=<m> p50=<m> p99=<m> min=<m> max=<m> stddev=<f>". Empty input → "N=0".
compute_stats() {
  awk '
    /^[0-9]+$/ { v[n++]=$1; sum+=$1 }
    END {
      if (n==0) { print "N=0 mean=0 p50=0 p99=0 min=0 max=0 stddev=0"; exit }
      for (i=0;i<n;i++) for (j=i+1;j<n;j++) if (v[j]<v[i]) { t=v[i]; v[i]=v[j]; v[j]=t }
      mean=int(sum/n)
      p50=v[int(n/2)]
      pi=int((n*99)/100); if (pi>=n) pi=n-1; p99=v[pi]
      ss=0; for (i=0;i<n;i++) { d=v[i]-mean; ss+=d*d }
      printf "N=%d mean=%d p50=%d p99=%d min=%d max=%d stddev=%.1f\n", n, mean, p50, p99, v[0], v[n-1], sqrt(ss/n)
    }'
}

# stat_field "<statsline>" <key> → the value for key (e.g. mean, p99).
stat_field() {
  printf '%s\n' "$1" | tr ' ' '\n' | awk -F= -v k="$2" '$1==k {print $2}'
}

# assert_le <actual> <gate> <label> → PASS/FAIL line; rc 0 if actual<=gate, else 1.
assert_le() {
  local actual="$1" gate="$2" label="$3"
  if [ "$actual" -le "$gate" ]; then
    echo "PASS gate: $label ($actual <= $gate)"; return 0
  fi
  echo "FAIL gate: $label ($actual > $gate)"; return 1
}

# assert_true <0|1> <label> → PASS/FAIL line; rc 0 if 1, else 1.
assert_true() {
  if [ "$1" = "1" ]; then echo "PASS gate: $2"; return 0; fi
  echo "FAIL gate: $2"; return 1
}

# --- cluster reachability + readiness pollers (require agent-host) ---------------------

# cluster_reachable → rc 0 if the Incus remote answers; non-zero otherwise. Used by
# run.sh to SKIP (exit 0) rather than FAIL when off the cluster.
cluster_reachable() {
  command -v incus >/dev/null 2>&1 || return 2
  incus list "${REMOTE}:" >/dev/null 2>&1
}

# guest_exec <cmd...> — run a command inside the agent-host LXC.
guest_exec() { incus exec "${REMOTE}:${GUEST}" -- bash -lc "$*"; }

# coord_ssh <cmd...> — run a command INSIDE the durable coordinator micro-VM, SSH'd into
# from agent-host (which holds the root@agent-host key). FLEET_COORD_IP defaults to the
# static 10.88.0.2 from guests/coordinator-vm.nix.
COORD_IP="${FLEET_COORD_IP:-10.88.0.2}"
coord_ssh() {
  guest_exec "ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -o BatchMode=yes root@${COORD_IP} '$*'"
}

# coord_push_unit [src] [dst] — copy the in-guest launcher into the coord VM. The file
# streams Mac → agent-host (incus exec stdin) → coord VM (ssh stdin → cat), avoiding scp
# key juggling. Returns non-zero if the copy fails.
coord_push_unit() {
  local src="${1:-$FLEET_UNIT_SRC}" dst="${2:-/root/fleet-unit.sh}"
  [ -f "$src" ] || { echo "coord_push_unit: missing $src" >&2; return 1; }
  incus exec "${REMOTE}:${GUEST}" -- bash -lc \
    "ssh -o StrictHostKeyChecking=no -o BatchMode=yes root@${COORD_IP} 'cat > ${dst} && chmod +x ${dst}'" < "$src"
}

# vm_active <unit> — rc 0 if the named microVM systemd unit is active on agent-host.
vm_active() { guest_exec "systemctl is-active ${1}" 2>/dev/null | grep -q '^active'; }

# now_ms — wall clock in milliseconds (python3 for portability).
now_ms() { python3 -c 'import time; print(int(time.time()*1000))'; }

# launch_golden_copy <image-alias> — launch a FRESH disposable copy from a golden image
# (btrfs CoW; never built live) and poll until exec-ready. Echoes the instance name on
# success (the caller must reap it with reap_copy); rc 1 if launch fails. Used by the
# ITER-0005c golden/skills/provider scenarios (0065/0067/0068).
launch_golden_copy() {
  local img="$1" inst="golden-probe-$(now_ms)"
  incus launch "${REMOTE}:${img}" "${inst}" >/dev/null 2>&1 || return 1
  local i; for i in $(seq 1 120); do
    incus exec "${REMOTE}:${inst}" -- true >/dev/null 2>&1 && break
    sleep 0.5
  done
  echo "$inst"
}

# reap_copy <instance> — stop-then-delete a disposable copy (D5 discipline: --force on the
# STOP, never `incus delete -f` of a running instance, to avoid the delete-hang).
reap_copy() {
  incus stop --force "${REMOTE}:$1" >/dev/null 2>&1 || true
  incus delete "${REMOTE}:$1" >/dev/null 2>&1 || incus delete --force "${REMOTE}:$1" >/dev/null 2>&1 || true
}

# wait_microvm_ready <unit> [timeout_polls] — READINESS SENTINEL for a Firecracker
# microVM: systemd unit active AND Main PID present (then caller adds network settle).
# Echoes 1 (ready) / 0 (timed out).
wait_microvm_ready() {
  local unit="$1" max="${2:-120}" i=0
  while [ "$i" -lt "$max" ]; do
    if guest_exec "systemctl is-active ${unit}" 2>/dev/null | grep -q '^active' \
       && guest_exec "systemctl show -p MainPID --value ${unit}" 2>/dev/null | grep -qE '^[1-9][0-9]*$'; then
      echo 1; return 0
    fi
    sleep 0.1; i=$((i+1))
  done
  echo 0
}
