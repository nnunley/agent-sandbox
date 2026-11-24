# modules/llm-proxy.nix
#
# LLM reverse proxy service for agent workers: Incus containers on the host's
# network reach it as agent-host.incus:12071, micro-VMs as 10.88.0.1:12071.
# Only callers on allowedNetworks may use it; workers hold no provider
# credentials. The default admits only the micro-VM bridge: add the fleet
# containers' range explicitly, preferably an Incus network only workers use,
# since the default Incus network also carries every other container.
#
# Credentials are loaded from systemd credentials injected by Incus. An OAuth
# token (a Claude subscription) takes precedence over an Anthropic API key:
#   incus config set ndn-desktop:agent-host \
#     systemd.credential.anthropic_oauth_token=sk-ant-oat01-... \
#     systemd.credential.anthropic_api_key=sk-ant-... \
#     systemd.credential.openai_api_key=sk-...
#
# Agents configure their SDK:
#   ANTHROPIC_BASE_URL=http://agent-host.incus:12071/anthropic
#   OPENAI_BASE_URL=http://agent-host.incus:12071/openai

{ config, lib, pkgs, ... }:

let
  cfg = config.services.llm-proxy;
  llm-proxy = pkgs.callPackage ./llm-proxy/package.nix { };
in {
  options.services.llm-proxy = {
    enable = lib.mkEnableOption "LLM reverse proxy for agent micro-VMs";

    listenAddr = lib.mkOption {
      type = lib.types.str;
      default = ":12071";
      description = ''
        Address and port to listen on. All interfaces by default, so both the
        micro-VM bridge and the Incus network reach it; allowedNetworks limits
        who may use it.
      '';
    };

    allowedNetworks = lib.mkOption {
      type = lib.types.str;
      default = "10.88.0.0/24";
      example = "10.88.0.0/24,10.77.0.0/24";
      description = ''
        Comma-separated CIDRs allowed to spend the proxy's credentials. The
        default is the micro-VM bridge only. Add the fleet containers' network
        here; a dedicated Incus network for workers keeps other containers out.
        "local-subnets" admits every subnet agent-host is attached to,
        including the shared default Incus network, so any container there
        could use the credentials.
      '';
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

    ollamaCloudUrl = lib.mkOption {
      type = lib.types.str;
      default = "https://ollama.ai";
      description = ''
        Upstream URL for the /ollama-cloud/* route. API key comes from
        systemd credentials (ollama_cloud_api_key).
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
          "anthropic_oauth_token"
          "anthropic_api_key"
          "openai_api_key"
          "ollama_cloud_api_key"
        ];
      };

      environment = {
        LLM_PROXY_ADDR        = cfg.listenAddr;
        LLM_PROXY_ALLOW       = cfg.allowedNetworks;
        LOCAL_FAST_URL        = cfg.localFastUrl;
        LOCAL_LARGE_URL       = cfg.localLargeUrl;
        OLLAMA_CLOUD_URL      = cfg.ollamaCloudUrl;
        LLM_PROXY_MAX_BODY    = toString cfg.maxBodyBytes;
      };

      # Read credentials from $CREDENTIALS_DIRECTORY into env vars before exec
      script = ''
        export ANTHROPIC_OAUTH_TOKEN=""
        export ANTHROPIC_API_KEY=""
        export OPENAI_API_KEY=""
        export OLLAMA_CLOUD_API_KEY=""

        creds="''${CREDENTIALS_DIRECTORY:-}"
        if [ -n "$creds" ]; then
          [ -f "$creds/anthropic_oauth_token" ] && ANTHROPIC_OAUTH_TOKEN=$(cat "$creds/anthropic_oauth_token")
          [ -f "$creds/anthropic_api_key" ]     && ANTHROPIC_API_KEY=$(cat "$creds/anthropic_api_key")
          [ -f "$creds/openai_api_key" ]        && OPENAI_API_KEY=$(cat "$creds/openai_api_key")
          [ -f "$creds/ollama_cloud_api_key" ]  && OLLAMA_CLOUD_API_KEY=$(cat "$creds/ollama_cloud_api_key")
        fi

        exec ${llm-proxy}/bin/llm-proxy
      '';
    };

    # br-microvm is a trustedInterface (host/networking.nix); the Incus
    # network arrives on eth0, so open the port there. The proxy's own
    # allowlist decides who may spend the credentials.
    networking.firewall.interfaces.eth0.allowedTCPPorts = [
      (lib.toInt (lib.last (lib.splitString ":" cfg.listenAddr)))
    ];
  };
}
