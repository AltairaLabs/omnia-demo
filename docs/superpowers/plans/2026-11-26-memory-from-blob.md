# Company documents from Azure Blob: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Larkmere's and Wrenmoor's documents reach institutional memory from
a private Blob container the deployer owns, through a scheduled keyless
connector that re-embeds nothing unchanged.

**Architecture:** Documents move to `memory-documents/<tenant>/` as the seed.
`make azure-memory` creates containers, seeds only changed files and grants
read access. `memory-ingestion` gains a Blob source and a one-shot forget
mode, and loses S3. The chart renders a CronJob in place of the Helm-hook Job
and the S3 gateway.

**Tech Stack:** Go (Azure SDK for Go: azblob, azidentity), Helm, bash, `az`.

**Spec:** `docs/superpowers/specs/2026-11-26-memory-from-blob-design.md`

## Global Constraints

- Everything in the parent plan's Global Constraints still applies
  (`docs/superpowers/plans/2026-11-26-standalone-azure-deploy.md`).
  - `az` puts `--subscription` after the command, through lib.sh's `az_in`
    and `azm`.
  - Every write goes through `run`. DRY_RUN works.
  - No committed value of ours.
- No account key, SAS or connection string anywhere.
  - Blob access is `--auth-mode login` for `az`, and workload identity for
    the connector.
  - Never allow public or anonymous access on a container.
- The tenant identity gets `Storage Blob Data Reader` scoped to its own
  container only:
  `<account id>/blobServices/default/containers/<tenant>-documents`.
- The containers are named `<tenant>-documents`. The connector's
  ServiceAccount is `memory-ingest`.
- Without a `memory` block in site.yaml, memory is off for every tenant and
  every memory step is a no-op that says so.
- The connector sends `modified_at` from the blob's Last-Modified, a sha256
  `content_hash` of the content, and the `present` manifest.

## Review Focus

1. **Re-running seeds nothing.** Running `make azure-memory` again with
   unchanged documents makes no upload call. Task 4 tests it.
2. **Scope of rights.** The role assignment's scope is the container, never
   the account. Task 4 tests it.
3. **The connector fails loudly.** It fails on a credential or list error,
   and never sends an empty `present` that would forget everything. Task 2
   tests it.
4. **Memory is optional.** A site without `memory`, or a tenant with memory
   off, renders no CronJob. Task 3 tests it.
5. **No ingest on upgrade.** No ingest runs on upgrade: no Helm hook remains
   for ingestion. Task 3 tests it.

---

### Task 1: Revert the uploader and move the documents

- [ ] Revert commit 3c7d81b (`git revert`, signed off).
- [ ] Move `charts/omnia-demo/files/fixtures/memory-s3/*` to
  `memory-documents/larkmere-mutual/`, and
  `charts/omnia-demo/files/fixtures/wrenmoor-field-procedures/*` to
  `memory-documents/wrenmoor-energy/`, with `git mv`.
- [ ] Leave the chart rendering exactly as before for now: point
  `fixtures.yaml` at the new path through a symlink or a chart-relative copy.
  Helm `.Files` can't read outside the chart, so keep a temporary copy if
  needed. Task 3 removes the ConfigMap anyway. The cleanest option is to do
  the move in Task 3 and only revert here; the controller ruling allows that.
- [ ] Run `go test ./deploy/...` with BASE_REF=2d253a0. Expected: green.

### Task 2: The connector reads Blob, can forget a source, and drops S3

**Files:** `memory-ingestion/main.go` (split a `blob.go` and a `forget.go` if
that keeps files focused), tests, `go.mod`, the Dockerfile if needed.

- **Blob source:**
  - It is configured by `BLOB_ACCOUNT_URL` and `BLOB_CONTAINER`.
  - Credentials come from `azidentity.NewWorkloadIdentityCredential` when
    `AZURE_FEDERATED_TOKEN_FILE` is set, else `NewDefaultAzureCredential`.
  - It lists the container and reads each text-like blob (same limits as
    today).
  - It sends `url = <account url>/<container>/<name>`,
    `site = <account url>/<container>`,
    `modified_at = LastModified (UTC RFC3339)`, the sha256 `content_hash`
    and `present` keys.
  - `MEMORY_SOURCE` defaults to `blob`.
- **Forget mode:** `FORGET_SOURCE=<source>` posts a single batch for that
  source with `present: {kind: document, keys: []}` and no documents, logs
  memory-api's `forgotten` count, and exits.
- **Removals:** the S3 source and the aws-sdk dependencies go.
  `SOURCE_DIR` stays.
- **Tests (TDD):**
  - Blob reads run against an `httptest` server speaking the Blob list and
    get API, or through an interface seam like today's `objectStore`.
  - A listing error stops the run before any post (Review Focus 3).
  - `modified_at` and the URL are as specified.
  - Forget mode sends an empty `present` for the named source only.
- `go test ./memory-ingestion/...` and `go vet` pass.

### Task 3: The chart renders a CronJob from Blob

**Files:** `charts/omnia-demo/templates/ingest.yaml` (which becomes the
CronJob), `fixtures.yaml` and the S3 gateway templates (removed),
`values.yaml`, the profiles' `memory` blocks, `deploy/azure/site-values.sh`,
`deploy/azure/identity.sh`, and the tests.

- **Values:**
  - `memory.source: blob`;
  - `memory.blob.accountURL` and `memory.blob.container`;
  - `memory.schedule` (default `"0 * * * *"`);
  - ServiceAccount `memory-ingest`, with the runtimeIdentity annotations and
    `azure.workload.identity/use: "true"` on the pod.
- **Removed:**
  - `memory.objectStore.*`, `memory.s3.*`, `memory.fixtures`;
  - the S3 gateway Deployment, Service and credentials Secret;
  - the fixtures ConfigMap;
  - any ingest Helm hook.
- **site-values.sh:** for a tenant whose profile has `memory.enabled: true`,
  it emits `memory.blob.accountURL` (`https://<memory.storageAccount>.blob.core.windows.net`)
  and `memory.blob.container` (`<tenant>-documents`). Without a site
  `memory` block, it emits `memory.enabled: false`.
- **identity.sh** federates `memory-ingest` among its fixed ServiceAccounts.
- **TestRenderedTenantsUnchanged:** the larkmere-mutual and wrenmoor-energy
  renders change by design. Assert the exact intended difference (the S3
  gateway, ConfigMap and hook Job gone; the CronJob and ServiceAccount
  added), not a blanket skip. Every other tenant stays byte-for-byte equal.
- **Tests:**
  - no `helm.sh/hook` on any ingest resource;
  - no CronJob when memory is off;
  - the CronJob's ServiceAccount, labels and env as specified.

### Task 4: `make azure-memory`

**Files:** `deploy/azure/memory.sh`, the Makefile, its tests, and
site.example.yaml (the `memory` block).

- **Tenants:** those whose profile sets `memory.enabled: true`.
- **Container:** created if absent with
  `az storage container create --auth-mode login --public-access off`,
  through `run`.
- **Uploads:** for each file in `memory-documents/<tenant>/`, compare its
  base64 MD5 with the blob's `properties.contentSettings.contentMd5`
  (`az storage blob show`, which is read-only; not-found means upload). Upload
  with `az storage blob upload --overwrite --content-md5 …` only on a
  difference.
- **Deletes:** blobs with no matching file are deleted.
- **Grant:** `Storage Blob Data Reader` to `<prefix>-<tenant>` on the
  container scope, if not already held.
- **First run:** it starts one connector run with
  `kubectl create job --from=cronjob/memory-ingest memory-ingest-<epoch>`
  when the CronJob exists.
- **Tests:**
  - re-running is idempotent (Review Focus 1);
  - the grant's scope is the container (Review Focus 2);
  - dry run;
  - a missing `memory` block is a clean no-op;
  - every call passes context and subscription checks.

### Task 5: Docs

`deploy/azure/README.md` (the memory step and the rights it needs),
`docs/stand-ins.md` (what the store stands in for now), the RUNBOOKs that
mention fixtures or S3, and `memory-ingestion/README.md`.

### Task 6 (controller, live)

1. Create a Standard_LRS StorageV2 account in the install's persistent resource group,
   with public blob access disabled and shared-key access disabled
   (user-approved 2026-11-26). Put it in our site.yaml.
2. Build the connector image with `make images`, then run `make azure-memory`.
3. Install the tenants with the new chart.
4. Run `FORGET_SOURCE=s3` once for each memory tenant.
5. Check that ingest logs show the 8 documents accepted once. A second
   triggered run must show `skipped` equal to the document count.
6. Resume Task 9 step 5 of the parent plan.
