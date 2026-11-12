#!/usr/bin/env bash
set -euo pipefail

# Re-ingest one changed manual, the way a document store's connector picks up
# an edit: put the new version in the bucket and run the ingestion service
# again. Nothing is redeployed, and the agent is not touched. That is demo 2's
# point: the copilot's knowledge lives in the documents.
#
# Usage: hack/reingest-manual.sh <file> [namespace]
#   <file>       the edited document; its basename is its key in the bucket,
#                so it replaces the fixture of the same name
#   [namespace]  default wrenmoor-energy
#
# It derives a one-off Job from the chart's ingest Job (omnia-demo-memory-ingest):
# the same image, identity, bucket and memory-api, but an init step that copies
# only this file, instead of mirroring the chart's fixtures (which would put
# the old version back). The ingestion service's manifest sees the changed
# file and replaces its memories.
#
# Needs: kubectl (context KUBE_CONTEXT, default aks-omnia-demo), jq.

file=${1:?usage: hack/reingest-manual.sh <file> [namespace]}
ns=${2:-wrenmoor-energy}
ctx=${KUBE_CONTEXT:-aks-omnia-demo}
k() { kubectl --context "${ctx}" -n "${ns}" "$@"; }

[[ -f "${file}" ]] || { echo "no such file: ${file}" >&2; exit 2; }
name=$(basename "${file}")
run="reingest-$(date +%s)"

base=$(k get job omnia-demo-memory-ingest -o json)
bucket=$(jq -r '.spec.template.spec.containers[0].env[] | select(.name=="S3_BUCKET") | .value' <<< "${base}")
prefix=$(jq -r '.spec.template.spec.containers[0].env[] | select(.name=="S3_PREFIX") | .value' <<< "${base}")
[[ -n "${bucket}" && "${bucket}" != null ]] || { echo "the ingest Job has no S3_BUCKET: is memory.objectStore enabled?" >&2; exit 1; }

k create configmap "${run}" --from-file="${name}=${file}" >/dev/null

: "The init step does what the chart's does (make the bucket, mirror the"
: "fixtures, which the in-cluster gateway loses on a restart), then puts the"
: "edited file over its fixture. Uploading only the edited file would leave"
: "the bucket holding one document, and the connector would forget the rest."
jq --arg run "${run}" --arg name "${name}" --arg bucket "${bucket}" --arg prefix "${prefix}" '
  {apiVersion: "batch/v1", kind: "Job",
   metadata: {name: $run, namespace: .metadata.namespace},
   spec: {backoffLimit: 0, ttlSecondsAfterFinished: 600,
     template: {spec: (.spec.template.spec
       | .restartPolicy = "Never"
       | .initContainers[0].args = [
           "set -eu; s3() { aws --endpoint-url http://memory-s3:9000 \"$@\"; }; " +
           "until s3 s3 ls >/dev/null 2>&1; do sleep 2; done; " +
           "s3 s3 mb s3://" + $bucket + " 2>/dev/null || true; " +
           "s3 s3 sync /fixtures/ s3://" + $bucket + "/" + $prefix + " --delete --exclude \"*\" --include \"*.md\" --exclude \"..*\"; " +
           "s3 s3 cp /doc/" + $name + " s3://" + $bucket + "/" + $prefix + $name]
       | .initContainers[0].volumeMounts += [{name: "doc", mountPath: "/doc"}]
       | .volumes += [{name: "doc", configMap: {name: $run}}])}}}
' <<< "${base}" | k apply -f - >/dev/null

echo "==> ${name} -> s3://${bucket}/${prefix}${name}; ingesting (job ${run})"
: "Never restarts and keeps a failed pod, so its logs can be read."
if ! k wait --for=jsonpath='{.status.conditions[0].status}'=True "job/${run}" --timeout=300s >/dev/null \
   || [[ "$(k get "job/${run}" -o jsonpath='{.status.succeeded}')" != 1 ]]; then
  k logs "job/${run}" --all-containers --tail=20 >&2 || true
  echo "re-ingest failed" >&2
  exit 1
fi
k logs "job/${run}" -c ingest --tail=5
k delete configmap "${run}" >/dev/null
