# What members ask that we can't answer (demo 11)

> Commands name the cluster as `"$KUBE_CONTEXT"`. Set it to your cluster's kube
> context first (`aks.kubeContext` in `deploy/azure/site.yaml`).

The head of claims looks for gaps in the society's guidance. The Memory Galaxy
shows a large cluster of members asking about flood, with no flood procedure
beside it. The society closes the gap in two parts, kept apart as they would be
in a real insurer:

- **The rule**, CP-7: every flood claim goes to Flood Response, whatever its
  amount. A ToolPolicy rule (`claimsTriage.floodRule`). The assessor is never
  told it.
- **The guidance**, what a flooded member needs to know
  (`demos/claims-triage/flood-claims.md`). Ingested as institutional memory;
  the assessor uses it to tell the member what happens next.

Live on Omnia main `896d399a7eea` (#4009: a function now gets institutional
memory through retrieval).

## The beats

1. **The gap.** Dashboard, Memories, the Memory Galaxy for Larkmere Mutual: the
   flood cluster (labelled **flooding · flood** on the current seed; members worried about flood zones,
   flood and the mortgage valuation, whether the policy covers flooding). The
   few blue institutional points in it are what the policy covers and the
   Flood claim type's definition; no procedure says what happens to a flood
   claim. The corpus plants it (`demos/memory-galaxy/taxonomy.yaml`, "Planted
   finding 1").
2. **Before.** Invoke claims triage on the flood claim (dashboard, Functions,
   `claims-triage`, Test): `{"claim_ref": "LM-40751", "peril": "Flood"}`. £420
   payable, within the £500 limit, and no rule says otherwise: `SETTLED`.
   Reset the claims system after (`kubectl --context "$KUBE_CONTEXT" -n
   larkmere-mutual rollout restart deploy/claims-systems`).
3. **Add the rule.** Turn CP-7 on:
   ```bash
   helm --kube-context "$KUBE_CONTEXT" -n larkmere-mutual upgrade larkmere-mutual charts/omnia-demo --reuse-values --set claimsTriage.floodRule=true
   ```
   Show the rule in the ToolPolicy (`kubectl --context "$KUBE_CONTEXT" -n
   larkmere-mutual get toolpolicy claims-triage-policy -o yaml`): it asks the
   claims system for the claim's peril and refuses a payment on a flood claim.
   A policy change takes effect on the next invocation.
4. **Add the guidance.** Ingest it, **after** the upgrade: every `helm
   upgrade` re-runs the chart's ingest Job, which mirrors the chart's fixtures
   and so forgets the guidance.
   ```bash
   hack/reingest-manual.sh demos/claims-triage/flood-claims.md larkmere-mutual member-guidance
   ```
   It must go under `member-guidance/`: the assessor's retrieval keeps only
   that folder (see Setup).
   Restart `claims-systems` too, as in step 2.
5. **After.** Invoke again. The assessor tries to settle; the broker refuses
   with `deny-flood-settlement`; the assessor refers the claim, and the claims
   system routes it to Flood Response. The note tells the member what happens
   next, from the guidance. Show it in the session's system prompt: under "The
   society's guidance" are the flood-claims passages.

Verified live 2026-11-13 on main: before, `LM-40751` settled at £420; after,
`claim_settle` was refused by `deny-flood-settlement`, the claim was referred
to Flood Response, and the note said the team "will contact the member within
one working day", from the guidance. Each section of `flood-claims.md` names
flood claims on its own, because retrieval searches on the claim reference and
peril and returns passages, not the document: a section that says only "the
team contacts the member" is not found.

Put it back after: `--set claimsTriage.floodRule=false`, and restart
`claims-systems`.

## Setup already in place

- `claimsTriage.procedures` (Larkmere profile): memory retrieval on, the prompt
  renders `memory_context`, and the function's input takes the adjuster's
  `peril`, which retrieval searches on with the claim reference.
- The assessor's retrieval has an access filter (`denyCEL`) that keeps only
  documents under `member-guidance/`. The same tier holds the society's
  claims-handling rules, including the £500 authority; an assessor that read
  them would refer a £990 claim on its own and never reach the ToolPolicy,
  which is demo 3's whole point. Rules belong to the ToolPolicy; what the
  assessor reads is what it may tell a member.
- `memory.enabled` (Larkmere profile): the society's documents
  (`charts/omnia-demo/files/fixtures/memory-s3`) ingested at install. None of
  them covers flood.
- The flood claim `LM-40751` in the claims stub, and its `/policy/claim-peril`
  lookup for the policy.
