# Deploying the demos onto your own Omnia on AKS

These scripts install the demo tenants onto an Omnia you already run on AKS.
They bring no Omnia and no cluster of their own. The demos were first run on
the maintainers' own deployment; everything that was specific to it is now a
field in `deploy/azure/site.yaml`.

`deploy/azure/site.example.yaml` documents every field, with placeholders.
Copy it to `deploy/azure/site.yaml` (gitignored) and fill it in; this page does
not repeat it. A required field that is missing stops a script with
`site.yaml: <field> is required`, before anything is changed.

## What it needs

Your install, ready before the first command:

| Need | Used for | `site.yaml` |
| --- | --- | --- |
| Omnia on AKS, with the OIDC issuer and workload identity on | each agent runs as the tenant's managed identity, federated to its ServiceAccount | `aks.*`, `omnia.*` |
| A Postgres Flexible Server | each tenant gets sessions, memory and consent databases | `postgres.*` |
| An Azure AI Foundry account (and optionally a speech account) | the tenant identity is granted `Cognitive Services OpenAI User` on it | `foundry.*`, `voice.*` |
| An Azure DNS zone | A records for the Poppy hosts (`larkmere.<zone>`, `auth.larkmere.<zone>`, `assistant.<zone>`, ...) | `dns.*`, `dashboardHost` |
| A Gateway and a TLS source (cert-manager or another) | the hosts above are served from it; its certificate must cover them | `gateway.*` |
| A Key Vault | the key `omnia-connections`, which Poppy account tools seal tokens with | `keyVault.*` |
| External Secrets, if the Postgres password lives in a vault | otherwise a plain Secret holds it. With External Secrets, the databases Job still reads a Secret `omnia-db-admin` (key `PGPASSWORD`) in the Omnia namespace, which you create (see below) | `postgres.adminPassword` |
| A ClusterSecretStore holding an AI gateway key (optional) | the scope guardrail on Larkmere's member assistant. Without it the guardrail is off | `guardrail.*` |
| NFS storage account (optional) | a shared content volume per tenant | `nfs.*` |
| A Blob storage account (optional, you bring it) | one private container per tenant, `<tenant>-documents`, holding the company's documents for institutional memory. Without it, memory is off for every tenant | `memory.*` |
| An ACR (optional, you bring it) | `make images` builds the demo images into it | `images.*` |

On your machine: `az`, `kubectl`, `helm`, `jq` and mikefarah `yq` v4, signed in
to the subscription, with a kubeconfig context for the cluster
(`aks.kubeContext`). Every `kubectl` and `helm` call names that context, and
every `az` call names its subscription, so nothing depends on what is current.

## The rights you need

The scripts act as you. Nothing is granted to anyone but the identities and
ServiceAccounts they name and each AgentRuntime's `<name>-facade`.

| Step | Azure calls | Right |
| --- | --- | --- |
| `azure-tenant` | `az identity create` in `identityResourceGroup`; `az role assignment create` of `Cognitive Services OpenAI User` on the Foundry (and speech) account | Managed Identity Contributor on the resource group; Role Based Access Control Administrator on the Foundry and speech accounts (conditioned to assign only `Cognitive Services OpenAI User` if you like) |
| `azure-identity` | `az aks show` (reads the OIDC issuer); `az identity federated-credential list/create/delete` | Reader on the cluster; Managed Identity Contributor on the identity resource group |
| `azure-poppy` | `az network dns record-set a show/add-record/delete` in the zone (its subscription is `dns.subscription`); `az network public-ip show` if you name a public IP (in `dns.gatewayPublicIP.subscription` if set); `az keyvault show`; `az rest --method put` of the Key Vault key `omnia-connections` through ARM; `az role assignment create` of `Key Vault Crypto User` on that key | DNS Zone Contributor on the zone; Reader on the vault, the public IP and the identity resource group (it reads each tenant identity's principal); Key Vault Contributor on the vault; Role Based Access Control Administrator on the key (conditioned to assign only `Key Vault Crypto User` if you like) |
| `azure-memory` | `az storage container/blob ...` with `--auth-mode login` (data plane); `az role assignment create` of `Storage Blob Data Reader` on one container; `az storage account show`, `az identity show` | Storage Blob Data Contributor on the storage account (a control-plane Owner or Contributor is not enough: the data plane needs its own role); Role Based Access Control Administrator on the account, conditioned to assign only `Storage Blob Data Reader`; Reader on the account and on the identity resource group |
| `images` | `az acr build` | Container Registry Tasks Contributor on the registry |

`azure-memory` uses Entra ID only: no account key, connection string or SAS, and
no public access. The tenant's identity gets `Storage Blob Data Reader` on its
own container, never the account.

In the cluster, `azure-tenant` and `make demos` run `helm upgrade --install`,
which creates namespaces and the chart's Workspace, ServiceAccounts, Roles and
Jobs, so the context needs rights to do that and to read AgentRuntimes.

## First install, in order

1. **Write `deploy/azure/site.yaml`** from `site.example.yaml`.
2. **`make omnia-values`** writes `out/omnia-values.yaml` and prints
   `TLS hosts: ...`. Merge the file into the values of the install that runs
   Omnia (default exposure on your Gateway, and the trusted token issuers),
   upgrade it, and make its certificate cover the printed hosts. `valuesRoot`
   in `site.yaml` places the values under your umbrella chart if you have one.
   The trusted issuers it lists are the demo's stand-in identity edges
   (docs/stand-ins.md); a real install lists its own identity provider.
3. **Create the Secrets** the demos read ("Secrets and accounts you create"
   below), except the Poppy ones, which need the tenants' namespaces: those go
   in after step 5.
4. **`make azure-tenant WS=ellens-assistant`**, then
   **`hack/personal-agent-keys.sh`**. The assistant's namespace must exist
   before its keys, and its Deployment cannot start without them. The script
   reads the kube context from `CONTEXT`, `KUBE_CONTEXT` or `aks.kubeContext`.
5. **`make images`** builds the demo images into your registry and writes
   `.images.env`. Skip it only if you pin published images in the profiles.
   Nothing is published from this repository.
6. **`make demos`** installs every tenant: its Azure side, the chart, then
   federation of its agents. `make demos TENANTS=larkmere-mutual` does one.
7. **`make azure-poppy`** makes the DNS records, the Key Vault key and the right
   to use it. It reads each Poppy tenant's identity, which `make demos` made,
   so it comes after it; the DNS records and the key are not needed for the
   helm install. It is safe to run again.
8. **`make azure-memory`** (optional; needs the `memory` block in `site.yaml`,
   and `make demos` first, which installs the CronJob). For each tenant whose
   profile sets `memory.enabled`, it creates the private container
   `<tenant>-documents` in `memory.storageAccount`, uploads
   `memory-documents/<tenant>/` (subfolders become blob folders; only files
   whose MD5 changed, and it deletes blobs with no file), grants the tenant identity `Storage Blob Data Reader`
   on that container, and starts a first run of the `memory-ingest` CronJob when
   it has never run. **A new grant needs a few minutes** to apply, so that
   first run is not started then: run `make azure-memory` again. Edit a
   document and run it again to update memory; the CronJob also reads the
   container hourly. With no `memory` block it does nothing.

## Secrets and accounts you create

The scripts make no Secret and no account on a vendor's side. These you bring,
by hand, before the step that needs them. None of the values go in a file in
this repository.

- **External Secrets mode only: the databases Job's admin Secret.** The
  `omnia-workspace` chart's databases Job reads the Postgres admin password from
  a Secret in the Omnia namespace (`omnia.namespace`), key `PGPASSWORD`. The
  default name is `omnia-db-admin`; `postgres.adminPassword.externalSecret.dbAdminSecret`
  names another. In plain Secret mode the Job reads the Secret you name in
  `postgres.adminPassword.secret` (also in `omnia.namespace`) instead, and
  nothing more is needed.
- **The Poppy sign-in** (only to turn the account tools on; milestone 2 in
  `demos/personal-agent/RUNBOOK.md`). One OpenID Connect application at your
  upstream sign-in provider per Company (its issuer is `poppy.upstream.issuer`,
  its client ID `poppy.companies.<tenant>.clientID`), with the callback
  `https://auth.<company>.<zone>/oauth/callback`. Then, in each Company's
  namespace (`larkmere-mutual`, `wrenmoor-energy`), after `make demos`:
  - `larkmere-gateway-upstream` / `wrenmoor-gateway-upstream` (or the name in
    `poppy.companies.<tenant>.clientSecretName`), key `clientSecret`: the
    application's client secret.
  - `larkmere-exchange-client` / `wrenmoor-exchange-client`, key `clientSecret`
    (at least 32 random bytes), labelled
    `omnia.altairalabs.ai/token-exchange-client-secret=true`.
- **The scope guardrail's key** (optional): the AI gateway's API key, stored in
  the secret store behind `guardrail.clusterSecretStore` under `guardrail.key`.
  An ExternalSecret in Larkmere's namespace reads it. No `guardrail` block, no
  guardrail.
- **The personal assistant's keys:** `hack/personal-agent-keys.sh` makes them
  (`personal-agent-identity`, `personal-agent-token-key`) in `ellens-assistant`
  once its namespace exists.

`make builder` adds the Arena builder to Halvern Partners. `make lint
SITE=deploy/azure/testdata/site-full.yaml` renders every tenant against a
fixture site file, with no cluster.

## Day two

- **After adding an agent**, run `make azure-identity WS=<tenant>`. Each agent
  runs as its own `<name>-facade` ServiceAccount and AKS takes no wildcard
  subject, so a new agent has no cloud access until it is federated (an
  `AADSTS700213` in its pod log). It also removes credentials of agents that
  are gone. An identity holds at most 20 credentials.
- **`make azure-tenant-remove WS=<tenant>`** uninstalls the workspace release.
  Its databases, volume content and identity stay.
- **`DRY_RUN=1`** before any target prints each write as `DRY_RUN: <command>`
  and runs none; reads still run.
- Every step checks before it writes, so running again changes nothing.
