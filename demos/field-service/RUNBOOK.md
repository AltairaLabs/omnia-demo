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
2. The safety team updates the manual. Edit
   `memory-documents/wrenmoor-energy/isolation-and-safety.md` (leave it
   uncommitted), add a step after "Prove dead" (the take used *"Photograph the
   indicator showing dead and attach the photo to the job in FieldDesk before
   you apply any earths."*), and seed it:
   ```bash
   make azure-memory
   # ==> uploading wrenmoor-energy-documents/isolation-and-safety.md
   # ==> starting a memory-ingest run in wrenmoor-energy
   ```
   `make azure-memory` uploads only the file that changed to the tenant's Blob
   container and starts a run of the `memory-ingest` CronJob. Follow it with
   `kubectl --context "${KUBE_CONTEXT:?set KUBE_CONTEXT to your cluster's context}" -n wrenmoor-energy logs -f job/<the memory-ingest-... job it started>`;
   the last line is `sync complete` with `"documents":5`.
3. Start a new conversation and ask the same question. The answer now
   includes the photo step. Reload the console page to start it: on Omnia main
   e33940cd6 the console's reset button clears the screen but keeps sending to
   the old session, so the answer comes from that conversation's history.
4. Put the manual back:
   ```bash
   git checkout memory-documents/wrenmoor-energy/isolation-and-safety.md
   make azure-memory
   ```

Verified live 2026-11-12 with `memory__recall`: before the edit, the steps
quoted from the manual; after, the same answer with the photo step, word for
word, from the re-ingested section.

## Gotchas

- **Redeploying does not undo the edit.** An upgrade never runs the ingest and
  the Blob container keeps your edited manual. Only `make azure-memory` from a
  working tree without the edit puts the original back, so reset with step 4
  when you are done.
- **Sections and chunks.** Each manual section is stored twice: a section item
  (`#section-N`, a summary of its opening lines) and its full text (`#N`).
  Recall can return both; the full text is the one with every step.
- **An empty container forgets every manual.** The connector sends the list of
  documents the container holds, and memory forgets the rest. `make
  azure-memory` refuses to seed from an empty documents directory for that
  reason.
- **A new grant needs a few minutes.** The first `make azure-memory` for a
  tenant grants its identity read access to the container; Azure takes a few
  minutes to honour it, so the script does not start a run then. Run it again
  later.
- **A changed manual needs a moment to embed.** The ingest reports an
  `embedding_backlog`; wait a few seconds before asking again.
