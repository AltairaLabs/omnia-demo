#!/usr/bin/env bash
# Shared helpers for the deploy/azure scripts. Source it; do not run it.
#
# Reads deploy/azure/site.yaml (path in $SITE) with mikefarah yq v4.
# Every write goes through `run`, so DRY_RUN=1 prints instead of acting.
set -euo pipefail

SITE="${SITE:-deploy/azure/site.yaml}"

# site_get <dotted.path>: print the value, or nothing when absent or null.
site_get() {
  yq e ".$1 | select(. != null)" "$SITE"
}

# site_has <dotted.path>: succeed when the key is present and non-null.
site_has() {
  [ "$(yq e "(.$1 != null)" "$SITE")" = "true" ]
}

# site_req <dotted.path>: print the value, or fail naming the field.
site_req() {
  local v
  v="$(site_get "$1")"
  if [ -z "$v" ]; then
    echo "site.yaml: $1 is required" >&2
    return 1
  fi
  printf '%s\n' "$v"
}

# run <cmd...>: execute, or print "DRY_RUN: <cmd>" when DRY_RUN=1.
run() {
  if [ "${DRY_RUN:-0}" = "1" ]; then
    echo "DRY_RUN: $*"
  else
    "$@"
  fi
}

# The three CLIs, each pinned to the install in site.yaml. A missing field
# stops the sourcing script here, naming the field.
_sub="$(site_req subscription)" || exit 1
_ctx="$(site_req aks.kubeContext)" || exit 1
AZ=(az --subscription "$_sub")
KUBECTL=(kubectl --context "$_ctx")
HELM=(helm --kube-context "$_ctx")
unset _sub _ctx
