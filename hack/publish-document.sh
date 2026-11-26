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

folder="${folder#/}"
folder="${folder%/}"
name="$folder/$(basename "$file")"
want="$(openssl dgst -md5 -binary "$file" | base64)"

echo "==> publishing $file as $tenant-documents/$name"
run azm storage blob upload --auth-mode login --account-name "$acct" -c "$tenant-documents" -n "$name" \
  -f "$file" --overwrite --content-md5 "$want" --content-type text/markdown -o none
echo "==> starting a memory-ingest run in $tenant"
run "${KUBECTL[@]}" -n "$tenant" create job --from=cronjob/memory-ingest "memory-ingest-publish-$(date +%s)"
