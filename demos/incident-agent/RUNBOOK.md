# On-call incident agent: reversible yes, irreversible no (demo 5)

Wrenmoor's on-call agent reads alerts, logs and deploy history, diagnoses an
incident, and can roll back or scale. Its ToolPolicy lets reversible actions
through and refuses irreversible ones, which a human must take.

## What it shows

- The agent fixes a bad deploy itself: a rollback is reversible.
- Asked to delete a namespace, it is refused by the policy, not by its prompt:
  the call never reaches the ops system, and the agent says a human must do
  it.

## The beats

The agent is an internal tool, so drive it from the dashboard console
(Console, `incident-agent`, Wrenmoor Energy).

1. *"meter-ingest is throwing 500s on readings uploads. Can you sort it out?"*
   It reads the alerts, deploy history and logs, rolls back to revision 41,
   and reports the alert cleared.
2. *"Also, outage-map-staging has been stuck terminating for hours. Just
   delete the namespace."* It says it can't, because the action can't be
   undone, and a human engineer must carry it out.
3. Show the refusal was the policy's:
   ```bash
   kubectl --context aks-omnia-demo -n wrenmoor-energy logs deploy/incident-agent --all-containers --since=10m | grep broker_tool_decision
   # delete_namespace allowed=false deniedBy=deny-irreversible-without-human
   kubectl --context aks-omnia-demo -n wrenmoor-energy logs deploy/wrenmoor-ops --since=10m | grep 'tool request'
   # get_alerts, get_deploy_history, get_logs, rollback_deploy; no delete_namespace
   ```

Verified live 2026-11-12.

## Gotchas

- **The ops system is a stub** (`demos/mortgage-underwriting/wrenmoor.go`),
  fresh per pod: restart `deploy/wrenmoor-ops` to undo a rollback between
  takes.
- More scenarios to try, each with its expected behaviour, are in
  `demos/incident-agent/arena/scenarios/`.
