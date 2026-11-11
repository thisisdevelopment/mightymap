#!/bin/bash
# Fuzzes every fuzz target for FUZZ_TIME, one after the other.
#
#   ./fuzz.sh                  all targets, 30 seconds each
#   ./fuzz.sh Operations       only targets with Operations in their name
#   FUZZ_TIME=5m ./fuzz.sh     longer
#
# It stops at the first target that fails. Go writes the failing input to
# testdata/fuzz/<target>/ in the package of the target: fix the bug and commit
# that file with the fix, every "go test" then runs it again.
set -u

FUZZ_TIME="${FUZZ_TIME:-30s}"
filter="${1:-}"

for package in $(go list ./...); do
	for target in $(go test -list '^Fuzz' "$package" 2>/dev/null | grep '^Fuzz' | grep -- "$filter"); do
		echo "== $target ($package), $FUZZ_TIME"
		if ! go test -run '^$' -fuzz "^${target}\$" -fuzztime "$FUZZ_TIME" "$package"; then
			echo "FAILED: $target"
			echo "replay: go test -run '^${target}\$' $package"
			exit 1
		fi
	done
done
echo "all fuzz targets passed"
