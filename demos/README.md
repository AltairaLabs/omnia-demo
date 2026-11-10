# Public demo examples

These directories contain the source and synthetic services used by the
optional examples. They are intentionally separate from Omnia's platform
repository and do not access platform databases.

| Example | Contents | Requirements |
| --- | --- | --- |
| `sharepoint-adapter/` | Microsoft Graph document adapter, Office extraction, synthetic corpus generator, and Arena load-test source | Graph credentials and an Omnia institutional-memory endpoint |
| `memory-seeder/` | Synthetic multi-tier Galaxy data generator with consent categories | Omnia Enterprise, Workspace and AgentRuntime UIDs, the workspace memory API and its pseudonym key |
| `mortgage-underwriting/` | Synthetic case-system and income-verification services plus Arena source; the same binary serves the claims, identity and Wrenmoor Energy endpoints | Omnia Enterprise for policy enforcement |
| `incident-agent/` | Wrenmoor Energy on-call incident agent Arena source, with per-tool `action_scope` | Omnia Enterprise for policy enforcement |
| `data-analyst/` | Wrenmoor Energy grid data analyst Arena source | Omnia Enterprise for scheduled functions and policy |
| `field-service/` | Wrenmoor Energy field-service copilot Arena source | Omnia Enterprise institutional memory |
| `pii-chat/` | Synthetic card-support Arena source and privacy-governance declarations | Omnia Enterprise for privacy policy behavior |
| `claims-triage/` | Synthetic claims Arena source | Omnia Enterprise when policy enforcement is enabled |
| `demo-sites/` | The demo companies' customer-facing sites (Larkmere Mutual, Wrenmoor Energy) and assistant proxy, one image | The chat examples (piiChat, wren) |

The public Helm chart currently deploys the memory-ingestion example and the
optional Memory Galaxy seeder. The other examples are deliberately shipped as
source and Arena projects first; their platform resources still belong to the
Omnia Enterprise demo chart until the equivalent public chart values and
released CRD contract are documented. Each example remains independently
buildable and must document its image, secrets, Enterprise prerequisites, and
cleanup procedure before being added to the chart.

All data in these examples is fictional. Do not connect them to a real tenant
or commit credentials.
