# Omnia public memory demo

This chart is an add-on for an existing Omnia installation. It deploys a
local MinIO source, uploads the synthetic fixtures, and runs the public example
ingestion service once. It does not install or copy Omnia's private templates.

Install Omnia separately, with Enterprise institutional ingestion enabled, then
install this chart:

```bash
helm upgrade --install omnia-demo ./charts/omnia-demo \
  --namespace omnia-demo --create-namespace
```

`memory.apiURL` defaults to the workspace's memory-api; set it to target
another.

## One release per tenant

Each release can create its own tenant Workspace and run every demo agent on a
Provider that already exists in it. For example, a keyless Azure AI Foundry
Provider with workload identity:

```yaml
# tenant.yaml
namespace: larkmere-mutual
llm:
  providerRef: azure-foundry-gpt-4o      # no demo creates a Provider of its own
workspace:
  create: true
  displayName: Larkmere Mutual
  services:
    - name: default
      mode: managed
      session: { database: { secretRef: { name: omnia-postgres } } }
      memory:
        database: { secretRef: { name: omnia-postgres-memory } }
        providerRef: { name: azure-foundry-embedding }   # memory needs one
  runtimeServiceAccount:                 # every agent runs as this SA
    name: omnia-runtime-wi
    annotations: { azure.workload.identity/client-id: <client-id> }
    podLabels: { azure.workload.identity/use: "true" }
mortgageUnderwriting: { enabled: true }
claimsTriage: { enabled: true }
piiChat: { enabled: true }
```

```bash
helm upgrade --install larkmere-mutual oci://ghcr.io/altairalabs/charts/omnia-demo \
  --version <release-version> -n larkmere-mutual --create-namespace -f tenant.yaml
```

The Providers and database Secrets named in `tenant.yaml` live in the
workspace namespace and are the installer's to create: a Workspace cannot use
another namespace's Provider. Image tags default to the chart's appVersion.
`memory.apiURL` defaults to the workspace's own memory-api
(`memory-<workspace>-<workspace.serviceGroup>`).

## The governance demos

`mortgageUnderwriting`, `claimsTriage` and `piiChat` each show one control: the
DSA-114 egress forbids, the £500 delegated-authority limit, and session and
memory redaction. Each has a `deterministic` switch that runs it on its own
mock Provider and a scripted chain, with no API key and whatever
`llm.providerRef` says; the tools, policy broker and session record stay live.
`piiChat.portal.enabled` adds the Larkmere member portal and opens the agent
to it with `externalAuth.edgeTrust`. The mortgage demo is invoked async, so
Omnia needs a queue backend (Redis). Provider tuning in a demo's `provider`
block (`maxTokens`, `streamIdleTimeout`, `thinkingBudgetTokens`) applies to
its own Provider only, not to `llm.providerRef`.

## The Wrenmoor Energy demos

Wrenmoor Energy, the utility and grid operator, has three agents:

- `incidentAgent` — the on-call incident agent (chat). It reads alerts, logs
  and deploy history, rolls back and scales. Its ToolPolicy forbids any tool
  whose declared `action_scope.reversibility` is `irreversible`, or that
  declares nothing: namespace deletion and database failover are in its
  registry, and refused by what they declare, not by name.
- `dataAnalyst` — a function (`POST /functions/data-analyst`) for load and
  consumption questions over a synthetic feeder dataset, fired at 06:00
  Europe/London by the `grid-daily-digest` ScheduledFunction, which carries a
  cursor between runs. With `dataAnalyst.a2a.enabled` the same pack also runs
  as `data-analyst-a2a` behind an A2A facade, and the incident agent calls it
  as `a2a__grid_analyst__feeder_load` (a function-mode agent cannot serve A2A
  itself).
- `fieldService` — the field-service copilot (chat), answering from the field
  manuals in `files/fixtures/wrenmoor-field-procedures/`. They reach it as
  institutional memory: enable `memory` with
  `memory.fixtures: wrenmoor-field-procedures`.

The incident agent and the data analyst use the stub image the Larkmere demos
use; it serves Wrenmoor Energy's `/ops` and `/grid` endpoints too.

```yaml
# wrenmoor-energy.yaml, on top of the tenant values above
incidentAgent: { enabled: true }
dataAnalyst: { enabled: true }
fieldService: { enabled: true }
memory:
  enabled: true
  fixtures: wrenmoor-field-procedures
```

Runbooks: [mortgage underwriting](../../demos/mortgage-underwriting/RUNBOOK.md),
[PII chat](../../demos/pii-chat/RUNBOOK.md). The packs are compiled from
`demos/<demo>/arena` by `hack/build-packs.sh`.

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

`galaxy.memoryAPIURL` is required whenever `galaxy.namespace` is set. Without
it the job targets the release's own workspace memory-api.

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
