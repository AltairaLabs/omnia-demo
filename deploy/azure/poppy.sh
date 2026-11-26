#!/usr/bin/env bash
# The Azure side of the Poppy demo, on your own install: the DNS records for
# its hosts, the Key Vault key the account tools seal exchanged tokens with,
# and the right to use that key for each tenant that needs it. Idempotent.
#
#   deploy/azure/poppy.sh
#
# It prints the vault URL to put in a profile's poppy.account.encryption.vaultURL.
# Which tenants get the key is read from the profiles in this repo
# (profiles/azure/<tenant>.yaml), never from the cluster. DRY_RUN=1 prints each
# write instead of running it. PROFILES_DIR overrides the profiles directory
# (the tests point it at fixtures).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=lib.sh
source "$here/lib.sh"
# shellcheck source=hosts.sh
source "$here/hosts.sh"

profiles="${PROFILES_DIR:-$here/../../profiles/azure}"

# Everything is read, and every required value resolved, before the first
# write. These run at the top level (not inside $(...) functions) so that a
# failure stops the script: bash clears errexit inside a command substitution.
dns_sub="$(site_req dns.subscription)"
dns_rg="$(site_req dns.resourceGroup)"
zone="$(site_req dns.zone)"
kv_name="$(site_req keyVault.name)"
prefix="$(site_req identityNamePrefix)"
id_rg="$(site_req identityResourceGroup)"

# DNS lives in its own subscription, which may not be the install's.
DNS_AZ=(az --subscription "$dns_sub")

# The gateway's address: given directly, or read from its public IP resource.
if site_has dns.gatewayIP; then
  gateway_ip="$(site_get dns.gatewayIP)"
elif site_has dns.gatewayPublicIP.name; then
  pip_rg="$(site_req dns.gatewayPublicIP.resourceGroup)"
  pip_name="$(site_req dns.gatewayPublicIP.name)"
  gateway_ip="$("${AZ[@]}" network public-ip show -g "$pip_rg" -n "$pip_name" --query ipAddress -o tsv)"
else
  site_req dns.gatewayIP
fi
if [ -z "$gateway_ip" ]; then
  echo "gateway public IP has no address yet" >&2
  exit 1
fi

vault_id="$("${AZ[@]}" keyvault show --name "$kv_name" --query id -o tsv)"
if [ -z "$vault_id" ]; then
  echo "key vault $kv_name not found" >&2
  exit 1
fi
key_scope="$vault_id/keys/omnia-connections"

# The tenants that seal exchanged tokens under this key: those whose profile
# turns Poppy and its sign-in on and names the Key Vault as the key provider.
# common.yaml holds shared values, not a tenant.
tenants=()
for f in "$profiles"/*.yaml; do
  t="$(basename "$f" .yaml)"
  [ "$t" = common ] && continue
  want="$(yq e '.poppy.enabled == true and .poppy.auth.enabled == true and .poppy.account.encryption.kmsProvider == "azure-keyvault"' "$f")"
  [ "$want" = "true" ] && tenants+=("$t")
done

# Principal ids, read now so a missing identity stops the script before any write.
principals=()
# (${#tenants[@]} guards below: an empty array trips set -u on bash 3.2.)
for ((i = 0; i < ${#tenants[@]}; i++)); do
  t="${tenants[$i]}"
  p="$("${AZ[@]}" identity show -g "$id_rg" -n "$prefix-$t" --query principalId -o tsv)"
  if [ -z "$p" ]; then
    echo "identity $prefix-$t not found in $id_rg (run make azure-tenant WS=$t first)" >&2
    exit 1
  fi
  principals+=("$p")
done

# DNS: point each host at the gateway. A record set is replaced, not edited:
# delete it (it may not exist yet) and add the address.
for h in $(poppy_dns_names); do
  run "${DNS_AZ[@]}" network dns record-set a delete -g "$dns_rg" -z "$zone" -n "$h" --yes 2>/dev/null || true
  run "${DNS_AZ[@]}" network dns record-set a add-record -g "$dns_rg" -z "$zone" -n "$h" \
    -a "$gateway_ip" --ttl 300 --output none
  echo "==> $h.$zone -> $gateway_ip"
done

# The key goes in through ARM (Microsoft.KeyVault/vaults/keys), not the vault's
# own key API: the vault is RBAC-only and the person running this holds no
# data-plane key role. ARM creates a key only if it is absent and never returns
# key material.
if ! "${AZ[@]}" resource show --ids "$key_scope" -o none 2>/dev/null; then
  echo "==> creating key omnia-connections in $kv_name"
  run "${AZ[@]}" rest --method put --url "https://management.azure.com$key_scope?api-version=2023-07-01" \
    --body '{"properties":{"kty":"RSA","keySize":3072,"keyOps":["wrapKey","unwrapKey"]}}' -o none
fi

# Key Vault Crypto User on the key, to the named tenant identities only.
for ((i = 0; i < ${#tenants[@]}; i++)); do
  mi="$prefix-${tenants[$i]}"
  held="$("${AZ[@]}" role assignment list --assignee "${principals[$i]}" --scope "$key_scope" \
    --role 'Key Vault Crypto User' --query '[0].id' -o tsv)"
  if [ -z "$held" ]; then
    echo "==> granting $mi Key Vault Crypto User on omnia-connections"
    run "${AZ[@]}" role assignment create --assignee-object-id "${principals[$i]}" \
      --assignee-principal-type ServicePrincipal --role 'Key Vault Crypto User' \
      --scope "$key_scope" -o none
  fi
done

echo "==> vault-url https://$kv_name.vault.azure.net/ (a profile's poppy.account.encryption.vaultURL)"
