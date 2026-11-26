#!/usr/bin/env bash
# The Azure side of one tenant's Omnia workspace: its managed identity, its
# Foundry (and voice) access, and the omnia-workspace chart (namespace,
# databases, Providers, volume). Generic: it knows nothing about what will run
# in the workspace. Whatever installs into it reads the omnia-workspace
# ConfigMap for the identity to put on each agent.
#
#   deploy/azure/tenant.sh <tenant>            create or update
#   deploy/azure/tenant.sh --remove <tenant>   helm uninstall; the databases,
#                                              volume content and identity stay
#
# Every step checks first, so a re-run changes nothing. DRY_RUN=1 prints each
# write instead of running it.
#
# Identity: one managed identity per tenant, with Foundry access. Every agent
# runs as its own <agent>-facade ServiceAccount; AKS's issuer takes no wildcard
# subject and an identity holds at most 20 federated credentials, so each
# tenant gets its own identity and one credential per agent (a later step).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mode=install
if [ "${1:-}" = "--remove" ] || [ "${1:-}" = "--values" ]; then
  mode="${1#--}"
  shift
fi
tenant="${1:-}"
if [ -z "$tenant" ]; then
  echo "usage: tenant.sh [--remove] <tenant>   (make azure-tenant WS=<tenant>)" >&2
  exit 1
fi

# shellcheck source=lib.sh
source "$here/lib.sh"

omnia_ns="$(site_req omnia.namespace)"
release="workspace-$tenant"

# values_file <clientId> <tenantId>: the chart's values for this tenant, from
# site.yaml. Only what the install sets is written; the rest is the chart's.
values_file() {
  local f
  f="$(mktemp)"
  yq -n '{}' > "$f" || exit 1
  # Called inside $(...), where errexit is off: every step exits explicitly, and
  # the caller does `|| exit 1`, so a missing field never reaches helm.
  set_val() { K="$1" V="$2" yq -i 'eval(strenv(K)) = strenv(V)' "$f" || exit 1; }
  set_req() { local v; v="$(site_req "$2")" || exit 1; set_val "$1" "$v"; }
  set_val .name "$tenant"
  set_req .postgres.host postgres.host
  set_req .postgres.adminLogin postgres.adminLogin
  # The admin password: an External Secrets store, or a plain Secret.
  if site_has postgres.adminPassword.externalSecret; then
    set_req .postgres.password.externalSecret.store postgres.adminPassword.externalSecret.clusterSecretStore
    set_req .postgres.password.externalSecret.key postgres.adminPassword.externalSecret.key
    # The databases Job still reads the admin password from a Secret in the
    # Omnia namespace (the chart's dbBootstrap.adminSecret, key PGPASSWORD);
    # unset, the chart's default name stands.
    if site_has postgres.adminPassword.externalSecret.dbAdminSecret; then
      set_req .dbBootstrap.adminSecret postgres.adminPassword.externalSecret.dbAdminSecret
    fi
  elif site_has postgres.adminPassword.secret; then
    set_req .postgres.password.secretRef.name postgres.adminPassword.secret.name
    set_req .postgres.password.secretRef.key postgres.adminPassword.secret.key
  fi
  set_req .foundry.endpoint foundry.endpoint
  set_req .foundry.region foundry.region
  if site_has foundry.providers; then
    yq -i '.foundry.providers = load(strenv(SITE)).foundry.providers' "$f" || exit 1
  fi
  if site_has voice; then
    set_req .voice.endpoint voice.endpoint
    set_req .voice.region voice.region
    if site_has voice.providers; then
      yq -i '.voice.providers = load(strenv(SITE)).voice.providers' "$f" || exit 1
    fi
  fi
  # The chart's volume is on by default; without an nfs block there is none.
  if site_has nfs; then
    yq -i '.nfs.enabled = true' "$f" || exit 1
    set_req .nfs.storageAccount nfs.storageAccount
    for k in shareName storageClass size rootClaim; do
      if site_has "nfs.$k"; then set_req ".nfs.$k" "nfs.$k"; fi
    done
  else
    yq -i '.nfs.enabled = false' "$f" || exit 1
  fi
  set_val .identity.clientId "$1"
  set_val .identity.tenantId "$2"
  echo "$f"
}

if [ "$mode" = "values" ]; then
  # A seam for tests and for diffing against another install: print the values
  # for <tenant> <clientId> <tenantId>.
  f="$(values_file "${2:?clientId}" "${3:?tenantId}")" || exit 1
  cat "$f"
  rm -f "$f"
  exit 0
fi

if [ "$mode" = "remove" ]; then
  run "${HELM[@]}" uninstall "$release" -n "$omnia_ns"
  exit 0
fi

rg="$(site_req identityResourceGroup)"
location="$(site_req location)"
mi="$(site_req identityNamePrefix)-$tenant"
foundry_id="$(site_req foundry.accountId)"
voice_id=""
if site_has voice; then voice_id="$(site_req voice.accountId)"; fi
# Fail on a missing field before anything is created: the values are built
# only after the identity exists, so check them now (the file is discarded).
probe="$(values_file placeholder placeholder)" || exit 1
rm -f "$probe"

echo "==> tenant $tenant: identity $mi"
ids_query='{clientId:clientId,principalId:principalId}'
# Only a not-found answer means "create it"; any other failure (auth, wrong
# subscription, network) stops the script before a write.
show_err="$(mktemp)"
if ids="$(azm identity show -g "$rg" -n "$mi" --query "$ids_query" -o json 2>"$show_err")"; then
  :
else
  shown="$(cat "$show_err")"
  case "$(printf '%s' "$shown" | tr '[:upper:]' '[:lower:]')" in
    *resourcenotfound* | *"not found"* | *"not exist"* | *notfound*) ids="" ;;
    *)
      echo "looking up identity $mi failed: $shown" >&2
      rm -f "$show_err"
      exit 1
      ;;
  esac
fi
rm -f "$show_err"
if [ -n "$ids" ]; then
  :
elif [ "${DRY_RUN:-0}" = "1" ]; then
  # Nothing was created, so there are no ids; placeholders keep the plan whole.
  run azm identity create -g "$rg" -n "$mi" -l "$location" -o none
  ids='{"clientId":"<client-id>","principalId":"<principal-id>"}'
else
  ids="$(azm identity create -g "$rg" -n "$mi" -l "$location" --query "$ids_query" -o json)"
fi
client_id="$(printf '%s' "$ids" | yq e '.clientId' -)"
principal_id="$(printf '%s' "$ids" | yq e '.principalId' -)"
tenant_id="$(azm account show --query tenantId -o tsv)"

# grant <accountId>: Cognitive Services OpenAI User, if the identity lacks it.
grant() {
  local role='Cognitive Services OpenAI User'
  if [ -z "$(azm role assignment list --assignee "$principal_id" --scope "$1" --role "$role" --query '[0].id' -o tsv)" ]; then
    echo "==> granting $mi $role on ${1##*/}"
    run azm role assignment create --assignee-object-id "$principal_id" \
      --assignee-principal-type ServicePrincipal --role "$role" --scope "$1" -o none
  fi
}
grant "$foundry_id"
if [ -n "$voice_id" ]; then
  grant "$voice_id"
fi

echo "==> tenant $tenant: namespace, databases, Providers, volume"
vals="$(values_file "$client_id" "$tenant_id")" || exit 1
trap 'rm -f "$vals"' EXIT
run "${HELM[@]}" upgrade --install "$release" "$here/../../charts/omnia-workspace" \
  -n "$omnia_ns" -f "$vals" --wait --timeout 5m

# Enrol the namespace in the ambient mesh BEFORE anything is installed into it,
# so every pod starts captured. Restarting function agents afterwards is not
# safe: a pod replaced mid-report trips Omnia's capability gate (Omnia #3877).
# No ztunnel means the cluster is not ambient, and there is nothing to enrol.
if "${KUBECTL[@]}" -n istio-system get daemonset ztunnel >/dev/null 2>&1; then
  run "${KUBECTL[@]}" label ns "$tenant" istio.io/dataplane-mode=ambient istio.io/use-waypoint=waypoint --overwrite
  # Held in a variable, not piped: a DRY_RUN `run` never reads stdin, and the
  # sed would die of SIGPIPE under pipefail.
  waypoint="$(sed -e "s|__WORKSPACE_NS__|$tenant|g" -e "s|__OMNIA_NS__|$omnia_ns|g" "$here/ambient-waypoint.yaml")"
  run "${KUBECTL[@]}" apply -f - <<<"$waypoint"
fi

echo "==> tenant $tenant ready (omnia-workspace ConfigMap: client id $client_id)"
