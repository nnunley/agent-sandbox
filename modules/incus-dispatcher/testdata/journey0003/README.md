# JOURNEY-0003 grading fixture (13→0)

Durable capture of the worker fix that the external grader must reproduce (STORY-0068 AC-2).
Captured 2026-06-20 from the (ephemeral) `/tmp/lvl1-focused.diff` produced by the ITER-0000
dogfood, so the proof survives `/tmp` being cleared (the PAR scope review flagged the ephemeral
path as a Critical evidence-durability gap).

## Files
- `lvl1-focused.diff` — the proven worker fix (source-file hunks), 23436 bytes.

## Target + oracle (to reproduce the 13→0 result)
- **Target repo:** `let-go` (external — `/Users/ndn/development/let-go` on the author's box;
  delivered to the worker as a git bundle by the dogfood runner).
- **Target ref:** the *pre-fix* commit where cluster-A had 13 failures. **TODO (pin during
  ITER-0003 implementation):** record the exact SHA. Current let-go HEAD is `23bfd87f1`
  (post-fix, `feat(ir): native-Go lowering … fix cluster-A engine divergence (#249)`); the
  target is the parent state before that fix landed.
- **Oracle gates:** `make generate` → `go test -tags gogen_ir ./pkg/ir/` (cluster-A count, expect
  0, down from 13) → `make check-generated` (exit 0) → untagged `go test ./...` (exit 0).
- **Expected grade JSON:** `{passed: true, clusterA: 0, check_generated: true, untagged_fails: 0, e2e: true}`.

## Seam note
JOURNEY-0003 (reproduce 13→0) is a **cluster/e2e** scenario — it needs the `let-go` checkout +
`make generate` toolchain and is NOT a CI sentinel. STORY-0068 **AC-1** (the generic grader
mechanism + grade-JSON shape) is proven separately in CI against a small synthetic fixture so the
corpus has a durable, always-runnable check independent of the external repo.
