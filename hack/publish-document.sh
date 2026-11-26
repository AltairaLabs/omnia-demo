#!/usr/bin/env bash
# Publish one document to a tenant's document store, the way someone adds a
# document to the company's own store, and have it picked up at once.
#
#   hack/publish-document.sh <file> <tenant> <folder>
#   hack/publish-document.sh demos/claims-triage/flood-claims.md larkmere-mutual member-guidance
#
# It uploads <file> to <tenant>-documents/<folder>/<basename> in the storage
# account named by memory.storageAccount in deploy/azure/site.yaml (SITE
# overrides the path), then starts a run of the tenant's memory-ingest CronJob
# instead of waiting for the hourly one. Entra ID only (--auth-mode login): you
# need Storage Blob Data Contributor on the account, as for `make azure-memory`.
#
# STAND-IN (docs/stand-ins.md, "The company's document store"): the company's
# editor adding a document. The file is not in memory-documents/, so the next
# `make azure-memory` removes it again; that is the reset.
#
# The kube context is CONTEXT (or KUBE_CONTEXT) if set, else aks.kubeContext in
# site.yaml. DRY_RUN=1 prints each write instead of running it.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=../deploy/azure/lib.sh
source "$here/../deploy/azure/lib.sh"

file="${1:?usage: hack/publish-document.sh <file> <tenant> <folder>}"
tenant="${2:?usage: hack/publish-document.sh <file> <tenant> <folder>}"
folder="${3:?usage: hack/publish-document.sh <file> <tenant> <folder>}"

[ -f "$file" ] || { echo "no such file: $file" >&2; exit 2; }
if ! site_has memory; then
  echo "site.yaml has no memory block: there is no document store to publish to" >&2
  exit 2
fi
acct="$(site_req memory.storageAccount)"

ctx="${CONTEXT:-${KUBE_CONTEXT:-}}"
if [ -n "$ctx" ]; then
  KUBECTL=(kubectl --context "$ctx")
fi

# Everything below becomes part of a container, namespace or blob name, so it
# is checked, as whole strings, before anything runs. Slashes at either end of
# the folder are dropped; each folder segment is letters, digits, dot,
# underscore or hyphen, and is not . or ..; the tenant is lowercase letters,
# digits and hyphens; the file's name is the same characters as a segment and
# does not start with a dot.
while [ "${folder#/}" != "$folder" ]; do folder="${folder#/}"; done
while [ "${folder%/}" != "$folder" ]; do folder="${folder%/}"; done
base="$(basename "$file")"
seg_re='^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$'
tenant_re='^[a-z0-9-]+$'
base_re='^[A-Za-z0-9_-][A-Za-z0-9._-]*$'
if ! [[ $folder =~ $seg_re ]] || [[ /$folder/ == */./* || /$folder/ == */../* ]]; then
  echo "invalid folder '${3}': use segments of letters, digits, '.', '_' and '-' separated by single '/', with no '.' or '..' segment" >&2
  exit 1
fi
if ! [[ $tenant =~ $tenant_re ]]; then
  echo "invalid tenant '$tenant': use lowercase letters, digits and '-'" >&2
  exit 1
fi
if ! [[ $base =~ $base_re ]]; then
  echo "invalid file name '$base': use letters, digits, '.', '_' and '-', not starting with '.'" >&2
  exit 1
fi
name="$folder/$base"
want="$(openssl dgst -md5 -binary "$file" | base64)"

echo "==> publishing $file as $tenant-documents/$name"
run azm storage blob upload --auth-mode login --account-name "$acct" -c "$tenant-documents" -n "$name" \
  -f "$file" --overwrite --content-md5 "$want" --content-type text/markdown -o none
echo "==> starting a memory-ingest run in $tenant"
run "${KUBECTL[@]}" -n "$tenant" create job --from=cronjob/memory-ingest "memory-ingest-publish-$(date +%s)"
