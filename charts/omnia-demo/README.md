# Omnia public memory demo

This chart is an add-on for an existing Omnia installation. It deploys a
local S3 gateway (Versity Gateway) source, uploads the synthetic fixtures, and runs the public example
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

Wrenmoor Energy, the wind and solar supplier, has four agents and a website:

- `wren` — Wren, the customer assistant (chat), in the customer portal's
  side panel. It reads the customer's bill, half-hourly usage and weather,
  quotes tariffs, and switches tariff only with the customer's OK:
  `request_consent` is a client tool the portal renders as "Needs your OK",
  and `switch_tariff` is refused without the `consent_id` that click creates,
  by the ToolPolicy before dispatch and by the account system after it.
- `wrenmoorSite` — the public website, the customer portal and the Wrenmoor
  Hub intranet (`demos/demo-sites`, `--site wrenmoor-energy`). Serve it
  through the catalogue at `/wrenmoor-energy`; it needs no rewrites.
- `halvernSite` — Halvern Partners' public website, Practice Desk and
  design-system reference (`demos/demo-sites`, `--site halvern-partners`).
  Serve it through the catalogue at `/halvern-partners`; no rewrites.

- `incidentAgent` — the on-call incident agent (chat). It reads alerts, logs
  and deploy history, rolls back and scales. Its ToolPolicy forbids any tool
  whose declared `action_scope.reversibility` is `irreversible`, or that
  declares nothing: namespace deletion and database failover are in its
  registry, and refused by what they declare, not by name.
- `dataAnalyst` — the grid analyst, a function (`POST /functions/data-analyst`)
  for questions about Wrenmoor Edge (wind, 29 MW), Cragg Vale Solar (18 MW),
  Heptonstall Store (a 40 MWh battery) and the live energy mix, fired at 06:00
  Europe/London by the `grid-daily-digest` ScheduledFunction, which carries a
  cursor between runs. With `dataAnalyst.a2a.enabled` the same pack also runs
  as `data-analyst-a2a` behind an A2A facade, and the incident agent calls it
  as `a2a__grid_analyst__site_output` (a function-mode agent cannot serve A2A
  itself).
- `fieldService` — the field-service copilot (chat), answering from the field
  manuals in `files/fixtures/wrenmoor-field-procedures/`. They reach it as
  institutional memory: enable `memory` with
  `memory.fixtures: wrenmoor-field-procedures`.

Wren, the incident agent and the data analyst use the stub image the Larkmere
demos use; it serves Wrenmoor Energy's `/wren`, `/ops` and `/grid` endpoints
too.

```yaml
# wrenmoor-energy.yaml, on top of the tenant values above
wren: { enabled: true }
wrenmoorSite: { enabled: true }
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

### The memory Galaxy

`galaxy.enabled` seeds the workspace's memory with a generated corpus (the
Larkmere Mutual Galaxy, from [`demos/memory-galaxy`](../../demos/memory-galaxy/))
across all four tiers, sends the institutional tier as a knowledge graph, and
fits the Galaxy's clusters. It also installs the MemoryPolicy that declares the
corpus's types and relations (`<workspace>-memory`, or `galaxy.policyName`) and
points the Workspace's memory services at it.

```yaml
galaxy: { enabled: true }   # profiles/azure/larkmere-mutual.yaml
```

The seed is a Job named after the seeder image and the fixture, so each new
build seeds once and an upgrade with the same build does not. Re-seeding is
safe: every memory carries an about key (`galaxy`, its corpus id), so a second
write supersedes the first, and the seeder forgets agent memories a regenerated
corpus no longer has.

The Job runs as the workspace memory-api's own ServiceAccount
(`memory-<workspace>-<galaxy.serviceGroup>`), because memory-api lets only its
own identity write the shared tiers and name users. Its init container reads
the Workspace and agent UIDs (a ClusterRole for that one Workspace and a Role
for AgentRuntimes are bound to that ServiceAccount) and waits for memory-api.
The seeder then:

1. sends the knowledge graph (products, claim types, procedures, guidance and
   their edges) through `POST /api/v1/institutional/ingest/batch`;
2. grants each synthetic member, through privacy-api, the consent categories
   their memories need (`memory:identity`, `memory:location` and
   `memory:health` refuse a write until granted), when `workspace.privacy` is
   set;
3. writes the agent, user_for_agent and user memories, pseudonymising each
   member with the workspace's key (the `omnia-workspace-pseudonym-key`
   Secret) exactly as the facade would;
4. drains pending embeddings and fits the Galaxy's leaf clusters
   (`galaxy.leaves`).

It needs Omnia Enterprise and an embedding provider on the workspace memory
service. It writes only through the public memory and privacy APIs.
