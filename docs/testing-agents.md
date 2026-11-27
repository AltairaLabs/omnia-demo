# Testing the demo agents

Every demo pack has a PromptArena kit next to its prompt (`demos/<demo>/arena`,
and `demos/<demo>/poppy` for the Company Agents Ellen's assistant talks to).
A kit is the measure of the agent: change a prompt, run its kit, and only take
the change to a live demo once the kit is green. A live take is one sample of a
non-deterministic system; a kit is the same questions asked every time.

## What a kit holds

- **Scripted scenarios**, each pinning one decision the demo depends on: the
  tool the agent must call, the action it must refuse, what it must never
  claim. Deterministic assertions where they can decide it (`tools_called`,
  `tool_args`, `content_excludes`, `regex`); `llm_judge_session` where it needs
  judgement over the whole conversation.
- **Self-play**: a persona plays the user for several turns. Each kit has at
  least a cooperative and an adversarial persona; the adversarial one aims at
  the pack's real guardrails, and its assertions are ones it would trip if a
  guardrail failed.
- **Stand-ins for platform controls.** Where a demo's point is that Omnia, not
  the prompt, is the control (Wren's consent, the analyst sandbox, a Company
  tool refused until the member signs in), the kit's mock tools refuse the way
  the platform would, and the scenarios check what the agent does with the
  refusal. They do not test the prompt for a rule it deliberately leaves out.
- **Ellen's assistant** runs against `demos/personal-agent/arenafake`, a
  stateful fake of Larkmere and Wrenmoor behind the same HTTP tool interface
  the personal-agent service serves (see docs/stand-ins.md).

## Running a kit

Real runs need `OPENAI_API_KEY` (the agent on gpt-4o; self-play users and
judges on gpt-4o-mini). From the kit's directory:

```bash
promptarena run --config config.arena.yaml --provider openai --ci
```

Each kit's config header gives its exact commands, including a keyless mock
run that checks the scenarios load and the deterministic assertions hold. Ellen's
assistant runs inside the fake:

```bash
cd demos/personal-agent/arena
go run ../arenafake -- promptarena run --config config.test.arena.yaml --provider openai --ci
```

pii-chat's scope guardrail also needs `AI_GATEWAY_API_KEY` (the Jev classifier)
and a promptarena that binds `role: inference` providers (2.9 or later).

After changing a prompt, rebuild its pack with `hack/build-packs.sh <demo>`.

## From a bad live session to a scenario

When a live take goes wrong, turn the session into a scenario rather than
describing it: the phrasings a real Personal Agent sends are the inputs a
Company Agent's kit must cover, and the ones we invent are not. `promptarena
generate` pulls sessions from Omnia through the deploy adapter and writes one
scenario per session, with its recorded evals as assertions.

1. Make a local copy of the kit's config with a `deploy:` block naming your
   Omnia. Name it `*.local.arena.yaml`: those are gitignored, because `deploy
   login` writes your install's endpoint and workspace into it.

   ```yaml
   spec:
     # ...the kit's own config...
     deploy:
       provider: omnia
       config:
         api_endpoint: "https://<your-omnia>"
   ```

2. Log in (opens the browser; pick the Company's workspace). The token goes to
   `~/.promptarena/credentials`, never into the config.

   ```bash
   promptarena deploy adapter install omnia     # v1.5.4 or later
   promptarena deploy login --provider omnia --config config.signin.local.arena.yaml
   ```

3. Generate scenarios from the failed sessions, then read and trim them before
   adding them to the kit:

   ```bash
   promptarena generate --source omnia --config config.signin.local.arena.yaml \
     --filter-passed=false --output scenarios/from-production
   ```

Short sessions convert best. In a long one, later user turns were written
against the recorded answers, which the model will not give again; generate
keeps them and says so. Sessions come from production: check what your install
redacts before committing them, and keep this repo's rule of synthetic data
only.
