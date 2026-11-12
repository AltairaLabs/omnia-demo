# What members ask that we can't answer (demo 11)

**Held**: the agent half needs an Omnia fix (functions get no institutional
memory through retrieval, below). The Galaxy half runs today.

The head of claims looks for gaps in the society's guidance. The Memory
Galaxy shows a large cluster of members asking about flood and no flood
procedure beside it. They write one; it's ingested; claims triage follows it
on the next flood claim.

## The beats

1. **The gap.** Dashboard, Memories, the Memory Galaxy for Larkmere Mutual: the
   flood cluster (members worried about flood zones, flood and the mortgage
   valuation, whether the policy covers flooding) with no institutional
   procedure near it. The corpus plants it (`demos/memory-galaxy/taxonomy.yaml`,
   "Planted finding 1").
2. **Before.** Invoke claims triage on the flood claim (dashboard, Functions,
   `claims-triage`, Test): `{"claim_ref": "LM-40751", "peril": "Flood"}`. £420
   payable, within the £500 limit, and no procedure says otherwise: `SETTLED`.
   Reset the claims system after (`kubectl --context aks-omnia-demo -n
   larkmere-mutual rollout restart deploy/claims-systems`).
3. **Write the procedure.** `demos/claims-triage/flood-claims.md` (CP-7: every
   flood claim goes to the Flood Response team, whatever its amount).
   Ingest it:
   ```bash
   hack/reingest-manual.sh demos/claims-triage/flood-claims.md larkmere-mutual
   ```
4. **After.** Invoke again: the agent refers the claim to Flood Response under
   CP-7. **Not yet:** see below.

## Why it's held

A function has no end user, so Omnia registers no memory tools for it, and its
docs say ambient retrieval still reads the agent and institutional tiers
without one. On 0.9.0-beta.23 it doesn't: the retriever returns nothing when
the scope has no user (`memory retrieval returned no context`, reason `no
subject key in scope`), so `memory_context` renders empty and the agent never
sees CP-7. Verified 2026-11-12: beat 2 settles as expected; after beat 3 the
agent still had no procedure in its prompt.

Don't work round it by giving the function a member's identity: a back-office
function doesn't act as the member.

## Setup already in place

- `claimsTriage.procedures` (Larkmere profile): memory retrieval on, the prompt
  renders `memory_context`, and the function's input takes the adjuster's
  `peril`, which retrieval searches on with the claim reference.
- `memory.enabled` (Larkmere profile): the society's procedure documents
  (`charts/omnia-demo/files/fixtures/memory-s3`) ingested at install. None of
  them covers flood.
- The flood claim `LM-40751` in the claims stub.
