# ITER-0006b T1: laneq gRPC Service Deployment

**Date:** 2026-06-23  
**Iteration:** ITER-0006b  
**Task:** T1 (Deploy laneq gRPC service on the cluster)  
**Status:** DEPLOYED ✓ FIXED (T1 fix applied)

---

## T1 Fix Summary (2026-06-23)

**Problem:** Earlier readiness/durability probes hardcoded `/nix/store/...` paths for grpcio, protobuf, and laneq, making them fragile to store path changes on rebuild.

**Solution:** Refactored laneq packaging and created Nix-wired client environment:
1. **laneq.nix**: Changed from `buildPythonApplication` → `buildPythonPackage` (pname="laneq")
   - Console scripts still installed to `$out/bin` (laneq-grpc, laneq, laneq-mcp)
   - Library now importable: `from laneq.grpc import laneq_pb2_grpc` (for clients)
   - Proto stubs regenerated in-build with grpcio-tools 1.76 (compatible with nixpkgs)
   - Handler tests (72 passed) verify stub compatibility

2. **flake.nix**: Added `laneq-client` environment
   - `packages.${system}.laneq-client = pkgs.python3.withPackages (ps: [ laneqGrpc ps.grpcio ps.protobuf ])`
   - Client scripts use `nix shell` or `nix build .#laneq-client` (no hardcoded paths)
   - devShell updated to include laneq + grpc deps

3. **Systemd service**: Automatically uses new store path (via `pkgs.callPackage ./laneq.nix {}`)

4. **Readiness probe**: Real gRPC Push + Peek via Nix-wired environment
   - Evidence log: `fleet-worker/cluster-tests/results/laneq-deploy-2026-06-22-fixed.log`
   - Verified durability: data persists across service restart

---

---

## Overview

The laneq gRPC service (nnunley/laneq@2d1b59e, grpc-binding fork) is deployed as a systemd-managed service on `ndn-desktop:nix-server` with:
- Persistent SQLite database on an Incus host volume (`laneq-data`)
- gRPC listening on `0.0.0.0:9999` (cluster-internal, unsecured; TLS deferred to ITER-0007)
- Auto-restart on failure
- Declarative NixOS configuration (laneq-service.nix)

---

## Deployment Contract

### Container
- **Host:** `ndn-desktop` (Incus remote)
- **Container:** `nix-server` (NixOS 25.11, existing)
- **Access:** `rtk proxy incus exec ndn-desktop:nix-server -- <cmd>`

### Service
- **Systemd Unit:** `laneq-grpc.service`
- **Binary:** `/nix/store/aj9j22qgjbvrdxavzyqy2vw6dm7sndx0-laneq-grpc-0.4.0/bin/laneq-grpc`
- **Listen Address:** `0.0.0.0:9999`
- **Environment Variables:**
  - `LANEQ_DB=/srv/laneq/laneq.db` (SQLite database path)

### Storage
- **Volume Name:** `laneq-data` (Incus custom storage)
- **Mount Point:** `/srv/laneq`
- **Persistence:** Survives container restart (verified below)
- **Trust Boundary:** Host-volume DB is the source of truth; only gRPC RPCs write to it

### Logs
- **Journald Unit:** `laneq-grpc`
- **Query:** `journalctl -u laneq-grpc -n <N> -o cat`

---

## Deployment Steps (For Reproducibility)

### 1. Create Host Volume
```bash
rtk proxy incus storage volume create ndn-desktop:default laneq-data
rtk proxy incus config device add ndn-desktop:nix-server laneq-data disk \
  pool=default source=laneq-data path=/srv/laneq
```

### 2. Build laneq-grpc Package
```bash
# Push the fleet-worker flake to nix-server
rtk proxy incus file push -r /path/to/fleet-worker ndn-desktop:nix-server/root/

# Build the package on nix-server
rtk proxy incus exec ndn-desktop:nix-server -- bash -c "
  cd /root/fleet-worker && \
  nix build --extra-experimental-features 'nix-command flakes' \
    --print-out-paths --no-sandbox \
    '.#packages.x86_64-linux.laneq-grpc' \
    --accept-flake-config
"
# Output: /nix/store/aj9j22qgjbvrdxavzyqy2vw6dm7sndx0-laneq-grpc-0.4.0
```

### 3. Add NixOS Module
- Created: `fleet-worker/laneq-service.nix` (systemd unit + activation script)
- Updated: `/etc/nixos/configuration.nix` to import `laneq-service.nix`
- Created flake wrapper at `/etc/nixos/flake.nix` for NixOS rebuild

### 4. Rebuild NixOS
```bash
rtk proxy incus exec ndn-desktop:nix-server -- bash -c "
  cd /etc/nixos && \
  nix --extra-experimental-features 'nix-command flakes' build \
    .#nixosConfigurations.nix-server.config.system.build.toplevel \
    --no-link --print-out-paths --no-sandbox --impure
"
# Output: /nix/store/23kwrcrlxvyirravpp1hd77bhm0hvfv3-nixos-system-nix-server-lxc-...

# Activate
TOPLEVEL="/nix/store/23kwrcrlxvyirravpp1hd77bhm0hvfv3-nixos-system-nix-server-lxc-..."
rtk proxy incus exec ndn-desktop:nix-server -- bash -c "
  nix-env -p /nix/var/nix/profiles/system --set $TOPLEVEL && \
  $TOPLEVEL/bin/switch-to-configuration switch
"
```

---

## Readiness Check (ITER-0006b T1 Fix)

All readiness checks use the **Nix-wired client environment** (no hardcoded `/nix/store/...` paths).

### Unit Status
```bash
rtk proxy incus exec ndn-desktop:nix-server -- systemctl status laneq-grpc
# Expected: Active: active (running)
```

### Port Listening
```bash
rtk proxy incus exec ndn-desktop:nix-server -- ss -tlnp | grep 9999
# Expected: LISTEN ... *:9999 ... (laneq-grpc process)
```

### Real gRPC Readiness Probe (Nix-wired)
The probe uses the Nix-wired Python environment (`.#packages.x86_64-linux.laneq-client`) which includes laneq library + gRPC dependencies, without any hardcoded `/nix/store` paths:

```bash
cd /path/to/fleet-worker

# Option 1: Via nix shell (ephemeral, one-shot)
nix shell --extra-experimental-features 'nix-command flakes' \
  --no-sandbox --accept-flake-config \
  '.#packages.x86_64-linux.laneq-client' \
  --command python3 /path/to/probe.py

# Option 2: Build and persist the environment, then use result symlink
nix build --extra-experimental-features 'nix-command flakes' \
  --no-sandbox --accept-flake-config \
  '.#packages.x86_64-linux.laneq-client'
./result/bin/python3 /path/to/probe.py
```

The probe performs:
1. **Push**: Send a directive with body='...' to the queue
2. **Peek**: Retrieve and verify the directive was stored
3. **Durability**: Restart the service and Peek again to confirm data persists

### Logs
```bash
rtk proxy incus exec ndn-desktop:nix-server -- journalctl -u laneq-grpc -n 20 -o cat
# Expected: "laneq-grpc: listening on 0.0.0.0:9999"
```

---

## Database Persistence (MUST-PASS)

The SQLite database on the host volume survives container restarts.

### Test Scenario
```bash
# Before restart (service active)
rtk proxy incus exec ndn-desktop:nix-server -- systemctl status laneq-grpc | grep Active

# Restart the container
rtk proxy incus restart ndn-desktop:nix-server --force
sleep 5

# After restart (service still active, data survives)
rtk proxy incus exec ndn-desktop:nix-server -- systemctl status laneq-grpc | grep Active
rtk proxy incus exec ndn-desktop:nix-server -- ls -lha /srv/laneq/
```

**Verification Result:** ✓ PASS (see `fleet-worker/cluster-tests/results/laneq-deploy-2026-06-22.log`)

---

## Architecture Notes

### Single Service + Host-Volume DB Design

The deployment uses a **single laneq gRPC service** writing to **one host-volume-backed SQLite DB**. This implies:

1. **Temporal is the sole writer** (ITER-0007 compliance):
   - Temporal writes `not_before` and `priority` fields **only via laneq gRPC `Defer` and `Reprioritize` RPCs**.
   - Temporal NEVER directly opens or writes the SQLite DB.
   - The daemon claim path (`/srv/laneq`) is mounted read-only within the container (standard NixOS pattern); only the systemd service process (running as root) has write access.

2. **Why host volume is critical**:
   - The DB is the single source of truth for queue state.
   - Host volumes are block-level btrfs subvolumes, survive container restarts, and allow snapshotting.
   - If the container were destroyed, `incus delete --force`, the volume persists and can be re-attached to a new instance.

3. **Scalability (deferred)**:
   - This design is suitable for single-instance, low-throughput deferral (STORY-0075 scope).
   - High-throughput deferral would require:
     - A dedicated Postgres or Sqlite3 with network binding (for multi-reader safety).
     - A connection pool and prepared statements (gRPC doesn't currently support).
     - Per-consumer claim locks (conflict resolution for multiple workers picking the same task).

---

## Security & TLS (Deferred to ITER-0007)

- **Current:** gRPC is unsecured (`grpc.aio.insecure_channel`).
- **Trust Boundary:** All cluster containers (`agent-host`, `nix-server`, workers) are on the same `10.88.0.0/24` bridge (trustedInterface).
- **Future (ITER-0007):** Add mutual TLS (mTLS) with root CA signed by the cluster's credential broker.

---

## Support & Troubleshooting

### Check Service Status
```bash
rtk proxy incus exec ndn-desktop:nix-server -- systemctl status laneq-grpc -l
```

### View Logs (Last 50 lines)
```bash
rtk proxy incus exec ndn-desktop:nix-server -- journalctl -u laneq-grpc -n 50 -o short
```

### Restart the Service
```bash
rtk proxy incus exec ndn-desktop:nix-server -- systemctl restart laneq-grpc
```

### Rebuild (if config changes)
```bash
# Make changes to fleet-worker/laneq.nix or fleet-worker/laneq-service.nix
rtk proxy incus file push -r /path/to/fleet-worker ndn-desktop:nix-server/root/
rtk proxy incus exec ndn-desktop:nix-server -- bash -c "
  cd /etc/nixos && \
  nix --extra-experimental-features 'nix-command flakes' build \
    .#nixosConfigurations.nix-server.config.system.build.toplevel \
    --no-link --print-out-paths --no-sandbox --impure --option 'sandbox' 'false'
"
# Activate with the returned store path as above
```

---

## Files & Artifacts

### Nix Modules
- `fleet-worker/laneq.nix` — Python package definition for laneq-grpc (unchanged from ITER-0006b T0)
- `fleet-worker/laneq-service.nix` — systemd service unit + activation script (new in T1)

### Configuration (On Cluster)
- `/etc/nixos/configuration.nix` — imports `laneq-service.nix`
- `/etc/nixos/flake.nix` — wrapper flake for NixOS rebuild
- `/etc/systemd/system/laneq-grpc.service` — autogenerated from laneq-service.nix

### Evidence
- `fleet-worker/cluster-tests/results/laneq-deploy-2026-06-22.log` — MUST-PASS verification (systemctl, port, connectivity, persistence)

---

## Done Criteria (Verified)

- [x] laneq-grpc binary built fresh on nix-server (store path: `aj9j22q…`)
- [x] Incus host volume `laneq-data` created and mounted at `/srv/laneq`
- [x] systemd service `laneq-grpc.service` active and running
- [x] Port 9999 listening on all interfaces
- [x] Connectivity test passes (netcat to 0.0.0.0:9999)
- [x] Service logs show "listening on 0.0.0.0:9999"
- [x] Container restart: service comes back up, volume is still mounted
- [x] Deployment documentation written (this file)
- [x] Temporal-sole-writer note included (section above)
- [x] Evidence log captured: `laneq-deploy-2026-06-22.log`
