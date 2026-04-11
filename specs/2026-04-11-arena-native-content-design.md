# Arena-Native Content Pivot — Design

**Status**: Approved 2026-04-11, pending implementation
**Supersedes**: `promptpacks/variant-a-support-promptpack.md` (archival), persona repo layout under top-level `personas/`
**Referenced by**: `specs/demo-build-plan.md` (D6 resolution, T1–T8 row annotations, K1/K2/O1/O2 phase shifts), `README.md` (Current state)
**Implementation plan**: to follow this design doc

---

## Summary

Demo content (PromptPack contents, personas, tool schemas, evals, scenarios, providers) is being re-sourced as native PromptKit arena YAML under a new top-level `acme-apparel-support/` directory. The existing markdown-with-embedded-JSON form for T7 is superseded; personas move in place with a `.persona.yaml` suffix. Variant A, Variant B, scenario scripts, tool schemas, and pack-level evals are all authored as the same kind of artifact using the same toolchain that powers `promptarena run` and `packc compile`. Content authoring completes earlier, H0 reliability verification can exercise real content against Azure AI Foundry without requiring the Omnia runtime, and the demo content becomes runnable locally from day one.

## Background

Before this pivot, the demo had two authoring-form asymmetries:

1. **Personas were arena-native YAML** (`kind: Persona`) under `personas/*.yaml`, but the Variant A PromptPack was a markdown design doc (`promptpacks/variant-a-support-promptpack.md`) with a JSON pack body embedded in a code block. Author once as markdown, hand-compile to JSON, wrap as ConfigMap, reference from CRD.
2. **Hero demo scenes existed only as prose** in `specs/hero-demo-proposal.md`. There was no runnable form — every verification of agent behavior required either the full Omnia runtime or manual rehearsal.

Additionally, the existing T7 draft carried a "7 open questions" list flagged for PromptKit schema verification *later*. That was an artifact of having authored the pack without looking at the real schemas.

Reading the PromptKit repo revealed:

- PromptKit schemas at `../promptkit/schemas/v1alpha1/` define `kind: PromptConfig`, `kind: Persona`, `kind: Scenario`, `kind: Tool`, `kind: Provider`, `kind: Arena`, `kind: Eval`, plus related types.
- The `customer-support-integrated` example at `../promptkit/examples/customer-support-integrated/` is a near-perfect analog for the Acme Apparel hero demo: one PromptConfig, multiple personas, self-play via persona references in scenario `claude-user` turns, mock tool results for local run.
- `packc` at `../promptkit/tools/packc/` compiles arena source directories into PromptPack JSON per the open `promptpack-spec` standard at `../promptpack-spec/schema/promptpack.schema.json`. It derives pack ID from the source folder name, reads `prompt_configs[]`, `tools[]`, `pack_evals[]`, `workflow`, `self_play` and friends from the arena config, and emits a `.pack.json` file.
- The fragment system, LLM-judge eval with `metric: MetricDef` declaration, tool policy, validators, and variables that T7's markdown described as design inventions are all native PromptConfig fields — they were never inventions; the markdown was a design-doc narrative for what should have been a YAML file.
- Authoring tools (`packc compile` for producing the pack JSON, `packc validate` for checking the output against the PromptPack schema, and `promptarena run` for executing scenarios) exist and are usable against this repo's content as soon as the arena sources land. Schema-detail questions that the old T7 flagged as "verify later" are answerable immediately by authoring YAML and running the tools.

The pivot fixes the asymmetry, eliminates the compile-from-markdown step, unlocks local test/iteration, and materially de-risks H0 reliability work by making content runnable against Azure AI Foundry through PromptKit instead of through the Omnia runtime.

## Goals

- Single source of truth for all demo arena content, in one directory, one authoring form, one toolchain.
- Content authoring (T7, T8, K1, K2, O1, O2) completes earlier — specifically during H1's content phase rather than spread across H2/H4.
- Hero demo scenes become runnable (and assertable) scenarios, not just prose.
- H0 reliability verification can exercise real demo content against Azure AI Foundry via `promptarena run`, without needing the Omnia runtime.
- The eventual `omnia-demo` Helm chart (H2.b) consumes `packc compile` output directly; no hand-maintained JSON, no translation layer.
- Latent DevX capability: the author-test-iterate loop works locally from day one. Whether or not it later becomes a standalone demo is a post-H3 decision.

## Non-goals

- Standalone DevX demo (that was Approach 3 from brainstorming — deferred).
- Full promptpack-spec conformance today. A known gap (see "Open questions") means pack-level `name`, `version`, `template_engine`, and `metadata` are currently synthesized by packc defaults; we work around it with CLI flags and file an upstream PromptKit issue.
- Changes to platform (Omnia) code. All pivot work is in this repo or, if required, a single upstream PromptKit follow-up.

## Approach

**Approach 2 — Full re-sourcing as one unified arena source tree** (from the 2026-04-11 brainstorming session).

Everything current and planned that maps to a PromptKit `kind:` lives under `acme-apparel-support/`. The directory is named `acme-apparel-support/` (not `arena/`) because `packc compile` derives pack ID from folder name and `acme-apparel-support` is what the pack ID should be.

Runtime data (KB articles) and narrative specs (hero-demo-proposal, operator-demo-proposal) stay where they are — they're not arena-native content.

## Directory layout

```
omnia-demo/
├── acme-apparel-support/                    ← NEW — single source of truth for demo content
│   ├── config.arena.yaml                    ← kind: Arena — pack manifest + run config
│   ├── prompts/
│   │   ├── variant-a-agent.yaml             ← kind: PromptConfig, task_type: acme-support-variant-a
│   │   ├── variant-b-agent.yaml             ← kind: PromptConfig, task_type: acme-support-variant-b
│   │   └── fragments/
│   │       ├── variant-a/
│   │       │   ├── brand-voice.txt
│   │       │   ├── apology-style.txt
│   │       │   ├── response-length.txt
│   │       │   └── escalation-style.txt
│   │       └── variant-b/
│   │           └── (same four filenames, different content per D6)
│   ├── personas/                            ← MOVED from top-level personas/
│   │   ├── sarah-chen.persona.yaml
│   │   ├── marcus-webb.persona.yaml
│   │   ├── emma-patel.persona.yaml
│   │   ├── kai-nakamura.persona.yaml
│   │   ├── priya-shah.persona.yaml
│   │   └── alex-rodriguez.persona.yaml
│   ├── tools/
│   │   ├── lookup-order.tool.yaml
│   │   ├── lookup-customer.tool.yaml
│   │   ├── search-kb.tool.yaml
│   │   ├── issue-discount-code.tool.yaml
│   │   └── escalate-to-human.tool.yaml
│   ├── scenarios/
│   │   ├── hero-delayed-shipment.scenario.yaml
│   │   ├── hero-marcus-escalation.scenario.yaml
│   │   ├── hero-memory-recall.scenario.yaml
│   │   └── selfplay-mixed-personas.scenario.yaml
│   └── providers/
│       ├── azure-gpt4o.provider.yaml
│       └── ollama-local.provider.yaml
├── kb/                                      ← UNCHANGED — runtime data served by stub KB service
├── specs/                                   ← UNCHANGED — narrative + this design doc
├── personas/                                ← DELETED after move completes
├── promptpacks/                             ← DELETED after variant-a content migrates
└── README.md                                ← UPDATED
```

Everything is nominative: a reader opening `acme-apparel-support/` immediately sees the PromptKit example convention they probably already know from `../promptkit/examples/customer-support-integrated/`.

## File type mapping

| Current state | New state | Notes |
|---|---|---|
| `promptpacks/variant-a-support-promptpack.md` (markdown + embedded JSON) | `acme-apparel-support/prompts/variant-a-agent.yaml` + `fragments/variant-a/*.txt` | T7 rework. Design narrative carries forward as archival input; authoring form changes. Current markdown file carries a "SUPERSEDED" header and stays in place until content has fully migrated. |
| `promptpacks/variant-a-support-promptpack.md` §6 `customer_simulator` sub-prompt | **Deleted.** | Persona `system_prompt` drives the customer role via scenario `claude-user` turns. Pattern A confirmed. |
| `personas/*.yaml` (top level) | `acme-apparel-support/personas/*.persona.yaml` | File moves + `.persona.yaml` suffix rename. Content unchanged. |
| Not yet authored (H4 O2) | `acme-apparel-support/prompts/variant-b-agent.yaml` + `fragments/variant-b/*.txt` | Authored upfront per D6 (less-apologetic / confident, persona-dependent winners). O2 moves from H4 into H1. |
| K1, K2 (narrative-only in T7 markdown; planned for H2.a) | `acme-apparel-support/config.arena.yaml` `spec.pack_evals[]` inline `EvalDef` entries with `metric: MetricDef` | Pack-level because both variants A and B are measured identically — the entire point of the A/B. K1, K2, `no_pii_leak`, `memory_utilization` all pack-level. |
| `tool_efficiency`, `memory_utilization`, `no_pii_leak` (narrative in T7) | Same — pack-level entries in `config.arena.yaml` `spec.pack_evals[]` | No separate task row. |
| T1–T5 ToolRegistry entries (H1.c) | `acme-apparel-support/tools/*.tool.yaml` (`kind: Tool`) with `mode: mock` and fixture; wrapped for Omnia ToolRegistry CRD in H2.b | Bifurcation: schema authoring is content work during H1, CRD wrapping is Helm chart work during H2.b. |
| T6 ("ToolRegistry entries for T4/T5") | **Folded into T1–T5.** | The `.tool.yaml` files are the registry entries. |
| Hero demo scenes (prose in `specs/hero-demo-proposal.md` only) | `acme-apparel-support/scenarios/hero-*.scenario.yaml` with scripted turns + assertions | New capability: scenes become runnable/assertable. |
| O1 self-play scenario (not yet authored; planned for H4) | `acme-apparel-support/scenarios/selfplay-mixed-personas.scenario.yaml` | Authored upfront. Uses existing personas via `claude-user` turn pattern. Pulled forward from H4. |
| Azure GPT-4o provider (planned CRD in Helm chart only) | `acme-apparel-support/providers/azure-gpt4o.provider.yaml` + Omnia Provider CRD wrapper in H2.b Helm chart | Author once, reference twice. |
| Ollama-local provider (planned for H4 self-play) | `acme-apparel-support/providers/ollama-local.provider.yaml` | Used as the cheap customer-role provider in self-play; referenced from `config.arena.yaml` `spec.self_play.roles[]`. |
| `kb/*.md` | **Unchanged.** | KB articles are data served by the T4 stub KB service. Not arena-native. |

## `config.arena.yaml` shape

Top-level pack manifest and run config. Schema: `kind: Arena`, `apiVersion: promptkit.altairalabs.ai/v1alpha1`, `spec: Config` per `../promptkit/schemas/v1alpha1/arena.json`.

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: acme-apparel-support
  labels:
    demo: omnia-hero-demo
    bundle: acme-apparel

spec:
  prompt_configs:
    - id: variant-a
      file: prompts/variant-a-agent.yaml
    - id: variant-b
      file: prompts/variant-b-agent.yaml

  tools:
    - file: tools/lookup-order.tool.yaml
    - file: tools/lookup-customer.tool.yaml
    - file: tools/search-kb.tool.yaml
    - file: tools/issue-discount-code.tool.yaml
    - file: tools/escalate-to-human.tool.yaml

  # Pack-level evals — identical measurement surface for both variants.
  # This is the A/B comparison surface.
  pack_evals:
    - id: session_outcome
      type: llm_judge
      trigger: session_completion
      description: Classify session outcome
      params:
        rubric: |
          Classify this support session into one of: resolved, escalated, abandoned, unresolved.
        model: gpt-4o
      metric:
        name: acme_session_outcome_total
        type: counter
        labels: { outcome: "" }

    - id: customer_sentiment
      type: llm_judge
      trigger: session_completion
      description: Customer sentiment at session end, -1 to +1
      params:
        rubric: |
          Rate sentiment at the END of the session from -1 (very negative) to +1 (very positive).
        model: gpt-4o
      metric:
        name: acme_customer_sentiment
        type: gauge
        range: { min: -1, max: 1 }

    - id: no_pii_leak
      type: regex
      trigger: turn_completion
      description: Block credit-card-like strings in agent output
      params:
        pattern: '\b(?:\d[ -]*?){13,16}\b'
        expect: no_match
      threshold: { passed: true }

    - id: memory_utilization
      type: rule
      trigger: session_completion
      description: Did the agent call any memory__* tool during the session?
      params:
        rule: session.tool_calls.any(name.startsWith("memory__"))
      metric:
        name: acme_memory_utilization
        type: gauge

  providers:
    - file: providers/azure-gpt4o.provider.yaml
    - file: providers/ollama-local.provider.yaml

  self_play:
    personas:
      - file: personas/sarah-chen.persona.yaml
      - file: personas/marcus-webb.persona.yaml
      - file: personas/emma-patel.persona.yaml
      - file: personas/kai-nakamura.persona.yaml
      - file: personas/priya-shah.persona.yaml
      - file: personas/alex-rodriguez.persona.yaml
    roles:
      - id: customer
        provider: ollama-local        # cheap customer-role provider for self-play

  scenarios:
    - file: scenarios/hero-delayed-shipment.scenario.yaml
    - file: scenarios/hero-marcus-escalation.scenario.yaml
    - file: scenarios/hero-memory-recall.scenario.yaml
    - file: scenarios/selfplay-mixed-personas.scenario.yaml

  defaults:
    temperature: 0.6
    max_tokens: 800
    concurrency: 3
    output:
      dir: out
      formats: [json, html]
      html:
        file: report.html
```

## `variant-a-agent.yaml` shape

Single-prompt authoring file. Schema: `kind: PromptConfig` per `../promptkit/schemas/v1alpha1/promptconfig.json`.

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: acme-support-variant-a
  labels:
    demo: omnia-hero-demo
    variant: a
spec:
  task_type: acme-support-variant-a
  version: 0.1.0
  description: Acme Apparel support — warm/empathetic variant

  template_engine:
    version: v1
    syntax: "{{variable}}"
    features: [basic_substitution, fragments]

  fragments:
    - { name: brand_voice,      path: fragments/variant-a/brand-voice.txt,      required: true }
    - { name: apology_style,    path: fragments/variant-a/apology-style.txt,    required: true }
    - { name: response_length,  path: fragments/variant-a/response-length.txt,  required: true }
    - { name: escalation_style, path: fragments/variant-a/escalation-style.txt, required: true }

  variables:
    - { name: company,          type: string, required: true,  default: Acme Apparel }
    - { name: support_email,    type: string, required: true,  default: support@acme-apparel.example.com }
    - { name: business_hours,   type: string, required: false, default: "9am–6pm Pacific, Monday–Friday" }
    - { name: escalation_queue, type: string, required: false, default: acme-apparel-support }

  system_template: |
    You are a support agent for {{company}}.

    Business hours: {{business_hours}}.
    Escalation queue: {{escalation_queue}}.
    Support email: {{support_email}}.

    ## Voice
    {{brand_voice}}

    ## Apology style
    {{apology_style}}

    ## Response length
    {{response_length}}

    ## Escalation handling
    {{escalation_style}}

    ## Memory discipline
    At session start, call memory__recall to surface any prior context about this customer.
    During the session, call memory__remember when the customer shares something worth remembering.

    ## Tools
    - lookup_order / lookup_customer / search_kb / issue_discount_code / escalate_to_human
    - memory__recall / memory__remember

  allowed_tools:
    - lookup_order
    - lookup_customer
    - search_kb
    - issue_discount_code
    - escalate_to_human
    - memory__recall
    - memory__remember

  tool_policy:
    tool_choice: auto
    max_rounds: 8
    max_tool_calls_per_turn: 3

  parameters:
    temperature: 0.6
    max_tokens: 800

  validators:
    - type: regex_blocklist
      params:
        patterns: ['\b(?:\d[ -]*?){13,16}\b']
      message: Response blocked — possible PII leak
      fail_on_violation: true
    - type: max_length
      params:
        max_characters: 2000

  # No spec.evals[] — evals are pack-level in config.arena.yaml.

  metadata:
    domain: e-commerce-support
    tags: [hero-demo, variant-a, acme-apparel]
```

`variant-b-agent.yaml` is the same structure with `task_type: acme-support-variant-b`, fragment paths pointing at `fragments/variant-b/`, and optional `parameters` or `system_template` adjustments per D6.

## Key design decisions

1. **Variants A and B are two PromptConfig files inside one pack**, not two packs. Tools, providers, pack-level evals, self-play personas, and scenarios are shared at pack level. The A/B test run references both `task_type`s in the same arena config.
2. **Pack-level evals, not per-prompt evals.** K1, K2, `no_pii_leak`, `memory_utilization` all live in `config.arena.yaml` `spec.pack_evals[]`. Both variants are measured identically — that's the point of the A/B. Per-PromptConfig `spec.evals[]` is reserved for variant-specific evals we don't currently need.
3. **Tools in `spec.tools[]`, whitelisted per-prompt via `allowed_tools[]`.** One tool schema with mock results, referenced twice: once pack-level for registration, once in each PromptConfig's `allowed_tools` to whitelist.
4. **`customer_simulator` deleted from the pack.** Persona `system_prompt` drives the customer role via scenario `claude-user` turn pattern (see `../promptkit/examples/customer-support-integrated/scenarios/social-engineering-selfplay.scenario.yaml`). If rehearsal surfaces a need for persona-independent customer-side rules later, add a `customer-wrapper.yaml` PromptConfig at that point — not now.
5. **Fragments are external text files under `prompts/fragments/<variant>/`.** PromptKit `FragmentRef { name, path, required }` is the native form. Author as `.txt` plaintext; `packc compile` will fail (and `packc validate` on the compiled output will flag) if a different file format is expected.
6. **Folder name = pack ID.** `acme-apparel-support/` is the folder and the pack ID. `packc compile` derives the ID from the directory without flags. If a second pack ever lives in this repo, it goes in a sibling top-level folder (`acme-apparel-sales/`, whatever) — not nested inside `acme-apparel-support/`.
7. **Hero demo scenes gain a runnable form.** Each scene in `specs/hero-demo-proposal.md` becomes a `hero-*.scenario.yaml` with scripted turns and assertions. This is material new capability beyond re-sourcing — not just "same content, different form," but "content that was prose only is now testable."

## Deployment bridge to Omnia

`packc compile -c acme-apparel-support/config.arena.yaml --id acme-apparel-support --compiler-version <semver> -o build/acme-apparel-support.pack.json` produces a pack JSON conforming to `promptpack-spec`. That JSON is what Omnia's `PromptPack` CRD consumes. The bridge is two steps in H2.b:

1. **Build-time**: CI or the Helm chart's pre-install hook runs `packc compile`.
2. **Deploy-time**: the `charts/omnia-demo/` Helm chart's `PromptPack` template wraps the compiled JSON in a `ConfigMap` and references it from a `PromptPack` CRD.

Files under `acme-apparel-support/` are the single source of truth. The compiled JSON is build output, `.gitignore`'d under `build/`. The Helm chart templating doesn't touch source files; it runs the compiler and mounts the output.

**H2.b verification**: confirm that Omnia's `PromptPack` CRD spec expects exactly the shape `packc` emits, or identify a translation shim. Low-risk sanity check — Omnia's own `internal/schema/promptpack.schema.json` mirrors the promptpack-spec structure — but a 10-minute verification before H2.b starts avoids surprises.

## Build plan delta

Summary (detailed row-by-row updates are already applied in `specs/demo-build-plan.md`):

- **Pre-H0**: +1 item `VP` — `packc compile` smoke test. 30 minutes, runs after the pivot implementation completes.
- **H1 (content phase)**: T7 reworked to arena-native form. T8 file move + rename. T1–T5 bifurcate (tool YAMLs authored now, Helm wrap deferred). T6 folded into T1–T5. K1, K2 moved from H2.a into H1 as pack-level evals. O1, O2 moved from H4.a into H1 as scenarios and a second PromptConfig. Hero demo scenes converted to runnable scenarios.
- **H2.a**: K1/K2 removed (folded into T7 rework). K3, K4, K5 unchanged.
- **H2.b**: D1 Helm chart now consumes `packc compile` output directly rather than hand-maintained JSON. D3 `DEMO_SHAPE.md` carries forward unchanged.
- **H4.a**: O1 and O2 removed (moved to H1). O3 (continuous ArenaJob CRD) remains — cluster-side, not content.
- **Phase totals**: not recalculated at row level because net calendar impact is approximately zero. Work shifts earlier into H1; H2.a and H4.a shrink by roughly the same amount.

## Open questions / upstream follow-ups

1. **Pack-level `name`, `version`, `template_engine`, `metadata` gap.** `promptpack-spec` requires these fields in a compiled PromptPack, but PromptKit's `Arena.spec` does not have a top-level declaration surface for them. `packc compile` currently:
   - Sets `id` from folder name or `--id` flag.
   - Sets `compilerVersion` from `--compiler-version` or defaults to `"compiler-dev"`.
   - Synthesizes / defaults `name`, `version`, `template_engine`, `metadata`.
   
   **Workaround for the demo**: pass CLI flags in the build script. File an upstream PromptKit issue proposing `Arena.spec.pack: { name, version, template_engine, metadata }` as a new field. Not gating the demo.
2. **Fragment file format**. PromptKit's `FragmentRef { name, path, required }` schema doesn't constrain the MIME of the fragment body. Author as `.txt` plaintext; `packc compile` will surface any expectation mismatch by failing to compile.
3. **Memory tool naming**. `memory__recall` / `memory__remember` in `allowed_tools` — verify exact tool names with a minimal `promptarena run` that exercises the memory path, or by reading PromptKit's memory SDK code. Could be `memory.recall` / `memory.remember` or similar. Low-risk, two-minute fix if wrong.
4. **`EvalDef.type` and `EvalDef.trigger` enumerations**. `llm_judge`, `rule`, `regex`, `session_completion`, `turn_completion` are the design intent. Verified at `packc compile` time against the EvalDef schema and at runtime by `promptarena run` executing a scenario that triggers each.
5. **Omnia `PromptPack` CRD ↔ `packc compile` output shape.** 10-minute H2.b sanity check — does the CRD accept the pack JSON directly, or is there a translation shim? Expected direct, risk low.

## What this unlocks

Net new capability beyond re-sourcing:

1. **Local dev loop** — `packc compile` + `promptarena run` produces eval reports in `out/report.html` without Omnia. Iteration cycle is edit YAML → compile → run → read report.
2. **H0 verification through PromptKit, not Omnia.** R1/R2 memory extraction and eval-judge quality can be verified against Azure AI Foundry via `promptarena run` before the Omnia runtime is configured. Smaller verification surface, lower risk, earlier feedback.
3. **Runnable hero demo scenes** — scripted scenarios with assertions give regression tests for every content edit. Today, scenes only exist as prose.
4. **A/B differential is a file diff** — `diff acme-apparel-support/prompts/variant-a-agent.yaml acme-apparel-support/prompts/variant-b-agent.yaml` plus the fragment dirs. Concrete grounding for the operator demo's Act 2 narration.
5. **Latent DevX capability** — the author-test-iterate loop exists, free. Becomes a standalone demo only if post-H3 product decisions call for it; no retrofit needed.

## What this does NOT unlock

- Standalone DevX demo (Approach 3 from brainstorming — explicit non-goal, deferred).
- Promptpack-spec-conformant pack-level metadata — see Open questions #1.

## References

- `../promptkit/examples/customer-support-integrated/` — near-perfect analog for the hero demo layout.
- `../promptkit/examples/workflow-support/config.arena.yaml` — example of multi-PromptConfig arena with workflow state machine (not used here, but informative).
- `../promptkit/schemas/v1alpha1/promptconfig.json` — `kind: PromptConfig` schema (fragments, evals, validators, tool_policy).
- `../promptkit/schemas/v1alpha1/arena.json` — `kind: Arena` schema (prompt_configs, tools, pack_evals, self_play, scenarios, providers).
- `../promptkit/schemas/v1alpha1/persona.json` — `kind: Persona` schema.
- `../promptkit/schemas/v1alpha1/scenario.json` — `kind: Scenario` schema.
- `../promptkit/tools/packc/compiler/compiler.go` — packc compile logic; source of truth for how arena sources become a PromptPack.
- `../promptkit/docs/src/content/docs/reference/packc-cli.txt` — packc CLI reference.
- `../promptpack-spec/schema/promptpack.schema.json` — compiled PromptPack JSON schema.
- `promptpacks/variant-a-support-promptpack.md` — archival design narrative for Variant A content, input to the rework.
- `personas/README.md` — persona set description + Pattern A confirmation.
- `specs/demo-build-plan.md` — annotated build plan showing phase shifts.
- `specs/hero-demo-proposal.md` §6.5 — original KPIs-as-evals narrative, now realized as pack-level evals.
