# Claims triage: the £500 limit (demo 3)

A claims assessor may settle up to £500 on its own. Above that, Larkmere's
delegated authority schedule says a human approves. Claims triage is a function
(`mode: function`), invoked with a claim reference.

## What it shows

The rule lives in a ToolPolicy and nowhere else. The assessor's prompt and
tools say nothing about a limit: it settles what the claims system says is
payable, and refers a claim when the platform refuses. So the refusal on
screen is the platform's, not a well-behaved model's.

1. **£180 settles.** The assessor reads the claim and pays it.
2. **£990 is refused.** The assessor tries to pay what is payable. The policy
   broker refuses `claim_settle` before the call reaches the claims system,
   quoting DA-2, and the assessor refers the claim with that reason.

## Before each take

Reset the claims system (its state is in memory):

```bash
kubectl --context aks-omnia-demo -n larkmere-mutual rollout restart deploy/claims-systems
kubectl --context aks-omnia-demo -n larkmere-mutual rollout status deploy/claims-systems
```

Wait a few seconds after it reports ready: the first invocation straight after
a restart can fail to fetch the claim.

## The beats

Invoke from the dashboard (Functions, `claims-triage`, Test, `claim_ref`).

1. **`LM-40733`** (£180 payable): `SETTLED`. The claims system logs
   `claim_fetch`, `claim_settle`.
2. **`LM-40718`** (£990 payable): `REFERRED`. The session shows the refused
   `claim_settle` and the policy's message. The agent's runtime logs the
   decision:
   ```bash
   kubectl --context aks-omnia-demo -n larkmere-mutual logs deploy/claims-triage --all-containers --since=5m | grep broker_tool_decision
   # claim_settle allowed=false deniedBy=deny-settlement-above-delegated-authority
   ```
   The claims system logs `claim_fetch`, `claim_refer`: no settlement reached
   it.

Verified live 2026-11-12 on the rule-free prompt: `LM-40733` settled;
`LM-40718`'s `claim_settle` was refused by
`deny-settlement-above-delegated-authority` and never reached the claims
system, and the assessor referred it citing the refusal.

**Show the prompt.** The session's system prompt has no limit in it. That is
the point to make on camera: the model didn't know, and it didn't need to.

## Gotchas

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
