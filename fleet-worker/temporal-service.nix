# fleet-worker/temporal-service.nix
#
# ITER-0007b T0.1: NixOS systemd service for the Temporal time plane.
#
# Runs a durable single-node Temporal server via temporal-cli's `server start-dev`
# with a file-backed SQLite DB on the host-mounted volume /srv/temporal (so deferred
# workflows + durable timers survive a container/host restart, not just a service
# restart). gRPC frontend on 0.0.0.0:7233.
#
# Why start-dev (not the upstream services.temporal module): the stock module runs
# `temporal-server start`, which needs explicit schema bootstrapping (temporal-sql-tool)
# for a file-backed SQLite store — the upstream NixOS test dodges this with an in-memory
# (non-durable) store. `start-dev --db-filename` auto-bootstraps the schema on first boot
# and reopens it on restart (empirically verified on the cluster 2026-06-24: a registered
# namespace survived a kill+restart against the same db file). This keeps the approved
# Task 0 mandate — Nix package + systemd service + host-volume persistence — and mirrors
# the hand-rolled fleet-worker/laneq-service.nix discipline.
#
# Sole-writer seam: Temporal writes laneq scheduling fields (effective_priority, not_before)
# ONLY via the laneq gRPC Defer/Reprioritize RPCs, never the laneq DB directly. The single-
# writer guarantee is process-level discipline (one Temporal worker role), NOT laneq lease
# exclusivity (laneq leases are non-exclusive; SCENARIO-0092). See
# docs/plans/2026-06-23-iter0007b-temporal-deploy.md.
{ config, lib, pkgs, ... }:

with lib;

let
  temporalPackage = pkgs.temporal-cli;
  temporalDataDir = "/srv/temporal";
  temporalDbPath = "${temporalDataDir}/temporal.db";
  temporalPort = "7233";
in

{
  # Ensure the data directory exists (idempotent). The actual durable storage is the
  # Incus host volume mounted at /srv/temporal (attached via `incus config device add`).
  system.activationScripts.temporal-data-dir = stringAfter [ "users" ] ''
    mkdir -p ${temporalDataDir}
    chmod 0755 ${temporalDataDir}
  '';

  # Systemd service: temporal time plane (durable single-node).
  systemd.services.temporal = {
    description = "Temporal time plane (ITER-0007b, durable single-node start-dev)";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" ];
    requires = [ "network-online.target" ];

    serviceConfig = {
      Type = "simple";
      # --headless: no Web UI (we only need the gRPC frontend).
      # --ip 0.0.0.0: bind frontend on the cluster bridge (trust boundary, same as laneq).
      # --db-filename: file-backed SQLite on the host volume → durable across restart.
      ExecStart = ''
        ${temporalPackage}/bin/temporal server start-dev \
          --db-filename ${temporalDbPath} \
          --ip 0.0.0.0 \
          --port ${temporalPort} \
          --headless \
          --log-level error
      '';
      Restart = "on-failure";
      RestartSec = 5;
      StandardOutput = "journal";
      StandardError = "journal";
      SyslogIdentifier = "temporal";
    };

    preStart = ''
      mkdir -p ${temporalDataDir}
      chmod 0755 ${temporalDataDir}
      # Schema is auto-bootstrapped by start-dev on first boot; no explicit init needed.
    '';
  };

  # Logging: journald captures stdout/stderr. Query with:
  #   journalctl -u temporal -n 20 -o cat
}
