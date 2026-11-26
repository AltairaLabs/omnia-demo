# Company documents come from Azure Blob, not the chart

Status: draft for approval, 2026-11-26.

## Intent

Larkmere's claims procedures and Wrenmoor's field manuals reach institutional
memory the way a real company's would. They live in a document store the
company owns. A connector the company runs reads that store on a schedule, and
sends only what changed, with each document's real modified time.

Today they don't. The documents are chart files baked into a ConfigMap, and a
Helm hook copies them into an in-cluster S3 gateway (Versity) on every
upgrade. That bumps each object's modified time, so memory-api re-embeds every
document on every `make demos`. It costs money and it is nothing like real
life. A ConfigMap is not a document store, and S3 has no place in an Azure
demo.

The memory demo stays optional: a tenant with no memory block, or a site.yaml
with no `memory` block, deploys without it.

## Goals

- Each memory tenant (today larkmere-mutual and wrenmoor-energy) has a private
  Blob container in a storage account the deployer brings.
- `make azure-memory` creates the containers if absent and seeds them from the
  repo's documents. It uploads a file only when its content differs from the
  blob, so the blob's last-modified time moves only when a document is edited.
  It grants the tenant's identity `Storage Blob Data Reader` on its own
  container, and nothing wider.
- `memory-ingestion` reads Blob keylessly through the tenant's workload
  identity. It sends the blob's last-modified time as `modified_at` and a
  content hash, and sends the `present` manifest, so a deleted blob is
  forgotten.
- The connector runs as a CronJob (default hourly), not a Helm hook, so
  upgrades never trigger it. `make azure-memory` also starts one run at once,
  so a fresh install doesn't wait an hour.
- Re-running anything on unchanged documents re-embeds nothing (`skipped`
  equals the document count).
- Removed: the in-cluster S3 gateway, its credentials, the uploader (including
  the unreviewed commit 3c7d81b, reverted), the fixtures ConfigMap, the S3
  code path in memory-ingestion, and the Helm-hook ingest Job.

## Non-goals

- SharePoint. The repo's `sharepoint-adapter` stays as it is.
- Other clouds' object stores.
- The memory Galaxy seeder (`demos/memory-galaxy`). It writes generated
  memories directly and is a separate demo.

## Anti-goals

- A public or anonymously readable container.
- An account key or SAS anywhere: in a Secret, in site.yaml, or in a script.
- Granting the tenant identity anything beyond read on its own container.

## Approaches considered

- **Each deployer's own private container (chosen by the user).** This is
  what a real company does, and it shows the keyless path.
- **A public container we own, read anonymously.** Rejected: it ties every
  deployment to our account, and it drops the identity story.
- **Someone else's public corpus** (AWS Open Data, Azure Open Datasets, NASA).
  Rejected: the agents answer from these Companies' own documents, and the
  repo uses synthetic data only.

## Design

- **site.yaml (optional block):** `memory.storageAccount` (name) and
  `memory.resourceGroup`. Containers are named `<tenant>-documents`. Without
  the block, `site-values.sh` turns memory off for every tenant, and `make
  azure-memory` says there is nothing to do.
- **Documents:** they move out of `charts/omnia-demo/files/fixtures/` to
  `memory-documents/<tenant>/`. They are the seed, not chart content.
- **`make azure-memory`** (deploy/azure/memory.sh): every write goes through
  `run`, with DRY_RUN support, the subscription after the command, and the
  same fake-binary tests as the other scripts. The tenants are the ones whose
  profile sets `memory.enabled: true`.
- **Uploads:** compare the local MD5 with the blob's `Content-MD5` and upload
  only on a difference. Delete blobs whose document left the repo. Use
  `--auth-mode login`.
- **Connector:** a new Blob source (Azure SDK for Go, `azblob` plus a workload
  identity credential), configured by `BLOB_ACCOUNT_URL` and `BLOB_CONTAINER`.
  The directory source stays for local runs and tests. The S3 source goes.
- **Chart:** the `memory` values take `source: blob`, the account URL, the
  container and a schedule. They render a CronJob whose ServiceAccount
  `memory-ingest` carries the workload identity annotations and label.
  `identity.sh` federates `memory-ingest` (a fixed SA it names, as for the
  others).
- **Migration on our deployment:** the URLs change from `s3://omnia-memory/…`
  to `https://<account>.blob.core.windows.net/…`. The documents are therefore
  embedded once more, 8 in all, and the old `s3` source must be forgotten. The
  connector gets a one-shot `FORGET_SOURCE=s3` mode, sending an empty `present`
  manifest for that source, which runs once during the migration.
- **Storage account for our deployment:** it has none suitable. The NFS one is
  a FileStorage account with no Blob. Creating one is a new Azure resource, so
  the controller asks the maintainer first.

## Left to the builder

The CronJob's concurrency and history settings. Whether the blob MD5 compare
uses `az storage blob show` or a list call. The Go package layout for the
Blob source.
