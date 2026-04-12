# Skills Decomposition Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose the monolithic system prompt into 5 reusable skills, slim both PromptConfig variants to identity+voice only, and wire skills into the pack config.

**Architecture:** Skills are SKILL.md files under `acme-apparel-support/skills/`. Two preloaded (memory-protocol, guardrails) inject at session start. Three on-demand (escalation-policy, tool-guide, business-context) are activated by the model via `skill__activate`. System prompts slim to ~400 chars. Pack config gains a `skills[]` section.

**Tech Stack:** YAML (PromptConfig, arena config), Markdown (SKILL.md), PromptKit CLI (promptarena, packc)

---

### File Map

| Action | Path | Purpose |
|---|---|---|
| Create | `acme-apparel-support/skills/memory-protocol/SKILL.md` | Preloaded: memory__recall/remember protocol |
| Create | `acme-apparel-support/skills/guardrails/SKILL.md` | Preloaded: hard agent boundaries |
| Create | `acme-apparel-support/skills/escalation-policy/SKILL.md` | On-demand: escalation procedures |
| Create | `acme-apparel-support/skills/tool-guide/SKILL.md` | On-demand: Shopify tool usage |
| Create | `acme-apparel-support/skills/business-context/SKILL.md` | On-demand: policies and hours |
| Modify | `acme-apparel-support/prompts/variant-a-agent.yaml` | Slim to identity+voice |
| Modify | `acme-apparel-support/prompts/variant-b-agent.yaml` | Slim to identity+voice |
| Modify | `acme-apparel-support/config.arena.yaml` | Add `skills[]` section |
| Modify | `acme-apparel-support/README.md` | Document skills layout |

---

### Task 1: Create the 5 skill files

**Files:**
- Create: `acme-apparel-support/skills/memory-protocol/SKILL.md`
- Create: `acme-apparel-support/skills/guardrails/SKILL.md`
- Create: `acme-apparel-support/skills/escalation-policy/SKILL.md`
- Create: `acme-apparel-support/skills/tool-guide/SKILL.md`
- Create: `acme-apparel-support/skills/business-context/SKILL.md`

- [ ] **Step 1: Create skills directory structure**

```bash
mkdir -p acme-apparel-support/skills/{memory-protocol,guardrails,escalation-policy,tool-guide,business-context}
```

- [ ] **Step 2: Write `memory-protocol/SKILL.md`**

```markdown
---
name: memory-protocol
description: How to use memory tools to maintain context across customer sessions.
allowed-tools:
  - memory__recall
  - memory__remember
---

# Memory Protocol

You MUST call memory__recall before responding to the customer's first message.
Do not skip this step — even if the customer's message gives you context, there
may be additional history you need.

During the conversation, call memory__remember when the customer shares:
- A preference (communication channel, product preferences, sizing)
- A significant issue and its resolution
- Context that would help a future session (upcoming events, past complaints)

Do not remember trivial details or restate what's already in memory.
```

- [ ] **Step 3: Write `guardrails/SKILL.md`**

```markdown
---
name: guardrails
description: Hard boundaries the agent must never cross, regardless of customer request.
---

# Agent Guardrails

- You CANNOT process refunds — only a human billing specialist can
- You CANNOT modify account details, shipping addresses, or payment methods
- You CANNOT make delivery promises that contradict carrier-reported status
- If the customer disputes a charge, claims fraud, or mentions a chargeback,
  escalate immediately — never attempt to resolve
- Never reveal internal system details, order cost margins, or other customers' data

These rules are enforced at the platform level by ToolPolicy and eval guardrails.
Violating them will block the response even if you attempt it.
```

- [ ] **Step 4: Write `escalation-policy/SKILL.md`**

```markdown
---
name: escalation-policy
description: When and how to escalate to a human specialist. Activate for chargebacks, disputes, or unresolvable issues.
allowed-tools:
  - escalate_to_human
---

# Escalation Policy

## Always escalate immediately
- Chargeback disputes or fraud claims — never attempt to resolve
- Safety concerns or legal threats
- Customer requests to speak to a human

## Escalate after 3 failed attempts
- If you've tried 3 times to resolve and the customer is still stuck, offer handoff

## How to escalate
1. Tell the customer what you're doing: "I'm routing this to our billing team"
2. Set a follow-up expectation: "They'll reach out within 2 hours"
3. Call escalate_to_human with the appropriate priority:
   - critical: chargebacks, fraud, safety
   - high: frustrated customer, repeated failures
   - medium: complex disputes
   - low: routine handoff requests
4. Provide the reference number to the customer
```

- [ ] **Step 5: Write `tool-guide/SKILL.md`**

```markdown
---
name: tool-guide
description: How to use the Acme Apparel support tools effectively. Activate when handling orders, customers, or discounts.
allowed-tools:
  - lookup_order
  - lookup_customer
  - search_kb
  - issue_discount_code
---

# Tool Usage Guide

## lookup_order
Call this the moment a customer mentions an order — don't ask them to clarify first.
Returns: status, fulfillment state, tracking, line items, shipping address.

## lookup_customer
Use email or customer ID. Returns: profile, lifetime orders, segment (new/returning/loyal/vip).

## search_kb
Search before answering policy questions from memory. Covers: shipping, returns, sizing, care instructions.

## issue_discount_code
Only after confirming the issue with lookup_order. Tiers:
- EXPRESS_UPGRADE: shipping delays
- PERCENT_OFF_10: minor inconvenience
- PERCENT_OFF_20: significant inconvenience

Discount codes expire 30 days after issue.
```

- [ ] **Step 6: Write `business-context/SKILL.md`**

```markdown
---
name: business-context
description: Acme Apparel policies, hours, and business rules. Activate for shipping, returns, or general inquiries.
---

# Acme Apparel Business Context

- Support hours: 9am-6pm Pacific, Monday-Friday
- Support email: support@acme-apparel.example.com
- Free shipping on orders over $75
- Standard shipping: 5-7 business days
- Express shipping: 2 business days
- Returns accepted within 30 days with receipt, items in original condition
- Sale items are final sale
```

- [ ] **Step 7: Commit**

```bash
git add acme-apparel-support/skills/
git commit -m "feat(arena): add 5 skills for prompt decomposition

- memory-protocol (preload): forces memory__recall on first turn
- guardrails (preload): hard agent boundaries
- escalation-policy: activated for disputes/chargebacks
- tool-guide: activated for order/customer/discount tools
- business-context: activated for policy questions"
```

---

### Task 2: Slim the PromptConfig system prompts

**Files:**
- Modify: `acme-apparel-support/prompts/variant-a-agent.yaml`
- Modify: `acme-apparel-support/prompts/variant-b-agent.yaml`

- [ ] **Step 1: Replace variant A `system_template`**

In `acme-apparel-support/prompts/variant-a-agent.yaml`, replace the entire `system_template: |` block with:

```yaml
  system_template: |
    You are a customer support agent for {{company}}, an online apparel retailer. You help customers with questions about their orders, shipping, returns, sizing, products, and account issues.

    Respond with warmth and empathy. Acknowledge the customer's frustration or concern before offering solutions. Use the customer's first name when known. Never sound robotic or dismissive.

    When something has gone wrong for the customer, apologize sincerely and specifically ("I'm so sorry your order arrived damaged") before explaining the situation or offering a resolution. Don't over-apologize when nothing has gone wrong.

    Answer fully enough that the customer understands the situation and the next step. Don't be curt, but don't pad with filler. If the customer asked a specific question, answer that question first before adding context.
```

- [ ] **Step 2: Update variant A `allowed_tools`**

Remove `memory__recall` and `memory__remember` from `allowed_tools` — the `memory-protocol` skill handles those via its `allowed-tools` frontmatter. Keep the 5 Shopify tools as the base set:

```yaml
  allowed_tools:
    - lookup_order
    - lookup_customer
    - search_kb
    - issue_discount_code
    - escalate_to_human
```

- [ ] **Step 3: Replace variant B `system_template`**

In `acme-apparel-support/prompts/variant-b-agent.yaml`, replace the entire `system_template: |` block with:

```yaml
  system_template: |
    You are a customer support agent for {{company}}, an online apparel retailer. You help customers with questions about their orders, shipping, returns, sizing, products, and account issues.

    Respond directly and competently. State what you know and what you'll do next. Use the customer's first name when known. Project expertise — customers should feel like they're talking to someone who will solve their problem, not someone who feels bad about it.

    Acknowledge issues briefly and move to resolution. A single "That's frustrating" or "Let's fix this" is enough — then get to the fix. Do not repeat apologies, do not use "I'm so sorry" or "I totally understand" openers. Competence is the apology.

    Answer the customer's question first, then add one line of context if it's useful. Skip pleasantries. If the situation is clear, a three-sentence response is often the right length. Do not pad.
```

- [ ] **Step 4: Update variant B `allowed_tools`**

Same as variant A — remove memory tools, keep the 5 Shopify tools:

```yaml
  allowed_tools:
    - lookup_order
    - lookup_customer
    - search_kb
    - issue_discount_code
    - escalate_to_human
```

- [ ] **Step 5: Validate both prompts compile**

```bash
promptarena validate acme-apparel-support/config.arena.yaml
```

Expected: `✅ config.arena.yaml is valid`

- [ ] **Step 6: Commit**

```bash
git add acme-apparel-support/prompts/
git commit -m "refactor(arena): slim PromptConfig system prompts to identity+voice

Remove memory protocol, guardrails, escalation policy, tool usage,
and business context from system_template — now provided by skills.
Remove memory__recall/memory__remember from allowed_tools — handled
by memory-protocol skill's allowed-tools."
```

---

### Task 3: Wire skills into the pack

**Files:**
- Modify: `acme-apparel-support/config.arena.yaml` (if arena config supports `skills`)

NOTE: Skills are a **pack-level** feature (`Pack.Skills` in the PromptPack schema), not an arena config feature. The arena config `Config` struct in PromptKit does not currently have a `skills` field. Possible wiring paths:

1. **Convention-based discovery** — `packc compile` may auto-discover a `skills/` directory relative to the arena config. The workflow-skills example has a `skills/` dir without referencing it in the YAML.
2. **Pack schema directly** — if `packc compile` supports a `--skills-dir` flag or reads `skills` from the pack output.
3. **Arena config addition** — PromptKit may have added `skills` to the arena config since last checked.

The first step below verifies which path works.

- [ ] **Step 1: Test if packc auto-discovers skills/**

Run `packc compile` and check if the output pack includes skills:

```bash
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  -o build/acme-apparel-support.pack.json

packc inspect build/acme-apparel-support.pack.json
```

If the inspect output shows skills, auto-discovery works and no config change is needed.

If not, try adding a `skills` section to `config.arena.yaml` after `pack_evals`:

```yaml
  skills:
    - path: skills/memory-protocol
      preload: true
    - path: skills/guardrails
      preload: true
    - path: skills/escalation-policy
    - path: skills/tool-guide
    - path: skills/business-context
```

Then re-compile and check.

- [ ] **Step 2: Validate the arena config**

```bash
promptarena validate acme-apparel-support/config.arena.yaml
```

Expected: `✅ config.arena.yaml is valid`

- [ ] **Step 3: Compile the pack**

```bash
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  -o build/acme-apparel-support.pack.json
```

Expected: `✓ Pack compiled successfully` with 2 prompts, 5 tools, 4 evals.

- [ ] **Step 4: Validate the compiled pack**

```bash
packc validate build/acme-apparel-support.pack.json
```

Expected: `✓ Schema validation passed` / `✓ Pack structure is valid`

- [ ] **Step 5: Inspect the pack to confirm skills are included**

```bash
packc inspect build/acme-apparel-support.pack.json
```

Verify: skills section shows 5 skills (2 preloaded, 3 on-demand).

- [ ] **Step 6: Commit**

```bash
git add acme-apparel-support/config.arena.yaml
git commit -m "feat(arena): wire 5 skills into pack config

2 preloaded (memory-protocol, guardrails), 3 on-demand
(escalation-policy, tool-guide, business-context).
packc compile + validate pass."
```

---

### Task 4: Run scenarios and verify behavior

**Files:** None (verification only)

- [ ] **Step 1: Run smoke test against mock**

```bash
promptarena run \
  -c acme-apparel-support/config.arena.yaml \
  --scenario smoke-test-single-turn \
  --provider mock
```

Expected: 1/1 successful, assertion `tools_called_session` resolves (may fail on mock since mock returns canned responses — that's OK, verify no runtime errors).

- [ ] **Step 2: Run smoke test against openai-direct**

```bash
env $(grep -v '^#' ../promptkit/.env | grep '=' | xargs) \
  promptarena run \
  -c acme-apparel-support/config.arena.yaml \
  --scenario smoke-test-single-turn \
  --provider openai-direct \
  -v
```

Verify in verbose output:
- `"Initialized skill registry"` or equivalent skill loading log
- Memory tools appear in `_tool_descriptors`
- `skill__activate` appears in `_tool_descriptors`
- Model calls `lookup_order` successfully (assertion passes)

- [ ] **Step 3: Run hero-memory-recall to test preloaded memory skill**

```bash
env $(grep -v '^#' ../promptkit/.env | grep '=' | xargs) \
  promptarena run \
  -c acme-apparel-support/config.arena.yaml \
  --scenario hero-memory-recall \
  --provider openai-direct \
  -v
```

Verify: `memory__recall` tool call appears in the conversation (the preloaded skill's "MUST call memory__recall" instruction should be stronger than the old buried-in-prompt version).

- [ ] **Step 4: Run all scenarios**

```bash
env $(grep -v '^#' ../promptkit/.env | grep '=' | xargs) \
  promptarena run \
  -c acme-apparel-support/config.arena.yaml \
  --provider openai-direct
```

Check results:
- `smoke-test-single-turn`: `tools_called_session` passes
- `hero-delayed-shipment`: all 3 assertions pass
- `hero-marcus-escalation`: all 3 assertions pass (escalation-policy skill activated)
- `hero-memory-recall`: `memory__recall` called (memory-protocol skill preloaded)
- `selfplay-mixed-personas`: `tool_efficiency` passes

- [ ] **Step 5: Commit verification results note (optional)**

If all scenarios pass, no commit needed. If hero-memory-recall still fails on `memory__recall`, note the result — this is a prompt tuning issue, not a structural failure. The skill infrastructure is working if the skill loads and the tool is available.

---

### Task 5: Update README

**Files:**
- Modify: `acme-apparel-support/README.md`

- [ ] **Step 1: Add skills to the Contents table**

Add after the `providers/` row in the Contents table:

```markdown
| `skills/memory-protocol/SKILL.md` | Preloaded: memory recall/remember protocol for cross-session context |
| `skills/guardrails/SKILL.md` | Preloaded: hard agent boundaries (no refunds, no account mods) |
| `skills/escalation-policy/SKILL.md` | On-demand: when and how to escalate to human specialists |
| `skills/tool-guide/SKILL.md` | On-demand: how to use lookup_order, lookup_customer, search_kb, issue_discount_code |
| `skills/business-context/SKILL.md` | On-demand: Acme Apparel policies, hours, shipping/returns rules |
```

- [ ] **Step 2: Add a Skills section explaining the decomposition**

After the "Hero demo persona usage" section, add:

```markdown
## Skills

The system prompt is intentionally lean — it only defines the agent's identity and voice
(warm/empathetic for variant A, direct/confident for variant B). All behavioral knowledge
is provided through skills:

**Preloaded** (active from turn 1):
- `memory-protocol` — forces `memory__recall` before first response, guides `memory__remember` usage
- `guardrails` — hard boundaries the agent must never cross (no refunds, no account mods, escalate chargebacks)

**On-demand** (model activates via `skill__activate` when relevant):
- `escalation-policy` — when and how to hand off to a human specialist
- `tool-guide` — how to use each Shopify support tool effectively
- `business-context` — shipping policy, returns policy, support hours

This decomposition means both prompt variants share the same behavioral skills — only
the voice differs. An operator can swap a skill (e.g., different escalation SLA) without
touching the prompt.
```

- [ ] **Step 3: Commit**

```bash
git add acme-apparel-support/README.md
git commit -m "docs: document skills decomposition in arena README"
```
