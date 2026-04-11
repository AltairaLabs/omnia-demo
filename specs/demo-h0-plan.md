# H0 Reliability Pre-flight — Implementation Plan

**Phase**: H0 (starts Day 2 or Day 3 of week 1, after the Pre-H0 verification batch completes)
**Duration**: ~1 week (4-7 engineer-days)
**Exit criterion**: 10 consecutive `MemoryPersistsAcrossSessions` doctor smoke-test runs pass 29/29 against the demo LLM (GPT-4o on Azure AI Foundry), with measured memory retrieval latency p95 < 300ms.

**Why this phase exists**: The hero demo's Scene 3 memory moment is the single most impressive beat in either demo. If memory extraction / retrieval is flaky on the chosen LLM, the demo is dead on arrival. H0 is the quality gate. No demo work proceeds until H0 passes.

---

## R1 — Confirm demo LLM reliability (~1 day)

**Context**: D1 is resolved as GPT-4o on Azure AI Foundry (assuming V1 doesn't reveal Claude-on-Azure-AI-Foundry). V3 has already proven the transport path works. R1 is about confirming the LLM itself can handle the memory workflow reliably.

### Task R1.1 — Smoke test memory__remember + memory__recall against GPT-4o Azure AI Foundry

**Goal**: Run ~20 synthetic "remember then recall" conversations against the demo LLM. Expect 100% success rate.

**Procedure**:
1. With V3's test environment still up (Azure AI Foundry Private Endpoint + Omnia `azure-ai` provider), deploy a minimal agent with `sdk.WithMemory()` wired (use the hero demo's expected config, not a stripped-down one).
2. Write a tiny test script (Go, Node, or bash+curl) that opens a WebSocket session to the facade and sends canned messages:
   - Session 1: *"My name is Alice and I prefer Python over JavaScript."* → close session
   - Session 2 (same user ID, new session): *"What's my name and language preference?"* → expect agent to recall "Alice" and "Python"
3. Run this 20 times with 20 different fake user IDs. Count successes.

**Success criterion**: 20/20 successful recalls. If anything less, investigate before proceeding.

**Expected outcome**: clean pass. GPT-4o handles `memory__remember` tool calls reliably; it omits null optional fields rather than sending them explicitly, so PromptKit#836 doesn't bite.

**If R1.1 fails**: this is a major red flag. The memory path isn't working even with a reliable LLM. Investigate the whole memory wiring (`internal/runtime/conversation.go:168-183`, memory-api connectivity, populator pipeline) before moving on.

### Task R1.2 — Verify memory tool call signature compatibility

**Goal**: Confirm the `memory__remember` tool schema (as emitted by PromptKit) round-trips cleanly through the Azure AI Foundry GPT-4o client.

**Procedure**:
1. Run one of the R1.1 sessions with `LOG_LEVEL=debug` on the runtime
2. Find the log line where PromptKit emits the `memory__remember` tool schema to the LLM (should be in the tool-call loop)
3. Copy the schema, paste it into the Azure OpenAI Studio / AI Foundry playground as a function definition, ask GPT-4o to use it with arbitrary input
4. Verify GPT-4o produces a tool call that PromptKit's `CoerceArgs` accepts cleanly

**Why this matters**: PromptKit#836 is specifically about `CoerceArgs` rejecting null values in optional fields. Different LLM providers emit nulls differently. This test confirms GPT-4o doesn't trip the bug.

**If R1.2 fails**: we hit a new variant of #836, or something specific to Azure OpenAI's function-calling format. File upstream. Decide whether to work around (strip nulls in Omnia runtime) or block on upstream fix.

---

## R2 — PromptKit extraction verification (~0.5-1 day, gated on V3)

**Context**: R2.1 was completed on 2026-04-11 and resolved the question definitively. **Omnia's `LLMConversationPopulator`, `ConversationPopulator`, and `OmniaExtractor` are orphaned code** — instantiated only in test files, never wired into any production binary (`cmd/memory-api/main.go`, `cmd/runtime/main.go`, `cmd/main.go`, `cmd/agent/main.go`, `cmd/doctor/main.go`, `ee/cmd/*`). Memory extraction in production is handled entirely by **PromptKit's built-in pipeline** via `sdk.WithMemory()` at `internal/runtime/conversation.go:176`, with the runtime's configured LLM doing the extraction work. Omnia's role is "give PromptKit a `pkmemory.Store` to persist into."

This is **good news for R2** — the complexity the original plan anticipated doesn't exist. But it shifts R2.2 from "test Omnia's populators" to "verify PromptKit's extraction pipeline produces demo-quality entities when pointed at GPT-4o via Azure OpenAI."

### Task R2.1 — Code investigation (DONE, 2026-04-11)

**Finding**: Omnia-side populators are dead code. See §"R2 context" above. Evidence trail:
- `grep LLMConversationPopulator` → only test files reference it
- `grep NewConversationPopulator` → only test files reference it
- `grep NewOmniaExtractor` → only test files reference it
- `cmd/memory-api/main.go` → no extraction code, only CRUD over `memory.NewPostgresMemoryStore(pool)`
- `internal/runtime/server.go:100` → `memoryStore pkmemory.Store` — typed as PromptKit's interface, not Omnia's
- `internal/runtime/conversation.go:176` → `sdk.WithMemory(s.memoryStore, scope)` — PromptKit SDK receives the store and handles extraction internally

**Files to delete (separate cleanup PR, NOT in demo scope)**:
- `internal/memory/populator_llm.go` + `populator_llm_test.go`
- `internal/memory/populator_conversation.go` + `populator_conversation_test.go`
- `internal/memory/populator.go` (interface)
- `internal/memory/extractor.go` + `extractor_test.go`
- `internal/memory/integration_test.go` — needs adjustment (exercises orphaned extraction path; coverage should move to end-to-end via a real agent session, or be dropped)
- `internal/memory/embedding_integration_test.go` — same; adjust to use direct store API

File as a backlog item. Do not block the demo on it.

**Also update** `docs/local-backlog/agentic-memory-remaining.md` line 71 — current status "Code exists, wiring needs verification" should become "**Orphaned — verified 2026-04-11**. PromptKit's built-in extraction is the production path; Omnia's `populator_llm.go` was pre-PromptKit legacy. File for deletion in a separate cleanup PR after the demo ships."

### Task R2.2 — PromptKit extraction quality test (gated on V3, ~0.5 day)

**Goal**: With the Azure AI Foundry / Azure OpenAI Private Endpoint environment from V3 up, run a realistic support conversation through a real agent session; confirm PromptKit's extraction produces clean semantic entities.

**Procedure**:
1. Using the R1.1 test harness (or extending it), run a multi-turn conversation against the GPT-4o-on-Azure agent:
   - Turn 1: *"Hi, I'm Sarah Chen at sarah@example.com. My order A-1234 — a linen dress I needed for a birthday party this weekend — never arrived and it's been 10 days. I'm really stressed."*
   - Turn 2 (agent calls `lookup_order`, responds with info, offers voucher)
   - Turn 3: *"Thank you, the EXPRESS_UPGRADE voucher arrived and the dress got here in time. Really appreciate it."*
2. Close the session.
3. Wait for any async extraction to complete (~10-30 seconds).
4. Query memory-api directly (`GET /memories?user_id=<sarah-pseudonymized>`) for the user's entities.
5. Verify the extracted entities include at minimum: customer name, email, order ID, issue (delayed shipment), resolution (voucher issued). Sentiment transition (stressed → relieved) is a bonus.

**Success criterion**: clean semantic entities, not raw conversation chunks. If the memory-api returns proper typed entities, we're good — Scene 3's "What the agent remembered" panel will look great in the demo.

**If R2.2 reveals PromptKit extraction is too coarse (raw chunks instead of semantic entities)**: this is a real gap. Options:
1. Configure PromptKit's extraction more aggressively via `sdk.WithMemory` options (check PromptKit SDK docs for extraction tuning)
2. Use PromptKit's `memory__remember` tool path more explicitly — add a system prompt instruction to the PromptPack: *"After resolving a customer issue, call `memory__remember` with key customer context so you can pick up where you left off next time."*
3. Add a custom extraction step as an Omnia middleware around PromptKit — more work, outside the demo scope

**Expected outcome**: GPT-4o + PromptKit's extraction should produce good entities. This is a well-trodden path. R2.2 is a sanity check, not an expected failure point.

### R2 exit criterion

- [x] R2.1: Orphaned-code status documented, cleanup filed as backlog item
- [ ] R2.2: Real agent session with GPT-4o produces clean semantic entities (gated on V3)
- [ ] `docs/local-backlog/agentic-memory-remaining.md` line 71 updated

---

## R3 — Doctor reliability (2-3 days)

**Context**: `internal/doctor/checks/memory.go` has the `MemoryPersistsAcrossSessions` check. Currently flaky (26-29/29 overall per `agentic-memory-remaining.md`). Need 29/29 reliable.

### Task R3.1 — Point doctor at the demo LLM

**Goal**: Configure the doctor smoke test to run against the chosen demo LLM (GPT-4o on Azure AI Foundry), not the default dev Ollama.

**Procedure**:
1. Read `internal/doctor/checks/memory.go` + related doctor infrastructure
2. Figure out how to point the doctor at a specific Provider CRD / AgentRuntime
3. Create a doctor-specific config that uses the Azure AI Foundry Provider from V3

**If the doctor is hardcoded to Ollama**: small refactor needed to parameterize the provider. ~0.5 day. File as a separate item if it grows beyond that.

### Task R3.2 — Run 10 consecutive passes

**Goal**: Run `MemoryPersistsAcrossSessions` 10 times in a row. Expect 10/10 passes of 29/29 checks.

**Procedure**:
```bash
for i in {1..10}; do
  echo "Run $i:"
  # command to invoke the doctor against the demo Provider
  # e.g.: go run ./cmd/doctor --provider=azure-ai-foundry-demo --namespace=demo
done
```

**If any run fails**: investigate the specific failure. Common causes:
- Session timing (stale session selected) — fix the session selection logic
- LLM doesn't always call memory__remember — prompt tuning in the test's PromptPack
- Memory extraction race (session closes before populator finishes) — increase the wait window
- Memory retrieval race (next session opens before indexing completes) — fix the retrieval side

**Each failure class is 0.5-1 day to fix.** Budget for 2-3 iterations.

### Task R3.3 — Gate the demo on R3

**Goal**: Formalize "do not start H1 until R3 passes" with a recorded evidence artifact.

**Procedure**:
1. Capture 10 successful runs' output as `docs/local-backlog/notes/h0-r3-evidence.txt`
2. Note the Omnia git SHA + the demo LLM + the PromptKit SDK version used
3. If any subsequent PR during H1+ breaks this, reproduce R3 before merging

This is the canary. Protect it.

---

## R4 — Memory retrieval latency (~1 day)

**Context**: Scene 3 of the hero demo depends on memory retrieval feeling instant when Sarah's second session opens. If retrieval takes 1-2 seconds, the demo feels laggy and the story falters. Budget: **p95 < 300ms** at session start.

### Task R4.1 — Find or add instrumentation

**Goal**: Verify there's an OTel span or Prometheus histogram measuring "memory retrieval at session start." If not, add one.

**Procedure**:
1. Check `internal/tracing/` for memory-related spans
2. Check the runtime's `/metrics` endpoint for memory retrieval histograms
3. Check `internal/runtime/conversation.go:168-183` for existing instrumentation around the `sdk.WithMemory()` call
4. If instrumentation exists, proceed to R4.2. If not, add a histogram — ~0.5 day.

**Most likely**: there's OTel span coverage but not a dedicated Prometheus histogram. Adding a histogram is small — `~20 lines in the runtime package.

### Task R4.2 — Measure against Azure AI Foundry

**Goal**: Run 100 sessions against the demo environment, each with pre-populated memory (from R1.2's entities), and measure retrieval latency at session start.

**Procedure**:
1. Using R1.1's test harness, seed memory for 100 synthetic users with realistic entity counts (~20-50 entities each)
2. Open a new session for each user, measure the time from session creation to "memory hydration complete" signal
3. Compute p50, p95, p99

**Success criterion**: p95 < 300ms. p99 < 500ms is acceptable.

**If p95 > 300ms**:
- Is the bottleneck the DB query? → check Postgres query plan, add an index if needed
- Is it memory-api → facade HTTP round-trip? → check connection pooling, keepalive
- Is it the embeddings lookup? → check pgvector index state, nearest-neighbor query plan
- Is it the populator running during retrieval (shouldn't be)? → fix the race

Each is 0.5-1 day to investigate + fix.

### Task R4.3 — Document the budget

**Goal**: Codify the latency budget as a test assertion.

**Procedure**:
1. Add a latency assertion to the doctor smoke test: *"memory retrieval p95 across 10 runs must be < 300ms"*
2. Document it in `docs/local-backlog/notes/h0-r4-evidence.txt` alongside R3's evidence

---

## H0 exit criterion (gate to H1)

- [ ] R1.1: 20/20 successful memory remember → recall cycles against GPT-4o Azure AI Foundry
- [ ] R1.2: GPT-4o tool-call schema compatibility confirmed
- [ ] R2.1: LLM populator status documented (wired, dead, or needs-a-flag)
- [ ] R2.2: End-to-end entity extraction produces clean semantic entities (either populator path acceptable)
- [ ] R3.2: 10 consecutive doctor `MemoryPersistsAcrossSessions` runs pass 29/29
- [ ] R3.3: Evidence artifact captured
- [ ] R4.2: Memory retrieval p95 < 300ms measured against Azure AI Foundry
- [ ] R4.3: Latency assertion added to doctor smoke test

**When all checkboxes are green**: H1 begins. Kick off SH2 (seed Shopify dev store with fixtures) and W1 (vanilla JS widget core) in parallel.

---

## What can run in parallel with H0

If there's bandwidth (or a second engineer), these are independent of H0 and can start on Day 2:

- **SH2** — seed Shopify products/customers/orders (0.5-1 day). Independent of memory work.
- **T7** — author variant A PromptPack (1-2 days). Independent of memory work, though benefits from confirming the demo LLM choice first.
- **T8** — author Acme Apparel personas as PromptArena Persona YAML (1 day). Independent; provides inputs to both demos.
- **O1** — Acme Apparel self-play scenario YAML (1-1.5 days). Depends on T7 + T8; can start as soon as those land.
- **H3.4** — Azure AKS sandbox provisioning for the hero demo (parallel ~1 week Azure resource work).

None of these are gated by the H0 exit criterion. Starting them in parallel is the main way to compress the ~7.5-10 week sequential timeline into ~6-7 weeks calendar.

---

## Risks specific to H0

| Risk | Mitigation |
|---|---|
| Rule-based populator is too dumb for demo-quality entity extraction, AND LLM populator is dead code | Build minimal LLM populator (~2-3 days). Already in `docs/local-backlog/agentic-memory-remaining.md` as a roadmap item. |
| GPT-4o on Azure AI Foundry has tool-calling quirks we haven't seen before | Fall back to GPT-4 (earlier model, better documented tool-calling) — same provider, same Private Endpoint |
| Doctor test is tied to Ollama and refactoring it to parameterize the provider is larger than expected | Write a parallel minimal test harness for R3 purposes only. The doctor's deeper refactor can come later. |
| R4 reveals p95 > 1 second — architectural problem | Unlikely but possible. Would block the demo's Scene 3. Would need a memory-retrieval performance work item before H1 can start. |
| V3 hasn't passed yet when H0 starts (Pre-H0 batch incomplete) | Block. Don't start H0 until V3 passes. Everything downstream assumes the Azure AI Foundry path works. |

---

## Time budget summary

| Task | Low | High |
|---|---|---|
| R1.1 + R1.2 | 0.5 day | 1 day |
| R2.1 + R2.2 | 1 day | 2 days |
| R3.1 + R3.2 + R3.3 | 2 days | 3 days |
| R4.1 + R4.2 + R4.3 | 0.5 day | 1.5 days |
| **Total H0** | **4 days** | **7.5 days** |

Matches the ~4-7 day budget in `demo-build-plan.md`.
