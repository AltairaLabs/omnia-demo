#!/usr/bin/env bash
# Install one demo tenant: its Azure side, then the omnia-demo chart with the
# profile and your site's values, then federate its agents. What `make demos`
# runs for each tenant.
#
#   deploy/azure/demos.sh <tenant>
#
# Environment (make sets them): CHART (default charts/omnia-demo), PROFILE_DIR
# (default profiles/azure), IMAGE_ENV (default .images.env; when the file
# exists its IMAGE_REGISTRY and IMAGE_TAG replace the chart's image pins).
# DRY_RUN=1 prints each write instead of running it.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

tenant="${1:-}"
if [ -z "$tenant" ]; then
  echo "usage: demos.sh <tenant>   (make demos)" >&2
  exit 1
fi

# shellcheck source=lib.sh
source "$here/lib.sh"

chart="${CHART:-charts/omnia-demo}"
profile_dir="${PROFILE_DIR:-profiles/azure}"
image_env="${IMAGE_ENV:-.images.env}"

# The site's values first: a missing field stops here, before anything is made.
overlay="$(mktemp)"
trap 'rm -f "$overlay"' EXIT
bash "$here/site-values.sh" "$tenant" >"$overlay"

image_set=()
if [ -f "$image_env" ]; then
  reg="$(awk -F' *:?= *' '$1=="IMAGE_REGISTRY"{print $2}' "$image_env")"
  tag="$(awk -F' *:?= *' '$1=="IMAGE_TAG"{print $2}' "$image_env")"
  image_set=(--set-string "images.registry=$reg" --set-string "images.tag=$tag")
  echo "==> Images: $reg, tag $tag ($image_env)"
fi

echo "==> $tenant"
bash "$here/tenant.sh" "$tenant"

# The workspace's identity, from the ConfigMap tenant.sh wrote. In a dry run
# nothing was written, so placeholders stand in.
if [ "${DRY_RUN:-0}" = "1" ]; then
  client_id="<client-id>"
  tenant_id="<tenant-id>"
else
  client_id="$("${KUBECTL[@]}" -n "$tenant" get configmap omnia-workspace -o jsonpath='{.data.workloadIdentityClientId}')"
  tenant_id="$("${KUBECTL[@]}" -n "$tenant" get configmap omnia-workspace -o jsonpath='{.data.workloadIdentityTenantId}')"
  if [ -z "$client_id" ] || [ -z "$tenant_id" ]; then
    echo "configmap omnia-workspace in $tenant has no workload identity ids (run make azure-tenant WS=$tenant)" >&2
    exit 1
  fi
fi

quiet=/dev/null
if [ "${DRY_RUN:-0}" = "1" ]; then quiet=/dev/stdout; fi
run "${HELM[@]}" upgrade --install "$tenant" "$chart" -n "$tenant" \
  -f "$profile_dir/common.yaml" -f "$profile_dir/$tenant.yaml" -f "$overlay" \
  --set "namespace=$tenant" ${image_set[@]+"${image_set[@]}"} \
  --set-string "workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/client-id=$client_id" \
  --set-string "workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/tenant-id=$tenant_id" \
  --wait --timeout 10m >"$quiet"

bash "$here/identity.sh" "$tenant"
