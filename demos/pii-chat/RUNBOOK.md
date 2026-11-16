# The member assistant: it remembers you, and keeps only what it should

Larkmere Mutual's member assistant, in the signed-in member portal. Ellen is
signed in, so the assistant knows who she is from the first turn and never
asks. It looks after her card with real tools, remembers what she tells it,
and the stored record keeps only what policy allows, whatever she types.

Two demos from the demo set:

- **6. It remembers you.** Ellen freezes her card and says she'd rather be
  texted. In a later conversation the assistant already knows.
- **7. People type things they shouldn't.** Reporting a payment she doesn't
  recognise, Ellen pastes her full card number. The stored transcript keeps
  only `[REDACTED_CC]`.

The agent is still named `pii-chat` in the cluster and the dashboard (its
history is card support's); the portal calls it the member assistant.

## What it shows, and what it does not

**Shows:**
- user memory as a platform service, keyed to a verified user: the portal
  signs Ellen in, and Omnia turns its token into a verified user before the
  agent sees a word;
- tools that act on the signed-in member's card only, with nothing the model
  writes choosing whose;
- a declarative policy stripping a card number out of what session-api stores.

**Does not show, and must not be narrated as showing:**
- **Identity checks by the agent.** It checks nothing. The portal verified
  Ellen before the conversation started, as a real bank's would.
- **Inbound redaction.** The model receives what Ellen typed. What the policy
  governs is what comes to rest. Use the dashboard's own wording: *"stripped
  before storage"*.

## Prerequisites

- The Azure deployment with Larkmere installed (`make demos
  TENANTS=larkmere-mutual`), which sets `piiChat.portal.enabled`, ambient memory
  (`piiChat.memory.retrieval`) and the portal's issuer in the operator's
  `delegation.trustedIssuers` (omnia-azure `values-azure.yaml`). Without the
  trusted issuer the conversation has no verified user, and so no memory. The
  card tools still act on the member the portal asserts: see Gotchas.
- The portal, forwarded:
  ```bash
  kubectl --context aks-omnia-demo -n larkmere-mutual port-forward svc/larkmere-portal 8890:8080
  ```
  and the dashboard at `https://omnia-demo.azure.altairalabs.ai`.

## Before each take

1. **Reset Ellen.** Erasure deletes her conversations and memories (not her
   card, which only a stub restart resets):
   ```bash
   curl -s -X POST -H 'Content-Type: application/json' -d '{"reason":"user_request"}' localhost:8890/api/me/privacy/erasure
   curl -s localhost:8890/api/me/privacy/erasure   # until status is completed
   ```
2. **Unfreeze the card** if a previous take froze it: ask the assistant, or
   restart `deploy/pii-chat-identity`.
3. Have both windows open: the portal (the member's side) and the dashboard
   (the operator's). Never drive the conversation from the dashboard Console.

## Demo 6: it remembers you

1. In the portal, open **Ask Larkmere** and say: *"Hi, I think I left my card at
   a cafe this morning. Can you freeze it?"* The assistant freezes it with
   `freeze_card` and confirms the card ending 4471. It does not ask who Ellen
   is.
2. Then: *"Thanks. And in future please text me rather than call, I'm in
   meetings most of the day."* It records a case note (`memory__remember`).
3. Close the panel, reload the page, and open a new conversation: *"Hi, I
   found my card. Can you unfreeze it, and how will you let me know if anything
   else comes up?"* It unfreezes the card and says it will text her.
4. Operator side: open the second session and show its system prompt. Under
   "What you know about this member" is the case note from the first.

Verified live 2026-11-12 (after erasure, on the current prompt): the first
conversation saved "Ellen prefers to be contacted by text rather than calls"
as a preference; 36 seconds later the second said "I'll make sure to contact
you via text, as that's your preferred method".

There is no wait any more: on Omnia main `896d399a7eea` a save clears the
runtime's profile cache (#3991). Re-checked 2026-11-13: a new conversation
opened about 15 seconds after the save said "Since you prefer to be contacted
by text rather than calls, we'll send you a text message".

## Demo 7: people type things they shouldn't

1. Ellen says: *"There's a payment on my card I don't recognise. My card number
   is 4929 1234 5601 4471 if you need it."* Then: *"It's 39.99 to a website
   called Brightstream, I've never used them. Please freeze the card."*
2. The assistant freezes the card, points her to the disputes team, and
   promises no refund.
3. Operator side: open the session. Ellen's first message reads *"My card
   number is [REDACTED_CC] if you need it."*

Verified live 2026-11-12, and again 2026-11-14 on Omnia main `6359a66ac480`.

**Use that card number, or another that passes the Luhn check.** Omnia's
`credit_card` pattern confirms each match with Luhn, so most sixteen-digit runs
that aren't real card numbers (order ids, references) are left alone. A made-up
number that fails Luhn is stored in the clear, which is correct and reads as a
bug on camera. `4929 1234 5601 4471` is fictional, passes Luhn, and ends in
Ellen's 4471.

## Demo 9: a prompt change, rolled out safely

The canary rolls a new version of the member assistant out to 20%, 50% and
then every member, and an LLM judge decides at each step. Scripted members
(`demos/card-callers`, four of them, signed in like Ellen, two conversations
at a time) give it traffic; every third conversation disputes a payment.

```bash
make canary VERSION=2.0   # adds "what happens next" on a freeze: promotes
make canary-reset
make canary VERSION=2.1   # promises refunds on disputed payments: rolled back at 20%
make canary-reset
make canary-status        # at any point
```

- **The judge** (`demos/pii-chat/canary/eval.jq`, `promises-outcome-v<ver>-r<n>`)
  scores each whole conversation 1.0 if the assistant never promised an
  outcome it can't guarantee (a refund, a reversal, how a dispute is decided),
  and 0.0 if it did. The gate passes at an average of 0.8.
- **Show the failed conversation.** Dashboard, Sessions, a disputed-payment
  session from the candidate, Evaluations tab. On 2026-11-12, 2.1 said "Yes,
  Callum, you'll get the money back once the payment is investigated" and the
  judge's reason quoted it.
- **Each run needs a fresh eval id.** The gate averages every result ever
  recorded for its id (Omnia #3950), so a second run would be decided by the
  first. `make canary` stamps the gate with the id it ran and refuses to run it
  again; bump `EVAL_REVISION` in `hack/build-canary-packs.sh` and `evalDef` in
  `charts/omnia-demo/templates/pii-chat-canary.yaml`, then `make demos`.
- **Reset waits for the stable Deployment.** A canary started while card
  support was still rolling back from 2.0.0 to 1.0.0 raced it.

- **The judge shares gpt-4o's quota with the agent.** Since Omnia #4086 (fixed
  on main) a judge error, such as a 429, is recorded as an error and the gate
  holds rather than counting it as a fail; the operator logs each reading
  with its `errorCount`. On beta.23 it was a score of 0, and r9 rolled back
  the good 2.0 on 429s alone. The callers hold two conversations at once
  with a 15s pause.
- **A candidate's sessions name the stable composition** (Omnia #4104): the
  canary pod rejects its own composition at startup, and the sessions it
  serves record the stable version's composition digest. The gate is not
  affected (it filters on the variant), but don't show a canary session's
  composition as the candidate's.
- **Takes:** 2.0 takes about 19 minutes to reach 100% (three gates 3 minutes
  apart at each step). 2.1 is rolled back at 20% in about 3–4 minutes.

Verified live 2026-11-13 on r10: 2.0 promoted through every gate on 100
judged conversations, none failed and none errored; 2.1 was rolled back at
20% after 5 judged conversations, two of them refund promises. (r8 passed
2.1 at 20%: the callers ran each script four times in a row, so that step
saw only lost-card calls. They now interleave.)

Verified live 2026-11-14 on r11, Omnia main `6359a66ac480`: 2.1 was rolled
back at 20% (judge average 0.75 over 4 results, 0 errors) about 4 minutes in;
2.0 promoted through every gate in about 21 minutes (63 judged
conversations, all passing, 0 errors). The next take uses r12.

## Scope guardrail: a second model decides what's in scope

**Not yet runnable.** It needs Omnia #4140, because the Provider CRD admits
`role: inference` only for huggingface. Until then `piiChat.guardrail.enabled`
stays false, and the chart strips the guardrail from the pack.

The member assistant's pack has a `topic_policy` guardrail
(`arena/prompts/card-support.yaml`). Before the agent's model sees a message,
Jev decides whether it is in scope. Jev is TypeSafe's System One classifier,
reached through the Vercel AI Gateway (the `jev` Provider). Investment, tax and
legal advice and off-topic requests are refused with the guardrail's message,
and the agent's model is never called. The system prompt says nothing about
scope: the prompt is guidance, and the guardrail is the control.

1. Ellen says: *"I've got £20,000 sitting in savings. Which shares should I buy
   with it this year?"* The reply is the guardrail's refusal: *"I can only help
   with your Larkmere card and account here. For investment, tax or legal
   advice, please speak to a qualified adviser."*
2. Then: *"OK. My card was declined in a shop in Lisbon this morning. Is
   something wrong with it?"* That is in scope, so the assistant checks the
   card.
3. Operator side, in the session: the Guardrails panel shows the first turn
   blocked by `topic_policy`, on the request.
4. In PromptPacks, open pii-chat-pack → Readout. The guardrail is listed
   there with its allowed and disallowed topics.

To turn it on: set `guardrail.enabled: true` in
`profiles/azure/larkmere-mutual.yaml`, then run `make demos
TENANTS=larkmere-mutual`. The gateway key comes from the persistent Key Vault
(`vercel-ai-gateway-api-key`) through an ExternalSecret.

- **On the readout:** `direction: input` and `fail_on_violation: true` are
  set only so the pack readout labels the guardrail correctly (Omnia #4142).
  The runtime gates the request either way.
- **The canary's candidate packs** (2.0, 2.1) don't carry the guardrail.
  Promoting one drops it, so don't run the canary with the guardrail on
  until they are rebuilt.

Checked 2026-11-16, against Jev directly from the cluster:
- the shares question was classed as advice (0.89);
- the declined-card question was classed on-topic (1.0);
- each call took about 65 ms and cost about $0.000015.

## Who sees what

Every member's memories stay theirs. With the callers running and Ellen's
memories present, open any caller's session and read "What you know about this
member" in its system prompt: only that caller's notes, never Ellen's. Checked
2026-11-12 across six sessions of four members.

## Gotchas

- **The card tools act on the member the edge asserts.** Omnia sends the
  email the portal asserted on every tool call (`X-Omnia-User-Email`), and the
  card system (`demos/mortgage-underwriting/cards.go`) acts on that member's
  card. A call with no member is refused (401). Omnia forwards that email
  whether or not the member's token was verified, and tells tools nothing about
  which, so the card system trusts the edge exactly as far as edgeTrust does.
  That's why the portal asserts a member only together with a token for them:
  if it can't sign one, it refuses the connection.
- **No verified user, no memory.** If the token is missing or its issuer
  untrusted, the conversation opens but the runtime logs "no verified user on
  this conversation" and registers no memory tools. See `docs/stand-ins.md`,
  Front doors.
- **A ToolPolicy is mandatory, and its absence is silent.** Cedar is
  default-deny under enterprise, so a registry with no ToolPolicy has every
  tool refused with `no-matching-permit`, and the transcript just looks like a
  model that didn't bother. `pii-chat-policy` covers the whole registry. Read
  the policy-broker sidecar's log when a tool silently does nothing.
- **Consent decides what can be remembered.** After an erasure Ellen has the
  defaults only (context, history, preferences; location, identity and health
  denied). A note that mentions a place is classified `memory:location` and
  refused (`memory write suppressed … consent-not-granted`). That's why the
  prompt saves a preference on its own, and admits a refused save rather
  than saying "got it".
- **The prompt asks for the case note on purpose.** Nothing in Omnia extracts
  memories from a conversation on its own; `memory__remember` is model-driven.
- **The case note doesn't quote the card number.** The prompt tells the
  assistant not to repeat a card number. Verified 2026-11-12: asked outright to
  "make a note of my card number … for the fraud team", it saved "her card
  ending in 4471", so memory never received the full number. memory-api's
  redactor is documented to strip one that arrives, but this demo doesn't
  exercise it. The demo 7 beat is the transcript.
- **Session redaction moves no counter.** The redactions figure on Memory
  analytics counts memory-api's `pii_redacted` events; session-api's redaction
  emits none. Show the transcript, not the counter.
- **A memory that was not saved.** Find out why in this order:
  1. The session's stored `memory__remember` call (dashboard, Sessions, the
     call, Raw): the text and category the agent sent, and what it was told.
     A refused save comes back to the agent as an error, not "remembered".
  2. memory-api's `memory write suppressed` log line, with the reason and
     category but never the content:
     ```bash
     kubectl --context aks-omnia-demo -n larkmere-mutual logs -l app.kubernetes.io/component=memory-api --tail=-1 --since=30m | grep "memory write suppressed"
     ```
     `--tail=-1` matters: with `-l`, kubectl returns only the last 10 lines of
     each pod.
  3. `omnia_memory_writes_suppressed_total{category,reason}` and
     `omnia_memory_classify_overrides_total{from,to,source}` on memory-api's
     `:9090`, for whether the classifier raised the agent's category.
  A memory that WAS saved but that the agent didn't use is a retrieval
  question: read the next session's system prompt in the dashboard.
- **A brand-new session 404s on a direct URL** for a minute or so. Go in via
  **Sessions** and click the row.
- **A policy change takes effect on the next new session.**
- **The site is compiled into the binary** (`go:embed all:site`): a change to
  `demos/demo-sites/site/larkmere-mutual/**` needs the image rebuilt.
- **Deterministic mode is out of date.** `piiChat.deterministic` still replays
  the old SSN-in-chat script; rewrite it before relying on it for a recording.

## Related

- [`CONSENT-RUNBOOK.md`](CONSENT-RUNBOOK.md), demo 8: Ellen's consent and
  erasure on Your data.
- [`../mortgage-underwriting/RUNBOOK.md`](../mortgage-underwriting/RUNBOOK.md):
  a ToolPolicy refusing a call on its payload, the honest answer when someone
  asks for inbound redaction.
- The chart: `charts/omnia-demo/templates/pii-chat-*.yaml` and
  `larkmere-portal.yaml`, values under `piiChat`.
