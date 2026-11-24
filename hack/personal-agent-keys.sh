#!/usr/bin/env bash
set -euo pipefail

# Make the keys of Ellen's assistant, the demo's Personal Agent, and store them
# in two Secrets the personal-agent Deployment mounts
# (charts/omnia-demo/templates/personal-agent.yaml): personal-agent-identity
# (its identity keys) and personal-agent-token-key (the key its sign-in tokens
# are sealed under in Redis).
#
#   CONTEXT=aks-omnia-demo hack/personal-agent-keys.sh           once
#   CONTEXT=aks-omnia-demo hack/personal-agent-keys.sh --force   new keys
#
# Order on a new install: make -C ../omnia-azure workspace WS=ellens-assistant
# (makes the namespace), then this script, then make demos. The Deployment
# mounts both Secrets, so on an existing cluster that has only
# personal-agent-identity, run this again before the next make demos: it
# creates whichever Secret is missing and leaves the other alone.
#
# Two keys, PKCS#8 PEM, as demos/personal-agent/identity.Load reads them:
#   es256.pem   ECDSA P-256 (ES256)
#   ps384.pem   RSA 3072 (PS384)
# And token.key: 32 random bytes, one line of standard base64 (openssl rand
# -base64 32), as --token-key-file reads it.
#
# STAND-IN (docs/stand-ins.md, "Personal Agent"): a real Personal Agent keeps
# its keys in an HSM or KMS. Here they are a cluster Secret.
#
# Idempotent: each of the two Secrets is made only if it is missing, and an
# existing one is left alone unless --force. --force replaces the keys of both
# Secrets. A new token key leaves every stored sign-in unreadable, so Ellen
# signs in again. New identity keys keep the agent's client_id, and the
# Companies' facades refetch jwks.json when they meet an unknown key id. Its
# conversations and User IDs are in Redis (pa:*), not here, so they stay; to
# clear them, use the Reset in demos/personal-agent/RUNBOOK.md. Restart the
# Deployment after --force so it signs with the new keys.
#
# CONTEXT is required and every kubectl names it: the current context is
# shared by every terminal on the machine and can change under you.
#
# Env: CONTEXT (required), NAMESPACE (default ellens-assistant),
#      SECRET (default personal-agent-identity),
#      TOKEN_SECRET (default personal-agent-token-key).

CONTEXT="${CONTEXT:-}"
NAMESPACE="${NAMESPACE:-ellens-assistant}"
SECRET="${SECRET:-personal-agent-identity}"
TOKEN_SECRET="${TOKEN_SECRET:-personal-agent-token-key}"

force=false
for arg in "$@"; do
  case "${arg}" in
    --force) force=true ;;
    -h|--help) sed -n '4,42p' "$0"; exit 0 ;;
    *) echo "unknown argument: ${arg}" >&2; exit 2 ;;
  esac
done

if [[ -z "${CONTEXT}" ]]; then
  echo "CONTEXT is not set: name the kube context, e.g. CONTEXT=aks-omnia-demo $0" >&2
  exit 2
fi
command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }

kc() { kubectl --context "${CONTEXT}" "$@"; }

# The namespace is made by omnia-azure's `make workspace WS=ellens-assistant`,
# before this script and before `make demos` (profiles/azure/ellens-assistant.yaml).
# Assigned, not tested inline: set -e stops on a failed assignment, but not on
# a failed substitution inside [[ ]].
ns="$(kc get namespace "${NAMESPACE}" --ignore-not-found -o name)"
if [[ -z "${ns}" ]]; then
  echo "namespace ${NAMESPACE} does not exist on ${CONTEXT}: run make -C ../omnia-azure workspace WS=${NAMESPACE} first" >&2
  exit 1
fi

# --ignore-not-found: NotFound is an empty answer, and any other error (RBAC,
# a timeout) fails the script under set -e instead of reading as "absent".
existing="$(kc -n "${NAMESPACE}" get secret "${SECRET}" --ignore-not-found -o name)"
existing_token="$(kc -n "${NAMESPACE}" get secret "${TOKEN_SECRET}" --ignore-not-found -o name)"
if [[ -n "${existing}" && -n "${existing_token}" && "${force}" != true ]]; then
  echo "secrets ${NAMESPACE}/${SECRET} and ${NAMESPACE}/${TOKEN_SECRET} already exist on ${CONTEXT}; leaving them (--force replaces the keys)"
  exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT
chmod 700 "${TMP}"

# genpkey writes PKCS#8 ("BEGIN PRIVATE KEY") for both.
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "${TMP}/es256.pem" 2>/dev/null
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out "${TMP}/ps384.pem" 2>/dev/null
openssl rand -base64 32 > "${TMP}/token.key"

# Rendered client-side, then created or replaced. Never `kubectl apply`:
# client-side apply copies the whole object, private keys included, into the
# kubectl.kubernetes.io/last-applied-configuration annotation. `create` also
# fails if the Secret appeared since the check, so existing keys are never
# replaced without --force.
render() {
  local name="$1"; shift
  kc -n "${NAMESPACE}" create secret generic "${name}" "$@" \
    --dry-run=client -o yaml \
    | kc -n "${NAMESPACE}" label --local -f - app.kubernetes.io/part-of=omnia-demo -o yaml
}

# put NAME EXISTING FILE=PATH...: create the Secret, or replace it (--force only).
put() {
  local name="$1" have="$2"; shift 2
  local args=()
  local f
  for f in "$@"; do args+=("--from-file=${f}"); done
  if [[ -n "${have}" ]]; then
    if [[ "${force}" == true ]]; then
      echo "==> replacing ${NAMESPACE}/${name} on ${CONTEXT} (--force)"
      render "${name}" "${args[@]}" | kc -n "${NAMESPACE}" replace -f - >/dev/null
    else
      echo "==> ${NAMESPACE}/${name} exists; leaving it (--force replaces)"
    fi
  else
    render "${name}" "${args[@]}" | kc -n "${NAMESPACE}" create -f - >/dev/null
    echo "==> created ${NAMESPACE}/${name} on ${CONTEXT}"
  fi
}

put "${SECRET}" "${existing}" es256.pem="${TMP}/es256.pem" ps384.pem="${TMP}/ps384.pem"
put "${TOKEN_SECRET}" "${existing_token}" token.key="${TMP}/token.key"

if [[ -n "${existing}" || -n "${existing_token}" ]] && [[ "${force}" == true ]]; then
  echo "    restart it: kubectl --context ${CONTEXT} -n ${NAMESPACE} rollout restart deploy/personal-agent"
fi
