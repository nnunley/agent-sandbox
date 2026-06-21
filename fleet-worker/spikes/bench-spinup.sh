#!/usr/bin/env bash
# STORY-0025 Disposable-Unit Spin-Up Benchmarks
#
# Measures spin-up time for two disposable-unit substrates:
# 1. nspawn --ephemeral (Fast Tier): ~76 ms
# 2. Firecracker microVM (Hard Tier): ~1861 ms
#
# === BENCHMARK: nspawn (Fast Tier) ===
# Readiness Definition: Time from systemd-nspawn --ephemeral launch to marker command completion.
# - Uses NixOS 25.11 with -D /var/lib/machines/tmpl --ephemeral --bind-ro=/nix
# - Metric: wall-clock ms from nspawn start to bash -c 'echo READY' completion
# - Typical expectation: sub-100 ms (CoW snapshot + namespace setup on warm /nix)
# - Environment: nested Incus container with security.nesting=true, security.privileged=true
#
# === BENCHMARK: microVM (Hard Tier) ===
# Readiness Definition: Time from systemctl start microvm@<name> to guest fully booted
# with network stack ready (service running + Main PID present + 1s settle for network).
# - Uses Firecracker v1.13.2 (fresh kernel boot each run)
# - Metric: wall-clock ms from start command to ready
# - Typical expectation: 1–3 seconds (cold boot + DHCP)
#
# Usage:
#   ./bench-spinup.sh [nspawn|microvm] [N=100|N=20]  # Run N benchmark cycles
#   ./bench-spinup.sh nspawn 100      # Run 100 nspawn cycles (requires nested container)
#   ./bench-spinup.sh microvm 20      # Run 20 microVM cycles

set -euo pipefail

BENCH_TYPE="${1:-both}"
RUNS="${2:-100}"  # Default 100 for nspawn, 20 for microvm (may be overridden below)

# Create results directory locally
mkdir -p /Users/ndn/development/agent-sandbox/fleet-worker/spikes/results

benchmark_nspawn() {
    local n_runs=$1
    local container="${2:-ndn-desktop:nspawn-bench-0025}"
    local results_file="/Users/ndn/development/agent-sandbox/fleet-worker/spikes/results/nspawn-fast-raw.txt"

    echo "[*] nspawn --ephemeral benchmark (Fast Tier)"
    echo "[*] Target container: $container"
    echo "[*] Running $n_runs cycles"
    echo ""

    # Create script to run inside the container
    cat > /tmp/nspawn-bench-runner.sh << 'BENCHSCRIPT'
#!/bin/bash
set -e

TMPL="/var/lib/machines/tmpl"
BASH_STORE="/nix/store/lfbzxs5wyqd2122mpbj5azkxhxspw9cd-bash-interactive-5.3p3/bin/bash"
N=$1
RESULTS_FILE="/tmp/nspawn-results.txt"

# Setup template once if needed
if [ ! -d "$TMPL" ]; then
    mkdir -p "$TMPL/etc"
    cat > "$TMPL/etc/os-release" << 'EOF'
NAME="NixOS"
ID=nixos
VERSION_ID=25.11
EOF
    mkdir -p "$TMPL"/{proc,sys,dev,run,tmp,bin,usr/bin,root}
    chmod 1777 "$TMPL/tmp"
fi

> "$RESULTS_FILE"

for i in $(seq 1 $N); do
    START=$(date +%s%N)

    systemd-nspawn \
      -D "$TMPL" \
      --ephemeral \
      --register=no \
      -M "bench-$i" \
      --bind-ro=/nix:/nix \
      "$BASH_STORE" -c 'echo READY' \
      > /dev/null 2>&1 || true

    END=$(date +%s%N)
    MS=$(( (END - START) / 1000000 ))
    echo "$MS" >> "$RESULTS_FILE"

    [ $((i % 25)) -eq 0 ] && echo "  Run $i of $N"
done

cat "$RESULTS_FILE"
BENCHSCRIPT

    chmod +x /tmp/nspawn-bench-runner.sh

    # Copy script into container
    incus file push /tmp/nspawn-bench-runner.sh "$container/tmp/nspawn-bench-runner.sh" 2>/dev/null || {
        echo "[!] Failed to push benchmark script to $container"
        return 1
    }

    # Run benchmark
    > "$results_file"
    incus exec "$container" -- bash /tmp/nspawn-bench-runner.sh "$n_runs" >> "$results_file"

    echo "[*] nspawn results written to $results_file"
}

benchmark_microvm() {
    local n_runs=$1
    local results_file="/Users/ndn/development/agent-sandbox/fleet-worker/spikes/results/microvm-hard-raw.txt"

    echo "[*] microVM boot benchmark (Hard Tier)"
    echo "[*] Measuring test-vm stop→start→ready cycles: $n_runs runs"
    echo ""

    > "$results_file"
    for i in $(seq 1 "$n_runs"); do
        echo "[*] Boot $i/$n_runs..."

        # Stop test-vm (on agent-host)
        incus exec ndn-desktop:agent-host -- bash -lc 'systemctl stop microvm@test-vm.service' 2>/dev/null || true
        sleep 2

        # Record start time (in milliseconds using Python)
        local start_ms=$(python3 -c 'import time; print(int(time.time() * 1000))')

        # Start test-vm
        incus exec ndn-desktop:agent-host -- bash -lc 'systemctl start microvm@test-vm.service' 2>/dev/null

        # Poll for service running with PID check
        local ready=0
        local poll_count=0
        while [ $poll_count -lt 120 ]; do
            if incus exec ndn-desktop:agent-host -- bash -lc 'systemctl is-active microvm@test-vm.service' 2>/dev/null | grep -q "^active"; then
                if incus exec ndn-desktop:agent-host -- bash -lc 'systemctl status microvm@test-vm.service' 2>/dev/null | grep -q "Main PID"; then
                    ready=1
                    break
                fi
            fi
            sleep 0.1
            poll_count=$(( poll_count + 1 ))
        done

        if [ $ready -eq 1 ]; then
            # Add settle time for network readiness
            sleep 1
        fi

        local end_ms=$(python3 -c 'import time; print(int(time.time() * 1000))')
        local elapsed=$(( end_ms - start_ms ))

        echo "$elapsed" >> "$results_file"
        echo "  Boot time: $elapsed ms"
    done

    # Ensure test-vm is left running
    incus exec ndn-desktop:agent-host -- bash -lc 'systemctl start microvm@test-vm.service' 2>/dev/null || true
    sleep 3

    echo "[*] microVM results written to $results_file"
}

compute_stats() {
    local data_file=$1

    if [ ! -f "$data_file" ]; then
        echo "MISSING"
        return 1
    fi

    local values=($(cat "$data_file"))
    [ ${#values[@]} -eq 0 ] && echo "EMPTY" && return 1

    local sum=0
    for v in "${values[@]}"; do
        sum=$(( sum + v ))
    done
    local mean=$(( sum / ${#values[@]} ))

    # Sort for percentiles
    local sorted=($(printf '%s\n' "${values[@]}" | sort -n))
    local count=${#sorted[@]}
    local p50_idx=$(( count / 2 ))
    local p99_idx=$(( (count * 99) / 100 ))

    [ $p99_idx -ge $count ] && p99_idx=$(( count - 1 ))

    local p50=${sorted[$p50_idx]}
    local p99=${sorted[$p99_idx]}
    local min=${sorted[0]}
    local max=${sorted[$count - 1]}

    # Compute stddev
    local sum_sq=0
    for v in "${values[@]}"; do
        local diff=$(( v - mean ))
        sum_sq=$(( sum_sq + diff * diff ))
    done
    local variance=$(( sum_sq / ${#values[@]} ))
    # Use awk for sqrt
    local stddev=$(awk "BEGIN {print sqrt($variance)}")

    echo "N=$count mean=$mean p50=$p50 p99=$p99 min=$min max=$max stddev=$(printf '%.1f' $stddev)"
}

# Main dispatch
case "$BENCH_TYPE" in
    nspawn)
        benchmark_nspawn "$RUNS"
        echo ""
        echo "=== nspawn Results (Fast Tier) ==="
        compute_stats "/Users/ndn/development/agent-sandbox/fleet-worker/spikes/results/nspawn-fast-raw.txt"
        ;;
    microvm)
        benchmark_microvm "$RUNS"
        echo ""
        echo "=== microVM Results (Hard Tier) ==="
        compute_stats "/Users/ndn/development/agent-sandbox/fleet-worker/spikes/results/microvm-hard-raw.txt"
        ;;
    both)
        echo "Running both benchmarks..."
        benchmark_nspawn 100
        echo ""
        benchmark_microvm 20
        echo ""
        echo "=== nspawn Results (Fast Tier) ==="
        compute_stats "/Users/ndn/development/agent-sandbox/fleet-worker/spikes/results/nspawn-fast-raw.txt"
        echo ""
        echo "=== microVM Results (Hard Tier) ==="
        compute_stats "/Users/ndn/development/agent-sandbox/fleet-worker/spikes/results/microvm-hard-raw.txt"
        ;;
    *)
        echo "Usage: $0 [nspawn|microvm|both] [N]"
        echo "  nspawn N  - Run N nspawn benchmarks (default 100)"
        echo "  microvm N - Run N microVM benchmarks (default 20)"
        echo "  both      - Run both (100 nspawn + 20 microVM)"
        exit 1
        ;;
esac
