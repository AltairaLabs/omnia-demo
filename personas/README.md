# Acme Apparel Personas

**Status**: Draft v0.1
**Part of**: T8 in `specs/demo-build-plan.md`
**Depends on**: T7 (variant A support content)
**Used by**: Hero demo Scenes 2, 3, 4 + operator demo self-play (O1)

Six reusable customer personas for the Acme Apparel support bundle, authored as PromptKit `kind: Persona` YAML files per `../promptkit/schemas/v1alpha1/persona.json`. These files move into `acme-apparel-support/personas/` when the arena-native content pivot (`specs/2026-04-11-arena-native-content-design.md`) executes.

---

## The set

| Persona | Archetype | Hero demo role | Operator demo role | Friction tags |
|---|---|---|---|---|
| **sarah-chen.yaml** | Polite-but-stressed professional | **Scene 2** (delayed order) AND **Scene 3** (returning thanks) | Self-play variety | `time_pressure`, `emotional_stakes_high`, `returning_customer` |
| **marcus-webb.yaml** | Assertive escalation-demander | **Scene 4** (chargeback dispute) | Self-play variety | `dispute`, `chargeback`, `escalation_demanded` |
| **emma-patel.yaml** | Friendly new visitor | — | Self-play variety (tests KB search path) | `pre_purchase`, `new_visitor`, `kb_dependent` |
| **kai-nakamura.yaml** | Patient confused older customer | — | Self-play variety (tests lookup_customer path) | `ambiguous_identity`, `needs_plain_language`, `patient` |
| **priya-shah.yaml** | Loyal disappointed repeat customer | — | Self-play variety (tests memory recall path) | `returning_customer`, `wrong_item`, `memory_dependent` |
| **alex-rodriguez.yaml** | Happy returning (generic) | — | Self-play variety | `positive_return`, `short_interaction` |

---

## Hero demo persona usage (important)

**Sarah Chen appears in two hero demo scenes with the same persona but different situations.** Per the hero-demo-proposal §3:

- **Scene 2**: Sarah signs into the Acme store for the first time in the demo. Persona situation: *"Your order #1023 — a linen dress for a birthday party this weekend — hasn't arrived and it's been 10 days. You're stressed."*
- **Scene 3**: Sarah signs back in 30 minutes later (or after a time-skip cut). Persona situation: *"You're back to say the express shipping voucher worked and the dress arrived in time. You're happy."*

Both scenes use the **same `sarah-chen.yaml` persona file** — the situation is injected per-scenario, not via a new persona. This is the whole point of Scene 3's memory moment: the *same* customer returns, and memory surfaces what the agent remembers from Scene 2.

**Marcus Webb appears in Scene 4 only** — chargeback dispute, tests the escalation guardrails.

The other four personas (Emma, Kai, Priya, Alex) are **not on camera in the hero demo**. They exist to provide variety in the operator demo's self-play traffic so the Grafana dashboards show diverse tool call patterns, not just "every session is Sarah complaining about a dress."

---

## Operator demo self-play usage

The operator demo's continuous ArenaJob (O3 in `operator-demo-proposal.md`) runs self-play scenarios that cycle through all six personas, producing ~20-50 synthetic sessions per minute. Each session uses one persona + one scenario situation, driven by an LLM playing the customer side (ideally in-cluster Ollama for cost, per D1 resolution).

Coverage the set provides:
- **Memory recall**: Priya and Alex test whether the agent retrieves memory at session start
- **Memory write**: Sarah, Priya, Marcus test whether the agent writes memory during/after session
- **KB search**: Emma forces the agent through `search_kb` for pre-purchase questions
- **lookup_customer (email-based)**: Kai forces the agent through email-based customer lookup when order ID is unknown
- **Escalation guardrails**: Marcus tests that chargebacks always escalate, never get handled directly
- **Discount code logic**: Sarah (goodwill for shipping delay), Priya (significant inconvenience for wrong item)
- **Tone adaptation**: the loyal-customer tone for Priya should differ from the new-visitor tone for Emma — tests whether the agent adapts

---

## Relationship to T7 (variant A support PromptPack)

**Resolved 2026-04-11**: Pattern A is the PromptKit-native convention. Personas carry their own complete `system_prompt` and are referenced from scenario `claude-user` turns (see `../promptkit/examples/customer-support-integrated/scenarios/social-engineering-selfplay.scenario.yaml`). No `customer_simulator` prompt is needed in the Acme Apparel pack — it's deleted as part of the arena-native content pivot.

The personas as drafted here are already in Pattern A form and carry forward unchanged into the new layout under `acme-apparel-support/personas/` (file moves + `.persona.yaml` suffix rename). Content doesn't change.

---

## Design principles

1. **Complete characters, not stubs**: each persona has a full `system_prompt` that can stand alone as a self-play customer role. This makes testing easy — you can instantiate one persona and run a single self-play turn against the agent without needing the full scenario machinery.

2. **Realistic voices, not caricatures**: each persona is a plausible real customer. No "angry troll" or "completely unhinged" stereotypes. The demo needs to look like realistic traffic, not adversarial testing.

3. **Explicit failure modes**: each persona's `constraints` section says what the persona will NOT do (no tool calls, no breaking character, no fabricating facts). This keeps self-play honest.

4. **Explicit end conditions**: each persona has "how you end conversations" guidance so self-play sessions don't drag on forever. PromptArena's self-play termination logic is important for clean session boundaries.

5. **Demographic + situational variety**: names, ages, backgrounds, tones, and situations cover a realistic range. No "angry man #1, angry man #2, angry man #3" monotony.

6. **Tag-friendly**: each persona has `friction_tags` that the operator demo can use to filter/aggregate self-play metrics. "Resolution rate for customers with `time_pressure` friction tag" becomes a meaningful metric in the Grafana dashboard.

---

## Testing these personas

Once Azure is up and the agent is running:

1. **Single-persona smoke test**: pick one persona, configure a minimal scenario with the situation injected, run one self-play session against the agent, eyeball the transcript for plausibility. Do this for all six.

2. **Memory recall test (Scene 3 rehearsal)**: run sarah-chen twice — once with the "delayed order" situation, once with the "thank you" situation, with the second session 30 seconds after the first. Verify the agent's first response in the second session references the first session's details.

3. **Escalation guardrail test**: run marcus-webb through 3-5 iterations. Verify the agent ALWAYS calls `escalate_to_human` within 2 turns. If it ever tries to resolve directly, the guardrail fragment in T7 isn't working.

4. **Self-play loop test**: configure a scenario that cycles through all 6 personas, run for 10 minutes, verify 20-50 sessions close with varied `session_outcome` labels (resolved/escalated/abandoned/unresolved should all appear).

---

## What's missing / next

Per the arena-native content pivot (`specs/2026-04-11-arena-native-content-design.md`), remaining work on the persona side is:

- **Move + rename** all six files into `acme-apparel-support/personas/` with `.persona.yaml` suffix.
- **Scenario YAML files** pairing personas with hero-demo scene situations and operator self-play — authored as `kind: Scenario` under `acme-apparel-support/scenarios/`. Pulled forward into H1 alongside the variant A / variant B PromptConfigs.
- **Variant B PromptConfig** — authored upfront under `acme-apparel-support/prompts/variant-b-agent.yaml` (no longer H4-gated).

Persona content itself is complete and does not change.
