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
- [`memory-ingestion/`](memory-ingestion/) — a public S3-compatible source
  connector that sends documents to Omnia institutional memory.
- [`charts/omnia-demo/`](charts/omnia-demo/) — the Helm add-on that deploys the
  demos, one release per tenant, with the memory ingestion example, local S3 gateway (Versity Gateway)
  and synthetic fixtures.

- [`profiles/`](profiles/) — per-platform values for the demo tenants (one
  Workspace per company in the cast). `profiles/azure/` targets an
  [omnia-azure](https://github.com/AltairaLabs/omnia-azure) deployment.

## Installing the demo tenants

```bash
make demos                        # every tenant (larkmere-mutual, wrenmoor-energy, halvern-partners)
make demos TENANTS=larkmere-mutual
make builder                      # the Arena builder, in Halvern Partners
```

For each tenant, the azure profile:
1. asks a sibling omnia-azure checkout (`OMNIA_AZURE`, default `../omnia-azure`)
   to provision the workspace's Azure side: `make workspace WS=<tenant>`
   creates its namespace, databases, Foundry Providers, content volume and
   managed identity;
2. installs `charts/omnia-demo` with `profiles/azure/common.yaml` and the
   tenant's file, putting that identity on every agent;
3. has omnia-azure federate each agent (`make workspace-identity`).

omnia-azure knows nothing about the demos; the cast and what each tenant runs
live here.

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
