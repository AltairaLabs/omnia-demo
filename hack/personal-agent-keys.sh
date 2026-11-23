#!/usr/bin/env bash
set -euo pipefail

# Make the identity keys of Ellen's assistant, the demo's Personal Agent, and
# store them in Secret personal-agent-identity, which the personal-agent
# Deployment mounts (charts/omnia-demo/templates/personal-agent.yaml).
#
#   CONTEXT=aks-omnia-demo hack/personal-agent-keys.sh           once
#   CONTEXT=aks-omnia-demo hack/personal-agent-keys.sh --force   new keys
#
# Two keys, PKCS#8 PEM, as demos/personal-agent/identity.Load reads them:
#   es256.pem   ECDSA P-256 (ES256)
#   ps384.pem   RSA 3072 (PS384)
#
# STAND-IN (docs/stand-ins.md, "Personal Agent"): a real Personal Agent keeps
# its keys in an HSM or KMS. Here they are a cluster Secret.
#
# Idempotent: an existing Secret is left alone unless --force. Replacing it
# changes the agent's identity: the Companies' facades refetch jwks.json when
# they meet an unknown key id, but every conversation and User ID signed with
# the old keys is lost to it. Restart the Deployment after --force.
#
# CONTEXT is required and every kubectl names it: the current context is
# shared by every terminal on the machine and can change under you.
#
# Env: CONTEXT (required), NAMESPACE (default ellens-assistant),
#      SECRET (default personal-agent-identity).

CONTEXT="${CONTEXT:-}"
NAMESPACE="${NAMESPACE:-ellens-assistant}"
SECRET="${SECRET:-personal-agent-identity}"

force=false
for arg in "$@"; do
  case "${arg}" in
    --force) force=true ;;
    -h|--help) sed -n '4,27p' "$0"; exit 0 ;;
    *) echo "unknown argument: ${arg}" >&2; exit 2 ;;
  esac
done

if [[ -z "${CONTEXT}" ]]; then
  echo "CONTEXT is not set: name the kube context, e.g. CONTEXT=aks-omnia-demo $0" >&2
  exit 2
fi
command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }

kc() { kubectl --context "${CONTEXT}" "$@"; }

if kc -n "${NAMESPACE}" get secret "${SECRET}" >/dev/null 2>&1; then
  if [[ "${force}" != true ]]; then
    echo "secret ${NAMESPACE}/${SECRET} already exists on ${CONTEXT}; leaving it (--force replaces the keys)"
    exit 0
  fi
  echo "==> replacing ${NAMESPACE}/${SECRET} on ${CONTEXT} (--force)"
fi

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT
chmod 700 "${TMP}"

# genpkey writes PKCS#8 ("BEGIN PRIVATE KEY") for both.
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "${TMP}/es256.pem" 2>/dev/null
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out "${TMP}/ps384.pem" 2>/dev/null

# Rendered client-side and applied, so a --force replaces the Secret in place
# and a first run creates it.
kc -n "${NAMESPACE}" create secret generic "${SECRET}" \
  --from-file=es256.pem="${TMP}/es256.pem" \
  --from-file=ps384.pem="${TMP}/ps384.pem" \
  --dry-run=client -o yaml \
  | kc -n "${NAMESPACE}" label --local -f - app.kubernetes.io/part-of=omnia-demo -o yaml \
  | kc apply -f - >/dev/null

echo "==> ${NAMESPACE}/${SECRET} on ${CONTEXT}: es256.pem, ps384.pem"
if [[ "${force}" == true ]]; then
  echo "    restart it: kubectl --context ${CONTEXT} -n ${NAMESPACE} rollout restart deploy/personal-agent"
fi
