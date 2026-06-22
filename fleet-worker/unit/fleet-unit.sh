#!/usr/bin/env bash
# fleet-unit.sh — in-guest disposable-unit launcher for the FAST isolation tier
# (STORY-0021 / STORY-0008). Runs INSIDE the durable coordinator micro-VM
# (guests/coordinator-vm.nix), never on agent-host or the Mac.
#
# A "disposable unit" is a `systemd-nspawn --ephemeral` NixOS container sharing the
# VM kernel (namespace isolation: PID/mount/IPC/UTS) and bind-mounting the warm,
# read-only /nix store. Because the unit is ephemeral, its root is a throwaway COW
# overlay: teardown is a process kill + overlay discard — NEVER `incus delete`
# (STORY-0008 AC-2 / D5). incus is not even reachable from inside the guest, so the
# hot path is structurally incus-free.
#
# Subcommands:
#   ensure-template            idempotently create the minimal nspawn template root
#   run <name> <cmd...>        ephemeral run; cmd's exit code propagates (auto-teardown)
#   spawn-bg <name>            background ephemeral sleeper; prints the leader PID
#   kill <pid>                 SIGTERM the leader, wait for exit (teardown, no incus)
#
# Spin-up was measured at 56–66 ms in-guest (gate: sub-second; gates.env
# GATE_NSPAWN_SPINUP_MS=1000).
set -uo pipefail

TMPL="${FLEET_UNIT_TMPL:-/var/lib/machines/tmpl}"

# Resolve the guest's bash store path dynamically (no hardcoded hash).
guest_bash() { readlink -f /run/current-system/sw/bin/bash; }

ensure_template() {
  [ -d "$TMPL" ] && return 0
  mkdir -p "$TMPL"/{proc,sys,dev,run,tmp,bin,usr/bin,root,etc}
  chmod 1777 "$TMPL/tmp"
  printf 'NAME="NixOS"\nID=nixos\nVERSION_ID=25.11\n' > "$TMPL/etc/os-release"
}

# run <name> <cmd...> — synchronous ephemeral unit; warm /nix bound read-only. The guest's
# system profile (/run/current-system/sw) is bound read-only and put on PATH so the unit has
# the full NixOS toolchain (coreutils, git, …), not just bash — a usable task environment.
unit_run() {
  local name="$1"; shift
  ensure_template
  systemd-nspawn \
    -D "$TMPL" \
    --ephemeral \
    --register=no \
    -M "$name" \
    --bind-ro=/nix:/nix \
    --bind-ro=/run/current-system:/run/current-system \
    --setenv=PATH=/run/current-system/sw/bin:/usr/bin:/bin \
    "$(guest_bash)" -lc "$*"
}

# spawn-bg <name> — background ephemeral sleeper for teardown timing; prints leader PID.
unit_spawn_bg() {
  local name="$1"
  ensure_template
  systemd-nspawn \
    -D "$TMPL" \
    --ephemeral \
    --register=no \
    -M "$name" \
    --bind-ro=/nix:/nix \
    "$(guest_bash)" -lc 'sleep 300' >/dev/null 2>&1 &
  echo $!
}

# kill <pid> — terminate the unit leader and wait for it to exit (no incus delete).
unit_kill() {
  local pid="$1"
  kill -TERM "$pid" 2>/dev/null || true
  for _ in $(seq 1 50); do
    kill -0 "$pid" 2>/dev/null || { echo killed; return 0; }
    sleep 0.1
  done
  kill -KILL "$pid" 2>/dev/null || true
  echo killed
}

case "${1:-}" in
  ensure-template) ensure_template ;;
  run)             shift; unit_run "$@" ;;
  spawn-bg)        shift; unit_spawn_bg "$@" ;;
  kill)            shift; unit_kill "$@" ;;
  *) echo "usage: fleet-unit.sh {ensure-template|run <name> <cmd...>|spawn-bg <name>|kill <pid>}" >&2; exit 64 ;;
esac
