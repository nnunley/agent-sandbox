#!/usr/bin/env bash
# Bounded clean-room regen attempt (STORY-0075 AC-2 = JOURNEY-0003 / STORY-0068 AC-2).
# Launches a fleet-golden copy, delivers let-go @ d4c36cf2d (bundle) + the captured worker fix,
# applies source-only, then runs the oracle gates on the golden's nix-pinned toolchain.
set -uo pipefail
REMOTE=ndn-desktop
REPO=/Users/ndn/development/agent-sandbox
BUNDLE=/tmp/let-go-fixture.bundle
DIFF=$REPO/modules/incus-dispatcher/testdata/journey0003/lvl1-focused.diff
NIXDEV="nix develop /etc/fleet-worker --accept-flake-config --no-sandbox -c bash -lc"

inst="cleanroom-$(date +%s)"
echo "=== launch $inst from fleet-golden ==="
incus launch "$REMOTE:fleet-golden" "$inst" >/dev/null 2>&1 || { echo "FAIL launch"; exit 1; }
for i in $(seq 1 90); do incus exec "$REMOTE:$inst" -- test -S /nix/var/nix/daemon-socket/socket >/dev/null 2>&1 && break; sleep 1; done

echo "=== deliver let-go bundle + worker fix ==="
incus file push "$BUNDLE" "$REMOTE:$inst/root/let-go.bundle" >/dev/null 2>&1
incus file push "$DIFF" "$REMOTE:$inst/root/fix.diff" >/dev/null 2>&1

echo "=== clone + checkout pre-fix ref + apply source-only ==="
incus exec "$REMOTE:$inst" -- bash -lc '
  set -e
  cd /workspace 2>/dev/null || cd /root
  git config --global user.email c@c >/dev/null 2>&1; git config --global user.name c >/dev/null 2>&1
  git clone -q /root/let-go.bundle let-go
  cd let-go && git checkout -q _fixture_base
  echo "HEAD: $(git log --oneline -1)"
  git apply --exclude="pkg/rt/generated.sums" --exclude="*.lgb" /root/fix.diff && echo "apply: source-only OK" || echo "apply: FAILED"
' 2>&1 | grep -vE 'Ignoring unknown'

echo "=== oracle gates on golden toolchain (make generate -> check-generated -> cluster-A) ==="
incus exec "$REMOTE:$inst" -- bash -lc "
  cd /workspace/let-go 2>/dev/null || cd /root/let-go
  $NIXDEV 'set -x; make generate && echo GEN_OK; make check-generated && echo CHECKGEN_BYTE_IDENTICAL; go test -tags gogen_ir ./pkg/ir/ 2>&1 | tail -5' 2>&1 | grep -vE 'Ignoring unknown'
"
rc_marker=$?
echo "=== reap $inst ==="
incus stop --force "$REMOTE:$inst" >/dev/null 2>&1 || true
incus delete "$REMOTE:$inst" >/dev/null 2>&1 || incus delete --force "$REMOTE:$inst" >/dev/null 2>&1 || true
echo "done (marker rc=$rc_marker)"
