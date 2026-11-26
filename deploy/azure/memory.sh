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
# Their documents are memory-documents/<tenant>/, subfolders included: a blob's
# name is the file's path under that directory (member-guidance/x.md). A site.yaml without a `memory`
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
#
# list_docs <dir>: the documents under dir, one path relative to dir per line
# (member-guidance/x.md). A blob's name is that relative path. Dotfiles and
# anything under a dot-directory are not documents.
list_docs() {
  (cd "$1" && find . -type f ! -name '.*' ! -path '*/.*' | sed 's|^\./||' | sort)
}

principals=()
for ((i = 0; i < ${#tenants[@]}; i++)); do
  t="${tenants[$i]}"
  if [ ! -d "$docs/$t" ]; then
    echo "no documents directory $docs/$t" >&2
    exit 1
  fi
  # An empty documents directory (or a wrong DOCS_DIR) would make every blob
  # look removed and delete them all: refuse before any write.
  nfiles=0
  while IFS= read -r f; do
    [ -n "$f" ] && nfiles=$((nfiles + 1))
  done < <(list_docs "$docs/$t")
  if [ "$nfiles" -eq 0 ]; then
    echo "no documents in $docs/$t: refusing to seed (it would delete every blob in $t-documents)" >&2
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

# stderr of the reads whose stdout is compared or parsed goes to a file: an az
# warning mixed into stdout would corrupt the MD5 compare or a blob name.
errf="$(mktemp)"
trap 'rm -f "$errf"' EXIT

changed=""
granted=""
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

  # Uploads: only what differs. The names are read into an array first so no
  # az call can swallow the list from stdin.
  names=()
  while IFS= read -r name; do
    [ -n "$name" ] && names+=("$name")
  done < <(list_docs "$docs/$t")
  for name in "${names[@]}"; do
    file="$docs/$t/$name"
    want="$(md5_b64 "$file")"
    if have="$(azm storage blob show --auth-mode login --account-name "$acct" -c "$c" -n "$name" \
      --query properties.contentSettings.contentMd5 -o tsv 2>"$errf")"; then
      if [ "$have" = "$want" ]; then
        continue
      fi
    else
      err="$(cat "$errf")"
      if ! is_not_found "$err"; then
        echo "looking up blob $c/$name failed: $err" >&2
        exit 1
      fi
    fi
    echo "==> uploading $c/$name"
    run azm storage blob upload --auth-mode login --account-name "$acct" -c "$c" -n "$name" \
      -f "$file" --overwrite --content-md5 "$want" --content-type text/markdown -o none
    did=1
  done

  # Deletes: blobs with no file behind them. A container just created in a
  # dry run does not exist to list, so there is nothing to delete either.
  if listed="$(azm storage blob list --auth-mode login --account-name "$acct" -c "$c" \
    --query '[].name' -o tsv 2>"$errf")"; then
    while IFS= read -r name; do
      [ -n "$name" ] || continue
      if [ ! -f "$docs/$t/$name" ]; then
        echo "==> deleting $c/$name (no such document)"
        run azm storage blob delete --auth-mode login --account-name "$acct" -c "$c" -n "$name" </dev/null
        did=1
      fi
    done <<<"$listed"
  else
    err="$(cat "$errf")"
    if [ "${DRY_RUN:-0}" != "1" ] || ! is_not_found "$err"; then
      echo "listing container $c failed: $err" >&2
      exit 1
    fi
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
    granted="$granted $t "
  fi

  if [ "$did" = 1 ]; then
    changed="$changed $t "
  fi
done

# Start one connector run for a tenant when its container or documents changed,
# or when the CronJob has never run (none of its Jobs exists yet), so the
# documents reach memory without waiting for the schedule. `--from` Jobs do not
# set the CronJob's lastScheduleTime, so existence of a memory-ingest Job (by
# name or owner) is what says it has run. A re-run that changes nothing, after a
# run, starts nothing. The CronJob comes from `make demos`; before that there is
# nothing to start, and a later re-run starts the first run.
#
# A role assignment made in this run takes a few minutes to reach the data
# plane, and a Job started now would fail with 403: wait for a re-run instead.
for ((i = 0; i < ${#tenants[@]}; i++)); do
  t="${tenants[$i]}"
  if ! "${KUBECTL[@]}" -n "$t" get cronjob memory-ingest -o name >/dev/null 2>&1; then
    echo "memory-ingest is not installed in $t yet (make demos first, then re-run make azure-memory)"
    continue
  fi
  jobs="$("${KUBECTL[@]}" -n "$t" get jobs \
    -o jsonpath='{range .items[*]}{.metadata.name}{" "}{range .metadata.ownerReferences[*]}{.kind}/{.name}{" "}{end}{"\n"}{end}' 2>/dev/null || true)"
  # One line per Job: its name, then each owner as Kind/name. Ours is a Job
  # named memory-ingest-* or owned by CronJob/memory-ingest; the old hook Job
  # omnia-demo-memory-ingest is neither, so it does not count.
  ran=0
  while IFS= read -r line; do
    case " $line" in
      " memory-ingest-"* | *" CronJob/memory-ingest "*) ran=1 ;;
    esac
  done <<<"$jobs"
  need=0
  case "$changed" in *" $t "*) need=1 ;; esac
  [ "$ran" = 1 ] || need=1
  [ "$need" = 1 ] || continue
  case "$granted" in
    *" $t "*)
      echo "$prefix-$t was just granted read access; that takes a few minutes to propagate." \
        "Re-run make azure-memory then to start the first ingest run in $t"
      continue
      ;;
  esac
  echo "==> starting a memory-ingest run in $t"
  run "${KUBECTL[@]}" -n "$t" create job --from=cronjob/memory-ingest "memory-ingest-$(date +%s)"
done
