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
AgentRuntime, the Workspace's namespace, and its memory-api URL:

```bash
WS=<workspace>
WS_NS=$(kubectl get workspace "$WS" -o jsonpath='{.spec.namespace.name}')
helm upgrade --install omnia-demo ./charts/omnia-demo \
  --set galaxy.enabled=true \
  --set galaxy.workspaceUID=$(kubectl get workspace "$WS" -o jsonpath='{.metadata.uid}') \
  --set galaxy.agentUID=$(kubectl -n "$WS_NS" get agentruntime <agent> -o jsonpath='{.metadata.uid}') \
  --set galaxy.namespace="$WS_NS" \
  --set galaxy.memoryAPIURL=$(kubectl get workspace "$WS" -o jsonpath='{.status.services[0].memoryURL}')
```

The Galaxy job writes synthetic data through the public memory API only. It
does not connect to or modify the memory database directly. It runs in the
Workspace's namespace because both things it needs from Omnia live there:

- **The pseudonym key.** The job reads the operator-managed
  `omnia-workspace-pseudonym-key` Secret through a `secretKeyRef`, so the
  synthetic users get the same pseudonyms Omnia's facade would give them. An
  install without a key has no such Secret, and the job pseudonymises unkeyed.
  The job checks its key against the one memory-api is bound to before it
  writes anything.
- **Service authentication.** The job mounts a projected ServiceAccount token
  with the `omnia-session-api` audience (`galaxy.serviceAuth`), from its own
  `omnia-demo-memory-galaxy` ServiceAccount. It needs no RBAC.

`galaxy.memoryAPIURL` is required whenever `galaxy.namespace` is set: the
fallback, `memory.apiURL`, is the ingestion example's endpoint, not the
workspace's.

With Omnia's `internalServiceAuth.enabled: true` (its default), the job seeds
only what a workspace ServiceAccount may write. Once Omnia lets any workspace
ServiceAccount use the ingest routes (a change in progress, matching Omnia's
docs), that is the knowledge-base documents. Curated institutional facts,
agent and user memories and the leaf fit need one of Omnia's own exact-listed
services, and Omnia has no setting to add a caller to that list. The job skips
those tiers with one log line each and still succeeds. Before that Omnia
change it is refused everything and fails. For the full Galaxy, seed a
development install with `internalServiceAuth.enabled: false`. The seeder's
README lists when the job exits non-zero.

The job also requires Omnia Enterprise and an embedding provider on the
workspace memory service. After seeding it drains pending embeddings and fits
the Galaxy's leaf clusters (`galaxy.buildGalaxy`).

The ServiceAccount is a regular chart resource. The Job is a Helm hook with
`before-hook-creation`, so the finished Job and its Pod stay in the workspace
namespace until the next upgrade replaces them, and `helm uninstall` does not
remove them: `kubectl -n <workspace-namespace> delete job
omnia-demo-memory-galaxy` when you are done.
