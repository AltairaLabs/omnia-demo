# Memory Galaxy seeder

This example populates a realistic synthetic memory galaxy through the public
memory HTTP API. It creates institutional documents, agent memories, user
memories, observations, and representative relations across six semantic
categories.

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

