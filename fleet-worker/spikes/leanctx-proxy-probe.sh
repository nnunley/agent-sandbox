#!/usr/bin/env bash
# Learn the lean-ctx proxy interface (no model call). Runs inside worker nix develop.
set +e
lean-ctx init --agent claude >/dev/null 2>&1
lean-ctx setup >/dev/null 2>&1
echo "===PROXY_HELP==="
lean-ctx proxy --help 2>&1 | head -50
echo "===PROXY_ENABLE==="
lean-ctx proxy enable 2>&1 | tail -25
echo "===PROXY_START==="
( lean-ctx proxy start --port 4444 >/tmp/proxy.out 2>&1 & ) ; sleep 4
echo "proxy.out:"; sed 's/\x1b\[[0-9;]*m//g' /tmp/proxy.out 2>&1 | head -25
echo "===PROXY_STATUS==="
lean-ctx proxy status 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | head -30
echo "===LISTENING_PORTS==="
( ss -ltnp 2>/dev/null || netstat -ltnp 2>/dev/null ) | grep -E ':4444|:8080|lean' | head
echo "===CONFIG_TOML==="
cat /root/.config/lean-ctx/config.toml 2>/dev/null | head -40 || echo "(no config.toml)"
echo "===PROBE_DONE==="
