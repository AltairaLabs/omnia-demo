# Omnia public memory demo

This chart is an add-on for an existing Omnia installation. It deploys a
local MinIO source, uploads the synthetic fixtures, and runs the public example
ingestion service once. It does not install or copy Omnia's private templates.

Install Omnia separately, with Enterprise institutional ingestion enabled, then
install this chart:

```bash
helm upgrade --install omnia-demo ./charts/omnia-demo \
  --namespace omnia-demo --create-namespace \
  --set memory.apiURL=http://<memory-api-service>.<omnia-namespace>:8080
```

The chart is intentionally an add-on rather than an umbrella chart. This keeps
the public repository independent of private Omnia implementation details and
makes the dependency on a released Omnia version explicit in the operator's
installation procedure.

For a protected memory API, create a Secret containing the bearer token and
reference it without putting the token in Helm values:

```bash
kubectl -n omnia-demo create secret generic memory-api-token \
  --from-literal=token="$MEMORY_API_TOKEN"
helm upgrade --install omnia-demo ./charts/omnia-demo \
  --set memory.auth.secretName=memory-api-token
```

Released chart installs use the OCI registry instead of a local path:

```bash
helm upgrade --install omnia-demo oci://ghcr.io/altairalabs/charts/omnia-demo \
  --version <release-version> --namespace omnia-demo --create-namespace
```

To populate the richer Memory Galaxy as well as the institutional documents,
enable `galaxy` and provide the metadata UIDs from the target Workspace and
AgentRuntime:

```bash
helm upgrade --install omnia-demo ./charts/omnia-demo \
  --set galaxy.enabled=true \
  --set galaxy.workspaceUID=<workspace-uid> \
  --set galaxy.agentUID=<agent-uid>
```

The Galaxy job writes synthetic data through the public memory API only. It
does not connect to or modify the memory database directly.
