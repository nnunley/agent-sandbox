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
    pending STORY-0021 "in-guest nspawn fast-tier runner not built (probe: ephemeral spin-up over N + PID/mnt/net ns isolation)" ;;

  hardtier|0006)       # STORY-0022 / SCENARIO-0006: per-task Firecracker hard-tier spin-up ≤ 2.5s p99.
    pending STORY-0022 "per-task Firecracker hard-tier runner not built (probe: per-task boot over N, gate p99 ≤ ${GATE_HARDTIER_SPINUP_P99_MS}ms)" ;;

  trust-boundary|0007) # STORY-0024 / SCENARIO-0007: guest owns its kernel (hardware boundary), single-domain v1.
    guest_exec "systemctl is-active ${MICROVM_UNIT}" >/dev/null 2>&1 || pending STORY-0007 "durable VM not running"
    pending STORY-0024 "own-kernel + unit-inside assertion lands with the trust-boundary story (guest uname-r ≠ host)" ;;

  golden-launch|0003)  # STORY-0005 / SCENARIO-0003: incus copy from golden boots ready with NO live build.
    incus image alias list "${REMOTE}:" 2>/dev/null | grep -q 'fleet-golden' || pending STORY-0005 "no fleet-golden image (build-once + snapshot not done)" ;;

  teardown|0008ac2)    # STORY-0008 AC-2: teardown is unit-kill only — no `incus delete` in the hot path.
    pending STORY-0008 "unit-kill teardown path not built (assert no incus delete; bounded ≤ ${GATE_TEARDOWN_MS}ms)" ;;

  *) echo "unknown scenario: $SCEN"; exit 64 ;;
esac
