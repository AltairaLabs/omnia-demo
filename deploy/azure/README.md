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
| External Secrets, if the Postgres password lives in a vault | otherwise a plain Secret holds it | `postgres.adminPassword` |
| NFS storage account (optional) | a shared content volume per tenant | `nfs.*` |
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
| `azure-tenant` | `az identity create` in `identityResourceGroup`; `az role assignment create` of `Cognitive Services OpenAI User` on the Foundry (and speech) account | Managed Identity Contributor on the resource group; User Access Administrator (or Owner) on the Foundry and speech accounts |
| `azure-identity` | `az aks show` (reads the OIDC issuer); `az identity federated-credential list/create/delete` | Reader on the cluster; Managed Identity Contributor on the identity resource group |
| `azure-poppy` | `az network dns record-set a show/add-record/delete` in the zone (its subscription is `dns.subscription`); `az network public-ip show` if you name a public IP; `az keyvault show`; `az rest --method put` of the Key Vault key `omnia-connections` through ARM; `az role assignment create` of `Key Vault Crypto User` on that key | DNS Zone Contributor on the zone; Reader on the vault and public IP; Key Vault Contributor on the vault; User Access Administrator on the key |
| `images` | `az acr build` | Contributor (or ACR Tasks rights) on the registry |

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
3. **`make azure-poppy`** makes the DNS records, the Key Vault key and the right
   to use it. It is safe to run again.
4. **`make azure-tenant WS=ellens-assistant`**, then
   **`hack/personal-agent-keys.sh`**. The assistant's namespace must exist
   before its keys, and its Deployment cannot start without them. The script
   reads the kube context from `CONTEXT`, `KUBE_CONTEXT` or `aks.kubeContext`.
5. **`make images`** builds the demo images into your registry and writes
   `.images.env`. Skip it only if you pin published images in the profiles.
   Nothing is published from this repository.
6. **`make demos`** installs every tenant: its Azure side, the chart, then
   federation of its agents. `make demos TENANTS=larkmere-mutual` does one.

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
