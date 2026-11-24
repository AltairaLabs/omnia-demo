# A personal agent for the Poppy demos

Status: story approved 2026-11-23; design for review

## Why

Omnia's latest release adds the `poppy` facade: a company's agent takes
conversations from a person's own AI assistant (a Personal Agent, in the
[Personal Agent Protocol](https://personalagentprotocol.org/docs/1.0/spec),
PAP 1.0) instead of from the company's website. The Azure demo runs a build
that has it (`5d9d48cac`, which includes held turns, #4229).

A Poppy demo needs something on the other side of the wire. The only consumer
Personal Agent that speaks Poppy today is Meta's Muse, and Muse is not
available in the UK. No other consumer agent has shipped it, and the protocol's
own repository has no demo Personal Agent, only a company-side validator. So
this repo builds one.

What the audience should see: a person asks their own assistant to sort
something out. The assistant finds the right companies by itself, talks to
Larkmere Mutual's and Wrenmoor Energy's agents on Omnia, and comes back only
when it needs the person: to sign in, to approve something, or to say it's
done. Then the same exchange, seen from the company's side in Omnia's
dashboard: the session, the policy decisions, the account the conversation is
bound to.

## The story

Ellen Whitcomb lives in Hebden Bridge (HX7), in the Calder Valley. She is a
Larkmere Mutual member with home insurance and a Wrenmoor Energy customer.
After a storm she tells her assistant:

> "The storm's flooded the kitchen and the power's off. Sort it out."

1. **It finds both companies.** The assistant knows her insurer and her
   energy supplier, reads each one's `poppy.json` and opens a conversation
   with each at the same time.
2. **Wrenmoor: the power cut.** Wrenmoor's agent confirms the outage on her
   postcode and when power is due back. Signed in, it confirms the
   compensation she is owed for a long cut.
3. **Larkmere: the claim.** Larkmere's agent asks for sign-in partway
   through. The assistant asks Ellen, she approves on Larkmere's own page,
   and the agent reports the claim: what happened and what's damaged. It
   also says whether emergency repairs or a hotel are covered, and what
   happens next.
4. **The handoff only a personal agent can make.** Wrenmoor confirms the
   power was off for 19 hours. The assistant takes that back to Larkmere and
   adds the freezer contents to the claim. Neither company could have
   connected the two.
5. **"We'll get back to you."** The kitchen is over the £500 an automated
   assessor may settle, so Larkmere's agent says a claims handler will look
   at it and the conversation moves on. When the handler decides, the outcome
   arrives as a later Larkmere message. The assistant's background reader
   picks it up and tells Ellen. This beat needs Omnia #4421 and #4422, which
   turn a long hold into a follow-up. Until they ship, the demo stops at
   step 4.

Wrenmoor's area already fits: the flood claims demo (demo 11), the incident
agent's outage data and `get_weather` for HX7 all exist.

## Goals

- A Personal Agent that a person talks to in plain language, and that holds
  Poppy conversations with several companies on their behalf.
- It speaks PAP 1.0 correctly enough that the company side runs with no
  shortcut: public identity documents, verified Session JWTs, an allowlist
  that names it, discovery from each company's own domain.
- Larkmere Mutual and Wrenmoor Energy each published as a Poppy Company, on
  a domain of their own.
- Each protocol feature the demo shows is visible in the assistant's UI, in
  terms a person understands: who it is talking to, what it's waiting for,
  when it needs them.

## Non-goals

- A general-purpose consumer assistant. It knows how to find companies and
  talk to them, and nothing else.
- Passing the PAP validator in full, or implementing every optional part of
  the spec (MCP and OpenAPI interfaces, tasks, feedback, browser sessions,
  extensions, Mediated Sign-In, DPoP).
- Running on a phone. It is a web page laid out like a phone app.

## Anti-goals: a failure even if it works

- **Any loosened gate on the company side.** No `personalAgents.any: true`, no
  `facade.poppy.insecureTestHosts` (marked test-only in Omnia's docs), no
  reaching the facade by its cluster address. The facade must fetch the
  Personal Agent's documents from its public address, as it would for Muse.
- **Copying Omnia's test Personal Agent** (`omnia/test/poppyagent`). It is
  under Omnia's commercial licence and this repo is public. Write it from the
  spec. The PAP repository is Apache-2.0, and its JSON schemas
  (`kit/company/schemas`) may be vendored with attribution.
- **Imitating Meta's Muse,** or any real assistant, in name or look.
- **A demo that only works because the companies' prompts know about the
  Personal Agent.** The company agents must treat it like any other Personal
  Agent.

## Constraints

- **AGENTS.md:** this is a stand-in. It gets a section in
  `docs/stand-ins.md` and a comment where it is built, saying what it fakes,
  what the real system would be and what changes downstream.
- **AGENTS.md:** the fictional companies are Larkmere Mutual, Wrenmoor Energy
  and Halvern Partners, and no others. A branded assistant would be a fourth
  company, so the assistant is unbranded: "Ellen's assistant".
- **PAP 1.0, Personal Agent side** (§4.1, §5.1, §5.2):
  - `agent.json` (its client metadata) and `jwks.json`, served over HTTPS at
    the `client_id` URL with no redirect. `jwks_uri` and every
    `redirect_uris` entry must be on the same domain as `client_id`.
  - A delegation JWT (`poppy-delegation+jwt`, ES256, at most 24 hours) for
    each delegated key, and a Session JWT (`poppy-session+jwt`, ES256)
    signed by that key for every request. Its claims are `iss`, `aud`,
    `sid`, `sub`, `htm`, `htu`, `iat` and `jti`, plus `ath` when an access
    token is sent.
  - One delegated key per User ID. User IDs are random, one per User per
    Company, never derived from personal data, and kept by the agent.
- **Omnia's facade** verifies the above and refuses a Personal Agent not on
  the Company's `personalAgents.allow`. It fetches client metadata from
  public addresses only, without following redirects, and keeps each `jti`
  for 60 seconds either way (so the agent's clock must be right).
- **Discovery is on the Company's own domain:**
  `https://<domain>/.well-known/poppy.json`, routed to management-api, with
  the Host header kept. Today every demo site is a path under
  `omnia-demo.azure.altairalabs.ai`, so each Company needs a hostname of its
  own.
- **The cluster has no general egress** (demos/demo-sites/README.md). The
  agent fetches the Companies' documents from their public domains, and the
  facades fetch the agent's documents from its public domain. All three are
  this cluster's own gateway, reached from inside it. Omnia's docs say this
  is the only supported way to run both sides on one install. Whether pods on
  this cluster can reach its own public IP is unverified (milestone 0).
- **Both company assistants act only on the signed-in customer.** Wren's tools
  (`read_bill`, `quote_tariff`, `switch_tariff`, …) and the Larkmere member
  assistant's card tools work on "the signed-in customer". Signed out, they
  can answer only general questions. Anything personal needs sign-in, which
  needs an OAuth issuer the demo doesn't have yet (milestone 2).
- **What the existing agents can't do over Poppy:**
  - **Client-side tools are refused.** A Poppy conversation has no client to
    run them. Wren's `request_consent` is one, so Wren's `switch_tariff`
    (which needs its consent id) can never pass over Poppy.
  - **The Larkmere member assistant answers card questions only.** Its scope
    guardrail refuses general insurance questions as off-topic.

## Approaches

Three ways to build the assistant were considered.

1. **The assistant's model runs as an Omnia agent; Poppy is a tool service.**
   *(My call; correct me.)* The assistant is an AgentRuntime in a namespace of
   its own, with a PromptPack and tools served by a small Go service
   (`demos/personal-agent`). The service holds the keys, signs the JWTs,
   discovers Companies and drives conversations. The page talks to the
   assistant over its websocket facade, and to the service for events that
   arrive between turns.
   - *For:* it reuses the providers, PromptKit tool calling and tracing the
     repo already has. The protocol code is ordinary Go, testable without a
     model. Both sides of a conversation show in the same dashboard, which
     makes "here's what the company saw" a single click.
   - *Against:* the person's agent and the companies' agents run on one
     platform, which a sharp viewer may question. Answer: it's a stand-in,
     and Omnia's docs describe exactly this as supported, with no shortcut
     between the two sides.
2. **A standalone service with its own model loop.** One Go binary does the
   chat, the model calls and the protocol.
   - *For:* clearly independent of Omnia.
   - *Against:* it re-implements tool calling, streaming and provider
     credentials, which is a lot of code to write and maintain for a stand-in,
     and none of it is what the demo is about.
3. **A scripted assistant.** Fixed lines driving the protocol.
   - *Against:* it isn't an agent, and you've said the demo won't make sense
     without one. It can still be offered as a deterministic mode later (as
     other demos do), with the protocol staying live.

## The design

### Parts

- **`demos/personal-agent`** (Go, a new image): the Personal Agent's
  protocol side.
  - **Identity:** serves `/agent.json` and `/jwks.json`. Its keys are an
    ES256 key and a PS384 key, kept in a Secret so a restart doesn't change
    them. It mints delegations and Session JWTs.
  - **Users:** one person, Ellen Whitcomb, with a random User ID per Company,
    kept in a small store (Redis, which the demo already runs). The
    assistant knows her two companies' domains as part of what it knows
    about her, as a real assistant would.
  - **Discovery:** fetches and checks `poppy.json` and the protected
    resource metadata, as §3 says.
  - **Conversations:** start, message, read and stream events, close, and
    open a Direct Conversation. It keeps a cursor per conversation and a
    background reader that records events as they arrive, so a reply that
    comes hours later is not lost.
  - **Tools for the assistant:** `find_company(domain)`,
    `start_conversation(company, message)`, `send_message(conversation,
    text)`, `read_conversation(conversation)`, `close_conversation`, and
    later `sign_in(company, scopes)`.
  - **An updates endpoint for the page:** server-sent events of what
    happened in each conversation.
- **The assistant** (chart templates): an AgentRuntime in a new namespace,
  with a PromptPack that tells it to find companies by their domain, never
  claim something was done before a company confirms it, and ask the person
  before signing in or agreeing to anything.
- **The page:** a phone-shaped chat, plus a strip of the person's open
  conversations ("Larkmere Mutual: waiting for you to sign in"). It goes in
  the demo-sites image, like the other front doors.
- **Company side** (omnia-demo chart values, per tenant):
  - `spec.poppy.companies[]` on the Larkmere and Wrenmoor Workspaces, with
    `personalAgents.allow` naming the assistant's `client_id` and no other.
  - **A separate Company Agent per Company**, not a `poppy` facade added to
    `pii-chat` or `wren`. *(My call.)* A `poppy` facade needs
    `spec.context.type: redis`, which neither sets. `pii-chat` also carries
    the canary, its judge and Dreaming, and Poppy sessions would flow into
    all three. Each Company Agent is its own AgentRuntime with a `poppy`
    facade and Redis context. Its prompt is written for that Company's
    customers, never for this assistant.
    - **Larkmere: home claims.** It reports a home-insurance claim (first
      notice of loss), adds items to an open claim, and answers what the
      policy covers from the society's procedures in institutional memory.
      The claims stub (`demos/mortgage-underwriting`) gains `claim_open` and
      `claim_add_item`, keyed on the signed-in member. Settling over £500
      reuses the existing ToolPolicy rule, and its referral becomes a
      follow-up once Omnia #4421 and #4422 ship.
    - **Wrenmoor: outages.** Outage status and estimated restoration for a
      postcode, which is public and needs no sign-in. Signed in, it covers
      the customer's own outage duration and the compensation owed. It
      reuses the incident agent's outage data and `get_weather`, and the
      Wrenmoor stub gains outage and compensation reads.
  - **Ellen at Wrenmoor:** she gains a Wrenmoor account in HX7 in the
    Wrenmoor stub's data, beside Amira Saleem.
- **Who can reach what.** *(My call.)*
  - Public, because the protocol requires it:
    - the assistant's `agent.json` and `jwks.json`;
    - each Company's `/.well-known/poppy.json` and
      `/.well-known/oauth-protected-resource`;
    - the Company Agents' poppy endpoints;
    - from milestone 2, the issuers' metadata, keys and token endpoints.
  - **The assistant's page and its updates endpoint sit behind the same
    sign-in as the demo catalogue.** That sign-in is what vouches for "the
    person": anyone who can open the page acts as Ellen, as anyone who can
    open the member portal does today. The assistant's websocket facade
    takes the page's asserted identity through `externalAuth`, as the
    portals do.
  - The Companies' websites stay where they are. The new hostnames serve only
    the paths above.
- **Install side (omnia-azure):** five hostnames:
  - `larkmere.azure.altairalabs.ai` and `wrenmoor.azure.altairalabs.ai` for
    the Companies;
  - `auth.larkmere.azure.altairalabs.ai` and
    `auth.wrenmoor.azure.altairalabs.ai` for their issuers;
  - `assistant.azure.altairalabs.ai` for the assistant.

  Each gets DNS to the
  gateway IP, a gateway listener and a certificate issued by DNS-01,
  following the `agents.<domain>` pattern omnia-azure already has. All five
  go in at once, even though the issuers are needed only from milestone 2.
  *(My call, to make one install change rather than two.)* Omnia refuses one
  issuer shared by two Companies (`PoppyIssuerConflict`), so each Company
  needs its own. Then the HTTPRoutes: on each Company's host,
  `/.well-known/poppy.json` and `/.well-known/oauth-protected-resource` to
  management-api, and the poppy endpoint to its Company Agent.

### Milestones

0. **Spike: can the cluster reach itself by its public names?** Serve a
   static `agent.json` on the assistant's hostname, give Larkmere a
   `poppy.json`, and check from a pod in each namespace. Then run the PAP
   repository's validator (`kit/company/validate`) from a laptop against
   Larkmere's domain, as an independent check of the Company side. Its test
   identity (`keygen`) is hosted on the assistant's hostname, so the facade
   fetches it from inside the cluster. *(My call.)* Stacktree's hosted
   simulator would need the facade to fetch from the simulator's own host,
   and the cluster has no egress to it. If pods cannot reach the cluster's
   own public names, stop and decide with the maintainers (see Risks) before
   building anything else.
1. **Signed out, two companies** (story steps 1 and 2, signed-out parts).
   The assistant finds Larkmere and Wrenmoor and asks both at once:
   - Wrenmoor gives the outage on her postcode;
   - Larkmere says what home cover usually includes for flood and what to do
     first.

   Ellen can take over the Larkmere conversation herself as a Direct
   Conversation. Shows: discovery, the allowlist, Session verification,
   parallel conversations, and the person taking over. The dashboard shows
   the Poppy sessions with their `poppy-sid` and `poppy-agent` tags.
2. **Signed in.** *Revised 2026-11-24:* no mainstream provider meets PAP's
   issuer requirements on its own. Auth0, measured live, has CIMD but accepts
   no ES256 or PS384 client assertions and publishes no `protected_resources`.
   The issuer is therefore an Omnia component, the Poppy issuer gateway
   (Omnia #4443), in front of an Auth0 tenant, and is not built in this repo.
   This repo builds the Personal Agent's half: Direct Sign-In, the callback,
   token storage and refresh, and a `sign_in` tool. It also makes the stubs
   verify the exchanged token. The list below was the original plan for a
   custom issuer; the gateway now owns it:
   - RFC 8414 metadata listing the Company's domain in
     `protected_resources`;
   - the authorization code grant with PKCE and a consent page;
   - `private_key_jwt` client authentication, fetching the assistant's keys
     from its public `client_id`;
   - RS256 `at+jwt` access tokens.

   The Workspace names it in `auth.issuer`. The tools that act on the
   customer move to Omnia's `oauthOBO` handlers (Enterprise) with a
   `tokenExchange` Connection, and the stub systems learn to accept the
   exchanged token. The demo moment: the company asks for sign-in mid-turn
   (an `authorization` event), the assistant asks the person, the person
   approves on Larkmere's own page, and the Company acts on her account.
   Story steps 3 and 4: the claim is reported, the outage's length comes
   back from Wrenmoor, and the freezer contents are added.
3. **The follow-up** (story step 5). Blocked on Omnia #4421 and #4422. The
   claim over £500 is referred, Larkmere's agent says a claims handler will
   look at it, and the decision arrives later as a Larkmere message that the
   assistant relays to Ellen.

Each milestone is a demo in its own right and is added to the catalogue when
it works.

### Stand-in entry (docs/stand-ins.md)

- **Fakes:** the person's own AI assistant, and its vendor.
- **Real system:** a consumer Personal Agent (Muse today), run by another
  company on its own infrastructure, with its own users and sign-in.
- **Downstream:**
  - It runs on the same Omnia install as the Companies it talks to, so the
    dashboard shows both sides. They share nothing but public HTTPS.
  - Its keys are in a cluster Secret, not an HSM.
  - It has one user.
  - Its tool service, not the model, holds the keys and User IDs, so the
    model never sees them.

## Risks

- **Hairpin and egress.** If pods cannot reach the cluster's public IP, or
  egress policy blocks it, neither side can verify the other. The fixes for
  that are in omnia-azure (allowing egress to the gateway IP), never
  Omnia's test-only switch. Milestone 0 exists to find out first.
  **Checked 2026-11-23:** `curl https://omnia-demo.azure.altairalabs.ai/`
  from pods in `larkmere-mutual`, `wrenmoor-energy` and `omnia` answered
  `200` from `20.237.155.43`. Hairpin works and nothing blocks egress to the
  gateway IP.
- **Sign-in is the biggest piece** (milestone 2): a correct OAuth server, a
  consent page, token exchange, and stubs that take bearer tokens. It is a
  separate design once milestone 1 works.
- **Omnia bugs.** This is the first time Poppy runs outside Omnia's own e2e
  suite. Follow AGENTS.md: classify before working around anything.

## Decisions taken

- **The story:** the storm, with the freezer handoff as the climax
  (approved 2026-11-23).
- **The assistant:** unbranded, "Ellen's assistant" *(my call)*.
- **The person:** Ellen Whitcomb, who gains a Wrenmoor account in HX7
  *(my call)*.
- **The hostnames:** as listed under Install side *(my call)*.
- **Long holds:** never resumed in session. Past a short window, Larkmere
  says "we'll get back to you" and follows up in the same conversation.
  That is Omnia's work (#4419), not this repo's.

## Left to the builder

- The tool service's internal structure, store schema and event fan-out.
- The page's exact layout, within "phone-shaped chat plus conversation strip".
- The assistant's prompt wording, within the rules above.
- How keys are generated and loaded into the Secret.
