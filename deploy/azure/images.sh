#!/usr/bin/env bash
# Build the demo images into your container registry (images.* in site.yaml)
# with `az acr build`, tagged dev-<commit>, and record the registry and tag in
# $IMAGE_ENV (default .images.env), which `make demos` then deploys. Nothing is
# published from this repository, and no registry is created: you bring it.
#
#   deploy/azure/images.sh <name,context,Dockerfile> ...
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ "$#" -eq 0 ]; then
  echo "usage: images.sh <name,context,Dockerfile> ...   (make images)" >&2
  exit 1
fi

# shellcheck source=lib.sh
source "$here/lib.sh"

acr="$(site_req images.registry)"
server="$(site_req images.loginServer)"
sub="$(site_req images.subscription)"
image_env="${IMAGE_ENV:-.images.env}"

tag="dev-$(git rev-parse --short HEAD)"
if [ -n "$(git status --porcelain)" ]; then tag="$tag-$(date +%Y%m%d%H%M)"; fi
echo "==> Building into $server, tag $tag"
for spec in "$@"; do
  IFS=, read -r name context file <<<"$spec"
  echo "==> $name"
  run az acr build --subscription "$sub" --registry "$acr" --platform linux/amd64 \
    --image "$name:$tag" --file "$file" --no-logs --output none "$context"
done
printf 'IMAGE_REGISTRY := %s\nIMAGE_TAG := %s\n' "$server" "$tag" >"$image_env"
echo "==> Built $tag; make demos now runs it (rm $image_env to go back)"
