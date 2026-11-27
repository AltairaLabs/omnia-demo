# Claims triage: the £500 limit (demo 3)

> Commands name the cluster as `"$KUBE_CONTEXT"`. Set it to your cluster's kube
> context first (`aks.kubeContext` in `deploy/azure/site.yaml`).

A claims assessor may settle up to £500 on its own. Above that, Larkmere's
delegated authority schedule says a human approves. Claims triage is a function
(`mode: function`), invoked with a claim reference.

## What it shows

The rule lives in a ToolPolicy and nowhere else. The assessor's prompt and
tools say nothing about a limit: it settles what the claims system says is
payable, and refers a claim when the platform refuses. So the refusal on
screen is the platform's, not a well-behaved model's.

1. **£180 settles.** The assessor reads the claim and pays it.
2. **£990 waits for a person.** The assessor tries to pay what is payable. The
   policy broker **holds** `claim_settle` before it reaches the claims system
   and raises a referral to a claims handler. The handler approves, and that
   same call runs: the claim settles with no new invocation, and the record
   says who decided. If the handler rejects it, the assessor refers the claim.

The hold is `claimsTriage.referral.enabled` (on in the Larkmere profile). With
it off, DA-2 refuses outright, as this demo first did.

## Before each take

Reset the claims system (its state is in memory):

```bash
kubectl --context "${KUBE_CONTEXT:?set KUBE_CONTEXT to your cluster's context}" -n larkmere-mutual rollout restart deploy/claims-systems
kubectl --context "${KUBE_CONTEXT:?set KUBE_CONTEXT to your cluster's context}" -n larkmere-mutual rollout status deploy/claims-systems
```

Wait a few seconds after it reports ready: the first invocation straight after
a restart can fail to fetch the claim.

## The beats

Invoke from the dashboard (Functions, `claims-triage`, Test, `claim_ref`).

1. **`LM-40733`** (£180 payable): `SETTLED`. The claims system logs
   `claim_fetch`, `claim_settle`.
2. **`LM-40718`** (£990 payable): the Test tab comes back straight away. The
   broker held the settlement:
   ```bash
   kubectl --context "${KUBE_CONTEXT:?set KUBE_CONTEXT to your cluster's context}" -n larkmere-mutual logs deploy/claims-triage --all-containers --since=5m | grep broker_tool_decision
   # claim_settle outcome=refer deniedBy=refer-settlement-above-delegated-authority phase=hold
   ```
   The claims system has logged `claim_fetch` only. Nothing has been paid.
3. **Be the claims handler.** List what's waiting, then approve it:
   ```bash
   demos/claims-triage/hack/handler.sh list
   # ref_…  pending  claim_settle  {"amount_pence":99000,"claim_ref":"LM-40718"}  raised …
   demos/claims-triage/hack/handler.sh approve ref_…
   # ref_… approved by larkmere-mutual/claims-handler
   ```
   Within a few seconds the claims system logs `claim_settle`. In
   **Invocations** the invocation is `Completed` (its latency includes the
   wait), and its session shows `claim_settle` taking the length of the hold,
   then `SETTLED`.
4. **Or reject it** (`handler.sh reject ref_…`): the assessor refers the claim
   (`claim_refer`), and nothing is paid.

Verified live 2026-11-18 on Omnia 5d9d48cac541: held, approved by
`larkmere-mutual/claims-handler`, `claim_settle` ran and the invocation
completed `SETTLED`. A second run was rejected and referred.

**Show the prompt.** The session's system prompt has no limit in it. That is
the point to make on camera: the model didn't know, and it didn't need to.

## Gotchas

- **The Test tab says "Failed" on a held invocation**, with an HTTP 200. It
  isn't failed: the invocation continues in its held record once decided.
  Show **Invocations** instead, which reads `Completed` after the approval.
- **The handler is a ServiceAccount**, `claims-handler`, and approving is a
  script, not a page. Omnia has no console page for referrals and no user
  approvers on this install yet (Omnia #4248). Say so on stage: the decision
  and who made it are real, the page isn't there yet.
- **Turning referral on rolls every agent** in the workspace's default group,
  and on a full cluster the new pods wait for the autoscaler. Wait for
  `kubectl --context "${KUBE_CONTEXT:?set KUBE_CONTEXT to your cluster's context}" -n larkmere-mutual rollout status deploy/claims-triage`
  before the take: until it finishes, the old pod refuses instead of holding.

- **The function's facade takes no anonymous calls.** `curl` to
  `svc/claims-triage` answers 401; drive it from the dashboard, which calls
  `POST /api/v1/workspaces/larkmere-mutual/functions/claims-triage/invoke`
  with `{"claim_ref": "..."}` as the operator.
- **Don't drive it concurrently.** The claims stub's maps have no lock
  (`docs/stand-ins.md`).
- **The limit is in one place:** the ToolPolicy
  (`charts/omnia-demo/templates/claims-triage-tools.yaml`). Don't add it to
  the prompt, a tool description or the stub.
- **The claims system routes referrals**, by peril: the agent gives a reason,
  never a team.
