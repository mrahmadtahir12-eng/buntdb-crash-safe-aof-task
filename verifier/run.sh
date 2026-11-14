#!/usr/bin/env bash
# Run every check against a candidate checkout of buntdb.
#
#   ./run.sh <candidate dir>            full run
#   QUICK=1 ./run.sh <candidate dir>    fewer kill runs and a smaller oversized value
#
# The candidate is never compiled in place. A clean tree is built from its
# non-test .go files plus the pinned go.mod and go.sum; candidate _test.go
# files are ignored. The seed is chosen here unless BUNTDB_VERIFY_SEED is set.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
CAND="$(cd "${1:?usage: run.sh <candidate dir>}" && pwd)"
WORK="$HERE/_work"
rm -rf "$WORK"
mkdir -p "$WORK/candidate" "$WORK/upstream_suite" "$WORK/results"

echo "== clean tree from $CAND"
for f in "$CAND"/*.go; do
  case "$f" in *_test.go) ;; *) cp "$f" "$WORK/candidate/";; esac
done
cp "$HERE/pinned/go.mod" "$HERE/pinned/go.sum" "$WORK/candidate/"
ls "$WORK/candidate"

echo "== screen: branching on the test environment (a screen, not proof)"
grep -nE 'os\.Getenv|os\.LookupEnv|testing\.Testing|runtime\.Caller|flag\.Lookup\("test' "$WORK/candidate"/*.go \
  | tee "$WORK/results/screen.txt" || echo "(nothing found)"

echo "== V11 surface: build the verifier (api_surface.go must compile)"
( cd "$HERE" && go vet ./... ) 2>&1 | tee "$WORK/results/vet.txt" || true
( cd "$HERE" && go build ./... ) 2>&1 | tee "$WORK/results/build.txt"

echo "== V1b: upstream test suite from the pinned commit against the candidate sources"
cp "$WORK/candidate"/* "$WORK/upstream_suite/"
cp "$HERE/pinned/buntdb_test.go" "$WORK/upstream_suite/"
( cd "$WORK/upstream_suite" && go test -count=1 -timeout 30m . ) 2>&1 | tee "$WORK/results/upstream_suite.txt" || true

echo "== V7 fixtures present?"
ls "$HERE/fixtures"/*.db >/dev/null 2>&1 || { echo "no fixtures; run gen_fixtures.sh first"; exit 1; }

# RACE="" turns the race detector off (needed on Windows without gcc).
RACE="${RACE--race}"
if [ "${QUICK:-0}" = "1" ]; then
  export BUNTDB_VERIFY_BIG=${BUNTDB_VERIFY_BIG:-70000}
  export BUNTDB_VERIFY_KILL_RUNS=${BUNTDB_VERIFY_KILL_RUNS:-20}
  export BUNTDB_VERIFY_SHRINK_KILL_RUNS=${BUNTDB_VERIFY_SHRINK_KILL_RUNS:-15}
  export BUNTDB_VERIFY_SHRINK_ROUNDS=${BUNTDB_VERIFY_SHRINK_ROUNDS:-10}
  export BUNTDB_VERIFY_BIGDB_MB=${BUNTDB_VERIFY_BIGDB_MB:-10}
fi
export BUNTDB_VERIFY_SEED="${BUNTDB_VERIFY_SEED:-$(( $(date +%s%N) % 1000000007 ))}"
export BUNTDB_VERIFY_CANDIDATE="$CAND"
echo "== harness, seed $BUNTDB_VERIFY_SEED"
( cd "$HERE" && go test $RACE -count=1 -timeout 3h -v . ) 2>&1 | tee "$WORK/results/harness.txt" || true

echo "== summary"
grep -E '^(--- |ok|FAIL|PASS)' "$WORK/results/harness.txt" | sed 's/^/   /'
grep -E '^(ok|FAIL)' "$WORK/results/upstream_suite.txt" | sed 's/^   upstream suite: /   /'
echo "results in $WORK/results"
