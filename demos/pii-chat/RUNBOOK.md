# The member assistant: it remembers you, and keeps only what it should

Larkmere Mutual's member assistant, in the signed-in member portal. Ellen is
signed in, so the assistant knows who she is from the first turn and never
asks. It looks after her card with real tools, remembers what she tells it,
and the stored record keeps only what policy allows, whatever she types.

Two demos from the demo set (`docs/superpowers/specs/2026-11-12-demo-set-design.md`):

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
3. **Wait at least 30 seconds** (the runtime caches a member's profile for 30
   seconds, Omnia #3991), close the panel, and open a new conversation: *"Hi, I
   found my card. Can you unfreeze it, and how will you let me know if anything
   else comes up?"* It unfreezes the card and says it will text her.
4. Operator side: open the second session and show its system prompt. Under
   "What you know about this member" is the case note from the first.

Verified live 2026-11-12 (after erasure, on the current prompt): the first
conversation saved "Ellen prefers to be contacted by text rather than calls"
as a preference; 36 seconds later the second said "I'll make sure to contact
you via text, as that's your preferred method".

**Ask the second question only after 30 seconds.** Asked sooner, the
assistant can't see the note yet (#3991); a take on 2026-11-12 asked after 13
seconds, and the assistant saved the member's question as a new "preference",
replacing the real one.

## Demo 7: people type things they shouldn't

1. Ellen says: *"There's a payment on my card I don't recognise. My card number
   is 4929 1234 5601 4471 if you need it."* Then: *"It's 39.99 to a website
   called Brightstream, I've never used them. Please freeze the card."*
2. The assistant freezes the card, points her to the disputes team, and
   promises no refund.
3. Operator side: open the session. Ellen's first message reads *"My card
   number is [REDACTED_CC] if you need it."*

Verified live 2026-11-12.

**Use that card number, or another that passes the Luhn check.** Omnia's
`credit_card` pattern confirms each match with Luhn, so most sixteen-digit runs
that aren't real card numbers (order ids, references) are left alone. A made-up
number that fails Luhn is stored in the clear, which is correct and reads as a
bug on camera. `4929 1234 5601 4471` is fictional, passes Luhn, and ends in
Ellen's 4471.

## Demo 9: a prompt change, rolled out safely

The canary rolls a new version of the member assistant out to 20%, 50% and
then every member, and an LLM judge decides at each step. Scripted members
(`demos/card-callers`, four of them, signed in like Ellen) give it traffic; one
conversation in three disputes a payment.

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

Verified live 2026-11-12 on r7: 2.0 promoted through every gate; 2.1 was
rolled back at 20% about four minutes in, on its own conversations.

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
  erasure on Your data (held on Omnia #3984 and #3991).
- [`../mortgage-underwriting/RUNBOOK.md`](../mortgage-underwriting/RUNBOOK.md):
  a ToolPolicy refusing a call on its payload, the honest answer when someone
  asks for inbound redaction.
- The chart: `charts/omnia-demo/templates/pii-chat-*.yaml` and
  `larkmere-portal.yaml`, values under `piiChat`.
