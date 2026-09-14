# Omnia demos

Public demo content and deployable examples for [Omnia](https://github.com/AltairaLabs/Omnia).

This repository contains synthetic data, PromptKit/Arena content, and optional
Helm add-ons. Omnia itself remains in the platform repository. Each add-on
documents its prerequisites and can be enabled or disabled through values.

## Examples

- [`acme-apparel-support/`](acme-apparel-support/) — synthetic customer-support
  agent content, tools, personas, scenarios, and knowledge-base articles.
- [`memory-ingestion/`](memory-ingestion/) — a public S3-compatible source
  connector that sends documents to Omnia institutional memory.
- [`charts/omnia-demo/`](charts/omnia-demo/) — the Helm add-on for the memory
  ingestion example, including local MinIO and synthetic fixtures.

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
