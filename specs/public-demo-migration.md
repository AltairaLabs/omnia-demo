# Public demo migration map

The public `omnia-demo` repository contains demo content and deployable,
public-safe add-ons. Omnia remains the private platform repository. The public
repository must never become a copy of Omnia's private `charts/omnia-demos`
umbrella chart.

## Migration status

| Demo | Source in private Omnia | Public status | Public extraction boundary |
| --- | --- | --- | --- |
| Memory ingestion | `demos/s3-memory-populator`, memory chart | Done | Apache-2.0 source connector, synthetic fixtures, standalone add-on chart |
| SharePoint RAG | `demos/sharepoint-adapter`, `charts/omnia-demos/templates/sharepoint-hero-*` | Planned | Synthetic SharePoint-shaped documents and configurable adapter; no tenant data or private middleware |
| Mortgage underwriting | `demos/mortgage-underwriting`, `charts/omnia-demos/templates/mortgage-*` | Planned | Synthetic case data and Arena content; Enterprise ToolPolicy is a documented prerequisite |
| PII chat | `demos/pii-chat`, `charts/omnia-demos/templates/pii-chat-*` | Planned | Synthetic PII only; demonstrate storage-time policy behavior, never claim inbound redaction |
| Claims triage | `demos/claims-triage`, `charts/omnia-demos/templates/claims-triage-*` | Planned | Synthetic claims and public Arena content; private runtime remains in Omnia |
| Larkmere portal | `demos/larkmere-portal` | Planned | Synthetic customer-facing portal assets; no customer branding, credentials, or internal services |

Each planned demo should land as a reviewable slice with:

1. an isolated Helm values gate;
2. synthetic fixtures and a local deterministic path where practical;
3. a README covering prerequisites, expected behavior, and cleanup;
4. CI validation for source and Helm rendering; and
5. a pinned or explicitly configurable image built from this repository.

Enterprise demos may depend on a released Omnia Enterprise installation, but
their public content must not depend on private Omnia source files being copied
here.

