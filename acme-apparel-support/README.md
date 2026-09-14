# Acme Apparel Support

Public PromptKit/Arena source for a fictional customer-support agent.

The pack includes two prompt variants, synthetic personas, mock tools,
regression scenarios, providers, and reusable skills. It is intended to be
compiled with the PromptKit tooling and then deployed to an Omnia installation.

## Contents

- `config.arena.yaml` — pack manifest
- `prompts/` — agent prompt variants and fragments
- `personas/` — synthetic self-play customers
- `tools/` — mock tool schemas and fixtures
- `scenarios/` — deterministic and self-play scenarios
- `providers/` — local/mock provider examples
- `skills/` — support behavior and policy guidance

## Local validation

Install the PromptKit CLI from the [PromptKit repository](https://github.com/AltairaLabs/PromptKit),
then run:

```bash
promptarena validate acme-apparel-support/config.arena.yaml
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  -o build/acme-apparel-support.pack.json
packc validate build/acme-apparel-support.pack.json
```

The examples use fictional data and mock tools. Configure any live provider
credentials through environment variables; do not commit credentials or
customer data.
