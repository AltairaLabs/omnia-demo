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
    operator's `delegation.trustedIssuers` (`make omnia-values` writes the
    list for your Omnia install), exactly as it would list Entra or Okta.
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
  need `workspaceServices.referralApi.oidc` set on the Omnia install.
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
  `https://assistant.<zone>` (your `dns.zone`), discovers Companies and holds
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
    page and the Gateway on :8080, but under Istio ambient (the install
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
  - **Sign-in tokens are sealed in the shared Redis under a key from a
    Secret** (`personal-agent-token-key`, made once by
    `hack/personal-agent-keys.sh`, AES-256-GCM, bound to the Company and User
    ID they belong to). The Redis is the one above, so anyone who can read it
    sees only sealed blobs. Anyone who can read the Secret can open them.
    Replacing the key (`--force`) leaves every stored sign-in unreadable, and
    Ellen signs in again. A real Personal Agent keeps its tokens in its own
    store or an HSM.
  - **The public `/signin/callback` cannot tell who completed a sign-in.**
    A real Personal Agent receives the redirect inside its own app, signed in
    as the User (PAP section 4.2, section 10.8), so it knows whose browser
    came back. Here the page is on another host, behind the catalogue's
    sign-in, and the callback is a public route (the one `/signin/callback`
    Exact path on the HTTPRoute): whoever holds a valid `state` can finish
    that sign-in. The `state` is 128 random bits, single use and valid for 10
    minutes, and the redirect's `iss` must match. The defence left is
    Larkmere's consent page (`prompt=consent`), which Ellen must approve.
  - **Pending sign-ins are in memory.** The personal-agent runs as a single
    replica, and a restart between the button and the callback loses the
    pending `state`: Ellen signs in again. (Tokens already granted are in
    Redis and survive.) After a page reload a pending sign-in shows "ask your
    assistant to start the sign-in again".

### The assistant's test companies (`demos/personal-agent/arenafake`)

- **Fakes:** the personal-agent service and the Companies behind it, for the
  assistant's PromptArena kit only (`arena/config.test.arena.yaml`). It
  serves the same `/tools/*` routes on 127.0.0.1:18640, and Larkmere,
  Wrenmoor and Halvern Partners are fixed scripts. It never ships: the pack
  is built from `config.arena.yaml`, whose tools are mocks.
- **Real system:** PAP discovery and conversations with the Companies' Poppy
  agents, whose replies arrive later over a stream, and Direct Sign-In on the
  Company's own page.
- **Downstream:**
  - **A Company replies as soon as it is written to.** The kit sends the
    page's "Update:" lines itself, and pins "does not wait" with assertions,
    not timing.
  - **Conversation ids are fixed per Company** (`conv-wrenmoor-1`, ...) so a
    scripted "Update:" line can name them. Starting a conversation resets
    that Company's, so the kit runs one scenario at a time.
  - **Both Companies ask for sign-in through the protocol** (`sign_in.needed`).
    A first message about the area instead picks Wrenmoor's prose-refusal
    script (it writes "she needs to sign in" with no `sign_in.needed`), kept
    from a live take as a robustness case.
  - **It also judges the kit's sign-in check** (`POST /eval/sign-in-warranted`,
    a `rest_eval_session`) from its own record of what each read showed.
  - **Nobody signs in.** After `sign_in`, the next message to that Company
    completes it: declined if it says Ellen would rather not sign in, signed
    in otherwise.
  - **The script is picked by keywords** in the first message (planned work,
    last month's power cut, a fence claim), and the prompt sees the literal
    `@WRENMOOR_DOMAIN@` tokens as domains. A pass shows how the assistant
    handles these replies, not that it works against the real Companies.

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
    The figures are loosely modelled on the regulator's guaranteed standards
    for supply interruptions; the real tiers, amounts and conditions differ,
    so do not quote them as the regulator's.
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
    There is no table of account links: the stub matches the exchanged
    token's `email`, which Omnia's issuer gateway puts in an exchanged token,
    with `email_verified: true`, only when the upstream provider's ID token
    marked the email verified. A token
    without both is refused with 401, and the stub never reads the email
    header on these routes. A real insurer holds its own link from the
    identity provider's user to the policyholder, made at registration, and
    does not depend on an email matching; here, whoever controls a verified
    email at the upstream provider is that member.
    Unlike the claims-triage maps these take a mutex. They are separate from
    the claims-triage claims, and no excess or £500 limit is enforced (the
    ToolPolicy's job, milestone 3). State is in memory.
  - **The issuer's keys are cached.** The account routes (`obo.go`) trust the
    keys they fetched from the issuer's JWKS until a token names an unknown
    `kid` (refetched, at most once per interval) or the stub restarts. A key
    the issuer withdraws keeps verifying tokens here until then. A real API
    would refresh its keys on a schedule as well.

### The Companies' Auth0 tenants behind the issuer gateway (`poppy.auth`)

The issuer gateway is not a stand-in. It is Omnia's own component (shipped in
Omnia dd2e94540): the product path for a Company whose identity provider does
not speak PAP, and what a real Company would run here too. The stand-ins are
what sits behind and beside it.

- **Fakes:**
  - **The upstream identity provider:** both Companies' customer identity
    provider is one free Auth0 tenant, with a gateway application per Company
    and one test user, Ellen (see "Ellen
    at the upstream identity provider" below).
  - **The account link:** the stub links a customer to a policy or account by
    the verified `email` the gateway copies from the tenant (see "Home claims
    belong to the signed-in member" above).
- **Real system:** the Company's own customer identity provider, with its
  real customers, and a link from each identity to the policyholder or
  account holder, made at registration. Whether the Workspace names that
  provider (`auth.issuer`, when it speaks PAP natively) or the gateway in
  front of it (`auth.gateway`) is the Company's choice, not a demo shortcut.
- **What the demo does:** the Workspace sets `auth.gateway`, with `auth.issuer`
  the gateway's own URL (`https://auth.<company>.<zone>`), in
  front of the Auth0 tenant. The gateway does the PAP parts the tenant cannot,
  signs its own tokens (RS256, `at+jwt`) and puts `email` and
  `email_verified: true` from the tenant into exchanged tokens only. The
  account tools' Connection exchanges at the gateway (`/oauth/token`, as the
  exchange client the Workspace registers), and it and the stub accept tokens
  only from the gateway (the chart refuses a `poppy.account.subjectIssuer` or
  `tokenURL` that names anything else).
- **Why not Auth0 directly:** Auth0's Poppy-relevant gaps, measured on
  2026-11-24, are that it accepts no ES256 or PS384 client assertions (the
  algorithms PAP names for a Personal Agent) and has no `protected_resources`
  (RFC 9728 resources a token can be bound to). So the issuer is Omnia's
  gateway, not Auth0.
- **Downstream:**
  - **Off in the profiles until the deploy prep is done** (the runbook's
    milestone 2 Setup). With `poppy.auth.enabled` false the Workspaces name
    no issuer, and the
    Company Agents run the signed-out pack. Turning it on selects the
    `-signin` pack, which adds the account tools. Both packs ship, and
    `poppy.auth.enabled` chooses.
  - **One scope per Company** (Larkmere `claims`, Wrenmoor `outages`).
  - **One Auth0 tenant for both Companies.** Auth0's free plan allows one
    tenant per team. The issuers stay distinct (no `PoppyIssuerConflict`)
    because each is its Company's gateway host; the tenant is only upstream.
    Two real Companies would never share a customer pool: here one login is
    Ellen at both, and an Auth0 session from signing in to one carries over
    to the other.
  - **The email link means whoever controls a verified email at the tenant is
    that customer**, and a customer whose email the tenant has not verified
    gets no `email` in the exchanged token and is locked out: the stub refuses
    the token (401).

### Ellen at the upstream identity provider

- **Fakes:** Ellen's login at Larkmere and at Wrenmoor.
- **Real system:** each Company's customer identity provider, with its own
  accounts. Here both Companies share one Auth0 tenant holding a
  test user, Ellen Whitcomb, `ellen.whitcomb@example.com`, with her email
  marked verified and a password kept by the presenter.
- **Downstream:** anyone with that password signs in as Ellen at both
  Companies. The stub links her to her policy or account by that verified
  email, so a test user created with a different email gets 404 "no account
  for this customer".

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

### The company's document store (`memory-documents/`, `deploy/azure/memory.sh`, `templates/ingest.yaml`)

- **Fakes:** a company's own document store (a SharePoint site, a DMS, a file
  share) and the people who keep it. It is a private Blob container per
  tenant, `<tenant>-documents`, that `make azure-memory` seeds from synthetic
  Markdown in `memory-documents/<tenant>/`, in a storage account the installer
  brings. The connector that reads it is the repo's example
  (`memory-ingestion/`), run by the `memory-ingest` CronJob (hourly) as the
  tenant's workload identity.
- **Real system:** the customer's own source, owned and edited by the
  company, read by a connector the company runs with its own identity and
  rights on that source.
- **Downstream:**
  - **The seeder is the document owner.** `make azure-memory` uploads changed
    files and deletes blobs that have no file, so the repo, not a person, is
    the source of truth. Editing a document means editing the file and seeding
    again.
  - **The grant is ours.** The tenant's identity holds `Storage Blob Data
    Reader` on its own container only, granted by the same script. A real
    company grants its connector read on its source. That identity is shared
    by every agent in the tenant (each agent's ServiceAccount is federated to
    it), so any of them could read the container with its token; a real
    connector would have an identity of its own. A new grant takes a few
    minutes to apply, so the first run waits for a second `make azure-memory`.
  - **A document's folder is part of its identity.** Subfolders of
    `memory-documents/<tenant>/` become blob folders, and the document's URL
    carries them. The claims agents' access filter reads a `/member-guidance/`
    folder in that URL.
  - **Publishing is a script.** `hack/publish-document.sh` stands in for an
    editor adding a document to the company's store: it uploads one file to a
    folder and starts an ingest run. The file is not in the repo, so the next
    `make azure-memory` removes it, and the next ingest run forgets it.
  - **Ingestion is hourly, not live.** A change in the source reaches memory
    on the next run; `make azure-memory` starts one at once.
  - **An empty container forgets everything.** The connector's list of what
    the source holds is the truth, so an empty one removes every document.
    The seeding script refuses to run from an empty directory.
  - **No memory block, no memory.** Without `memory` in `site.yaml` there is
    no container and memory is off for every tenant.

## Not yet audited

The following stand-ins exist but have not been documented here yet. Document
one before relying on its behaviour:

- the SharePoint sample documents;
- the data analyst's schedule.
