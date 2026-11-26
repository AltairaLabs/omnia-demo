#!/usr/bin/env bash
# One tenant's Helm values that come from site.yaml: the values the profiles in
# profiles/azure/ leave out because they belong to your install (your DNS zone,
# Entra groups, Key Vault, sign-in provider, gateway). Writes YAML to stdout, to
# be layered after the profile:
#
#   helm upgrade --install <t> charts/omnia-demo -f profiles/azure/common.yaml \
#     -f profiles/azure/<t>.yaml -f <(deploy/azure/site-values.sh <t>)
#
#   deploy/azure/site-values.sh <tenant>
#
# Every tenant gets the owner groups. The Poppy Companies, Ellen's assistant and
# the Halvern catalogue also get the hosts built from dns.zone and the rest of
# what only they use. A required field that is missing stops the script before
# it prints anything.
#
# Assumes Azure's public cloud: the Key Vault URL is https://<name>.vault.azure.net/.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

tenant="${1:-}"
if [ -z "$tenant" ]; then
  echo "usage: site-values.sh <tenant>" >&2
  exit 1
fi

# shellcheck source=lib.sh
source "$here/lib.sh"

# Required values are resolved here, at the top level: inside $(...) helpers
# bash clears errexit, and a missing field would go unnoticed.
if [ "$(yq e '.ownerGroups | length' "$SITE")" -eq 0 ]; then
  echo "site.yaml: ownerGroups is required" >&2
  exit 1
fi

zone=""
case "$tenant" in
  larkmere-mutual | wrenmoor-energy | ellens-assistant) zone="$(site_req dns.zone)" ;;
esac

# The company's short name is its tenant's first word (larkmere, wrenmoor): the
# same names hosts.sh builds the DNS records from.
company="${tenant%%-*}"
case "$tenant" in
  larkmere-mutual | wrenmoor-energy)
    upstream_issuer="$(site_req poppy.upstream.issuer)"
    client_id="$(site_req "poppy.companies.$tenant.clientID")"
    vault="$(site_req keyVault.name)"
    ;;
  halvern-partners)
    dashboard="$(site_req dashboardHost)"
    ;;
esac
# The Gateway every public route attaches to. sectionName is optional.
gw_name="" gw_ns="" gw_section=""
case "$tenant" in
  larkmere-mutual | wrenmoor-energy | ellens-assistant | halvern-partners)
    gw_name="$(site_req gateway.name)"
    gw_ns="$(site_req gateway.namespace)"
    gw_section="$(site_get gateway.sectionName)"
    ;;
esac

yq e -n '.workspace.roleBindings = [{"groups": load(strenv(SITE)).ownerGroups, "role": "owner"}]'

case "$tenant" in
  larkmere-mutual | wrenmoor-energy)
    DOMAIN="$company.$zone" ASSISTANT="https://assistant.$zone/agent.json" \
      ISSUER="https://auth.$company.$zone" UPSTREAM="$upstream_issuer" CLIENT_ID="$client_id" \
      VAULT="https://$vault.vault.azure.net/" GW_NAME="$gw_name" GW_NS="$gw_ns" GW_SECTION="$gw_section" \
      yq e -n '
        .poppy.domain = strenv(DOMAIN) |
        .poppy.allow = [strenv(ASSISTANT)] |
        .poppy.auth.issuer = strenv(ISSUER) |
        .poppy.auth.gateway.upstream.issuer = strenv(UPSTREAM) |
        .poppy.auth.gateway.upstream.clientID = strenv(CLIENT_ID) |
        .poppy.auth.accessTokens.audiences = ["https://" + strenv(DOMAIN)] |
        .poppy.account.audience = "https://" + strenv(DOMAIN) + "/api" |
        .poppy.account.encryption.vaultURL = strenv(VAULT) |
        .poppy.gatewayName = strenv(GW_NAME) |
        .poppy.gatewayNamespace = strenv(GW_NS) |
        (select(strenv(GW_SECTION) != "") | .poppy.gatewaySectionName) = strenv(GW_SECTION)'
    ;;
  ellens-assistant)
    HOST="assistant.$zone" LARKMERE="larkmere.$zone" WRENMOOR="wrenmoor.$zone" \
      GW_NAME="$gw_name" GW_NS="$gw_ns" GW_SECTION="$gw_section" \
      yq e -n '
        .personalAgent.host = strenv(HOST) |
        .personalAgent.knownCompanies = [strenv(LARKMERE), strenv(WRENMOOR)] |
        .poppy.gatewayName = strenv(GW_NAME) |
        .poppy.gatewayNamespace = strenv(GW_NS) |
        (select(strenv(GW_SECTION) != "") | .poppy.gatewaySectionName) = strenv(GW_SECTION)'
    ;;
  halvern-partners)
    GW_NAME="$gw_name" GW_NS="$gw_ns" GW_SECTION="$gw_section" DASHBOARD="$dashboard" \
      yq e -n '
        .catalogue.route.parentRefs = [{"name": strenv(GW_NAME), "namespace": strenv(GW_NS)}] |
        (select(strenv(GW_SECTION) != "") | .catalogue.route.parentRefs[0].sectionName) = strenv(GW_SECTION) |
        .catalogue.route.hostnames = [strenv(DASHBOARD)]'
    ;;
esac
