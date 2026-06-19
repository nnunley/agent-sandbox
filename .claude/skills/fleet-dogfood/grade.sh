# grade.sh — authoritative grade on a clean checkout.
# Expects DF_OUTDIR, DF_REPO, DF_REF, DF_ORACLE in env. Returns nonzero iff the
# oracle did not pass (anti-reward-hack: runs on a checkout the worker never saw).
_grade() {
  local clean; clean=$(mktemp -d)
  git clone -q "$DF_REPO" "$clean"; ( cd "$clean" && git checkout -q "$DF_REF" )
  local applied=false ec=1
  if [ -s "$DF_OUTDIR/worker.diff" ] && ( cd "$clean" && git apply --whitespace=nowarn "$DF_OUTDIR/worker.diff" 2>/dev/null ); then
    applied=true
    cp "$DF_ORACLE" "$clean/.oracle"; chmod +x "$clean/.oracle"
    ( cd "$clean" && ./.oracle ); ec=$?
  fi
  local pass=false; [ "$applied" = true ] && [ "$ec" -eq 0 ] && pass=true
  printf '{ "patch_applied": %s, "exit_code": %d, "pass": %s }\n' "$applied" "$ec" "$pass" > "$DF_OUTDIR/grade.json"
  rm -rf "$clean"
  [ "$pass" = true ]
}
_grade
