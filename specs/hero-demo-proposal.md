# Omnia Hero Demo — Proposal

**Status**: Draft v0.4 (demo deliberately does not define "bundle")
**Created**: 2026-04-10
**Updated**: 2026-04-10 — switched from fake Next.js storefront to Shopify dev store
**Updated**: 2026-04-10 — KPIs are evals with Prometheus metric declarations; drops ~2-3 days of work
**Updated**: 2026-04-10 — removed "bundle" as an abstraction the demo must define; demo uses existing CRDs + a Helm chart, and its concrete artifact set becomes an *input* to whoever writes the bundle spec v0.1 later
**Owner**: TBD
**Local-only**: This doc lives in `docs/local-backlog/` and is not committed. Canonical home (when promoted) is `planning/products/omnia/hero-demo-proposal.md`, which is referenced from `strategy/roadmap.md`, `products/omnia/build-inventory.md`, `products/omnia/cloud-marketplace-proposal.md`, and `products/omnia/mcp-tool-builder-proposal.md` — **but does not currently exist in the planning repo**. This draft fills that hole.

---

## 1. TL;DR

Build a 10-minute, prospect-facing demo of a **customer support agent** running on Omnia, resolving realistic ecommerce tickets, remembering returning customers across sessions, escalating when appropriate, and showing live KPIs (resolution rate, cost per conversation, escalation rate).

**Demo backdrop**: A **real Shopify development store** — a seeded "Acme Apparel" storefront with real products, customers, and orders. Omnia ships as a **chat widget injected via `<script>` tag** into the store's theme. The agent's tools call **real Shopify Admin API endpoints** (order lookup, discount code creation) plus two small stub services (KB search, escalation queue). The operator watches the session live in Omnia's dashboard, side-by-side with the customer's view of the storefront.

**LLM backend**: **GPT-4o on Azure AI Foundry with Private Endpoint** (uses the Omnia `azure-ai` provider type at `pkg/provider/types.go:50-52`). Aligns with the roadmap's Azure sandbox work (Phase 1 P1) and the operator demo's cloud-private Mode 2 deployment. If Claude becomes available on Azure AI Foundry, switch to Claude as a free upgrade (verify via V1 in `docs/local-backlog/demo-build-plan.md`).

Most of the platform capabilities the demo needs already exist. The gap is not plumbing — it is:
1. **Vanilla-JS chat widget** (script-tag embeddable) — new, ~5-6 days
2. **Shopify dev store setup + seed data** — new, ~1 day
3. **Shopify Admin API tool integration** — ToolRegistry entries pointing at Shopify endpoints; ~1-2 days
4. **KPI dashboard strip** — KPIs are **evals declared in the PromptPack** with Prometheus metric names; the runtime already emits eval values to Prometheus via `MetricRecorder`. Net work: define the evals (YAML + prompts, ~0.5 day) and build a dashboard component that queries Prometheus (~1-2 days). **No new middleware or aggregator.**
5. **Small stub services** — KB search + escalation queue; ~1-2 days total
6. **Reliability of memory** — platform wires it, but PromptKit arg-coercion bug and populator wiring gaps make it flaky on Ollama
7. **Demo packaging** — an `omnia-demo` Helm chart that wires existing CRDs (PromptPack, ToolRegistry, AgentRuntime, Workspace, Provider), stub services, and Shopify credentials into a working install. **No bundle manifest, no new CRD, no new abstraction.**

**Key insight for the KPI story**: Omnia already treats KPIs as a first-class, declarative, extensible concept. Evals in a PromptPack can declare `metric: { name, type, bounds }` (see `internal/schema/promptpack.schema.json:1170-1196`); the runtime's `MetricRecorder` emits eval result values with those names to Prometheus (see `internal/runtime/metrics_integration_test.go:241`); PromQL does the rest. This means the demo can honestly pitch *"every KPI on this dashboard is an eval declared in the support PromptPack; write your own in five minutes"* without that being aspirational — it's how the platform already works.

**A deliberate non-goal: defining "bundle."** The planning docs (`roadmap.md`, `build-inventory.md`) describe "Knowledge Bundles" as the unit of delivery — roughly `Adapters + Tools + Instructions + Workflows + Guardrails + KPIs` — but **there is no code in the Omnia repo that defines this abstraction**. No CRD, no Go type, no schema. The same roadmap has "Bundle spec v0.1" as a separate Phase 1 deliverable with its own 3-4 week budget, acknowledging that *formalizing what a bundle is* is still pending. This demo spec **deliberately does not try to define it**. Instead, the demo ships using existing Omnia primitives (CRDs + Helm chart); the resulting concrete artifact set then becomes an input to whoever eventually writes bundle spec v0.1. Bottom-up abstraction from a working example is cheaper and more honest than top-down invention.

This doc defines the narrative, maps each scene to the code paths it exercises, enumerates exactly what exists vs. what must be built, and proposes a phased build path.

---

## 2. Strategic context

From `planning/strategy/roadmap.md` (March 2026, Bundle-First / Talk-First):

- Omnia's first ICP is **Talk to Customers** (customer-facing conversational agents).
- The unit of delivery is a **Knowledge Bundle**: `Adapters + Tools + Instructions + Workflows + Guardrails + KPIs`.
- Phase 1 (weeks 1–8) is **Demonstrate**: hero demo + bundle spec v0.1 + Azure sandbox.
- Phase 2 (weeks 9–24) is **Differentiate**: agentic memory + KPI infra + escalation/handoff.
- Week-8 success: *"A prospect watches a 10-minute demo, sees live resolution rate and cost metrics, and says 'I want that for my team.'"*
- Week-24 success: *"An agent remembers a returning customer, resolves their issue faster because of context from prior conversations, and the dashboard shows measurable improvement."*

This demo **intentionally** collapses the Week-8 and Week-24 success criteria into a single artifact. The platform's memory work is already ~90% built (see §5); separating "hero demo" from "memory demo" wastes the investment and delivers a less differentiated Phase 1.

**Why Shopify and not a bespoke fake storefront**: A Shopify partner development store is free, comes pre-plumbed with products/cart/accounts/orders/admin, and — critically — gives the demo **real Shopify Admin API calls** as tool backends. That moves the story from "here's how it *would* integrate with your ecommerce platform" to "here it is, right now, reading live order data from a Shopify store and issuing real discount codes." It's also a strategic building block: the same widget + tool wiring is the starting point for a future Shopify Tier 2 private-app pilot or Tier 3 App Store listing (both explicitly out of scope for this demo — see §9).

---

## 3. The narrative (10 minutes)

**Setting**: "Acme Apparel" — a Shopify dev store, seeded with ~20 products, 4-5 customers, and ~15 orders including a few in interesting states (delayed shipping, duplicate charge, delivered). Omnia's chat widget is embedded via script tag in the Dawn theme.

**Format**: **Split screen**. Left side: the Acme Apparel storefront (what Sarah sees). Right side: the Omnia dashboard (what the operator sees). Both are real, both update live, both tell a different half of the story.

**Persona on camera**: Omnia platform operator / support lead (narrator).

### Scene 0 — Setup (30 sec, pre-roll)

Narrator: *"Two things on screen. On the left, a real Shopify store running the default theme with a chat widget in the corner. On the right, Omnia — the platform running the agent that powers that chat widget. Everything you're about to see is wired together for real: real Shopify orders, real Omnia sessions, real metrics."*

Screen: Both sides pre-loaded. Storefront idle. Omnia dashboard showing the KPI strip.

### Scene 1 — The dashboard (1.5 min)

Focus on the **right side** (Omnia dashboard).

Screen shows:
- **Support KPIs strip**: Resolution rate 73%, Avg handle time 2m18s, Cost/conversation $0.08, Escalation rate 12%, 847 conversations today
- **Live session list** with recent resolved/escalated tags
- **Cost breakdown** by provider

Narrator: *"These aren't vanity metrics. Resolution is measured from the conversation outcome, not human tagging. Cost is tracked per turn. Escalation is a first-class event. You can go from any of these aggregates into a specific conversation and see every tool call, every token, every decision the agent made."*

Narrator clicks into one recent session — shows the full session view: trace, tool calls, memory accesses, cost per turn.

### Scene 2 — A real customer with a real problem (3 min)

Focus shifts to the **left side** (Shopify storefront).

Screen: Acme Apparel home page. Narrator clicks "Sign in," logs in as **Sarah Chen** (one of the seeded test customers). Sarah's account shows a recent order — "Order #1023: Vintage Linen Dress — shipped, delayed."

Sarah clicks the chat launcher (bottom-right of every page). Widget slides open. Empty thread, greeting message.

Sarah types:
> *"My order #1023 never arrived and it's been 10 days. I need this for a birthday party this weekend, getting really stressed."*

**Both sides become active.** Left side: agent response streams into the chat widget. Right side: a new session appears in Omnia, trace expands live.

Viewer watches:

1. Agent calls `lookup_order("1023")` → **real Shopify Admin API** → returns order details, fulfillment status "in transit - carrier delay"
2. Agent calls `search_kb("shipping delay policy")` → returns policy excerpt from the seeded KB corpus
3. Agent drafts an empathetic reply + delivery update + offer (expedited shipping voucher)
4. Agent calls `issue_discount_code(customer="sarah@example.com", type="EXPRESS_UPGRADE")` → **real Shopify Admin API** → creates a real price rule + discount code
5. Reply sends. Sarah sees it in the widget.

Right-side dashboard shows: 3 tool calls, cost $0.06, session marked resolved. KPI strip increments.

Narrator: *"Three tool calls. Two of them hitting live Shopify APIs over the public internet. One hitting our internal knowledge base — a service running right inside the same cluster as Omnia. That's the deployment model: Omnia runs inside your network, so its agents can call your internal systems directly — no webhook holes in your firewall, no proxy agents, no public API contortions. The only thing exposed to the outside world is the chat widget's WebSocket connection. We'll show the full architecture in the operator demo."*

Narrator switches to the Shopify admin (briefly, ~15 sec) and shows the newly created discount code in Shopify's own UI. *"This is real. That code works."*

### Scene 3 — The memory moment (2.5 min)

Narrator: *"Now the interesting part."*

Left side: Sarah signs out. Wall-clock time skip (cut to "30 minutes later" overlay, or just a real 30-second pause).

Sarah signs back in. Opens the chat widget — fresh thread. Types:

> *"Hey — just wanted to say the voucher worked and I got the dress in time. Thanks!"*

**Agent's very first response, no tool calls**:
> *"Hi Sarah — so glad to hear the express shipping got your dress there in time for the party. Is there anything else I can help with today?"*

On the right side, narrator clicks the new **"What the agent remembered"** panel. Shows memory entries extracted from the previous session:
- `customer: Sarah Chen <sarah@example.com>`
- `issue: delayed order #1023 (linen dress, birthday gift, urgent)`
- `resolution: EXPRESS_UPGRADE discount code issued`
- `sentiment: stressed → resolved positively`

Narrator: *"The agent didn't look up the order again. It didn't ask her to explain. It remembered. This is the difference between an agent that transcribes and an agent that builds a relationship. And it's cross-session — Sarah is technically in a brand new WebSocket session, but her memory scope is consistent because the facade pseudonymizes her Shopify customer ID and the memory store uses that as the scope key."*

Brief right-side aside: narrator shows the `/memories` dashboard page, scoped to Sarah's user ID, with the graph view of her entities and relationships.

### Scene 4 — Escalation (1.5 min)

Left side: sign out. Sign in as **Marcus Webb** (seeded customer with a duplicate-charge order).

Marcus opens the chat:
> *"CHARGEBACK DISPUTE: You charged me twice for order #1041, my bank is involved, I want my money today."*

Right side shows the agent reasoning:
1. Calls `lookup_order("1041")` → order flagged as "duplicate_charge"
2. **Does not** attempt a refund directly — the bundle's guardrails forbid it
3. Calls `escalate_to_human(reason="chargeback_dispute", priority="high")` → stub queue
4. Responds to Marcus: *"I've flagged this for immediate review by our billing team. A human specialist will reach out within 2 hours. Your reference is TKT-4421."*

Right-side dashboard: escalation queue view updates with Marcus's ticket. Escalation rate KPI ticks up.

Narrator: *"Chargebacks require a human. The agent's guardrails — expressed in the PromptPack's system prompt — enforced it. The escalation is tracked as a KPI, not a failure. And the 'human team' in this demo is just a stub queue — but in a real deployment, it's whatever queue the merchant already uses."*

### Scene 5 — Compliance on camera (1 min)

Narrator: *"One last thing — GDPR."*

Sarah signs back in. Navigates to her Shopify account → a small "Chat history & privacy" section (added by the widget). Clicks "Delete my chat data."

- Widget calls Omnia's DSAR endpoint
- Right side: dashboard shows the cascade in real time — memory-api delete → session-api delete → audit log entry → warehouse sync trigger
- A fresh chat session is opened as Sarah. Agent responds as if it's the first time she's ever contacted Acme. The "remembered" panel is empty.

Narrator: *"One click. Memory-api, session-api, audit log, warehouse sync — all cascade. This isn't a post-hoc scrub; it's wired into the platform. Omnia's had this since PR #783."*

### Closing (30 sec)

Narrator: *"Everything you saw was configured declaratively — one PromptPack, a handful of ToolRegistry entries, a few Kubernetes manifests, installed with one Helm command. Swap the PromptPack to change the tone. Swap the ToolRegistry to change the tools. Swap the eval definitions to change the KPIs. The platform is the same. Ship a support bot on Shopify in an afternoon, not a quarter."*

Total: ~10 minutes.

---

## 4. Capability map — narrative to code paths

| Scene | What the viewer sees | Code paths exercised |
|---|---|---|
| 0 | Split-screen setup, widget already embedded | Shopify theme customizer with `<script>` tag injected, script loads from Omnia's CDN (or ingress) |
| 1 | KPI dashboard strip | **Existing eval → Prometheus path** (`PromptPack eval.metric` → `MetricRecorder` → Prometheus, see `internal/runtime/metrics_integration_test.go:241`). **NEW**: `session_outcome` eval definition in the support PromptPack (YAML); dashboard component that queries Prometheus HTTP API (or embeds Grafana). No new middleware. |
| 1 | Session list, session detail | Existing `/sessions` page, session-api, session store |
| 1 | Cost breakdown | Existing cost analytics in dashboard |
| 2 | Sarah signs into Shopify, opens chat widget | Shopify's own customer auth. Widget reads `window.Shopify.customer` (or modern Customer Account API equivalent — **verify current API**), passes identity + JWT to Omnia facade over WebSocket |
| 2 | Agent streams response into widget | Existing facade WS + runtime streaming. **NEW**: widget as a real WS client with streaming message rendering |
| 2 | `lookup_order` tool call | Existing ToolRegistry + HTTP executor; **NEW**: ToolRegistry entry pointing at `https://acme-apparel.myshopify.com/admin/api/2024-01/orders/{id}.json` with Admin API token |
| 2 | `search_kb` tool call | Existing HTTP executor; **NEW**: tiny stub KB service with seeded markdown corpus |
| 2 | `issue_discount_code` tool call | Existing HTTP executor; **NEW**: ToolRegistry entry pointing at Shopify Admin API (`price_rules` + `discount_codes` endpoints) |
| 2 | Session trace visible live in dashboard | Existing session recording (`OmniaEventStore`, `internal/runtime/conversation.go:152`) |
| 2 | Cost per turn | Existing eval/telemetry |
| 2 | Shopify admin showing real discount code | Shopify admin UI (built-in) — narrator just switches tabs to show it |
| 3 | Sarah signs out, signs back in, agent remembers | Existing: `sdk.WithMemory()` at `internal/runtime/conversation.go:176`, PostgresMemoryStore, populators. Widget must pass consistent user identity across sessions so memory scope key is stable. **RELIABILITY GAPS**: PromptKit#836, LLM populator wiring verification |
| 3 | "What did the agent remember" panel | **NEW**: dashboard view on session detail — may need memory-api query "entities accessed during session X" |
| 4 | Escalation tool call | Existing HTTP executor; **NEW**: small stub escalation queue service |
| 4 | Escalation queue view in dashboard | **NEW**: simple list page reading from stub queue |
| 4 | Escalation rate KPI | **NEW**: escalation events emitted at session close or inferred from tool call history |
| 4 | Guardrail: no direct refund on dispute | **Pragmatic**: handled via system prompt in the PromptPack. Full AgentPolicy enforcement (#728, #756) is nice-to-have but not required for demo |
| 5 | "Delete my chat data" button in Shopify account | **NEW**: small UI addition in the widget (account-area UI surface) |
| 5 | DSAR cascade visible in dashboard | Existing DSAR (`ee/pkg/privacy/deletion.go:222-230`, `memory_deleter.go`). **Minor NEW**: make the cascade visible as a progress stream, not just a silent 200 |

---

## 5. What we have (verified, grounded in code)

### 5.1 Platform infrastructure — production-ready

| Capability | Status | Evidence |
|---|---|---|
| Agent runtime (gRPC, streaming) | Done | `cmd/runtime/main.go`, `internal/runtime/` |
| Facade (WebSocket, session management) | Done | `cmd/agent/main.go`, `internal/facade/` |
| Session API (HTTP CRUD + hot/warm/cold tiers) | Done | `cmd/session-api/`, `internal/session/api/`, 15 Postgres migrations |
| Memory API (separate DB) | Done | PR #783 — `omnia_memory` DB, `internal/memory/postgres/migrator.go` |
| 7+ LLM providers | Done | Claude, OpenAI, Gemini, Ollama, Bedrock, Vertex, AzureAI |
| OpenTelemetry tracing | Done | `internal/tracing/` |
| Next.js dashboard embedded in operator | Done | `dashboard/`, served via `dashboard/server.js` |
| Helm chart + generated CRDs | Done | `charts/omnia/` |

**Architectural note relevant to Shopify**: Omnia is **WebSocket-native**. Customer chat is a live WS conversation, not a polled ticket queue. The Shopify widget opens a WebSocket directly to Omnia's facade; each chat session is a WebSocket session. There is no ticket-to-session translation layer required. This is already the shape the platform is built around.

**Architectural note relevant to deployment model**: Omnia is a **Kubernetes workload** that runs inside the customer's own network boundary. Its tool executors have direct network access to whatever the operator's cluster can reach — public APIs (Shopify Admin, SaaS vendors), internal systems (databases, internal HTTP APIs, MCP servers, LDAP, mainframe connectors), or a mix. The **only** externally exposed surface is the WebSocket endpoint the chat widget connects to — same exposure pattern as a public website's frontend. In the hero demo, the Shopify Admin API is accessed over the public internet (external call) while the KB and escalation services are in-cluster (internal calls); this mix is realistic for a typical mid-market deployment. The **operator demo** (see §9 and `docs/local-backlog/operator-demo-proposal.md`) goes deeper on the on-premises story, which is a first-class differentiator against every SaaS chatbot vendor — none of whom can run inside a customer network boundary.

### 5.2 Memory — wired but fragile

| Component | Status | Evidence |
|---|---|---|
| `sdk.WithMemory()` wired in runtime | Done | `internal/runtime/conversation.go:176` — conditional on `s.memoryStore != nil && s.workspaceUID != ""` |
| User ID pseudonymized by facade, passed through as memory scope key | Done | `internal/runtime/conversation.go:173-174` |
| PostgresMemoryStore + graph/timeline tools | Done | `internal/memory/tools.go` — registers `memory__related`, `memory__timeline` alongside PromptKit's `memory__remember`/`memory__recall` |
| Extractor (rule-based populator) | Done | `internal/memory/extractor.go`, populator |
| LLM-based populator | **UNVERIFIED** | `populator_llm.go` exists (190 lines) — unknown whether actually selected in the runtime pipeline. See `docs/local-backlog/agentic-memory-remaining.md` line 71 |
| Privacy middleware, `ShouldRemember()` | Done | `cmd/memory-api/main.go:472-484` |
| PII Redactor (dual: middleware + extraction) | Done | `cmd/memory-api/main.go:470`, `internal/memory/extractor.go:73-75` |
| Audit logging | Done | `internal/memory/api/service.go:32-289` |
| DSAR deletion cascade | Done | `ee/pkg/privacy/deletion.go:222-230` |
| Consent category grants | Done | `ee/pkg/privacy/consent_check.go`, `dashboard/src/components/memories/consent-banner.tsx` |
| `/memories` dashboard page with graph view | Done | PR #690-713 + PR #783 |
| Anonymous device ID support | Done | PR #783 — relevant as a fallback if Sarah chats before signing in |
| Retention worker (TTL-based) | Done | `internal/memory/retention.go` |

**Known reliability issues** (from `docs/local-backlog/agentic-memory-remaining.md`):
- **PromptKit#836** (unresolved): Ollama sends `{"confidence": null, "metadata": null}` for optional fields. PromptKit's `CoerceArgs` doesn't strip nulls, causing JSON schema validation failures on `memory__remember`. Affects demo Scene 3 directly.
- **Doctor smoke test** `MemoryPersistsAcrossSessions` — intermittent pass (26-29/29 overall), specifically because the LLM doesn't always call `memory__remember`.
- **LLM populator wiring**: Code exists, runtime selection path not proven.

### 5.3 Tools — platform ready, demo tools missing

| Capability | Status | Evidence |
|---|---|---|
| ToolRegistry CRD + controller | Done | `api/v1alpha1/toolregistry_types.go`, `internal/controller/` |
| HTTP executor (can call arbitrary HTTP endpoints with auth) | Done | `internal/runtime/tools/` |
| MCP executor + tool filter | Done | PR #771 |
| Client tools (browser-side) | Done | ToolRegistry `client://browser` endpoint |
| ToolPolicy sidecar injection | Done | PRs #728, #755, #756 |
| Circuit breaker on executors | Ticketed | #778 |
| **Shopify Admin API integration as tools** | **NOT BUILT** | Zero entries. Would be HTTP executor ToolRegistry entries with Shopify Admin API bearer token. Shopify's Admin REST API is standard auth + standard endpoints — no custom executor needed. |
| **Stub KB search service** | **NOT BUILT** | |
| **Stub escalation queue service** | **NOT BUILT** | |

### 5.4 Observability & session browser

| Capability | Status |
|---|---|
| Session recording via `OmniaEventStore` | Done (`internal/runtime/conversation.go:152-165`) |
| Session list, search, detail view in dashboard | Done |
| Per-turn cost tracking | Done |
| OTel span inventory for traces | Done (`SERVICES.md`) |
| Grafana dashboards | Done |
| **Support KPI view** (resolution rate, escalation rate, avg handle time) | **NOT BUILT** |
| **"What the agent remembered" per-session panel** | **NOT BUILT** |

### 5.5 Compliance & privacy story

| Capability | Status |
|---|---|
| DSAR cascade (memory + session + audit) | Done |
| Consent banner + category grants | Done |
| Audit log (created/accessed/deleted/exported) | Done |
| PII redaction (inbound + extraction) | Done |
| GDPR/HIPAA/CCPA middleware hooks | Done (`ee/pkg/compliance/`, used by `ee/pkg/privacy/middleware.go`) |

### 5.6 Summary of §5

**The platform is ~90% of the way to the demo already.** The gap is not foundational; it is: (a) a thin vanilla-JS chat widget, (b) ToolRegistry entries pointing at Shopify's Admin API, (c) two tiny stub services (KB + escalation), (d) a KPI view, (e) a Shopify dev store with seeded data, (f) reliability fixes for memory on the chosen demo LLM, (g) a Helm chart wiring it all up.

**What explicitly does NOT need to be built**: a fake storefront, a fake order database, a fake customer auth system, a ticket injection affordance, a fake product catalog. Shopify provides all of those for free.

---

## 6. What we need (gaps, sized)

Ordered roughly by dependency, smallest-first within each group.

### 6.1 Reliability pre-flight (must-fix, blocks demo)

| # | Item | Size | Notes |
|---|---|---|---|
| R1 | **Decide demo LLM backend.** Ollama is flaky on structured tool args (PromptKit#836). Options: (a) fix PromptKit#836 upstream, (b) use Claude/OpenAI with budget caps, (c) use a dedicated Ollama model that behaves better. | Decision + ≤1 day | See §8 open question 1 |
| R2 | Verify LLM populator is actually wired + selected at runtime. Either prove it works or delete `populator_llm.go` from the demo path and rely on rule-based. | 1-2 days investigation | `docs/local-backlog/agentic-memory-remaining.md` line 71 |
| R3 | Doctor smoke test `MemoryPersistsAcrossSessions` must hit 29/29 reliably on the chosen demo LLM. This is the canary. No demo work proceeds past this gate. | 2-3 days (includes R1/R2 fixes flowing in) | `internal/doctor/checks/memory.go` |
| R4 | Memory retrieval latency budget at session start must not exceed ~300ms (so Scene 3 feels instant, not laggy). Measure and document. | 1 day | Existing instrumentation should cover |

### 6.2 Shopify dev store setup (new, small)

| # | Item | Size | Notes |
|---|---|---|---|
| SH1 | Create Shopify partner dev store. Pick theme (recommend default Dawn for familiarity). | 0.5 day | Free; unlimited dev stores per partner account |
| SH2 | Seed products (~20 apparel items — can use Shopify sample CSV or generate with Claude). Seed 4-5 customers (Sarah Chen, Marcus Webb, + 2-3 extras). Seed ~15 orders spanning states: delivered, in-transit, delayed, duplicate-charge, refunded. | 0.5-1 day | |
| SH3 | Generate Shopify Admin API access token (custom app or private app). Store in Omnia workspace secret for the ToolRegistry HTTP executor. | 0.5 day | Scopes needed: `read_orders`, `read_customers`, `write_discounts`, `read_products` |
| SH4 | Seed a small KB corpus (5-10 markdown articles: shipping policy, return policy, sizing guide, etc.) to back the stub KB service. | 0.5 day | |

**Total: ~2-3 days.** This replaces the fake-storefront work from the prior spec revision entirely.

### 6.3 Chat widget (new, medium)

| # | Item | Size | Notes |
|---|---|---|---|
| W1 | Vanilla JS widget: launcher button (bottom-right pill), slide-out panel, message list, input box, typing indicator, streaming message rendering | 2-3 days | No framework, keeps bundle small. Single `chat.js` file bundled with esbuild or vite |
| W2 | WebSocket client → Omnia facade. Handle reconnect, session persistence (reopen widget → resume thread), auth token passthrough | 1 day | |
| W3 | Styling (Acme Apparel-appropriate, mobile responsive, accessible keyboard nav, contrast WCAG AA) | 1 day | |
| W4 | Shopify customer context read. On page load, check `window.Shopify.customer` (or modern Customer Account API equivalent — **verify current state**, see §8). Pass customer ID + email to facade as auth context. Fall back to anonymous device ID when signed out. | 0.5-1 day | Uses existing anonymous device ID path from PR #783 |
| W5 | "Delete my chat data" affordance in widget's settings panel — calls Omnia DSAR endpoint, shows confirmation | 0.5 day | |
| W6 | Build pipeline: `chat.js` + `chat.css` published to a URL the Shopify theme can `<script src="...">` | 0.5 day | Host via operator's existing static asset serving, or S3 + CloudFront |
| W7 | Install into Acme Apparel dev store via theme customizer (paste script tag into `theme.liquid` or use theme settings) | 0.5 day | |

**Total: ~6-7 days.**

**Explicitly out of scope for W1-W7**: Shopify App OAuth flow, ScriptTag API automation, Theme App Extension format, App Store submission, billing integration, per-merchant admin UI, multi-tenant widget hosting. Those are Shopify App Tier 2/3 work — a separate ~3-month product bet (see §9).

### 6.4 Support tools + stub backings (new, small)

| # | Item | Size | Notes |
|---|---|---|---|
| T1 | ToolRegistry entry for `lookup_order` → Shopify Admin API `GET /admin/api/2024-01/orders/{id}.json` with bearer token. Schema maps Shopify's response fields. | 0.5-1 day | |
| T2 | ToolRegistry entry for `issue_discount_code` → two-step: `POST /price_rules` then `POST /price_rules/{id}/discount_codes`. May need small server-side glue (an MCP server or HTTP wrapper) to compose the two calls into one logical tool. | 1-2 days | Could also fold into a tiny "acme-tools" HTTP service if ToolRegistry can't express the composition |
| T3 | ToolRegistry entry for `lookup_customer` → Shopify Admin API `GET /admin/api/2024-01/customers/{id}.json`. Useful for agent to verify customer identity + history. | 0.5 day | |
| T4 | **Stub KB service**: single Go binary exposing `GET /search?q=...` over a seeded markdown corpus. BM25 or simple keyword match; no embeddings needed for demo. Deployed as part of `omnia-demo` bundle. | 1-2 days | |
| T5 | **Stub escalation queue service**: single Go binary exposing `POST /escalations` (append to in-memory list), `GET /escalations` (list for dashboard view), `DELETE /escalations/{id}` (resolve). Deployed as part of `omnia-demo` bundle. | 0.5-1 day | |
| T6 | ToolRegistry entries for T4 (`search_kb`) and T5 (`escalate_to_human`) | 0.5 day | |
| T7 | Support agent PromptPack (**variant A** — the baseline): system prompt, tone instructions, escalation rules, guardrails (no refunds without human on disputes, tone, scope limits), eval definitions (see §6.5). **Structure it to be cleanly variant-forkable** — the operator demo will fork variant B from this and A/B test them. | 1-2 days | The PromptPack is where the demo's agent personality + behavior + KPI declarations live. Avoid hardcoding values that a variant would naturally change (tone, verbosity, escalation threshold); expose them as top-level fields in the prompt template. |
| T8 | **Acme Apparel customer personas** authored as PromptArena `kind: Persona` YAML files (not just narrator descriptions). Sarah Chen, Marcus Webb, and 2-3 additional archetypes (new customer, returning happy customer, angry customer, confused customer). Each persona carries a voice + motivation + typical request pattern. | 1 day | These are the hero demo's on-camera customers AND the operator demo's self-play seeds — a single source of truth. The hero demo narrator just picks one from the set; the operator demo's ArenaJob loops through them continuously. |

**Total: ~5-8 days.** Of that, 3-5 days is Shopify Admin API glue + the PromptPack; the stub services are small.

### 6.5 KPIs via evals + Prometheus (new, small)

**Architecture is already in place.** Evals declared in a PromptPack carry a `metric: { name, type, bounds }` declaration (see `internal/schema/promptpack.schema.json:1170-1196`). The runtime's eval pipeline uses a `MetricRecorder` interface that emits eval result values as Prometheus metrics with the declared names (see `internal/runtime/metrics_integration_test.go:241` for the `ctx.Record(result, metricDef)` pattern in production runtime code). Dashboard types already understand `MetricDef` (see `dashboard/src/lib/data/types.ts:205`).

**So the KPI work is: declare evals, query Prometheus.** Nothing new to build in the middleware layer.

| # | Item | Size | Notes |
|---|---|---|---|
| K1 | **Define `session_outcome` eval** in the support PromptPack. YAML + LLM-as-judge prompt: *"Classify this conversation as resolved / escalated / abandoned / unresolved based on tool calls made, final agent message, customer tone."* Declares `metric: { name: "acme_session_outcome_total", type: "counter" }` with an `outcome` label. Runs on PromptKit `SessionCompletion` trigger. | ~0.5 day | The eval runs at session close via the existing `ee/pkg/evals/worker.go` `SessionCompletion` path. No new Go code. |
| K2 | **Define `customer_sentiment` eval** (optional, nice-to-have for the Scene 3 memory moment narration). LLM-as-judge scoring -1..+1. Declares `metric: { name: "acme_customer_sentiment", type: "gauge", bounds: {min: -1, max: 1} }`. | ~0.5 day | Drop if budget is tight |
| K3 | **KPI dashboard strip component**. Home-page widget that queries Prometheus HTTP API directly: `rate(acme_session_outcome_total{outcome="resolved"}[1h]) / rate(acme_session_outcome_total[1h])` for resolution rate, existing `omnia_*` metrics for cost and handle time (**verify metric names exist before H1** — see §8 item 11). | ~1-2 days | React component. Could alternatively embed a Grafana panel via iframe but native is cleaner |
| K4 | **"What did the agent remember" panel** on session detail view. Reads memory-api for entities accessed during the session. | ~2 days | Not a KPI; still needed. May need a new memory-api query endpoint if "entities accessed in session X" isn't supported |
| K5 | **Escalation queue view page** in dashboard (reads from T5 stub queue, OR derived from `acme_session_outcome_total{outcome="escalated"}` + session list filter) | ~1 day | Can be cut if K3 strip is sufficient showcase |

**Total: ~4-5 days** (down from ~1 week in v0.2).

**What this means for the product story**: The prospect pitch for KPIs becomes —

> *"Every KPI on this dashboard is an eval declared in the agent's PromptPack. Evals can be rule-based, LLM-as-judge, or — via PromptKit's eval hooks — custom scorers in any language. Each eval declares a Prometheus metric name. The runtime emits, Prometheus stores, PromQL queries, Grafana (or our dashboard) renders. You don't configure any of this — just write the eval and the metric appears. Want a new KPI? Write a new eval. That's it."*

That's a Phase-1-differentiating story most competing agent platforms can't tell.

### 6.6 Demo packaging (new, small)

**Deliberate scope note**: This section does **not** define or build a "bundle" abstraction. It builds a Helm chart that wires existing CRDs into a working install. The resulting artifact set (a PromptPack, a set of ToolRegistry entries, an AgentRuntime, a Workspace, a Provider, the eval declarations in the PromptPack, and the Helm values that tie them together) is *itself the input* to whoever eventually writes bundle spec v0.1 — they look at what the demo needed, notice what shape emerges, and formalize that. We don't formalize up-front. See §1 ("A deliberate non-goal").

| # | Item | Size | Notes |
|---|---|---|---|
| D1 | `omnia-demo` Helm chart (sub-chart of `charts/omnia` or standalone). Installs: Provider, PromptPack, AgentRuntime, ToolRegistry entries (Shopify + stub), Workspace, stub KB service, stub escalation queue service, workspace secrets (Shopify API token). | 3-5 days | Depends on W6, T1-T7 |
| D2 | Pre-install Job or post-install README to: clear prior memories for demo customer IDs, verify Shopify connectivity, seed fresh KB corpus. | 1 day | |
| D3 | Short "demo shape" README at `charts/omnia-demo/DEMO_SHAPE.md` describing the CRDs and files that make the demo work. Not a spec — a descriptive artifact for whoever writes bundle spec v0.1 to look at. | 0.5 day | Explicit handoff to the future bundle-spec author |

**Total: ~4.5-6.5 days** (down from ~1-1.5 weeks).

### 6.7 Total gap estimate

- Reliability pre-flight (§6.1): ~1 week
- Shopify setup (§6.2): ~2-3 days
- Chat widget (§6.3): ~6-7 days
- Support tools + stubs (§6.4): ~5-8 days
- KPIs via evals + Prometheus (§6.5): ~4-5 days
- Demo packaging (§6.6): ~4.5-6.5 days

**Total: ~4-6 weeks for one engineer**. Still inside Phase 1's 8-week window with room for slippage.

**Net vs. v0.3 (bundle manifest as a deliverable)**: ~2-3 days less work (B1's "bundle manifest v0.1" budget is gone). The demo's concrete artifact set replaces it as input to a future bundle spec.

**Net vs. v0.2 (Shopify + custom KPI middleware)**: roughly **4-6 days less work** overall, and the KPI story becomes dramatically more compelling (the platform's existing eval → Prometheus architecture is the differentiator, not a custom dashboard route or custom middleware).

**Net vs. v0.1 (fake Acme + custom KPI middleware + bundle manifest)**: ~1 week less work overall, and a much better story on three axes: where the customer chat happens (real Shopify), how KPIs are defined (declarative evals → Prometheus), and what "bundle" means (deliberately undefined — see §1).

---

## 7. Phasing proposal

### Phase H0 — Reliability gate (~1 week)

Do **nothing** on the demo until this passes.

1. R1: pick demo LLM backend (decision)
2. R2: verify/delete LLM populator path
3. R3: doctor `MemoryPersistsAcrossSessions` → 29/29 reliable
4. R4: session-start memory retrieval latency measured + within budget

**Exit criterion**: 10 runs of the doctor smoke suite in a row, 29/29, using the chosen demo LLM.

### Phase H1 — Backdrop + widget (~2 weeks)

Can run mostly in parallel with H2 if there are two engineers.

5. SH1-SH4: Shopify dev store setup + seed data + API token
6. W1-W7: vanilla JS chat widget, built and embedded
7. T1-T3: Shopify Admin API tool registry entries
8. T4-T5: stub KB + escalation services
9. T6-T7: remaining tool registry entries + PromptPack

**Exit criterion**: Sarah can log into the Shopify store, open the widget, type a message, and get a real Shopify-Admin-API-powered response. No KPIs, no memory polish, no demo script — just the mechanics.

### Phase H2 — Demo machinery (~1 week)

10. K1 (and optionally K2): `session_outcome` eval definition in the support PromptPack, declaring its Prometheus metric name
11. K3: KPI strip component in dashboard, querying Prometheus HTTP API
12. K4: "What did the agent remember" panel on session detail
13. K5: Escalation view (if not derived from the session_outcome eval)
14. D1: `omnia-demo` Helm chart packaging everything from H1
15. D2: pre-install / seed job
16. D3: `DEMO_SHAPE.md` descriptive artifact for the future bundle-spec author

**Exit criterion**: All five scenes run start-to-finish in dev. `helm install omnia-demo` on a fresh cluster works. KPI strip shows live values from the `session_outcome` eval after a few demo runs seed the Prometheus counter. No narrator yet — just mechanics.

### Phase H3 — Polish + recording (~0.5-1 week)

14. DSAR cascade visibility polish (optional)
15. Write narration script, rehearse
16. Record demo, capture screenshots for landing page
17. Publish Azure sandbox (roadmap Phase 1 P1)

**Exit criterion**: Recorded demo + live Azure sandbox that partners can click into.

---

## 8. Open questions

1. **Demo LLM backend** (blocks H0). Is the demo's target runtime:
   - (a) A real cloud provider (Claude, OpenAI) with budget caps? — most reliable, costs real money, "looks like the real thing"
   - (b) Ollama with PromptKit#836 fixed upstream first? — free, self-contained, dependency on PromptKit release
   - (c) Ollama with a specific model known not to trigger the null-coercion bug? — needs testing
   - (d) Two configurations — Ollama for self-hosted partner demos, Claude for sales demos?
2. **Shopify customer auth API**. `window.Shopify.customer` was the classic path; Shopify has been pushing Customer Account API as the modern approach. **Verify current 2026 state** before committing to an approach. If classic is gone, widget auth path may need a ~1-2 day rework. *(I'm not confident on the current state — check Shopify docs before H1.)*
3. **Shopify Admin API scopes needed**. Confirmed minimum set: `read_orders`, `read_customers`, `write_discounts`, `read_products`. Verify that `write_discounts` alone suffices for the two-step price-rule+discount-code flow in T2, or whether `write_price_rules` is also needed. *(Shopify scope taxonomy has changed over versions — check current admin API docs.)*
4. **Shopify Admin API rate limits during demo recording**. REST API is 2 req/sec per store for standard dev stores (leaky bucket), GraphQL is more forgiving. Demo makes a handful of calls per session — well under limits — but rehearsal + multiple takes could hit them. Consider using GraphQL for T1-T3 if it matters. *(Rate limits may have changed; verify.)*
5. **Widget hosting**. Where does `chat.js` live? Options: (a) served by Omnia operator's static asset handler, (b) S3 + CloudFront, (c) published to npm and loaded via unpkg/jsdelivr. Recommend (a) for the demo — keeps everything in one Helm install.
6. **Bundle spec v0.1 is not this demo's problem — and deliberately so.** The planning docs sketch `Bundle = Adapters + Tools + Instructions + Workflows + Guardrails + KPIs` but no code defines it, no schema exists, and the roadmap allocates 3-4 weeks of *its own* Phase 1 budget to formalizing it. The open question for the demo is not "CRD vs manifest?" but "once the demo works, do we retcon its shape into a bundle spec, or is the shape of the demo's artifact set obviously-not-a-bundle-yet?" Answer this **after** H3, not before H1.
7. **Stub service language**. Go (consistent with Omnia). A single-file Go binary for both KB and escalation. ~100 lines each.
8. **On-camera third-party dependency**. Shopify is the primary risk. Mitigation: record, don't live-stream; rehearse just before recording; have a local fallback. For a live in-person demo (not recorded), consider whether to demo against a local fallback instead.
9. **Azure sandbox**. Does Phase H3 assume Azure AKS is already provisioned? If not, that's a parallel workstream (~1 week, roadmap Phase 1 line item 3) that needs to start alongside H0. Note: the Shopify dev store is not hosted in Azure — that's a separate concern, the Omnia stack runs on AKS and talks *to* Shopify.
10. **Ownership**. Who builds this? The spec assumes "one engineer" sequential — ~4.5-6.5 weeks. Two engineers in parallel could cut that to ~3-4 weeks.
11. **Existing metric name verification** (before K3 in H2). Before building the KPI strip's PromQL queries, confirm these metrics exist in the runtime's `/metrics` output on a running pod:
    - Session duration histogram (name TBD — possibly `omnia_session_duration_seconds_bucket` or similar). Needed for avg handle time.
    - Tool call counter with tool-name label (name TBD). Needed if deriving escalation rate from `escalate_to_human` invocations rather than from the `session_outcome` eval.
    - Per-turn / per-session cost metric (name TBD). Needed for cost-per-conversation.
    **Effort**: ~30 minutes grepping + poking at a running operator. If any are missing, either add them (~0.5-1 day each) or derive from eval results. Either path stays inside the §6.5 budget.
12. **PromptKit eval hook language-agnosticism**. The pitch in §6.5 ("custom scorers in any language via PromptKit's eval hooks") needs verification — does PromptKit actually expose HTTP/gRPC/subprocess eval hooks with a working non-Go example? If not, downgrade the on-camera pitch to "custom Go scorers + LLM-as-judge + rule-based" (still strong) and file the language-agnostic story as a near-term PromptKit enhancement. **Verify before recording Scene 1's narration.**
13. **Session outcome eval design** — LLM-as-judge vs rule-based? Recommendation: LLM-as-judge. More extensible, better story ("the platform uses the LLM to grade itself"), exercises the full eval pipeline end-to-end. Cost per session is trivial at demo scale. Production deployments can switch to rule-based or sampled LLM-as-judge via the existing `ee/pkg/evals/sampling.go` path.

---

## 9. Out of scope (explicit)

**The hero demo is one of two paired demos.** The other is the operator demo (`docs/local-backlog/operator-demo-proposal.md`), which showcases the production-operations story: Arena A/B testing, progressive rollouts, provider failover, compliance at scale. The hero demo deliberately does not cover that territory — it has its own 10-minute job to do, and the operator demo reuses the hero demo's assets to tell a longer, more differentiated story.

**Showcased in the operator demo, not here**:
- **Arena A/B prompt testing** — PromptArena evaluates variant B against variant A using live self-play traffic; uses the hero demo's `session_outcome` eval as the judgment
- **Progressive rollout** with Istio traffic routing and cohort tracking (#758-#763, already wired)
- **Provider failover** at runtime (#785, already wired)
- **Live self-play traffic generation** via ArenaJob + Persona + self-play scenarios (a standard PromptArena feature) — the background engine that makes the operator demo's dashboards tick in real time
- **Aggregate memory / operator analytics** — topics trending, sentiment shifts, emerging issues
- **Compliance + DSAR at scale** — the same DSAR cascade the hero demo touches briefly, shown at thousands-of-sessions volume
- **Platform scale demonstration** — shown indirectly via sustained self-play load during the operator demo (not via an explicit "load test")

**Genuinely out of scope for both demos (not this round)**:
- Multi-agent coordination
- Build or Sense pillars
- Voice / audio modality (separate demo opportunity)
- Explicit load / stress testing (Arena Fleet Phase 3+ work — `planning/products/omnia/load-testing/implementation-plan.md`)

**Shopify productization (Tiers 2 and 3) is explicitly deferred**:
- **Tier 2** — Shopify private app with OAuth install flow for pilot merchants (~2-3 weeks of work beyond the hero demo)
- **Tier 3** — Shopify App Store listing with Theme App Extension, Billing API, multi-tenant per-merchant provisioning, app review (~3-4 months, a separate product bet)

These are valuable follow-ons but are **not** part of Phase 1. The hero demo's widget is a **demo artifact**, not a productized app. It is hand-installed on one controlled dev store. If the demo lands well and a Shopify pilot is validated, Tier 2 becomes its own spec.

---

## 10. Success criteria (recap)

- [ ] The 10-minute narrative in §3 runs end-to-end without flakes, split-screen, on a fresh kind cluster + Shopify dev store
- [ ] The 10-minute narrative runs end-to-end without flakes on the Azure sandbox + same Shopify dev store
- [ ] Memory persists across sessions and the Scene 3 moment lands 10 runs in a row
- [ ] DSAR cascade (Scene 5) completes visibly within 5 seconds
- [ ] `helm install omnia-demo` on a fresh cluster takes < 5 minutes to reach a ready state
- [ ] The chat widget is installable on any Shopify store with 2 lines of `<script>` tag in a theme
- [ ] A recording of the demo exists and is linkable
- [ ] `charts/omnia-demo/DEMO_SHAPE.md` clearly describes the concrete CRDs and files that make the demo work, usable as input by whoever eventually writes bundle spec v0.1
- [ ] Shopify API rate limits / outages during the demo are mitigated by rehearsal + retakes (recording) or a local fallback (live demo)

---

## 11. Relationship to other planning docs

| Doc | Relationship |
|---|---|
| `planning/strategy/roadmap.md` | This spec realizes Phase 1 "Hero demo" + pulls forward the Week-24 memory success criterion |
| `planning/products/omnia/build-inventory.md` | Implements inventory item #1 ("Hero demo: customer support bundle, 2-3w") — but reframes it: the demo ships without formally defining "bundle". Item #2 ("Bundle spec formalization") is a **separate downstream deliverable** that consumes this demo's `DEMO_SHAPE.md` as its primary input |
| `planning/products/omnia/load-testing/implementation-plan.md` | Explicitly deferred until after this demo ships; load test will run *this demo's scenario* at scale in Phase 3 |
| `docs/local-backlog/agentic-memory-remaining.md` | Reliability pre-flight items R1-R4 are the sub-items from that doc that block this demo |
| `docs/local-backlog/memory-analytics-operator-view-spec.md` | Adjacent — aggregate memory patterns would enrich the demo (Phase 2+) |
| `planning/products/omnia/cloud-marketplace-proposal.md` | Azure sandbox from Phase H3 feeds the marketplace listing. Note: distinct from "Shopify App Store" — that's a separate channel entirely. |
| **(sibling)** `docs/local-backlog/operator-demo-proposal.md` | **Paired spec**, co-equal importance. Showcases the production operations story: Arena A/B testing, progressive rollouts (#758-#763), provider failover (#785), on-premises deployment + internal-systems access, compliance at scale. **Depends on this hero demo's PromptPack, ToolRegistry, Shopify backdrop, `session_outcome` eval, and customer personas as reusable inputs.** Must be planned in parallel, built after. Ships as a paired release. |
| **(future)** `docs/local-backlog/shopify-pilot-proposal.md` | Tier 2 private-app pilot plan, written only if the hero demo lands and a Shopify pilot is validated. Should not be started until H3 is complete. |

---

## 12. Next step

Review this spec, mark open questions that need decisions, and if the direction is right: write the implementation plan for Phase H0 (reliability pre-flight) since it's the gate on everything else. In parallel, kick off SH1 (create the Shopify dev store) since it's 30 minutes of clicking, has zero dependencies, and lets you start seeding data immediately.
