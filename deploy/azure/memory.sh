#!/usr/bin/env bash
# The Azure side of institutional memory, on your own install: a private Blob
# container per memory tenant holding that tenant's documents, the documents
# themselves, and the right for the tenant's identity to read its container.
# Idempotent: it uploads only files whose content differs from the blob.
#
#   deploy/azure/memory.sh
#
# Which tenants get a container is read from the profiles in this repo
# (profiles/azure/<tenant>.yaml, memory.enabled: true), never from the cluster.
# Their documents are memory-documents/<tenant>/. A site.yaml without a `memory`
# block turns memory off: this script then does nothing.
#
# Access is Entra ID only (--auth-mode login). No account key, connection
# string or SAS is used, and containers are never public. DRY_RUN=1 prints each
# write instead of running it. PROFILES_DIR and DOCS_DIR override the profiles
# and documents directories (the tests point them at fixtures).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=lib.sh
source "$here/lib.sh"

profiles="${PROFILES_DIR:-$here/../../profiles/azure}"
docs="${DOCS_DIR:-$here/../../memory-documents}"

if ! site_has memory; then
  echo "no memory block in site.yaml: nothing to do"
  exit 0
fi

# Everything is read, and every required value resolved, before the first
# write. These run at the top level (not inside $(...) functions) so that a
# failure stops the script: bash clears errexit inside a command substitution.
acct="$(site_req memory.storageAccount)"
acct_rg="$(site_req memory.resourceGroup)"
prefix="$(site_req identityNamePrefix)"
id_rg="$(site_req identityResourceGroup)"

acct_id="$(azm storage account show -g "$acct_rg" -n "$acct" --query id -o tsv)"
if [ -z "$acct_id" ]; then
  echo "storage account $acct not found in $acct_rg" >&2
  exit 1
fi

# The tenants that read documents: those whose profile turns memory on.
# common.yaml holds shared values, not a tenant.
tenants=()
for f in "$profiles"/*.yaml; do
  t="$(basename "$f" .yaml)"
  [ "$t" = common ] && continue
  want="$(yq e '.memory.enabled == true' "$f")"
  [ "$want" = "true" ] && tenants+=("$t")
done

if [ "${#tenants[@]}" -eq 0 ]; then
  echo "no tenant profile sets memory.enabled: nothing to do"
  exit 0
fi

# Principal ids, read now so a missing identity or documents directory stops
# the script before any write. (${#tenants[@]} guards: an empty array trips
# set -u on bash 3.2.)
principals=()
for ((i = 0; i < ${#tenants[@]}; i++)); do
  t="${tenants[$i]}"
  if [ ! -d "$docs/$t" ]; then
    echo "no documents directory $docs/$t" >&2
    exit 1
  fi
  p="$(azm identity show -g "$id_rg" -n "$prefix-$t" --query principalId -o tsv)"
  if [ -z "$p" ]; then
    echo "identity $prefix-$t not found in $id_rg (run make azure-tenant WS=$t first)" >&2
    exit 1
  fi
  principals+=("$p")
done

# is_not_found <message>: succeed when an az error says the thing is absent.
# Only that answer means "create it"; any other failure (no rights, throttling)
# stops the script rather than writing on a guess.
is_not_found() {
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
    *notfound* | *"not found"* | *"does not exist"*) return 0 ;;
    *) return 1 ;;
  esac
}

# md5_b64 <file>: base64 of the raw MD5, the form Blob stores in contentMd5.
# openssl is on macOS and Linux alike; md5sum and md5 are not.
md5_b64() {
  openssl dgst -md5 -binary "$1" | base64
}

changed=()
for ((i = 0; i < ${#tenants[@]}; i++)); do
  t="${tenants[$i]}"
  c="$t-documents"
  scope="$acct_id/blobServices/default/containers/$c"
  did=0

  # The container, private.
  if ! shown="$(azm storage container show --auth-mode login --account-name "$acct" -n "$c" -o none 2>&1)"; then
    if is_not_found "$shown"; then
      echo "==> creating container $c in $acct"
      run azm storage container create --auth-mode login --account-name "$acct" -n "$c" \
        --public-access off -o none
      did=1
    else
      echo "looking up container $c failed: $shown" >&2
      exit 1
    fi
  fi

  # Uploads: only what differs.
  for file in "$docs/$t"/*; do
    [ -f "$file" ] || continue
    name="$(basename "$file")"
    want="$(md5_b64 "$file")"
    if have="$(azm storage blob show --auth-mode login --account-name "$acct" -c "$c" -n "$name" \
      --query properties.contentSettings.contentMd5 -o tsv 2>&1)"; then
      if [ "$have" = "$want" ]; then
        continue
      fi
    elif ! is_not_found "$have"; then
      echo "looking up blob $c/$name failed: $have" >&2
      exit 1
    fi
    echo "==> uploading $c/$name"
    run azm storage blob upload --auth-mode login --account-name "$acct" -c "$c" -n "$name" \
      -f "$file" --overwrite --content-md5 "$want" --content-type text/markdown -o none
    did=1
  done

  # Deletes: blobs with no file behind them. A container just created in a
  # dry run does not exist to list, so there is nothing to delete either.
  if listed="$(azm storage blob list --auth-mode login --account-name "$acct" -c "$c" \
    --query '[].name' -o tsv 2>&1)"; then
    while IFS= read -r name; do
      [ -n "$name" ] || continue
      if [ ! -f "$docs/$t/$name" ]; then
        echo "==> deleting $c/$name (no such document)"
        run azm storage blob delete --auth-mode login --account-name "$acct" -c "$c" -n "$name"
        did=1
      fi
    done <<<"$listed"
  elif [ "${DRY_RUN:-0}" != "1" ] || ! is_not_found "$listed"; then
    echo "listing container $c failed: $listed" >&2
    exit 1
  fi

  # Storage Blob Data Reader on this container only, never the account.
  held="$(azm role assignment list --assignee "${principals[$i]}" --scope "$scope" \
    --role 'Storage Blob Data Reader' --query '[0].id' -o tsv)"
  if [ -z "$held" ]; then
    echo "==> granting $prefix-$t Storage Blob Data Reader on $c"
    run azm role assignment create --assignee-object-id "${principals[$i]}" \
      --assignee-principal-type ServicePrincipal --role 'Storage Blob Data Reader' \
      --scope "$scope" -o none
    did=1
  fi

  if [ "$did" = 1 ]; then
    changed+=("$t")
  fi
done

# Start one connector run for each tenant whose container or access just
# changed, so the documents reach memory without waiting for the schedule. A
# re-run that changes nothing starts nothing. The CronJob comes from `make
# demos`; before that there is nothing to start, and its schedule picks the
# documents up once it exists.
for ((i = 0; i < ${#changed[@]}; i++)); do
  t="${changed[$i]}"
  if "${KUBECTL[@]}" -n "$t" get cronjob memory-ingest -o name >/dev/null 2>&1; then
    echo "==> starting a memory-ingest run in $t"
    run "${KUBECTL[@]}" -n "$t" create job --from=cronjob/memory-ingest "memory-ingest-$(date +%s)"
  else
    echo "memory-ingest is not installed in $t yet (make demos first); its schedule will ingest the documents"
  fi
done
