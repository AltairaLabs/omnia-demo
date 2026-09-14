# Public demo examples

These directories contain the source and synthetic services used by the
optional examples. They are intentionally separate from Omnia's platform
repository and do not access platform databases.

| Example | Contents | Requirements |
| --- | --- | --- |
| `sharepoint-adapter/` | Microsoft Graph document adapter, Office extraction, synthetic corpus generator | Graph credentials and an Omnia institutional-memory endpoint |
| `mortgage-underwriting/` | Synthetic case-system and income-verification services plus Arena source | Omnia Enterprise for policy enforcement |
| `pii-chat/` | Synthetic card-support Arena source and privacy-governance declarations | Omnia Enterprise for privacy policy behavior |
| `claims-triage/` | Synthetic claims Arena source | Omnia Enterprise when policy enforcement is enabled |
| `larkmere-portal/` | Fictional member-portal frontend and proxy | The corresponding chat example |

The deployment wiring for these examples will live in the public demo Helm
chart. Each example must remain independently gated through values and must
document its image, secrets, Enterprise prerequisites, and cleanup procedure.

All data in these examples is fictional. Do not connect them to a real tenant
or commit credentials.

