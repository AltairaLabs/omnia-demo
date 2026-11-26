# Building a memory ingestion connector

This directory is a working example of the piece a customer writes to get their
own documents into Omnia's institutional memory: a **source connector**. It is
a few small Go files ([`main.go`](main.go), [`blob.go`](blob.go), [`forget.go`](forget.go)), and the Wrenmoor Energy
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
  "source": "blob",
  "documents": [
    {
      "title": "isolation-and-safety.md",
      "url": "https://acct.blob.core.windows.net/wrenmoor-energy-documents/isolation-and-safety.md",
      "site": "https://acct.blob.core.windows.net/wrenmoor-energy-documents",
      "text": "# Isolation and safety\n...",
      "modified_at": "2026-11-09T21:14:03Z",
      "content_hash": "sha256:9f2c..."
    }
  ],
  "present": {
    "kind": "document",
    "keys": ["https://acct.blob.core.windows.net/wrenmoor-energy-documents/isolation-and-safety.md", "..."]
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

memory-api answers `202` with what it did:

```json
{"accepted": 4, "skipped": 1, "forgotten": 0, "rejected": [], "embedding_backlog": 12}
```

`skipped` counts unchanged documents (same `content_hash`), `forgotten` the
documents the manifest removed, and `embedding_backlog` the chunks still to be
embedded: ingest does not wait for vectors. A document in `rejected` (for
example, a `url` another `source` already owns) fails this connector's run, so a
bad document is never silently missing from memory.

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
| `BLOB_ACCOUNT_URL` | for Blob | Storage account URL, for example `https://<account>.blob.core.windows.net` |
| `BLOB_CONTAINER` | for Blob | Container to read |
| `SOURCE_DIR` | no | Read files from this directory instead of Blob, for local runs |
| `FORGET_SOURCE` | no | Forget mode: remove every document stored for this source, then exit. Reads nothing; needs only `MEMORY_API_URL` and `WORKSPACE_ID` |
| `MEMORY_SOURCE` | no | The `source` name (default `blob`) |
| `MEMORY_TOKEN_PATH` | no | File holding the bearer token |

Blob access is keyless. With `AZURE_FEDERATED_TOKEN_FILE` set (the AKS workload
identity webhook injects it with `AZURE_CLIENT_ID`, `AZURE_TENANT_ID` and
`AZURE_AUTHORITY_HOST`) the connector uses the workload identity credential;
otherwise the default Azure credential chain (for example `az login`). The
identity needs `Storage Blob Data Reader` on the container. There is no
account-key, SAS or connection-string path.

```bash
go run .
```

The connector lists the whole container before it posts anything. A credential
or list failure fails the run with nothing sent, so it can never send an empty
manifest that would forget documents still in the container. A single blob that
can't be read is skipped with a warning but kept in the manifest, so memory-api
keeps the copy it already holds. An empty listing is a valid manifest, so a container that is genuinely empty
makes memory forget every document it held. `modified_at` is the blob's Last-Modified,
which moves only when the blob is edited, so re-runs skip unchanged documents.

To forget a source (for example after renaming `MEMORY_SOURCE`):

```bash
FORGET_SOURCE=old-source go run .
```

It accepts `.txt`, `.md`, `.markdown`, `.csv`, `.json`, `.yaml`, `.yml` and
`.log` files up to 16 MiB. A batch that hits a network error, a 429 or a 5xx is
retried twice with backoff; any other error fails the run at once with
memory-api's message. The run ends with one JSON log line:

```json
{"level":"INFO","msg":"sync complete","container":"https://acct.blob.core.windows.net/wrenmoor-energy-documents","documents":5,"accepted":5,"skipped":0,"forgotten":0,"embedding_backlog":31}
```

`documents` counts what the source listed; a gap between it and
`accepted` + `skipped` is files the connector skipped (wrong type, too large,
unreadable), each logged as `skip blob` or `skip file`.

## How the demo runs it

`charts/omnia-demo` with `memory.enabled` (Larkmere Mutual and Wrenmoor Energy):

1. `make azure-memory` (`deploy/azure/memory.sh`) creates a private Blob
   container, `<tenant>-documents`, in the storage account named by
   `memory.storageAccount` in `deploy/azure/site.yaml`, and uploads the
   tenant's `memory-documents/<tenant>/` into it: only files whose MD5 differs,
   and it deletes blobs with no file behind them. It grants the tenant's
   managed identity `Storage Blob Data Reader` on that one container. The
   container stands in for the customer's own document store.
2. `templates/ingest.yaml` renders a ServiceAccount and a CronJob, both named
   `memory-ingest`, that run this connector hourly (`memory.schedule`) as the
   tenant's workload identity, reading the container keylessly and posting to
   the tenant's memory-api. An upgrade never runs it. `make azure-memory`
   starts a run (`kubectl create job --from=cronjob/memory-ingest`) when the
   documents changed or the CronJob has never run; right after a new grant it
   waits instead, because the grant takes a few minutes to apply: run it again.
3. The agents recall those documents as institutional memory.

An empty container after a successful listing forgets every document of the
source. That is the design: the manifest is the truth about what the source
holds. `make azure-memory` therefore refuses to seed from an empty documents
directory. A failed listing or credential error sends nothing.

## Writing your own

The Omnia docs walk through the same pattern in
[Write an S3 memory populator](https://omnia.altairalabs.ai/how-to/memory/write-an-s3-populator/).
In short: copy the package and replace the source side ([`blob.go`](blob.go)):

- `syncBlob` lists the source, then calls `readBlob` for each item. For
  SharePoint that is a Graph drive listing; for Confluence, a CQL search.
- Each item becomes a `document`: title, a stable `url`, plain text and a hash
  of the content.

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
