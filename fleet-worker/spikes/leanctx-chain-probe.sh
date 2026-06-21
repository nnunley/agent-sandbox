#!/usr/bin/env bash
# Chain spike: Claude (Bearer OAuth) -> lean-ctx proxy (:4444, compress+measure) -> api.anthropic.com.
# Tests whether the proxy transparently forwards the OAuth Bearer token AND measures savings (gain ON).
set +e
export CLAUDE_CODE_OAUTH_TOKEN="$(cat /root/.fleet-token)"
mkdir -p /root/proj && cd /root/proj
git init -q; printf '# Test\nhello world from spike\n' > README.md
# A large, compressible tool_result source (verbose, repetitive — like real build/log output).
for i in $(seq 1 4000); do echo "line $i: lorem ipsum dolor sit amet consectetur adipiscing elit $i"; done > big.txt
git add -A; git -c user.email=a@b.c -c user.name=x commit -qm init >/dev/null 2>&1

lean-ctx init --agent claude >/dev/null 2>&1
lean-ctx setup >/dev/null 2>&1
lean-ctx proxy enable >/dev/null 2>&1
setsid nohup lean-ctx proxy start --port 4444 >/root/proxy.out 2>&1 < /dev/null &
disown 2>/dev/null
sleep 5
echo "===PROXY_OUT (startup)==="; sed 's/\x1b\[[0-9;]*m//g' /root/proxy.out 2>&1 | head -15
echo "===LISTENING==="; ss -ltn 2>/dev/null | grep 4444 || echo "NOT LISTENING on 4444"
echo "===HEALTHCHECK==="; curl -sS -o /dev/null -w "connect http=%{http_code}\n" http://127.0.0.1:4444/ 2>&1 | head -3
echo "===PSTATUS_BEFORE==="
lean-ctx proxy status 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -iE 'process|port|requests|compressed|tokens|compression'

echo "===CLAUDE_RUN (ANTHROPIC_BASE_URL -> proxy, Bearer OAuth)==="
export ANTHROPIC_BASE_URL=http://127.0.0.1:4444
IS_SANDBOX=1 claude -p "Use your Bash tool to run: cat big.txt   (it is large). After you see the output, reply with just DONE." \
  --dangerously-skip-permissions --max-turns 5 --output-format stream-json --verbose \
  > /root/spike-events.jsonl 2>/root/spike-claude.err
echo "claude rc=$?"
echo "events lines: $(wc -l < /root/spike-events.jsonl)"
echo "result line:"; grep -o '"type":"result"[^}]*' /root/spike-events.jsonl | head -1
echo "err tail:"; tail -4 /root/spike-claude.err | sed 's/\x1b\[[0-9;]*m//g'

echo "===LISTENING_AFTER==="; ss -ltn 2>/dev/null | grep 4444 || echo "NOT LISTENING on 4444 (proxy died)"
echo "===PROXY_OUT (post-run)==="; sed 's/\x1b\[[0-9;]*m//g' /root/proxy.out 2>&1 | tail -20
echo "===PSTATUS_AFTER==="
lean-ctx proxy status 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -iE 'requests|compressed|tokens|compression'
echo "===GAIN==="
lean-ctx gain 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | tail -16
echo "===DONE==="
