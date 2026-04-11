# Demo Kickoff — Start Here

**Purpose**: Day 1 action list for starting the paired hero + operator demo build. Open this first each morning of week 1. By the end of day 2, all pre-flight verifications should be complete and H0 can begin.

**Read order** (10 minutes total):
1. This doc (§1-§3 below)
2. `docs/local-backlog/demo-build-plan.md` §Pre-flight decisions table (already resolved D1, D4, D5, D8)
3. Skim `docs/local-backlog/hero-demo-proposal.md` §1 TL;DR and §3 narrative
4. Skim `docs/local-backlog/operator-demo-proposal.md` §1 TL;DR and §3 Act 5 (the on-prem climax)

**Don't re-read the full specs.** They're reference material. Come back to them when you need detail on a specific item.

---

## 1. Day 1 goal

**End-of-day state**:
- [ ] Know whether Claude is available on Azure AI Foundry in 2026 (V1)
- [ ] Shopify partner dev store created, empty Dawn theme (SH1)
- [ ] Azure AI Foundry Private Endpoint setup procedure documented from current Azure docs (V2)
- [ ] **Critical**: Omnia `azure-ai` provider + Private Endpoint path verified end-to-end with a minimal agent session (V3) — this is the one that de-risks the entire operator demo Act 5

Time budget: ~6-7 hours focused work.

---

## 2. Day 1 schedule

### Morning block (~2.5 hours)

#### V1 — Claude on Azure AI Foundry (30-60 min)

**What**: Verify whether Anthropic Claude models are available on Azure AI Foundry as of 2026.

**How**:
1. Log into Azure Portal → search "Azure AI Foundry" (may still show as "Azure AI Studio" in some regions)
2. Open the model catalog
3. Filter by "Anthropic" or search "claude"
4. Also check `learn.microsoft.com/azure/ai-studio/concepts/models` or equivalent current docs
5. Note: model catalog has multiple deployment paths — "Managed compute" (customer-hosted) and "Serverless API" (Microsoft-hosted). Both count as "available on Azure AI Foundry" for our purposes.

**Outcome**:
- **If Claude is available**: Update `demo-build-plan.md` D1 to "Claude Sonnet on Azure AI Foundry." Free upgrade.
- **If Claude is not available** (expected): D1 stays at GPT-4o. No change to the plan.

**Don't spend more than 1 hour on this.** If the answer isn't clear from the portal + docs in 60 minutes, move on with GPT-4o and check back later.

---

#### SH1 — Create Shopify partner dev store (30 min)

**What**: Create a free Shopify partner development store that will host the Acme Apparel storefront for both demos.

**How**:
1. Go to `partners.shopify.com` → sign up for a free Partner account if you don't have one
2. Create a new development store:
   - Store name: `acme-apparel-omnia-demo` (or similar)
   - Store type: **Development store** (free, unlimited, for testing + partner demos)
   - Purpose: "To test or build apps or themes"
   - Data preference: Starter data is fine; we'll seed real fixtures in SH2
3. Verify Dawn theme is installed (should be default)
4. Note the store URL (`<store>.myshopify.com`) — you'll need it for T1-T3 ToolRegistry entries later
5. Enable Admin API access (Settings → Apps and sales channels → Develop apps) — **don't generate the token yet**; we do that in SH3

**Outcome**: A working dev store at a known URL. No data seeded yet (that's SH2, which happens during H1).

---

#### V2 — Azure AI Foundry Private Endpoint setup (1-2 hours)

**What**: Read current Azure docs on Private Endpoints for Azure AI Foundry. Produce a short procedure doc for the actual setup in V3 and H4.

**How**:
1. Start at `learn.microsoft.com/azure/ai-studio/how-to/configure-private-link` (or current equivalent — Microsoft renames these frequently)
2. Read the end-to-end procedure. Specifically look for:
   - Prereqs (Azure subscription, resource provider registrations, VNet in the target region)
   - Supported regions for Private Link on Azure AI Foundry / Azure OpenAI
   - DNS configuration (private DNS zone for `privatelink.openai.azure.com` or whatever the current zone name is)
   - How to disable public network access on the AI Foundry resource
   - How to test the Private Endpoint from a VM or AKS pod inside the VNet
3. Also check whether Azure OpenAI and Azure AI Foundry have the **same** Private Link story in 2026 or whether they've diverged

**Outcome**: Save a short cheat sheet (as `docs/local-backlog/notes/azure-ai-foundry-private-endpoint-procedure.md` — not committed) with:
- The exact steps to provision a Private Endpoint for an Azure AI Foundry resource
- The DNS zone name(s) needed
- Any region-specific gotchas
- Any "don't do X" warnings you find in the docs

This doc becomes the source of truth for O8b in H4.

---

### Lunch break. Go outside.

---

### Afternoon block (~3-4 hours)

#### V3 — Omnia `azure-ai` provider + Private Endpoint end-to-end (3-4 hours)

**This is the critical verification.** If it reveals a gap in Omnia's Azure SDK client, we fix it before committing the operator demo to this path. Better to find it today than in week 4.

**What**: Provision a minimal Azure AI Foundry resource with Private Endpoint, configure an Omnia Provider CRD pointing at it, and run a minimal agent session. Confirm traffic flows over the Private Endpoint (not the public endpoint).

**Procedure**:

**Step 1 — Provision Azure resources (~1 hour)**

Use Azure CLI or portal. You need:
- A resource group, e.g. `rg-omnia-demo-v3-test`
- A VNet with a subnet (`10.0.0.0/24` subnet is fine)
- An Azure AI Foundry (or Azure OpenAI) resource with a GPT-4o deployment. Start in a region that supports Private Link for this service — US East or West Europe are typically safe.
- A Private Endpoint attaching the AI Foundry resource to the VNet subnet
- A private DNS zone linked to the VNet (name per V2's research)
- Public network access disabled on the AI Foundry resource (critical — this is what proves the Private Endpoint is load-bearing, not a decoration)

Either run the operator AKS cluster you already have in the same VNet (with peering) or spin up a throwaway AKS cluster in the same VNet. The AKS cluster doesn't need to be anything fancy — a single node pool is fine.

**Step 2 — Configure Omnia Provider CRD (~30 min)**

Draft a Provider manifest referencing the Private Endpoint:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: azure-ai-foundry-test
  namespace: <your test namespace>
spec:
  type: azure-ai
  baseURL: https://<your-ai-foundry-resource>.privatelink.openai.azure.com  # or current private DNS name
  platform:
    type: azure
    # add region, any other required fields — check api/v1alpha1/provider_types.go PlatformConfig
  auth:
    type: workloadIdentity
    # add workload identity specifics — check api/v1alpha1/provider_types.go AuthConfig
  # no secretRef — workload identity is the auth path
```

Check `api/v1alpha1/provider_types.go` for the exact `PlatformConfig` and `AuthConfig` fields needed. The CRD validation at lines 158-159 will tell you what's required.

**Step 3 — Deploy Omnia + the Provider + a minimal AgentRuntime (~1 hour)**

You can use the existing `charts/omnia` chart. Set up a minimal:
- Workspace
- Provider (the one above)
- AgentRuntime pointing at the Provider
- A trivial PromptPack ("respond to greetings" — 3 lines of system prompt)
- No ToolRegistry needed for this test

Apply. Wait for AgentRuntime to be ready.

**Step 4 — Run a minimal session (~30 min)**

Open a facade connection to the AgentRuntime (via `kubectl port-forward` + a WebSocket client, or via the dashboard if the deployment has one). Send a single message: "Hello". Expect a response.

If you get a response, the Provider is working. **But you need to confirm it's going over the Private Endpoint**, not the public endpoint (which should be disabled, but verify):

- Check Azure Monitor / Private Endpoint metrics for inbound requests
- OR `kubectl exec` into the Omnia runtime pod and run `nslookup <your-private-dns-name>` — it should resolve to a `10.x.x.x` private IP, not a public IP
- OR check NSG flow logs if configured

**Step 5 — Document the outcome (~15 min)**

Three possible results:

| Outcome | What it means | Action |
|---|---|---|
| ✅ Agent responds, traffic goes over Private Endpoint, confirmed private IP resolution | Omnia `azure-ai` provider + Private Endpoint path works end-to-end | V3 passes. H4 O8b+O8c are straightforward. Nothing else to do. |
| ⚠️ Agent responds, but traffic goes over the **public** endpoint despite DNS override | Azure SDK client in Omnia runtime is ignoring the `baseURL` or caching a public resolver | **Small Omnia fix needed** — likely in the Azure AI provider's SDK client initialization. File as a new item; expected ~0.5-1 day to fix. This is exactly what V3 exists to catch. |
| ❌ Agent doesn't respond at all | Auth error, CRD validation error, or something else | Debug. If it's an auth setup issue (workload identity not configured right on AKS), that's Azure-side and expected. If it's an Omnia runtime error, log a bug. |

Update `docs/local-backlog/demo-build-plan.md` V3 line with the outcome.

---

## 3. End of Day 1 — what to have ready for Day 2

- [ ] `docs/local-backlog/demo-build-plan.md` updated with V1, V2, V3 outcomes
- [ ] `docs/local-backlog/notes/azure-ai-foundry-private-endpoint-procedure.md` written
- [ ] Shopify dev store created, URL noted, Admin API access enabled (but no token yet)
- [ ] Any V3-surfaced Omnia issues filed as GitHub issues
- [ ] D1 locked: either "Claude on Azure AI Foundry" or "GPT-4o on Azure AI Foundry"

**If V3 surfaced an issue**: spend Day 2 morning fixing it in Omnia before starting H0. A 0.5-day Azure SDK fix now saves weeks of debugging later.

**If V3 passed cleanly**: Day 2 begins H0 reliability pre-flight (R1-R4).

---

## 4. H0 reliability pre-flight — starts Day 2 or Day 3

See `docs/local-backlog/demo-build-plan.md` §H0 for the full item list.

**Brief**:
- **R1** (~1 day): With D1 already resolved as GPT-4o Azure AI Foundry, R1 is really "confirm the demo LLM is working from a dev environment and can handle the memory__remember tool call reliably." This should just work with a cloud LLM — PromptKit#836 is an Ollama-specific bug, not a GPT-4o one.
- **R2** (1-2 days): Verify the LLM populator is actually wired + selected at runtime. Check `internal/memory/populator_llm.go`. If it's dead code, delete it from the demo path and rely on rule-based. If it's wired, run a memory extraction against the demo LLM and confirm entities get created.
- **R3** (2-3 days): Run `internal/doctor/checks/memory.go` (`MemoryPersistsAcrossSessions`) 10 times in a row with the chosen demo LLM and confirm 29/29 reliable. Fixes flow from R1/R2.
- **R4** (~1 day): Measure memory retrieval latency at session start. Look for existing instrumentation in `internal/runtime/conversation.go` — the `sdk.WithMemory()` wiring at line 176 is probably traced. Confirm p95 < 300ms against the real Azure AI Foundry backend.

**H0 exit criterion**: 10 consecutive doctor smoke-test runs pass 29/29 on GPT-4o via Azure AI Foundry.

---

## 5. Blockers + escalation

| Blocker | What to do |
|---|---|
| Azure subscription lacks AI Foundry capacity in your region | Either switch region (check V2 for supported regions) or request quota |
| Azure workload identity setup for AKS is fiddly | Known Azure pain point; allow a buffer in V3. Fall back to a Managed Identity-with-secret approach if workload identity times out |
| Omnia `azure-ai` provider has an unexpected gap | File a GitHub issue; likely a small fix. Don't block the whole plan on it — start H0 R1/R2 in parallel with the fix |
| Shopify partner signup needs business info you don't have | Personal account is fine for dev stores; use a personal email if needed |
| V1 is ambiguous (Claude *kinda* on Azure AI Foundry?) | Default to GPT-4o; you can switch later with a Provider CRD change, zero code cost |
| Azure credits deplete faster than expected during testing | Cap the test AI Foundry deployment size; delete V3 test resources at end of day; use spot instances for the test AKS node |

**Who to ask**:
- Omnia-specific questions about the `azure-ai` provider runtime: check `internal/runtime/` and `pkg/provider/` code first; file GitHub issue if stuck
- Azure portal / Private Link questions: Microsoft support (you have credits, you probably have support)
- Strategic / scope questions: re-read `hero-demo-proposal.md` §1-§2 and `operator-demo-proposal.md` §1-§2

---

## 6. What to defer past Day 1

Don't touch these tomorrow:
- Hero demo narrative script (H3.2)
- Operator demo narration (O15)
- Any asset work — diagrams, comparison charts (O13, O14)
- The widget code (W1-W7)
- Variant B PromptPack (O2)
- Bundle spec debate (D3)

Focus Day 1 exclusively on the verification batch + pre-flight checks. Every hour spent on anything else before V3 passes is an hour potentially wasted if V3 reveals a problem.

---

## 7. Links

**Specs (reference, not required reading tomorrow)**:
- `docs/local-backlog/hero-demo-proposal.md` — hero demo (Shopify + widget + tools + memory moment)
- `docs/local-backlog/operator-demo-proposal.md` — operator demo (measurement, Arena, rollouts, failover, on-prem)
- `docs/local-backlog/demo-build-plan.md` — consolidated item list + phase totals

**Related (only if needed)**:
- `docs/local-backlog/agentic-memory-remaining.md` — current memory reliability gaps (context for H0 R2, R3)
- `planning/strategy/roadmap.md` — the strategic context these demos operationalize (the "Knowledge Bundles / Talk-first" narrative)
- `planning/products/omnia/build-inventory.md` — current Omnia capability inventory

**Code entry points**:
- `pkg/provider/types.go:50-52` — `TypeAzureAI` definition
- `api/v1alpha1/provider_types.go:158-179` — Provider CRD including `BaseURL` and `Platform` + `Auth` validation
- `internal/runtime/conversation.go:176` — where memory gets wired to the session
- `internal/doctor/checks/memory.go` — the canary R3 must green

---

## 8. One rule for tomorrow

**If V3 reveals a gap in Omnia's Azure SDK client, fix that before proceeding.** Everything downstream depends on the `azure-ai` provider with Private Endpoint working. It's cheaper to fix a 0.5-day Azure SDK issue now than to discover it in week 4 when the operator demo's Act 5 is blocked.

Go.
