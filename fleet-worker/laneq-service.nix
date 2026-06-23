# fleet-worker/laneq-service.nix
#
# ITER-0006b T1: NixOS systemd service for laneq gRPC server.
# Runs laneq-grpc on 0.0.0.0:9999, reads DB from /srv/laneq/laneq.db (host volume).
# Auto-restarts on failure. Temporal writes to not_before/priority ONLY via gRPC RPCs.
#
{ config, lib, pkgs, ... }:

with lib;

let
  # Build laneq-grpc from this flake's laneq.nix definition
  laneqPackage = pkgs.callPackage ./laneq.nix {};
  laneqDataDir = "/srv/laneq";
  laneqDbPath = "${laneqDataDir}/laneq.db";
  laneqPort = "9999";
in

{
  # Create the data directory if it doesn't exist (idempotent).
  system.activationScripts.laneq-data-dir = stringAfter [ "users" ] ''
    mkdir -p ${laneqDataDir}
    chmod 0755 ${laneqDataDir}
  '';

  # Systemd service: laneq-grpc
  systemd.services.laneq-grpc = {
    description = "laneq gRPC service (ITER-0006b)";
    wantedBy = [ "multi-user.target" ];
    after = [ "network-online.target" ];
    requires = [ "network-online.target" ];

    serviceConfig = {
      Type = "simple";
      ExecStart = "${laneqPackage}/bin/laneq-grpc --addr 0.0.0.0:${laneqPort}";
      Environment = "LANEQ_DB=${laneqDbPath}";
      Restart = "on-failure";
      RestartSec = 5;
      StandardOutput = "journal";
      StandardError = "journal";
      SyslogIdentifier = "laneq-grpc";
      # Security: only the root user runs this (standard practice in systemd).
      # gRPC is unsecured (cluster trust boundary); TLS is deferred to ITER-0007.
    };

    # Pre-start: ensure the directory exists and the SQLite DB is initialized.
    preStart = ''
      mkdir -p ${laneqDataDir}
      chmod 0755 ${laneqDataDir}
      # DB is created on first Push() call if missing; no explicit init needed.
    '';
  };

  # Logging: journald captures stdout/stderr. Query with:
  #   journalctl -u laneq-grpc -n 20 -o cat
}
