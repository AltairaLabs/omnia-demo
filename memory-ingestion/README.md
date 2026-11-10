# Building a memory ingestion connector

This directory is a working example of the piece a customer writes to get their
own documents into Omnia's institutional memory: a **source connector**. It is
one Go file ([`main.go`](main.go)), under 300 lines, and the Wrenmoor Energy
demo runs it to load the field manuals its field-service copilot answers from.

Use it as the template for a connector to SharePoint, Confluence, a DMS, a
ticketing system or a file share. The source changes; the boundary and the
contract below do not.

## The boundary

A connector owns the **source**: authenticating to it, listing what is there,
and extracting text. Omnia's memory-api owns the **memory**: chunking, the
document → section → chunk hierarchy, embeddings, relations, deduplication and
retrieval.

So the connector has no database access, no embedding model and no vector
store. It sends whole documents over one HTTP endpoint and is done. That is the
production pattern, not a demo shortcut: a connector built this way can't
corrupt memory, needs no migration when Omnia's storage changes, and can be
written by a team that has never seen Omnia's internals.

## The contract

The contract is documented in the Omnia docs, which are the reference:
[Ingest documents](https://omnia.altairalabs.ai/how-to/memory/ingest-documents/)
covers the request, its limits (100 documents or 5,000 items a request),
pre-split `sections[]`, structured `items[]` and `edges[]` for sources that
already know their structure, forgetting a whole source, and troubleshooting.
The [Memory API reference](https://omnia.altairalabs.ai/reference/platform/memory-api/)
covers authentication and scope. What this connector uses of it is one endpoint
on the workspace's memory-api:

```
POST <memory-api>/api/v1/institutional/ingest/batch
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "workspace_id": "wrenmoor-energy",
  "source": "wrenmoor-field-procedures",
  "documents": [
    {
      "title": "isolation-and-safety.md",
      "url": "s3://omnia-demo-memory/wrenmoor/isolation-and-safety.md",
      "text": "# Isolation and safety\n...",
      "modified_at": "2026-11-09T21:14:03Z",
      "content_hash": "sha256:9f2c..."
    }
  ],
  "present": {
    "kind": "document",
    "keys": ["s3://omnia-demo-memory/wrenmoor/isolation-and-safety.md", "..."]
  }
}
```

| Field | What it is for |
|---|---|
| `source` | Names this connector's documents. Everything below is scoped to it, so two connectors never step on each other. |
| `url` | The document's identity within the source, and the link an agent cites. Stable across runs. |
| `content_hash` | Makes re-runs cheap and safe: an unchanged document is skipped. |
| `modified_at` | Optional. Kept with the memory for recency. |
| `present` | Sent on the **final** batch only: every `url` the source holds now. Documents of this source that are not listed are forgotten, so a deleted file leaves memory too. |

`text` is split by memory-api on Markdown headings; a source that already knows
its headings can send `sections[]` instead (see the docs). Send documents in
batches (this example sends 20) and put `present` on the last request. A run that fails part-way leaves memory as it was for anything not yet
sent, and the next run picks up where the hashes say it should.

## Authentication

The connector presents a bearer token from a file (`MEMORY_TOKEN_PATH`), read
on every request so a rotated token is picked up.

In the demo chart that file is a projected Kubernetes ServiceAccount token
bound to the audience memory-api enforces (`memory.serviceAuth.audience`,
`omnia-session-api`), which is Omnia's internal service auth: no secret to
create or rotate. Outside the cluster, point `MEMORY_TOKEN_PATH` at a token
issued for the workspace (`memory.auth.secretName` in the chart).

## Running it

| Variable | Required | Meaning |
|---|---|---|
| `MEMORY_API_URL` | yes | Base URL of the workspace's memory-api |
| `WORKSPACE_ID` | yes | Workspace name or UID |
| `S3_BUCKET` | for S3 | Bucket to read |
| `S3_PREFIX`, `S3_REGION`, `S3_ENDPOINT`, `S3_FORCE_PATH_STYLE` | no | Where and how to reach it. Credentials use the standard AWS SDK chain: an IAM role, environment variables, a profile, or any S3-compatible store. |
| `SOURCE_DIR` | no | Read files from this directory instead of S3, for example a mounted ConfigMap |
| `MEMORY_SOURCE` | no | The `source` name (default `s3`) |
| `MEMORY_TOKEN_PATH` | no | File holding the bearer token |

```bash
go run .
```

It accepts `.txt`, `.md`, `.csv`, `.json`, `.yaml`, `.yml` and `.log` files up
to 16 MiB, and retries each batch three times with backoff.

## How the demo runs it

`charts/omnia-demo` with `memory.enabled` (the Wrenmoor Energy tenant):

1. `templates/object-store.yaml` runs [Versity Gateway](https://github.com/versity/versitygw),
   an S3-compatible store, in the tenant's namespace. It stands in for the
   customer's own bucket.
2. `templates/ingest.yaml` is a Job that runs after every install and upgrade.
   An init container uploads the chosen fixture set
   (`memory.fixtures`, here `files/fixtures/wrenmoor-field-procedures/`) into
   the bucket, then this connector reads it back out and posts it to the
   tenant's memory-api.
3. The field-service agent recalls those documents as institutional memory
   when an engineer asks about a procedure.

With `memory.objectStore.enabled: false` the Job skips the store and reads the
fixtures straight from their ConfigMap (`SOURCE_DIR`): the same connector and
contract, without S3.

## Writing your own

The Omnia docs walk through the same pattern in
[Write an S3 memory populator](https://omnia.altairalabs.ai/how-to/memory/write-an-s3-populator/).
In short: copy `main.go` and replace the two source functions:

- `sync` lists the source page by page and calls `read` for each item. For
  SharePoint that is a Graph drive listing; for Confluence, a CQL search.
- `read` turns one item into a `document`: title, a stable `url`, plain text and
  a hash of the content.

Keep `post` and the batch shape as they are. Then add what a real source
needs: MIME-aware extraction (PDF, DOCX, HTML), a durable cursor so a large
source isn't relisted from scratch, bounded concurrency, metrics, and a
dead-letter path for items that won't extract. Don't write to the memory
database directly: the batch-ingest contract is the versioned interface.

Institutional ingestion requires an Omnia Enterprise deployment with the
institutional ingest endpoint enabled. This repository contains no Enterprise
code or private package dependency.

## Image

`.github/workflows/publish.yml` builds `ghcr.io/altairalabs/omnia-demo-memory-ingestion`
on pushes to `main` (tag `latest`), on `v*` tags, and on manual runs (tag
`sha-<short>`). For local development:

```bash
docker build -t omnia-demo-memory-ingestion:dev memory-ingestion
```

and set `memory.image.repository` and `memory.image.tag` to use it.
