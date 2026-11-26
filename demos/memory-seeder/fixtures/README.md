# Generated corpora

Each directory here is a corpus `demos/memory-galaxy` generated, embedded in the
seeder image and replayed with `--fixture <name>`:

| File | Holds | Seeded through |
|---|---|---|
| `corpus.jsonl` | user, user_for_agent and agent memories, one per line | `POST /api/v1/memories`, `POST /api/v1/agent-memories` |
| `graph.json` | the institutional tier: graph nodes, guidance items, edges | `POST /api/v1/institutional/ingest/batch` (items and edges) |

Don't edit them by hand: change `demos/memory-galaxy/taxonomy.yaml` and
regenerate. A regeneration reads its answers from `demos/memory-galaxy/cache`
and costs nothing unless the taxonomy asks for prompts the cache has never seen.

| Fixture | Workspace |
|---|---|
| `larkmere-galaxy` | Larkmere Mutual |
