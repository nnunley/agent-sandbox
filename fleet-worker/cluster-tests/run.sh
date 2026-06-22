#!/usr/bin/env bash
# Cluster verification harness — scenario runner (ITER-0005b Task 0).
#
#   bash fleet-worker/cluster-tests/run.sh <scenario>
#   scenarios: microvm-boot(0029) durable-vm(0004) nspawn-fast(0005)
#              hardtier(0006) trust-boundary(0007) golden-launch(0003) teardown(0008ac2)
#   ITER-0005c image track: golden-full(0065) cleanroom(0066) provider-routing(0067)
#              skills-path(0068) skills-discovery(0069)
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

  golden-full|0065)    # STORY-0075 AC-1 / SCENARIO-0065: NixOS golden built once with the FULL
                       # toolchain realized; `incus copy` per task = zero rebuild.
    aliases="$(incus image alias list "${REMOTE}:" 2>/dev/null)"
    grep -q "${FLEET_GOLDEN_IMAGE}" <<<"$aliases" || pending STORY-0075 "no ${FLEET_GOLDEN_IMAGE} image (build-once + snapshot not done)"
    inst="$(launch_golden_copy "${FLEET_GOLDEN_IMAGE}")" || { echo "FAIL ${SCEN}: launch from ${FLEET_GOLDEN_IMAGE} failed"; exit 1; }
    rc=0
    # FULL-golden signal: the realized toolchain resolves on a launched copy WITHOUT a live
    # build — on PATH (baked) or via the realized devshell at GOLDEN_FLAKE_PATH (substitution
    # only). If the core agent CLIs are absent the image is only the substrate (not the full
    # golden) → PENDING STORY-0075 (T3 not yet landed).
    GFP="${GOLDEN_FLAKE_PATH:-/etc/fleet-worker}"
    # `command -v` is a shell builtin, so it must run INSIDE `bash -lc` under nix develop
    # (nix develop -c needs an executable, not a builtin).
    resolve_tool() { incus exec "${REMOTE}:${inst}" -- bash -lc \
      "command -v $1 >/dev/null 2>&1 || nix develop ${GFP} --accept-flake-config --no-sandbox -c bash -lc 'command -v $1' >/dev/null 2>&1"; }
    resolve_tool claude && resolve_tool lean-ctx || { reap_copy "${inst}"; pending STORY-0075 "full golden toolchain not realized (claude/lean-ctx absent on copy) — T3 pending"; }
    miss=""; for t in ${GOLDEN_TOOLCHAIN}; do resolve_tool "$t" || miss+="$t "; done
    assert_true "$([ -z "$miss" ] && echo 1 || echo 0)" "golden copy exposes realized toolchain (${GOLDEN_TOOLCHAIN})${miss:+ — missing: $miss}" || rc=1
    marker="$(incus exec "${REMOTE}:${inst}" -- cat /etc/fleet-golden-version 2>/dev/null | tr -d '\r')"
    assert_true "$([ -n "$marker" ] && echo 1 || echo 0)" "golden marker present (no live build): ${marker:-<absent>}" || rc=1
    # copy-per-task works: a SECOND fresh copy launches from the same golden (zero rebuild).
    inst2="$(launch_golden_copy "${FLEET_GOLDEN_IMAGE}")" && incus exec "${REMOTE}:${inst2}" -- true >/dev/null 2>&1
    assert_true "$([ -n "$inst2" ] && echo 1 || echo 0)" "incus copy golden per task works (2nd copy launched, no rebuild)" || rc=1
    reap_copy "${inst}"; [ -n "$inst2" ] && reap_copy "${inst2}"
    exit $rc ;;

  cleanroom|0066)      # STORY-0075 AC-2/AC-3 / SCENARIO-0066: clean-room byte-identical regen +
                       # bridge-ON graded run on the let-go repo (ITER-0003 journey0003 fixture).
    aliases="$(incus image alias list "${REMOTE}:" 2>/dev/null)"
    grep -q "${FLEET_GOLDEN_IMAGE}" <<<"$aliases" || pending STORY-0075 "no ${FLEET_GOLDEN_IMAGE} image (AC-1 full golden first)"
    # ATTEMPTED on the golden's nix-pinned go1.26.4 (2026-06-22, cleanroom-attempt.sh +
    # results/cleanroom-2026-06-22.log): `make generate` succeeds, but the regenerated native-Go
    # lowered TEST package does NOT compile (pkg/rt/core_go_lowered/test/test.go: "declared and not
    # used: v73" / "missing return"), so `make check-generated` + cluster-A build-fail. This is a
    # let-go upstream lowering codegen bug (reproduces on the pinned toolchain → NOT a Mac artifact,
    # refuting ITER-0003's hypothesis), not a golden/grader defect. AC-2/AC-3 CARRIED per the
    # ITER-0005c PAR carry-allowance (trigger a: regen produces non-compiling artifacts). The golden
    # itself (AC-1, SCENARIO-0065) is green. To re-attempt: bash cluster-tests/cleanroom-attempt.sh.
    pending STORY-0075 "AC-2/AC-3 carried: let-go native-Go lowering emits a non-compiling test pkg on `make generate` (upstream codegen bug; see results/cleanroom-2026-06-22.log). Golden AC-1 is green."
    ;;

  provider-routing|0067) # STORY-0076 AC-1 / SCENARIO-0067: golden EXPORTS the cheap-implementer CLIs.
                       # (Dispatcher --provider/--model passthrough + grader-determinism is the Go
                       #  contract test TestScenario0067 in modules/incus-dispatcher — run in CI.)
    aliases="$(incus image alias list "${REMOTE}:" 2>/dev/null)"
    grep -q "${FLEET_GOLDEN_IMAGE}" <<<"$aliases" || pending STORY-0076 "no ${FLEET_GOLDEN_IMAGE} image (golden not built)"
    inst="$(launch_golden_copy "${FLEET_GOLDEN_IMAGE}")" || { echo "FAIL ${SCEN}: launch from ${FLEET_GOLDEN_IMAGE} failed"; exit 1; }
    GFP="${GOLDEN_FLAKE_PATH:-/etc/fleet-worker}"
    # `command -v` is a shell builtin, so it must run INSIDE `bash -lc` under nix develop
    # (nix develop -c needs an executable, not a builtin).
    resolve_tool() { incus exec "${REMOTE}:${inst}" -- bash -lc \
      "command -v $1 >/dev/null 2>&1 || nix develop ${GFP} --accept-flake-config --no-sandbox -c bash -lc 'command -v $1' >/dev/null 2>&1"; }
    # If none of the provider CLIs resolve, the export line isn't enabled yet → PENDING STORY-0076.
    any=0; for c in ${GOLDEN_PROVIDER_CLIS}; do resolve_tool "$c" && any=1; done
    [ "$any" = 1 ] || { reap_copy "${inst}"; pending STORY-0076 "provider CLIs not exported in golden (flake export not enabled) — T4 pending"; }
    rc=0; miss=""; for c in ${GOLDEN_PROVIDER_CLIS}; do resolve_tool "$c" || miss+="$c "; done
    assert_true "$([ -z "$miss" ] && echo 1 || echo 0)" "golden exports provider CLIs (${GOLDEN_PROVIDER_CLIS})${miss:+ — missing: $miss}" || rc=1
    reap_copy "${inst}"
    exit $rc ;;

  skills-path|0068)    # STORY-0077 / SCENARIO-0068: curated skills resolve at the discovery path
                       # on a launched golden copy, as regular files (copy-tree, not symlinks).
    aliases="$(incus image alias list "${REMOTE}:" 2>/dev/null)"
    grep -q "${FLEET_GOLDEN_IMAGE}" <<<"$aliases" || pending STORY-0077 "no ${FLEET_GOLDEN_IMAGE} image (golden not built)"
    inst="$(launch_golden_copy "${FLEET_GOLDEN_IMAGE}")" || { echo "FAIL ${SCEN}: launch from ${FLEET_GOLDEN_IMAGE} failed"; exit 1; }
    incus exec "${REMOTE}:${inst}" -- test -d "${SKILLS_DISCOVERY_PATH}" 2>/dev/null \
      || { reap_copy "${inst}"; pending STORY-0077 "no ${SKILLS_DISCOVERY_PATH} on golden copy (skills not baked) — T2 pending"; }
    rc=0
    n="$(incus exec "${REMOTE}:${inst}" -- bash -lc "find -L '${SKILLS_DISCOVERY_PATH}' -name SKILL.md | wc -l" 2>/dev/null | tr -d '[:space:]')"
    echo "$SCEN: ${SKILLS_DISCOVERY_PATH} has ${n} SKILL.md"
    assert_true "$([ "$n" = "${GATE_SKILLS_COUNT}" ] && echo 1 || echo 0)" "all ${GATE_SKILLS_COUNT} curated skills at discovery path" || rc=1
    # copy-tree, not symlink: the path itself is a real directory and SKILL.md are regular files.
    nsl="$(incus exec "${REMOTE}:${inst}" -- bash -lc "find '${SKILLS_DISCOVERY_PATH}' -name SKILL.md -type l | wc -l" 2>/dev/null | tr -d '[:space:]')"
    assert_true "$([ "$nsl" = 0 ] && echo 1 || echo 0)" "skills are copy-tree (no symlinked SKILL.md; found ${nsl})" || rc=1
    reap_copy "${inst}"
    exit $rc ;;

  skills-discovery|0069) # STORY-0078 / SCENARIO-0069: the curated bundle BUILDS with all 13 skills
                       # (standalone gate — needs only the small bundle derivation, not the golden).
    BH="${BUNDLE_BUILD_HOST:-nix-server}"
    incus exec "${REMOTE}:${BH}" -- true >/dev/null 2>&1 || pending STORY-0078 "bundle build host ${BH} unreachable"
    dst=/tmp/fw-iter0005c
    if ! COPYFILE_DISABLE=1 tar --no-mac-metadata -C "$HERE/../.." --exclude='*.DS_Store' -czf - fleet-worker 2>/dev/null \
         | incus exec "${REMOTE}:${BH}" -- bash -lc "rm -rf $dst && mkdir -p $dst && tar -C $dst --warning=no-unknown-keyword -xzf -"; then
      echo "FAIL ${SCEN}: could not push fleet-worker to ${BH}"; exit 1
    fi
    incus exec "${REMOTE}:${BH}" -- bash -lc "grep -q 'agent-skills-bundle' $dst/fleet-worker/flake.nix" \
      || pending STORY-0078 "flake.nix does not expose agent-skills-bundle yet — T1 pending"
    # --no-sandbox: nix-server is an unprivileged LXC with no kernel-namespace build sandbox
    # (same constraint worker-container.nix disables declaratively). The bundle is a tiny
    # copy-tree derivation, so substitution + an unsandboxed copy is safe here.
    NIXFLAGS="--extra-experimental-features 'nix-command flakes' --accept-flake-config --no-sandbox"
    out="$(incus exec "${REMOTE}:${BH}" -- bash -lc "cd $dst/fleet-worker && nix build --no-link --print-out-paths '${BUNDLE_FLAKE_ATTR}' ${NIXFLAGS} 2>&1")" \
      || { echo "FAIL ${SCEN}: nix build ${BUNDLE_FLAKE_ATTR} failed:"; echo "$out" | tail -8; exit 1; }
    store="$(printf '%s\n' "$out" | tail -1)"
    n="$(incus exec "${REMOTE}:${BH}" -- bash -lc "find -L '$store' -name SKILL.md | wc -l" 2>/dev/null | tr -d '[:space:]')"
    echo "$SCEN: bundle ${store} has ${n} SKILL.md"
    assert_true "$([ "$n" = "${GATE_SKILLS_COUNT}" ] && echo 1 || echo 0)" "bundle builds with all ${GATE_SKILLS_COUNT} curated skills"; exit $? ;;

  *) echo "unknown scenario: $SCEN"; exit 64 ;;
esac
