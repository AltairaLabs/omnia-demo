#!/usr/bin/env bash
# handler.sh: be Larkmere's claims handler for demo 3.
#
#   demos/claims-triage/hack/handler.sh list              the held settlements
#   demos/claims-triage/hack/handler.sh approve <ref_id>  approve one: the held call runs
#   demos/claims-triage/hack/handler.sh reject <ref_id>   reject one: the agent refers it
#
# Decides as the claims-handler ServiceAccount the chart makes, the only kind
# of approver referral-api takes on this install (Omnia #4248: no console
# page, no user approvers). referral-api is ClusterIP only, so this
# port-forwards to it for the one call.
set -euo pipefail

CONTEXT=${CONTEXT:-aks-omnia-demo}
NS=${NS:-larkmere-mutual}
GROUP=${GROUP:-default}
PORT=${PORT:-8907}
SVC=referral-$NS-$GROUP

cmd=${1:-list}
id=${2:-}
case $cmd in
list) ;;
approve | reject) [[ -n $id ]] || { echo "usage: $0 $cmd <referral id>" >&2; exit 2; } ;;
*) echo "usage: $0 list | approve <id> | reject <id>" >&2; exit 2 ;;
esac

token=$(kubectl --context "$CONTEXT" -n "$NS" create token claims-handler --audience omnia-referral-api --duration 10m)

kubectl --context "$CONTEXT" -n "$NS" port-forward "svc/$SVC" "$PORT:8080" >/dev/null 2>&1 &
pf=$!
trap 'kill $pf 2>/dev/null' EXIT
for _ in $(seq 20); do
	curl -s -o /dev/null "http://127.0.0.1:$PORT/healthz" 2>/dev/null && break
	sleep 0.5
done

api=http://127.0.0.1:$PORT/api/v1/referrals
auth=(-H "Authorization: Bearer $token")
case $cmd in
list)
	curl -sf "${auth[@]}" "$api?role=claims-handler" |
		jq -r '(.referrals // .items // .)[] | "\(.id)  \(.state)  \(.tool)  \(.arguments // .args | tostring)  raised \(.createdAt // .raisedAt)"'
	;;
approve | reject)
	curl -sf "${auth[@]}" -X POST -H 'Content-Type: application/json' \
		-d "{\"reason\": \"Claims handler: $cmd from the demo runbook\"}" "$api/$id/$cmd" |
		jq -r '"\(.id) \(.state) by \(.decidedBy.subject // "?")"'
	;;
esac
