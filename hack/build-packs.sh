#!/usr/bin/env bash
set -euo pipefail

# Compile the public Arena projects into the packs the chart ships
# (charts/omnia-demo/files/arena/<demo>-pack.json), then merge in what packc
# cannot carry.
#
# Usage: hack/build-packs.sh [demo ...]   (default: every demo)
# Needs: packc, jq, yq (mikefarah v4), git.
#
# packc drops three things the demos depend on, so they are merged after
# compiling. Omnia's promptpack schema leaves `metadata` open and defines
# Tool.action_scope, so Omnia accepts the merged pack; packc's own schema does
# not define action_scope, which is why packc validates the pack before the
# merge rather than after.
#
#   arena/governance-declarations.yaml -> metadata.governance (RFC 0013)
#   arena/governance.yaml              -> metadata.asserted, plus
#                                         metadata.observed derived from git
#   arena/tools/*.tool.yaml            -> tools.<name>.action_scope, from each
#                                         tool's spec.action_scope
#   arena/pack-patch.jq                -> a jq filter applied last, for compiled
#                                         fields the source cannot set
#
# action_scope is not decoration: the mortgage ToolPolicy's leavesTheFirm
# producer reads tool.effect from it, so a pack without it never fires the
# DSA-114 forbids, and the incident agent's ToolPolicy refuses every tool whose
# reversibility is irreversible or undeclared.
#
# observed is derived here rather than typed, so it reports the tree that
# produced the pack, and a dirty tree is recorded as dirty. asserted and
# governance are typed by a human and verified by nothing; none of it is
# signed.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKC="${PACKC:-packc}"
OUT="${OUT:-${ROOT}/charts/omnia-demo/files/arena}"
mkdir -p "${OUT}"

if [[ $# -gt 0 ]]; then
  demos=("$@")
else
  demos=(mortgage-underwriting pii-chat claims-triage sharepoint-adapter
    incident-agent data-analyst field-service wren research)
fi

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

for demo in "${demos[@]}"; do
  arena="${ROOT}/demos/${demo}/arena"
  compiled="${TMP}/${demo}.compiled.json"
  out="${OUT}/${demo}-pack.json"

  echo "==> ${demo}: compiling"
  "${PACKC}" compile \
    --config "${arena}/config.arena.yaml" \
    --output "${compiled}" \
    --id "${demo}"
  # Validated BEFORE the merge: packc's schema closes Tool to
  # name/description/parameters, so it rejects action_scope. Omnia's own
  # promptpack schema defines it ($defs.Tool.action_scope, RFC 0013) and reads
  # it at runtime.
  "${PACKC}" validate "${compiled}"

  governance='null'
  if [[ -f "${arena}/governance-declarations.yaml" ]]; then
    governance="$(yq -o json -I0 '.' "${arena}/governance-declarations.yaml")"
  fi

  asserted='null'
  observed='null'
  if [[ -f "${arena}/governance.yaml" ]]; then
    asserted="$(yq -o json -I0 '.' "${arena}/governance.yaml")"
    # `git status --porcelain` sees untracked files too; `git diff` alone would
    # report a brand-new arena directory as clean.
    dirty=false
    if [[ -n "$(git -C "${ROOT}" status --porcelain -- "${arena}")" ]]; then
      dirty=true
    fi
    observed="$(jq -n -c \
      --arg commit "$(git -C "${ROOT}" rev-parse HEAD)" \
      --argjson dirty "${dirty}" \
      --arg author "$(git -C "${ROOT}" log -1 --format='%an <%ae>' -- "${arena}")" \
      --arg committed "$(git -C "${ROOT}" log -1 --format='%cI' -- "${arena}")" \
      --arg describe "$(git -C "${ROOT}" describe --tags --always --dirty 2>/dev/null || true)" \
      --arg pipeline "${GITHUB_RUN_ID:+github-actions/${GITHUB_RUN_ID}}" \
      '{source_commit: $commit, working_tree_clean: ($dirty | not)}
       + ({source_author: $author, source_committed_at: $committed,
           source_describe: $describe, build_pipeline: $pipeline}
          | with_entries(select(.value != "")))')"
  fi

  scopes='{}'
  shopt -s nullglob
  tool_files=("${arena}"/tools/*.tool.yaml)
  shopt -u nullglob
  if [[ ${#tool_files[@]} -gt 0 ]]; then
    scopes="$(yq -o json -I0 '{"name": .spec.name, "scope": .spec.action_scope}' "${tool_files[@]}" \
      | jq -s -c 'map(select(.scope != null) | {(.name): .scope}) | add // {}')"
  fi

  patch='.'
  if [[ -f "${arena}/pack-patch.jq" ]]; then
    patch="$(<"${arena}/pack-patch.jq")"
  fi

  echo "==> ${demo}: merging"
  jq \
    --argjson governance "${governance}" \
    --argjson asserted "${asserted}" \
    --argjson observed "${observed}" \
    --argjson scopes "${scopes}" \
    "
    # A scope for a tool the pack does not have is an error, not a no-op: it
    # would look declared to whoever wrote it and read as undeclared to every
    # consumer. Renaming a tool is exactly when that happens.
    ((.tools // {}) | keys) as \$have
    | (\$scopes | keys - \$have) as \$unknown
    | if (\$unknown | length) > 0
      then error(\"action_scope declared for tools not in the pack: \(\$unknown)\")
      else . end
    | if .tools then .tools |= with_entries(
        if \$scopes[.key] then .value.action_scope = \$scopes[.key] else . end)
      else . end
    | if \$observed != null then .metadata.observed = \$observed else . end
    | if \$asserted != null then .metadata.asserted = \$asserted else . end
    | if \$governance != null then .metadata.governance = \$governance else . end
    | ${patch}
    " "${compiled}" > "${out}"

  echo "    governance: $(jq -r '.metadata.governance | if . then (keys | length | tostring) + " keys" else "none" end' "${out}")"
  echo "    asserted  : $(jq -r '.metadata.asserted | if . then (keys | length | tostring) + " keys" else "none" end' "${out}")"
  echo "    scopes    : $(jq -r '[(.tools // {})[] | select(.action_scope)] | length' "${out}") of $(jq -r '.tools // {} | length' "${out}") tools"
  # Not `// true`: jq's alternative operator treats false as missing.
  if [[ "$(jq -r '.metadata.observed.working_tree_clean | if . == null then true else . end' "${out}")" == "false" ]]; then
    echo "    WARNING   : arena source is dirty; provenance records working_tree_clean=false"
  fi
  echo "==> wrote ${out#"${ROOT}/"}"
done
