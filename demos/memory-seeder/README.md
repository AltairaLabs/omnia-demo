# Memory Galaxy seeder

This example populates a realistic synthetic memory galaxy through the public
memory HTTP API. It ingests institutional documents and writes curated
institutional facts, agent memories, user memories and observations across six
semantic categories, then fits the Galaxy's clusters so the dashboard shows
them.

It is separate from `memory-ingestion/`: the ingestion service demonstrates an
authoritative document source, while this seeder creates varied demo data for
exploring the dashboard's projections and filters.

```bash
go run ./demos/memory-seeder \
  --memory-api http://localhost:8080 \
  --workspace-uid <workspace-uid> \
  --agent-uid <agent-uid>
```

The generated identities and content are synthetic and deterministic for a
given `--seed`. The seeder uses only the public memory API; it does not access
the memory database directly.

## What it writes

| Data | Route | Tier |
|---|---|---|
| Knowledge-base documents | `POST /api/v1/institutional/ingest/batch` | institutional (chunked by memory-api) |
| Policy facts | `POST /api/v1/institutional/memories` | institutional |
| Resolution facts | `POST /api/v1/agent-memories` | agent (`agent_id` = the AgentRuntime UID) |
| Customer memories, observations | `POST /api/v1/memories` | user |
| Embedding drain, leaf fit | `POST /admin/reembed`, `POST /admin/brain/build-leaves` | (Galaxy) |

Pass `--build-galaxy=false` to skip the last row. Both routes act on the
workspace's whole store: leave them off on a workspace whose clusters someone
else maintains.

User-tier memories use the types memory-api's built-in schema admits (`fact`,
`preference`, `observation`). A Workspace whose MemoryPolicy declares its own
`schema` must admit those types too.

## Requirements

- Omnia Enterprise. The Galaxy and the institutional routes are Enterprise;
  on Core those writes are skipped with `enterprise_required`.
- An embedding provider on the workspace memory service. A brain-store
  memory-api does not start without one.
- The Workspace UID and an AgentRuntime UID, not their names.

### Pseudonyms

memory-api stores a user as `virtual_user_id`, a pseudonym, never a raw id.
Each Omnia workspace pseudonymises under its own key, which the operator writes
into the `omnia-workspace-pseudonym-key` Secret (`key`) in the workspace
namespace. The seeder reads it from `OMNIA_PSEUDONYM_HMAC_KEY`, as Omnia's own
workspace pods do, and derives the same pseudonym the facade derives for the
same identity. Without the variable it uses the unkeyed form, which is right
only for an install with no key.

Before writing, the seeder compares its key's fingerprint with the one
memory-api's database is bound to, and stops on a mismatch rather than write
memories nobody can match. It sends the fingerprint on every write as well.
The key itself is never logged.

### Authentication and what gets seeded

With Omnia's `internalServiceAuth.enabled` (the chart default), memory-api
accepts a ServiceAccount token with the `omnia-session-api` audience from the
workspace namespace. The seeder sends the token at `SESSION_API_TOKEN_PATH` (or
`--token-path`) as a bearer token, and sends none when the file is absent.

What such a token lets the seeder write depends on the tier:

| Tier | Route | Service auth on | Service auth off |
|---|---|---|---|
| Documents | `/institutional/ingest/batch` | Seeded once Omnia lets any workspace ServiceAccount ingest (an Omnia change in progress, matching its docs); refused before it | Seeded |
| Institutional facts, agent memories | `/institutional/memories`, `/agent-memories` | Skipped | Seeded |
| User memories, observations | `/memories` | Skipped | Seeded |
| Leaf fit | `/admin/brain/build-leaves` | Skipped | Seeded |

The skipped rows need a caller on memory-api's exact subject allowlist (the
dashboard, operator, privacy-api and memory-api's own ServiceAccounts), and
Omnia has no setting to add one. memory-api answers them `403` with "shared
memory may be changed only by a system service" or "scope names a user, and
the request does not prove it is that user". The seeder takes the first such
refusal as the tier's answer: it logs one line saying which tier and why, and
writes nothing more to it. A Core install's `403 enterprise_required` on the
institutional routes is handled the same way. For the full Galaxy, seed a
development install with `internalServiceAuth.enabled: false`.

### Exit status

Single failed writes are logged (the first eight) and tolerated. The run
exits non-zero when:

- a step that ran had something to write and memory-api stored none of it:
  every user memory failed, say, or the embedding drain or leaf fit failed.
  A write the subject's consent refused counts as not stored. A tier skipped
  as above is not an empty step;
- every step was skipped, so nothing was seeded;
- memory-api refused the caller itself: `401`, a `403` other than the tier
  refusals above (a foreign workspace, a ServiceAccount from another
  namespace), or `409 pseudonym_key_mismatch`. These stop the run at once;
- five writes were dropped because privacy-api was unreachable (`204` without
  `X-Consent-Decision`). These are counted apart from consent refusals.

`429` and `5xx` (including memory-api's load-shedding `503`) are retried up to
five times, waiting at least the answer's `Retry-After`. Each step's outcome
is printed at the end.
