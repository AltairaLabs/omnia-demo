# PII redaction — card support chat

A support agent is given a Social Security number, passes it to a tool that
legitimately needs it, and files a case note. The conversation streams to the
browser in the clear. What is **kept** — the transcript, the tool call's
arguments, the memory — is governed by policy.

Five beats, five different outcomes for the same conversation.

## What it shows, and what it does not

**Shows:** a declarative policy stripping PII out of what session-api stores;
the same policy reaching inside a tool call's JSON arguments; a memory redactor
that keeps an email but never an SSN, depending on why the data is being held;
a consent gate that refuses a memory write outright; and an EU AI Act Art. 50(1)
disclosure declared by the pack, delivered on the wire, and recorded on the
session.

**Does not show, and must not be narrated as showing: inbound redaction.**
Omnia has no control that strips PII before the model. The user's text reaches
the runtime intact and the SSN is dispatched to the tool intact. What the policy
governs is what comes to rest. Use the dashboard's own wording — *"stripped
before storage"*.

## Prerequisites

- An Omnia install with `enterprise.enabled=true`. `SessionPrivacyPolicy` is an
  enterprise CRD and both privacy middlewares are gated on `--enterprise`. A
  licence is **not** required — the check logs a reminder and never blocks.
  The chart renders the beta.22 policy shape (`recording.pii` only; Omnia
  0.9.0-beta.22 removed `recording.enabled` and `auditLog`).
- session-api and memory-api running **in-cluster**; both skip their privacy
  middleware silently without an in-cluster kubeconfig. Confirm before recording
  (the workspace's managed services are named `<service>-<workspace>-<group>`):
  ```bash
  kubectl --context <ctx> logs -n <ns> -l app.kubernetes.io/name=session-<workspace>-default | grep "privacy middleware"
  kubectl --context <ctx> logs -n <ns> -l app.kubernetes.io/name=memory-<workspace>-default | grep "privacy middleware"
  ```
  You want `privacy middleware enabled` from both.
- **No embedding provider is needed.** The agent names each memory's consent
  category itself (`memory__remember`'s `category`), and memory-api's
  classifier can only raise it to a stricter one. Without an embedding Provider
  that classifier is a keyword list (memory-api logs `consent classifier
  running in regex-only mode`); with one, as on Azure, it also compares each
  memory with example sentences per category.
- For live mode, an `anthropic-api-key` Secret in the demo namespace
  (`piiChat.provider.secretRef`), or `llm.providerRef` naming a Provider that
  already exists there. Deterministic mode needs neither.

## Install

```bash
helm upgrade --install omnia-demo charts/omnia-demo \
  --namespace <ns> \
  --set namespace=<ns> \
  --set piiChat.enabled=true \
  --set piiChat.portal.enabled=true \
  --set piiChat.deterministic=true    # omit for the live model
```

`piiChat.portal.enabled` deploys the Larkmere member portal and gives the agent
`externalAuth.edgeTrust`, so the portal can proxy the conversation and assert
the member's identity. `piiChat.memory.enabled` (on by default) adds the memory
step; the deterministic script calls `memory__recall` and `memory__remember`,
so leave it on when capturing deterministic.

Forward the two surfaces:

```bash
kubectl --context <ctx> -n <ns> port-forward svc/larkmere-portal 8890:8080
# and the Omnia dashboard, however your install exposes it, on :3000
```

## Before each take

Two surfaces, two windows. **localhost:8890** is the member's — the Larkmere
Mutual portal, the customer-facing side. **localhost:3000** is the operator's —
the Omnia dashboard. The whole argument of this demo is the cut between them,
so have both open before you start and never drive the conversation from the
dashboard Console: filming a cardholder typing a Social Security number into
admin tooling quietly says customers have access to your ops console.

Reset between takes. Every rehearsal leaves a session and a memory behind and
both are on camera. After four practice runs the Memory Galaxy showed five
identical dots — which reads as a system that has been poked at, not a member's
first call. Delete the demo agents' sessions and the demo workspace's memories
(and nothing else) before each take, then reload `/memories` once; the
projection is cached.

Then deep-link the operator side rather than hunting for the row on camera. You
drove the conversation in the browser, so nothing printed the id — take it
from the newest session in the session database:

```bash
kubectl --context <ctx> exec -n <postgres-ns> <postgres-pod> -- \
  psql -U omnia -d omnia_sessions -t -A \
  -c "SELECT id FROM sessions WHERE agent_name = 'pii-chat' ORDER BY created_at DESC LIMIT 1;"
```

```
http://localhost:3000/sessions/<session-id>
```

If that 404s, go in via **Sessions** and click the top row — see Gotchas. It
resolved immediately every time on the last pass, but it has not always.

## The five beats (≈2½ minutes)

1. **The conversation, in the member's portal (≈40s).** Open
   **http://localhost:8890**, click **Ask Larkmere** bottom-right. Three
   exchanges:

   - Click the **I've lost my card** opener. No personal data in it.
   - The agent asks for full name, date of birth and Social Security number.
     **Type the answer** — the member's bubble shows `123-45-6789` in the clear,
     and the agent verifies and cancels the card:

     > Ellen Whitcomb, ellen.whitcomb@example.com — my SSN is 123-45-6789 and I
     > was born 1988-03-14.

   - **Type** a sign-off — "That's everything, thanks." The agent files the case
     note and says so: *"I've made a note on your file so the next person you
     speak to has the background."*

   There are deliberately **no chips for those last two**. Both existed and both
   were cut: a portal that offers to type your Social Security number for you is
   not a thing any real product does, and a canned mid-conversation reply reads
   as staged the moment it is on camera. Suggested openers are a real pattern
   and stay. Type the rest — watching a person put an SSN into a chat box is the
   thing the whole demo is about.

   **The agent asking is the point, not staging.** A demo where the member
   volunteers a Social Security number into a chat box unprompted shows the
   wrong thing, and an agent that verified someone it had never asked to
   identify themselves would show something worse.

   **The note is filed at the END, on its own turn**, for the same reason: that
   is when a support agent writes one, it cannot summarise a call before the
   call is over, and it makes beat 4 a moment the audience watches happen rather
   than something that already went past while they were reading the
   verification reply.

   Say out loud that the stream is not redacted. It isn't, and pretending
   otherwise makes the next beat dishonest.

2. **The transcript (≈30s).** Open the session under Sessions. The message reads
   `[REDACTED_SSN]` and `[REDACTED_EMAIL]`. The token names the *kind* of value
   removed, so the record alone says an SSN was here and the policy took it.

3. **The tool call (≈30s).** Click the `verify_identity` row in the transcript,
   then the **Tool** tab in the panel that opens. Under `ARGS`:
   `"ssn": "[REDACTED_SSN]"`, while `"dob": "1988-03-14"` survives untouched.
   (**Raw** has the same thing inside the whole record; **Tool** is the tighter
   shot.) Different endpoint, same policy, no extra configuration — and
   redaction removes PII rather than blanking the argument set.

   Worth saying out loud: this one was broken until this demo was built. The
   redactor asserted the field to a string and returned silently when it wasn't,
   so object-valued `arguments` were stored in the clear.

4. **The memory (≈40s).** The agent filed the case note on the last turn of the
   call, and said so on screen — so this beat is following up on something the
   member was told, not revealing something that happened behind their back.
   Two things to show:

   The Galaxy (`/memories`) shows the dot arriving under **User-for-agent**, but
   it masks the content (Omnia #1643), so read the text itself from Postgres:

   ```bash
   kubectl --context <ctx> exec -n <postgres-ns> <postgres-pod> -- \
     psql -U omnia -d omnia_memory -t \
     -c "SELECT content FROM memory_observations;"
   ```

   **What it keeps and what it drops.** The stored memory holds
   `ellen.whitcomb@example.com` but never the SSN. That is not a pattern list — it
   is provenance. `memory__remember` tags every write `user_requested`, which
   maps to `TrustExplicit`, and `filterPatternsByTrust` then drops the
   *non-structural* patterns (email, phone) so the memory stays useful, while
   SSN, card and IP are scrubbed regardless of who asked. Contrast it with beat
   2, where the same email *was* redacted: same value, different reason for
   holding it, different answer.

   **The consent gate.** Re-run the same conversation without the
   `memory:identity` grant and nothing is stored at all:
   ```
   omnia_memory_writes_suppressed_total{category="memory:identity",
                                        layer="persistent",
                                        reason="consent-not-granted"} 1
   omnia_audit_events_total{event_type="memory_write_blocked"} 1
   ```
   The labels narrate themselves: the category the classifier assigned by
   reading the content, the layer it was refused at, and why.

5. **The AI disclosure (≈20s).** Cut back to the portal. Under the composer,
   beside an **AI** badge: *"You are chatting with an AI assistant, not a
   person."* That line is not hardcoded in the page — the panel renders it only
   because `connected.ai_disclosure` arrived on the first frame of the socket.
   Delete the declaration from the pack and the notice disappears.

   That is the part worth showing, because a delivery obligation that nothing
   delivers is just a column asserting something nobody received.

   The pack declares `requires_ai_disclosure: true` in its RFC 0013 governance
   block, because a customer-facing chat bot is the textbook EU AI Act Art.
   50(1) case. Then show the chain:

   ```
   demos/pii-chat/arena/governance-declarations.yaml   requires_ai_disclosure: true
     -> merged into pack metadata.governance by hack/build-packs.sh
     -> facade reads it at startup (Omnia's internal/promptpack.ReadAIDisclosure)
     -> first frame on the wire:  "connected": { "ai_disclosure": true }
     -> recorded on the session:  ai_disclosure_declared = t, delivered = t
   ```

   A declaration by the pack's author, delivered to the client before the first
   turn, and written to a row someone can audit later. Pair it with
   `mortgage-underwriting`, which declares the same field **false** — deliberately,
   because that agent is backend and never speaks to a person. Same field,
   opposite value; the obligation follows from who the agent talks to.

   Declared rather than omitted, on both. An absent value means *undeclared*,
   which is weaker and different from "considered, and not required" — Omnia
   keeps those apart with a pointer-typed field
   (Omnia's `internal/promptpack/governance.go`).

## Live vs deterministic — they show different true things

This distinction matters more than it looks, and getting it wrong on camera
would be dishonest.

**Measured on Haiku 4.5, three runs of the full three-exchange conversation**
(`piiChat.deterministic=false`, `piiChat.provider.model=claude-haiku-4-5-20251001`):

| | live, 3 runs | deterministic |
|---|---|---|
| Asks for identification unprompted | **3/3** | always |
| Calls `verify_identity` | **3/3** | always |
| Calls `memory__recall` first | **0/3** | always |
| Files a case note at the end | **1/3** | always |
| SSN in the case note when written | **0/1** | always |
| Redaction of message + tool args | identical | identical |

What that means for a recording:

- **Beats 1, 2, 3 and 5 are safe live.** The premise holds without a script: a
  real model does ask for the SSN rather than being handed one, it does put it
  in a tool call, and redaction is policy so it does not care who wrote the turn.
- **Beat 4 is not.** The case note landed once in three, and the model never
  opened with `memory__recall`. **Record beat 4 deterministic**, or be ready to
  re-take it.
- When the live model *did* write a note it wrote *"Contact:
  ellen.whitcomb@example.com"* and no SSN at all — the fact, not the value.

**Memory redaction is a backstop for an agent that writes something it
shouldn't, and a well-behaved model often won't.** That is exactly why the
control exists: you cannot rely on the model's discretion. Narrating a scripted
mistake as "watch the policy catch this" is fine. Implying that live models
routinely dump SSNs into memory is not.

Two live-only cosmetics, neither fatal: the model writes markdown (**4471**),
which the panel renders literally, and it invents its own quantities — "7-10
business days" where the script says three to five.

## The conversation

Three exchanges. The member says what is wrong; the agent asks for
identification; the member answers; the member closes the call and the agent
files its note:

> **Member** — Hi, I've lost my card.
>
> **Agent** — I'm sorry to hear that … First I need to verify you. Could you
> give me your full name, your date of birth and your Social Security number?
>
> **Member** — Ellen Whitcomb, ellen.whitcomb@example.com — my SSN is
> 123-45-6789 and I was born 1988-03-14.
>
> **Agent** — Thank you, Ellen — you're verified … I've cancelled it and
> ordered a replacement …
>
> **Member** — That's everything, thanks.
>
> **Agent** — Of course. I've made a note on your file so the next person you
> speak to has the background …

One tool call per exchange, in the order the call needs them: `memory__recall`
(do we know this member?), then `verify_identity`, then `memory__remember`. The
note is last because it summarises the call.

`123-45-6789` is the SSN Omnia's `test/e2e/privacy_e2e_test.go` already asserts
on. The script is `charts/omnia-demo/files/pii-chat-script.yaml`.

### Why turn 1 is a tool call

The turn index is not "the nth thing the user said". PromptKit derives it from
the accumulated assistant-message count and clamps to 1 on the first call, so
with **text-only** turns the clamp collides and turn 1 is served twice. Measured
with a probe whose every turn returned its own number:

```
user msg 1  ->  TURN-1
user msg 2  ->  TURN-1     <- served twice
user msg 3  ->  TURN-2
```

That is why an earlier cut of this demo could not have the agent ask: turn 1
asking would ask *again* as the reply to the answer.

A **tool call** breaks the collision, because the tool round-trip appends its
own messages and the counter advances past the clamp inside one exchange. Same
probe, with tool calls on 1, 3 and 5:

```
user msg 1  ->  TURN-2     (turn 1 was the tool call)
user msg 2  ->  TURN-4     (turn 3 was the tool call)
user msg 3  ->  TURN-6     (turn 5 was the tool call)
user msg 4  ->  TURN-7
```

So each exchange consumes two turns and its reply lands on the even one, and
nothing repeats. That is what makes a three-exchange conversation possible at
all — and it is why every exchange here opens with a tool call.

`memory__recall` on turn 1 is not filler to make the arithmetic work. It is what
a support agent does first: check whether we already know this member. On a
clean demo it correctly finds nothing, which is *why* the agent has to ask.

If you change the shape, **measure**. Reasoning "+2" from an observed number has
been wrong here twice.

## Defaults chosen for the camera

- **`strategy: replace`.** `[REDACTED_SSN]` names the kind of value removed;
  `mask` shows that something went but not what, and `*******6789` beside
  `*******4471` is two anonymous runs of stars. `replace` also matches the
  Omnia install's global default policy, which matters because memory-api resolves its
  policy with an **empty agent name** — a per-agent `privacyPolicyRef` never
  applies there, so memory falls through to that global default. With both on
  `replace`, beats 2 and 4 render identically.
- **`phone_number` is deliberately absent** from the default patterns. Its regex
  matches any bare ten-digit run, so a case number or order ID would be eaten
  too — which reads as a bug rather than a policy.

## Gotchas

### The portal

- **Forward it to :8890, not :8888.** In Omnia's local dev loop
  `omnia-vscode-server` already forwards 8888, and a second forward to the same
  port silently serves the other workload. The symptom is a 404 and a redirect
  to `?folder=/workspace`.
- **The portal reaches the agent at `http://pii-chat.<ns>:8080`** (its
  `--agent-url`), the facade's external listener. Without
  `externalAuth.edgeTrust` that listener rejects it, which is why the chart
  only adds edgeTrust together with the portal.
- **Documents, Retirement and Settings are empty states.** The design project
  has no screen for them, so they render a titled placeholder. They are safe to
  click (a silent bounce back to the dashboard read as a broken page), but do
  not linger on one. Your data is a real screen: see
  [CONSENT-RUNBOOK.md](CONSENT-RUNBOOK.md).
- **The site is compiled into the binary** (`go:embed all:site`), so any change
  to `demos/demo-sites/site/larkmere-mutual/**` needs the image rebuilt; there is no volume
  to edit in place. If a `.dockerignore` ever excludes the tree the failure is
  at BUILD time — `pattern all:site: cannot embed directory site: contains no
  embeddable files` — not at runtime.
- **The composer is one line**, so a filled-in message scrolls and the SSN is not
  fully visible *in the box*. It is fully visible in the member's sent bubble,
  which is the frame to cut from.

### Everything else

- **A policy change takes effect on the next NEW session.** Start a fresh
  conversation after any `helm upgrade`.
- **A ToolPolicy is mandatory, and its absence is silent.** Cedar is
  default-deny and the operator injects a policy-broker sidecar under
  enterprise, so an agent with a ToolRegistry and no ToolPolicy has every tool
  refused with `no-matching-permit`. Server tools never appear in the WebSocket
  stream, so it looks like a model that chose not to call anything. Read the
  sidecar's log, not the agent's. `pii-chat-policy` covers the whole registry.
- **`memory__remember` and `memory__recall` are PromptKit platform tools, not
  ToolRegistry entries**, so they do *not* pass through the broker. Only
  `verify_identity` does. They still have to be in the prompt's `allowed_tools`
  — that list is an allow-list, and a tool missing from it is simply never
  offered to the model.
- **The prompt asks for the case note on purpose.** Nothing in Omnia extracts
  memories from a conversation by itself — `memory__remember` is model-driven,
  document ingestion is for documents, and the consolidation worker only
  reorganises rows already in the store. Without that instruction a live model
  has no reason to write one.
- **The Memory Galaxy masks memory content** (`"masked": true` in the projection
  response, Omnia #1643), so the memory text is not readable in that view. Read it
  from Postgres for a close-up.
- **The agent knows the member from the first turn.** Ambient retrieval
  (`piiChat.memory.retrieval`) fills the prompt's `memory_context` each turn,
  and the member's preferences, identity and health memories are always in
  it. Two things follow:
  - **The profile is cached for 30 seconds per member.** A chat started
    within 30 seconds of the one that saved a memory may not know it yet.
    Leave a pause between the two calls of a "it remembers you" take.
  - **Takes depend on earlier takes.** Reset Ellen between takes by erasing
    her data: `POST /api/me/privacy/erasure` on the portal, or Your data's
    Erase button. It deletes her conversations and memories, not her card.
- **A memory that was not saved.** Find out why in this order:
  1. The session's stored `memory__remember` call (dashboard, Sessions, the
     call, Raw): the text and category the agent sent, and what it was told.
     A refused save comes back to the agent as an error, not "remembered".
  2. memory-api's `memory write suppressed` log line, with the reason and
     category but never the content:
     ```bash
     kubectl --context <ctx> -n <ns> logs -l app.kubernetes.io/component=memory-api --tail=-1 --since=30m | grep "memory write suppressed"
     ```
     `--tail=-1` matters: with `-l`, kubectl returns only the last 10 lines
     of each pod.
  3. `omnia_memory_writes_suppressed_total{category,reason}` and
     `omnia_memory_classify_overrides_total{from,to,source}` on memory-api's
     `:9090`, for whether the classifier raised the agent's category.
  A memory that WAS saved but that the agent did not use is a retrieval
  question, not a storage one: read the next session's system prompt in the
  dashboard, which shows what `memory_context` rendered.
- **`enforcement-stats` lags.** memory-api's audit events drain to privacy-api
  asynchronously (the e2e allows three minutes), and the dashboard hook
  refetches every five minutes. Local metrics on memory-api `:9090` are
  immediate.
- **A brand-new session 404s on a direct URL** for a minute or so even though
  the API has it. Go in via **Sessions** and click the row.
- **The identity service lives in `demos/mortgage-underwriting/`** and ships in
  the `omnia-demo-mortgage-underwriting` image — one binary serving the
  mortgage, claims-triage and card-support demos. The directory name is the
  cost of not maintaining a second image.
- **The stub never logs the SSN.** `jsonTool` logs `req.ref()`, empty for this
  endpoint, deliberately.

## Related

- [`../mortgage-underwriting/RUNBOOK.md`](../mortgage-underwriting/RUNBOOK.md)
  — a Cedar ToolPolicy refusing an egress on payload content. The nearest thing
  Omnia has to "do not send that to the tool", and the honest answer when
  someone asks for inbound redaction.
- Omnia's `test/e2e/privacy_e2e_test.go` — the redaction path under test.
- The chart: `charts/omnia-demo/templates/pii-chat-*.yaml` and
  `larkmere-portal.yaml`, values under `piiChat`.
