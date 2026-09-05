#!/usr/bin/env bash
#
# Run a fuzzing campaign across every parser.
#
# The seed corpus — including every input that has ever crashed one of these —
# runs automatically as part of `go test ./...`, so a fixed bug stays fixed
# without anyone remembering to do anything. This script is the other half: it
# goes looking for the next one.
#
# Usage:
#   scripts/fuzz.sh [seconds-per-target]    # default 30
#
set -euo pipefail

seconds=${1:-30}
cd "$(cd "$(dirname "$0")/.." && pwd)"

# Every fuzz target in the tree, found rather than listed, so a new one is
# picked up by writing it and nothing else.
targets=$(grep -rn "^func Fuzz" --include='*_test.go' internal/ \
  | sed -E 's|^(.*)/[^/]+_test\.go:[0-9]+:func (Fuzz[A-Za-z0-9_]*).*$|\1 \2|' \
  | sort)

failed=0
count=0
while read -r pkg target; do
  [ -z "$target" ] && continue
  count=$((count + 1))
  printf '%-24s %s ... ' "${pkg#internal/}" "$target"

  if go test "./$pkg/" -run "^$target\$" -fuzz "^$target\$" \
      -fuzztime="${seconds}s" > /tmp/fuzz-$$.log 2>&1; then
    echo "ok"
  else
    echo "FAILED"
    # The crashing input is written into the package's testdata, where it
    # becomes a permanent regression case. Print what happened, keep going:
    # one broken parser should not hide the next one.
    grep -A8 "FAIL:" /tmp/fuzz-$$.log | head -12 || true
    failed=$((failed + 1))
  fi
done <<< "$targets"

rm -f /tmp/fuzz-$$.log

echo
if [ "$failed" -eq 0 ]; then
  echo "$count targets, ${seconds}s each, no crashes."
  exit 0
fi
echo "$failed of $count targets crashed. The inputs are in their package's"
echo "testdata/fuzz directory — commit them with the fix so they stay tested."
exit 1
