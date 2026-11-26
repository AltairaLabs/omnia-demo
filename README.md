# Omnia demos

Public demo content and deployable examples for [Omnia](https://github.com/AltairaLabs/Omnia).

This repository contains synthetic data, PromptKit/Arena content, and optional
Helm add-ons. Omnia itself remains in the platform repository. Each add-on
documents its prerequisites and can be enabled or disabled through values.

## Examples

- [`demos/`](demos/) — PromptArena sources and synthetic back-end services for
  the demo cast's agents: Larkmere Mutual (mortgage underwriting, claims
  triage, card support) and Wrenmoor Energy (on-call incident agent, grid data
  analyst, field-service copilot).
- [`memory-ingestion/`](memory-ingestion/) — a public Azure Blob source
  connector that sends documents to Omnia institutional memory.
- [`memory-documents/`](memory-documents/) — each company's synthetic documents,
  one directory per tenant. `make azure-memory` seeds them into that tenant's
  private Blob container.
- [`charts/omnia-demo/`](charts/omnia-demo/) — the Helm add-on that deploys the
  demos, one release per tenant, with the memory ingestion example (a CronJob
  that reads the tenant's Blob container).

- [`profiles/`](profiles/) — per-platform values for the demo tenants (one
  Workspace per company in the cast). `profiles/azure/` targets an Omnia
  install on AKS.

## Installing the demo tenants

The demos deploy onto an Omnia you already run on AKS. Describe it in
`deploy/azure/site.yaml` (copy [`site.example.yaml`](deploy/azure/site.example.yaml)),
then:

```bash
make omnia-values                 # values to add to your Omnia install, and its TLS hosts
make azure-tenant WS=ellens-assistant   # the assistant's namespace ...
hack/personal-agent-keys.sh             # ... then its keys
make images                       # build the demo images into your registry
make demos                        # every tenant (larkmere-mutual, wrenmoor-energy, ellens-assistant, halvern-partners, halvern-research)
make demos TENANTS=larkmere-mutual
make azure-poppy                  # DNS records and the Key Vault key (after demos: it reads the tenants' identities)
make azure-memory                 # the company documents (optional; needs a memory block)
make builder                      # the Arena builder, in Halvern Partners
```

For each tenant, `make demos`:
1. makes the workspace's Azure side (`make azure-tenant WS=<tenant>`): its
   namespace, databases, Foundry Providers, content volume and managed identity;
2. installs `charts/omnia-demo` with `profiles/azure/common.yaml`, the tenant's
   file and your site's values, putting that identity on every agent;
3. federates each agent to the identity (`make azure-identity WS=<tenant>`).

The order of a first install, the Azure resources and rights it needs, and the
day-two operations are in [`deploy/azure/README.md`](deploy/azure/README.md).
The demos were first run on the maintainers' own deployment; nothing in this
repository is specific to it.

More public-safe examples will be added as independent packages. Enterprise
examples may require an Omnia Enterprise installation; they will not copy
private Omnia platform code into this repository.

## Safety and scope

All examples use fictional names, synthetic documents, and development-only
defaults. Supply real credentials through Kubernetes Secrets or environment
variables; never commit them. See [CONTRIBUTING.md](CONTRIBUTING.md) for the
public-content rules and review requirements.

## License

Unless a file states otherwise, the original content in this repository is
Apache-2.0. Third-party assets retain their respective licenses.
