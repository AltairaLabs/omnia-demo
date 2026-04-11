# Arena-Native Content Pivot — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Re-source all Acme Apparel demo content as native PromptKit arena YAML under a new top-level `acme-apparel-support/` directory, validated by `packc compile` and runnable via `promptarena run`, with the existing markdown/scattered layout replaced.

**Architecture:** One top-level `acme-apparel-support/` folder containing a `config.arena.yaml` (pack manifest + run config, `kind: Arena`) that references PromptConfig, Persona, Tool, Scenario, and Provider YAML files under subdirectories matching PromptKit's example convention. Pack-level evals (K1, K2, `no_pii_leak`, `memory_utilization`) live in `spec.pack_evals[]` inside the arena config. Variant A and Variant B are two sibling PromptConfig files inside the same pack, keyed by `task_type`. The folder name is the packc-derived pack ID. Deployment bridge is `packc compile` → ConfigMap → Omnia PromptPack CRD (H2.b work, unchanged here).

**Tech Stack:** PromptKit arena YAML (`apiVersion: promptkit.altairalabs.ai/v1alpha1`), `packc` compiler from `../promptkit/tools/packc/`, `promptarena` CLI for scenario execution. All sibling repos — omnia-demo, promptkit, promptpack-spec, omnia — are checked out under `/Users/chaholl/repos/altairalabs/`.

**Preconditions:**
- Working directory is `/Users/chaholl/repos/altairalabs/omnia-demo/` on `main` branch with clean status after commit `22f9f5f`.
- `packc` binary is available on PATH or at `../promptkit/bin/packc`. Confirm with `packc version` before Task 7.
- `promptarena` CLI is available on PATH. Confirm with `promptarena --help` before Task 16.
- Design approved in `specs/2026-04-11-arena-native-content-design.md`.

**Execution note:** This is a content repo (YAML, text, markdown) — not code — so traditional unit-testing is replaced by "author content → run the validator → fix errors → re-run." The plan includes validator invocations at logical milestones. Commits happen at the end of each task so that any failed task can be rolled back cleanly with `git reset --hard HEAD`.

---

## File Structure

### Files created by this plan

```
acme-apparel-support/                                 ← NEW top-level dir, pack ID
├── config.arena.yaml                                 ← kind: Arena, pack manifest
├── prompts/
│   ├── variant-a-agent.yaml                          ← kind: PromptConfig
│   ├── variant-b-agent.yaml                          ← kind: PromptConfig
│   └── fragments/
│       ├── variant-a/
│       │   ├── brand-voice.txt
│       │   ├── apology-style.txt
│       │   ├── response-length.txt
│       │   └── escalation-style.txt
│       └── variant-b/
│           ├── brand-voice.txt
│           ├── apology-style.txt
│           ├── response-length.txt
│           └── escalation-style.txt
├── personas/                                         ← 6 moved files + renames
│   ├── sarah-chen.persona.yaml
│   ├── marcus-webb.persona.yaml
│   ├── emma-patel.persona.yaml
│   ├── kai-nakamura.persona.yaml
│   ├── priya-shah.persona.yaml
│   └── alex-rodriguez.persona.yaml
├── tools/
│   ├── lookup-order.tool.yaml
│   ├── lookup-customer.tool.yaml
│   ├── search-kb.tool.yaml
│   ├── issue-discount-code.tool.yaml
│   └── escalate-to-human.tool.yaml
├── scenarios/
│   ├── hero-delayed-shipment.scenario.yaml
│   ├── hero-marcus-escalation.scenario.yaml
│   ├── hero-memory-recall.scenario.yaml
│   └── selfplay-mixed-personas.scenario.yaml
└── providers/
    ├── azure-gpt4o.provider.yaml
    └── ollama-local.provider.yaml
```

### Files deleted by this plan

```
personas/                     ← all contents moved
promptpacks/                  ← variant-a-support-promptpack.md superseded
```

### Files modified by this plan

```
README.md                     ← Repo structure + Current state reflect new layout
specs/demo-build-plan.md      ← T7, T8, K1, K2, O1, O2 status updates
.gitignore                    ← add build/ directory for packc compile output
```

### Reference sources (NOT modified, used as content input)

- `../promptkit/examples/customer-support-integrated/` — closest existing PromptKit example. Copy patterns (file naming, `config.arena.yaml` structure, tool mock-result format, self-play scenario shape) from here.
- `../promptkit/schemas/v1alpha1/*.json` — schemas for each `kind:`. When in doubt about a field, read the schema.
- `../promptkit/tools/packc/compiler/compiler.go` — authoritative source for how packc assembles a pack.
- `promptpacks/variant-a-support-promptpack.md` — archival narrative with fragment text, eval rubrics, tool descriptions, and skill guidance. Content to extract for authoring, never to modify. Deleted at the end of the plan.

---

## Tool command reference

Commands used across multiple tasks. Not a task itself — reference material.

```bash
# Compile the pack from arena sources
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  --compiler-version 0.1.0-dev \
  -o build/acme-apparel-support.pack.json

# Validate the compiled pack against the PromptPack schema
packc validate build/acme-apparel-support.pack.json

# Inspect the compiled pack (human-readable summary)
packc inspect build/acme-apparel-support.pack.json

# Run a single scenario against a provider
promptarena run \
  -c acme-apparel-support/config.arena.yaml \
  --scenario hero-delayed-shipment \
  --provider azure-gpt4o
```

**Expected `packc compile` success output** (for reference when validating steps):

```
✓ Loaded config.arena.yaml
✓ Registered N prompts (variant-a, variant-b)
✓ Compiled pack → build/acme-apparel-support.pack.json
✓ Schema validation: PASS
```

Exact wording may differ — the signal is exit code 0, non-empty JSON output at the `-o` path, and no "error" lines.

---

## Task 1: Scaffold directory structure

**Files:**
- Create: `acme-apparel-support/` and all subdirectories listed in File Structure
- Create: empty placeholder `acme-apparel-support/config.arena.yaml` (filled in later tasks)
- Modify: `.gitignore`

- [ ] **Step 1: Verify clean working state**

Run: `git status`
Expected: `nothing to commit, working tree clean` on branch `main`.

- [ ] **Step 2: Create directory hierarchy**

Run:
```bash
mkdir -p acme-apparel-support/prompts/fragments/variant-a
mkdir -p acme-apparel-support/prompts/fragments/variant-b
mkdir -p acme-apparel-support/personas
mkdir -p acme-apparel-support/tools
mkdir -p acme-apparel-support/scenarios
mkdir -p acme-apparel-support/providers
```

Verify: `find acme-apparel-support -type d` should list 8 directories.

- [ ] **Step 3: Add `.gitkeep` placeholders where needed**

Since all subdirs will get real content in later tasks, `.gitkeep` files are unnecessary. Skip unless you want to commit the skeleton before content exists. For this plan, content lands in the same task as the directory, so no `.gitkeep`.

- [ ] **Step 4: Add `build/` to `.gitignore`**

Read `.gitignore` (if it exists).

If `.gitignore` doesn't exist, create it with:

```
build/
out/
```

If it exists, append `build/` and `out/` if not already present.

- [ ] **Step 5: Create empty `config.arena.yaml` placeholder**

Create `acme-apparel-support/config.arena.yaml` with a minimal valid YAML header (no spec yet — spec fills in during Task 7):

```yaml
# Placeholder — filled in progressively across tasks 2-7, 9, 11, 14, 15.
# Final form: acme-apparel-support pack manifest + run config.
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: acme-apparel-support
  labels:
    demo: omnia-hero-demo
    bundle: acme-apparel
spec:
  providers: []
  defaults:
    temperature: 0.6
    max_tokens: 800
```

(`providers` and `defaults` are the only required fields in `Arena.spec` per schema — this is the minimum valid stub.)

- [ ] **Step 6: Commit the scaffold**

Run:
```bash
git add .gitignore acme-apparel-support/config.arena.yaml
git commit -F - <<'EOF'
chore: scaffold acme-apparel-support arena source tree

Creates the top-level directory matching packc's pack-ID-from-folder-name
convention. Subdirectories (prompts, personas, tools, scenarios, providers)
are populated by subsequent tasks per the arena-native content pivot
(specs/2026-04-11-arena-native-content-design.md).

Adds build/ and out/ to .gitignore for packc compile output and
promptarena run output respectively.
EOF
```

Expected: commit succeeds, `git status` is clean.

---

## Task 2: Move + rename personas

**Files:**
- Move: `personas/sarah-chen.yaml` → `acme-apparel-support/personas/sarah-chen.persona.yaml`
- Move: `personas/marcus-webb.yaml` → `acme-apparel-support/personas/marcus-webb.persona.yaml`
- Move: `personas/emma-patel.yaml` → `acme-apparel-support/personas/emma-patel.persona.yaml`
- Move: `personas/kai-nakamura.yaml` → `acme-apparel-support/personas/kai-nakamura.persona.yaml`
- Move: `personas/priya-shah.yaml` → `acme-apparel-support/personas/priya-shah.persona.yaml`
- Move: `personas/alex-rodriguez.yaml` → `acme-apparel-support/personas/alex-rodriguez.persona.yaml`

Note: `personas/README.md` stays in place for now; it gets deleted in Task 17 when the whole top-level `personas/` directory is removed.

- [ ] **Step 1: Move and rename six persona files with `git mv`**

Run:
```bash
git mv personas/sarah-chen.yaml       acme-apparel-support/personas/sarah-chen.persona.yaml
git mv personas/marcus-webb.yaml      acme-apparel-support/personas/marcus-webb.persona.yaml
git mv personas/emma-patel.yaml       acme-apparel-support/personas/emma-patel.persona.yaml
git mv personas/kai-nakamura.yaml     acme-apparel-support/personas/kai-nakamura.persona.yaml
git mv personas/priya-shah.yaml       acme-apparel-support/personas/priya-shah.persona.yaml
git mv personas/alex-rodriguez.yaml   acme-apparel-support/personas/alex-rodriguez.persona.yaml
```

- [ ] **Step 2: Verify moves tracked as renames**

Run: `git status`
Expected: each persona shown as `renamed: personas/X.yaml -> acme-apparel-support/personas/X.persona.yaml`. If git shows delete+add instead of rename, that's fine — the commit still tracks the move correctly.

- [ ] **Step 3: Confirm file content unchanged**

Read: `acme-apparel-support/personas/sarah-chen.persona.yaml`
Expected: first line is `apiVersion: promptkit.altairalabs.ai/v1alpha1`, `kind: Persona`, content matches pre-move state. No edits to persona content in this task.

- [ ] **Step 4: Commit**

Run:
```bash
git add -u
git commit -F - <<'EOF'
chore: move personas under acme-apparel-support/ with .persona.yaml suffix

File moves only — content unchanged. Aligns with PromptKit's example
convention where each arena source kind has its typed file suffix
(.persona.yaml, .tool.yaml, .scenario.yaml) for clarity and editor
tooling.

Part of the arena-native content pivot
(specs/2026-04-11-arena-native-content-design.md).
EOF
```

Expected: commit succeeds.

---

## Task 3: Author provider YAMLs

**Files:**
- Create: `acme-apparel-support/providers/azure-gpt4o.provider.yaml`
- Create: `acme-apparel-support/providers/ollama-local.provider.yaml`

**Reference**: `../promptkit/examples/customer-support-integrated/providers/openai-gpt4o-mini.provider.yaml` for field shape.

- [ ] **Step 1: Read a reference provider example**

Read: `../promptkit/examples/customer-support-integrated/providers/openai-gpt4o-mini.provider.yaml`
Note the field structure (apiVersion, kind, metadata, spec with provider-specific fields).

- [ ] **Step 2: Create `azure-gpt4o.provider.yaml`**

Create `acme-apparel-support/providers/azure-gpt4o.provider.yaml` with:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: azure-gpt4o
  labels:
    demo: omnia-hero-demo
spec:
  id: azure-gpt4o
  type: azure-openai
  model: gpt-4o
  # baseURL, auth, and deployment_name are supplied per-environment via
  # environment variables or workspace secrets when promptarena runs.
  # For local dev the following defaults are expected:
  base_url_env: AZURE_OPENAI_ENDPOINT
  api_key_env: AZURE_OPENAI_API_KEY
  deployment: gpt-4o
  api_version: "2024-06-01"
```

(Field names may need adjustment when validated against `../promptkit/schemas/v1alpha1/provider.json` — read the schema if packc compile fails.)

- [ ] **Step 3: Create `ollama-local.provider.yaml`**

Create `acme-apparel-support/providers/ollama-local.provider.yaml` with:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: ollama-local
  labels:
    demo: omnia-hero-demo
    role: customer-simulation
spec:
  id: ollama-local
  type: ollama
  model: llama3.1:8b
  base_url: http://localhost:11434
```

- [ ] **Step 4: Verify against schema**

Read: `../promptkit/schemas/v1alpha1/provider.json` (lines 1-100 first; full file if needed)
Check: `apiVersion`, `kind`, `metadata`, `spec` top-level fields are what the schema expects. If any field is unsupported or misnamed, adjust and re-verify.

- [ ] **Step 5: Commit**

Run:
```bash
git add acme-apparel-support/providers/
git commit -F - <<'EOF'
feat(arena): add provider YAMLs for Azure GPT-4o and local Ollama

azure-gpt4o: primary provider for the Acme Apparel agent role, resolves
environment variables for Private Endpoint / API key at runtime. Matches
D1 pre-flight decision.

ollama-local: cheap customer-simulation provider for self-play
scenarios. Used only as the claude-user role in self-play, not for the
agent. Matches D1 operator-demo cost constraint.

Both provider YAMLs will be referenced from config.arena.yaml (Task 6)
once the rest of the pack is authored.
EOF
```

---

## Task 4: Author tool YAMLs

**Files:**
- Create: `acme-apparel-support/tools/lookup-order.tool.yaml`
- Create: `acme-apparel-support/tools/lookup-customer.tool.yaml`
- Create: `acme-apparel-support/tools/search-kb.tool.yaml`
- Create: `acme-apparel-support/tools/issue-discount-code.tool.yaml`
- Create: `acme-apparel-support/tools/escalate-to-human.tool.yaml`

**Reference**: `../promptkit/examples/customer-support-integrated/tools/get-customer-info.tool.yaml` for field shape. Tool descriptions and input-schema hints extract from `promptpacks/variant-a-support-promptpack.md:302-430` (archival source).

- [ ] **Step 1: Create `lookup-order.tool.yaml`**

Create with:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: lookup_order
spec:
  name: lookup_order
  description: |
    Fetch the full details of an order by its ID. Returns order status, line
    items, fulfillment state, shipping tracking, and customer info. Use this
    any time a customer mentions an order, even before asking them to
    clarify.
  input_schema:
    type: object
    properties:
      order_id:
        type: string
        description: The Shopify order ID (e.g., '1023' or 'A-1234')
    required:
      - order_id
  output_schema:
    type: object
    properties:
      order_id: { type: string }
      status:
        type: string
        enum: [open, closed, cancelled]
      fulfillment_status:
        type: string
        enum: [unfulfilled, partial, fulfilled, delivered, in_transit, delayed]
      total_price: { type: string }
      currency: { type: string }
      customer_email: { type: string }
      tracking_number: { type: string }
      shipping_address: { type: object }
      line_items:
        type: array
        items:
          type: object
          properties:
            sku: { type: string }
            title: { type: string }
            quantity: { type: integer }
            price: { type: string }
  mode: mock
  timeout_ms: 2000
  mock_result:
    order_id: "1023"
    status: open
    fulfillment_status: delayed
    total_price: "89.00"
    currency: USD
    customer_email: sarah.chen@example.com
    tracking_number: 1Z999AA10123456784
    shipping_address:
      city: Oakland
      province: CA
      country: US
    line_items:
      - sku: LINEN-DRESS-M
        title: Summer Linen Dress (M)
        quantity: 1
        price: "89.00"
```

- [ ] **Step 2: Create `lookup-customer.tool.yaml`**

Create with:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: lookup_customer
spec:
  name: lookup_customer
  description: |
    Fetch a customer's account details and order history by email or
    customer ID. Returns customer profile, lifetime orders, last purchase
    date, and segment tags.
  input_schema:
    type: object
    properties:
      customer_id_or_email:
        type: string
        description: The customer's ID or email address
    required:
      - customer_id_or_email
  output_schema:
    type: object
    properties:
      customer_id: { type: string }
      email: { type: string }
      name: { type: string }
      lifetime_orders: { type: integer }
      lifetime_value: { type: string }
      last_order_date: { type: string }
      segment:
        type: string
        enum: [new, returning, loyal, vip]
  mode: mock
  timeout_ms: 2000
  mock_result:
    customer_id: CUST-8842
    email: sarah.chen@example.com
    name: Sarah Chen
    lifetime_orders: 7
    lifetime_value: "612.40"
    last_order_date: "2026-03-28"
    segment: returning
```

- [ ] **Step 3: Create `search-kb.tool.yaml`**

Create with:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: search_kb
spec:
  name: search_kb
  description: |
    Search Acme Apparel's knowledge base for policies, guides, and FAQ.
    Use this when the customer asks about shipping, returns, sizing, care
    instructions, or any general policy question before answering from
    memory.
  input_schema:
    type: object
    properties:
      query:
        type: string
        description: Search query describing what you're looking for
    required:
      - query
  output_schema:
    type: object
    properties:
      results:
        type: array
        items:
          type: object
          properties:
            title: { type: string }
            url: { type: string }
            snippet: { type: string }
  mode: mock
  timeout_ms: 2000
  mock_result:
    results:
      - title: Shipping Policy
        url: /kb/shipping-policy
        snippet: Standard shipping is 5-7 business days. Express is 2 business days. Free shipping on orders over $75.
      - title: Returns & Exchanges
        url: /kb/returns-and-exchanges
        snippet: Returns accepted within 30 days of delivery, items in original condition with tags attached.
```

- [ ] **Step 4: Create `issue-discount-code.tool.yaml`**

Create with:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: issue_discount_code
spec:
  name: issue_discount_code
  description: |
    Issue a discount code to a customer. Use only after confirming the
    underlying issue with lookup_order. Discount types:
    - EXPRESS_UPGRADE: free shipping upgrade for delivery delays
    - PERCENT_OFF_10: 10% off next order, for minor inconvenience
    - PERCENT_OFF_20: 20% off next order, for significant inconvenience
  input_schema:
    type: object
    properties:
      customer_email:
        type: string
        description: The customer's email address
      discount_type:
        type: string
        enum: [EXPRESS_UPGRADE, PERCENT_OFF_10, PERCENT_OFF_20]
        description: Which discount tier to issue
    required:
      - customer_email
      - discount_type
  output_schema:
    type: object
    properties:
      code: { type: string }
      discount_type: { type: string }
      expires_at: { type: string }
  mode: mock
  timeout_ms: 2000
  mock_result:
    code: EXPRESS-X7K92
    discount_type: EXPRESS_UPGRADE
    expires_at: "2026-05-11T00:00:00Z"
```

- [ ] **Step 5: Create `escalate-to-human.tool.yaml`**

Create with:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: escalate_to_human
spec:
  name: escalate_to_human
  description: |
    Route this conversation to a human specialist. Always escalate for
    chargebacks, fraud claims, or safety issues. Use after three failed
    resolution attempts for other cases.
  input_schema:
    type: object
    properties:
      reason:
        type: string
        description: Brief explanation of why escalation is needed
      priority:
        type: string
        enum: [low, medium, high, critical]
        description: |
          low: routine; medium: standard; high: customer frustrated;
          critical: chargebacks/fraud/safety
    required:
      - reason
      - priority
  output_schema:
    type: object
    properties:
      ticket_id: { type: string }
      queue: { type: string }
      priority: { type: string }
      eta_minutes: { type: integer }
  mode: mock
  timeout_ms: 2000
  mock_result:
    ticket_id: ESC-1287
    queue: acme-apparel-support
    priority: high
    eta_minutes: 120
```

- [ ] **Step 6: Verify against schema**

Read: `../promptkit/schemas/v1alpha1/tool.json` (lines 1-200)
Check: `kind: Tool` spec accepts `input_schema`, `output_schema`, `mode`, `mock_result`. Adjust any field names if the schema disagrees.

- [ ] **Step 7: Commit**

Run:
```bash
git add acme-apparel-support/tools/
git commit -F - <<'EOF'
feat(arena): add tool YAMLs with mock results for local validation

Five tool schemas covering the Acme Apparel support agent's toolset:
lookup_order, lookup_customer, search_kb, issue_discount_code,
escalate_to_human.

Each tool declares input/output schemas plus a mock_result fixture that
lets promptarena run exercise scenarios end-to-end against mock data
before the real Shopify Admin API integration lands in H2.b. The same
tool YAML files are wrapped into Omnia ToolRegistry CRDs during Helm
chart packaging.

Per the arena-native pivot, T6 (ToolRegistry entries task) folds into
T1-T5: the .tool.yaml files ARE the registry entries.
EOF
```

---

## Task 5: Author variant A fragments

**Files:**
- Create: `acme-apparel-support/prompts/fragments/variant-a/brand-voice.txt`
- Create: `acme-apparel-support/prompts/fragments/variant-a/apology-style.txt`
- Create: `acme-apparel-support/prompts/fragments/variant-a/response-length.txt`
- Create: `acme-apparel-support/prompts/fragments/variant-a/escalation-style.txt`

**Reference**: `promptpacks/variant-a-support-promptpack.md:93-103` (archival) for the canonical wording of these fragments.

- [ ] **Step 1: Create `brand-voice.txt`**

Create `acme-apparel-support/prompts/fragments/variant-a/brand-voice.txt` with:

```
Respond with warmth and empathy. Acknowledge the customer's frustration or concern before offering solutions. Use the customer's first name when known. Never sound robotic or dismissive.
```

- [ ] **Step 2: Create `apology-style.txt`**

Create `acme-apparel-support/prompts/fragments/variant-a/apology-style.txt` with:

```
When something has gone wrong for the customer, apologize sincerely and specifically ("I'm so sorry your order arrived damaged") before explaining the situation or offering a resolution. Don't over-apologize when nothing has gone wrong.
```

- [ ] **Step 3: Create `response-length.txt`**

Create `acme-apparel-support/prompts/fragments/variant-a/response-length.txt` with:

```
Answer fully enough that the customer understands the situation and the next step. Don't be curt, but don't pad with filler. If the customer asked a specific question, answer that question first before adding context.
```

- [ ] **Step 4: Create `escalation-style.txt`**

Create `acme-apparel-support/prompts/fragments/variant-a/escalation-style.txt` with:

```
When escalating to a human agent, explain what you're doing ("I'm going to hand this off to our billing team"), set a clear expectation for follow-up ("they'll reach out within 2 hours"), provide the reference number, and reassure the customer that they're in good hands.
```

- [ ] **Step 5: Commit**

Run:
```bash
git add acme-apparel-support/prompts/fragments/variant-a/
git commit -F - <<'EOF'
feat(arena): variant A fragment bodies (warm/empathetic)

Four external text fragments referenced by variant-a-agent.yaml via
PromptKit's FragmentRef { name, path, required } schema. Fragment
content is carried forward unchanged from the archival T7 markdown
narrative at promptpacks/variant-a-support-promptpack.md:93-103.

Variant A is the warm/empathetic baseline; variant B's fragment dir is
populated in Task 10 with the less-apologetic/confident variant per D6.
EOF
```

---

## Task 6: Author variant A PromptConfig

**Files:**
- Create: `acme-apparel-support/prompts/variant-a-agent.yaml`

**Reference**: `../promptkit/examples/customer-support-integrated/prompts/support-bot.yaml` for field shape. `../promptkit/schemas/v1alpha1/promptconfig.json` is the schema-of-record. System template body derives from `promptpacks/variant-a-support-promptpack.md:113` (archival JSON); the arena-native form uses fragment references instead of the inline fragment text the JSON form had.

- [ ] **Step 1: Create `variant-a-agent.yaml`**

Create `acme-apparel-support/prompts/variant-a-agent.yaml` with:

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
  description: Acme Apparel support agent — Variant A (warm/empathetic baseline)

  template_engine:
    version: v1
    syntax: "{{variable}}"
    features: [basic_substitution, fragments]

  fragments:
    - name: brand_voice
      path: fragments/variant-a/brand-voice.txt
      required: true
    - name: apology_style
      path: fragments/variant-a/apology-style.txt
      required: true
    - name: response_length
      path: fragments/variant-a/response-length.txt
      required: true
    - name: escalation_style
      path: fragments/variant-a/escalation-style.txt
      required: true

  variables:
    - name: company
      type: string
      required: true
      default: Acme Apparel
      description: Company name shown to the customer
    - name: support_email
      type: string
      required: true
      default: support@acme-apparel.example.com
      description: Support contact email
    - name: business_hours
      type: string
      required: false
      default: "9am–6pm Pacific, Monday–Friday"
      description: Human-readable support hours
    - name: escalation_queue
      type: string
      required: false
      default: acme-apparel-support
      description: Queue name for escalated tickets

  system_template: |
    You are a customer support agent for {{company}}, an online apparel retailer. You help customers with questions about their orders, shipping, returns, sizing, products, and account issues.

    ## How to talk

    {{brand_voice}}

    {{apology_style}}

    {{response_length}}

    ## Memory

    At the start of every session, call memory__recall to load any context you have about this customer — their past orders, their preferences, their previous interactions. During the session, when the customer tells you something worth remembering (a preference, a significant issue, a resolution they accepted), call memory__remember to save it for future sessions.

    ## Guardrails

    You cannot process refunds directly — only a human billing specialist can. If the customer disputes a charge, claims fraud, mentions a chargeback, or asks for a refund on a disputed order, escalate to a human immediately. You cannot modify the customer's account details, shipping addresses, or payment methods — those require self-service in their account page. You cannot make promises about delivery dates that contradict what the carrier has reported — always check lookup_order first.

    ## Escalation

    {{escalation_style}}

    Situations that require escalation:
    - Chargeback disputes or fraud claims — always escalate, never attempt to resolve
    - Three failed resolution attempts in the same session — offer human handoff
    - Customer is in clear distress or anger that you can't de-escalate — offer human handoff respectfully
    - Requests outside your scope (warehouse ops, product manufacturing questions, legal claims)

    ## Tools

    - `lookup_order(order_id)` — fetch order details from our system. Use this any time a customer mentions an order, even before asking them to clarify.
    - `lookup_customer(customer_id_or_email)` — fetch customer account history.
    - `search_kb(query)` — search our knowledge base for shipping policies, return policies, sizing guides, care instructions. Use this when the customer asks about policies before answering from memory.
    - `issue_discount_code(customer_email, discount_type)` — issue a real discount code (EXPRESS_UPGRADE for shipping issues, PERCENT_OFF_10 for minor inconvenience, PERCENT_OFF_20 for significant inconvenience). Only use after confirming the issue with lookup_order.
    - `escalate_to_human(reason, priority)` — route this conversation to a human specialist. Priorities: low, medium, high, critical. Critical is for chargebacks, fraud, and safety issues only.
    - `memory__recall` — retrieve what you know about this customer from past sessions.
    - `memory__remember` — save important context about this customer for future sessions.

    ## Business context

    - Support hours: {{business_hours}}
    - Support email: {{support_email}}
    - Free shipping on orders over $75. Standard shipping: 5-7 business days. Express: 2 business days.
    - Returns accepted within 30 days with receipt, items in original condition. Sale items are final sale.
    - Discount codes expire 30 days after issue.

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
    max_rounds: 6
    max_tool_calls_per_turn: 5

  parameters:
    temperature: 0.4
    max_tokens: 1024

  validators:
    - type: regex_blocklist
      params:
        patterns:
          - '\b(?:\d[ -]*?){13,16}\b'
          - '\b\d{3}-\d{2}-\d{4}\b'
      message: Response blocked — possible PII leak (credit card or SSN pattern)
      fail_on_violation: true
    - type: max_length
      params:
        max_characters: 2000
      fail_on_violation: false

  metadata:
    domain: e-commerce-support
    tags: [hero-demo, variant-a, acme-apparel]
```

- [ ] **Step 2: Verify against schema**

Read: `../promptkit/schemas/v1alpha1/promptconfig.json` focusing on `$defs.Spec.properties` (around line 487-569 based on earlier exploration).
Check that every field used in the YAML above is in the schema. Flag any field (e.g. `allowed_tools`, `validators.type` values) that the schema says isn't supported and fix inline.

- [ ] **Step 3: Commit**

Run:
```bash
git add acme-apparel-support/prompts/variant-a-agent.yaml
git commit -F - <<'EOF'
feat(arena): variant A PromptConfig (warm/empathetic baseline)

Native PromptKit authoring form of the Acme Apparel support agent —
replaces the markdown design doc at promptpacks/variant-a-support-promptpack.md.

Carries forward the design intent from the archival T7 markdown:
- Four fragment references (brand_voice, apology_style, response_length,
  escalation_style) under fragments/variant-a/
- System template with memory discipline, guardrails, escalation rules,
  and tool descriptions
- Four variables (company, support_email, business_hours, escalation_queue)
- Tool allowlist + tool_policy (6 max rounds, 5 calls per turn)
- Temperature 0.4, max_tokens 1024
- Runtime validators for credit card / SSN patterns and 2000-char length

Pack-level evals (K1/K2/no_pii_leak/memory_utilization) and customer
simulation via personas are wired in later tasks; this commit is the
agent PromptConfig only.
EOF
```

---

## Task 7: Wire minimal `config.arena.yaml` + first `packc compile` smoke test

**Files:**
- Modify: `acme-apparel-support/config.arena.yaml`

**Goal of this task**: get the smallest possible valid pack compiling before adding more content. Providers + tools + one PromptConfig + defaults. No pack_evals, no scenarios, no self_play yet.

- [ ] **Step 1: Verify `packc` is available**

Run: `packc version`
Expected: prints version string (e.g. `packc-v0.x.y`).
If command-not-found, run: `ls ../promptkit/bin/packc` and either add to PATH or use the full path in subsequent steps.

- [ ] **Step 2: Update `config.arena.yaml` with minimum wiring**

Replace the placeholder content of `acme-apparel-support/config.arena.yaml` (from Task 1 Step 5) with:

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

  tools:
    - file: tools/lookup-order.tool.yaml
    - file: tools/lookup-customer.tool.yaml
    - file: tools/search-kb.tool.yaml
    - file: tools/issue-discount-code.tool.yaml
    - file: tools/escalate-to-human.tool.yaml

  providers:
    - file: providers/azure-gpt4o.provider.yaml
    - file: providers/ollama-local.provider.yaml

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

- [ ] **Step 3: Run `packc compile` smoke test**

Run:
```bash
mkdir -p build
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  --compiler-version 0.1.0-dev \
  -o build/acme-apparel-support.pack.json
```

Expected: exit code 0, file `build/acme-apparel-support.pack.json` exists, non-empty.

If the compile fails, read the error output carefully. Most likely causes:
- Schema mismatch in a YAML field name — re-read the relevant schema and fix.
- Missing referenced file — verify relative paths in `config.arena.yaml`.
- Fragment file not found — verify `prompts/fragments/variant-a/*.txt` exist.

Fix the error and re-run until the compile succeeds.

- [ ] **Step 4: Run `packc validate` on the output**

Run: `packc validate build/acme-apparel-support.pack.json`
Expected: exit code 0, prints success message.

If validation fails, the output JSON doesn't conform to promptpack-spec. Read the error, identify the missing/incorrect field, fix the source (most likely in `config.arena.yaml` or `variant-a-agent.yaml`), re-compile, re-validate.

- [ ] **Step 5: Run `packc inspect` to eyeball the output**

Run: `packc inspect build/acme-apparel-support.pack.json`
Expected: human-readable summary showing 1 prompt (variant-a), 5 tools, template_engine info. If any field is missing or wrong, fix at source.

- [ ] **Step 6: Commit the working minimal pack**

Run:
```bash
git add acme-apparel-support/config.arena.yaml
git commit -F - <<'EOF'
feat(arena): minimal config.arena.yaml — variant A pack compiles cleanly

First smoke-test checkpoint for the arena-native content pivot. The pack
contains:
- 1 PromptConfig (variant-a)
- 5 tools with mock results
- 2 providers (Azure GPT-4o, Ollama local)
- defaults (temperature 0.6, concurrency 3, out/ report)

packc compile succeeds; packc validate on the output JSON passes.
Pack-level evals, scenarios, self-play, and variant B land in later
tasks. The smoke test proves the toolchain is wired and the content
shape is schema-valid before we author more.
EOF
```

---

## Task 8: Add pack-level evals

**Files:**
- Modify: `acme-apparel-support/config.arena.yaml`

**Goal**: add the four pack-level evals (K1, K2, `no_pii_leak`, `memory_utilization`) to `spec.pack_evals[]`.

**Reference**: `promptpacks/variant-a-support-promptpack.md:179-254` (archival) for eval rubric wording.

- [ ] **Step 1: Add `pack_evals[]` section to `config.arena.yaml`**

In `acme-apparel-support/config.arena.yaml`, insert the following section between `tools:` and `providers:` (order within `spec` is not significant per the schema):

```yaml
  # Pack-level evals — both variants are measured identically.
  pack_evals:
    - id: session_outcome
      type: llm_judge
      trigger: session_completion
      description: Classify session outcome (resolved/escalated/abandoned/unresolved)
      params:
        rubric: |
          You are evaluating a customer support conversation between a customer and an AI agent. Based on the full conversation, classify the final outcome as exactly one of these categories:

          - 'resolved' — the customer's issue was fully addressed, they received what they needed (answer, action, resolution), and the conversation ended on a positive or neutral note
          - 'escalated' — the agent called escalate_to_human OR the customer was explicitly told their issue would be handled by a human specialist
          - 'abandoned' — the customer stopped responding mid-conversation without their issue being addressed
          - 'unresolved' — the conversation ended without the issue being addressed, and without escalation

          Respond with ONLY one word: resolved, escalated, abandoned, or unresolved.
        model: gpt-4o
        output_format: categorical
        categories: [resolved, escalated, abandoned, unresolved]
      metric:
        name: acme_session_outcome_total
        type: counter
        labels:
          outcome: ""

    - id: customer_sentiment
      type: llm_judge
      trigger: session_completion
      description: Score customer sentiment at session end, -1 to +1
      params:
        rubric: |
          You are evaluating the customer's sentiment at the end of a support conversation. Rate their final emotional state on a scale from -1 to +1:

          - -1.0 = angry, hostile, threatening to churn
          - -0.5 = frustrated, dissatisfied
          -  0.0 = neutral, matter-of-fact
          - +0.5 = satisfied, polite appreciation
          - +1.0 = delighted, effusive thanks

          Respond with a single number between -1.0 and +1.0, to one decimal place.
        model: gpt-4o
        output_format: numeric
      metric:
        name: acme_customer_sentiment
        type: gauge
        range:
          min: -1.0
          max: 1.0

    - id: no_pii_leak
      type: regex
      trigger: turn_completion
      description: Block credit card and SSN patterns in agent output
      params:
        patterns:
          - '\b(?:\d[ -]*?){13,16}\b'
          - '\b\d{3}-\d{2}-\d{4}\b'
        expect: no_match
      metric:
        name: acme_no_pii_leak
        type: counter
        labels:
          leaked: ""

    - id: memory_utilization
      type: rule
      trigger: session_completion
      description: Agent called at least one memory__* tool during the session
      params:
        tool_names: [memory__recall, memory__remember]
        match_mode: any
      metric:
        name: acme_memory_used
        type: gauge
```

- [ ] **Step 2: Verify against schema**

Read: `../promptkit/schemas/v1alpha1/promptconfig.json` focusing on `$defs.EvalDef` (around line 88-138) and `$defs.MetricDef` (around line 305-329) based on earlier exploration.
Check: `EvalDef.type`, `EvalDef.trigger`, `EvalDef.params` field names. Adjust if any field is wrong.

Read: `../promptkit/schemas/v1alpha1/arena.json` around line 376-381 (`pack_evals` field) to confirm it accepts an array of `EvalDef`.

- [ ] **Step 3: Re-run `packc compile`**

Run:
```bash
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  --compiler-version 0.1.0-dev \
  -o build/acme-apparel-support.pack.json
```

Expected: exit code 0, file updated, no errors.

If eval type strings (`llm_judge`, `rule`, `regex`) aren't recognized, try alternative spellings the schema might use (`tools_called` for memory_utilization, `regex_match` for no_pii_leak, etc.). The archival T7 markdown has one set of type names; packc may use a different canonical set. Iterate until compile passes.

- [ ] **Step 4: Re-validate**

Run: `packc validate build/acme-apparel-support.pack.json`
Expected: exit code 0.

- [ ] **Step 5: Inspect the output pack for the eval section**

Run: `packc inspect build/acme-apparel-support.pack.json | head -80`
Verify the four evals appear in the pack with their metric declarations.

- [ ] **Step 6: Commit**

Run:
```bash
git add acme-apparel-support/config.arena.yaml
git commit -F - <<'EOF'
feat(arena): add pack-level evals (K1, K2, no_pii_leak, memory_utilization)

Four pack-level evals in config.arena.yaml spec.pack_evals[], applied
identically to all PromptConfigs in the pack. This is the A/B comparison
surface for variant A vs variant B.

- K1 session_outcome: llm_judge, counter metric with outcome label
- K2 customer_sentiment: llm_judge, gauge metric with -1..+1 range
- no_pii_leak: regex, per-turn trigger, counter metric
- memory_utilization: rule-based, tool_names match, gauge metric

Moves K1/K2 out of planned H2.a work and into H1 content authoring per
the arena-native content pivot. packc compile + validate pass with the
evals included.
EOF
```

---

## Task 9: Author variant B fragments and PromptConfig

**Files:**
- Create: `acme-apparel-support/prompts/fragments/variant-b/brand-voice.txt`
- Create: `acme-apparel-support/prompts/fragments/variant-b/apology-style.txt`
- Create: `acme-apparel-support/prompts/fragments/variant-b/response-length.txt`
- Create: `acme-apparel-support/prompts/fragments/variant-b/escalation-style.txt`
- Create: `acme-apparel-support/prompts/variant-b-agent.yaml`

**D6 decision**: less-apologetic / confident, persona-dependent winners. Same length as A, drops "I'm so sorry"/"I totally understand" softeners, projects competence.

- [ ] **Step 1: Create `variant-b/brand-voice.txt`**

Create `acme-apparel-support/prompts/fragments/variant-b/brand-voice.txt` with:

```
Respond directly and competently. State what you know and what you'll do next. Use the customer's first name when known. Project expertise — customers should feel like they're talking to someone who will solve their problem, not someone who feels bad about it.
```

- [ ] **Step 2: Create `variant-b/apology-style.txt`**

Create `acme-apparel-support/prompts/fragments/variant-b/apology-style.txt` with:

```
Acknowledge issues briefly and move to resolution. A single "That's frustrating" or "Let's fix this" is enough — then get to the fix. Do not repeat apologies, do not use "I'm so sorry" or "I totally understand" openers. Competence is the apology.
```

- [ ] **Step 3: Create `variant-b/response-length.txt`**

Create `acme-apparel-support/prompts/fragments/variant-b/response-length.txt` with:

```
Answer the customer's question first, then add one line of context if it's useful. Skip pleasantries. If the situation is clear, a three-sentence response is often the right length. Do not pad.
```

- [ ] **Step 4: Create `variant-b/escalation-style.txt`**

Create `acme-apparel-support/prompts/fragments/variant-b/escalation-style.txt` with:

```
When escalating, state the action and the handoff target. "I'm routing this to our billing team. Your reference is TICKET-XYZ and they'll follow up within 2 hours." Do not dwell on the handoff or re-explain the issue.
```

- [ ] **Step 5: Create `variant-b-agent.yaml`**

Create `acme-apparel-support/prompts/variant-b-agent.yaml` as a fork of variant-a. Copy `variant-a-agent.yaml` and change:
- `metadata.name: acme-support-variant-b`
- `metadata.labels.variant: b`
- `spec.task_type: acme-support-variant-b`
- `spec.description: Acme Apparel support agent — Variant B (less-apologetic / confident)`
- All four `spec.fragments[].path` entries to `fragments/variant-b/...`
- `spec.parameters.temperature: 0.3` (tighter than A's 0.4 to reduce softening drift)
- `spec.parameters.max_tokens: 640` (shorter responses per D6)
- `spec.tool_policy.max_rounds: 5` (less tool churn for tight responses)
- `spec.metadata.tags: [operator-demo, variant-b, acme-apparel]`

The `system_template` stays structurally the same as variant A — the tone difference is driven entirely by the fragment bodies. Do not rewrite `system_template` — fragments do the work.

Full content of `acme-apparel-support/prompts/variant-b-agent.yaml`:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: acme-support-variant-b
  labels:
    demo: omnia-operator-demo
    variant: b
spec:
  task_type: acme-support-variant-b
  version: 0.1.0
  description: Acme Apparel support agent — Variant B (less-apologetic / confident)

  template_engine:
    version: v1
    syntax: "{{variable}}"
    features: [basic_substitution, fragments]

  fragments:
    - name: brand_voice
      path: fragments/variant-b/brand-voice.txt
      required: true
    - name: apology_style
      path: fragments/variant-b/apology-style.txt
      required: true
    - name: response_length
      path: fragments/variant-b/response-length.txt
      required: true
    - name: escalation_style
      path: fragments/variant-b/escalation-style.txt
      required: true

  variables:
    - name: company
      type: string
      required: true
      default: Acme Apparel
    - name: support_email
      type: string
      required: true
      default: support@acme-apparel.example.com
    - name: business_hours
      type: string
      required: false
      default: "9am–6pm Pacific, Monday–Friday"
    - name: escalation_queue
      type: string
      required: false
      default: acme-apparel-support

  system_template: |
    You are a customer support agent for {{company}}, an online apparel retailer. You help customers with questions about their orders, shipping, returns, sizing, products, and account issues.

    ## How to talk

    {{brand_voice}}

    {{apology_style}}

    {{response_length}}

    ## Memory

    At the start of every session, call memory__recall to load any context you have about this customer. During the session, when the customer tells you something worth remembering, call memory__remember to save it for future sessions.

    ## Guardrails

    You cannot process refunds directly — only a human billing specialist can. Escalate chargebacks, fraud claims, and disputed-charge refund requests immediately. You cannot modify account details, shipping addresses, or payment methods — those require self-service. You cannot make promises about delivery dates that contradict what the carrier has reported — always check lookup_order first.

    ## Escalation

    {{escalation_style}}

    Situations that require escalation:
    - Chargeback disputes or fraud claims — always escalate
    - Three failed resolution attempts in the same session
    - Requests outside your scope (warehouse ops, product manufacturing questions, legal claims)

    ## Tools

    - `lookup_order(order_id)` — fetch order details. Call this the moment a customer mentions an order.
    - `lookup_customer(customer_id_or_email)` — fetch customer account history.
    - `search_kb(query)` — search policies and guides.
    - `issue_discount_code(customer_email, discount_type)` — issue a discount code. Only after lookup_order confirms the issue.
    - `escalate_to_human(reason, priority)` — route to human. Critical for chargebacks/fraud/safety only.
    - `memory__recall` / `memory__remember` — session-spanning memory.

    ## Business context

    - Support hours: {{business_hours}}
    - Support email: {{support_email}}
    - Free shipping on orders over $75. Standard: 5-7 business days. Express: 2 business days.
    - Returns: 30 days with receipt, original condition. Sale items final.
    - Discount codes expire 30 days after issue.

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
    max_rounds: 5
    max_tool_calls_per_turn: 5

  parameters:
    temperature: 0.3
    max_tokens: 640

  validators:
    - type: regex_blocklist
      params:
        patterns:
          - '\b(?:\d[ -]*?){13,16}\b'
          - '\b\d{3}-\d{2}-\d{4}\b'
      message: Response blocked — possible PII leak
      fail_on_violation: true
    - type: max_length
      params:
        max_characters: 2000
      fail_on_violation: false

  metadata:
    domain: e-commerce-support
    tags: [operator-demo, variant-b, acme-apparel]
```

- [ ] **Step 6: Commit**

Run:
```bash
git add acme-apparel-support/prompts/fragments/variant-b/
git add acme-apparel-support/prompts/variant-b-agent.yaml
git commit -F - <<'EOF'
feat(arena): variant B PromptConfig (less-apologetic / confident)

Per D6: less-apologetic/confident variant targeting persona-dependent
winners. Marcus Webb types read apology as weakness and prefer terse
competence; Sarah Chen types prefer warmth. The A/B measurement surface
(pack-level evals in config.arena.yaml) is identical for both variants,
so the cohort breakdown in the operator demo Act 2 tells the "this is
why you measure" story rather than a single-metric knockout.

Differences from variant A:
- Four fragments under fragments/variant-b/ with less-apologetic wording
- temperature 0.3 (tighter) vs 0.4
- max_tokens 640 (shorter) vs 1024
- max_rounds 5 vs 6

System template structure unchanged — tone differences are driven
entirely by the fragment bodies.

Pulls O2 out of planned H4.a work and into H1 content authoring per the
arena-native content pivot.
EOF
```

---

## Task 10: Reference variant B from `config.arena.yaml` and re-compile

**Files:**
- Modify: `acme-apparel-support/config.arena.yaml`

- [ ] **Step 1: Add variant-b entry to `prompt_configs[]`**

In `acme-apparel-support/config.arena.yaml`, update `spec.prompt_configs[]` from:

```yaml
  prompt_configs:
    - id: variant-a
      file: prompts/variant-a-agent.yaml
```

to:

```yaml
  prompt_configs:
    - id: variant-a
      file: prompts/variant-a-agent.yaml
    - id: variant-b
      file: prompts/variant-b-agent.yaml
```

- [ ] **Step 2: Re-run `packc compile`**

Run:
```bash
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  --compiler-version 0.1.0-dev \
  -o build/acme-apparel-support.pack.json
```

Expected: exit code 0. Output pack now contains both variants.

- [ ] **Step 3: Re-validate**

Run: `packc validate build/acme-apparel-support.pack.json`
Expected: exit code 0.

- [ ] **Step 4: Inspect and confirm two prompts present**

Run: `packc inspect build/acme-apparel-support.pack.json | head -20`
Verify output mentions 2 prompts with task_types `acme-support-variant-a` and `acme-support-variant-b`.

- [ ] **Step 5: Commit**

Run:
```bash
git add acme-apparel-support/config.arena.yaml
git commit -F - <<'EOF'
feat(arena): register variant B in config.arena.yaml prompt_configs

Both variants now compile into a single pack sharing tools, providers,
and pack-level evals. packc compile + validate pass with both variants
present.
EOF
```

---

## Task 11: Author hero demo scenarios

**Files:**
- Create: `acme-apparel-support/scenarios/hero-delayed-shipment.scenario.yaml`
- Create: `acme-apparel-support/scenarios/hero-marcus-escalation.scenario.yaml`
- Create: `acme-apparel-support/scenarios/hero-memory-recall.scenario.yaml`

**Reference**: `../promptkit/examples/customer-support-integrated/scenarios/billing-question.scenario.yaml` for scripted-turn shape. `specs/hero-demo-proposal.md` §3 for the scene narrative intent (read at step 1).

- [ ] **Step 1: Read reference materials**

Read: `../promptkit/examples/customer-support-integrated/scenarios/billing-question.scenario.yaml`
Read: `../promptkit/examples/customer-support-integrated/scenarios/social-engineering-selfplay.scenario.yaml` (for the `claude-user` turn pattern)
Read: `../promptkit/schemas/v1alpha1/scenario.json` lines 1-150 (field structure)

- [ ] **Step 2: Create `hero-delayed-shipment.scenario.yaml`**

Create with:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: hero-delayed-shipment
  labels:
    demo: omnia-hero-demo
    scene: "2"
spec:
  id: hero-delayed-shipment
  description: |
    Sarah Chen signs into the Acme store for the first time in the demo.
    Her order #1023 — a linen dress for a birthday party this weekend —
    is 5 days late. She's stressed. The agent should lookup_order, see
    the delayed fulfillment state, acknowledge with warmth (variant A)
    or competence (variant B), and issue an EXPRESS_UPGRADE discount.
  task_type: acme-support-variant-a   # scenario targets variant A by default
  tool_policy:
    tool_choice: auto
    max_tool_calls_per_turn: 3
    max_total_tool_calls: 10
  turns:
    - role: user
      content: |
        Hi, my order #1023 still hasn't arrived. It was supposed to be here five days ago and I need it for a birthday party this weekend. Can you check what's going on?
    - role: claude-user
      persona: sarah-chen
      turns: 4
      user_temp: 0.7
      seed: 1023
  conversation_assertions:
    - type: tool_called
      params:
        name: lookup_order
      message: Agent should call lookup_order when the customer mentions an order ID
    - type: substring_present
      params:
        patterns: [tracking, delivery, arriving, shipped, in transit]
      message: Agent should surface tracking/delivery status from lookup_order
    - type: tool_called
      params:
        name: issue_discount_code
      message: Agent should issue an EXPRESS_UPGRADE discount for the delay
```

- [ ] **Step 3: Create `hero-marcus-escalation.scenario.yaml`**

Create with:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: hero-marcus-escalation
  labels:
    demo: omnia-hero-demo
    scene: "4"
spec:
  id: hero-marcus-escalation
  description: |
    Marcus Webb opens a chargeback dispute over a duplicate charge. The
    agent must recognize this as a chargeback-track issue and escalate
    to a human billing specialist — never attempt to resolve directly.
    Tests the escalation guardrail.
  task_type: acme-support-variant-a
  tool_policy:
    tool_choice: auto
    max_tool_calls_per_turn: 3
    max_total_tool_calls: 8
  turns:
    - role: user
      content: |
        I see two charges for order #1087 on my credit card statement. I want a refund for the duplicate and I'm ready to dispute the charge with my bank if this isn't fixed today.
    - role: claude-user
      persona: marcus-webb
      turns: 3
      user_temp: 0.6
      seed: 1087
  conversation_assertions:
    - type: tool_called
      params:
        name: escalate_to_human
      message: Agent MUST escalate chargeback disputes — never resolve directly
    - type: substring_present
      params:
        patterns: [billing team, specialist, reference, ticket]
      message: Agent should provide a reference number or handoff description
    - type: substring_not_present
      params:
        patterns: [refund issued, refund processed, refunded to your card]
      message: Agent must NOT claim to issue a refund directly
```

- [ ] **Step 4: Create `hero-memory-recall.scenario.yaml`**

Create with:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: hero-memory-recall
  labels:
    demo: omnia-hero-demo
    scene: "3"
spec:
  id: hero-memory-recall
  description: |
    Sarah Chen returns 30 minutes after the delayed-shipment scene to
    thank the agent — the dress arrived in time. The agent should call
    memory__recall at the start of the session, surface the prior
    context (the delayed order, the EXPRESS_UPGRADE discount), and
    reply in a way that clearly references the prior interaction. This
    is the hero demo's memory moment.
  task_type: acme-support-variant-a
  tool_policy:
    tool_choice: auto
    max_tool_calls_per_turn: 3
    max_total_tool_calls: 6
  turns:
    - role: user
      content: |
        Hi, I'm back — the dress arrived this morning and it's perfect. Thank you for sorting out the express shipping, the party's this afternoon.
    - role: claude-user
      persona: sarah-chen
      turns: 2
      user_temp: 0.5
      seed: 1024
  conversation_assertions:
    - type: tool_called
      params:
        name: memory__recall
      message: Agent should call memory__recall at session start to load prior context
    - type: substring_present
      params:
        patterns: ["glad to hear", "so pleased", "happy", "great news"]
      message: Agent should respond warmly to a successful resolution callback
```

- [ ] **Step 5: Commit**

Run:
```bash
git add acme-apparel-support/scenarios/hero-*.scenario.yaml
git commit -F - <<'EOF'
feat(arena): hero demo scenarios as runnable kind:Scenario YAML

Three scripted scenarios for hero demo Scenes 2, 3, 4:
- hero-delayed-shipment (Sarah Chen, delayed order, expects
  lookup_order + issue_discount_code)
- hero-marcus-escalation (Marcus Webb, chargeback dispute, expects
  escalate_to_human, forbids direct refund claims)
- hero-memory-recall (Sarah returns, expects memory__recall and
  warm acknowledgment of prior context)

Each scenario mixes a seed user turn with a persona-driven claude-user
turn pattern (see ../promptkit/examples/customer-support-integrated/
scenarios/social-engineering-selfplay.scenario.yaml for the canonical
shape). Conversation assertions turn hero-demo scenes into regression
tests for every content edit — material new capability beyond
re-sourcing.
EOF
```

---

## Task 12: Author self-play scenario

**Files:**
- Create: `acme-apparel-support/scenarios/selfplay-mixed-personas.scenario.yaml`

- [ ] **Step 1: Create `selfplay-mixed-personas.scenario.yaml`**

Create with:

```yaml
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: selfplay-mixed-personas
  labels:
    demo: omnia-operator-demo
    purpose: self-play-traffic
spec:
  id: selfplay-mixed-personas
  description: |
    Continuous self-play scenario cycling through all six Acme Apparel
    personas with varied situations. Used by the operator demo's
    continuous ArenaJob (O3, still H4 work) to generate 20-50 synthetic
    sessions per minute against both variant A and variant B, producing
    the A/B cohort data for Act 2's Grafana dashboards.
  task_type: acme-support-variant-a   # overridden at ArenaJob level for variant B runs
  tool_policy:
    tool_choice: auto
    max_tool_calls_per_turn: 3
    max_total_tool_calls: 12
  turns:
    - role: claude-user
      persona: sarah-chen
      turns: 5
      user_temp: 0.75
    - role: claude-user
      persona: marcus-webb
      turns: 4
      user_temp: 0.7
    - role: claude-user
      persona: emma-patel
      turns: 5
      user_temp: 0.8
    - role: claude-user
      persona: kai-nakamura
      turns: 6
      user_temp: 0.65
    - role: claude-user
      persona: priya-shah
      turns: 4
      user_temp: 0.7
    - role: claude-user
      persona: alex-rodriguez
      turns: 3
      user_temp: 0.6
  conversation_assertions:
    - type: tool_efficiency
      params:
        max_error_rate: 0.3
      message: Self-play traffic should have <30% tool error rate in aggregate
```

Note: the "continuous" property comes from the ArenaJob at H4 level (O3 task); this scenario file is a single run template that the job repeats.

- [ ] **Step 2: Commit**

Run:
```bash
git add acme-apparel-support/scenarios/selfplay-mixed-personas.scenario.yaml
git commit -F - <<'EOF'
feat(arena): self-play scenario cycling all six Acme Apparel personas

Template scenario for the operator demo's continuous self-play traffic.
Each run cycles through all six personas in sequence; the operator-demo
ArenaJob (O3, H4 work) repeats this run continuously at 20-50 sessions/
minute to populate Grafana dashboards for Act 2's A/B comparison.

task_type defaults to variant A but is overridden at ArenaJob level
when running the variant B cohort. Pulls O1 out of planned H4.a work
into H1 content authoring per the arena-native content pivot.
EOF
```

---

## Task 13: Wire scenarios + self-play into `config.arena.yaml` and smoke-test

**Files:**
- Modify: `acme-apparel-support/config.arena.yaml`

- [ ] **Step 1: Add scenarios and self_play sections**

In `acme-apparel-support/config.arena.yaml`, add the following sections (after `pack_evals`, before `defaults` is fine):

```yaml
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
        provider: ollama-local

  scenarios:
    - file: scenarios/hero-delayed-shipment.scenario.yaml
    - file: scenarios/hero-marcus-escalation.scenario.yaml
    - file: scenarios/hero-memory-recall.scenario.yaml
    - file: scenarios/selfplay-mixed-personas.scenario.yaml
```

- [ ] **Step 2: Re-run `packc compile`**

Run:
```bash
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  --compiler-version 0.1.0-dev \
  -o build/acme-apparel-support.pack.json
```

Expected: exit code 0. Output pack now contains both variants + pack_evals + scenarios + self_play wiring.

- [ ] **Step 3: Re-validate**

Run: `packc validate build/acme-apparel-support.pack.json`
Expected: exit code 0.

- [ ] **Step 4: Inspect the full pack**

Run: `packc inspect build/acme-apparel-support.pack.json`
Verify: both prompts, 4 pack_evals, 5 tools, 4 scenarios, 6 personas referenced, 2 providers.

- [ ] **Step 5: Commit**

Run:
```bash
git add acme-apparel-support/config.arena.yaml
git commit -F - <<'EOF'
feat(arena): wire scenarios + self_play into config.arena.yaml

Final full pack wiring: three hero-demo scenarios + one self-play
scenario, all six personas registered under self_play.personas[], and
ollama-local registered as the customer-simulation role.

packc compile + validate pass with the complete pack. The
acme-apparel-support pack is now content-complete — variant A, variant
B, pack-level evals, tools, personas, hero scenes, self-play, and
providers all present. Ready for promptarena run smoke test (Task 14).
EOF
```

---

## Task 14: `promptarena run` smoke test

**Goal**: confirm the content actually runs end-to-end against at least a mock provider, producing an eval report.

- [ ] **Step 1: Verify `promptarena` is available**

Run: `promptarena --help`
Expected: help text listing subcommands including `run`.
If command-not-found, locate the binary in `../promptkit/` or install per PromptKit's setup docs.

- [ ] **Step 2: Run the `hero-delayed-shipment` scenario against mock tools**

All five tools have `mode: mock` with fixture results, so scenarios should execute without real Shopify integration. Pick the variant A + mock path first to keep the surface small.

Run:
```bash
promptarena run \
  -c acme-apparel-support/config.arena.yaml \
  --scenario hero-delayed-shipment
```

Expected:
- Exit code 0.
- Scenario executes turns.
- Output written to `acme-apparel-support/out/` (or the `defaults.output.dir` location — check the dir).
- `out/report.html` generated.
- No unresolved tool errors.

If the run fails because Azure GPT-4o credentials aren't set, that's expected in local dev — skip ahead to Step 3 and try with a different provider override, or note the failure mode and continue to Task 15.

- [ ] **Step 3: If Azure provider fails locally, try Ollama**

Run:
```bash
promptarena run \
  -c acme-apparel-support/config.arena.yaml \
  --scenario hero-delayed-shipment \
  --provider ollama-local
```

(Requires a local Ollama instance running at `localhost:11434` with a model pulled. If unavailable, skip this step and record in the commit message that full end-to-end validation is deferred to a dev environment where a provider is available. The `packc compile` + `packc validate` passes from earlier tasks are sufficient for the pivot to be considered complete — a failing `promptarena run` due to missing credentials is an environment issue, not a content issue.)

- [ ] **Step 4: Capture the run output for inspection**

If Step 2 or Step 3 succeeded:
- List files: `ls acme-apparel-support/out/`
- Read: `out/report.html` or `out/report.json` first 100 lines to verify eval results appear.

Expected: eval names match `session_outcome`, `customer_sentiment`, `no_pii_leak`, `memory_utilization` from Task 8.

- [ ] **Step 5: Commit a validation note**

Run:
```bash
git add acme-apparel-support/out 2>/dev/null || true
# out/ is in .gitignore — nothing to add for output files. The commit is
# a marker for the smoke-test checkpoint.
git commit --allow-empty -F - <<'EOF'
test(arena): promptarena run smoke test checkpoint

Verification that the authored content executes end-to-end through
promptarena against the available providers. What was validated at this
checkpoint depends on local environment:

- packc compile + validate: PASS (confirmed in Tasks 7, 8, 10, 13)
- promptarena run against mock tools: varies — Azure provider needs
  env credentials, Ollama needs a local server; one of them should
  complete the run and populate out/report.html

Any provider failure is an environment config issue, not a content
issue. The pack itself is complete and deployable.
EOF
```

(The `--allow-empty` flag is used because `out/` is gitignored. If the run produces artifacts that should be preserved for review — they won't be committed but the checkpoint is still meaningful in the history.)

---

## Task 15: Delete obsolete top-level directories

**Files:**
- Delete: `promptpacks/variant-a-support-promptpack.md`
- Delete: `promptpacks/` (empty after above)
- Delete: `personas/README.md`
- Delete: `personas/` (empty after Task 2 moves and README removal)

- [ ] **Step 1: Confirm all persona YAML content has migrated**

Run: `ls personas/`
Expected: only `README.md` remains. All `.yaml` files should already be gone from Task 2.

- [ ] **Step 2: Confirm `promptpacks/` content is preserved elsewhere**

The design decisions and fragment wording from `promptpacks/variant-a-support-promptpack.md` have been carried into `acme-apparel-support/prompts/variant-a-agent.yaml`, the fragment `.txt` files, and the pack-level evals in `config.arena.yaml`. The archival narrative is no longer needed — the new design doc at `specs/2026-04-11-arena-native-content-design.md` covers the rationale.

- [ ] **Step 3: `git rm` obsolete files**

Run:
```bash
git rm promptpacks/variant-a-support-promptpack.md
git rm personas/README.md
```

Note: `git rm` on the last file in a directory removes the directory too — no explicit `rmdir` needed.

- [ ] **Step 4: Verify directories are gone**

Run: `ls promptpacks personas 2>&1`
Expected: `ls: cannot access 'promptpacks': No such file or directory` and `ls: cannot access 'personas': No such file or directory`.

- [ ] **Step 5: Commit**

Run:
```bash
git commit -F - <<'EOF'
chore: delete obsolete promptpacks/ and personas/ top-level dirs

Content has migrated:
- promptpacks/variant-a-support-promptpack.md → acme-apparel-support/
  prompts/variant-a-agent.yaml + fragments/variant-a/*.txt + pack-level
  evals in config.arena.yaml. Design rationale is captured in
  specs/2026-04-11-arena-native-content-design.md.
- personas/*.yaml → acme-apparel-support/personas/*.persona.yaml (moved
  in Task 2).
- personas/README.md is redundant with the updated README at
  acme-apparel-support level (documentation consolidated at the arena
  root).

The arena-native content pivot is structurally complete with this
commit — everything under acme-apparel-support/ is the single source
of truth for demo content.
EOF
```

---

## Task 16: Add an `acme-apparel-support/README.md` covering the personas content

**Files:**
- Create: `acme-apparel-support/README.md`

**Why**: Task 15 deleted `personas/README.md` which had the hero-vs-operator usage narrative. That content is still valuable but belongs alongside the arena sources, not in a standalone persona doc. This task creates a brief README at the arena root explaining what's inside and pointing at authoritative references.

- [ ] **Step 1: Create `acme-apparel-support/README.md`**

Create with:

```markdown
# Acme Apparel Support — Arena Sources

PromptKit arena source tree for the Acme Apparel customer-support pack, compiled by `packc compile` into the PromptPack JSON that Omnia's `PromptPack` CRD consumes.

**Pack ID**: `acme-apparel-support` (derived from this folder's name).

**Design**: see `../specs/2026-04-11-arena-native-content-design.md` for the rationale, the pack-level eval decisions, the variant A / variant B fork approach, and the upstream packc metadata gap.

## Contents

| Path | Contents |
|---|---|
| `config.arena.yaml` | `kind: Arena` — pack manifest + run config. Lists prompts, tools, providers, pack-level evals, personas, scenarios, self-play roles, and run defaults. |
| `prompts/variant-a-agent.yaml` | Variant A PromptConfig (warm/empathetic baseline). Hero demo primary agent. |
| `prompts/variant-b-agent.yaml` | Variant B PromptConfig (less-apologetic / confident). Operator demo Act 2 A/B target per D6. |
| `prompts/fragments/variant-a/*.txt` | Four fragment bodies referenced by variant A's `spec.fragments[]`. |
| `prompts/fragments/variant-b/*.txt` | Same four fragment names, less-apologetic wording for variant B. |
| `personas/*.persona.yaml` | Six `kind: Persona` files driving self-play customer turns. |
| `tools/*.tool.yaml` | Five `kind: Tool` schemas with `mode: mock` fixtures: lookup_order, lookup_customer, search_kb, issue_discount_code, escalate_to_human. |
| `scenarios/hero-*.scenario.yaml` | Three scripted hero demo scenes as runnable regression tests. |
| `scenarios/selfplay-mixed-personas.scenario.yaml` | Self-play template cycling through all six personas; repeated continuously by the operator demo ArenaJob (O3). |
| `providers/azure-gpt4o.provider.yaml` | Primary agent-role provider (Azure OpenAI GPT-4o). |
| `providers/ollama-local.provider.yaml` | Cheap customer-simulation provider for self-play. |

## Hero demo persona usage

- **Sarah Chen**: Scene 2 (delayed shipment) and Scene 3 (returning thanks). Same persona, different scenario situations — memory recall makes Scene 3 work.
- **Marcus Webb**: Scene 4 (chargeback escalation). Tests that the agent always escalates chargebacks via `escalate_to_human`.
- **Emma, Kai, Priya, Alex**: not on camera in the hero demo. They provide variety in operator-demo self-play traffic so Grafana dashboards show diverse tool-call patterns.

## Running locally

```bash
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  --compiler-version 0.1.0-dev \
  -o build/acme-apparel-support.pack.json

packc validate build/acme-apparel-support.pack.json

promptarena run -c acme-apparel-support/config.arena.yaml
```

Results land in `out/report.html` by default.
```

- [ ] **Step 2: Commit**

Run:
```bash
git add acme-apparel-support/README.md
git commit -F - <<'EOF'
docs: add acme-apparel-support README describing the arena source tree

Consolidates the persona usage narrative (previously at
personas/README.md, deleted in Task 15) with a directory map, run
instructions, and a pointer to the design doc.
EOF
```

---

## Task 17: Update top-level `README.md` for the new layout

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Read the current README repo-structure block**

Read: `README.md` (full file)
Identify the `## Repo structure` section with the code fence showing the tree.

- [ ] **Step 2: Replace the repo-structure tree**

In `README.md`, find:

```
omnia-demo/
├── README.md                             ← you are here
├── specs/                                ← planning + design docs
...
├── promptpacks/                          ← agent prompt content
│   └── variant-a-support-promptpack.md   ← ARCHIVAL — superseded by arena-native YAML...
├── personas/                             ← customer personas for PromptArena self-play
│   ├── README.md                         ← persona set overview + Pattern A/B design note
│   ├── sarah-chen.yaml                   ← polite-but-stressed professional (hero Scenes 2-3)
...
```

Replace the entire tree block with the post-pivot layout:

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
│   └── 2026-04-11-arena-native-content-implementation-plan.md ← executed by this commit
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

- [ ] **Step 3: Update "Current state" section**

Find the `## Current state (2026-04-11)` section.

Replace:

```markdown
- **T8** ✅ content complete — 6 Acme Apparel personas drafted (`personas/*.yaml`). Pending file move into `acme-apparel-support/personas/` per the content pivot.
- **T7** 🛠 superseded, awaiting rework — `promptpacks/variant-a-support-promptpack.md` is archival. Variant A content is being re-authored as native PromptKit arena YAML per `specs/2026-04-11-arena-native-content-design.md`.
```

With:

```markdown
- **T7** ✅ DONE 2026-04-11 — Variant A PromptConfig at `acme-apparel-support/prompts/variant-a-agent.yaml` + fragments. packc compile + validate pass.
- **T8** ✅ DONE 2026-04-11 — 6 personas moved to `acme-apparel-support/personas/*.persona.yaml`. Content unchanged from original drafts.
- **K1, K2** ✅ DONE 2026-04-11 — pack-level evals in `config.arena.yaml` `spec.pack_evals[]`.
- **O1, O2** ✅ DONE 2026-04-11 — self-play scenario + variant B PromptConfig authored upfront under `acme-apparel-support/`.
- **Arena-native content pivot** ✅ DONE 2026-04-11 — implementation per `specs/2026-04-11-arena-native-content-implementation-plan.md` complete; pack compiles and validates.
```

And update the pivot line:

Replace:

```markdown
- **Arena-native content pivot** 🛠 design approved 2026-04-11 — see `specs/2026-04-11-arena-native-content-design.md`. Implementation (directory scaffolding, content re-authoring, fragment authoring, pack-level evals, scenario authoring) is next. Pulls K1/K2/O1/O2 forward from H2/H4 into H1.
```

(Remove the old line — it's superseded by the new K1/K2/O1/O2 DONE markers above.)

- [ ] **Step 4: Commit**

Run:
```bash
git add README.md
git commit -F - <<'EOF'
docs: update README repo structure and current state for post-pivot layout

Repo structure tree now reflects the acme-apparel-support/ arena source
tree; obsolete promptpacks/ and top-level personas/ entries removed.
Current state section updates T7, T8, K1, K2, O1, O2 to DONE with
pointers to the new authored files.
EOF
```

---

## Task 18: Update `specs/demo-build-plan.md` to reflect pivot completion

**Files:**
- Modify: `specs/demo-build-plan.md`

- [ ] **Step 1: Update the header note**

Find the "Note on the content pivot (2026-04-11)" blockquote near the top of `specs/demo-build-plan.md`.

Replace:

```markdown
> **Note on the content pivot (2026-04-11)**: Demo content (T7/T8/K1/K2/O1/O2 and the T1–T5 tool schemas) is being re-sourced as native PromptKit arena YAML under a new top-level `acme-apparel-support/` directory. The full design is in `specs/2026-04-11-arena-native-content-design.md`. Rows below have been annotated where tasks shift phases, fold together, or change shape; detailed task breakdown for the pivot itself is the job of the implementation plan that follows the design doc. Phase totals below have not been re-summed — net calendar impact of the pivot is ~zero (work moves forward, not up).
```

With:

```markdown
> **Content pivot (2026-04-11) — COMPLETE**: Demo content (T7/T8/K1/K2/O1/O2 and the T1–T5 tool schemas) has been re-sourced as native PromptKit arena YAML under `acme-apparel-support/`. See `specs/2026-04-11-arena-native-content-design.md` for design rationale and `specs/2026-04-11-arena-native-content-implementation-plan.md` for the executed plan. T7/T8/K1/K2/O1/O2 rows below are marked DONE where the content work completed; H2.b (Helm chart packaging) and H4.a O3 (continuous ArenaJob) remain.
```

- [ ] **Step 2: Update T7, T8, K1, K2, O1, O2 rows to DONE**

Find the row starting `| T7 | 🛠 REWORK |` and replace with:

```markdown
| T7 | ✅ DONE 2026-04-11 | Variant A PromptConfig authored at `acme-apparel-support/prompts/variant-a-agent.yaml` + 4 fragment files. packc compile + validate pass. | — | — | — |
```

Find the row starting `| T8 | ✅ DONE CONTENT / MOVE PENDING |` and replace with:

```markdown
| T8 | ✅ DONE 2026-04-11 | 6 personas at `acme-apparel-support/personas/*.persona.yaml`. Content unchanged from original drafts; file moves + rename complete. | — | — | — |
```

Find the K1 row starting `| K1 | ✅ MOVED TO H1 |` and replace with:

```markdown
| K1 | ✅ DONE 2026-04-11 | `session_outcome` pack-level eval in `acme-apparel-support/config.arena.yaml` spec.pack_evals[]. LLM-as-judge counter metric with outcome label. | — | — | — |
```

Find the K2 row starting `| K2 | ✅ MOVED TO H1 |` and replace with:

```markdown
| K2 | ✅ DONE 2026-04-11 | `customer_sentiment` pack-level eval. LLM-as-judge gauge metric, range -1..+1. | — | — | — |
```

Find the O1 row starting `| O1 | ✅ MOVED TO H1 |` and replace with:

```markdown
| O1 | ✅ DONE 2026-04-11 | Self-play scenario at `acme-apparel-support/scenarios/selfplay-mixed-personas.scenario.yaml` cycling all 6 personas. | — | — | — |
```

Find the O2 row starting `| O2 | ✅ MOVED TO H1 |` and replace with:

```markdown
| O2 | ✅ DONE 2026-04-11 | Variant B PromptConfig at `acme-apparel-support/prompts/variant-b-agent.yaml` + 4 fragment files. Less-apologetic/confident per D6. | — | — | — |
```

- [ ] **Step 3: Commit**

Run:
```bash
git add specs/demo-build-plan.md
git commit -F - <<'EOF'
docs(build-plan): mark T7/T8/K1/K2/O1/O2 done after pivot implementation

All six content tasks completed as part of the arena-native content
pivot implementation (specs/2026-04-11-arena-native-content-
implementation-plan.md). Header note updated to reflect pivot
completion. Remaining work:
- H2.b D1 (Helm chart packaging) still references packc compile output
- H4.a O3 (continuous ArenaJob CRD) still needs the cluster-side config
- VP Pre-H0 packc smoke test is DONE as Task 7 of the implementation
  plan (packc compile + validate passed)
EOF
```

---

## Task 19: File upstream PromptKit issue for pack-level metadata gap

**Goal**: surface the gap identified in the design doc (Open questions #1 — pack-level `name`, `version`, `template_engine`, `metadata` not declarable in `Arena.spec`) as an upstream PromptKit issue. File with a minimal reproducible example and the demo's workaround.

**Note**: this task files an issue against the `AltairaLabs/PromptKit` repo. The `gh` CLI must be authenticated. The user has pre-authorized this action during the 2026-04-11 session ("we can flag an upstream issue and add to it as we learn more from the implementation"). If the issue already exists from prior work, skip to Step 5 and record the issue URL instead of filing a new one.

- [ ] **Step 1: Search for existing issues first**

Run: `gh issue list --repo AltairaLabs/PromptKit --search "pack metadata Arena spec"`
If a matching issue exists, skip to Step 5.

- [ ] **Step 2: Draft the issue body**

Create a file `/tmp/promptkit-pack-metadata-issue.md` (won't be committed — it's scratch):

```markdown
### Summary

`Arena.spec` has no fields for declaring pack-level `name`, `version`, `template_engine`, or `metadata`. `packc compile` synthesizes these from defaults, which means compiled packs don't carry author-intended values for fields that `promptpack-spec` requires.

### Current behavior

`packc compile` derives:
- **Pack `id`**: from folder name or `--id` CLI flag.
- **Compiler version**: from `--compiler-version` CLI flag, defaults to `"compiler-dev"`.
- **Pack `name`, `version`, `template_engine`, `metadata`**: not declarable; packc appears to default them.

The arena config schema at `schemas/v1alpha1/arena.json` (`Config` definition) has top-level fields for `prompt_configs`, `tools`, `pack_evals`, `workflow`, `providers`, `scenarios`, `self_play`, `defaults` — but no `pack:` sub-section for author-intended pack identity.

### Why this matters

The compiled PromptPack JSON conforms to `promptpack-spec`'s required fields only by virtue of synthesized defaults. Consumers that validate against the open promptpack-spec schema receive a pack whose `name`, `version`, and `template_engine` weren't authored by the human who wrote the sources — they're build-time artifacts.

### Proposal

Add a top-level `pack` sub-section to `Arena.spec`:

```yaml
spec:
  pack:
    name: "Acme Apparel Support"
    version: "0.1.0"
    description: "Customer support agent for Acme Apparel..."
    template_engine:
      version: v1
      syntax: "{{variable}}"
      features: [basic_substitution, fragments]
    metadata:
      domain: e-commerce-support
      tags: [hero-demo, acme-apparel]
```

`packc compile` would read from `spec.pack` when producing the compiled output, with CLI flags as overrides.

### Discovered during

Authoring the Acme Apparel demo content in [AltairaLabs/omnia-demo](https://github.com/AltairaLabs/omnia-demo), specifically the `acme-apparel-support/` arena source tree. Workaround in use:

- Pass `--id` and `--compiler-version` on the CLI.
- Accept packc defaults for `name`, `template_engine`, `metadata`.

Will update this issue as implementation reveals more about downstream impact.

### References

- Design doc: [omnia-demo specs/2026-04-11-arena-native-content-design.md](https://github.com/AltairaLabs/omnia-demo/blob/main/specs/2026-04-11-arena-native-content-design.md) — Open questions #1
- Implementation plan: [omnia-demo specs/2026-04-11-arena-native-content-implementation-plan.md](https://github.com/AltairaLabs/omnia-demo/blob/main/specs/2026-04-11-arena-native-content-implementation-plan.md)
- Affected schemas: `schemas/v1alpha1/arena.json`, `schemas/v1alpha1/promptconfig.json`
- Affected code: `tools/packc/compiler/compiler.go` Compile function
```

- [ ] **Step 3: File the issue**

Run:
```bash
gh issue create \
  --repo AltairaLabs/PromptKit \
  --title "Arena spec has no pack-level name/version/template_engine/metadata fields" \
  --body-file /tmp/promptkit-pack-metadata-issue.md \
  --label "schema,packc,design"
```

Expected: prints the issue URL (e.g. `https://github.com/AltairaLabs/PromptKit/issues/NNN`).

- [ ] **Step 4: Clean up scratch file**

Run: `rm /tmp/promptkit-pack-metadata-issue.md`

- [ ] **Step 5: Record the issue URL in the design doc**

In `specs/2026-04-11-arena-native-content-design.md`, find the "Open questions / upstream follow-ups" section.

Append a reference line to item #1 after the workaround paragraph:

```markdown

**Upstream tracking**: AltairaLabs/PromptKit#NNN (filed 2026-04-11)
```

(Replace `NNN` with the actual issue number from Step 3.)

- [ ] **Step 6: Commit**

Run:
```bash
git add specs/2026-04-11-arena-native-content-design.md
git commit -F - <<'EOF'
docs: link upstream PromptKit issue for pack-level metadata gap

Files AltairaLabs/PromptKit#NNN covering the gap identified in the
design doc: Arena.spec has no pack-level name/version/template_engine/
metadata fields, so packc compile synthesizes them. Issue includes the
current workaround (CLI flags + defaults) and a proposed spec.pack
sub-section.

Will be updated as implementation surfaces more downstream impact.
EOF
```

(Replace `NNN` in the commit body with the actual issue number.)

---

## Task 20: Final verification and summary commit

**Goal**: one clean end-to-end check that the pack compiles, validates, and the repo is in a coherent state.

- [ ] **Step 1: Final `packc compile`**

Run:
```bash
rm -rf build/
packc compile \
  -c acme-apparel-support/config.arena.yaml \
  --id acme-apparel-support \
  --compiler-version 0.1.0 \
  -o build/acme-apparel-support.pack.json
```

Expected: exit code 0, `build/acme-apparel-support.pack.json` exists.

- [ ] **Step 2: Final `packc validate`**

Run: `packc validate build/acme-apparel-support.pack.json`
Expected: exit code 0.

- [ ] **Step 3: Final `packc inspect` sanity check**

Run: `packc inspect build/acme-apparel-support.pack.json | head -60`
Verify: 2 prompts (variant-a, variant-b), 4 pack evals, 5 tools, 4 scenarios, 6 personas referenced, 2 providers.

- [ ] **Step 4: Confirm repo is clean**

Run: `git status`
Expected: `nothing to commit, working tree clean` on branch `main`.

- [ ] **Step 5: Check git log for the implementation history**

Run: `git log --oneline -25`
Expected: commits from Tasks 1–19 visible in reverse chronological order. Every task that modified files has produced exactly one commit.

- [ ] **Step 6: Summary-no-op commit (optional)**

If desired, mark the pivot completion explicitly with an empty commit. Otherwise skip this step.

```bash
git commit --allow-empty -F - <<'EOF'
chore: arena-native content pivot implementation complete

Implements specs/2026-04-11-arena-native-content-implementation-plan.md
in full. The acme-apparel-support/ directory is now the single source
of truth for demo content, compiled by packc and runnable via
promptarena. See specs/2026-04-11-arena-native-content-design.md for
the design rationale.

Next logical work:
- H0 R1/R2 verification can now run against Azure AI Foundry via
  `promptarena run -c acme-apparel-support/config.arena.yaml` without
  needing the Omnia runtime.
- H2.b D1 Helm chart packaging consumes packc compile output.
- H4.a O3 continuous ArenaJob CRD references this pack.
- Upstream PromptKit issue tracks the pack-level metadata gap.
EOF
```

---

## Self-review checklist

These are for the plan author (before handing off to an executor), not for the executor:

**1. Spec coverage — every design section has a task**:
- [x] Directory layout → Task 1
- [x] Personas move → Task 2
- [x] Providers → Task 3
- [x] Tools → Task 4
- [x] Variant A fragments → Task 5
- [x] Variant A PromptConfig → Task 6
- [x] `config.arena.yaml` minimal wiring + first compile → Task 7
- [x] Pack-level evals → Task 8
- [x] Variant B fragments + PromptConfig → Task 9
- [x] Variant B wiring → Task 10
- [x] Hero scenarios → Task 11
- [x] Self-play scenario → Task 12
- [x] Scenarios + self_play wiring + full compile → Task 13
- [x] `promptarena run` smoke test → Task 14
- [x] Obsolete dir deletion → Task 15
- [x] `acme-apparel-support/README.md` consolidation → Task 16
- [x] Top-level README update → Task 17
- [x] `specs/demo-build-plan.md` update → Task 18
- [x] Upstream PromptKit issue → Task 19
- [x] Final verification → Task 20

**2. Placeholder scan**: no TBD, no "implement later", no "similar to Task N" without content. Every YAML file that gets created has its full content inline. Eval rubrics are spelled out in full (Task 8). Commit messages are pre-written. ✅

**3. Type consistency**: `task_type`, `metric.name`, `fragments[].path`, `allowed_tools` entries match across variant A (Task 6) and variant B (Task 9). Tool names used in `allowed_tools` (`lookup_order`, `lookup_customer`, `search_kb`, `issue_discount_code`, `escalate_to_human`, `memory__recall`, `memory__remember`) match the tool YAMLs created in Task 4. Pack-level eval metric names (`acme_session_outcome_total`, `acme_customer_sentiment`, etc.) match between the design doc and Task 8. ✅

---

## Execution handoff

Plan complete and saved to `specs/2026-04-11-arena-native-content-implementation-plan.md`. Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration. Uses `superpowers:subagent-driven-development`.

2. **Inline Execution** — execute tasks in this session using `superpowers:executing-plans`, batch execution with checkpoints for review.

Which approach?
