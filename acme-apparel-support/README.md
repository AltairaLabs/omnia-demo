# Acme Apparel Support — Arena Sources

PromptKit arena source tree for the Acme Apparel customer-support pack, compiled by `packc compile` into the PromptPack JSON that Omnia's `PromptPack` CRD consumes.

**Pack ID**: `acme-apparel-support` (derived from this folder's name).

**Design**: see `../specs/2026-04-11-arena-native-content-design.md` for the rationale, the pack-level eval decisions, the variant A / variant B fork approach, and the upstream packc metadata gap.

## Contents

| Path | Contents |
|---|---|
| `config.arena.yaml` | `kind: Arena` — pack manifest + run config. Lists prompts, tools, providers, pack-level evals, personas, scenarios, self-play roles, and run defaults. |
| `prompts/variant-a-agent.yaml` | Variant A PromptConfig (warm/empathetic baseline). Hero demo primary agent. |
| `prompts/variant-b-agent.yaml` | Variant B PromptConfig (less-apologetic / confident). Operator demo Act 2 A/B target per D6. |
| `prompts/fragments/variant-a/*.txt` | Four fragment bodies referenced by variant A's `spec.fragments[]`. |
| `prompts/fragments/variant-b/*.txt` | Same four fragment names, less-apologetic wording for variant B. |
| `personas/*.persona.yaml` | Six `kind: Persona` files driving self-play customer turns. |
| `tools/*.tool.yaml` | Five `kind: Tool` schemas with `mode: mock` fixtures: lookup_order, lookup_customer, search_kb, issue_discount_code, escalate_to_human. |
| `scenarios/hero-*.scenario.yaml` | Three scripted hero demo scenes as runnable regression tests. |
| `scenarios/selfplay-mixed-personas.scenario.yaml` | Self-play template cycling through all six personas; repeated continuously by the operator demo ArenaJob (O3). |
| `providers/azure-gpt4o.provider.yaml` | Primary agent-role provider (Azure OpenAI GPT-4o). Replace the `REPLACE-ME` URL for your environment. |
| `providers/ollama-local.provider.yaml` | Cheap customer-simulation provider for self-play. |

## Hero demo persona usage

- **Sarah Chen**: Scene 2 (delayed shipment) and Scene 3 (returning thanks). Same persona, different scenario situations — memory recall makes Scene 3 work.
- **Marcus Webb**: Scene 4 (chargeback escalation). Tests that the agent always escalates chargebacks via `escalate_to_human`.
- **Emma, Kai, Priya, Alex**: not on camera in the hero demo. They provide variety in operator-demo self-play traffic so Grafana dashboards show diverse tool-call patterns.

## Running locally

```bash
# Validate the sources
promptarena validate acme-apparel-support/config.arena.yaml

# Compile the pack (note: no --compiler-version flag — current packc
# does not accept it)
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  -o build/acme-apparel-support.pack.json

# Run a scenario end-to-end
promptarena run -c acme-apparel-support/config.arena.yaml --scenario hero-delayed-shipment
```

Results land in `out/report.html` by default.
