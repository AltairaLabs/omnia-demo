# PII chat

This example is a synthetic card-support Arena project. It demonstrates
identity verification, refusal before verification, privacy declarations, and
case-note tooling without connecting to a real card system.

Validate the Arena source with PromptArena:

```bash
promptarena validate demos/pii-chat/arena/config.arena.yaml
```

The project needs an Omnia Enterprise AgentRuntime to run. Provider
credentials and platform deployment manifests are supplied by the operator,
not stored here.
