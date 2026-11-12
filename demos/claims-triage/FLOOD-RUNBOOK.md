# What members ask that we can't answer (demo 11)

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

**Partly held.** The rule half works on beta.23. The guidance half needs Omnia
#4009: a function gets no institutional memory through retrieval yet, so the
assessor can't read the guidance. The Galaxy half hasn't yet been walked
through on camera against the 10k corpus.

## The beats

1. **The gap.** Dashboard, Memories, the Memory Galaxy for Larkmere Mutual: the
   flood cluster (members worried about flood zones, flood and the mortgage
   valuation, whether the policy covers flooding) with no institutional
   procedure near it. The corpus plants it (`demos/memory-galaxy/taxonomy.yaml`,
   "Planted finding 1").
2. **Before.** Invoke claims triage on the flood claim (dashboard, Functions,
   `claims-triage`, Test): `{"claim_ref": "LM-40751", "peril": "Flood"}`. £420
   payable, within the £500 limit, and no rule says otherwise: `SETTLED`.
   Reset the claims system after (`kubectl --context aks-omnia-demo -n
   larkmere-mutual rollout restart deploy/claims-systems`).
3. **Add the rule.** Turn CP-7 on:
   ```bash
   helm --kube-context aks-omnia-demo -n larkmere-mutual upgrade larkmere-mutual charts/omnia-demo --reuse-values --set claimsTriage.floodRule=true
   ```
   Show the rule in the ToolPolicy (`kubectl --context aks-omnia-demo -n
   larkmere-mutual get toolpolicy claims-triage-policy -o yaml`): it asks the
   claims system for the claim's peril and refuses a payment on a flood claim.
   A policy change takes effect on the next invocation.
4. **Add the guidance.** Ingest it:
   ```bash
   hack/reingest-manual.sh demos/claims-triage/flood-claims.md larkmere-mutual
   ```
5. **After.** Invoke again. The assessor tries to settle; the broker refuses
   with `deny-flood-settlement`; the assessor refers the claim, and the claims
   system routes it to Flood Response. With #4009, the assessor's note also
   tells the member what happens next and what to do now, from the guidance.

Put it back after: `--set claimsTriage.floodRule=false`, and restart
`claims-systems`.

## Setup already in place

- `claimsTriage.procedures` (Larkmere profile): memory retrieval on, the prompt
  renders `memory_context`, and the function's input takes the adjuster's
  `peril`, which retrieval searches on with the claim reference.
- `memory.enabled` (Larkmere profile): the society's documents
  (`charts/omnia-demo/files/fixtures/memory-s3`) ingested at install. None of
  them covers flood.
- The flood claim `LM-40751` in the claims stub, and its `/policy/claim-peril`
  lookup for the policy.
