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

# One eval id for both versions, and for every run: the arenaEval gate reads
# only this agent's candidate results since its rollout started (Omnia
# #3997), so nothing from an earlier run or the other version counts. It must
# match evalDef in charts/omnia-demo/templates/pii-chat-canary.yaml.
id="promises-outcome"

for ver in 2.0 2.1; do
  out="${ARENA}/pii-chat-pack-v${ver}.json"
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
