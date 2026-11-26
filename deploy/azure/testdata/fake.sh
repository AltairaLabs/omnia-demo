#!/usr/bin/env bash
# Shared body of the fake az, kubectl and helm. Usage: fake.sh <tool> <args...>
#
# - Appends "<tool> <args>" to $FAKE_LOG (every call, in order).
# - Exits 3 when "<tool> <args>" matches the glob $FAKE_FAIL (the call is
#   still logged).
# - Otherwise prints $FAKE_FIXTURES/<key>.json, where key is the args joined
#   with "_" (spaces and "/" turned into "_"); failing that, the first fixture
#   whose basename, read as a glob, matches the key. No match: no output, exit 0.
tool="$1"
shift
line="$tool $*"
[ -n "${FAKE_LOG:-}" ] && printf '%s\n' "$line" >>"$FAKE_LOG"

if [ -n "${FAKE_FAIL:-}" ]; then
  # shellcheck disable=SC2254
  case "$line" in $FAKE_FAIL) exit 3 ;; esac
fi

[ -n "${FAKE_FIXTURES:-}" ] && [ -d "$FAKE_FIXTURES" ] || exit 0
key="$*"
key="${key// /_}"
key="${key//\//_}"
if [ -f "$FAKE_FIXTURES/$key.json" ]; then
  cat "$FAKE_FIXTURES/$key.json"
  exit 0
fi
for f in "$FAKE_FIXTURES"/*.json; do
  [ -e "$f" ] || continue
  pat="$(basename "$f" .json)"
  # shellcheck disable=SC2254
  case "$key" in $pat)
    cat "$f"
    exit 0
    ;;
  esac
done
exit 0
