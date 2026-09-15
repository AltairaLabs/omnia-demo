# Claims triage

This example is a synthetic claims-triage Arena project. It models the
read-policy-settle-or-refer decision flow with fictional claim data and no
production integrations.

Validate the Arena source with PromptArena:

```bash
promptarena validate demos/claims-triage/arena/config.arena.yaml
```

The project needs an Omnia Enterprise AgentRuntime to run. Keep provider
credentials and platform deployment manifests outside this public repository.
