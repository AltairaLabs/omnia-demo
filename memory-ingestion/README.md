# S3 memory ingestion example

This is a public, source-only example of an ingestion service for Omnia. It
lists text-like objects under an S3 prefix, reads them, hashes their contents,
and sends them to Omnia's institutional batch-ingest endpoint.

The service deliberately has no database or memory-store access. Omnia owns
chunking, document/section/chunk hierarchy, embeddings, relations, idempotency,
and retrieval. That separation is the important production pattern: a source
connector owns source authentication and extraction; the memory service owns
memory semantics.

## Run locally

```bash
go run .
```

Required variables:

| Variable | Meaning |
| --- | --- |
| `MEMORY_API_URL` | Base URL of the workspace memory-api |
| `WORKSPACE_ID` | Workspace name or UID |
| `S3_BUCKET` | Bucket to scan |

Optional variables include `S3_PREFIX`, `S3_REGION`, `S3_ENDPOINT`,
`S3_FORCE_PATH_STYLE`, `MEMORY_SOURCE`, and `MEMORY_TOKEN_PATH`. AWS credentials
use the standard AWS SDK chain, so this works with an IAM role, environment
credentials, a local profile, MinIO, or LocalStack.

The example accepts `.txt`, `.md`, `.csv`, `.json`, `.yaml`, `.yml`, and `.log`
files up to 16 MiB. It sends batches of 20 documents and includes a manifest on
the final request so removed source objects can be forgotten. Re-running the
same source is safe because each document includes a SHA-256 `content_hash`.

## Production extensions

Keep the same boundary, then add MIME-aware extraction, structure-aware section
discovery, a durable source cursor, bounded concurrency, metrics, dead-letter
handling, and a queue if the source is large. Do not write directly to the
memory database: use the versioned batch-ingest contract.

Institutional ingestion requires an Omnia Enterprise deployment with the
institutional ingest endpoint enabled. The repository contains no Enterprise
implementation or private package dependency.
