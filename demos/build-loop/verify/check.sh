#!/usr/bin/env bash
# verify/ is a test OF the demo, not part of it.
#
# Pins the storyline the RUNBOOK promises, in three stages. Arena can't assert
# an expected failure, so this runs the first two configs and reads the
# per-scenario result from the JSON output:
#   1. seed (config.seed.arena.yaml): data-protection passes,
#      uncapped-indemnity FAILS.
#   2. after the beat 4 fix (config.beat4.arena.yaml): everything passes except
#      quillon-changes, which FAILS (it is beat 7's miss).
#   3. after the beat 7 fix: run config.fixed.arena.yaml directly; everything
#      must pass (arena's own exit code says so).
# Needs OPENAI_API_KEY. Exit 0 when stages 1 and 2 hold.
#
# Usage: bash demos/build-loop/verify/check.sh
set -uo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bad=0

run() { # config name -> out dir
  rm -rf "$here/out-$1"
  # ARENA can point at a wrapper that supplies OPENAI_API_KEY.
  ${ARENA:-promptarena} run --config "$here/config.$1.arena.yaml" --provider openai --out "$here/out-$1" --ci >/dev/null 2>&1 || true
}

status() { # config, scenario id -> pass | fail | missing
  local f
  f="$(ls "$here/out-$1"/*_"$2"_*.json 2>/dev/null | head -n 1)"
  [ -n "$f" ] || { echo missing; return; }
  jq -r 'if (.Error == "" and (.Violations // [] | length) == 0 and (.conversation_assertions.passed // true)) then "pass" else "fail" end' "$f"
}

expect() { # config, scenario, want
  local got
  got="$(status "$1" "$2")"
  printf '%-8s %-24s %-8s (want %s)\n' "$1" "$2" "$got" "$3"
  [ "$got" = "$3" ] || bad=1
}

run seed
expect seed data-protection pass
expect seed uncapped-indemnity fail

run beat4
for s in data-protection uncapped-indemnity brightpost-contract fernside-renewal selfplay-cooperative selfplay-waive-through; do
  expect beat4 "$s" pass
done
expect beat4 quillon-changes fail

if [ "$bad" = 0 ]; then
  echo "OK: the seed misses the indemnity, the beat 4 fix leaves only Quillon's changes."
else
  echo "FAIL: the storyline does not hold." >&2
  exit 1
fi
