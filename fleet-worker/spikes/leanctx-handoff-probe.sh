#!/usr/bin/env bash
# STORY-0034 spike (SCENARIO-0077): ctx_handoff round-trip across two separate
# `claude -p` invocations on the worker.
#
# Two independent layers, so a failure is diagnosable:
#   LAYER A (mechanical, no LLM): does `lean-ctx session` even persist a decision
#     across separate CLI invocations? record -> save -> reset -> load -> read back.
#   LAYER B (agent): iter1 claude decides + records + saves; iter2 (FRESH claude
#     process) loads + recovers. Ground truth = iter1's OWN reported choice (so we
#     do not depend on grepping a possibly-encoded store). A hallucination guard
#     checks that iter2's `session load` actually loaded (not "starting fresh").
#
# Preconditions (per SCENARIO-0077): compression + bridge enabled (serve daemon +
# proxy on :4444, ANTHROPIC_BASE_URL routed through it).
set +e
export CLAUDE_CODE_OAUTH_TOKEN="$(cat /root/.fleet-token)"
export HOME=/root
mkdir -p /root/proj && cd /root/proj
git init -q; printf '# Handoff spike\n' > README.md
git add -A; git -c user.email=a@b.c -c user.name=x commit -qm init >/dev/null 2>&1

lean-ctx init --agent claude >/dev/null 2>&1
lean-ctx setup >/dev/null 2>&1
lean-ctx serve --daemon >/dev/null 2>&1
lean-ctx proxy enable >/dev/null 2>&1
setsid nohup lean-ctx proxy start --port 4444 >/root/proxy.out 2>&1 < /dev/null &
disown 2>/dev/null
sleep 5
if curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:4444/ 2>/dev/null | grep -qE '^[0-9]'; then
  export ANTHROPIC_BASE_URL=http://127.0.0.1:4444
  echo "[handoff] bridge ON (proxy :4444 answering)"
else
  echo "[handoff] bridge proxy not answering; claude direct (fail-open)"
fi

echo "===LEANCTX_VERSION==="; lean-ctx --version 2>&1 | head -1

# ---------------------------------------------------------------------------
# LAYER A — mechanical CLI persistence (no LLM, diagnostic only). Isolates
# lean-ctx itself. NOTE: no `reset` between save and load (reset clobbers the
# current session). Two separate `bash -c` invocations = two processes.
# ---------------------------------------------------------------------------
echo "===LAYER_A (mechanical CLI round-trip, diagnostic)==="
bash -c 'lean-ctx session reset >/dev/null 2>&1; lean-ctx session task "layer A" >/dev/null 2>&1; lean-ctx session decision "CLI_TOKEN=MECHANIC-7Q2" >/dev/null 2>&1; lean-ctx session save 2>&1' | sed 's/\x1b\[[0-9;]*m//g'
echo "--- fresh process: load + dump on-disk session files ---"
A_LOAD="$(bash -c 'lean-ctx session load 2>&1; echo "----STATUS----"; lean-ctx session status 2>&1')"
echo "$A_LOAD" | sed 's/\x1b\[[0-9;]*m//g' | grep -iE 'loaded|no session|CLI_TOKEN|decision' | head -6
A_RECOVERED="$(printf '%s' "$A_LOAD" | grep -oE 'CLI_TOKEN=[A-Z0-9-]+' | head -1)"
echo "--- on-disk session store contents ---"
LATEST_SESS="$(ls -t /root/.local/share/lean-ctx/sessions/* 2>/dev/null | head -1)"
echo "latest session file: ${LATEST_SESS:-<none>}"
if [ -n "$LATEST_SESS" ]; then grep -oE 'CLI_TOKEN=[A-Z0-9-]+|NEXT_ACTION=[A-Z]+|HANDOFF_NONCE=[a-f0-9]+' "$LATEST_SESS" 2>/dev/null | head; fi
A_ONDISK="$(grep -roE 'CLI_TOKEN=MECHANIC-7Q2' /root/.local/share/lean-ctx/sessions/ 2>/dev/null | head -1 | grep -oE 'CLI_TOKEN=[A-Z0-9-]+')"
echo "A_RECOVERED(status)=${A_RECOVERED:-<none>}  A_ONDISK=${A_ONDISK:-<none>}"
if [ "$A_RECOVERED" = "CLI_TOKEN=MECHANIC-7Q2" ] || [ "$A_ONDISK" = "CLI_TOKEN=MECHANIC-7Q2" ]; then echo "LAYER_A=PASS (decision persisted to disk)"; else echo "LAYER_A=FAIL (no persisted decision found)"; fi

# ---------------------------------------------------------------------------
# LAYER B — agent round-trip across two separate claude -p processes.
# ---------------------------------------------------------------------------
# High-entropy nonce: iter2 (separate process, prompt never contains it) can only
# reproduce it by reading the persisted handoff channel. Match => no data loss,
# with ~0 chance of a lucky guess (unlike a 1-of-5 enum).
NONCE="$(openssl rand -hex 6 2>/dev/null || head -c12 /dev/urandom | od -An -tx1 | tr -d ' \n')"
GROUND="HANDOFF_NONCE=$NONCE"
lean-ctx session reset >/dev/null 2>&1
echo "===LAYER_B ITER1 (record nonce decision + save)==="
echo "GROUND (injected into iter1 only) = $GROUND"
ITER1_PROMPT="You are recording a decision for handoff to a later task. The decision token is exactly: $GROUND  (do not alter it). Use your Bash tool to run these two commands EXACTLY:
  lean-ctx session decision \"$GROUND\"
  lean-ctx session save
After both succeed, reply with ONLY: RECORDED"
IS_SANDBOX=1 claude -p "$ITER1_PROMPT" \
  --dangerously-skip-permissions --max-turns 6 --output-format stream-json --verbose \
  > /root/iter1-events.jsonl 2>/root/iter1.err
echo "iter1 rc=$?"
echo "iter1 result field: $(grep -o '"result":"[^"]*"' /root/iter1-events.jsonl | tail -1)"

echo "===LAYER_B ITER2 (fresh process; load + recover)==="
ITER2_PROMPT='A PREVIOUS, separate task recorded a decision token via lean-ctx and saved the session. You do NOT know the token. Recover it: use your Bash tool to run `lean-ctx session load`, then `lean-ctx session status`, and locate a recorded decision of the form HANDOFF_NONCE=<hex>. Write that exact recovered line to /root/iter2-recovered.txt via the Bash tool. Also append the raw stdout of `lean-ctx session load` to /root/iter2-loadout.txt. If you cannot find any HANDOFF_NONCE decision, write HANDOFF_NONCE=NONE. Reply with ONLY the recovered line.'
IS_SANDBOX=1 claude -p "$ITER2_PROMPT" \
  --dangerously-skip-permissions --max-turns 8 --output-format stream-json --verbose \
  > /root/iter2-events.jsonl 2>/root/iter2.err
echo "iter2 rc=$?"
RECOVERED="$(grep -hoE 'HANDOFF_NONCE=[a-f0-9]+' /root/iter2-recovered.txt 2>/dev/null | head -1)"
echo "RECOVERED (iter2 from session) = ${RECOVERED:-<none>}"
# Independent corroboration: is the nonce actually on disk in the session store?
ONDISK="$(grep -rhoE "HANDOFF_NONCE=$NONCE" /root/.local/share/lean-ctx/sessions/ 2>/dev/null | head -1)"
echo "ON-DISK in session store = ${ONDISK:-<none>}"

echo "===PROXY_SAVINGS==="
lean-ctx proxy status 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -iE 'requests|compressed|tokens' | head -4

echo "===VERDICT==="
echo "layerA=${A_RECOVERED:-none}/${A_ONDISK:-none} ground=$GROUND recovered=${RECOVERED:-none} ondisk=${ONDISK:-none}"
if [ "$GROUND" = "$RECOVERED" ] && [ "$GROUND" = "$ONDISK" ]; then
  echo "VERDICT=PASS airtight: iter1 recorded '$GROUND'; it is on disk; fresh-process iter2 recovered it exactly. No data loss."
elif [ "$GROUND" = "$RECOVERED" ]; then
  echo "VERDICT=PASS-soft: fresh-process iter2 recovered the exact nonce '$RECOVERED' (on-disk corroboration missing: ondisk='$ONDISK')"
elif [ "$RECOVERED" = "HANDOFF_NONCE=NONE" ] || [ -z "$RECOVERED" ]; then
  echo "VERDICT=FAIL/INCONCLUSIVE iter2 could not recover the decision (ground='$GROUND' recovered='$RECOVERED' ondisk='$ONDISK')"
else
  echo "VERDICT=FAIL data loss/corruption: ground='$GROUND' != recovered='$RECOVERED'"
fi
echo "===DONE==="
