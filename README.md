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
│   ├── demo-build-plan.md                ← consolidated flat build list with phase totals
│   ├── demo-kickoff.md                   ← Day 1 action list
│   └── demo-h0-plan.md                   ← H0 reliability gate decomposed into TDD tasks
├── promptpacks/                          ← agent prompt content
│   └── variant-a-support-promptpack.md   ← Variant A — warm/empathetic baseline (forked into variant B for operator demo A/B test)
├── personas/                             ← customer personas for PromptArena self-play
│   ├── README.md                         ← persona set overview + Pattern A/B design note
│   ├── sarah-chen.yaml                   ← polite-but-stressed professional (hero Scenes 2-3)
│   ├── marcus-webb.yaml                  ← assertive escalation-demander (hero Scene 4)
│   ├── emma-patel.yaml                   ← friendly new visitor (KB path)
│   ├── kai-nakamura.yaml                 ← patient confused older customer (lookup_customer path)
│   ├── priya-shah.yaml                   ← loyal disappointed repeat customer (memory recall path)
│   └── alex-rodriguez.yaml               ← happy returning (positive return archetype)
└── kb/                                   ← knowledge base articles
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
- **T7** ✅ DRAFT — Variant A support PromptPack drafted (`promptpacks/variant-a-support-promptpack.md`)
- **T8** ✅ DRAFT — 6 Acme Apparel personas drafted (`personas/*.yaml`)
- **SH4** ✅ DRAFT — 8 KB articles drafted (`kb/*.md`)
- **Azure infrastructure** — being provisioned out of band (V2, V3)
- **V1** ✅ — verified Claude is not available on Azure AI Foundry; D1 locked to GPT-4o
- **R2.1** ✅ — verified Omnia's memory populators are orphaned code; PromptKit handles extraction natively

Everything else in `specs/demo-build-plan.md` — what's DONE, PARTIAL, NOT STARTED, and VERIFY — is tracked there.

---

## Relationship to Omnia main repo

This repo is an **input** to Omnia's `charts/omnia-demo/` Helm chart, which doesn't exist yet. When H2 D1 is reached:

- The PromptPack gets compiled from the markdown design doc into a JSON file, embedded in a ConfigMap, and referenced from a `PromptPack` CRD
- The personas get mounted as a ConfigMap (or volume) that PromptArena's self-play reads
- The KB articles get mounted as a ConfigMap that the stub KB service reads

The specs in `specs/` describe the demo; they don't ship with the Helm chart. They stay here as the design record.

---

## Iterating

Since this is a content repo, most work is drafting, reviewing, and revising markdown and YAML. The workflow:

1. Edit the relevant file in `specs/`, `promptpacks/`, `personas/`, or `kb/`
2. Commit to a branch
3. Review / iterate
4. Merge to main when stable

No CI, no tests, no build pipeline — this is documentation + content.

When content is ready to be consumed by the Omnia Helm chart, it gets copied (or referenced via git submodule) into `charts/omnia-demo/content/` in the Omnia repo.

---

## License

All content in this repo is Apache-2.0 unless otherwise noted. The content describes a fictional brand ("Acme Apparel") and fictional customers — any resemblance to real persons, companies, or events is intentional parody or coincidental.
