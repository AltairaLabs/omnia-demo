# omnia-demo deploys itself onto any Omnia-on-AKS install

Status: draft for approval, 2026-11-26.

## Intent

omnia-demo is public. Anybody with their own Omnia install on AKS should be
able to deploy the demos from it alone. omnia-azure is private and is our
internal deployment of Omnia to Azure; it should hold only what deploying
Omnia needs, and name nothing from the demos.

Today the two are tangled. `make demos` calls omnia-azure's `make workspace`
and `make workspace-identity` for every tenant, and omnia-azure carries the
Poppy hostnames, DNS records, the Key Vault key for the account tools, the
stand-in portals' trusted issuers and the Company agents' exposure comments.
omnia-demo's `profiles/azure` hard-codes our tenant: the "Omnia Admins"
group, our vault URL, our domain and our dev registry.

## Goals

- `make demos` in omnia-demo works against any Omnia-on-AKS install, given one
  inputs file describing that install. No sibling checkout of omnia-azure.
- Everything demo-specific that needs Azure rights (each tenant's identity,
  databases, Providers, content volume, federated credentials; the Poppy DNS
  records, certificates and Key Vault key) is provisioned by omnia-demo.
- What the demos need from the Omnia install itself (trusted issuers, routed
  Poppy hosts, agent exposure) is published by omnia-demo as an Omnia values
  file the user passes to their own install.
- omnia-azure names no demo, tenant, host or issuer afterwards.
- Our live AKS deployment moves to the new path without recreating anything:
  same identities and client IDs, same databases, same Helm release names.

## Non-goals

- Other clouds. Only the Azure path is built; the layout leaves room for a
  `deploy/<cloud>` beside it.
- Installing Omnia. The user brings an Omnia install; omnia-demo only adds to it.
- Bringing back the SharePoint RAG demo (`rag-hero`). It is Omnia's published
  `omnia-demos` chart, was dropped, and is deleted from omnia-azure, not ported.

## Anti-goals

- omnia-demo depending on omnia-azure in any form: a path, a target name, an
  output file's schema.
- Our tenant's values (group IDs, vault, domain, subscription) committed to
  omnia-demo as if they were the defaults.
- Granting Azure rights from anything a Workspace or agent declares on the
  cluster (the "discovery" option): rights come from the inputs file and the
  demo's own known ServiceAccounts, as today.
- Loosening any security gate, now or in the migration.

## Approaches

- **A. Generic hooks in omnia-azure, names in omnia-demo.** omnia-demo calls
  parameterised omnia-azure targets. Rejected: omnia-azure is private, so no
  outside user could run it.
- **B. omnia-demo provisions its own Azure side (chosen).** A public
  `deploy/azure` takes the user's existing Azure resources as inputs and does
  the per-tenant and Poppy provisioning with `az` and Helm. It ports, rewritten,
  omnia-azure's `omnia-workspace` chart, its workspace targets and the ARM
  Key Vault workaround. The user approved publishing that code.
- **C. omnia-azure discovers what Workspaces need.** Rejected: magic, and it
  grants Azure rights from cluster state.

## Design

### Inputs: `deploy/azure/site.yaml`

One file describes the user's Omnia install. `site.example.yaml` is committed,
with placeholders and a comment per field. `site.yaml` is gitignored. Fields:

- `subscription`, `location`, `identityResourceGroup`, `identityNamePrefix`
  (identities are `<prefix>-<tenant>`; ours is `mi-omnia-demo`).
- `aks`: `resourceGroup`, `name` (for the OIDC issuer), `kubeContext`.
- `postgres`: `host`, `adminLogin`, and the admin password as either an
  External Secrets `ClusterSecretStore` and key (our deployment) or a Secret
  `namespace/name/key`.
- `foundry`: `endpoint`, `accountId`, `region`, and the Providers to make
  (name, role, model, defaults), as omnia-workspace's `foundry.providers`.
- `voice` (optional): `endpoint`, `accountId`, `region`, Providers.
- `nfs` (optional): `storageAccount`, `shareName`, `storageClass`, `size`,
  `rootClaim`.
- `dns`: `subscription`, `resourceGroup`, `zone` (the demo domain), and the
  gateway's public IP (as an address, or a public IP resource to read it from).
- `gateway`: the Gateway (`name`, `namespace`, `sectionName`) the demo's routes
  attach to, and `dashboardHost` (the catalogue is served beside the dashboard).
- `omnia.valuesRoot`: where Omnia's values sit in the user's install, `""` for
  Omnia's chart itself or e.g. `omnia` for an umbrella chart.
- `keyVault`: name and resource group, for the Poppy account tools' key.
- `ownerGroups`: the Entra group IDs bound `owner` on every tenant Workspace.
- `images` (optional): a registry to build into for `make images`.

A missing required field fails the target with the field's name.

### Targets (omnia-demo Makefile)

- `make azure-tenant WS=<t>`: replaces omnia-azure `make workspace`. Creates or
  reuses the identity, grants `Cognitive Services OpenAI User` on Foundry (and
  voice), installs the ported chart as release `workspace-<t>` in `omnia`
  (namespace, `omnia-workspace` ConfigMap, the three database Secrets, Providers,
  NFS PV and claim, the database and subtree Jobs), and enrols the namespace in
  the ambient mesh when ztunnel is present.
- `make azure-identity WS=<t>`: replaces `make workspace-identity`. Federates
  arena-worker, eval-worker, `memory-<t>-default`, `privacy-<t>`,
  `omnia-identity-foundry` and every `<agent>-facade`; prunes `*-facade`
  credentials whose AgentRuntime is gone; annotates memory-api's SA.
- `make azure-poppy`: the Poppy hosts' DNS A records, the `omnia-connections`
  key (through ARM, as now) and `Key Vault Crypto User` on it for the Company
  identities.
- `make omnia-values`: renders `out/omnia-values.yaml` from `site.yaml`, the
  values the user adds to their Omnia install:
  - `operator.delegationAPI.trustedIssuers` for the four stand-in edges
    (larkmere-portal, card-callers, wrenmoor-site, assistant-page);
  - `defaultExposure` for the Company agents, on `site.yaml`'s gateway;
  - and it prints the hosts the install's TLS certificate must cover.
- `make demos`: as now, but calls `azure-tenant` and `azure-identity`, takes
  the identity from the tenant's ConfigMap, and layers values derived from
  `site.yaml` (owner groups, vault URL, domain-based hosts) over the profile.
  It no longer reads `OMNIA_AZURE`. `make images` reads its registry from
  `site.yaml`.

### Profiles

`profiles/azure/*.yaml` keep the demo's own choices (agents, scopes, Poppy
companies) and lose our tenant's values: the group ID, the vault URL, the
`azure.altairalabs.ai` hosts and the Auth0 tenant move to `site.yaml`, or are
built from `dns.zone`. The Auth0 upstream (issuer, client IDs) is per-user too
and goes in `site.yaml` under `poppy`.

### omnia-azure afterwards

Renamed: `poppyHosts` to a generic `TLS_EXTRA_HOSTS` on `make install`.
Removed: `poppy-dns`, `poppy-kms`,
`POPPY_*`, the demo trusted issuers, the Poppy exposure comments, `workspace`,
`workspace-identity`, `workspace-remove`, the `omnia-workspace` chart, the
ambient waypoint manifest (if only workspaces used it), `demo-content` and its
script and values. Added: a generic `EXTRA_VALUES` on `make install`, to layer
a values file (ours: omnia-demo's `out/omnia-values.yaml`), and a generic
`make outputs` that prints the deployment's facts (Postgres host, Foundry
endpoint, Key Vault, DNS zone, OIDC issuer, gateway IP) as JSON, which we copy
into our own `site.yaml` once. Our `site.yaml` lives only on our machines.

### Migration of the live deployment

Same identity names and client IDs, same release names (`workspace-<t>` in
`omnia`, now from omnia-demo's chart), same database names and Secret names,
same federated credential names. Verify each step is a no-op before applying:
`helm diff` (or a rendered-manifest diff) of `workspace-<t>` shows no change,
`azure-identity` creates and deletes no credentials, and `make omnia-values`
renders the same trusted issuers as omnia-azure sets today.

### Settled points (calls made while writing; check them)

- **One shape for every tenant.** omnia-azure's `workspace` gives every
  workspace the same Foundry, voice, NFS and database set, so `site.yaml` is
  flat, with no per-tenant overrides.
- **The ambient waypoint moves.** `azure-tenant` labels the namespace and
  applies the per-namespace waypoint when ztunnel is present, as `workspace`
  does now. The canary needs it. The manifest moves to omnia-demo.
- **Which tenants get the Key Vault key.** `azure-poppy` grants
  `Key Vault Crypto User` to each tenant whose profile in this repo turns on
  `poppy.account` (today larkmere-mutual and wrenmoor-energy). The list is never
  derived from cluster state or `site.yaml`.
- **Arena builder.** `make builder` moves too. omnia-demo installs the
  `omnia-arena-builder` chart into a tenant, at the version and from the chart
  source `site.yaml` names, with an optional sandbox image override (how our
  dev builds pin theirs). It then runs `azure-identity`. `build-loop-reset`
  calls `azure-identity`.
- **omnia-azure's `demo-agent` stays.** It is a platform smoke test of
  omnia-azure's own runtime identity, not omnia-demo content.
- **Personal agent keys.** `hack/personal-agent-keys.sh` stays a separate,
  documented first-install step for ellens-assistant. It makes private keys,
  so it never runs implicitly.
- **Images.** The user brings their own registry (name, login server and
  subscription in `site.yaml`). omnia-demo never creates one.
- **Poppy routing and TLS stay as they work today** (revised while planning).
  Omnia's chart gateway has one HTTPS listener with one certificate, and
  #4475's per-host listeners cover only issuer and Company hosts, not the
  assistant or catalogue hosts. So omnia-demo keeps its own HTTPRoutes, which
  attach to the gateway `site.yaml` names, and states which hosts the install's
  certificate must cover. `make omnia-values` prints that list. omnia-azure
  takes it through a generic `TLS_EXTRA_HOSTS` (today's `poppyHosts`, renamed).
  There are no new Certificates and no `managementApi.poppy.hosts`, and nothing
  changes in routing.
- **Migration diff.** The ported chart keeps the name `omnia-workspace`. A small
  diff we can explain (chart version, labels) is acceptable; any change to a
  resource's spec is not.
- **Poppy upstream.** `site.yaml` `poppy.upstream` takes any OpenID Connect
  provider: an `issuer` and, per Company, a `clientID` and the name of its
  client-secret Secret. Nothing is specific to Auth0.

## Left to the builder

The ported chart's location and name inside omnia-demo; how `site.yaml` is
read in Make (yq is acceptable); the Certificate resources' shape; the commit
order between the two repos, provided omnia-azure's removals land only after
omnia-demo's path has been run against AKS.
