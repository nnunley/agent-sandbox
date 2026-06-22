#!/usr/bin/env bash
# Cluster verification harness — scenario runner (ITER-0005b Task 0).
#
#   bash fleet-worker/cluster-tests/run.sh <scenario>
#   scenarios: microvm-boot(0029) durable-vm(0004) nspawn-fast(0005)
#              hardtier(0006) trust-boundary(0007) golden-launch(0003) teardown(0008ac2)
#
# Exit codes:
#   0  PASS  — gate met
#   0  SKIP  — agent-host unreachable (CI-safe: no Mac Nix/cluster)
#   2  PENDING — substrate not yet provisioned (the owning story hasn't landed)
#   1  FAIL  — gate NOT met (real regression / unmet AC)
#
# The harness is the verification GATE for this cluster-only iteration: every story's
# acceptance is proven by `run.sh <scenario>` meeting its gate (gates.env). Until the
# owning substrate lands, the scenario reports PENDING (not a false PASS).
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"
# shellcheck disable=SC1091
source "$HERE/gates.env"

SCEN="${1:-}"
[ -n "$SCEN" ] || { grep -E '^#   (scenarios|bash)' "$0"; exit 64; }

# Off-cluster → SKIP (exit 0): the corpus command must not FAIL in CI.
if ! cluster_reachable; then
  echo "SKIP ${SCEN}: agent-host (${REMOTE}:${GUEST}) unreachable — cluster-only scenario"
  exit 0
fi

# pending <story> <human-readable why> — substrate not yet provisioned.
pending() { echo "PENDING ${SCEN}: $2 (owner ${1}; not yet provisioned)"; exit 2; }

MICROVM_UNIT="${FLEET_MICROVM_UNIT:-microvm@test-vm.service}"

case "$SCEN" in
  microvm-boot|0029)   # STORY-0017 AC-3 / SCENARIO-0029: durable microVM boot-to-ready ≤ 5s.
    guest_exec "systemctl cat ${MICROVM_UNIT}" >/dev/null 2>&1 || pending STORY-0007 "no ${MICROVM_UNIT} unit"
    raw=""; for i in $(seq 1 "${N_MICROVM}"); do
      guest_exec "systemctl stop ${MICROVM_UNIT}" >/dev/null 2>&1 || true; sleep 1
      t0=$(now_ms); guest_exec "systemctl start ${MICROVM_UNIT}" >/dev/null 2>&1
      [ "$(wait_microvm_ready "${MICROVM_UNIT}")" = "1" ] || { echo "FAIL ${SCEN}: boot $i never reached ready"; exit 1; }
      raw+="$(( $(now_ms) - t0 ))"$'\n'
    done
    s=$(compute_stats <<<"$raw"); echo "$SCEN stats: $s"
    assert_le "$(stat_field "$s" mean)" "${GATE_MICROVM_BOOT_MS}" "microVM boot-to-ready mean"; exit $? ;;

  durable-vm|0004)     # STORY-0007/0008 / SCENARIO-0004: durable VM stays up across task cycles
                       # (0 restarts) while in-guest units spin up fast on the warm /nix store.
    COORD_UNIT="${FLEET_COORD_UNIT:-microvm@fleet-coord.service}"
    vm_active "${COORD_UNIT}" || pending STORY-0007 "durable coordinator VM (${COORD_UNIT}) not active — not deployed"
    boot0="$(coord_ssh 'cat /proc/sys/kernel/random/boot_id' 2>/dev/null | tr -d '[:space:]')"
    [ -n "$boot0" ] || { echo "FAIL ${SCEN}: coordinator VM up but not SSH-reachable at ${COORD_IP}"; exit 1; }
    raw=""; cycles="${N_DURABLE_CYCLES:-10}"
    for i in $(seq 1 "$cycles"); do
      # One task cycle: a transient in-guest unit (disposable). Time launch→exit in the guest.
      ms="$(coord_ssh 'a=$(date +%s%N); systemd-run --wait --collect --quiet -- true; echo $(( ($(date +%s%N)-a)/1000000 ))' 2>/dev/null | tr -d '[:space:]')"
      [[ "$ms" =~ ^[0-9]+$ ]] && raw+="${ms}"$'\n'
    done
    boot1="$(coord_ssh 'cat /proc/sys/kernel/random/boot_id' 2>/dev/null | tr -d '[:space:]')"
    s="$(compute_stats <<<"$raw")"; echo "$SCEN unit-spinup stats: $s"
    rc=0
    assert_true "$([ -n "$boot0" ] && [ "$boot0" = "$boot1" ] && echo 1 || echo 0)" "durable VM 0 restarts across ${cycles} task cycles (boot_id stable)" || rc=1
    assert_le "$(stat_field "$s" p99)" "${GATE_UNIT_SPINUP_P99_MS}" "in-guest unit spin-up p99" || rc=1
    exit $rc ;;

  nspawn-fast|0005)    # STORY-0021 / SCENARIO-0005: in-guest nspawn --ephemeral sub-second + namespace isolation.
    COORD_UNIT="${FLEET_COORD_UNIT:-microvm@fleet-coord.service}"
    vm_active "${COORD_UNIT}" || pending STORY-0007 "durable coordinator VM (${COORD_UNIT}) not active — not deployed"
    coord_push_unit >/dev/null 2>&1 || { echo "FAIL ${SCEN}: could not push fleet-unit.sh into coord VM"; exit 1; }
    coord_ssh "bash /root/fleet-unit.sh run warm 'echo READY'" >/dev/null 2>&1 || true  # warm template once
    # N ephemeral spin-ups, measured IN-GUEST (launch → READY marker).
    raw="$(coord_ssh "for i in \$(seq 1 ${N_NSPAWN}); do a=\$(date +%s%N); bash /root/fleet-unit.sh run fast-\$i 'echo READY' >/dev/null 2>&1; b=\$(date +%s%N); echo \$(( (b-a)/1000000 )); done" 2>/dev/null)"
    s="$(compute_stats <<<"$raw")"; echo "$SCEN stats: $s"
    rc=0
    assert_le "$(stat_field "$s" mean)" "${GATE_NSPAWN_SPINUP_MS}" "nspawn fast-tier spin-up mean" || rc=1
    # Namespace isolation: the unit's PID namespace inode differs from the VM host's.
    unit_ns="$(coord_ssh "bash /root/fleet-unit.sh run nstest 'readlink /proc/self/ns/pid'" 2>/dev/null | tr -d '[:space:]')"
    host_ns="$(coord_ssh "readlink /proc/self/ns/pid" 2>/dev/null | tr -d '[:space:]')"
    assert_true "$([ -n "$unit_ns" ] && [ "$unit_ns" != "$host_ns" ] && echo 1 || echo 0)" "fast-tier PID namespace isolated from VM host" || rc=1
    exit $rc ;;

  hardtier|0006)       # STORY-0022 / SCENARIO-0006: per-task Firecracker hard-tier spin-up ≤ 2.5s p99.
    WORKER_UNIT="${FLEET_WORKER_UNIT:-microvm@worker-vm.service}"
    guest_exec "systemctl cat ${WORKER_UNIT}" >/dev/null 2>&1 || pending STORY-0022 "no ${WORKER_UNIT} (per-task hard-tier microVM not provisioned)"
    # Per-task boot: stop → start → ready (same readiness sentinel as microvm-boot 0029:
    # firecracker process up + guest MainPID present). N stop/start cycles model per-task spin-up.
    raw=""; for i in $(seq 1 "${N_HARDTIER}"); do
      guest_exec "systemctl stop ${WORKER_UNIT}" >/dev/null 2>&1 || true; sleep 1
      t0=$(now_ms); guest_exec "systemctl start ${WORKER_UNIT}" >/dev/null 2>&1
      [ "$(wait_microvm_ready "${WORKER_UNIT}")" = "1" ] || { echo "FAIL ${SCEN}: boot $i never reached ready"; exit 1; }
      raw+="$(( $(now_ms) - t0 ))"$'\n'
    done
    s="$(compute_stats <<<"$raw")"; echo "$SCEN stats: $s"
    assert_le "$(stat_field "$s" p99)" "${GATE_HARDTIER_SPINUP_P99_MS}" "per-task Firecracker hard-tier boot p99"; exit $? ;;

  trust-boundary|0007) # STORY-0024 / SCENARIO-0007: guest owns its kernel (hardware boundary), single-domain v1.
    COORD_UNIT="${FLEET_COORD_UNIT:-microvm@fleet-coord.service}"
    vm_active "${COORD_UNIT}" || pending STORY-0007 "durable coordinator VM (${COORD_UNIT}) not active — not deployed"
    rc=0
    # AC-1: the guest runs its OWN kernel (Firecracker = hardware trust boundary), distinct
    # from the agent-host LXC's host kernel — a hardware boundary, not a shared-kernel namespace.
    host_kern="$(guest_exec "uname -r" 2>/dev/null | tr -d '[:space:]')"
    guest_kern="$(coord_ssh "uname -r" 2>/dev/null | tr -d '[:space:]')"
    echo "$SCEN: host kernel=${host_kern} guest kernel=${guest_kern}"
    assert_true "$([ -n "$guest_kern" ] && [ "$guest_kern" != "$host_kern" ] && echo 1 || echo 0)" "guest owns its kernel (hardware trust boundary): ${guest_kern} ≠ host ${host_kern}" || rc=1
    # AC-2 (single-domain v1): disposable units run INSIDE that one VM (the trust domain).
    coord_push_unit >/dev/null 2>&1 || { echo "FAIL ${SCEN}: could not push fleet-unit.sh into coord VM"; exit 1; }
    unit_kern="$(coord_ssh "bash /root/fleet-unit.sh run domaincheck 'uname -r'" 2>/dev/null | tr -d '[:space:]')"
    # The unit runs on the VM's kernel (not the host's) ⇒ it executes INSIDE the trust-domain VM.
    assert_true "$([ -n "$unit_kern" ] && [ "$unit_kern" = "$guest_kern" ] && [ "$unit_kern" != "$host_kern" ] && echo 1 || echo 0)" "disposable unit runs inside the trust-domain VM (unit kernel ${unit_kern} = VM, ≠ host)" || rc=1
    exit $rc ;;

  golden-launch|0003)  # STORY-0005 / SCENARIO-0003: incus copy from golden boots ready with NO live build.
    # Capture before grep: `... | grep -q` + pipefail SIGPIPEs incus (141) → false negative.
    aliases="$(incus image alias list "${REMOTE}:" 2>/dev/null)"
    grep -q 'fleet-golden' <<<"$aliases" || pending STORY-0005 "no fleet-golden image (build-once + snapshot not done)"
    inst="fleet-golden-copy-$(now_ms)"
    rc=0
    # Launch a FRESH disposable copy from the golden image (btrfs CoW; never built live).
    t0=$(now_ms)
    if ! incus launch "${REMOTE}:fleet-golden" "${inst}" >/dev/null 2>&1; then
      echo "FAIL ${SCEN}: incus launch from fleet-golden failed"; exit 1
    fi
    # Readiness: poll until the container is exec-ready (the real signal; avoids the
    # `incus list | grep -q` + pipefail SIGPIPE trap).
    for _ in $(seq 1 60); do
      if incus exec "${REMOTE}:${inst}" -- true >/dev/null 2>&1; then break; fi
      sleep 0.5
    done
    launch_ms=$(( $(now_ms) - t0 ))
    # Proof it is a GOLDEN clone (pre-baked), not a freshly built stock image: the marker is
    # present immediately with NO nixos-rebuild / nix build run during launch (AC-2).
    marker="$(incus exec "${REMOTE}:${inst}" -- cat /etc/fleet-golden-version 2>/dev/null | tr -d '\r')"
    assert_true "$([ -n "$marker" ] && echo 1 || echo 0)" "golden marker present on launched copy (no live build): ${marker:-<absent>}" || rc=1
    # Writable scratch: /workspace and /tmp accept writes (STORY-0049 AC-5).
    ws="$(incus exec "${REMOTE}:${inst}" -- bash -lc 'touch /workspace/.probe 2>/dev/null && echo rw || echo ro' 2>/dev/null | tr -d '[:space:]')"
    tm="$(incus exec "${REMOTE}:${inst}" -- bash -lc 'touch /tmp/.probe 2>/dev/null && echo rw || echo ro' 2>/dev/null | tr -d '[:space:]')"
    assert_true "$([ "$ws" = rw ] && echo 1 || echo 0)" "/workspace writable on golden copy" || rc=1
    assert_true "$([ "$tm" = rw ] && echo 1 || echo 0)" "/tmp writable on golden copy" || rc=1
    echo "$SCEN: golden copy launched in ${launch_ms} ms (CoW from image, no live build)"
    # Teardown the disposable copy (stop-then-delete). --force on the STOP (not a delete -f of a
    # running instance) avoids the D5 delete-hang while still being deterministic; the per-task
    # hot-path teardown lives on the microVM tiers (unit-kill), not here.
    incus stop --force "${REMOTE}:${inst}" >/dev/null 2>&1 || true
    incus delete "${REMOTE}:${inst}" >/dev/null 2>&1 || incus delete --force "${REMOTE}:${inst}" >/dev/null 2>&1 || true
    exit $rc ;;

  teardown|0008ac2)    # STORY-0008 AC-2: teardown is unit-kill only — no `incus delete` in the hot path.
    COORD_UNIT="${FLEET_COORD_UNIT:-microvm@fleet-coord.service}"
    vm_active "${COORD_UNIT}" || pending STORY-0007 "durable coordinator VM (${COORD_UNIT}) not active — not deployed"
    coord_push_unit >/dev/null 2>&1 || { echo "FAIL ${SCEN}: could not push fleet-unit.sh into coord VM"; exit 1; }
    rc=0
    # Structural: the hot-path launcher invokes no `incus` command (only doc comments may mention it).
    if grep -vE '^[[:space:]]*#' "$FLEET_UNIT_SRC" | grep -qw incus; then
      echo "FAIL ${SCEN}: launcher invokes incus in the hot path"; rc=1
    else
      echo "PASS gate: teardown path is incus-free (unit-kill only, D5)"
    fi
    # Behavioral: spawn a background ephemeral unit, kill it, measure teardown IN-GUEST.
    ms="$(coord_ssh 'pid=$(bash /root/fleet-unit.sh spawn-bg teardown-probe); a=$(date +%s%N); bash /root/fleet-unit.sh kill $pid >/dev/null 2>&1; b=$(date +%s%N); echo $(( (b-a)/1000000 ))' 2>/dev/null | tr -d '[:space:]')"
    [[ "$ms" =~ ^[0-9]+$ ]] || { echo "FAIL ${SCEN}: could not measure unit teardown"; exit 1; }
    echo "$SCEN teardown: ${ms} ms (unit-kill, no incus delete)"
    assert_le "$ms" "${GATE_TEARDOWN_MS}" "teardown unit-kill bounded" || rc=1
    exit $rc ;;

  *) echo "unknown scenario: $SCEN"; exit 64 ;;
esac
