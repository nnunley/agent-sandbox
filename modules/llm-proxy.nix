# modules/llm-proxy.nix
#
# LLM reverse proxy service for agent micro-VMs.
# Listens on the micro-VM bridge (default: 10.88.0.1:12071).
#
# API keys are loaded from systemd credentials injected by Incus:
#   incus config set ndn-desktop:agent-host \
#     systemd.credential.anthropic_api_key=sk-ant-... \
#     systemd.credential.openai_api_key=sk-...
#
# Agents configure their SDK:
#   ANTHROPIC_BASE_URL=http://10.88.0.1:12071/anthropic
#   OPENAI_BASE_URL=http://10.88.0.1:12071/openai

{ config, lib, pkgs, ... }:

let
  cfg = config.services.llm-proxy;
  llm-proxy = pkgs.callPackage ./llm-proxy/package.nix { };
in {
  options.services.llm-proxy = {
    enable = lib.mkEnableOption "LLM reverse proxy for agent micro-VMs";

    listenAddr = lib.mkOption {
      type = lib.types.str;
      default = "10.88.0.1:12071";
      description = "Address and port to listen on (should be the bridge gateway).";
    };

    localFastUrl = lib.mkOption {
      type = lib.types.str;
      default = "http://192.168.86.49:8081";
      description = ''
        Upstream URL for the /local-fast/* route. Defaults to a llama.cpp
        instance on the host LAN. Override per-deployment to avoid baking
        a specific LAN address into the system closure.
      '';
    };

    localLargeUrl = lib.mkOption {
      type = lib.types.str;
      default = "http://192.168.86.49:8082";
      description = ''
        Upstream URL for the /local-large/* route. Defaults to a llama.cpp
        instance on the host LAN. Override per-deployment.
      '';
    };

    maxBodyBytes = lib.mkOption {
      type = lib.types.ints.positive;
      default = 32 * 1024 * 1024;
      description = "Maximum request body size in bytes. Larger requests get 413.";
    };
  };

  config = lib.mkIf cfg.enable {
    environment.systemPackages = [ llm-proxy ];

    systemd.services.llm-proxy = {
      description = "LLM reverse proxy for agent micro-VMs";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" "systemd-networkd.service" ];

      serviceConfig = {
        Restart = "on-failure";
        RestartSec = "5s";

        # Security hardening
        DynamicUser = true;
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        PrivateDevices = true;

        # Import API keys from systemd credentials (injected by Incus).
        # ImportCredential is non-fatal if the credential is absent — the
        # service starts with empty keys and logs proxy errors instead of
        # refusing to start.
        # Keys land in $CREDENTIALS_DIRECTORY/<name>
        ImportCredential = [
          "anthropic_api_key"
          "openai_api_key"
        ];
      };

      environment = {
        LLM_PROXY_ADDR        = cfg.listenAddr;
        LOCAL_FAST_URL        = cfg.localFastUrl;
        LOCAL_LARGE_URL       = cfg.localLargeUrl;
        LLM_PROXY_MAX_BODY    = toString cfg.maxBodyBytes;
      };

      # Read credentials from $CREDENTIALS_DIRECTORY into env vars before exec
      script = ''
        export ANTHROPIC_API_KEY=""
        export OPENAI_API_KEY=""

        creds="''${CREDENTIALS_DIRECTORY:-}"
        if [ -n "$creds" ]; then
          [ -f "$creds/anthropic_api_key" ] && ANTHROPIC_API_KEY=$(cat "$creds/anthropic_api_key")
          [ -f "$creds/openai_api_key" ]    && OPENAI_API_KEY=$(cat "$creds/openai_api_key")
        fi

        exec ${llm-proxy}/bin/llm-proxy
      '';
    };

    # Note: no firewall rule needed — br-microvm is already a trustedInterface
    # in host/networking.nix, so all traffic from the bridge is accepted.
  };
}
