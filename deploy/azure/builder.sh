#!/usr/bin/env bash
# Install the Arena builder (the agent behind Projects, Generate with AI and the
# editor's Builder tab) into a tenant's namespace, then federate its identity.
# The chart and version come from site.yaml (builder.chart, builder.version);
# builder.sandboxImage, when set, overrides the sandbox image the chart pins.
#
#   deploy/azure/builder.sh <tenant> <provider>
#
# <provider> is the Provider the builder agent runs on (it must exist in the
# tenant). Release name arena-builder. DRY_RUN=1 prints the install instead of
# running it.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

tenant="${1:-}"
provider="${2:-}"
if [ -z "$tenant" ] || [ -z "$provider" ]; then
  echo "usage: builder.sh <tenant> <provider>   (make builder)" >&2
  exit 1
fi

# shellcheck source=lib.sh
source "$here/lib.sh"

chart="$(site_req builder.chart)"
version="$(site_req builder.version)"

args=(--version "$version" -n "$tenant" --set "provider.name=$provider")
if site_has builder.sandboxImage; then
  repository="$(site_req builder.sandboxImage.repository)"
  args+=(--set "sandbox.image.repository=$repository")
  tag="$(site_get builder.sandboxImage.tag)"
  digest="$(site_get builder.sandboxImage.digest)"
  if [ -n "$tag" ]; then args+=(--set-string "sandbox.image.tag=$tag"); fi
  if [ -n "$digest" ]; then args+=(--set-string "sandbox.image.digest=$digest"); fi
fi

echo "==> arena-builder $version -> $tenant (provider $provider)"
quiet=/dev/null
if [ "${DRY_RUN:-0}" = "1" ]; then quiet=/dev/stdout; fi
run "${HELM[@]}" upgrade --install arena-builder "$chart" "${args[@]}" >"$quiet"

bash "$here/identity.sh" "$tenant"
