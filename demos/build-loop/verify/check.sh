#!/usr/bin/env bash
# verify/ is a test OF the demo, not part of it.
#
# Pins the seed half of the storyline: on the seed prompt, data-protection
# PASSES and uncapped-indemnity FAILS. Arena can't assert an expected failure,
# so this runs config.seed.arena.yaml and reads the per-scenario result from
# the JSON output. Needs OPENAI_API_KEY. Exit 0 when the storyline holds.
#
# Usage: bash demos/build-loop/verify/check.sh
set -uo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
out="$here/out"
rm -rf "$out"

# Arena exits non-zero when a scenario fails, which is the expected case here.
# ARENA can point at a wrapper that supplies OPENAI_API_KEY.
${ARENA:-promptarena} run --config "$here/config.seed.arena.yaml" --provider openai --out "$out" --ci >/dev/null 2>&1 || true

status() { # scenario id -> pass | fail | missing
  local f
  f="$(ls "$out"/*_"$1"_*.json 2>/dev/null | head -n 1)"
  [ -n "$f" ] || { echo missing; return; }
  jq -r 'if (.Error == "" and (.Violations // [] | length) == 0 and (.conversation_assertions.passed // true)) then "pass" else "fail" end' "$f"
}

seed="$(status data-protection)"
stage="$(status uncapped-indemnity)"
echo "data-protection (seed):       $seed   (want pass)"
echo "uncapped-indemnity (stage):   $stage   (want fail)"
if [ "$seed" = pass ] && [ "$stage" = fail ]; then
  echo "OK: the seed misses the indemnity, as the demo needs."
else
  echo "FAIL: the storyline does not hold on the seed prompt." >&2
  exit 1
fi
