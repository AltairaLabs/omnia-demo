# Claims triage: the £500 limit (demo 3)

A claims assessor may settle up to £500 on its own. Above that, Larkmere's
delegated authority schedule says a human approves. Claims triage is a function
(`mode: function`), invoked with a claim reference.

## What it shows

1. **The agent follows the schedule.** It reads the schedule with
   `claim_policy_fetch`, settles £180 itself, and refers £990.
2. **The policy holds when the schedule is wrong.** Someone sets the schedule
   in the claims system to £2,000 by mistake. The agent believes it and tries
   to settle £990. The ToolPolicy holds the real £500 independently, so the
   policy broker refuses the payment before it reaches the claims system, and
   the agent refers the claim instead.

The second beat is the point. A well-behaved agent never tests the control, so
a demo that only shows the agent referring proves the prompt, not the policy.
The agent's belief (the schedule a tool returns) and the enforced rule (the
ToolPolicy) are deliberately separate, and this is what happens when they
disagree.

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
   `claim_fetch`, `claim_policy_fetch`, `claim_settle`.
2. **`LM-40718`** (£990 payable): `REFERRED`, citing the £500 limit. No
   `claim_settle` was attempted.
3. **The drift.** Set the schedule wrong, restart, invoke `LM-40718` again:
   ```bash
   kubectl --context aks-omnia-demo -n larkmere-mutual set env deploy/claims-systems CLAIMS_SCHEDULE_LIMIT_PENCE=200000
   ```
   `REFERRED`, and the agent's runtime logs the refusal:
   ```bash
   kubectl --context aks-omnia-demo -n larkmere-mutual logs deploy/claims-triage --all-containers --since=5m | grep broker_tool_decision
   # claim_settle allowed=false deniedBy=deny-settlement-above-delegated-authority
   ```
   The claims system logs `claim_fetch`, `claim_policy_fetch`, `claim_refer`:
   no settlement reached it.
4. **Put the schedule back:**
   ```bash
   kubectl --context aks-omnia-demo -n larkmere-mutual set env deploy/claims-systems CLAIMS_SCHEDULE_LIMIT_PENCE-
   ```

Verified live 2026-11-12: all three outcomes as above.

## Gotchas

- **The function's facade takes no anonymous calls.** `curl` to
  `svc/claims-triage` answers 401; drive it from the dashboard, which calls
  `POST /api/v1/workspaces/larkmere-mutual/functions/claims-triage/invoke`
  with `{"claim_ref": "..."}` as the operator.
- **Don't drive it concurrently.** The claims stub's maps have no lock
  (`docs/stand-ins.md`).
- **The limit is in two places on purpose:** the ToolPolicy
  (`charts/omnia-demo/templates/claims-triage-tools.yaml`) and the schedule
  the claims system reports (`demos/mortgage-underwriting/claimsdata.go`).
