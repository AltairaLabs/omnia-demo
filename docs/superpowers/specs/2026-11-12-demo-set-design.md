# The demo set

**Date:** 2026-11-12 · **Status:** approved 2026-11-12 · **Replaces:** the demo
to-do list in omnia-azure's local backlog (`docs/local-backlog/2026-11-09-demo-tenants.md`)

## Why

The demos were drifting into use cases no company would build. The clearest
case: card support asks a caller for their SSN in chat, verifies it with a
tool, and is then trusted, by its prompt alone, not to use anything it
remembers about the caller first. Turning on memory made it do exactly that,
and the canary's judge failed it. A real bank verifies the member before the
conversation (a portal sign-in, or the phone system's own checks) and gives the
agent a verified user. It would never rely on a prompt for identity.

The audience is people who build agents with LangChain on a laptop. A demo
lands when it shows something they would recognise as worth having and could
not easily build themselves: memory that respects consent, a policy that
refuses a tool call on its payload, a rollout an LLM judge can stop.

## Goals

- Every demo is a use case a real company would implement, done the way it
  would implement it.
- Each demo makes one point, in two or three minutes, that a laptop setup
  can't make.
- The demos form a ladder, from familiar ground to what only a platform does,
  in small steps.
- Each runs live on the Azure deployment against a real model, with a
  runbook, and is marked Live in the catalogue only once verified.

## Non-goals

- Showing every Omnia feature. A feature without a realistic use case here
  waits.
- Deterministic (mock-model) captures as the main path. They stay available
  for recordings.
- Dreaming and self-improving agents. They are the sequel to Act 5, after
  Omnia ships the pieces they need.

## Anti-goals

These make a demo a failure even if it works:

- **A contrived flow**, such as identity proven in chat or a control that is
  really a prompt.
- **A workaround that hides an Omnia gap.** Fix stand-ins and our config;
  ticket Omnia bugs; hold the demo if it can't be shown honestly
  (AGENTS.md, "When Omnia does something unexpected").
- **A stand-in that can't do what its agent is asked to do.** The agent will
  pretend (docs/stand-ins.md).
- **Off-cast brands.** Only Larkmere Mutual, Wrenmoor Energy and Halvern
  Partners.

## Constraints

- Omnia 0.9.0-beta.23 on `aks-omnia-demo`, until beta.24 lands.
- All work stays local on omnia-demo `feat/demo-plan` and omnia-azure
  `feat/dev-registry`, then one omnia-demo PR. No tag (v0.2.0) until every
  demo here is verified live, including any held on Omnia: the tag waits for
  them.
- Images from the dev registry (`make images`), pinned by sha in the azure
  profile until the tag.
- Every demo company, person and figure is fictional, and every page keeps the
  fictional-company disclaimer.

## The ladder

Status: **exists** means the agent and stand-ins are built; it still needs a
live check against the current build and a runbook unless it says verified.

### Act 1: an agent, run properly

**1. Wren explains a bill** (Wrenmoor, exists, verified 2026-11-10)
- *Use case:* an energy supplier's customer assistant.
- *Beats:* Amira, signed in to the customer portal, asks why her bill went up.
  Wren reads the bill, the half-hourly usage and the weather. The portal shows
  the receipts: exactly which systems Wren read. It finds a cheaper tariff,
  and switching needs Amira's "Switch me".
- *Point:* tools with receipts, and an action that needs the customer's click.

**2. The field manual is the source** (Wrenmoor, exists, verified 2026-11-10)
- *Use case:* a copilot for engineers in vans.
- *Beats:* an engineer asks a safety question and gets an answer citing the
  manual. We change one manual; the next answer changes, with no redeploy.
- *Point:* knowledge lives in documents (institutional memory), not prompts.
- *Work:* the "edit the manual" beat is new: a scripted re-ingest of one
  changed document.

### Act 2: guardrails you can't prompt your way to

**3. The £500 limit** (Larkmere, exists)
- *Use case:* claims triage with delegated authority.
- *Beats:* a £300 claim settles on its own. At £800 the policy refuses the
  settlement before the claims system is reached, the agent refers it, and
  the refusal is on the record.

**4. Bank statements never leave** (Larkmere, exists)
- *Use case:* mortgage underwriting with a third-party income check.
- *Beats:* the agent tries to send raw transactions to the supplier; the
  policy refuses the call on its contents; the derived summary goes instead.

**5. Reversible yes, irreversible no** (Wrenmoor, exists)
- *Use case:* an on-call incident agent.
- *Beats:* it diagnoses an incident and rolls back a deploy itself; asked to
  delete a namespace, it is refused, and a human must do that.

### Act 3: it knows your customers, safely

These replace today's card support (`pii-chat`).

**6. It remembers you** (Larkmere, new: the member assistant)
- *Use case:* the assistant in a bank's signed-in member portal.
- *Beats:* Ellen is signed in, so the assistant has a verified member from
  the first turn and never asks who she is. She asks it to freeze her card
  and mentions she'd rather be texted. A week later (a new conversation) it
  already knows both.
- *Point:* user memory as a platform service, keyed to a verified user.
- *How:* card support becomes the member assistant. It keeps
  `freeze_card` / `unfreeze_card` and ambient memory, and loses the SSN check
  in chat. Its identity comes from the portal's signed-in member: the
  forwarded token Omnia's mint turns into a verified user (already built:
  `demos/demoidp`, `delegation.trustedIssuers`).

**7. People type things they shouldn't** (Larkmere, rework of today's redaction demo)
- *Use case:* the same assistant.
- *Beats:* reporting a suspicious payment, Ellen pastes her full card
  number. The assistant doesn't need it (it knows her card), and the stored
  transcript and the case note it remembers keep only `[REDACTED_…]`. The
  dashboard shows the redaction count.
- *Point:* what comes to rest is governed by policy, whatever the member
  types. As today, this is redaction at rest, never "the model didn't see it".
- *Builder's call:* no tool takes the pasted card number, since a real bank
  wouldn't route a typed PAN into a system; the tool-call redaction beat
  goes. The beat is the transcript and the memory.

**8. Ellen controls her data** (Larkmere, built, held)
- *Use case:* GDPR self-service in the portal.
- *Beats:* on Your data, Ellen sees what the assistant remembers, withdraws
  consent for a category (its memories go at once), and requests erasure,
  which completes with a record of what was deleted.
- *Blocked on:* Omnia #3984 (members can't see the memories agents save) and
  #3991 (the runtime's 30-second profile cache can serve deleted memories).
  The portal page, proxy and Workspace `endUserAuth` are built and tested.

### Act 4: change it without breaking it

**9. A prompt change, rolled out safely** (Larkmere, rework of today's canary)
- *Use case:* shipping a new version of the member assistant.
- *Beats:* v2.0 adds a useful line and promotes through 20%, 50% and 100%.
  v2.1 gets "friendlier" and starts promising members their money back on a
  disputed payment, which it can't guarantee; an LLM judge scores live
  conversations and the canary rolls it back on its own at 20%. Each
  conversation stays on the version it started on.
- *How:* same machinery as today (`make canary`), moved to the member
  assistant, with a new regression and a judge for "never promises an
  outcome it can't guarantee". The scripted callers become members of the
  portal, each signed in.
- *Builder's call:* the regression stays in the assistant's own domain
  (cards and payments), so it needs no claims tools; claims stay with
  claims triage.
- *Note:* the gate averages every result ever recorded for an eval id (Omnia
  #3950), so each clean run needs a new judge revision (`EVAL_REVISION`).

**10. Research on a budget** (Halvern Partners, new workspace `halvern-research`)
- *Use case:* a batch research function a team wants cost-capped, not a
  conversation. Halvern's knowledge team runs a research function over a
  batch of questions, for example summarising one regulatory change for each
  of 50 client matters, under a daily cap.
- *Beats:* the batch starts; partway through, the cap is reached; the
  remaining runs are refused rather than spent; cost analytics shows which
  runs used it and why it stopped.
- *How:*
  - **No real spend.** The function runs on a mock Provider, which reports
    token counts and a cost; the Provider's `pricing` sets a rate high enough
    that a few runs reach a small cap. Spending real money to show that
    spending stops is the wrong demo.
  - **Its own workspace.** A workspace budget covers every agent in it, so
    the cap lives in `halvern-research`, a workspace of its own (omnia-azure
    `make workspace`, plus a profile in omnia-demo). Reaching it blocks
    nothing else.
- *First checks:*
  - that Omnia applies a Provider's `pricing` to the mock provider;
  - that the workspace budget counts that cost, and that `block` applies to
    function invocations as well as conversations.
  If either fails, that's an Omnia question, not something to work around.

### Act 5: learn from it

**11. What members ask that we can't answer** (Larkmere, Galaxy exists)
- *Use case:* the head of claims looking for gaps in the society's guidance.
- *Beats:* the Memory Galaxy shows a large cluster of member questions about
  flood and no flood procedure beside it. The head of claims writes one; it is
  ingested; the claims agent uses it on the next flood claim.
- *Point:* agents' memories double as a map of where your own knowledge falls
  short.
- *Builder's call:* the Galaxy shows the gap; the close is the claims agent
  using the new procedure. The Galaxy needn't redraw the gap as closed, which
  could depend on Omnia's graph extraction. Claims triage gains knowledge
  retrieval for this.
- *Sequel, later:* Dreaming and the Evolution Kernel (Omnia #2511, #3613)
  turning that evidence into tested improvements.

## Outside the ladder

These keep their own tracks and are not reworked here:

- Halvern Partners: the Arena builder, starter templates, the legal agents
  for in-house teams and the Practice Desk.
- Wrenmoor's multi-provider evaluation grid.
- The grid analyst's A2A call from the incident agent.
- Exposing mortgage underwriting on `agents.azure.altairalabs.ai`: dropped;
  nobody could say what it was for.

## What changes in the code

- `pii-chat` becomes the member assistant: its prompt, pack and judge change;
  `verify_identity` and the SSN-in-chat flow go; the cardholder stub keeps
  freeze and unfreeze, keyed by the verified member rather than a customer
  reference from an SSN check.
- `demos/card-callers` sign in as members through the portal's issuer rather
  than giving an SSN.
- The redaction policy and its RUNBOOK beats are rewritten around Ellen
  pasting a card number.
- The catalogue's rows follow the ladder above, each marked Live only once
  verified.
- The memory "ladder" steps proposed on 2026-11-12 fold into demos 6, 8 and 11.

## Builder's calls

- **The agent keeps its name, `pii-chat`.** Its display name and
  description change to the member assistant. Renaming it would regenerate
  the Galaxy corpus (its agent lists name `pii-chat`) and rename identities,
  for little gain; revisit before recordings.
- **The reworked demos (6, 7, 9) are live only.** Their deterministic scripts
  are rewritten later, and only if a recording needs them.

## Left to the builder

- Exact wording of prompts, scripted lines and judge criteria.
- How the "edit the manual" beat (2) re-ingests a document.
- Which research question demo 10 uses, within Halvern's world.
- Runbook structure, one per demo or one per act.
