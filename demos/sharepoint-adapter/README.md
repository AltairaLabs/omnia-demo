# SharePoint document adapter

This example shows how to build a source connector that reads documents from
Microsoft Graph, extracts useful structure from Office files, and sends a
document batch to Omnia institutional memory.

The adapter has two modes:

- `serve` exposes a small HTTP tool surface for document lookup;
- `seed` performs a one-shot sync of the configured site.

Configure Graph credentials, the site ID, the memory API URL, and the
workspace through environment variables. Use a Kubernetes Secret or the
standard cloud credential chain for credentials; never place them in this
repository. The included sample corpus is synthetic.

The adapter is intentionally source-specific example code. It does not access
Omnia databases and does not replace the memory service's ingestion or
chunking logic.

