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
    aborted.
  - **The card tools trust the asserted member, verified or not.** Omnia
    forwards the asserted email on every tool call (`X-Omnia-User-Email`)
    whether or not the token was exchanged, and sends tools no signal of
    which. So the sites assert a member only together with a token for them:
    with `--issuer-url`, a token that can't be signed refuses the connection,
    and the card callers won't start without `--issuer-url`. Anyone who can
    reach the agent's port directly can still assert any member, as below.
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

- **Fakes:** four signed-in members of the portal, with scripted lines, so the
  canary has traffic to judge.
- **Real system:** members using the portal, verified by its sign-in before
  they reach the assistant.
- **What the stand-in does:** it is its own identity provider, signing a token
  for each member (`--issuer-url`), and asserts their email in `x-user-email`
  so the card tools find their card. The install trusts it like the portal.
  No line gives an SSN or other proof of identity.
- **Downstream:** the same as the front doors. The callers only run while
  `make canary` has scaled them up, so their JWKS is reachable only then.

### The claims handler (`demos/claims-triage/hack/handler.sh`)

- **Fakes:** the person who approves a held settlement. The script decides as
  the `claims-handler` ServiceAccount the chart creates, with a ten-minute
  token for the audience `omnia-referral-api`, over a port-forward to the
  workspace's referral-api.
- **Real system:** a claims handler signed in with the insurer's identity
  provider, working a queue page. Omnia has no such page, and referral-api
  takes no user approvers on this install (Omnia #4248). An Entra user would
  need `workspaceServices.referralApi.oidc` set in omnia-azure.
- **Downstream:**
  - **The record shows a ServiceAccount, not a person.** `decidedBy` is
    `larkmere-mutual/claims-handler`. Say so on stage.
  - **Only listed approvers can decide.** The ToolPolicy's `approverRoutes`
    names this ServiceAccount; any other caller gets 403. Don't widen the
    route to make the script work from elsewhere.
  - **referral-api stays ClusterIP.** The script needs kubectl access to the
    cluster, which a real handler would never have.

### Personal Agent (`demos/personal-agent`, the `ellens-assistant` workspace)

- **Fakes:** the person's own AI assistant, and the vendor that runs it.
  Ellen Whitcomb's assistant is an Omnia agent (`assistant`) whose five tools
  are the personal-agent service's `/tools/*` routes. It is not a fourth
  company.
- **Real system:** a consumer Personal Agent, run by another company on its
  own infrastructure, with its own users, sign-in and key storage.
- **What the stand-in does:** the service holds the agent's keys and Ellen's
  User IDs, serves `agent.json` and `jwks.json` at
  `https://assistant.azure.altairalabs.ai`, discovers Companies and holds
  their conversations. The model sees conversation ids, Company names and
  messages, never a key, a User ID or a Session JWT.
- **Downstream:**
  - **It runs on the same Omnia install as the Companies,** so the dashboard
    shows both sides of a conversation. The protocol itself runs over public
    HTTPS: the Companies fetch its documents from its public host, and it
    reaches them at theirs.
  - **Its store is the demo's shared Redis.** Ellen's User IDs and
    conversation records (keys `pa:*`) live in `demo-redis.omnia`, which has
    no authentication and which the Companies' agents also use for their
    conversation context. Any pod that can reach that Redis can read or alter
    them: learn Ellen's pairwise User IDs at each Company, or rewrite what the
    assistant reads back as a Company's reply. A real Personal Agent's store
    is its own, inside its vendor. Don't put anything there that the demo
    would be harmed by another tenant reading.
  - **Its keys are a cluster Secret** (`personal-agent-identity`, made once
    by `hack/personal-agent-keys.sh`), not an HSM. Replacing them changes the
    agent's identity under every Company.
  - **One listener serves public and private routes.** The HTTPRoute exposes
    only `/agent.json` and `/jwks.json` (and `/poppy-test/` while the PAP
    validator runs). The tools and the page's `/api/*` are reached
    in-cluster; anyone who can reach the Service's port can speak to a
    Company as Ellen. The NetworkPolicy admits only the assistant agent, the
    page and the Gateway on :8080, but under Istio ambient (omnia-azure
    enrols every workspace, with a waypoint) it must admit :15008 from the
    waypoint, so there it checks that a caller came through the mesh, not
    which caller it was. Accepted for the demo; the follow-up is an Istio
    AuthorizationPolicy on the waypoint that allows `/tools/*` only from the
    assistant agent's ServiceAccount and `/api/*` only from the page's.
  - **It has one user.** The assistant's facade trusts its page as the edge
    (`externalAuth.edgeTrust`), as the front doors do.
  - **The page stands in for Ellen's signed-in app** (`assistant-page`,
    `demos/demo-sites --site ellens-assistant`). It has no sign-in of its
    own: the catalogue's dashboard sign-in is in front of it, and anyone past
    that acts as Ellen. Like the front doors it signs a token for her
    (`--issuer-url`), so its issuer must be in the operator's
    `delegation.trustedIssuers` too, or the assistant's conversations have no
    verified user. It forwards only `/api/conversations`, `/api/updates` and
    `/api/direct` to personal-agent, never `/tools/*`, and nothing of the
    browser's cookie, Authorization or identity headers.

## Line-of-business systems

### Case, claims and account stubs (`demos/mortgage-underwriting`, one binary)

- **Fakes:** the systems the agents' tools call:
  - mortgage case, credit, income, affordability, valuation, AML and case
    notes;
  - the member's card (`get_card_status`, `freeze_card`, `unfreeze_card`);
  - claims (`claim_*`);
  - Wrenmoor's billing and tariff system.
  - Wrenmoor's network outage feed (`outages.go`): one storm outage for HX7
    that never ends. Its times are relative to the clock (restoration
    estimated at the top of the hour about three hours ahead, the cut
    started 18h20m before that), held still for a take and moved on once
    the estimate is under an hour away. Ellen Whitcomb is held as a Wrenmoor
    customer in HX7. The older bill, usage and tariff tools still serve Amira
    only.
  - **Wrenmoor's outage and compensation belong to the signed-in customer**
    (`compensation.go`, served with `--account-api wrenmoor`). The customer is
    the verified `email` of the exchanged token (Amira or Ellen), never a
    header; their outage is the HX7 feed's, from their postcode. The
    compensation rule is Wrenmoor's own fictional standard (£95 at 12 hours,
    then £45 per further full 12 hours, paid automatically within 28 days).
    `owed` is a statement: nothing is paid or credited, and the outage never
    ends, so it is always owed the £95 tier (hours off stay between 15 and 18).
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
  - **It answers the claims policy's peril lookup.** `/policy/claim-peril`
    is not a tool: the ToolPolicy's http producer asks it for the peril of the
    claim a call names, so CP-7 (flood claims go to Flood Response) can be a
    policy rule. A real insurer's policy would ask its claims system the same
    way.
  - **It routes referrals by peril**, as a claims system does: the agent gives
    a reason, never a team.
  - **Home claims belong to the signed-in member** (`homeclaims.go`, served
    with `--account-api larkmere`). The member is the verified `email` of the
    exchanged token, never a header. `homeclaimsdata.go` holds two home
    policies (Ellen, Gwen) by email; a real insurer would link claims to a
    policyholder record. A claim is visible and writable only to the member who
    opened it, and another member's claim answers 404, as an unknown one does.
    Unlike the claims-triage maps these take a mutex. They are separate from
    the claims-triage claims, and no excess or £500 limit is enforced (the
    ToolPolicy's job, milestone 3). State is in memory.

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
