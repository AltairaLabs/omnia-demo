#!/usr/bin/env bash
# Compile every UI kit's JSX into one app.js beside its index.html.
#
# The design exports load their .jsx in the browser through kit-loader.js and
# Babel standalone: no build step, but a 2.4 MB compiler download and one
# round trip per module, which behind the catalogue's sign-in proxy is a white
# page for 15 seconds. Compiled here instead, the page loads React, ReactDOM and
# one small bundle.
#
# Run by the Dockerfile before `go build` embeds the site, and by hand for a
# local `go run` (the bundles are gitignored). Needs node (npx).
#
#   demos/demo-sites/hack/build-kits.sh
set -euo pipefail

ESBUILD_VERSION=0.24.2
here=$(cd "$(dirname "$0")/.." && pwd)
shim="$here/site/shared/react-shim.js"

for entry in "$here"/site/*/ui_kits/*/App.jsx; do
  out="$(dirname "$entry")/app.js"
  npx --yes "esbuild@$ESBUILD_VERSION" "$entry" \
    --bundle --format=iife --target=es2020 --minify --log-level=warning \
    --jsx=transform --jsx-factory=React.createElement --jsx-fragment=React.Fragment \
    --alias:react="$shim" \
    --outfile="$out"
  echo "built ${out#"$here"/}"
done
