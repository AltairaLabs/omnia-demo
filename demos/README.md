# Public demo examples

These directories hold each demo's PromptArena content: the Arena and Poppy
kits (configs, prompts, tools, scenarios, personas, providers and mock
responses), pack patches, canary evals, and the RUNBOOKs. The synthetic
services the demos call live in [`../services/`](../services/). Both are
intentionally separate from Omnia's platform repository and do not access
platform databases.

| Example | Contents | Requirements |
| --- | --- | --- |
| `sharepoint-adapter/` | Arena load-test and quality source for the SharePoint-grounded agent | Graph credentials and an Omnia institutional-memory endpoint |
| `mortgage-underwriting/` | Mortgage underwriting Arena source and runbook | Omnia Enterprise for policy enforcement |
| `incident-agent/` | Wrenmoor Energy on-call incident agent Arena source, with per-tool `action_scope` | Omnia Enterprise for policy enforcement |
| `data-analyst/` | Wrenmoor Energy grid data analyst Arena source | Omnia Enterprise for scheduled functions and policy |
| `field-service/` | Wrenmoor Energy field-service copilot Arena source | Omnia Enterprise institutional memory |
| `pii-chat/` | Synthetic card-support Arena source and privacy-governance declarations | Omnia Enterprise for privacy policy behavior |
| `claims-triage/` | Synthetic claims Arena source | Omnia Enterprise when policy enforcement is enabled |
| `personal-agent/` | Ellen's assistant Arena source, its stateful test fake (`arenafake/`) and runbook | Omnia Enterprise |

The services, under `../services/`:

| Service | Contents | Requirements |
| --- | --- | --- |
| `sharepoint-adapter/` | Microsoft Graph document adapter, Office extraction and synthetic corpus generator | Graph credentials and an Omnia institutional-memory endpoint |
| `memory-seeder/` | Synthetic multi-tier Galaxy data generator with consent categories | Omnia Enterprise, Workspace and AgentRuntime UIDs, the workspace memory API and its pseudonym key |
| `mortgage-underwriting/` | Synthetic case-system and income-verification services; the same binary serves the claims, identity and Wrenmoor Energy endpoints | Omnia Enterprise for policy enforcement |
| `demo-sites/` | The demo companies' customer-facing sites (Larkmere Mutual, Wrenmoor Energy) and assistant proxy, one image | The chat examples (piiChat, wren) |
| `personal-agent/` | The Personal Agent service Ellen's assistant calls | Omnia Enterprise |

The public Helm chart currently deploys the memory-ingestion example and the
optional Memory Galaxy seeder. The other examples are deliberately shipped as
source and Arena projects first; their platform resources still belong to the
Omnia Enterprise demo chart until the equivalent public chart values and
released CRD contract are documented. Each example remains independently
buildable and must document its image, secrets, Enterprise prerequisites, and
cleanup procedure before being added to the chart.

All data in these examples is fictional. Do not connect them to a real tenant
or commit credentials.
