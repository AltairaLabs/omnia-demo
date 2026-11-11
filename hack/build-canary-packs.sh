#!/usr/bin/env bash
set -euo pipefail

# The card-support canary's candidate packs, derived from the compiled 1.0.0
# pack (hack/build-packs.sh pii-chat) rather than compiled from their own Arena
# kit: each is 1.0.0 with one prompt change (demos/pii-chat/canary/v<ver>.jq)
# and the judge they are gated on (eval.jq). Deriving them keeps the diff
# between stable and candidate exactly the change under test.
#
# Usage: hack/build-canary-packs.sh
# Needs: jq.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARENA="${ROOT}/charts/omnia-demo/files/arena"
SRC="${ROOT}/demos/pii-chat/canary"

# The judge's revision, in its eval id. Omnia's arenaEval gate averages every
# result for an eval id it has ever recorded (Omnia #3950), so a changed judge
# needs a new id, or its scores average with the old judge's. Bump it whenever
# eval.jq's criteria change, and the same in pii-chat-canary.yaml's evalDef.
EVAL_REVISION=r5

for ver in 2.0 2.1; do
  out="${ARENA}/pii-chat-pack-v${ver}.json"
  id="identity-before-account-v${ver/./-}-${EVAL_REVISION}"
  jq --arg id "${id}" \
    "$(cat "${SRC}/v${ver}.jq") | $(cat "${SRC}/eval.jq")" \
    "${ARENA}/pii-chat-pack.json" > "${out}"
  if [[ "$(jq -r '.prompts.card_support.system_template' "${out}")" == \
        "$(jq -r '.prompts.card_support.system_template' "${ARENA}/pii-chat-pack.json")" ]]; then
    echo "v${ver}: the prompt change did not apply" >&2
    exit 1
  fi
  echo "==> wrote ${out#"${ROOT}/"} (eval ${id})"
done
