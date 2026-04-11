# Omnia Demo Build Plan

**Status**: Consolidated build list for the paired hero + operator demos
**Created**: 2026-04-10
**Last significant update**: 2026-04-11 — arena-native content pivot (see `2026-04-11-arena-native-content-design.md`)

> **Content pivot (2026-04-11) — COMPLETE**: Demo content (T7/T8/K1/K2/O1/O2 and the T1–T5 tool schemas) has been re-sourced as native PromptKit arena YAML under `acme-apparel-support/`. See `specs/2026-04-11-arena-native-content-design.md` for design rationale and `specs/2026-04-11-arena-native-content-implementation-plan.md` for the executed plan. T7/T8/K1/K2/O1/O2 rows below are marked DONE where the content work completed; H2.b (Helm chart packaging) and H4.a O3 (continuous ArenaJob) remain.

## 📍 Starting work? Open `demo-kickoff.md` first

`docs/local-backlog/demo-kickoff.md` is the Day 1 action list. It walks through the pre-flight verification batch with specific procedures. This doc (build plan) is reference material; kickoff is actionable.

---

## About this doc

Consolidates the work items from:
- `docs/local-backlog/hero-demo-proposal.md` (v0.4+)
- `docs/local-backlog/operator-demo-proposal.md` (v0.2)

**Read those specs for context and rationale.** This doc is a flat build list — what needs to happen, in what order, how big each piece is, what depends on what.

## Companion docs

- `docs/local-backlog/demo-kickoff.md` — **start here tomorrow morning** — Day 1 schedule with procedures for V1-V3, SH1
- `docs/local-backlog/demo-h0-plan.md` — H0 reliability pre-flight decomposed into TDD-ready tasks (starts Day 2 or Day 3)
- `docs/local-backlog/hero-demo-proposal.md` — full narrative + capability map + gap detail for the hero demo
- `docs/local-backlog/operator-demo-proposal.md` — full narrative + capability map + gap detail for the operator demo

---

## Legend

| Marker | Meaning |
|---|---|
| 🔒 | **DECIDE** — human decision needed; blocks downstream work |
| 🧪 | **VERIFY** — check existing state; may result in zero code |
| 🛠 | **BUILD** — new code, config, or CRD |
| 🎨 | **ASSET** — diagram, chart, screenshot, script; non-code |
| 🎬 | **PRODUCTION** — on-camera recording or post-production |

---

## Totals

| Phase | Theme | Engineer-days | Calendar |
|---|---|---|---|
| **Pre-H0** | Verification batch (V1-V3 + SH1) | 1-2.5 | ~Day 1-2 |
| **H0** | Reliability pre-flight | 4-7 | ~1 week |
| **H1** | Hero demo: Shopify + widget + tools + content | 13-19 | ~2 weeks |
| **H2** | Hero demo: evals + dashboard + Helm chart | 8.5-12 | ~1-1.5 weeks |
| **H3** | Hero demo: polish + record | 3-5 | ~0.5-1 week |
| **H4** | Operator demo: self-play + environment | 5.5-8 | ~1.5 weeks |
| **H5** | Operator demo: dashboards + rehearsal | 7-12 | ~1-1.5 weeks |
| **H6** | Operator demo: record + edit | 2-4 | ~0.5-1 week |
| **Total** | Both demos, sequential | **44-69** | **~7.5-10 weeks** |

Two engineers in parallel after H3: **~6-7 weeks calendar**.

**Start here**: `docs/local-backlog/demo-kickoff.md` — Day 1 actions with clear procedures.

---

## Pre-flight decisions

These gate the entire build. Several are now **resolved** — marked ✅.

| ID | Decision | Resolution | Blocks |
|---|---|---|---|
| ✅ D1 | **Demo LLM backend** | **GPT-4o (or Azure AI Foundry's current GPT flagship) on Azure AI Foundry with Private Endpoint.** Uses Azure credits. Matches operator demo's cloud-private deployment story. In-cluster Ollama used only for self-play customer role to cap rehearsal costs. **Claude-on-Azure-AI-Foundry is unverified (see V1) and probably not available**; if it turns out to be available, switch to Claude as a free win. If a Claude dogfood is strictly required AND Claude isn't on Azure AI Foundry, the fallback is Anthropic API key directly — but that breaks the operator demo's Act 5 private-deployment story, so NOT recommended | H0 (R3), H1 (T1-T7), operator demo Act 5 |
| ✅ D2 | **Shopify customer auth API** — `window.Shopify.customer` vs Customer Account API | **Customer Account API (the newer path).** Forward-compatible with Shopify 2026+. W4 design pass still needed before implementation — OAuth wiring is more involved than the legacy global. | H1 (W4) |
| ✅ D3 | **Bundle spec** — is this demo allowed to ship without formalizing "bundle"? | **Yes — ship with existing CRDs + Helm.** "Bundle" is conceptual, not a product yet; no spec exists to block on. `charts/omnia-demo/DEMO_SHAPE.md` (D3 task in H2.b) ships as a descriptive artifact — input to any future bundle-spec work, not a spec itself. | H2 (D1-D3) |
| ✅ D4 | **On-prem Ollama for operator demo Act 5** | **Not mandatory as the primary path.** Primary Act 5 target is cloud-private deployment (Azure AI Foundry + Private Endpoint). On-prem Ollama becomes a **~20-second optional sub-beat** showing the full air-gap variant of the same platform. Since in-cluster Ollama is already needed for self-play customer role, the sub-beat is basically free to include. | H4 (O8 reframed) |
| ✅ D5 | **Demo cluster for operator demo** | **Dedicated Azure AKS cluster** (not shared with hero demo) labeled as the "customer's private K8s cluster". Enables Azure AI Foundry Private Endpoint setup and makes the network isolation story credible. | H4 (O4) |
| ✅ D6 | **Variant B PromptConfig design** — what style is variant B? | **Less-apologetic / confident**, targeting persona-dependent winners. Same length as Variant A, drops "I'm so sorry"/"I totally understand" softeners, projects competence. Persona-dependent outcomes (Marcus prefers terse confidence, Sarah prefers warmth) make Act 2's narrative "this is why cohort analysis matters" rather than a single-metric knockout. O2 authoring pulls forward into H1 per the arena-native content pivot. | H4 (O2), H1 rework |
| ✅ D7 | **Two demos sequentially (one engineer) or in parallel (two engineers)?** | **Sequential, one engineer.** Hero demo H0→H3 first, then operator demo H4→H6. Calendar: ~7.5–10 weeks total (see Totals table). No compression from parallelism. | H4 start date |
| ✅ D8 | **Cloud-private LLM backend for Act 5** | **Azure AI Foundry with Private Endpoint on Azure AKS.** Rationale: (a) user has Azure credits, (b) roadmap Phase 1 already allocates Azure sandbox work, (c) Omnia's `azure-ai` provider type is first-class with `baseURL` support for private endpoints (`pkg/provider/types.go:50-52`, `api/v1alpha1/provider_types.go:174`), (d) Azure workload identity support aligns with native cloud-private auth. | H4 (O4, new O8b) |

---

## Pre-H0 — Verification batch (~Day 1-2 of week 1)

**Exit criterion**: V1, V2, V3, SH1 complete. Any issues V3 surfaces are either fixed or filed with a known work item.

**Start here**: `docs/local-backlog/demo-kickoff.md` — Day 1 schedule with procedures for each item.

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| V1 | 🧪 | Verify Claude availability on Azure AI Foundry 2026 | 30-60 min | — | Y |
| SH1 | 🛠 | Create Shopify partner dev store (Dawn theme, note URL) | 30 min | — | Y |
| V2 | 🧪 | Read Azure AI Foundry Private Endpoint docs; produce procedure cheat sheet | 1-2 hours | — | Y |
| V3 | 🧪 | **Critical**: provision Azure AI Foundry + Private Endpoint, configure Omnia `azure-ai` Provider CRD, run minimal agent session end-to-end, confirm traffic flows over Private Endpoint | 3-4 hours (0.5-1 day) | V2 | N |
| VP | 🧪 | **packc smoke test** — once arena-native content lands, run `packc compile -c acme-apparel-support/config.arena.yaml --id acme-apparel-support` and confirm the output pack JSON validates via `packc validate`. Catches schema misunderstandings before H0 starts; gates the "content is real" claim. | 30 min | arena-native content pivot complete | Y |

**Pre-H0 total: ~1-2.5 engineer-days (mostly Day 1) — unchanged; VP is trivial once pivot content exists.**

If V3 surfaces a gap in Omnia's Azure SDK client: fix that before H0. Estimated 0.5-1 day. Better to catch this now than in week 4.

---

## H0 — Reliability pre-flight (~1 week)

**Detailed plan**: `docs/local-backlog/demo-h0-plan.md` — TDD-ready task decomposition for each R-item.

**Exit criterion**: 10 consecutive doctor smoke-test runs pass 29/29 on GPT-4o Azure AI Foundry, with memory retrieval p95 < 300ms.

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| R1 | 🛠 | Confirm GPT-4o Azure AI Foundry handles memory tool-calls reliably (20 remember→recall cycles, schema compatibility) | 0.5-1 day | V3 | N |
| R2 | 🧪 | **R2.1 DONE 2026-04-11**: Omnia populators confirmed orphaned — PromptKit handles extraction natively via `sdk.WithMemory()`. Cleanup filed as separate backlog. **R2.2 remaining**: verify PromptKit extraction quality against GPT-4o via Azure OpenAI (gated on V3) | 0.5 day (R2.2 only, gated on V3) | R1, V3 | N |
| R3 | 🛠 | Doctor `MemoryPersistsAcrossSessions` → 10/10 consecutive runs at 29/29. Fixes flow from R1+R2. Capture evidence | 2-3 days | R1, R2 | N |
| R4 | 🛠 | Find or add memory retrieval latency instrumentation. Measure p95 against Azure AI Foundry over 100 sessions. Add test assertion | 0.5-1.5 days | R3 | Y (with R3 end) |

**H0 total: ~4-7 engineer-days.**

---

## H1 — Hero demo: Shopify + widget + tools + content (~2 weeks)

**Exit criterion**: Sarah can log into the Shopify dev store, open the widget, type a message, get a real Shopify-Admin-API-powered response. No KPIs, no script — just mechanics.

### H1.a — Shopify dev store (SH1-SH4)

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| SH1 | ✅ DONE 2026-04-11 | Shopify partner dev store created — `https://acme-apparel-omnia-demo.myshopify.com/` | 0.5d | — | Y (with everything) |
| SH2 | 🛠 | Seed products (~20 apparel items), customers (Sarah Chen, Marcus Webb, +2-3), orders (~15 across states: delivered, in-transit, delayed, duplicate-charge, refunded) | 0.5-1d | SH1 | Y |
| SH3 | 🛠 | Generate Shopify Admin API token; store in Omnia workspace secret. Verify required scopes: `read_orders`, `read_customers`, `write_discounts`, `read_products`, possibly `write_price_rules` | 0.5d | SH1 | Y |
| SH4 | ✅ DRAFT 2026-04-11 | 8 KB articles drafted at `docs/local-backlog/drafts/kb/` — shipping-policy, returns-and-exchanges, sizing-guide, care-instructions, order-tracking, damaged-or-incorrect-items, international-shipping, faq. README + consistent-facts table. Ready to be mounted by stub KB service (T4) in H1. | 0.5d | — | Y |

### H1.b — Chat widget (W1-W7)

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| W1 | 🛠 | Vanilla JS widget core: launcher, panel, message list, input, typing indicator, streaming render. Single `chat.js`, esbuild/vite. | 2-3d | — | Y |
| W2 | 🛠 | WebSocket client → Omnia facade; reconnect, session persistence, auth token passthrough | 1d | W1 | N |
| W3 | 🛠 | Styling: Acme Apparel-appropriate, mobile responsive, accessible, WCAG AA | 1d | W1 | Y (with W2) |
| W4 | 🛠 | Shopify customer context read via **Customer Account API** (D2 resolved); fallback to anonymous device ID when signed out. Size bumped to reflect OAuth wiring vs legacy global. | 1-1.5d | W2 | N |
| W5 | 🛠 | "Delete my chat data" affordance in widget settings panel; calls DSAR endpoint | 0.5d | W1 | Y |
| W6 | 🛠 | Build pipeline: `chat.js` + `chat.css` published to URL Shopify theme can `<script src="...">`. Recommend serving from operator's static asset handler | 0.5d | W1, W2, W3 | N |
| W7 | 🛠 | Install into Acme Apparel dev store via theme customizer | 0.5d | W6, SH1 | N |

### H1.c — Support tools + stub backings (T1-T8)

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| T1 | 🛠 | `lookup_order` tool → Shopify Admin API `GET /orders/{id}.json`. Per arena-native pivot: authored as `acme-apparel-support/tools/lookup-order.tool.yaml` (`kind: Tool`, `mode: mock` with fixture) at content-authoring time; wrapped into Omnia ToolRegistry CRD during H2.b Helm chart packaging. | 0.5-1d | SH3 | Y |
| T2 | 🛠 | `issue_discount_code` tool → two-step price_rule + discount_code. May need small HTTP wrapper for composition. Same bifurcation as T1 (tool YAML + Helm wrap). | 1-2d | SH3 | Y |
| T3 | 🛠 | `lookup_customer` tool → Shopify Admin API `GET /customers/{id}.json`. Same bifurcation as T1. | 0.5d | SH3 | Y |
| T4 | 🛠 | Stub KB service: single Go binary, `GET /search?q=...`, BM25 or keyword match. Tool schema (`search-kb.tool.yaml`) authored alongside T1-T3; backing Go service is unchanged runtime work. | 1-2d | SH4 | Y |
| T5 | 🛠 | Stub escalation queue service: Go binary, `POST /escalations`, `GET /escalations`, `DELETE /escalations/{id}`. Tool schema (`escalate-to-human.tool.yaml`) authored alongside T1-T3; backing Go service is unchanged runtime work. | 0.5-1d | — | Y |
| T6 | ~~🛠~~ | **FOLDED INTO T1-T5** per arena-native pivot. The `.tool.yaml` files authored under T1-T5 are the registry entries; no separate "ToolRegistry entries" task needed. H2.b Helm chart packaging wraps them for deployment. | — | — | — |
| T7 | ✅ DONE 2026-04-11 | Variant A PromptConfig authored at `acme-apparel-support/prompts/variant-a-agent.yaml` + 4 fragment files. promptarena validate passes. | — | — | — |
| T8 | ✅ DONE 2026-04-11 | 6 personas at `acme-apparel-support/personas/*.persona.yaml`. Content unchanged from original drafts; file moves + rename complete. | — | — | — |

**H1 total: ~13-19 engineer-days.**

---

## H2 — Hero demo: evals + dashboard + Helm chart (~1 week)

**Exit criterion**: All five hero demo scenes run end-to-end in dev. `helm install omnia-demo` on a fresh cluster works.

### H2.a — KPIs as evals (K1-K5)

> **Pivot impact**: K1 and K2 move into H1 as part of the T7 rework — they're authored inline in `config.arena.yaml` `spec.pack_evals[]` alongside the PromptConfigs. K3, K4, K5 remain in H2.a as Grafana / UI work downstream of the metrics.

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| K1 | ✅ DONE 2026-04-11 | `session_outcome` pack-level eval in `acme-apparel-support/config.arena.yaml` spec.pack_evals[]. LLM-as-judge counter metric with outcome label. | — | — | — |
| K2 | ✅ DONE 2026-04-11 | `customer_sentiment` pack-level eval. LLM-as-judge gauge metric, range -1..+1. | — | — | — |
| K3 | 🛠 | KPI dashboard strip component. React component querying Prometheus HTTP API for: resolution rate (from K1), cost/conversation (existing metrics), avg handle time (existing metrics), escalation rate (from K1). **Requires §8-Q11 metric name verification** | 1-2d | K1 | Y |
| K4 | 🛠 | "What did the agent remember" panel on session detail view. Reads memory-api for entities accessed during the session. May need new memory-api query endpoint | 2d | — | Y |
| K5 | 🛠 | Escalation queue view page — reads from T5 stub, OR derived from `acme_session_outcome_total{outcome="escalated"}` | 1d | T5 or K1 | Y |

### H2.b — Demo packaging (D1-D3, renamed from "bundle packaging")

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| D1 | 🛠 | `omnia-demo` Helm chart. Installs: Provider, PromptPack, AgentRuntime, ToolRegistry entries, Workspace, stub KB service, stub escalation queue service, workspace secrets (Shopify API token) | 3-5d | All of H1 | N |
| D2 | 🛠 | Pre-install Job / post-install README: clear demo memories, verify Shopify connectivity, seed fresh KB corpus | 1d | D1 | N |
| D3 | 🎨 | `charts/omnia-demo/DEMO_SHAPE.md` — descriptive artifact for the future bundle-spec author. Not a spec; a description of the CRDs and files that make the demo work | 0.5d | D1 | Y |

### H2.c — Verification tasks (from hero demo §8)

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| Q11 | 🧪 | Verify existing metric names: session duration histogram, tool call counter w/ tool-name label, per-turn/per-session cost metric. Grep + poke `/metrics` on a running pod | 0.5h | — | Y |
| Q12 | 🧪 | Verify PromptKit eval hooks are language-agnostic (HTTP/gRPC/subprocess). Affects Act 1 narration, not demo mechanics | 0.5-1d | — | Y |
| Q3 | 🧪 | Verify Shopify Admin API scopes (`write_discounts` vs `write_price_rules` for T2) | 0.5h | SH3 | Y |
| Q4 | 🧪 | Check Shopify Admin API rate limits for 2026; consider GraphQL for T1-T3 if needed | 0.5h | — | Y |

**H2 total: ~8.5-12 engineer-days.**

---

## H3 — Hero demo: polish + record (~0.5-1 week)

**Exit criterion**: Recorded hero demo + live Azure sandbox.

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| H3.1 | 🛠 | DSAR cascade visibility polish (toast → progress stream) — OPTIONAL | 1-2d | H2 | Y |
| H3.2 | 🎨 | Narration script, rehearse | 1d | H2 | Y |
| H3.3 | 🎬 | Record hero demo, capture screenshots for landing page | 1-2d | H3.2 | N |
| H3.4 | 🛠 | Provision Azure AKS sandbox; deploy `omnia-demo` chart; verify demo runs end-to-end on sandbox | 1 week parallel (Phase 1 P1 from roadmap) | D1 | Y (parallel with all H0-H3) |

**H3 total: ~3-5 engineer-days (plus Azure sandbox ~1w parallel).**

---

## H4 — Operator demo: self-play + environment (~1.5 weeks)

**Exit criterion**: Self-play ArenaJob runs 20-50 sessions/min against variant A; variant B authored; private namespace enforces NetworkPolicy; `check_warehouse_inventory` works from inside the cluster and fails from outside.

### H4.a — Self-play + variant B (O1-O3)

> **Pivot impact**: O1 and O2 move into H1 as part of the arena-native pivot — both are authored under `acme-apparel-support/scenarios/` and `acme-apparel-support/prompts/` alongside variant A. O3 remains in H4 because it's cluster-side (ArenaJob CRD), not content.

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| O1 | ✅ DONE 2026-04-11 | Self-play scenario at `acme-apparel-support/scenarios/selfplay-mixed-personas.scenario.yaml` cycling all 6 personas. | — | — | — |
| O2 | ✅ DONE 2026-04-11 | Variant B PromptConfig at `acme-apparel-support/prompts/variant-b-agent.yaml` + 4 fragment files. Less-apologetic/confident per D6. | — | — | — |
| O3 | 🛠 | Continuous ArenaJob config (Omnia cluster CRD) running self-play scenarios at ~20-50 sessions/min indefinitely. Unchanged — still H4 work. | 0.5d | O1 (content) | N |

### H4.b — On-prem demo environment (O4-O8)

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| O4 | 🛠 | "Private network" namespace + NetworkPolicy: no public ingress, only WebSocket facade exposed via LoadBalancer/Ingress. **Depends on D5** | 0.5-1d | D5 | Y |
| O5 | 🛠 | Stub warehouse management service: Go HTTP, `GET /inventory/{sku}`, fixture data | 0.5d | — | Y |
| O6 | 🛠 | ToolRegistry entry `check_warehouse_inventory` → in-cluster warehouse service (internal DNS) | 0.25d | O5 | N |
| O7 | 🛠 | "Outside world" demo pod (or external jumphost) for curl-fails beat | 0.25d | O4 | Y |
| O8 | 🛠 | On-prem Ollama instance in demo cluster; Provider CRD pointing at it. **Depends on D4. Recommended mandatory** | 1d | D4 | Y |

**H4 total: ~5.5-8 engineer-days.**

---

## H5 — Operator demo: dashboards + rehearsal (~1-1.5 weeks)

**Exit criterion**: All six operator demo acts can be demoed manually end-to-end in real time with live dashboards; rollout drill (Act 3) and failover drill (Act 4) have been rehearsed ≥ twice.

### H5.a — Dashboarding (O9-O12)

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| O9 | 🛠 | Grafana operator-demo dashboard: live resolution rate, cost histogram, eval score distributions, escalation ticker, active sessions gauge. Reuses hero demo's evals → Prometheus pipeline | 2-3d | K1, K2, O3 | Y |
| O10 | 🛠 or 🧪 | Rollout split-cohort KPI view. **Verify first** — may already exist from #758-#763 | 0d (if exists) or 1-2d | #758-#763 exists | Y |
| O11 | 🛠 or 🧪 | Provider health panel. **Verify first** — may already exist from #785 | 0d (if exists) or 1d | #785 | Y |
| O12 | 🛠 or 🎨 | Aggregate memory view ("trending topics") — use real `memory-analytics-operator-view-spec.md` if built, else static placeholder screenshot | 2-3d real / 0.5d placeholder | — | Y |

### H5.b — Assets (O13-O14)

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| O13 | 🎨 | Architecture diagram asset — SVG or high-res PNG showing on-prem deployment, network boundary, WebSocket exposure | 0.5d | — | Y |
| O14 | 🎨 | Comparison chart asset — SaaS chatbots vs Omnia (hosted where, internal-systems access, data boundary, etc.) | 0.5d | — | Y |

### H5.c — Rehearsal (O15-O16)

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| O15 | 🎨 | Narration script, act-by-act | 1d | All H5.a-b | N |
| O16 | 🎬 | Rehearsal: full 6-act run-through with rollout timing drill and provider failover drill. ≥ 2 full rehearsals | 1-2d | H4, H5.a, O15 | N |

**H5 total: ~7-12 engineer-days (depending on how much of H5.a already exists).**

---

## H6 — Operator demo: record + edit (~0.5-1 week)

**Exit criterion**: Final recorded operator demo published to landing page alongside hero demo.

| ID | Type | Item | Size | Depends on | Parallel? |
|---|---|---|---|---|---|
| O17 | 🎬 | Record all 6 acts; edit — time-compress Act 3's rollout analysis window (see §8-Q1 recommendation: configure shorter analysis window for demo, label truthfully), splice acts, add overlays, animate architecture diagram. Review + retake as needed | 1-2d | H5 | N |
| O18 | 🎨 | Publish both recordings to landing page with shared visual language (colors, fonts, overlay style) | 0.5-1d | O17, H3.3 | N |

**H6 total: ~2-4 engineer-days.**

---

## External dependencies & risks

Things outside the build that could delay:

| Risk | Affects | Mitigation |
|---|---|---|
| **PromptKit#836** (null arg coercion on Ollama) | Mitigated by D1 resolution (using GPT-4o on Azure AI Foundry, not Ollama, for agent role). Still relevant for self-play customer role — but the customer role uses in-cluster Ollama only for text generation, not tool-calling, so #836 doesn't bite that path. | Only bites if we ever switch the agent role back to Ollama |
| **Claude-on-Azure-AI-Foundry unknown** | D1 assumes GPT-4o; if Claude becomes available on Azure AI Foundry, we'd rather use Claude | V1 verifies; trivial to swap Provider CRD if Claude is available |
| **Omnia `azure-ai` provider + Private Endpoint end-to-end path unverified** | H4 (O8b), all of Act 5 private deployment story | V3 verifies end-to-end before H4 |
| **Shopify API changes** — Customer Account API vs `window.Shopify.customer`, current rate limits, scope taxonomy | W4, T1-T3 | Q-tasks verify current state before H1; Q3, Q4 in H2.c |
| **PromptKit eval hook language-agnosticism** | Operator demo Act 1 narration (not mechanics) | Q12 verifies before recording |
| **Memory analytics view status** (`memory-analytics-operator-view-spec.md`) | O12 — real or placeholder | Placeholder path available; defer for v1 if not ready |
| **Shopify on-camera outage** during recording | H3.3, O17 | Record don't live-stream; rehearsal just before recording; local fallback for hero demo |
| **Azure sandbox provisioning delay** | H3.4, H4 (O4) | Kick off early; parallel with H0-H2; ~1 week allocation. Operator demo's dedicated AKS (D5) may be same or different from hero demo's AKS |
| **Test LLM budget** (ongoing self-play customer role) | H4-H5 | Use in-cluster Ollama for customer role (free); GPT-4o Azure AI Foundry for agent role (covered by Azure credits); burst self-play only during rehearsal + recording |

## Verification tasks (V1-V3)

Cheap up-front investigations that reduce risk. Do before committing to H1/H4.

| ID | Type | Item | Size | Depends on | Unblocks |
|---|---|---|---|---|---|
| V1 | 🧪 | **Verify Claude availability on Azure AI Foundry 2026.** Check model catalog in Azure portal and `learn.microsoft.com/azure/ai-studio`. If Claude is available as a serverless API or managed endpoint, switch D1 to Claude (free upgrade). If not (my expectation), stick with GPT-4o. | 30-60 min | — | D1 final confirmation |
| V2 | 🧪 | **Verify Azure AI Foundry Private Endpoint setup procedure for the demo cluster.** Read current Azure docs on Private Endpoints for Azure AI Foundry / Azure OpenAI Service. Confirm the steps: VNet + subnet + Private Endpoint + DNS zone. Identify any gotchas (region availability, resource provider pre-reqs, managed identity setup). | 1-2 hours | — | H4 O4, O8b |
| V3 | 🧪 | **Verify Omnia's `azure-ai` provider works end-to-end with a Private Endpoint baseURL.** Provision a test Azure AI Foundry resource, create a Private Endpoint, configure an Omnia Provider CRD with `type: azure-ai`, `baseURL: <private endpoint>`, `auth.type: workloadIdentity`, and run a minimal agent session. This is the cheapest way to catch any gaps between the CRD definition and the runtime Azure SDK client's private-endpoint handling. | 0.5-1 day | V2, Azure test tenancy | H4 O8b |

---

## Parallelization map

**Sequential spine (can't be broken)**:
R1/R2 → R3 → H1.a+H1.b+H1.c → D1 → H3.3 → H5.c → O17

**Opportunities for parallelism**:

| Stream | Tasks | When |
|---|---|---|
| **Azure sandbox provisioning** | H3.4 | Parallel with H0-H2 (independent) |
| **Shopify dev store setup** (SH1-SH4) | independent | Any time once D1 (LLM decision) is made |
| **Content authoring** (T7, T8, K1, K2, O2) | independent | Any time once T7 exists (O2 depends on T7) |
| **Widget core** (W1, W3) | independent | Any time |
| **Tool ToolRegistry entries** (T1, T2, T3, T6, O6) | independent | Any time once SH3 is done |
| **Stub services** (T4, T5, O5) | independent | Any time |
| **Self-play setup** (O1, O3) | requires T8 | Can start as soon as T8 lands, potentially overlapping H2-H3 |
| **Operator demo environment** (O4-O8) | mostly independent | Can start anytime after D5 + D4 decisions |
| **Diagram assets** (O13, O14, D3) | independent | Any time |

**With two engineers, the plan compresses to ~6-7 weeks calendar** (one on hero demo H0→H3, the other on operator demo H4→H6 starting after hero demo's T7+T8 land in H1).

---

## What this build plan is NOT

- Not a replacement for the specs. The specs explain *why*; this is *what*.
- Not an implementation plan. Each item needs a design pass before coding.
- Not a project schedule. Calendar weeks assume full-time single-engineer focus; adjust for context.
- Not a substitute for the pre-flight decisions (D1-D7). Resolve those first.

---

## Next concrete action

Pick one:

1. **Resolve D1 (demo LLM backend)** — unblocks H0
2. **Start SH1 (create Shopify dev store)** — 30 minutes, zero dependencies, lets content authoring begin
3. **Write H0 implementation plan** — turn R1-R4 into TDD-ready tasks
4. **Verify Q11 metric names** — 30 minutes, eliminates risk from H2
5. **Verify D2 (Shopify customer auth API state)** — 1 hour of reading Shopify docs; unblocks W4 design

I'd do them in the order (2, 4, 5, 1, 3) — cheap/fast items first to eliminate unknowns before committing to direction.
