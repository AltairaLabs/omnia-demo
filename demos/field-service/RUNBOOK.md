# Field-service copilot: the field manual is the source (demo 2)

Engineers in vans ask Wrenmoor's field-service copilot about procedures. Its
answers come from the field manuals, ingested as institutional memory, which
it searches with `memory__recall`. Change a manual and the next answer
changes: no redeploy, no prompt edit.

**Show the lookup.** In the session (dashboard, Sessions), the transcript has
a `memory__recall` row: open it to show what the copilot searched for and the
manual section it got back, the same words as its answer.

## The beats

The copilot is an internal tool: drive it from the dashboard console (Console,
`field-service`, Wrenmoor Energy).

1. Ask: *"I've opened the points of isolation on an HV ring main unit and
   locked them off. What exactly do I do next?"* The answer lists the manual's
   steps: caution notices, prove dead, apply earths.
2. The safety team updates the manual. Copy
   `charts/omnia-demo/files/fixtures/wrenmoor-field-procedures/isolation-and-safety.md`,
   add a step after "Prove dead" (the take used *"Photograph the indicator
   showing dead and attach the photo to the job in FieldDesk before you apply
   any earths."*), and re-ingest it:
   ```bash
   hack/reingest-manual.sh /path/to/edited/isolation-and-safety.md
   # ==> ... ingesting
   # {"msg":"sync complete", ... "documents":5, "accepted":5 ...}
   ```
3. Press **reset** in the console (a new conversation) and ask the same
   question. The answer now includes the photo step.
4. Put the manual back:
   ```bash
   hack/reingest-manual.sh charts/omnia-demo/files/fixtures/wrenmoor-field-procedures/isolation-and-safety.md
   ```

Verified live 2026-11-12 with `memory__recall`: before the edit, the steps
quoted from the manual; after, the same answer with the photo step, word for
word, from the re-ingested section.

## Gotchas

- **Don't redeploy between the edit and the question.** `make demos` reruns
  the chart's ingest Job, which mirrors the original fixtures back over your
  edited manual.
- **Sections and chunks.** Each manual section is stored twice: a section item
  (`#section-N`, a summary of its opening lines) and its full text (`#N`).
  Recall can return both; the full text is the one with every step.
- **The in-cluster S3 gateway loses its bucket when the cluster restarts.**
  `hack/reingest-manual.sh` recreates it and mirrors every fixture before it
  puts the edited file over its original, so the connector never sees a bucket
  with one document in it (it would forget the rest).
- **A changed manual needs a moment to embed.** The ingest reports an
  `embedding_backlog`; wait a few seconds before asking again.
