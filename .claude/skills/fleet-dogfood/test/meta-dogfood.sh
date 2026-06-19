#!/usr/bin/env bash
# Meta-dogfood: a REAL micro-task (add Queue.Pending()) dispatched through the skill,
# graded by a HIDDEN go-test oracle the worker never sees. Proves the skill reproduces
# the ITER-0000 loop on the real repo with a go-test oracle.
set -uo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"; ROOT="$(cd "$HERE/../../.." && pwd)"

cat > /tmp/df-pending-brief.txt <<'EOF'
In modules/incus-dispatcher/queue/memory.go, add a method:
    func (q *MemoryQueue) Pending() int
that returns the number of PENDING directives (pushed but not yet claimed and not done).
Match the existing locking/style of the other MemoryQueue methods. Do NOT commit.
EOF

# Hidden oracle: writes its own go test into the clean checkout, then runs it.
cat > /tmp/df-pending-oracle.sh <<'EOF'
#!/usr/bin/env bash
set -e
cat > modules/incus-dispatcher/queue/pending_oracle_test.go <<'GO'
package queue
import "testing"
func TestPendingOracle(t *testing.T) {
	q := NewMemoryQueue()
	if q.Pending() != 0 { t.Fatalf("empty: Pending()=%d want 0", q.Pending()) }
	q.Push(Directive{Intent: "a"}); q.Push(Directive{Intent: "b"})
	if q.Pending() != 2 { t.Fatalf("two pushed: Pending()=%d want 2", q.Pending()) }
	if _, _, err := q.Claim("c", 0); err != nil { t.Fatalf("claim: %v", err) }
	if q.Pending() != 1 { t.Fatalf("after claim: Pending()=%d want 1", q.Pending()) }
}
GO
cd modules/incus-dispatcher && go test ./queue/ -run TestPendingOracle
EOF
chmod +x /tmp/df-pending-oracle.sh

FLEET_TOKEN="${FLEET_TOKEN:-$(cat ~/.fleet-token 2>/dev/null)}" \
bash "$HERE/fleet-dogfood.sh" --name pending-meta --brief /tmp/df-pending-brief.txt \
  --repo "$ROOT" --ref HEAD --oracle /tmp/df-pending-oracle.sh --output-dir /tmp/df-pending --timeout 900
rc=$?
echo "=== meta-dogfood exit=$rc ==="; cat /tmp/df-pending/grade.json 2>/dev/null
exit $rc
