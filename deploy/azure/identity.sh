#!/usr/bin/env bash
# Federate a tenant's workloads to its managed identity (made by tenant.sh), so
# their pods get Azure tokens keylessly. Run it after installing or adding
# agents: it reads the AgentRuntimes from the cluster, so a new agent needs
# nothing else. Idempotent; an agent's pods get their Foundry token once their
# credential exists.
#
#   deploy/azure/identity.sh <tenant>
#
# Rights go only to the ServiceAccounts named below, plus each AgentRuntime's
# <name>-facade. DRY_RUN=1 prints each write instead of running it.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

tenant="${1:-}"
if [ -z "$tenant" ]; then
  echo "usage: identity.sh <tenant>   (make azure-identity WS=<tenant>)" >&2
  exit 1
fi

# shellcheck source=lib.sh
source "$here/lib.sh"

# Everything is read, and every required value resolved, before the first
# write. These run at the top level (not inside $(...) functions) so that a
# failure stops the script: bash clears errexit inside a command substitution.
rg="$(site_req identityResourceGroup)"
aks_rg="$(site_req aks.resourceGroup)"
aks_name="$(site_req aks.name)"
mi="$(site_req identityNamePrefix)-$tenant"

client_id="$(identity_lookup "$rg" "$mi" clientId "$tenant")" || exit 1
tenant_id="$(azm account show --query tenantId -o tsv)"
issuer="$(azm aks show -g "$aks_rg" -n "$aks_name" --query oidcIssuerProfile.issuerUrl -o tsv)"
have="$(azm identity federated-credential list -g "$rg" --identity-name "$mi" --query '[].subject' -o tsv)"
# Credentials made for another issuer (an AKS cluster that was recreated keeps
# its identities, and its old credentials then trust a cluster that is gone).
stale="$(azm identity federated-credential list -g "$rg" --identity-name "$mi" \
  --query "[?issuer!='$issuer'].[name,subject]" -o tsv)"

# Memory is a profile choice: only a tenant whose profile turns it on has the
# memory-ingest ServiceAccount, so only it gets a credential for it (an identity
# holds at most 20). common.yaml's default is off.
profile_dir="${PROFILE_DIR:-$here/../../profiles/azure}"
memory_on=0
if [ -f "$profile_dir/$tenant.yaml" ] && [ "$(yq e '.memory.enabled == true' "$profile_dir/$tenant.yaml")" = "true" ]; then
  memory_on=1
fi

# The agents, as their <name>-facade ServiceAccounts. If this fails the script
# stops here, before anything is created or pruned: treating "could not list"
# as "there are none" would delete every agent's credential. A list that
# succeeds and is empty is real, and prunes all of them.
agents="$("${KUBECTL[@]}" -n "$tenant" get agentruntime -o jsonpath='{range .items[*]}{.metadata.name}-facade{" "}{end}')"

# Stale issuers first: delete this tenant's credentials made for another issuer
# and drop them from the list, so the loop below recreates them on the current
# one. Credentials of other namespaces are not this script's to touch.
sa_prefix="system:serviceaccount:$tenant:"
while IFS=$'\t' read -r cname csubject; do
  [ -n "$cname" ] || continue
  case "$csubject" in "$sa_prefix"*) ;; *) continue ;; esac
  echo "==> replacing $csubject (credential $cname trusts another issuer)"
  run azm identity federated-credential delete -g "$rg" --identity-name "$mi" \
    --name "$cname" --yes --output none </dev/null
  kept=""
  for subject in $have; do
    [ "$subject" = "$csubject" ] || kept="$kept $subject"
  done
  have="$kept"
done <<<"$stale"

# Prune next: an agent's credential outlives the agent (dev agents, deleted
# projects), and an identity holds at most 20 federated credentials, so stale
# ones must go before new ones are created or a full identity could never
# recover. Remove each *-facade subject in this tenant's namespace whose
# AgentRuntime is gone; the fixed SAs below, and subjects of any other
# namespace, are never touched.
for subject in $have; do
  case "$subject" in "$sa_prefix"*) ;; *) continue ;; esac
  sa="${subject#"$sa_prefix"}"
  case "$sa" in *-facade) ;; *) continue ;; esac
  case " $agents " in *" $sa "*) continue ;; esac
  name="$(azm identity federated-credential list -g "$rg" --identity-name "$mi" \
    --query "[?subject=='$subject'].name | [0]" -o tsv)"
  echo "==> pruning $subject (no AgentRuntime ${sa%-facade})"
  run azm identity federated-credential delete -g "$rg" --identity-name "$mi" \
    --name "$name" --yes --output none
done

# eval-worker: the SA the eval worker of a tenant runs as, so an llm_judge
# reaches Foundry keylessly. A credential for an SA that is not there is inert.
# omnia-identity-foundry: the Workspace cloud identity (Omnia #4052); the
# operator mints its token for every agent, run-time ones included. Inert where
# a workspace does not declare it.
# privacy-<tenant>: privacy-api reaches a Key Vault key keylessly (Omnia
# #4466); only used where the install turns that on.
# memory-ingest: the chart's ingest CronJob reads the tenant's Blob container
# keylessly. Only where the tenant's profile turns memory on.
fixed=(arena-worker eval-worker)
if [ "$memory_on" = 1 ]; then fixed+=(memory-ingest); fi
fixed+=("memory-$tenant-default" "privacy-$tenant" omnia-identity-foundry)
for sa in "${fixed[@]}" $agents; do
  subject="system:serviceaccount:$tenant:$sa"
  case " $(echo $have) " in *" $subject "*) continue ;; esac
  echo "==> federating $subject -> $mi"
  run azm identity federated-credential create -g "$rg" --identity-name "$mi" \
    --name "$tenant-$sa" --issuer "$issuer" --subject "$subject" \
    --audience api://AzureADTokenExchange --output none
done

# memory-api is not an agent: its operator-made SA needs the annotation. No SA
# yet (the operator has not made it) means nothing to do; a re-run catches up.
sa_name="memory-$tenant-default"
if current="$("${KUBECTL[@]}" -n "$tenant" get sa "$sa_name" \
  -o jsonpath='{.metadata.annotations.azure\.workload\.identity/client-id}' 2>/dev/null)"; then
  if [ "$current" != "$client_id" ]; then
    run "${KUBECTL[@]}" -n "$tenant" annotate sa "$sa_name" --overwrite \
      azure.workload.identity/client-id="$client_id" azure.workload.identity/tenant-id="$tenant_id"
    # Delete the pods, not a rollout restart: the operator owns the Deployment
    # and reverts the restartedAt annotation, so a restart never rolls it.
    run "${KUBECTL[@]}" -n "$tenant" delete pod -l app.kubernetes.io/component=memory-api --wait=false
    echo "==> $sa_name now on $mi"
  fi
fi
