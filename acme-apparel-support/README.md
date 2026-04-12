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
| `skills/memory-protocol/SKILL.md` | Preloaded: memory recall/remember protocol for cross-session context |
| `skills/guardrails/SKILL.md` | Preloaded: hard agent boundaries (no refunds, no account mods) |
| `skills/escalation-policy/SKILL.md` | On-demand: when and how to escalate to human specialists |
| `skills/tool-guide/SKILL.md` | On-demand: how to use lookup_order, lookup_customer, search_kb, issue_discount_code |
| `skills/business-context/SKILL.md` | On-demand: Acme Apparel policies, hours, shipping/returns rules |

## Skills

The system prompt is intentionally lean — it only defines the agent's identity and voice
(warm/empathetic for variant A, direct/confident for variant B). All behavioral knowledge
is provided through skills:

**Preloaded** (active from turn 1):
- `memory-protocol` — forces `memory__recall` before first response, guides `memory__remember` usage
- `guardrails` — hard boundaries the agent must never cross (no refunds, no account mods, escalate chargebacks)

**On-demand** (model activates via `skill__activate` when relevant):
- `escalation-policy` — when and how to hand off to a human specialist
- `tool-guide` — how to use each Shopify support tool effectively
- `business-context` — shipping policy, returns policy, support hours

This decomposition means both prompt variants share the same behavioral skills — only
the voice differs. An operator can swap a skill (e.g., different escalation SLA) without
touching the prompt.

### Current test status

All 5 scenarios pass 9/9 assertions against gpt-4.1 with skills active:

| Scenario | Assertions | Notes |
|---|---|---|
| `smoke-test-single-turn` | 1/1 | `lookup_order` called after user mentions order |
| `hero-delayed-shipment` | 3/3 | `lookup_order` + tracking info surfaced + `issue_discount_code` |
| `hero-marcus-escalation` | 3/3 | `escalate_to_human` + specialist mention + no false refund claims |
| `hero-memory-recall` | 2/2 | `memory__recall` called (preloaded memory-protocol skill) + warm response |
| `selfplay-mixed-personas` | 1/1 | Tool efficiency within limits across 6 personas |

## Hero demo persona usage

- **Sarah Chen**: Scene 2 (delayed shipment) and Scene 3 (returning thanks). Same persona, different scenario situations — memory recall makes Scene 3 work.
- **Marcus Webb**: Scene 4 (chargeback escalation). Tests that the agent always escalates chargebacks via `escalate_to_human`.
- **Emma, Kai, Priya, Alex**: not on camera in the hero demo. They provide variety in operator-demo self-play traffic so Grafana dashboards show diverse tool-call patterns.

## Prerequisites

Install the PromptKit tools from source:

```bash
go -C ../promptkit install ./tools/arena/cmd/promptarena
go -C ../promptkit install ./tools/packc
```

For Shopify store access (SH2/SH3):

```bash
npm install -g @shopify/cli
shopify auth login
shopify store auth --store acme-apparel-omnia-demo.myshopify.com \
  --scopes read_products,write_products,read_customers,write_customers,\
read_orders,write_orders,write_discounts,read_discounts,\
write_price_rules,read_price_rules,write_draft_orders
```

The `shopify store auth` command creates an OAuth `shpat_` Admin API token stored at `~/Library/Preferences/shopify-cli-store-nodejs/config.json`. Re-run the command to refresh an expired token.

## Running locally

### Environment

Arena scenarios need API keys for the providers they use. Set `OPENAI_API_KEY` for `openai-direct` (gpt-4.1). The `azure-gpt4o` provider uses Azure platform auth (Managed Identity, Azure CLI, or `AZURE_CLIENT_ID`/`AZURE_TENANT_ID`/`AZURE_CLIENT_SECRET`) — no separate API key needed.

```bash
export OPENAI_API_KEY=sk-...
```

### Validate and compile

```bash
# Validate the arena config + all referenced files
promptarena validate acme-apparel-support/config.arena.yaml

# Compile the pack into deployable JSON (includes skills)
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  -o build/acme-apparel-support.pack.json

# Validate the compiled pack
packc validate build/acme-apparel-support.pack.json

# Inspect pack contents (prompts, tools, evals, skills)
packc inspect build/acme-apparel-support.pack.json
```

If schema validation fails with "Additional property not allowed" on a field that exists in your local PromptKit schema, run the above commands with `PROMPTKIT_SCHEMA_SOURCE=local` from the PromptKit repo root:

```bash
cd ../promptkit && PROMPTKIT_SCHEMA_SOURCE=local packc compile -c ../omnia-demo/acme-apparel-support/config.arena.yaml --id acme-apparel-support -o ../omnia-demo/build/acme-apparel-support.pack.json
```

### Run scenarios

```bash
# Single scenario against mock provider (no API key needed, fast)
promptarena run \
  -c acme-apparel-support/config.arena.yaml \
  --scenario smoke-test-single-turn \
  --provider mock

# Single scenario against real LLM (needs OPENAI_API_KEY)
promptarena run \
  -c acme-apparel-support/config.arena.yaml \
  --scenario smoke-test-single-turn \
  --provider openai-direct

# All scenarios against real LLM
promptarena run \
  -c acme-apparel-support/config.arena.yaml \
  --provider openai-direct

# Add -v for verbose debug logging
```

Results land in `out/report.html` by default.

### Seed the Shopify store

The dev store at `acme-apparel-omnia-demo.myshopify.com` needs demo data (products, customers, orders). The seed script creates everything via GraphQL:

```bash
./scripts/seed-data/create-all.sh
```

Requires `shopify store auth` (see Prerequisites). Idempotent for products and customers; orders will duplicate on re-run. See `scripts/seed-data/` for individual mutation files.

### Known issues

- **Schema validation**: the published schema at `https://promptkit.altairalabs.ai/schemas/v1alpha1/` lags the PromptKit `main` branch. If `promptarena validate` or `packc compile` fails with "Additional property X is not allowed" on a field that exists in the local schema, run with `PROMPTKIT_SCHEMA_SOURCE=local` from the PromptKit repo directory until the published schema is updated.
