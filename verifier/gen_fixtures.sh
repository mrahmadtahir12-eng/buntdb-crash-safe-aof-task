#!/usr/bin/env bash
# Generate the V7 fixtures with the unmodified pinned upstream code, and copy
# the pinned go.mod, go.sum and test file that the verifier owns.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
UP="$HERE/../upstream/buntdb"
mkdir -p "$HERE/pinned" "$HERE/fixtures"
cp "$UP/go.mod" "$UP/go.sum" "$UP/buntdb_test.go" "$HERE/pinned/"
( cd "$UP" && git rev-parse HEAD ) > "$HERE/pinned/COMMIT"
echo "pinned $(cat "$HERE/pinned/COMMIT")"
# the verifier module itself must resolve for fixturegen's replace; build the
# clean tree once from upstream so go build of buntdbverify succeeds
mkdir -p "$HERE/_work/candidate"
cp "$UP"/*.go "$HERE/_work/candidate/"; rm -f "$HERE/_work/candidate"/*_test.go
cp "$UP/go.mod" "$UP/go.sum" "$HERE/_work/candidate/"
( cd "$HERE/fixturegen" && go mod tidy && go run . "$HERE/fixtures" )
ls -l "$HERE/fixtures"
