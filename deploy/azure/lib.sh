#!/usr/bin/env bash
# Shared helpers for the deploy/azure scripts. Source it; do not run it.
#
# Reads deploy/azure/site.yaml (path in $SITE) with mikefarah yq v4.
# Every write goes through `run`, so DRY_RUN=1 prints instead of acting.
set -euo pipefail

# The default site file sits beside this script, so it does not depend on the
# directory a script is run from.
_lib_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SITE="${SITE:-$_lib_dir/site.yaml}"
unset _lib_dir
# Exported: yq load(strenv(SITE)) in the scripts reads it from the environment.
export SITE

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

# is_not_found <message>: succeed when an az error says the thing is absent.
# Only that answer means "create it" or "run the step that makes it"; any other
# failure (no rights, throttling) stops the script rather than acting on a guess.
is_not_found() {
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
    *notfound* | *"not found"* | *"does not exist"* | *"not exist"*) return 0 ;;
    *) return 1 ;;
  esac
}

# run <cmd...>: execute, or print "DRY_RUN: <cmd>" when DRY_RUN=1.
run() {
  if [ "${DRY_RUN:-0}" = "1" ]; then
    case "$(type -t "$1")" in
      # az_in / azm print the az command they would run, not their own name.
      function) echo "DRY_RUN: $(AZ_PRINT=1 "$@")" ;;
      *) echo "DRY_RUN: $*" ;;
    esac
  else
    "$@"
  fi
}

# az_in <subscription> <az args...>: run az pinned to a subscription. The real
# az rejects a global argument before the command group, so --subscription goes
# last. With AZ_PRINT=1 it prints the command instead (used by `run`).
az_in() {
  local sub="$1"
  shift
  if [ "${AZ_PRINT:-0}" = "1" ]; then
    echo az "$@" --subscription "$sub"
  else
    az "$@" --subscription "$sub"
  fi
}

# The three CLIs, each pinned to the install in site.yaml. A missing field
# stops the sourcing script here, naming the field. azm is az in the install's
# subscription; a script that needs another uses az_in <sub> directly.
AZ_SUB="$(site_req subscription)" || exit 1
_ctx="$(site_req aks.kubeContext)" || exit 1
azm() { az_in "$AZ_SUB" "$@"; }
KUBECTL=(kubectl --context "$_ctx")
HELM=(helm --kube-context "$_ctx")
unset _ctx

# identity_lookup <resource-group> <identity> <field> <tenant>: print one field of
# a tenant's managed identity (clientId, principalId). A missing identity says to
# run make azure-tenant; any other failure is shown as it is. Call it at the top
# level as `x="$(identity_lookup ...)" || exit 1`: bash clears errexit inside
# the $(...), so this function returns explicitly.
identity_lookup() {
  local rg="$1" name="$2" field="$3" tenant="$4" out err msg
  err="$(mktemp)"
  if out="$(azm identity show -g "$rg" -n "$name" --query "$field" -o tsv 2>"$err")"; then
    rm -f "$err"
    if [ -z "$out" ]; then
      echo "identity $name not found in $rg (run make azure-tenant WS=$tenant first)" >&2
      return 1
    fi
    printf '%s\n' "$out"
    return 0
  fi
  msg="$(cat "$err")"
  rm -f "$err"
  if is_not_found "$msg"; then
    echo "identity $name not found in $rg (run make azure-tenant WS=$tenant first)" >&2
  else
    echo "looking up identity $name failed: $msg" >&2
  fi
  return 1
}
