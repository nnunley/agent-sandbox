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

  # ITER-0007c: PASETO grant auth (sender-constrained; off|log-only|enforce).
  # Rollout starts in LOG-ONLY: the interceptor verifies + logs failures but ALLOWS
  # every RPC, so existing unauthenticated cluster clients (e.g. the Temporal worker
  # harness) are NOT broken. Flip to "enforce" only once every live laneq client
  # attaches a grant (the Temporal worker's laneq client must be grant-wired first).
  laneqAuthMode = "log-only";
  laneqAudience = "laneq://agent-host:${laneqPort}";
  laneqIssuerPubPath = "${laneqDataDir}/issuer.pub";
  # Issuer PUBLIC key (the Mac trust root holds the private half). Public — safe in-repo.
  laneqIssuerPubPem = ''
    -----BEGIN PUBLIC KEY-----
    MCowBQYDK2VwAyEA9PzwZbioY4tb6c8KjLJQe2LntQAgfizUAP3a3YKnLTE=
    -----END PUBLIC KEY-----
  '';
in

{
  # Create the data directory + install the issuer public key (idempotent, declarative).
  system.activationScripts.laneq-data-dir = stringAfter [ "users" ] ''
    mkdir -p ${laneqDataDir}
    chmod 0755 ${laneqDataDir}
    # ITER-0007c: write the issuer public key the auth interceptor verifies grants against.
    cp ${pkgs.writeText "laneq-issuer.pub" laneqIssuerPubPem} ${laneqIssuerPubPath}
    chmod 0644 ${laneqIssuerPubPath}
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
      # ITER-0007c: LANEQ_DB + PASETO grant-auth config (log-only rollout).
      Environment = [
        "LANEQ_DB=${laneqDbPath}"
        "LANEQ_AUTH_MODE=${laneqAuthMode}"
        "LANEQ_AUTH_AUDIENCE=${laneqAudience}"
        "LANEQ_AUTH_PUBKEY_PATHS=${laneqIssuerPubPath}"
      ];
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
