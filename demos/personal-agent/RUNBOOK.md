# Ellen's assistant: the storm (Poppy, milestone 1)

After a storm, Ellen Whitcomb tells her own AI assistant to sort it out. The
assistant finds her insurer (Larkmere Mutual) and her energy supplier
(Wrenmoor Energy) from their own domains, opens a conversation with each at
the same time under the Personal Agent Protocol (PAP 1.0, "Poppy"), and
reports back. Then the same conversations, seen from each company's side in
the dashboard.

This is **milestone 1, signed out**, with **milestone 2, signing in** built
but not yet run live: it needs Omnia #4443 (the Poppy issuer gateway) and an
Auth0 tenant per Company, neither of which exists yet (see
[Milestone 2: signing in](#milestone-2-signing-in)). The follow-up after a
claims handler is not built (see [What isn't here yet](#what-isnt-here-yet)).

## What it shows

- **Discovery.** The assistant reads each company's
  `/.well-known/poppy.json`; nothing is wired between them.
- **The allowlist.** Each company's agent admits only the assistant's
  `client_id` (`https://assistant.azure.altairalabs.ai/agent.json`) and
  verifies a signed Session JWT on every request.
- **Parallel conversations**, each with its own state in the strip.
- **The person taking over:** "Talk to them yourself" opens a Direct
  Conversation.
- **Both sides.** The company's session in the dashboard, tagged with the
  assistant's identity.

## Setup

First install, in this order. Each step is idempotent, so on a cluster that
already has the demos you can skip what's done.

1. **Hostnames (omnia-azure).** The companies and the assistant each need a
   hostname of their own, with a certificate that covers it. The
   `poppyHosts` values and the `poppy-dns` target are on omnia-azure's
   `feat/poppy-hosts` branch until it merges, so check it out first.
   ```bash
   make -C ../omnia-azure poppy-dns
   kubectl --context aks-omnia-demo -n omnia get certificate omnia-dashboard-tls
   ```
   The certificate must be `READY True` and list all five names as SANs:
   `larkmere`, `wrenmoor`, `auth.larkmere`, `auth.wrenmoor` and `assistant`,
   each `.azure.altairalabs.ai`. (The `auth.` names are for milestone 2; they
   go in now so the install changes once.)
2. **The page's issuer is trusted.** The assistant's page signs a token for
   Ellen, as the front doors do, so its issuer goes in omnia-azure's
   `operator.delegationAPI.trustedIssuers` (`charts/omnia-azure/values-azure.yaml`),
   beside the portals':
   ```yaml
   - http://assistant-page.ellens-assistant.svc.cluster.local:8080=http://assistant-page.ellens-assistant.svc.cluster.local:8080/.well-known/jwks.json
   ```
   Apply it with omnia-azure's `make install`. Without it the conversations
   with the assistant have no verified user (`docs/stand-ins.md`, "Front
   doors").
3. **The assistant's namespace.** Its keys need it, and its Deployment can't
   start without the keys:
   ```bash
   make -C ../omnia-azure workspace WS=ellens-assistant
   ```
4. **Its keys**, made once into Secret `personal-agent-identity`:
   ```bash
   CONTEXT=aks-omnia-demo hack/personal-agent-keys.sh
   ```
   A second run leaves the Secret alone. `--force` replaces only the keys:
   the `client_id` stays the same and the companies refetch `jwks.json`.
   Its conversations and User IDs stay in Redis (`pa:*`; [Reset](#reset)
   clears them). Restart `deploy/personal-agent` afterwards.
5. **Images.** No published personal-agent image is pinned yet, and the
   pinned demo-sites image has no `ellens-assistant` site:
   ```bash
   make images
   ```
   Without it the personal-agent pod sits in `ImagePullBackOff` and
   `make demos` times out.
6. **The tenants.** `make demos` installs all five (Larkmere, Wrenmoor,
   Halvern Partners, Halvern Research, Ellen's assistant) and federates each
   tenant's agents (`workspace-identity`) as it goes:
   ```bash
   make demos
   ```
   For one tenant, `make demos TENANTS=ellens-assistant`. If you add agents
   later without `make demos`, federate them with
   `make -C ../omnia-azure workspace-identity WS=<namespace>`, for the new
   Company Agents `larkmere-mutual` and `wrenmoor-energy`, and
   `ellens-assistant` for the assistant.
7. **Larkmere's member guidance**, so the claims desk has its procedures. An
   upgrade re-runs the ingest and drops what `reingest-manual.sh` added, so
   do this *after* the last `make demos`:
   ```bash
   hack/reingest-manual.sh demos/claims-triage/flood-claims.md larkmere-mutual member-guidance
   ```
   `larkmere-claims-desk` reads only documents under `member-guidance/`.
   The claims-handling rules in the same store are filtered out of what it
   can read.

## Checks before presenting

Run all of them. Each takes seconds.

```bash
# The assistant's public identity (no redirect, valid certificate).
curl -s https://assistant.azure.altairalabs.ai/agent.json
curl -s https://assistant.azure.altairalabs.ai/jwks.json

# Each company's discovery document, on its own domain.
curl -s https://larkmere.azure.altairalabs.ai/.well-known/poppy.json
curl -s https://wrenmoor.azure.altairalabs.ai/.well-known/poppy.json
```

`agent.json` names `client_id` as its own URL. Each `poppy.json` lists a
`poppy` protocol at `https://<domain>/poppy`. A 404 or a redirect on any of
these is a routing or certificate problem, not an agent one.

```bash
# The Workspaces publish their Companies: True, PoppyDiscoveryPublished.
kubectl --context aks-omnia-demo get workspace larkmere-mutual -o jsonpath='{.status.conditions[?(@.type=="PoppyDiscovery")]}'
kubectl --context aks-omnia-demo get workspace wrenmoor-energy -o jsonpath='{.status.conditions[?(@.type=="PoppyDiscovery")]}'

# Each Company Agent serves: True.
kubectl --context aks-omnia-demo -n larkmere-mutual get agentruntime larkmere-claims-desk -o jsonpath='{.status.conditions[?(@.type=="PoppyFacade")]}'
kubectl --context aks-omnia-demo -n wrenmoor-energy get agentruntime wrenmoor-outages -o jsonpath='{.status.conditions[?(@.type=="PoppyFacade")]}'

# The assistant's side is up.
kubectl --context aks-omnia-demo -n ellens-assistant get pods
```

In `ellens-assistant` you should see `assistant`, `personal-agent` and
`assistant-page` all `Running` and ready.

## Before each take

Start from an empty strip. See [Reset](#reset). The Wrenmoor outage (a storm
outage on HX7) never ends, and its times are relative to the clock:
restoration is estimated for the top of the hour about three hours from now
(UTC), and the cut started 18h20m before that. The estimate holds still
within a take and moves on only once it is less than an hour away, so expect
a different time from one session to the next, always on the day.

Two checks, each a few seconds:

```bash
# Wrenmoor's outage route, from inside wrenmoor-energy. A 404 means the wren
# image (pinned in profiles/azure/common.yaml) predates /wren/outage-status:
# wrenmoor-outages would have no outage to report.
kubectl --context aks-omnia-demo -n wrenmoor-energy run outage-check --rm -i --restart=Never \
  --image=curlimages/curl:8.11.1 -- curl -sS -X POST -H 'Content-Type: application/json' \
  -d '{"postcode":"HX7 6QA"}' -w '\n%{http_code}\n' http://wren-account.wrenmoor-energy:8080/wren/outage-status

# After the Reset: the assistant's store is empty. personal-agent's
# NetworkPolicy admits only the assistant and its page, so go through a
# port-forward, which it does not filter.
kubectl --context aks-omnia-demo -n ellens-assistant port-forward deploy/personal-agent 18080:8080 &
curl -s http://localhost:18080/api/conversations   # []
kill %1
```

The first answers `200` with `"district":"HX7","status":"off"` and today's
`estimated_restoration`. The second answers `[]`; anything else is a
conversation left from an earlier take, and the strip will show it.

## The line Ellen types

Open **Ellen's assistant** from the catalogue (`/ellens-assistant/`, behind
the dashboard sign-in) and type, word for word:

> The storm's flooded the kitchen and the power's off. Sort it out.

## The beats

1. **Both conversations appear in the strip**, one row each, as the
   assistant opens them: Larkmere Mutual and Wrenmoor Energy. Each row shows
   where it stands: working, replied (the company spoke last), waiting for
   you (the company asked for Ellen herself), done or stopped.
2. **Wrenmoor reports the outage.** HX7, the storm damage to the 11kV line at
   Mytholm, and when power is due back. This needs no sign-in: outage status
   is public, as on a real outage page.
3. **Larkmere explains cover and first steps** (what home cover usually
   includes for storm and flood, what to do first) and says that reporting
   the claim needs her to sign in. Nothing is claimed as reported.
4. **"Talk to them yourself"** on Larkmere's row. Type a line as Ellen. Her
   message opens a Direct Conversation with Larkmere's agent, and it answers
   her there. The assistant may not write in it: it is Ellen's own.
5. **The company's side.** Dashboard, **Sessions**, in each company's
   workspace (`larkmere-mutual`, `wrenmoor-energy`). Each conversation is a
   session tagged:
   - `poppy`;
   - `poppy-sid:<sid>`, the Poppy session it belongs to;
   - `poppy-agent:<agent id>`, the assistant's `client_id`.

   Open one: the agent ran as the assistant's pseudonymous User ID for that
   company, not as a named member. Larkmere's session shows the society's
   guidance in its prompt.

**Show the allowlist.** On each Workspace, `spec.poppy.companies[].personalAgents`
names `https://assistant.azure.altairalabs.ai/agent.json` and nothing else
(no `any: true`). The facade fetched that document from the assistant's
public address, as it would fetch Muse's.

## What to say about the stand-ins

- **The assistant is a stand-in** for a consumer Personal Agent such as Muse.
  Muse isn't available in the UK, and no other consumer agent speaks Poppy
  yet, so this one is built from the PAP 1.0 spec. It is unbranded on
  purpose: "Ellen's assistant" is the person's side, not a fourth company.
- **It runs on the same Omnia install** as the companies, which is why the
  dashboard shows both sides. They share only public HTTPS: each company
  fetches the assistant's `agent.json` and `jwks.json` from
  `assistant.azure.altairalabs.ai`, and the assistant reads each company's
  `poppy.json` from its own domain. No test switch, no cluster addresses.
- **The model never holds a key.** The personal-agent service signs and
  keeps Ellen's User IDs. The model sees conversation ids, company names and
  messages.
- **The caveat to own:** its store is the demo's shared Redis, which has no
  authentication and which the companies' agents also use. A real Personal
  Agent keeps its store inside its vendor. Its keys are a cluster Secret, not
  an HSM, and it has one user.
- **The company agents treat it like any Personal Agent.** Their prompts say
  nothing about this assistant.

Full entry: `docs/stand-ins.md`, "Personal Agent".

## What isn't here yet

Milestone 1 is signed out. Not in this demo until milestone 2 is switched on:

- **Sign-in:** Larkmere says reporting a claim needs Ellen to sign in, and the
  code to go through it is built, but it is off in the profiles
  (`poppy.auth.enabled: false`) until Omnia #4443 exists.
- **The freezer handoff:** taking Wrenmoor's 19-hour outage to Larkmere to add
  the freezer contents. It needs sign-in at both.
- **The follow-up after a claims handler** decides a claim over £500. It needs
  Omnia #4421 and #4422 (milestone 3).

## Milestone 2: signing in

Story steps 3 and 4: Ellen signs in to each Company from her assistant, and
each Company acts for her. **Every step marked [needs #4443 + Auth0] cannot be
done today:** the gateway (Omnia #4443, proposed) and the Auth0 tenants do not
exist yet. Until they do, leave `poppy.auth.enabled: false` and run milestone 1.
The live run is deferred.

### Setup [needs #4443 + Auth0]

1. **[needs #4443] The gateway.** Each Company's Workspace names Omnia's
   issuer gateway instead of an issuer (`auth.gateway`, rendered from the
   profile's `poppy.auth.gateway`): its host (`auth.larkmere.azure.altairalabs.ai`,
   `auth.wrenmoor.azure.altairalabs.ai`) and the upstream tenant. Auth0 can't
   be the issuer itself: it accepts no ES256 or PS384 client assertions and has
   no `protected_resources` (measured 2026-11-24). The gateway must also put
   `email` and `email_verified: true` in the tokens it issues, copied from the
   upstream user. The stub matches Ellen to her policy and account by that
   verified email. There is no account-links table and no `sub` to copy in
   by hand.
2. **[needs Auth0] One tenant per Company.** Larkmere uses its existing Auth0
   tenant and Wrenmoor a second free one, so the issuers differ
   (`PoppyIssuerConflict` otherwise). In each, create Ellen Whitcomb as a test
   user, `ellen.whitcomb@example.com`, with the email marked verified, and a
   regular web application for the gateway, whose ID and secret go in the
   profile's `poppy.auth.gateway.upstream` (`clientID`, `clientSecretRef`).
   One scope per Company: Larkmere `claims`, Wrenmoor `outages`.
3. **[needs #4443] The exchange client's secret.** The account tools swap the
   token Ellen's assistant holds for one for the Company's API (RFC 8693,
   `poppy.account.tokenURL`, `clientID`, `audience`). Put its secret in the
   Company's namespace, with the label the operator looks for:
   ```bash
   kubectl --context aks-omnia-demo -n larkmere-mutual create secret generic larkmere-exchange-client \
     --from-literal=clientSecret=<secret>
   kubectl --context aks-omnia-demo -n larkmere-mutual label secret larkmere-exchange-client \
     omnia.altairalabs.ai/token-exchange-client-secret=true
   ```
   Do the same in `wrenmoor-energy`, and name the Secret in
   `poppy.account.clientSecretRef`. Don't put the secret in a values file.
4. **[needs a Key Vault key] The Connection's KMS key.**
   `poppy.account.encryption` (`{kmsProvider, keyID, secretRef: {name}}`) seals
   the exchanged tokens. omnia-azure configures none today, so a key and its
   credentials Secret come first.
5. **Rebuild and re-pin the stub image.** The pinned stub image
   (`mortgageUnderwriting`, `wren` in `profiles/azure/common.yaml`) predates
   the account routes. Without the new image the account tools answer 404.
   ```bash
   make images
   ```
   then re-pin the tags it prints.
6. **The token key.** The Personal Agent seals its sign-in tokens under Secret
   `personal-agent-token-key`, made by the keys script alongside the identity
   keys. On a cluster that already has `personal-agent-identity`, running it
   creates only what is missing and leaves existing keys alone:
   ```bash
   CONTEXT=aks-omnia-demo hack/personal-agent-keys.sh
   ```
   Restart `deploy/personal-agent` afterwards. `--force` replaces both keys and
   orphans every stored sign-in.
7. **[needs #4443] Turn sign-in on.** Set `poppy.auth.enabled: true` in
   `profiles/azure/larkmere-mutual.yaml` and `wrenmoor-energy.yaml` (with the
   gateway, scopes and `poppy.account` values above), then `make demos`.
   This also switches each Company Agent from the signed-out pack to its
   `-signin` pack, which carries the account tools. Both packs ship, and
   `poppy.auth.enabled` chooses.
8. **Checks** [needs #4443 + Auth0], for each Company (Wrenmoor:
   `wrenmoor-energy`, `wrenmoor-outages`, `wrenmoor.azure.altairalabs.ai`):
   ```bash
   # The gateway is the issuer and verified: True, PoppyIssuerVerified.
   kubectl --context aks-omnia-demo get workspace larkmere-mutual -o jsonpath='{.status.conditions[?(@.type=="PoppyIssuer")]}'

   # The issuer switch has published.
   kubectl --context aks-omnia-demo get workspace larkmere-mutual -o jsonpath='{.status.poppy.companies[*].switch.phase}'

   # The resource metadata names the gateway.
   curl -s https://larkmere.azure.altairalabs.ai/.well-known/oauth-protected-resource

   # The facade is not refusing on-behalf-of tools (not PoppyOnBehalfNeedsEnterprise).
   kubectl --context aks-omnia-demo -n larkmere-mutual get agentruntime larkmere-claims-desk -o jsonpath='{.status.conditions[?(@.type=="PoppyFacade")]}'
   ```
   The phase should reach `Published`, and the metadata should name
   `https://auth.larkmere.azure.altairalabs.ai`.

With sign-in still off, the milestone 1 checks above are the only ones that
apply, and no conversation shows a sign-in row.

### The beats (steps 3 and 4) [needs #4443 + Auth0]

Use **one tab** of the assistant's page. Pending sign-ins are held in memory by
personal-agent, and a second tab, a reload or a restart in the middle of one
loses it: the strip then shows "ask your assistant to start the sign-in again",
and you ask the assistant to start it again.

Ellen types the same storm line.

3. **Sign-in, mid-turn.** Larkmere's row shows "Needs you to sign in", and the
   assistant asks her. She says yes and the button opens Larkmere's consent
   page on `auth.larkmere.azure.altairalabs.ai` (her login at the Auth0
   tenant comes first, if she has no session). She approves it. The callback
   says "Signed in to Larkmere Mutual", the chat shows "I've signed in to
   Larkmere Mutual.", and Larkmere reports the claim, with an `LM-41...`
   reference. The consent page is the defence that is left against someone else
   completing the sign-in (see `docs/stand-ins.md`).
4. **Wrenmoor, and the freezer.** Wrenmoor's sign-in goes the same way, to
   `auth.wrenmoor.azure.altairalabs.ai`, and it reports her hours off (15 to 18
   hours, as the outage never ends) and the compensation owed (the fictional
   £95 tier; a statement, nothing is paid). The assistant then adds the
   freezer contents to the Larkmere claim, and Larkmere confirms the item and
   the new total.

Check, after a take:

```bash
# Larkmere acted as Ellen: status=200 and her email.
kubectl --context aks-omnia-demo -n larkmere-mutual logs deploy/claims-systems | grep 'tool=claim_add_item'

# No token anywhere in the logs: this prints nothing.
kubectl --context aks-omnia-demo -n ellens-assistant logs deploy/personal-agent | grep -i -E 'bearer|eyJ'
```

On the dashboard, Larkmere's session carries a `poppy-account:` tag, and the
account tools' calls are allowed by the ToolPolicy `<agent>-account-policy`
(`larkmere-claims-desk-account-policy`, `wrenmoor-outages-account-policy`).

### Troubleshooting sign-in

Classify every refusal before changing anything (AGENTS.md): our
configuration, a stand-in's shortcut, or an Omnia bug. Report only an Omnia bug
upstream, quoting the contradiction. Never loosen a gate to make it pass.

- **`401 invalid_token` at the stub** (the account tool's call fails; read
  `logs deploy/claims-systems`, or `deploy/wren-account`). The exchanged token
  lacks `email`, lacks `email_verified: true`, or is not typed `at+jwt`
  (RFC 9068). The usual cause is the gateway not mapping the claims: our
  configuration of #4443, not a stub bug. The stub also refuses a wrong
  `aud`, the other Company's issuer, an expired token or an `alg` other than
  RS256, and never falls back to the email header.
- **`insufficient_scope`.** The token lacks the route's scope: `claims`
  (Larkmere) or `outages` (Wrenmoor). Check the scope in the Workspace
  Company's `auth.scopes`, and what Ellen's consent granted (a response may
  grant a subset, and the row then stays "needs sign-in").
- **`no account for this customer`.** The token's verified email is not one
  the stub holds. Ellen's test user at the Auth0 tenant must have
  `ellen.whitcomb@example.com`.
- **`sign_in` answers 400 with no known scopes.** The Company's discovery
  document or resource metadata listed no scopes, so personal-agent had none to
  ask for. Check `poppy.auth.scopes` and the Workspace's `auth` on the
  Company, then the `oauth-protected-resource` curl above.
- **"Ask your assistant to start the sign-in again."** The pending sign-in was
  lost: a reload, a second tab, a restart, or the 10-minute `state`
  expired. Ask the assistant to start it again.
- **The row stays "Needs you to sign in" after consent.** The consent granted
  fewer scopes than needed, or the exchange failed: read the personal-agent
  log for the reason (it never prints a token).

## Reset

Between takes, close the conversations and clear the assistant's store.

1. **Close them.** Tell the assistant: "Close both conversations." It calls
   `close_conversation` for each, and each company's agent closes its side.
   This works only in the chat session that opened them: the assistant knows
   a conversation's id from its own tool results, and has no tool to list
   them, so a new chat session (as after a page reload) can't list or close
   earlier conversations. That is why the reset relies on step 2. Skip this
   step if you are about to clear the store; the companies' sides then
   expire with their context (24 hours).
2. **Clear the assistant's keys in the shared Redis** (`pa:*`: its
   conversations, events and User IDs):
   ```bash
   kubectl --context aks-omnia-demo -n omnia exec deploy/demo-redis -- sh -c "redis-cli --scan --pattern 'pa:*' | xargs -r redis-cli del"
   ```
   Clearing the User IDs is safe: the assistant makes new ones, and each
   company then sees a new User. It leaves the companies' own keys
   (`omnia:poppy:*`) alone.
3. **Restart personal-agent**, so no Reader is still following a conversation
   you deleted, and the page starts empty:
   ```bash
   kubectl --context aks-omnia-demo -n ellens-assistant rollout restart deploy/personal-agent
   kubectl --context aks-omnia-demo -n ellens-assistant rollout status deploy/personal-agent
   ```
   Also restart it after replacing the keys (`--force`), after a clock
   correction, or when the strip shows a conversation that isn't there. On
   start it resumes a Reader for every stored conversation that is neither
   closed nor stopped, which is why it should only be restarted *after*
   clearing, or when you want that.
4. **Reload the page.** It loads `/api/conversations` first, then follows
   `/api/updates`.

Reset Larkmere's guidance only if you changed it (`reingest-manual.sh`, as in
Setup). Redis here is not persistent: if `demo-redis` itself restarts, every
`pa:*` key goes with it, and the strip starts empty.

## Troubleshooting

First look at the company's facade, which says why it refused:

```bash
kubectl --context aks-omnia-demo -n larkmere-mutual logs deploy/larkmere-claims-desk -c facade --since=10m
kubectl --context aks-omnia-demo -n wrenmoor-energy logs deploy/wrenmoor-outages -c facade --since=10m
```

After a good take, `| grep -i 'refus\|invalid_session'` on either prints
nothing.

- **`invalid_session` (HTTP 400).** The Session JWT failed a check;
  `error_description` names it. In order of likelihood:
  - **The clock.** The facade accepts a Session whose `iat` is within 60
    seconds either way, and remembers each `jti` for the same window, so a
    drifted clock on the assistant's node, or a retry that reuses a `jti`,
    fails. Compare `date -u` in the `personal-agent` pod with your own.
  - **The allowlist.** The Session names an agent not in the Company's
    `personalAgents.allow`. With an empty allowlist `PoppyFacade` is False
    but the facade serves and refuses every Session this way. Check the
    Workspace's `spec.poppy.companies[].personalAgents.allow`, and the
    profile's `poppy.allow`.
  - **`client_id`.** It must be exactly
    `https://assistant.azure.altairalabs.ai/agent.json`: the same scheme,
    host and path as `personalAgent.host` in `profiles/azure/ellens-assistant.yaml`
    and as the allowlist. `jwks_uri` and the redirect URIs must be on that
    host too, and the documents must be served with no redirect (the
    HTTPRoute exposes them as Exact matches).
  - **A rotated key.** After `hack/personal-agent-keys.sh --force`, restart
    `deploy/personal-agent`; the facade refetches `jwks.json` on an unknown
    key id.
- **`PoppyDiscovery` is False on a Workspace.** Read the reason and message:
  ```bash
  kubectl --context aks-omnia-demo get workspace larkmere-mutual -o jsonpath='{.status.conditions[?(@.type=="PoppyDiscovery")]}'
  ```
  - `PoppyNothingToPublish`: no Company Agent has a ready poppy facade yet.
    Check `PoppyFacade` on the agent (below). A Workspace with the Company
    but no agent is in this state.
  - A reason naming a Company, or `PoppyIssuerConflict`: two Companies share
    an issuer. Each needs its own (the `auth.` hostnames, milestone 2).
  - `PoppyAnyAgentEgressRestricted` / `PoppyEgressRestricted`: the
    Workspace's `networkPolicy.allowExternalAPIs` is false, or `any` is
    set. The facade must reach the assistant's public host, so egress must
    stay open, and `personalAgents.any` is never the fix.
  - Also check `PoppyFacade` on the agent: `PoppyCompanyNotFound` (the
    Workspace doesn't define the Company the facade names),
    `PoppyNoPersonalAgents` (empty allowlist) or `PoppyEgressRestricted`.
    A facade answers `not_served` when it has no Redis context store;
    `poppy.agent.redisURL` feeds it. The facade's log line
    `poppy listener refusing conversations` gives the reason.
- **The page is empty after a reload**, or "Talk to them yourself" fails.
  The page reads `/api/conversations` from personal-agent. Check the pods
  are up (`get pods -n ellens-assistant`), and that `assistant-page` carries
  the label `app.kubernetes.io/component: assistant-page` (personal-agent's
  NetworkPolicy admits the page by it). If the page loads but the store
  really is empty, the Redis keys were cleared or `demo-redis` restarted:
  start a new take. If `curl` of `/api/conversations` from inside
  `ellens-assistant` answers but the page shows nothing, hard-reload; the
  page's update stream may have dropped.
- **A conversation shows "stopped".** personal-agent's Reader gave up on it,
  for instance when the company asked for something the assistant can't do
  (sign-in when it is off, or after a refused refresh). The conversation's record says why:
  ```bash
  kubectl --context aks-omnia-demo -n ellens-assistant logs deploy/personal-agent --since=10m
  ```
  A stopped conversation isn't resumed. Ask the assistant to start a new one
  with that company, or reset and take again.
- **Larkmere answers "I don't have guidance for that"**, or has nothing about
  flood. The member guidance isn't ingested: redo Setup step 7.
- **The assistant says it can't find a company.** `find_company` reports
  `no Poppy agent at <domain>` when `poppy.json` answers 404 (or another
  4xx), names a different domain, or has no `poppy` protocol. It reports
  `couldn't reach <domain> right now` for a network error or a 5xx, and the
  assistant tries once more. Check the `curl` in *Checks* and the
  `PoppyDiscovery` condition, and `personal-agent`'s log, which has the
  underlying error.
