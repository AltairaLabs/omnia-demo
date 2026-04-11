# Omnia Operator Demo — Proposal

**Status**: Draft v0.2 (Azure cloud-private deployment target)
**Created**: 2026-04-10
**Updated**: 2026-04-10 — switched Act 5 framing from "on-prem Ollama only" to "four-mode deployment spectrum" with Mode 2 (cloud-private, Azure AI Foundry + Private Endpoint) as the primary target
**Owner**: TBD
**Local-only**: This doc lives in `docs/local-backlog/` and is not committed.
**Sibling spec**: `docs/local-backlog/hero-demo-proposal.md` — must be read first. This demo depends on the hero demo's PromptPack, ToolRegistry, personas, and `session_outcome` eval as inputs.

---

## 1. TL;DR

Build a ~17-minute, pre-recorded, operator-facing demo of **what it's actually like to run an Omnia-powered agent fleet in production**: live measurement, Arena A/B experimentation, progressive rollouts with Istio cohort tracking, provider failover, **on-premises deployment with direct access to internal systems**, and compliance at scale.

**This is the differentiation demo.** The hero demo answers "does Omnia do what a modern support bot does?" (table stakes in 2026). The operator demo answers **"why Omnia instead of Gorgias / Intercom / Ada / Tidio / Chatdesk?"** Every SaaS chatbot vendor can match the hero demo inside of 6 months; none of them can match this one.

**The single biggest differentiator — architecturally impossible for SaaS vendors to match — is Omnia's deployment spectrum.** Four modes, one platform: (1) cloud-standard, (2) cloud-private with the LLM reachable via private networking inside the customer's cloud tenant, (3) hybrid on-prem, (4) full air-gap with an on-prem LLM. Every SaaS vendor lives outside the customer's network; Omnia lives inside it. For enterprise buyers in regulated industries — financial services, healthcare, government, defense, data-sovereign — this is not a "nice to have." It's often the *only* way they can buy at all.

**The demo itself runs on Mode 2** — Azure AKS hosting Omnia, Azure AI Foundry hosting the LLM, traffic flowing over a VNet Private Endpoint. No customer data leaves the Azure tenant. Act 5 shows this as the climax with a visceral "curl from outside fails, curl from inside succeeds" beat, plus a ~20-second sub-beat swapping the Provider CRD to in-cluster Ollama to cover the full air-gap variant.

**The demo is live, not faked.** A background ArenaJob runs PromptArena **self-play scenarios** continuously against the Acme Apparel support bundle — 20-50 synthetic conversations per minute, driven by personas seeded from the hero demo. Every KPI, every Grafana panel, every eval result, every rollout cohort metric you see is *actually updating in real time* because real traffic is actually flowing. The narrator's job is to direct attention, not to fake motion.

**Pre-recorded** so that act transitions, time-compression of the rollout analysis window, and the provider failover drill can be cleanly stitched.

Most of what this demo showcases already exists in the codebase — rollouts (#758-#763), provider health checks (#785), Arena + PromptArena self-play, DSAR cascade, retention tiers. The gap is **assembly** (scenarios + variant B PromptPack + on-prem isolation setup + narrative scripting), not new platform code.

---

## 2. Strategic context

### 2.1 The hero demo is necessary but insufficient

As of 2026, every major Shopify/ecommerce chatbot vendor has or is actively shipping: cross-session memory, LLM-based customer intent understanding, tool calls to backend APIs, live chat via WebSocket, resolution rate / CSAT dashboards, escalation routing, GDPR-compliant deletion. Intercom AI, Gorgias Automate, Ada, Tidio AI, Chatdesk, Kustomer IVA — they all demo versions of the hero demo's narrative. **The hero demo gets Omnia into the consideration set. It does not close the sale.**

### 2.2 What closes the sale

Two categories of differentiation, both invisible in the hero demo:

**(A) Production operations that actually work at scale.** Every chatbot vendor has "we do analytics"; few have "prompt promotion is a Kubernetes rollout with Istio cohort tracking and auto-rollback." Every vendor has "you can run experiments"; few have "experiments run against 50k real historical sessions via self-play replay, with the same eval framework that powers live KPIs." Every vendor has "provider flexibility"; few have "Claude outage, automatic failover to GPT-4, zero sessions dropped."

**(B) The deployment spectrum.** Omnia runs as a Kubernetes workload inside the customer's network boundary — **on any of four topologies**:

1. **Cloud-standard**: Customer's AKS/EKS/GKE + cloud LLM API over public TLS. Fastest to deploy. Still better than SaaS chatbots because tools reach the customer's private systems.
2. **Cloud-private**: Customer's AKS/EKS/GKE + cloud LLM via Private Endpoint — Azure AI Foundry Private Endpoint, AWS Bedrock PrivateLink, or GCP Vertex AI Private Service Connect. Data stays in the customer's cloud tenant. Contractual no-training guarantee. Private networking end-to-end. **This is the enterprise sweet spot** — financial services, healthcare, regulated industries already in AWS/Azure/GCP. It's also what the operator demo actually runs on.
3. **Hybrid on-prem**: Customer's on-prem K8s + cloud-private LLM reached via cross-premise private peering. Defense contractors, hybrid cloud, partial air-gap.
4. **Full air-gap**: Customer's on-prem K8s + self-hosted LLM (Ollama/vLLM/Llama/Mistral/Phi) in-cluster. Zero external egress. Classified environments, fully disconnected networks, maximum sovereignty.

**SaaS chatbot vendors can't offer even mode 1**, because their architecture inverts the assumption — they live in their cloud, they call out to the customer, they need the customer to poke holes. They can't run *inside* the customer's network without rebuilding as a customer-deployed product, which would mean abandoning their business model. Omnia, because its primary artifact is a Helm chart, supports all four modes from the same codebase, the same CRDs, the same PromptPack. The only things that change between modes are the Provider CRD configuration and the cluster topology.

**This unlocks buyer segments SaaS vendors literally cannot sell to**: financial services with data that cannot leave the bank's network, healthcare with PHI, government/defense with classified or FOUO systems, any regulated industry with data sovereignty requirements, and — critically — the much larger "we run in AWS/Azure and we need privacy and compliance but we're not air-gapped" segment, which is most mid-market and enterprise buyers in 2026.

### 2.3 What this demo is NOT

It is **not** a load test. Arena Fleet load testing is Phase 3+ work (`planning/products/omnia/load-testing/implementation-plan.md`). The self-play ArenaJob here generates *demo-scale* traffic (20-50 sessions/min) — enough to make dashboards tick, not enough to stress the platform.

It is **not** a hero demo extension. It is a paired sibling spec, recorded separately, landed on the landing page as a companion. Prospects may watch one, both, or neither.

It is **not** about "new features." Almost everything it showcases already exists. This is **choreography of existing capabilities into a coherent story**, which is itself the work — and the reason this capability surface has been undervalued up to now.

---

## 3. The narrative (~17 minutes)

**Setting**: Same fictional customer as the hero demo — **Acme Apparel**. Same support PromptPack (variant A). Same ToolRegistry. Same personas. The difference is the camera: the hero demo is from the *customer's* POV (Sarah on the storefront). The operator demo is from the *operator's* POV (the SRE/platform team watching and managing the fleet).

**Format**: Mostly single-pane on the operator's screen, with occasional insets showing the customer-side chat to remind the viewer what the agents are doing. No split-screen as the primary format — the story here is the *platform side*, not the *customer side*.

**Running engine**: A background ArenaJob has been running for ~15 minutes before the demo "starts." It runs PromptArena self-play scenarios: one provider role is the Acme Apparel agent, another role is an LLM playing a customer using one of the seeded personas (Sarah Chen, Marcus Webb, new customer, angry customer, confused customer, happy returning customer). Each self-play run is a full conversation — the customer-LLM opens with a realistic support request, the agent responds with tool calls and answers, the conversation closes when the customer-LLM produces a natural goodbye or an escalation trigger fires. Session-api records every session. Eval worker runs `session_outcome` + `customer_sentiment` on every closed session. Prometheus sees all of it.

**Persona on camera**: Omnia platform lead / SRE / head of CX at Acme Apparel.

### Scene 0 — Recap + setup (30 sec)

Narrator: *"You saw the first demo — Sarah Chen talking to an agent on an Acme Apparel Shopify store, the agent resolving her order problem and remembering her across sessions. That's the product. This demo is about the platform — what running that agent in production actually looks like. A couple hundred conversations are happening right now, and we're going to look at what the operator does with them."*

Screen: Operator's Grafana dashboard. Multiple panels visibly updating. Session count ticker incrementing.

### Act 1 — Live measurement (~2.5 min)

Screen: Operator's Grafana dashboard. Multiple panels actively updating from live self-play traffic:

- **Resolution rate** rolling 5-min average — ticking around 73%, visibly moving ±1% as new sessions close
- **Cost per conversation** histogram reshaping as new sessions close
- **Eval score distributions** — `customer_sentiment` gauge, LLM-judge politeness score
- **Escalation event ticker** — little red dots dropping onto a timeline every few seconds
- **Active sessions** gauge (~30)

Narrator: *"These aren't vanity metrics. Every number you see is an eval declared in the bundle's PromptPack — `session_outcome`, `customer_sentiment`, plus a handful of others. Each eval declares a Prometheus metric; the runtime emits; PromQL queries; Grafana renders. Want a new KPI? Write a new eval. You don't configure any of this."*

Narrator spots a dip in the resolution rate ~90 seconds back. Clicks the dip. Grafana drills into the affected time window. Narrator clicks through to Omnia's dashboard, filters sessions by `session_outcome = unresolved` in that window. Three sessions. Opens one.

Screen: Full session trace. Tool calls in order, memory accesses, eval verdicts, LLM provider used, cost per turn, full transcript.

Narrator: *"This is the thing every chatbot vendor promises and most don't deliver. From 'resolution rate dipped' to 'here's the exact conversation that failed, the exact tool call that returned wrong, the exact token cost' in three clicks. Because every session is recorded at the span level, not just as a transcript — and the same eval framework that aggregates KPIs also annotates individual sessions."*

**What this act proves**: the platform's measurement story is real, live, drillable, and powered by a single coherent abstraction (evals + Prometheus + session recording) rather than stitched-together analytics.

### Act 2 — Arena A/B experimentation (~4 min)

Narrator: *"The current prompt is fine. Let me try to make it better."*

Screen: Operator opens PromptArena (Omnia dashboard's Arena view).

Narrator shows **variant A** — the current support PromptPack. Then pastes in **variant B** — a revised prompt: more concise, less apologetic, more directive.

Creates an ArenaJob:
- Scenario: `acme-apparel-support.self-play.yaml`
- Personas: all 5 seeded Acme personas
- Providers: the same LLM provider used in production (for fairness)
- Variants: A and B, 250 self-play runs each
- Evals: `session_outcome`, `customer_sentiment`, `politeness_score`, `factual_correctness`, `cost_per_turn`

Clicks "Run."

ArenaJob controller creates worker Pods. Arena worker fleet spins up. Screen shows the work queue filling, then draining as workers process items. Narrator: *"This is the same arena-worker infrastructure that PromptArena uses for every eval in the platform. Each worker runs one variant against one persona, records the full session, runs the evals."*

ArenaJob completes (in the pre-recorded demo, cut to ~45 sec; realistically 3-5 min of wall clock). Screen shows the comparison view:

| Metric | Variant A | Variant B | Delta |
|---|---|---|---|
| Resolution rate | 71% | 74% | **+3%** ✓ |
| Avg cost/session | $0.08 | $0.06 | **-25%** ✓ |
| Avg handle time | 2m18s | 1m42s | **-26%** ✓ |
| LLM-judge politeness | 0.89 | 0.72 | **-19%** ✗ |
| Escalation rate | 13% | 16% | **+3%** ✗ |
| Factual correctness | 0.94 | 0.93 | flat |

Narrator: *"Variant B resolves faster and cheaper, but it's blunter and escalates more. Is that the tradeoff we want? The platform won't answer that for us — but it will show us exactly what we're choosing between. Let me promote it carefully."*

**What this act proves**: before-and-after evaluation happens inside the platform, against synthetic traffic that exercises the real production code path, using the same eval framework that powers live KPIs — not separate tooling, not a different measurement model, no correlation problems between "what we A/B tested" and "what runs in prod."

### Act 3 — Progressive rollout, live-ish (~3.5 min)

Narrator: *"I want variant B to handle 5% of live traffic for the next 10 minutes. If resolution rate doesn't drop more than 2 points, promote it. If it does, roll back automatically."*

Screen: Operator creates a rollout resource referencing variant B and the promotion rules. Under the hood: Istio DestinationRule updated (#760, #761), cohort tracking enabled (#762), analysis step executor watches the metrics (#763).

Narrator: *"All of this is already wired. Istio, rollout cohort isolation, auto-promotion logic — shipped in PRs 758 through 763."*

Screen: dashboard goes into **split mode** — the KPI strip is now rendering *two cohorts*, variant A and variant B, side by side. Both are populated by the same live self-play traffic; Istio routes 95% to A and 5% to B.

Narrator: *"Watch the numbers."*

**On-camera time compression**: for the recording, the 10-minute analysis window is either (a) pre-recorded from an earlier rehearsal run and replayed in ~30-45 sec of screen time with a "~10 minutes later" overlay, or (b) shown in real time if the recording session has the budget. Either way, the viewer sees variant B's cohort metrics populating alongside variant A's, diverging a bit, then stabilizing.

At the analysis window end, the analysis step executor evaluates: *"Variant B cohort resolution rate 74.2% vs baseline 73.1% — within tolerance; politeness down 19% — documented regression; escalation up slightly — within 5% tolerance."* Decision: **promote**. Istio DestinationRule flips to 100% variant B.

Narrator: *"That's a production prompt change. With evidence from live traffic. With automatic rollback if anything had broken. In 10 minutes of real time, compressed for you. Not a deploy, not a PR review, not a planning meeting."*

**What this act proves**: the Istio + rollout cohort infrastructure in the codebase (#758-#763) is a real competitive differentiator, it actually works end-to-end, and the developer experience for the operator is "write a rollout spec" — not "orchestrate a custom deployment pipeline."

### Act 4 — Provider failover (~1.5 min)

Narrator: *"One more thing on the runtime side."*

Screen: operator opens the provider health dashboard (#785). Shows two providers active: Claude (primary) and OpenAI (secondary).

Narrator: *"I'm going to simulate a Claude outage."*

Operator kills the Claude provider — either by `kubectl delete pod` on a Claude simulator (if the demo env has one), by flipping a provider health flag to unhealthy, or by network-partitioning the Claude endpoint. The health check (#785) immediately reports Claude unhealthy. The agent's runtime resolver picks OpenAI for the next session.

Screen: live KPI ticker on the dashboard continues uninterrupted. Self-play sessions keep closing. Sentiment and resolution rate steady. The only visible change is the provider attribution on new sessions flipping from "claude-sonnet" to "gpt-4".

Narrator: *"Zero sessions dropped. Not a single one. The provider failover happens at the runtime layer, not the agent layer — the agent doesn't know or care that the LLM changed. Two things this unlocks: you're never locked into a provider, and you never have a single point of failure in your inference path. Try that on a SaaS chatbot."*

**What this act proves**: LLM provider lock-in is not a thing on Omnia. Operators can mix providers, failover in real time, and make buying decisions (Claude vs OpenAI vs Bedrock vs on-prem) without rearchitecting.

### Act 5 — Network architecture + deployment spectrum (the climax, ~3 min)

Narrator: *"Now the thing we couldn't show you in the first demo. This is the one nobody else can match."*

Screen: **deployment spectrum diagram** (pre-rendered, high-quality) — four modes, same platform:

```
  ┌──────────────────────────── Omnia Deployment Spectrum ──────────────────────────┐
  │                                                                                    │
  │  Mode 1: Cloud-standard     Mode 2: Cloud-private       Mode 3: Hybrid on-prem     │
  │  ─────────────────────     ──────────────────────       ─────────────────────      │
  │                                                                                     │
  │  Your AKS/EKS/GKE          Your AKS/EKS/GKE            Your on-prem K8s            │
  │  + cloud LLM API           + cloud LLM via             + cloud LLM via              │
  │    (public TLS)              Private Endpoint            private peering            │
  │                              (Azure AI Foundry /                                    │
  │                              Bedrock / Vertex)                                      │
  │                                                                                     │
  │  SMB / mid-market          Financial services,         Defense contractors,         │
  │  fastest deployment        healthcare, regulated        hybrid cloud,                │
  │                            industries (the              partial air-gap              │
  │                            enterprise sweet spot)                                    │
  │                                                                                     │
  │                            Mode 4: Full air-gap                                     │
  │                            ─────────────────────                                    │
  │                                                                                     │
  │                            Your on-prem K8s                                         │
  │                            + Ollama / vLLM                                          │
  │                            in-cluster                                               │
  │                                                                                     │
  │                            Classified / maximum                                     │
  │                            sovereignty / fully                                      │
  │                            disconnected                                             │
  │                                                                                     │
  └─────────────────────────────────────────────────────────────────────────────────────┘

  Same Helm chart. Same CRDs. Same PromptPack.
  The only thing that changes is the Provider CRD and the cluster topology.
```

Narrator: *"Four deployment modes, one platform. Omnia is a Kubernetes workload — wherever you can run Kubernetes, you can run Omnia. The only thing that changes between modes is the Provider CRD pointing the agent at a different LLM backend, and the cluster's network topology. Same Helm chart installs all four."*

Screen zooms into **Mode 2 (Cloud-private)** — the mode this demo is actually running on.

```
┌─────────────────────────────────────────────────────────────┐
│              Acme Apparel — Azure Tenant                     │
│                                                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │  Warehouse  │  │  Internal   │  │  Mainframe  │          │
│  │  Management │  │     CRM     │  │  Connector  │          │
│  │   System    │  │             │  │             │          │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘          │
│         │                │                │                   │
│         └────────────────┼────────────────┘                   │
│                          │ VNet                                │
│                   ┌──────▼──────┐                              │
│                   │    Omnia    │       ┌─────────────────┐   │
│                   │   on AKS    │──────►│ Azure AI Foundry│   │
│                   │             │       │ via Private      │   │
│                   │             │       │ Endpoint (GPT-4o)│   │
│                   │             │       └─────────────────┘   │
│                   └──────┬──────┘                              │
│                          │                                      │
│                          │ WebSocket (only thing exposed)        │
└──────────────────────────┼──────────────────────────────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │   Public     │
                    │   internet   │
                    │              │
                    │ Chat widget  │
                    └──────────────┘
```

Narrator: *"This is the deployment we're running right now. Acme Apparel's Azure tenant. Omnia on Azure Kubernetes Service. Their internal systems — warehouse management, CRM, mainframe connector — in the same VNet, directly callable. Azure AI Foundry hosting GPT-4o, reached via Private Endpoint inside the VNet. The LLM call never leaves their Azure tenant. Their data never touches the public internet — not even when the LLM processes a customer's message. The only thing exposed to the outside is the chat widget's WebSocket."*

Narrator clicks into a live self-play session. Shows the tool trace:
- `lookup_order` → Shopify Admin API (external call, visibly over the internet — *"this one's a SaaS integration, so it does go outbound"*)
- `search_kb` → internal knowledge base (in-cluster service, private)
- `check_warehouse_inventory` → stub "warehouse management" service in the private network (*"this is the kind of system SaaS chatbots can't reach without you punching holes in your firewall"*)

**The visceral beat**: narrator switches to a terminal.

```bash
# From outside the cluster / outside the VNet (a laptop with no VPN, public internet only)
$ curl https://acme-warehouse.internal/inventory/A-1234
curl: (6) Could not resolve host: acme-warehouse.internal

# From a pod inside the cluster (where Omnia lives)
$ kubectl exec -it omnia-runtime -- curl http://warehouse-mgmt.acme.svc.cluster.local/inventory/A-1234
{"sku":"A-1234","available":42,"warehouse":"DC-West"}
```

Narrator: *"First curl, from outside the VNet, fails. The hostname doesn't even resolve. Second curl, from inside the Omnia cluster, succeeds. That's your compliance boundary doing its job — and it's intact whether you're on Azure, AWS, GCP, or a rack in your own datacenter."*

Screen: comparison chart (pre-rendered):

| | SaaS chatbots (Intercom, Gorgias, Ada, etc.) | Omnia |
|---|---|---|
| Hosted where? | Vendor cloud | **Your** cloud tenant or **your** on-prem |
| Access to internal systems | Webhooks punched through firewall OR customer-operated proxy OR public APIs | Direct network access |
| Data leaves your network boundary | **Always** | Only what you choose (if anything) |
| LLM egress | Vendor's choice, vendor's region | Your choice — public API, cloud-private endpoint, or on-prem LLM |
| Authentication | Federated token juggling | Native workload identity / LDAP / SAML directly |
| Compliance boundary | Shared with vendor | Fully yours |
| Data sovereignty | Vendor's region | Your region, always |
| Air-gapped deployment | Impossible | Mode 4 (full on-prem + on-prem LLM) |
| Tool call latency to internal systems | 100-500ms via public internet | single-digit ms on private network |

**~20-sec sub-beat for Mode 4 (air-gap)**: narrator opens the Provider CRD, shows it pointing at `azure-ai`, then shows a second Provider CRD in the same cluster pointing at an **in-cluster Ollama** instance. *"And for the fully air-gapped case — classified environments, fully disconnected, maximum sovereignty — same platform, same chart, different Provider CRD. Here's the Ollama variant running right now in this same cluster. Zero egress to any LLM provider. We're not going to demo the whole agent path on Ollama today, but the pattern is identical — you change one CRD and the agent's now talking to a model running on hardware you own."*

**What this act proves**:
- Omnia supports four deployment modes from the same artifact — no SaaS vendor can match even mode 1
- Mode 2 (cloud-private) is the realistic enterprise case, and the demo runs on it
- Mode 4 (full air-gap) exists for regulated/classified buyers and is a CRD swap away from mode 2
- The curl-from-outside-fails beat proves the network boundary physically, not just in diagrams
- The comparison chart makes the spectrum tangible for a buyer

### Act 6 — Compliance at scale (~2 min)

Narrator: *"Last thing. Compliance isn't bolted on."*

Screen: aggregate memory view (if `memory-analytics-operator-view-spec.md` is built, or a static placeholder panel if not). Shows: topics customers asked about this week, sentiment trend, top memory entities. Narrator: *"This isn't a support metric. It's a product signal. Your product team can see what customers are struggling with before a single ticket hits the human queue."*

Narrator picks one of the self-play personas (say, the "frustrated returning customer" persona). Clicks "Forget this user." DSAR cascade fires.

Screen shows in real time:
- memory-api delete (batch, scoped to that user's pseudonymized ID)
- session-api delete (cascade across hot/warm/cold tiers, 15 Postgres migrations' worth of schemas)
- Audit log entry
- Warehouse sync trigger (if analytics sync is configured)

Narrator: *"We ran that same DSAR against ~40 sessions just now, including ones in hot cache, warm Postgres, and cold archive. It completed in under 3 seconds. Retention policy, consent grants, PII redaction, audit — these aren't features we bolted on for this demo. They're in 15 production Postgres migrations. They work at 40 sessions and they work at 40 million."*

Brief aside: narrator shows the retention policy controlling the hot→warm→cold tiering in action. Shows a session aged out of hot cache into warm Postgres.

**What this act proves**: compliance is wired end-to-end, tested, shipped. Not a roadmap item.

### Closing (30 sec)

Narrator: *"Everything on screen is the same install, the same Helm chart, the same PromptPack you saw at the end of the first demo. Nobody faked a number. Nobody ran a second environment for this video. The self-play traffic running in the background is optional — you turn it on when you want synthetic load, you turn it off when you don't. The platform is the same whether it's running 20 sessions a minute or 20,000."*

*"SaaS chatbots are fine if you don't have internal systems, don't need to A/B test prompts with rigor, don't care who sees your customer data, and don't mind being locked to whichever LLM your vendor happens to prefer this quarter. If any of those are true, we're not for you. If none of them are, we probably are."*

*"Ship a support bot on Shopify in an afternoon. Run it properly in production forever."*

**Total**: ~17 minutes.

---

## 4. Capability map — narrative to code paths

| Act | What the viewer sees | Code paths exercised |
|---|---|---|
| 0 | Pre-roll, dashboard with live ticker | **NEW**: self-play ArenaJob running in the background. **Existing**: dashboard, KPI strip from hero demo. |
| 1 | Grafana panels actively updating | **Existing**: eval→Prometheus pipeline (hero demo §6.5 + `internal/runtime/metrics_integration_test.go:241`), session recording (`internal/runtime/conversation.go:152`), `/sessions` browser. **NEW**: Grafana dashboard with operator-demo panels (dashboarding work, not platform code). |
| 1 | Drill from dip to failing session | **Existing**: session search + filter by eval result (already in dashboard). |
| 2 | PromptArena A/B testing variant A vs B | **Existing**: Arena (`ee/pkg/arena/`), ArenaJob controller (`ee/internal/controller/arenajob_controller.go`), arena-worker (`ee/cmd/arena-worker/`), PromptArena self-play (standard feature, `ee/cmd/arena-worker/SERVICE.md:143`), `ee/cmd/arena-eval-worker/`. **NEW**: **variant B PromptPack** (authored), scenario YAML referencing Acme personas, A/B comparison view polish in the Arena dashboard page. |
| 3 | Progressive rollout split cohorts | **Existing**: rollout CRD + controller (#758-#763), Istio DestinationRule wiring (#760, #761), cohort tracking (#762), analysis step executor (#763). **NEW**: rollout-aware KPI strip (split mode showing two cohorts side by side) — may be minor dashboard polish or may already exist. |
| 4 | Provider failover on camera | **Existing**: provider health checks (#785), provider resolver in runtime, Ollama + OpenAI providers. **NEW**: the failover *drill* — a rehearsed procedure, not new code. |
| 5 | Deployment spectrum diagram (4 modes) | Pre-rendered asset. Not code. |
| 5 | Mode 2 zoom-in diagram (Azure AKS + Azure AI Foundry + Private Endpoint) | Pre-rendered asset. Not code. |
| 5 | Curl-from-outside-fails beat | **NEW**: Azure VNet + NetworkPolicy setup (Kubernetes-native, Azure-specific for Private Endpoint). Zero Omnia code. |
| 5 | `check_warehouse_inventory` tool | **NEW**: stub "warehouse management" service in the private VNet + ToolRegistry entry. Analogous to the hero demo's KB stub. |
| 5 | Agent calls → Azure AI Foundry via Private Endpoint | **Existing**: Omnia `azure-ai` provider type (`pkg/provider/types.go:50-52`, `api/v1alpha1/provider_types.go:174`). Supports `baseURL` for private endpoints and `workloadIdentity` auth. **NEW**: the Azure-side provisioning (Private Endpoint, DNS zone, workload identity setup) + Provider CRD authoring. |
| 5 | Comparison chart (SaaS vs Omnia) | Pre-rendered asset. Not code. |
| 5 | Mode 4 sub-beat: in-cluster Ollama Provider CRD | **Existing**: Ollama provider runs in-cluster. The operator demo's Ollama instance is the **same** instance used for self-play customer role — a twofer. No new code. |
| 6 | Aggregate memory / trending topics view | **Dependent** on `memory-analytics-operator-view-spec.md` — if that work ships, use it. Otherwise, a static placeholder for v1. |
| 6 | DSAR cascade against 40 self-play sessions | **Existing**: DSAR cascade (`ee/pkg/privacy/deletion.go:222-230`), `memory_deleter.go`, session-api delete cascade, audit log, retention tier handling. No new code — just demo the existing cascade against real self-play data. |
| 6 | Retention tier aging | **Existing**: retention worker, SessionRetentionPolicy CRD, compaction engine. |

---

## 5. What we have (verified, grounded in code)

### 5.1 PromptArena + self-play + ArenaJob

| Capability | Status | Evidence |
|---|---|---|
| PromptArena core | Done | `ee/pkg/arena/` — fetcher, queue, storage, template, providers, binding, fleet, partitioner, threshold, aggregator |
| ArenaJob CRD + controller | Done | `api/v1alpha1/arenajob_types.go`, `ee/internal/controller/arenajob_controller.go`, `ee/internal/controller/arena_config_validation.go` |
| arena-worker binary | Done | `ee/cmd/arena-worker/` — execution, provider groups, load profiles, session recording, integration tests |
| Self-play scenarios | Done | `ee/cmd/arena-worker/SERVICE.md:143` has dedicated Self-Play section. `ee/cmd/arena-worker/provider_groups.go:534-635` — provider remapping for self-play roles + judges. Tested in `ee/cmd/arena-worker/integration_test.go:1187-1260+` (`TestExecuteWorkItemWithSelfPlayRemap`). |
| Personas as first-class schema | Done | `apiVersion: promptkit.altairalabs.ai/v1alpha1, kind: Persona` — `ee/cmd/promptkit-lsp/server/schemas/persona.json` + integration tests using persona YAML |
| Scenarios as first-class schema | Done | `ee/cmd/promptkit-lsp/server/schemas/scenario.json` |
| arena-eval-worker | Done | `ee/cmd/arena-eval-worker/main.go` |
| Arena dashboard management UI | Done | Dashboard has Arena pages (see `planning/products/omnia/build-inventory.md`) |

**Takeaway**: the self-play engine this demo depends on is mature, tested, and standard. We don't need to build it. We need to **author scenario and persona YAML for Acme Apparel** and configure an ArenaJob.

### 5.2 Rollouts (progressive delivery)

| Capability | Status | PR |
|---|---|---|
| Rollout CRD | Done | #758 (Phase 1) |
| Rollout controller | Done | #759 (Phase 2) |
| Istio traffic routing | Done | #760 (Phase 3) |
| Sticky sessions via DestinationRule | Done | #761 (Phase 4) |
| Cohort tracking propagation | Done | #762 (Phase 5) |
| Analysis step executor | Done | #763 (Phase 6) |
| Documentation | Done | PR #764 |

**Takeaway**: the entire rollout infrastructure the operator demo showcases shipped in #758-#764 across March-April 2026. The demo is the first time this will be shown on camera.

### 5.3 Provider health + failover

| Capability | Status | PR |
|---|---|---|
| Provider endpoint health checks | Done | #785 |
| Dev Ollama provider + removal of mock providers | Done | #785 |
| Provider health in dashboard | Done | #785 (presumably — **verify before H5**) |

### 5.4 Compliance & retention

| Capability | Status |
|---|---|
| DSAR deletion cascade (memory + session + audit) | Done — `ee/pkg/privacy/deletion.go:222-230`, `memory_deleter.go` |
| Retention worker | Done — `internal/memory/retention.go` |
| SessionRetentionPolicy CRD | Done |
| Hot / warm / cold tiering | Done — 3-tier storage per build-inventory |
| Session analytics sync (Snowflake/BigQuery/ClickHouse) | Done |
| Session streaming (Kafka/Kinesis/Pulsar/NATS) | Done |
| 15 Postgres migrations covering privacy/audit | Done |

### 5.5 Deployment spectrum (on-premises + cloud-private)

The four-mode deployment spectrum (Mode 1 cloud-standard → Mode 4 full air-gap) isn't a code capability — it's an **architectural property** of Omnia being a K8s workload with a flexible Provider CRD. There's no "on-prem mode" toggle; the spectrum is just "where did you install the Helm chart, and what Provider CRD did you configure."

Evidence for each mode:
- **Mode 1 (cloud-standard)**: Helm chart installs on any conformant K8s (EKS/AKS/GKE). Provider CRDs for public cloud APIs (OpenAI, Claude, Gemini) work today.
- **Mode 2 (cloud-private)**: **Omnia already has `azure-ai`, `bedrock`, and `vertex` provider types as first-class CRDs** — see `pkg/provider/types.go:50-52` (`TypeAzureAI`), and `api/v1alpha1/provider_types.go:158-159` (hyperscaler validation). The `ProviderSpec.BaseURL` field (line 174) supports pointing at a Private Endpoint URL. `RequiresCredentials() == false` for these types (line 86) because they use cloud-native auth (workload identity, IAM roles, etc.) rather than API keys. The operator demo exercises this path with Azure AI Foundry.
- **Mode 3 (hybrid on-prem)**: K8s-native. On-prem K8s can reach cloud-private LLMs via cross-premise private networking (Express Route, Direct Connect, Interconnect).
- **Mode 4 (full air-gap)**: Helm chart installs on on-prem K8s; Ollama provider runs in-cluster with a locally-hosted model. Zero external egress. Already works in dev today.

All HTTP tool executors, MCP executors, and other tool paths respect Kubernetes cluster networking — no hardcoded public endpoints. No code path requires internet egress for core operation.

**For the operator demo, Mode 2 is the primary target** — the richest enterprise buyer segment. Mode 4 is included as a ~20-second sub-beat (CRD swap demonstrates the air-gap case).

### 5.6 What we have in summary

**Essentially everything the operator demo showcases already exists and has shipped.** The operator demo is a *choreography problem*, not an engineering problem. The engineering gaps are small and mostly in the "demo environment setup" bucket, not "build new capability."

---

## 6. What we need (gaps, sized)

### 6.1 Self-play scenario + variant B (new, small)

| # | Item | Size | Notes |
|---|---|---|---|
| O1 | **Acme Apparel self-play scenario YAML** — references the personas from hero demo §6.4 T8, defines the self-play roles (customer-LLM + agent), conversation termination conditions, eval attachments. | 1-1.5 days | Uses the existing PromptArena scenario schema |
| O2 | **Variant B PromptPack** — forked from hero demo variant A. More concise, more directive, less apologetic. Same tools, same evals, different system prompt + style guide. | 0.5-1 day | Must use the same eval declarations as variant A so the A/B comparison is apples-to-apples |
| O3 | **Continuous ArenaJob config** — runs the self-play scenario at ~20-50 sessions/min, indefinitely. Feeds real traffic into the Omnia agent so dashboards tick. | 0.5 day | |

### 6.2 Demo environment: cloud-private AKS + Azure AI Foundry (new, small-medium)

This realizes **Mode 2** (cloud-private) as the primary operator demo target. Mode 4 (full air-gap) is included as a ~20-second Act 5 sub-beat via an in-cluster Ollama provider.

| # | Item | Size | Notes |
|---|---|---|---|
| O4 | **Dedicated Azure AKS cluster** for the operator demo, in a new resource group labeled as "Acme Apparel's customer-private cluster". VNet with private subnets. NetworkPolicy enforcing: internal systems not reachable from outside the VNet, only WebSocket facade exposed via Ingress with public IP. | 1-1.5 days | Azure-specific setup. D5 resolved — dedicated AKS, not shared with hero demo. Covered by user's Azure credits. |
| O5 | **Stub warehouse management service** — tiny Go HTTP service (~50 lines) exposing `GET /inventory/{sku}` with fixture data. Deployed in the private VNet, reachable only from inside the cluster. Analogous to the hero demo's KB stub. | 0.5 day | |
| O6 | **ToolRegistry entry for `check_warehouse_inventory`** — HTTP executor pointing at the in-cluster warehouse service via cluster-internal DNS | 0.25 day | |
| O7 | **"Outside world" demo setup** — a laptop or pod outside the VNet, with no VPN access, used for the curl-fails-from-outside beat | 0.25 day | Could literally be the recording laptop; or a pod in a separate Azure resource group / separate cluster with no VNet peering |
| O8a | **Azure AI Foundry provisioning** — create an Azure AI Foundry resource (or Azure OpenAI Service if Foundry is unavailable in the target region), deploy GPT-4o (or current flagship), verify the model is accessible from the AKS cluster via public endpoint first (sanity check) | 0.5 day | Needs a working Azure subscription. User has monthly credits. |
| O8b | **Private Endpoint for Azure AI Foundry** — set up VNet Private Endpoint + private DNS zone, lock public network access off on the AI Foundry resource, verify the AKS cluster can still reach Azure AI Foundry via private networking only. **Depends on V2 verification (build plan)** | 1-1.5 days | This is the core of the cloud-private story. All Azure AKS → Azure AI Foundry traffic must traverse the Private Endpoint, not the public endpoint. |
| O8c | **Omnia Provider CRD for Azure AI Foundry** — `type: azure-ai`, `baseURL: <private endpoint URL>`, `auth.type: workloadIdentity`, platform config for Azure region. **Depends on V3 verification** that the Omnia `azure-ai` provider path works end-to-end with a Private Endpoint `baseURL` | 0.5-1 day | Uses existing Omnia code (`pkg/provider/types.go:50-52` — `TypeAzureAI`). Azure workload identity on AKS must be set up. |
| O8d | **Optional Mode 4 sub-beat: in-cluster Ollama** — deploy Ollama with a small model (mistral-7b or similar) in the same AKS cluster. Create a **second** Provider CRD (`type: ollama`) alongside the primary Azure AI Foundry one. Used for Act 5's ~20-second air-gap sub-beat **and** for the self-play customer role (O1-O3). | 1 day | **Twofer**: also serves as the customer-role LLM for self-play in O1-O3, so effectively "free" once you need Ollama for self-play anyway. Strongly recommended to include. |

### 6.3 Dashboarding polish (new, small)

| # | Item | Size | Notes |
|---|---|---|---|
| O9 | **Grafana dashboard with operator-demo panels** — the panels Act 1 showcases. Live resolution rate, cost histogram, eval score distributions, escalation ticker, active sessions gauge. Queries Prometheus via the hero demo's eval→metric pipeline. | 2-3 days | Dashboarding work, not platform code. Reuses the metrics the hero demo's `session_outcome` eval already emits. |
| O10 | **Rollout split-cohort KPI view** — during Act 3, the KPI strip needs to show variant A and variant B side by side, partitioned by the cohort label Istio attaches via #762. | 1-2 days | May already exist if the rollout dashboard work shipped alongside #758-#763 — **verify before H5**. If not, build it. |
| O11 | **Provider health panel** — status lights for each configured provider, failover event log. | 1 day | May already exist from #785 — **verify**. |
| O12 | **Aggregate memory view** (optional) — trending topics, sentiment, top entities. Reads memory-api. | 2-3 days OR static placeholder (~0.5 day) | Conditional on `memory-analytics-operator-view-spec.md` status; if not shipping soon, use a static screenshot or deferred. |

### 6.4 Narrative + recording (new, small)

| # | Item | Size | Notes |
|---|---|---|---|
| O13 | Architecture diagram asset (SVG or high-resolution PNG) | 0.5 day | |
| O14 | Comparison chart asset (SaaS vs Omnia) | 0.5 day | |
| O15 | Narration script, act-by-act | 1 day | |
| O16 | Rehearsal — especially Act 3's rollout timing and Act 4's failover drill | 1-2 days | |
| O17 | Recording + editing (screen capture, splicing, time-compression of Act 3's analysis window, overlays) | 1-2 days | |

### 6.5 Total gap estimate

- Self-play + variant B (§6.1): ~2-3 days
- Demo environment — cloud-private AKS + Azure AI Foundry (§6.2): ~5-7 days (includes Azure AI Foundry provisioning + Private Endpoint + optional in-cluster Ollama for Mode 4 sub-beat + self-play customer role)
- Dashboarding (§6.3): ~3-5 days (assuming aggregate memory view is deferred)
- Narrative + recording (§6.4): ~3-5 days

**Total: ~13-20 engineer-days + ~3-5 days narration/editing = ~3-5 weeks for one person.**

Net increase vs. v0.1: ~2-3 days on §6.2 for the Azure-specific private networking setup, largely offset by Ollama becoming a twofer (self-play customer role + Mode 4 sub-beat in one artifact) rather than two separate pieces of work.

Can be cut to ~2.5-3 weeks with parallel work (one person on §6.1+6.2, another on §6.3, narration after both converge).

---

## 7. Phasing proposal

Picks up where the hero demo's H3 ends.

### Phase H4 — Self-play + variant B + environment (~1.5 weeks)

1. O1: Acme Apparel self-play scenario YAML
2. O2: Variant B PromptPack
3. O3: Continuous ArenaJob
4. O4: Private-network demo namespace with NetworkPolicy
5. O5: Stub warehouse service
6. O6: ToolRegistry entry for `check_warehouse_inventory`
7. O7: "Outside world" demo pod
8. (Optional) O8: On-prem Ollama instance

**Exit criterion**: A continuously-running self-play ArenaJob generates 20-50 sessions/min against variant A. Variant B is authored and can be triggered for a one-shot A/B run. The private namespace enforces NetworkPolicy. The `check_warehouse_inventory` tool works from inside the cluster and fails from outside. Optionally, the agent runs on on-prem Ollama with zero external LLM egress.

### Phase H5 — Dashboarding + rehearsal (~1-1.5 weeks)

9. O9: Grafana operator-demo dashboard
10. O10: Rollout split-cohort view (build or verify existing)
11. O11: Provider health panel (build or verify existing)
12. O12: Aggregate memory view OR static placeholder
13. O13, O14: Architecture + comparison chart assets
14. O16: Rollout drill rehearsal, failover drill rehearsal

**Exit criterion**: All six acts can be demoed manually by the operator end-to-end, in real time, with the Grafana dashboards visibly updating. Rollout compression (Act 3) and failover (Act 4) have been rehearsed at least twice.

### Phase H6 — Recording + edit (~0.5-1 week)

15. O15: Finalize narration script
16. O17: Record all acts
17. Edit: time-compress Act 3, splice acts, add overlays, add architecture diagram animations
18. Review + retake as needed
19. Publish alongside the hero demo recording

**Exit criterion**: Final recorded operator demo published to the landing page alongside the hero demo. Both demos linkable.

---

## 8. Open questions

1. **Rollout time-compression in Act 3 — real-time or edited?** Three options:
   - (a) Record the full 10-minute analysis window in real time; viewer sees real drift. Most honest. Makes the video longer.
   - (b) Pre-record a real rollout from a rehearsal run, replay in compressed time with "~10 minutes later" overlay. Dishonest-looking unless clearly labeled. Fastest on camera.
   - (c) Configure the analysis window to be shorter (e.g., 1 minute) *for the demo only*, but label it truthfully as "we compressed the analysis window to 1 minute for this demo; in production you'd use 10+ minutes." Most defensible.
   - **Recommendation**: (c). It's honest about what was done, the platform supports variable analysis windows, and it keeps the narrative snappy.

2. **On-prem Ollama (Mode 4 sub-beat) — ship or drop?** *Previously asked as "mandatory?" — reframed after the Mode 2 (cloud-private) decision.* Mode 2 is now the primary Act 5 target (Azure AI Foundry + Private Endpoint). Mode 4 becomes a ~20-second sub-beat showing a CRD swap to in-cluster Ollama. Since in-cluster Ollama is **already needed for the self-play customer role** (O8d serves both purposes), it's essentially free to include the Mode 4 sub-beat. **Recommendation: include.** Shows the full spectrum without dominating Act 5.

2b. **Claude-on-Azure-AI-Foundry availability (verification V1)**. My current assumption is Claude is **not** available on Azure AI Foundry as of 2026 (Microsoft's frontier-model story is OpenAI; Claude is available on AWS Bedrock and GCP Vertex AI but not on Azure). **Verify before H4.** If Claude *is* available, switch the primary Provider CRD to Claude Sonnet (free upgrade — dogfoods Claude while keeping the full cloud-private story intact). If not (expected), stay with GPT-4o on Azure AI Foundry — which is itself a strong, well-tested choice and keeps the full private deployment narrative.

2c. **Omnia `azure-ai` provider + Private Endpoint end-to-end (verification V3)**. Code-level: the provider type exists, `BaseURL` is configurable, `workloadIdentity` auth is supported. What's unverified is whether the runtime Azure SDK client in Omnia actually uses the `BaseURL` correctly when pointed at a Private Endpoint URL (some Azure SDKs have historically ignored or mis-handled custom base URLs when the default resolver has cached a public endpoint). **Verify with a minimal end-to-end test** — provision a real Azure AI Foundry resource with Private Endpoint, configure an Omnia Provider CRD, run a minimal agent session, confirm the traffic flows over the Private Endpoint (Azure Monitor / NSG flow logs). If there's a gap, it's likely a small Omnia runtime fix (Azure SDK endpoint override) rather than a rearchitect.

3. **Memory analytics view (O12) — built or placeholder?** Depends on status of `docs/local-backlog/memory-analytics-operator-view-spec.md`. If that work is underway or near done, use the real thing. If not, a single placeholder screenshot is fine for v1 and the narration can be deferred: *"we'll dive deeper into memory analytics in a separate demo."*

4. **Demo cluster — dedicated or shared with hero demo?** Options:
   - (a) Same Azure AKS sandbox as the hero demo. Simpler. Shared state risks.
   - (b) Dedicated "operator demo" cluster, provisioned from the same Helm chart. More isolated.
   - (c) Kind cluster on the recording laptop. Fully controlled, no cloud dependencies, but visually less credible ("is this running on your laptop?").
   - **Recommendation**: (b). Keeps the two demos independent and makes the on-prem story more credible (it's a dedicated cluster, labeled as if it were the customer's).

5. **Stub warehouse service — is it honest enough, or do we need a real internal system analog?** The warehouse service is a stub. A skeptical viewer could say "that's a stub, not a real internal system." **Mitigation**: narrator explicitly frames it — *"this is a stub for the demo, standing in for whatever internal system you have — warehouse management, internal CRM, mainframe connector, a proprietary pricing engine. The point is that Omnia can reach it because it runs inside your network. The tool call path is the same whether the service is a stub or SAP."*

6. **Variant B's intentional regression** — is "-19% politeness" honest? The prompt change for variant B should be REAL and produce real measurable differences, not rigged. Recommendation: write variant B as a genuine alternative ("more concise" style) and let the evals judge it honestly. If the numbers come out differently than expected (e.g., B is worse on more axes), that's fine — the story is "evaluate tradeoffs with evidence," not "variant B is better."

7. **Self-play LLM provider** — which LLM plays the customer role? Using the same LLM as the agent introduces correlation bias in evals. Using a different LLM (e.g., agent = Claude, customer = GPT-4) is cleaner. Recommendation: **different LLM for customer role**. PromptArena's self-play remapping (`ee/cmd/arena-worker/provider_groups.go`) explicitly supports this.

8. **"Outside world" curl beat — real or faked?** The curl demo at the Act 5 climax should be a real shell running on the operator's laptop, not a text overlay. If the demo environment is a cloud cluster, the "outside" is "the operator's laptop with no VPN." If it's a kind cluster, the "outside" is "a pod in a different namespace with NetworkPolicy blocking the first one." Either works — the point is it's observable and not faked on screen.

9. **Demo LLM budget for ongoing self-play** — continuous self-play costs real LLM tokens. At 20 sessions/min × avg 5 turns × ~1k tokens/turn × 24h = ~7M tokens/day per provider role. At Claude Sonnet pricing, that's significant. Mitigations: (a) use Ollama in-cluster for the customer role (zero cost, also great story), (b) cap the self-play job to "burst on demand" rather than continuous, (c) run self-play only during recording sessions. **Recommendation**: (a) for the customer role, actual target LLM for the agent role. Best of both worlds.

10. **Does this demo need the Shopify dev store at all?** The hero demo uses Shopify as the customer-facing surface. The operator demo is operator-POV — does it still need Shopify in the background, or can it run entirely in-cluster with just the ArenaJob driving traffic? **Recommendation**: Shopify stays, because (a) the `lookup_order` and `issue_discount_code` tools calling Shopify Admin API are a real external-integration showcase that reinforces the on-prem contrast ("here's what talking to a SaaS IS like, vs talking to your internal systems"), and (b) narrative continuity with the hero demo. But the Shopify dev store itself doesn't need to be on screen much — just the agent's tool traces showing mixed external/internal calls.

---

## 9. Out of scope (explicit)

- **Explicit load testing / stress testing** — Phase 3+ Arena Fleet work, separate effort.
- **Multi-agent coordination** — Phase 5+ roadmap.
- **Build or Sense pillars** — separate pillar work.
- **Voice / audio modality** — separate demo.
- **PromptKit runtime comparisons** (LangChain, Vercel AI SDK benchmarks) — Phase 7 of the load-testing plan, not a demo.
- **Live customer-in-the-wild deployment** — this demo is a controlled pre-recording, not a live pilot walk-through.
- **Bundle spec v0.1 formalization** — still deliberately deferred (per hero demo §1).

---

## 10. Success criteria

- [ ] All six acts run end-to-end in the demo environment without hand-intervention beyond what the narrator does on camera
- [ ] Self-play traffic has been running for at least 15 minutes before recording starts, producing a realistic backlog of sessions for the drill-down moments
- [ ] Act 3's rollout decision is a real decision made by `#763`'s analysis step executor, not a hard-coded script
- [ ] Act 4's provider failover is a real provider health event, not a scripted animation
- [ ] Act 5's curl-from-outside-fails is observed on a real shell with real network isolation, not a text overlay
- [ ] Act 5's optional on-prem LLM beat (if included) shows zero egress to public LLM providers during the demo run — verifiable from cluster egress logs
- [ ] Act 6's DSAR cascade operates on real self-play data with visible row counts, not a mocked response
- [ ] Total runtime is 15-20 minutes after editing
- [ ] A recorded version exists and is linkable alongside the hero demo
- [ ] Both demos share visual language (same color palette, fonts, overlay style) for landing-page coherence

---

## 11. Relationship to other planning docs

| Doc | Relationship |
|---|---|
| `docs/local-backlog/hero-demo-proposal.md` | **Paired sibling spec**. This operator demo reuses the hero demo's variant A PromptPack, ToolRegistry entries, Acme Apparel personas, `session_outcome` eval, and Shopify backdrop. Must be built after hero demo H3. |
| `planning/strategy/roadmap.md` | Not explicitly in the roadmap as a standalone line item; this spec proposes it as a **co-equal deliverable** to the hero demo because the hero demo alone is not differentiated in the 2026 market. |
| `planning/products/omnia/build-inventory.md` | Implements inventory item #1 (hero demo) in paired form; arguably extends #2-#7 (differentiation) into a concrete showcase artifact. |
| `planning/products/omnia/load-testing/implementation-plan.md` | **Not this demo.** Arena Fleet load testing is Phase 3+ and intentionally separate. This demo's self-play is demo-scale (~20-50 sessions/min), not load-test scale. |
| `docs/local-backlog/agentic-memory-remaining.md` | Same reliability pre-flight items (R1-R4 from hero demo §6.1) also gate the operator demo's memory-related beats. If hero demo H0 passes, operator demo inherits the verification. |
| `docs/local-backlog/memory-analytics-operator-view-spec.md` | Adjacent — if that work ships in parallel, Act 6 uses the real view; otherwise, a placeholder. |
| `docs/local-backlog/multi-tier-memory-proposal.md` | Not demoed here — multi-tier memory is future work. If it ships before the operator demo records, the aggregate memory view (O12) could lean on it. |
| **(future)** `docs/local-backlog/on-prem-deployment-guide.md` | The operator demo's Act 5 is a sales-facing condensation; a proper technical doc on "how to deploy Omnia on-prem with internal network access" is a natural follow-on. |

---

## 12. Next step

Review this spec, mark open questions that need decisions (particularly items 1, 2, 4 which affect environment setup before H4). Once the hero demo's H0 reliability gate passes and H1 begins, start planning the operator demo's H4 so it can begin as soon as hero H3 completes. If there is a second engineer available, O1-O3 (self-play scenario, variant B PromptPack, ArenaJob config) can begin in parallel with hero H2-H3 since they depend only on the hero demo's variant A PromptPack being authored, not on the full hero demo being recorded.
