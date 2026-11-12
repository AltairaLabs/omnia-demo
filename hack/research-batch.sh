#!/usr/bin/env bash
set -euo pipefail

# Run Halvern's research function over a batch of client matters, one call per
# matter, paced, and count what was served and what the budget refused (demo
# 10).
#
# Usage: RESEARCH_KEY=<client key> hack/research-batch.sh [n] [pause-seconds] [url]
#   n              matters in the batch, default 30
#   pause-seconds  between calls, default 15. Omnia's operator updates a
#                  workspace's spend about every five minutes, so refusals start
#                  up to ~5.5 minutes after the cap is reached: pace the batch
#                  over longer than that, or every call is served first.
#   url            the function, default http://localhost:8895/functions/research
#                  (kubectl --context aks-omnia-demo -n halvern-research
#                   port-forward svc/research 8895:8080)
#
# RESEARCH_KEY is a client key for the `research` agent, minted in the
# dashboard (Agents, research, Connectivity). It is sent as a bearer token and
# never written anywhere.
#
# Prints one line per matter and a final `served=<a> refused=<b> failed=<c>`.
# Exits 0 when every call was either served or refused by the budget.

: "${RESEARCH_KEY:?set RESEARCH_KEY to a client key for the research function}"
n=${1:-30}
pause=${2:-15}
url=${3:-http://localhost:8895/functions/research}
change="The Consumer Duty extended to closed-book products"

served=0 refused=0 failed=0
for i in $(seq 1 "${n}"); do
  body=$(jq -n -c --arg m "M-${i}" --arg c "${change}" '{matter_ref: $m, change: $c}')
  out=$(curl -s -m 60 -w '\n%{http_code}' -X POST -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${RESEARCH_KEY}" -d "${body}" "${url}") || out=$'\n000'
  code=${out##*$'\n'}
  resp=${out%$'\n'*}
  if [[ "${code}" == 2* ]]; then
    served=$((served + 1))
    cost=$(jq -r '.usage.cost_usd // "?"' <<< "${resp}" 2>/dev/null || echo "?")
    echo "M-${i}: served (cost \$${cost})"
  elif [[ "${code}" == 429 && "${resp}" == *budget_exceeded* ]]; then
    refused=$((refused + 1))
    echo "M-${i}: refused, budget exceeded"
  else
    failed=$((failed + 1))
    echo "M-${i}: failed (HTTP ${code}): ${resp:0:200}"
  fi
  [[ "${i}" -lt "${n}" ]] && sleep "${pause}"
done

echo "served=${served} refused=${refused} failed=${failed}"
[[ "${failed}" -eq 0 ]]
