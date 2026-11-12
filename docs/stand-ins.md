# Stand-ins: what this demo fakes

A real Omnia customer brings their own front doors, identity provider, line of
business systems and users. This repo brings stand-ins for all of them, so a
demo runs with nothing but an Omnia install. Each stand-in fakes something,
and the platform reacts to what it is given, not to what was meant.

When Omnia does something unexpected in a demo, check this page before
changing anything (see [AGENTS.md](../AGENTS.md)). If a stand-in is the cause,
fix the stand-in to behave like the real system it replaces. Don't patch
prompts, agents or charts around it, and don't loosen a platform control.

Every stand-in here lists three things:
- **Fakes:** what is not real.
- **Real system:** what a customer would have instead.
- **Downstream:** what the fake changes in the platform.

Add a row when you add a stand-in.

## Identity

### Front doors (`demos/demo-sites`: the Larkmere member portal, the Wrenmoor customer portal)

- **Fakes:** the signed-in user. Each site has one fixed, fictional user (Ellen
  Whitcomb at Larkmere, Amira Saleem at Wrenmoor) and no login page.
- **Real system:** an edge (an API gateway, or an Istio RequestAuthentication)
  that verifies the user's token from the customer's identity provider. It
  forwards both the claim headers and the token.
- **What the stand-in does:** it acts as the edge and as the identity provider.
  It asserts the user in the headers `externalAuth.edgeTrust` reads
  (`x-user-id`, `x-user-email`, `x-user-roles`). It also forwards an RS256
  token it signed for that user (`demos/demoidp`, flag `--issuer-url`) and
  serves its key at `/.well-known/jwks.json`.
- **Downstream:**
  - **The headers admit the conversation.** Omnia's mint exchanges the token
    for the subject proof that makes the user a *verified* user. User-scoped
    memory needs a verified user, and so does everything else keyed on one.
  - **The issuer must be trusted.** The install lists each site in the
    operator's `delegation.trustedIssuers` (omnia-azure
    `charts/omnia-azure/values-azure.yaml`), exactly as it would list Entra
    or Okta.
  - **Without the token, or with an untrusted issuer,** the conversation still
    opens but has no user for memory. The runtime then registers no memory
    tools and logs "no verified user on this conversation". An agent whose
    prompt uses memory tools will misbehave: card support, when it still
    checked identity in chat, looped on `verify_identity` until it was
    aborted. The card tools also see no member and refuse.
  - **The key is not persisted.** It is generated at start, so a restart
    rotates it. The mint refetches the JWKS when it meets an unknown key id.
  - **The site cannot be trusted further than its network.** Anyone who can
    reach the agent facade's port can assert any user in the headers, and
    anyone who can reach the site can talk to the agent as its user. That is
    true of any edge-trust deployment. The real system's network controls are
    what make it safe.

### Your data (the Larkmere portal's `/api/me/`, `piiChat.portal.myData`)

- **Fakes:** the member's sign-in to their own privacy controls. The portal
  signs a second token for Ellen, with the audience `larkmere-members`, and
  forwards her consent, memory and erasure calls with it.
- **Real system:** the customer's identity provider issues the member a token
  for Omnia's end-user plane, and the customer's application sends it to
  management-api's `/api/v1/me/workspaces/<name>` routes.
- **What the stand-in does:** it serves an OpenID Connect discovery document
  (`/.well-known/openid-configuration`), because the end-user plane finds the
  issuer's keys through discovery. The Workspace's `spec.endUserAuth.oidc`
  names the portal's issuer and that audience. The proxy forwards only the
  page's routes, never the plane's MCP endpoint or opt-outs, and never the
  browser's cookie, Authorization or identity headers.
- **Downstream:**
  - **The audience is not the agent's.** The conversation's token has no
    audience; this one has `larkmere-members`. Omnia's docs ask for that
    split, so that a token for a conversation cannot open the person's privacy
    controls. Don't merge them.
  - **Omnia picks the member from the token alone.** No path, query or body
    names a subject, so the page cannot read or erase anyone else's data.
  - **Erasure deletes Ellen's real conversations and memories.** She has none
    in the Galaxy corpus, so the Galaxy is untouched, and she starts afresh
    the next time she talks to the member assistant.

### Scripted callers (`demos/card-callers`)

- **Fakes:** Larkmere's telephony channel and the cardholders on it, four
  synthetic members with scripted lines.
- **Real system:** a contact-centre platform that verifies a caller before it
  connects them to an agent.
- **What the stand-in does:** it is its own identity provider, signing a token
  for each caller (`--issuer-url`). The install trusts it like the portal.
- **Downstream:** the same as the front doors. The callers only run while
  `make canary` has scaled them up, so their JWKS is reachable only then.

## Line-of-business systems

### Case, claims and account stubs (`demos/mortgage-underwriting`, one binary)

- **Fakes:** the systems the agents' tools call:
  - mortgage case, credit, income, affordability, valuation, AML and case
    notes;
  - the member's card (`get_card_status`, `freeze_card`, `unfreeze_card`);
  - claims (`claim_*`);
  - Wrenmoor's billing and tariff system.
- **Real system:** the customer's own systems of record.
- **Downstream:**
  - **State is in memory.** A restart resets every claim, case and consent,
    and nothing is shared between replicas, so run one.
  - **Writes are not safe under load.** The claims maps have no lock.
    Concurrent writes can crash the stub, so drive the claims demo one
    invocation at a time.
  - **The card system knows members by email.** `identitydata.go` holds five
    members, keyed by the email the runtime forwards on every tool call
    (`X-Omnia-User-Email`, the verified user the edge asserted). The card
    tools take no arguments, so they act on that member's card and nobody
    else's. A call with no member is refused with 401, and an unknown member
    with 404. No tool takes an SSN, date of birth or card number: identity is
    the portal's job.
  - **Freezing is real, within the stub.** `freeze_card` and `unfreeze_card`
    (`cards.go`) change the status `get_card_status` reports, so a card frozen
    in one conversation is frozen in the next, until the stub restarts. Before
    they existed, card support told callers their card was frozen with no tool
    to freeze it: give an agent the tools for what it is asked to do, or it
    will pretend.
  - **Limits are deliberately not enforced here.** The stubs leave the controls
    under demonstration to the platform: the claims system does not enforce
    the £500 delegated-authority limit, because the ToolPolicy does. Do not add
    the check here; it would make the demo impossible to read.

## Models

### Deterministic mode (`<demo>.deterministic: true`)

- **Fakes:** the model. A mock Provider replays a scripted conversation by turn
  index.
- **Real system:** a real model.
- **Downstream:** the tool calls, the policy and the session record stay live.
  The user's lines must follow the script (each demo's RUNBOOK.md). An
  off-script question gets the next scripted answer anyway.

## Memory

### The Galaxy corpus (`demos/memory-galaxy`, replayed by `demos/memory-seeder`)

- **Fakes:** the history of conversations. About 10,000 memories for fictional
  members, three agents and the society's knowledge graph,
  generated offline.
- **Real system:** memories written by agents during real conversations, and
  documents ingested from the customer's sources.
- **Downstream:**
  - **The seeder writes as memory-api's own ServiceAccount.** That is a system
    caller allowed to name users. It is not an agent and holds no subject
    proof.
  - **It grants each member the consent categories their memories need**,
    through privacy-api. A memory in a category the member did not grant is
    refused; a seed run reports how many.
  - **Expiry:** the seeder sets one per agent memory. Member memories take the
    MemoryPolicy's per-category default, because the user save route has no
    expiry field.

## Not yet audited

The following stand-ins exist but have not been documented here yet. Document
one before relying on its behaviour:

- the SharePoint sample documents;
- the object store and S3 ingestion fixtures (`object-store.yaml`,
  `fixtures.yaml`, `ingest.yaml`);
- the data analyst's schedule.
