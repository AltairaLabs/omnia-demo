# Omnia Demo

Planning docs, content artifacts, and production materials for Omnia's paired hero + operator demos.

**Status**: In active development. Drafts v0.1 across all content. See `specs/demo-build-plan.md` for the current state of work.

---

## What's in this repo

This repo holds everything demo-related that would otherwise clutter the Omnia main repo or get lost in a local scratch directory. Two demos, one body of work:

### The two demos

**Hero demo** (see `specs/hero-demo-proposal.md`)
10-minute prospect-facing narrative: customer support agent on a real Shopify dev store ([acme-apparel-omnia-demo.myshopify.com](https://acme-apparel-omnia-demo.myshopify.com/)), memory moment across sessions, escalation, compliance. Answers *"does Omnia do what a modern support bot does?"* — table stakes in 2026.

**Operator demo** (see `specs/operator-demo-proposal.md`)
~17-minute differentiator demo: live measurement via Grafana, Arena A/B experimentation with PromptArena self-play, progressive rollouts with Istio cohort tracking, provider failover, **on-premises / cloud-private deployment spectrum**, compliance at scale. Answers *"why Omnia instead of Gorgias / Intercom / Ada?"* — the real sales story. Runs on Azure AKS with Azure AI Foundry via Private Endpoint as the primary deployment mode.

The hero demo is the ante. The operator demo is the differentiator.

### Repo structure

```
omnia-demo/
├── README.md                             ← you are here
├── specs/                                ← planning + design docs
│   ├── hero-demo-proposal.md             ← hero demo narrative + gaps
│   ├── operator-demo-proposal.md         ← operator demo narrative + gaps
│   ├── demo-build-plan.md                ← consolidated build list with phase totals
│   ├── demo-kickoff.md                   ← Day 1 action list
│   ├── demo-h0-plan.md                   ← H0 reliability gate TDD tasks
│   ├── 2026-04-11-arena-native-content-design.md       ← the arena-native content pivot design
│   └── 2026-04-11-arena-native-content-implementation-plan.md ← executed by these commits
├── acme-apparel-support/                 ← ARENA SOURCES (pack ID = folder name)
│   ├── README.md                         ← layout + run instructions
│   ├── config.arena.yaml                 ← kind: Arena — pack manifest + run config
│   ├── prompts/
│   │   ├── variant-a-agent.yaml          ← kind: PromptConfig — warm/empathetic baseline
│   │   ├── variant-b-agent.yaml          ← kind: PromptConfig — less-apologetic/confident
│   │   └── fragments/
│   │       ├── variant-a/                ← 4 fragment text files referenced by variant A
│   │       └── variant-b/                ← 4 fragment text files referenced by variant B
│   ├── personas/                         ← 6 kind: Persona files
│   ├── tools/                            ← 5 kind: Tool files with mock fixtures
│   ├── scenarios/                        ← 3 hero scenes + 1 self-play scenario
│   └── providers/                        ← Azure GPT-4o + Ollama local
└── kb/                                   ← runtime data served by stub KB service (not arena-native)
    ├── README.md                         ← editorial principles + consistent-facts table
    ├── shipping-policy.md
    ├── returns-and-exchanges.md
    ├── sizing-guide.md
    ├── care-instructions.md
    ├── order-tracking.md
    ├── damaged-or-incorrect-items.md
    ├── international-shipping.md
    └── faq.md
```

### Public memory ingestion demo

[`memory-ingestion/`](memory-ingestion/) and [`charts/omnia-demo/`](charts/omnia-demo/)
are a standalone S3-to-Omnia institutional-memory example. They are designed
to run against a released Omnia installation and contain only synthetic
fixtures and source-connector code. The chart is an add-on: it does not copy
Omnia templates or install private platform components.

---

## What's NOT in this repo (and where to find it)

- **Omnia itself** — the platform code lives at [AltairaLabs/Omnia](https://github.com/AltairaLabs/Omnia). This repo is consumed by Omnia (content is bundled into a Helm chart there) but doesn't contain platform code.
- **PromptKit** — lives at [AltairaLabs/PromptKit](https://github.com/AltairaLabs/PromptKit). The SDK that Omnia uses for memory, tool calling, and evals.
- **PromptArena deploy adapter** — Omnia's mechanism for deploying Arena agents. Lives in the Omnia repo under `ee/pkg/arena/`.
- **The Acme Apparel Shopify dev store** — lives at `acme-apparel-omnia-demo.myshopify.com`. Not in git. Seeded manually for the demo.
- **Azure AI Foundry + Private Endpoint infrastructure** — provisioned out of band as part of V3 verification (see `specs/demo-kickoff.md`).

---

## How to read the specs

**Start here**: `specs/demo-kickoff.md` — Day 1 action list with procedures for each verification task.

**For context on the current plan**: `specs/demo-build-plan.md` — consolidated build list across both demos with phase totals (Pre-H0 → H0 → H1-H6).

**For each demo in detail**: `specs/hero-demo-proposal.md` and `specs/operator-demo-proposal.md`. These are ~600 lines each, with narrative, capability maps, gap analysis, open questions, and phasing.

**For the reliability gate specifically**: `specs/demo-h0-plan.md` — the one-week phase that every other phase depends on.

---

## Current state (2026-04-11)

- **SH1** ✅ — Shopify dev store live at `acme-apparel-omnia-demo.myshopify.com`
- **T7** ✅ DONE 2026-04-11 — Variant A PromptConfig at `acme-apparel-support/prompts/variant-a-agent.yaml` + fragments. promptarena validate passes.
- **T8** ✅ DONE 2026-04-11 — 6 personas moved to `acme-apparel-support/personas/*.persona.yaml`. Content unchanged from original drafts.
- **K1, K2** ✅ DONE 2026-04-11 — pack-level evals in `config.arena.yaml` `spec.pack_evals[]`.
- **O1, O2** ✅ DONE 2026-04-11 — self-play scenario + variant B PromptConfig authored upfront under `acme-apparel-support/`.
- **SH4** ✅ DRAFT — 8 KB articles drafted (`kb/*.md`) — unchanged by the pivot (KB content is runtime data, not arena-native).
- **Azure infrastructure** — being provisioned out of band (V2, V3)
- **V1** ✅ — verified Claude is not available on Azure AI Foundry; D1 locked to GPT-4o
- **R2.1** ✅ — verified Omnia's memory populators are orphaned code; PromptKit handles extraction natively
- **D2, D3, D6, D7** ✅ — all pre-flight decisions now resolved (see `specs/demo-build-plan.md` §Pre-flight decisions)
- **Arena-native content pivot** ✅ DONE 2026-04-11 — implementation per `specs/2026-04-11-arena-native-content-implementation-plan.md` complete; pack validates cleanly. See commit history on branch `impl/arena-native-content`.

Everything else in `specs/demo-build-plan.md` — what's DONE, PARTIAL, NOT STARTED, and VERIFY — is tracked there.

---

## Relationship to Omnia main repo

This repo is an **input** to Omnia's `charts/omnia-demo/` Helm chart, which doesn't exist yet. When H2 D1 is reached:

- The PromptPack gets compiled from the markdown design doc into a JSON file, embedded in a ConfigMap, and referenced from a `PromptPack` CRD
- The personas get mounted as a ConfigMap (or volume) that PromptArena's self-play reads
- The KB articles get mounted as a ConfigMap that the stub KB service reads

The specs in `specs/` describe the demo; they don't ship with the Helm chart. They stay here as the design record.

### Separation of concerns

**This repo (`omnia-demo`)** owns:
- Demo narratives, phasing, and open questions
- Demo content (PromptPack, personas, KB articles)
- Build plans, kickoff docs, and reliability-gate implementation plans
- Production coordination for recording and publishing the demos

**The Omnia main repo (`AltairaLabs/Omnia`)** owns:
- All platform code (operator, runtime, facade, session-api, memory-api, doctor, arena-worker)
- All product-level specs for platform features (memory, arena, privacy, rollouts, etc.)
- Product PRs, issues, and CI
- The eventual `charts/omnia-demo/` Helm chart that bundles this repo's content
- Any reliability fixes the demo needs from the platform (PromptKit#836, Azure SDK private-endpoint handling, etc.)

**Rule of thumb**: if a change affects Omnia's platform behavior, it's a PR against `AltairaLabs/Omnia`. If a change affects how the demo is framed, scripted, or scoped, it's a commit here.

## File reference convention

The specs in `specs/` reference files and line numbers in the Omnia main repo using bare paths like `internal/runtime/conversation.go:176` or `ee/pkg/arena/`. These are **NOT paths within this repo** — they point to files in `AltairaLabs/Omnia`.

To resolve any reference:

- **Base URL**: [`https://github.com/AltairaLabs/Omnia`](https://github.com/AltairaLabs/Omnia)
- **File URL**: `https://github.com/AltairaLabs/Omnia/blob/main/<path>` — e.g., [`internal/runtime/conversation.go`](https://github.com/AltairaLabs/Omnia/blob/main/internal/runtime/conversation.go)
- **Line URL**: append `#L<line>` — e.g., [`internal/runtime/conversation.go#L176`](https://github.com/AltairaLabs/Omnia/blob/main/internal/runtime/conversation.go#L176)

Most spec references don't get rewritten as clickable links because there are hundreds of them and the base URL is stable. Readers can copy the path into the URL template above.

### Key Omnia PRs referenced across the specs

These are the merged PRs the specs rely on as "existing infrastructure that the demo uses":

| PRs | What they shipped | Referenced in |
|---|---|---|
| [#758](https://github.com/AltairaLabs/Omnia/pull/758)–[#764](https://github.com/AltairaLabs/Omnia/pull/764) | **Rollout support Phase 1-6**: CRD, controller, Istio routing, sticky sessions, cohort tracking, analysis step executor, docs | `operator-demo-proposal.md` Act 3 |
| [#785](https://github.com/AltairaLabs/Omnia/pull/785) | Provider endpoint health checks, dev Ollama | `operator-demo-proposal.md` Act 4, hero demo §5.1 |
| [#783](https://github.com/AltairaLabs/Omnia/pull/783) | Workspace settings UI, anonymous device ID, memory-api/session-api DB separation, infra fixes | hero demo §5.2 |
| [#786](https://github.com/AltairaLabs/Omnia/pull/786), [#787](https://github.com/AltairaLabs/Omnia/pull/787) | Doctor smoke test reliability + ollama-agent tool calling | `demo-h0-plan.md` R3 |
| [#788](https://github.com/AltairaLabs/Omnia/pull/788) | CI release workflow publishes to `charts.altairalabs.ai` HTTPS helm repo | `demo-build-plan.md` release pipeline context |
| [#773](https://github.com/AltairaLabs/Omnia/pull/773) | AgentPolicy.OnFailure wiring | hero demo §5.3 |
| [#728](https://github.com/AltairaLabs/Omnia/pull/728), [#755](https://github.com/AltairaLabs/Omnia/pull/755), [#756](https://github.com/AltairaLabs/Omnia/pull/756) | ToolPolicy enforcement sidecar injection | hero demo §5.3 (guardrail context) |
| [#771](https://github.com/AltairaLabs/Omnia/pull/771) | MCP tool filter | hero demo §5.3 |
| #690–#694 | Memory API and privacy wiring | hero demo §5.2 (memory is mostly built) |
| [#717](https://github.com/AltairaLabs/Omnia/pull/717) | Per-workspace session-api and memory-api | spec archival note |

### Key Omnia source files referenced across the specs

The demo's reliability and capability story depends on these paths working as documented:

| Path | What it proves | Specs that cite it |
|---|---|---|
| [`internal/runtime/conversation.go:176`](https://github.com/AltairaLabs/Omnia/blob/main/internal/runtime/conversation.go#L176) | `sdk.WithMemory()` wiring into PromptKit SDK | hero demo §5.2, demo-h0-plan R2 |
| [`internal/runtime/server.go:100`](https://github.com/AltairaLabs/Omnia/blob/main/internal/runtime/server.go#L100) | `memoryStore` typed as PromptKit's `pkmemory.Store` | demo-h0-plan R2.1 |
| [`pkg/provider/types.go:50-52`](https://github.com/AltairaLabs/Omnia/blob/main/pkg/provider/types.go#L50-L52) | `TypeAzureAI` first-class provider type | operator demo §5.5, demo-h0-plan V3 |
| [`api/v1alpha1/provider_types.go:174`](https://github.com/AltairaLabs/Omnia/blob/main/api/v1alpha1/provider_types.go#L174) | `BaseURL` field supports Private Endpoint | operator demo §5.5 |
| [`internal/schema/promptpack.schema.json:1170-1196`](https://github.com/AltairaLabs/Omnia/blob/main/internal/schema/promptpack.schema.json#L1170-L1196) | Eval `metric` declaration (name, type, bounds) | hero demo §6.5 (KPIs as evals) |
| [`internal/runtime/metrics_integration_test.go:241`](https://github.com/AltairaLabs/Omnia/blob/main/internal/runtime/metrics_integration_test.go#L241) | `MetricRecorder.Record(evalResult, metricDef)` — eval → Prometheus emission | hero demo §6.5 |
| [`ee/cmd/arena-worker/SERVICE.md:143`](https://github.com/AltairaLabs/Omnia/blob/main/ee/cmd/arena-worker/SERVICE.md#L143) | Self-play section — PromptArena native feature | operator demo §5.1 |
| [`internal/doctor/checks/memory.go`](https://github.com/AltairaLabs/Omnia/blob/main/internal/doctor/checks/memory.go) | Doctor `MemoryPersistsAcrossSessions` check | demo-h0-plan R3 |
| [`ee/pkg/privacy/deletion.go:222-230`](https://github.com/AltairaLabs/Omnia/blob/main/ee/pkg/privacy/deletion.go#L222-L230) | DSAR cascade | hero demo Scene 5, operator demo Act 6 |

### Where to file issues and PRs

**Platform bugs, reliability gaps, feature requests, infrastructure issues**:
- File against [AltairaLabs/Omnia](https://github.com/AltairaLabs/Omnia/issues)
- Example: PromptKit#836 (null arg coercion on Ollama), Azure SDK private-endpoint handling gap (if V3 surfaces one)

**Demo narrative changes, content revisions, scope adjustments**:
- File against [AltairaLabs/omnia-demo](https://github.com/AltairaLabs/omnia-demo/issues) (this repo)
- Example: "Act 2 A/B comparison table should include X metric", "Sarah persona feels too accommodating"

**Azure infrastructure tickets**:
- Wherever the Azure provisioning is being tracked (out-of-band for now — see `specs/demo-kickoff.md`)

### Known cross-repo coordination points

The following items require coordinated work across both repos:

1. **PromptKit#836** — null arg coercion upstream fix. Blocks Ollama as a demo LLM (mitigated by using GPT-4o on Azure instead). File upstream in [AltairaLabs/PromptKit](https://github.com/AltairaLabs/PromptKit).
2. **Omnia `azure-ai` provider + Private Endpoint** — if V3 reveals the Azure SDK client in Omnia doesn't respect a custom `baseURL` pointing at a Private Endpoint, it's an Omnia PR, blocking H4.
3. **Bundle spec v0.1** — when the demo Helm chart crystallizes its shape, whoever writes the `bundle spec` formalization in the planning repo uses this demo's `charts/omnia-demo/` structure as input. See `specs/hero-demo-proposal.md` §1 for context on why bundle is a deliberate non-goal of the demo.
4. **LLM populator cleanup** — R2.1 found that `internal/memory/populator_llm.go`, `populator_conversation.go`, `extractor.go` and friends are orphaned code in Omnia. A cleanup PR to delete them is filed as a post-demo backlog item in `docs/local-backlog/agentic-memory-remaining.md` in the Omnia repo.

---

## Iterating

Since this is a content repo, most work is drafting, reviewing, and revising markdown and YAML. The workflow:

1. Edit the relevant file in `specs/`, `acme-apparel-support/`, or `kb/`
2. Commit to a branch
3. Review / iterate
4. Merge to main when stable

No CI, no tests, no build pipeline — this is documentation + content.

When content is ready to be consumed by the Omnia Helm chart, it gets copied (or referenced via git submodule) into `charts/omnia-demo/content/` in the Omnia repo.

---

## License

All content in this repo is Apache-2.0 unless otherwise noted. The content describes a fictional brand ("Acme Apparel") and fictional customers — any resemblance to real persons, companies, or events is intentional parody or coincidental.
