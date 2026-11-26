# Poppy personal agent: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Milestones 0 and 1 of the storm demo. Ellen's unbranded assistant
finds Larkmere Mutual and Wrenmoor Energy by their own domains, holds
signed-out Poppy conversations with a Company Agent at each, and lets Ellen
take one over. Milestones 2 and 3 are outlined at the end.

**Architecture:**
- **`demos/personal-agent`** is a Go service that is the Personal Agent's
  protocol side: identity documents, delegation and Session JWTs, discovery,
  conversations with a background reader, HTTP tools for the assistant, and
  an updates stream for the page.
- **The assistant's model** is an Omnia AgentRuntime that calls those tools.
- **The page** is a new site in the demo-sites image.
- **Each Company** gets a Workspace `spec.poppy` entry, routes on its own
  hostname, and a separate Company Agent with a `poppy` facade.

**Tech Stack:**
- Go 1.26, standard library, and `github.com/golang-jwt/jwt/v5`, which is
  already a dependency. No go-jose: JWKs and RFC 7638 thumbprints are built
  by hand.
- Helm (`charts/omnia-demo`), and the React site kits in demo-sites
  (esbuild).
- omnia-azure for DNS and the certificate.

**Spec:** `docs/superpowers/specs/2026-11-23-poppy-personal-agent-design.md`

## Global Constraints

- **Protocol:** PAP 1.0. The `Poppy-Version` header is `1.0`.
- **Session JWT:**
  - header `typ` `poppy-session+jwt`, `alg` `ES256`, `kid` set to the
    RFC 7638 thumbprint of the delegated key, and `poppy_dlg` set to the
    compact delegation;
  - claims `iss`, `aud`, `sid`, `sub`, `htm`, `htu`, `iat` and `jti`, with no
    `agent_id`, and `ath` only when an access token is sent;
  - `htu` has no query or fragment.
- **Delegation JWT:**
  - header `typ` `poppy-delegation+jwt`, `alg` `ES256`, `kid` set to the
    identity ES256 key's id from `jwks.json`;
  - claims `iss`, `cnf.jwk` (public members only), `iat`, `exp` and `jti`;
  - `exp` at most 24h after `iat`;
  - no User ID, Agent ID or Company information.
- **Keys and User IDs:**
  - one delegated key per User ID;
  - User IDs are random (128-bit, base64url), one per User per Company, and
    never derived from personal data.
- **Client metadata:**
  - `client_id` is `https://assistant.azure.altairalabs.ai/agent.json` and
    `jwks_uri` is `https://assistant.azure.altairalabs.ai/jwks.json`;
  - `token_endpoint_auth_method` is `private_key_jwt`;
  - `redirect_uris` holds `https://assistant.azure.altairalabs.ai/signin/callback`;
  - both documents are served with no redirect.
- **Company domains:** `larkmere.azure.altairalabs.ai` and
  `wrenmoor.azure.altairalabs.ai`. Issuer hosts `auth.larkmere.` and
  `auth.wrenmoor.azure.altairalabs.ai` are created now and used in
  milestone 2.
- **No loosened gates:** never `personalAgents.any: true`, never
  `facade.poppy.insecureTestHosts`, never a cluster address for the facade
  or the agent. `personalAgents.allow` lists only the assistant's
  `client_id`, plus the validator's for Task 13 only.
- **Copy:** the assistant is "Ellen's assistant", unbranded. Only Larkmere
  Mutual, Wrenmoor Energy and Halvern Partners exist. The person is Ellen
  Whitcomb, HX7.
- **Licensing:** nothing copied from `omnia/test/poppyagent` (commercial
  licence). PAP schemas may be vendored from
  `personal-agent-protocol/personal-agent-protocol` (Apache-2.0), with
  attribution.
- **Stand-ins:** every stand-in added gets a `docs/stand-ins.md` entry and a
  comment where it is built (AGENTS.md).
- **kubectl:** every command names `--context aks-omnia-demo`.
- **Commits:** conventional commits with `git commit -s`. Run
  `.github/workflows/ci.yml`'s commands before any PR: `go test ./...`,
  `go vet ./...`, `go mod tidy` with no diff, `helm lint charts/omnia-demo`,
  and `helm template omnia-demo charts/omnia-demo`.

## Review Focus

- **Clock skew.** The facade accepts `iat` within ±60s. A pod clock that
  drifts gets `400 invalid_session` on every request. The client must sign
  each request at send time, never ahead, and surface the error text.
  (Task 4: a test that a Session JWT is signed per call with the current
  clock.)
- **Retries reusing a JWT.** The facade keeps each `jti`, so a retried
  request with the same JWT is refused as a replay. Each attempt re-signs.
  (Task 6: a retry test asserting two distinct `jti` values.)
- **Discovery that redirects or answers for another host.** A
  `poppy.json` reached through a redirect to another path, or whose
  `organization.domain` isn't the host asked, must be rejected with a
  reason. (Task 5 tests.)
- **Events the reader has lost.** `cursor_expired`, `conversation_closed`
  and an SSE stream that drops must not lose or duplicate events. The
  reader resumes from its stored cursor and reconnects with
  `Last-Event-ID`. (Task 6 tests.)
- **The model asking about a company it can't find.** `find_company` on a
  domain with no `poppy.json` returns a plain "no Poppy agent at <domain>",
  not an error the model retries forever. (Task 7 test.)

---

## Milestone 0: can the cluster reach itself?

### Task 1: Spike: hairpin and egress from pods

Throwaway, with no code to keep. It answers whether pods reach this
cluster's own public gateway over HTTPS. Every later task depends on it.

**Files:** none committed. The findings go in the spec's Risks section.

- [ ] **Step 1: Curl the dashboard host from a tenant namespace.** The
  dashboard host is on the same gateway IP as the future hosts.

```bash
kubectl --context aks-omnia-demo -n larkmere-mutual run hairpin --rm -i --restart=Never \
  --image=curlimages/curl:8.11.1 -- curl -sS -o /dev/null -w '%{http_code} %{remote_ip}\n' \
  https://omnia-demo.azure.altairalabs.ai/
```

Expected: an HTTP status (`200`, `302` or `401`) and `remote_ip` `20.237.155.43`.

- [ ] **Step 2: Repeat in `wrenmoor-energy` and `omnia`.** The facades run
  in the tenant namespaces, and management-api runs in `omnia`. Expect the
  same result.
- [ ] **Step 3: Decide.**
  - All three succeed: record "hairpin OK, no egress block on the gateway
    IP" in the spec's Risks section, commit
    (`docs: milestone 0 hairpin check passed`), and go on.
  - Any fails (a timeout, or `curl: (7)`): stop. Record the failure and
    its namespace, and raise it with the maintainers. The fix belongs in
    omnia-azure, never in Omnia's test-only switch.

### Task 2: Hostnames (omnia-azure)

**Repo:** `../omnia-azure`, on a new branch `feat/poppy-hosts`.

**Files:**
- Modify: `charts/omnia-azure/values.yaml`. Add:
  ```yaml
  poppyHosts:
    - larkmere.azure.altairalabs.ai
    - wrenmoor.azure.altairalabs.ai
    - auth.larkmere.azure.altairalabs.ai
    - auth.wrenmoor.azure.altairalabs.ai
    - assistant.azure.altairalabs.ai
  ```
  The comment says these share the catch-all :443 listener and certificate,
  like `agentExposure.host`.
- Modify: `charts/omnia-azure/templates/dashboard-certificate.yaml:9-14`:
  `range .Values.poppyHosts` adds each host to `dnsNames`.
- Modify: `Makefile`: target `poppy-dns`, modelled on `agents-dns`
  (`Makefile:514-520`). It writes an A record for each host's relative name
  (`larkmere`, `wrenmoor`, `auth.larkmere`, `auth.wrenmoor`, `assistant`)
  to the gateway IP, TTL 300. Add it to `dns:` and to `.PHONY`.

- [ ] **Step 1: Render.** Run
  `helm template omnia-azure charts/omnia-azure | grep -A12 'kind: Certificate'`.
  Expected: all five hosts in `dnsNames`.
- [ ] **Step 2: Apply.** Ask the user before running omnia-azure's install
  or upgrade target, which deploys to the live cluster. Then run
  `make -C ../omnia-azure poppy-dns`.
- [ ] **Step 3: Verify.**
  - `kubectl --context aks-omnia-demo -n omnia get certificate omnia-dashboard-tls`
    shows `READY True`.
  - `dig +short <host> @1.1.1.1` gives `20.237.155.43` for each host.
- [ ] **Step 4: Commit** in omnia-azure
  (`feat: hostnames for the Poppy demo companies, issuers and assistant`).

---

## Milestone 1: signed out, two companies

### Task 3: Wrenmoor outage data and Ellen's account

**Files:**
- Modify: `demos/mortgage-underwriting/wrendata.go`. Add Ellen Whitcomb
  (`ellen.whitcomb@example.com`, the email the Larkmere data uses) as a
  Wrenmoor customer in HX7, beside Amira Saleem.
- Create: `demos/mortgage-underwriting/outages.go` and `outages_test.go`.
- Modify: `demos/mortgage-underwriting/wren.go:142-146`. Register
  `POST /wren/outage-status` beside the other `/wren/` routes. If `w.tool`
  requires a member, use a wrapper that doesn't, because outage status is
  public.

**Interfaces:**
- Produces: `POST /wren/outage-status`.
  - Request body `{"postcode": "HX7 8AD"}`.
  - Response `{"district":"HX7","status":"off|restored","started":"2026-11-22T21:40:00Z","estimated_restoration":"2026-11-23T16:00:00Z","restored":null,"customers_affected":1840,"cause":"storm damage to the 11kV line at Mytholm"}`.
  - Public: no member header needed.
  - An unknown district answers 404 `{"error":"no outage data for <district>"}`.

- [ ] **Step 1: Write the failing tests.**
  - `TestOutageStatusHX7` asserts `status=="off"`, `district=="HX7"`, and
    `estimated_restoration` after `started`.
  - `TestOutageStatusUnknownDistrict` asserts 404 for `LS1`.
  - `TestOutageStatusNeedsNoMember` sends no `X-Omnia-User-Email` and
    expects 200.
- [ ] **Step 2: Run them and see them fail.**
  `go test ./demos/mortgage-underwriting -run TestOutageStatus` fails:
  route not found.
- [ ] **Step 3: Implement `outageStatus`** in `outages.go`. One fixed storm
  outage for HX7; a postcode is reduced to its district before lookup.
  Add the comment the stand-ins rule needs.
- [ ] **Step 4: Run the tests and see them pass.**
  `go test ./demos/mortgage-underwriting` passes, with existing tests
  unchanged.
- [ ] **Step 5: Commit**
  (`feat(wrenmoor): storm outage status for HX7, and Ellen as a customer`).

### Task 4: Personal agent: identity and JWTs

**Files:**
- Create: `demos/personal-agent/identity/keys.go`, `jwk.go`, `session.go`,
  and `identity_test.go`.

**Interfaces:**
- Produces, in package `identity`:
  - `type Identity struct` holding the ES256 and PS384 identity keys and
    `ClientID`, `JWKSURI` and `RedirectURI` strings.
  - `func Load(dir, base string) (*Identity, error)`: reads
    `es256.pem` and `ps384.pem` (PKCS#8) from `dir`. `base` is
    `https://assistant.azure.altairalabs.ai`.
  - `func (i *Identity) Metadata() map[string]any`: the §4.1 document,
    with `client_name` "Ellen's assistant".
  - `func (i *Identity) JWKS() []byte`: public keys, each `kid` the
    RFC 7638 thumbprint, `use` `sig`, and `alg` `ES256` or `PS384`.
  - `type Delegated struct` holding the delegated ES256 key, its `kid`,
    the compact delegation and its expiry.
  - `func (i *Identity) Delegate(now time.Time) (*Delegated, error)`:
    `exp` is now+24h.
  - `func (d *Delegated) SignSession(c SessionClaims, now time.Time) (string, error)`.
  - `type SessionClaims struct{ Aud, Sub, Sid, Method, URL, AccessToken string }`.
    `SignSession` strips the query and fragment from `URL`, sets
    `ath` only when `AccessToken != ""`, and sets a fresh `jti` and
    `iat = now`.
  - `func Thumbprint(pub crypto.PublicKey) (string, error)`: RFC 7638
    SHA-256, base64url with no padding.

- [ ] **Step 1: Write the failing tests.**
  - `TestThumbprintRFC7638Example` uses the RFC 7638 §3.1 RSA example key
    and expects `NzbLsXh8uDCcd-6MNwXF4W_7noWXFZAfHkxZsRGC9Xs`.
  - `TestMetadataShape` checks `client_id`, `jwks_uri`, `redirect_uris`
    and `token_endpoint_auth_method=="private_key_jwt"`, and that
    `jwks_uri` shares `client_id`'s host.
  - `TestDelegationClaims` parses the delegation and checks the header
    `typ` and `kid`, `exp-iat<=24h`, that `cnf.jwk` has no `d`, and that
    the claims have no `sub`, `aud` or `agent_id`.
  - `TestSessionJWT` checks:
    - header `typ` `poppy-session+jwt`, `alg` `ES256`, `kid` set to the
      delegated key's thumbprint, and `poppy_dlg` present;
    - claims `iss` set to the `client_id`, and `htu` without `?x=1`;
    - no `ath` and no `agent_id`;
    - it verifies with the delegated public key.
  - `TestSessionJWTWithToken` checks that `ath` is base64url SHA-256 of
    the token.
  - `TestSessionSignedPerCall`: two calls give two `jti` values, and `iat`
    equals the `now` passed in (Review Focus: clock skew).
- [ ] **Step 2: Run them and see them fail.**
  `go test ./demos/personal-agent/identity` fails: package missing.
- [ ] **Step 3: Implement.** Use golang-jwt's ES256 and PS384 signing
  methods. Build the JWK JSON by hand: EC members `crv`, `x`, `y`; RSA
  members `e`, `n`. Thumbprint input is the required members in
  lexicographic order with no whitespace.
- [ ] **Step 4: Run them and see them pass.**
  `go test ./demos/personal-agent/identity` passes.
- [ ] **Step 5: Commit**
  (`feat(personal-agent): identity documents and PAP JWTs from the spec`).

### Task 5: Personal agent: discovery

**Files:**
- Create: `demos/personal-agent/discovery/discovery.go` and
  `discovery_test.go`.

**Interfaces:**
- Produces:
  - `type Company struct{ Name, Domain, Identifier, Issuer string; Agents []AgentEntry }`.
  - `type AgentEntry struct{ Endpoint, Description, Access string }`.
  - `func Discover(ctx context.Context, hc *http.Client, domain string) (*Company, error)`.
- Rules:
  - Fetch `https://<domain>/.well-known/poppy.json`. A redirect is
    followed only if the final URL's path is exactly
    `/.well-known/poppy.json` with no query.
  - `organization.domain` must equal `domain`.
  - Agents are the `agent.protocols[]` entries with `type=="poppy"` and an
    https `endpoint`.
  - Protected resource metadata: a 404 means no issuer; a 200 with
    `authorization_servers[0]` names the issuer; any other status is an
    error with no fallback.
  - `Identifier` is the issuer if there is one, else `https://<domain>`.
    It is the Session `aud`.

- [ ] **Step 1: Write the failing tests** against an
  `httptest.NewTLSServer`.
  - `TestDiscoverNoIssuer` expects `Identifier=="https://"+host` and one
    agent.
  - `TestDiscoverWithIssuer` expects `Identifier` equal to the issuer.
  - `TestDiscoverDomainMismatch` and `TestDiscoverRedirectToOtherPath`
    expect errors that name the reason.
  - `TestDiscoverProtectedResource500` expects an error.
  - `TestDiscoverIgnoresNonPoppyProtocols`.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them and see them pass.**
- [ ] **Step 5: Commit**
  (`feat(personal-agent): discover a company's poppy.json and issuer`).

### Task 6: Personal agent: conversations, store and reader

**Files:**
- Create: `demos/personal-agent/conversation/client.go`, `reader.go`,
  `store.go` and `conversation_test.go`.

**Interfaces:**
- Consumes: `identity.Delegated.SignSession` and `discovery.Company`.
- Produces:
  - `type Store interface`:
    - `UserID(company string) (string, error)`, which creates one on first
      use;
    - `Delegation(userID string) (*identity.Delegated, error)`, which
      re-delegates when within 1h of expiry;
    - `SaveConversation(Conversation) error` and
      `Conversations() ([]Conversation, error)`;
    - `AppendEvent(convID string, e Event) error` and
      `Events(convID string) ([]Event, error)`;
    - `Cursor(convID) (string, error)` and `SetCursor(convID, c string) error`.
  - Two `Store` implementations: `NewRedisStore(url string)` (keys under
    `pa:`) and `NewMemoryStore()` for tests.
  - `type Conversation struct{ ID, Company, Endpoint, ParentID string; Closed bool }`.
  - `type Event struct{ ID, Type, Role, AuthorType, Text string; Working *bool; Raw json.RawMessage }`.
  - `type Client struct`:
    - `Start(ctx, co *discovery.Company, agent discovery.AgentEntry, text string) (Conversation, error)`;
    - `Send(ctx, conv Conversation, text string) error`;
    - `Close(ctx, conv Conversation) error`;
    - `OpenDirect(ctx, parent Conversation, text string) (Conversation, error)`,
      which posts with `parent_conversation_id`;
    - `ReadOnce(ctx, conv Conversation, wait time.Duration) ([]Event, error)`.
  - `type Reader struct` with
    `func (r *Reader) Run(ctx context.Context, conv Conversation, publish func(Conversation, Event))`.
    It streams with SSE and resumes with `Last-Event-ID`. On
    `cursor_expired` it re-reads from the start and drops already-stored
    event IDs. It stops after `conversation_closed` or the conversation's
    `state` event shows it closed.
- Every request carries `Poppy-Version: 1.0` and a freshly signed
  `Poppy-Session`. Message bodies carry a random `id` (request ID) and
  `author.type` `agent`, except a Direct Conversation's first message,
  which is `human`.

- [ ] **Step 1: Write the failing tests** against a fake Company server
  (`httptest`) that verifies the Session header shape and records requests.
  - `TestStartSendsSignedSession`: the `aud` is the Company identifier, and
    the `sub` is the same User ID on two starts.
  - `TestRetryResignsSession`: the server answers 503 then 201, and the
    test asserts two distinct `jti` values.
  - `TestDirectConversationBody`: `parent_conversation_id` is set and
    `author.type=="human"`.
  - `TestReaderResumesAfterDrop`: the stream closes after 2 events and
    reconnects with `Last-Event-ID`. The 3rd event is delivered once.
  - `TestReaderCursorExpired`: no duplicates.
  - `TestReaderStopsWhenClosed`.
  - `TestUserIDsArePerCompanyAndRandom`: two companies give two IDs, and
    neither contains "ellen".
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.** Pick the event field names from PAP §8.5 and
  §8.6 and the vendored `event.schema.json`, which goes under
  `demos/personal-agent/conversation/schema/` with its Apache-2.0 notice.
- [ ] **Step 4: Run them and see them pass.**
- [ ] **Step 5: Commit**
  (`feat(personal-agent): conversations, a background reader and per-company user IDs`).

### Task 7: Personal agent: tools, updates, server and image

**Files:**
- Create: `demos/personal-agent/main.go`, `server.go`, `server_test.go`,
  `Dockerfile` (modelled on `demos/mortgage-underwriting/Dockerfile`) and
  `README.md`.
- Modify: `Makefile:44-49`. Add
  `omnia-demo-personal-agent,.,demos/personal-agent/Dockerfile` to
  `IMAGES`.
- Modify: `.github/workflows/publish.yml`. Add a path filter and a matrix
  entry: image `ghcr.io/altairalabs/omnia-demo-personal-agent`.

**Interfaces:**
- Consumes: Tasks 4–6.
- Produces, as the HTTP surface. Tool routes take a JSON body and answer
  JSON, as the stubs' tools do.
  - `GET /agent.json` and `GET /jwks.json` are public.
  - `GET /poppy-test/{file}` is served from `--test-identity-dir` when that
    flag is set (Task 13 only).
  - `POST /tools/find_company` takes `{"domain"}` and answers
    `{"name","domain","agents":[{"description","access"}]}`, or
    `{"found":false,"reason":"no Poppy agent at <domain>"}` with HTTP 200
    (Review Focus).
  - `POST /tools/start_conversation` takes `{"domain","message"}` and
    answers `{"conversation_id","company"}`. It starts a Reader.
  - `POST /tools/send_message` takes `{"conversation_id","text"}`.
  - `POST /tools/read_conversation` takes `{"conversation_id"}` and answers
    `{"company","working","handoff","messages":[{"role","author","text"}],"closed"}`.
  - `POST /tools/close_conversation` takes `{"conversation_id"}`.
  - `POST /api/direct` takes `{"conversation_id","text"}` from the page,
    for Ellen taking over. It is not a tool: only Ellen opens a Direct
    Conversation.
  - `GET /api/updates` is SSE, one JSON line per new event:
    `{"conversation_id","company","event"}`.
- Flags: `--identity-dir`, `--base-url`, `--redis-url`, `--addr :8080`,
  `--test-identity-dir`, and `--known-companies` (comma-separated domains
  Ellen has accounts at).

- [ ] **Step 1: Write the failing tests.**
  - `TestFindCompanyNotFoundIsAnswerNotError`.
  - `TestStartConversationStartsReaderAndPublishes`, with a fake Company:
    an update arrives on `/api/updates`.
  - `TestIdentityDocsServedWithoutRedirect`.
  - `TestToolsNeverExposeUserIDOrKeys`: no response body contains the User
    ID or a private JWK member.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.** The stand-in comment at the top of `main.go`
  says what it fakes, what the real system is, and what changes downstream
  (from the spec).
- [ ] **Step 4: Run them and see them pass.** Also run
  `docker build -f demos/personal-agent/Dockerfile .`, which must succeed.
- [ ] **Step 5: Commit**
  (`feat(personal-agent): tools for the assistant, an updates stream and its image`).

### Task 8: Companies published: Workspace poppy and discovery routes

**Files:**
- Modify: `charts/omnia-demo/templates/workspace.yaml`. Render
  `spec.poppy.companies` from `.Values.poppy.company` when
  `.Values.poppy.enabled`:
  - `name: default`;
  - `organization.name` from `workspace.displayName`;
  - `organization.domain` from `poppy.domain`;
  - `personalAgents.allow` from `poppy.allow`.
- Create: `charts/omnia-demo/templates/poppy-routes.yaml`.
  - An HTTPRoute in `.Values.poppy.omniaNamespace` (default `omnia`) on
    `poppy.domain`, with two Exact rules:
    `/.well-known/poppy.json` and `/.well-known/oauth-protected-resource`.
    Both go to `omnia-management-api:8080`, with no URLRewrite (the Host
    header must be kept).
  - parentRef `omnia-external` in the gateway's namespace.
- Modify: `charts/omnia-demo/values.yaml`. Add a `poppy:` block (`enabled`,
  `domain`, `allow`, `omniaNamespace`), each key with a `# --` comment.
- Modify: `profiles/azure/larkmere-mutual.yaml` and `wrenmoor-energy.yaml`.
  Enable it with their domain and
  `allow: ["https://assistant.azure.altairalabs.ai/agent.json"]`.

- [ ] **Step 1: Render.**
  `helm template t charts/omnia-demo -f profiles/azure/common.yaml -f profiles/azure/larkmere-mutual.yaml`
  shows the Workspace's `poppy` block and the HTTPRoute. `helm lint` passes.
- [ ] **Step 2: Install** with `make demos` (ask the user before deploying).
  Then read the condition:
  `kubectl --context aks-omnia-demo get workspace larkmere-mutual -o jsonpath='{.status.conditions[?(@.type=="PoppyDiscovery")]}'`.
  Expected: `False` with `PoppyNothingToPublish`. No agent exists until
  Task 9, so this is the correct state.
- [ ] **Step 3: Commit**
  (`feat(chart): publish Larkmere and Wrenmoor as Poppy Companies`).

### Task 9: The two Company Agents

**Files:**
- Create: `charts/omnia-demo/templates/poppy-company-agent.yaml`, one
  AgentRuntime per tenant when `.Values.poppy.agent.enabled`:
  - `mode: agent`;
  - facades: `type: poppy` with `poppy.description`, and
    `expose: {enabled: true, host: <poppy.domain>}`;
  - `context.type: redis`, with a context store like `claims-triage`'s;
  - `providerRef` from `poppy.agent.providerRef`;
  - the prompt pack and tools below.
- Create, for Wrenmoor: `demos/wren/poppy/` (pack source) and
  `charts/omnia-demo/templates/poppy-wrenmoor-tools.yaml`. The
  ToolRegistry has `outage_status` (endpoint
  `http://wren-account.<ns>:8080/wren/outage-status`, from Task 3) and
  `get_weather` (the existing endpoint).
- Create, for Larkmere: `demos/claims-triage/poppy/` (pack source). No
  tools in milestone 1. It answers from the society's procedures in
  institutional memory (`memory.enabled`, as the claims agent reads them).
- Modify: `values.yaml` (`poppy.agent.*`) and both profiles.
  - Wrenmoor's `description` is "Power cuts, restoration times and
    compensation for Wrenmoor Energy customers".
  - Larkmere's is "Home insurance claims and cover for Larkmere Mutual
    members".

**Prompt rules** (the packs carry these and nothing about this assistant):
- Speak to a customer's representative, and say when something needs the
  customer themselves.
- Wrenmoor: give outage status by postcode, and say compensation needs the
  customer's account.
- Larkmere: explain what home cover usually includes for storm and flood,
  and what to do first. Say that reporting a claim needs the member to sign
  in.

- [ ] **Step 1: Validate the packs** with `promptarena validate` (see
  memory note: it is the validator) on both pack directories. It must pass.
- [ ] **Step 2: Render and lint the chart.**
- [ ] **Step 3: Deploy** (ask first). Then check:
  - `kubectl --context aks-omnia-demo -n wrenmoor-energy get agentruntime -o jsonpath='{range .items[*]}{.metadata.name} {.status.conditions[?(@.type=="PoppyFacade")].status}{"\n"}{end}'`
    shows the new agent `True`.
  - The Workspace's `PoppyDiscovery` is `True` with
    `PoppyDiscoveryPublished`.
  - `curl -s https://wrenmoor.azure.altairalabs.ai/.well-known/poppy.json | jq '.agent.protocols'`
    lists `https://wrenmoor.azure.altairalabs.ai/poppy`.
  - The same for Larkmere.
- [ ] **Step 4: Commit**
  (`feat(chart): a Company Agent at Larkmere and Wrenmoor behind the poppy facade`).

### Task 10: The assistant: workspace, agent, tools and personal-agent

**Files:**
- Create: `profiles/azure/ellens-assistant.yaml`, a tenant for the
  assistant in namespace `ellens-assistant`.
  - Its comment says it is not a fourth company: it is the person's side,
    a stand-in.
  - `make demos` provisions it as it does other tenants (`Makefile:56-76`).
- Create: `charts/omnia-demo/templates/personal-agent.yaml`, with
  everything behind `.Values.personalAgent.enabled`:
  - a Deployment of the Task 7 image, with args
    `--base-url https://assistant.azure.altairalabs.ai`,
    `--known-companies larkmere.azure.altairalabs.ai,wrenmoor.azure.altairalabs.ai`,
    the Redis URL, and identity keys from Secret `personal-agent-identity`;
  - a Service;
  - an HTTPRoute on `assistant.azure.altairalabs.ai` with Exact `/agent.json`
    and `/jwks.json` only, plus `/poppy-test/` only when
    `personalAgent.testIdentity`.
- Create: `charts/omnia-demo/templates/assistant-agent.yaml` (AgentRuntime:
  websocket facade, `externalAuth` edge trust as the portals use it) and
  `assistant-tools.yaml` (ToolRegistry: the five tools on
  `http://personal-agent.<ns>:8080/tools/*`). The pack source goes in
  `demos/personal-agent/arena/`.
- Create: `hack/personal-agent-keys.sh`. It generates ES256 and PS384
  PKCS#8 keys with `openssl` and creates Secret `personal-agent-identity`
  with kubectl `--context` from `$CONTEXT`. It is idempotent: it refuses
  to replace an existing Secret without `--force`.

**Prompt rules:**
- You act for Ellen.
- Find companies from the domains you know her accounts at.
- Open both conversations when a request involves both.
- Never say a company did something until it says so.
- Ask Ellen before signing in or agreeing to anything.
- Tell her what each company said, in a line each.

- [ ] **Step 1: Validate the pack** with `promptarena validate`.
- [ ] **Step 2: Render and lint.**
- [ ] **Step 3: Deploy** (ask first). Then check:
  - `curl -s https://assistant.azure.altairalabs.ai/agent.json | jq .client_id`
    is the `client_id`.
  - `curl -sI https://assistant.azure.altairalabs.ai/agent.json` is `200`,
    not `3xx`.
- [ ] **Step 4: Commit**
  (`feat: Ellen's assistant, its tools and the personal-agent service`).

### Task 11: The page

**Files:**
- Create: `demos/demo-sites/site/ellens-assistant/`: tokens and
  `ui_kits/phone/App.jsx`, in the kits' existing style. The page is
  unbranded, plain and phone-shaped.
  - A chat with the assistant over the websocket.
  - A strip of open conversations, one row per company with its latest
    state, fed by `/api/updates`.
  - A "Talk to them yourself" action on a row, which opens a Direct
    Conversation through `/api/direct` and shows the company's replies in
    the strip.
- Modify: `demos/demo-sites/server.go`. Add `--site ellens-assistant`. It
  proxies the chat to the assistant agent with the asserted user, as the
  portals do. It proxies `/api/updates` and `/api/direct` to personal-agent.
- Modify: `charts/omnia-demo/templates/catalogue.yaml` and
  `files/catalogue/index.html`. Serve the page under the catalogue's path
  behind the dashboard sign-in, like the other sites, and add the demo
  entry with status Live.
- Modify: `demos/demo-sites/README.md`: add a row to the sites table.

- [ ] **Step 1: Run `hack/build-kits.sh`.** It must build.
- [ ] **Step 2: `go test ./demos/demo-sites`.** It must pass, with a new
  `TestAssistantSiteProxiesUpdates` added first and seen to fail.
- [ ] **Step 3: Commit**
  (`feat(demo-sites): Ellen's assistant, a phone-shaped page with a conversation strip`).

### Task 12: Stand-ins, runbook and the live run

**Files:**
- Modify: `docs/stand-ins.md`. Add a "Personal Agent" section using the
  spec's stand-in entry. Add the Wrenmoor outage data to the
  line-of-business stubs entry.
- Create: `demos/personal-agent/RUNBOOK.md`: setup, the line Ellen types,
  the beats, what to show in the dashboard, and reset.

- [ ] **Step 1: Run the demo live.** Ellen types: "The storm's flooded the
  kitchen and the power's off. Sort it out." Expected:
  - Both conversations appear in the strip.
  - Wrenmoor reports the HX7 outage and its restoration time.
  - Larkmere explains cover and first steps, and says reporting the claim
    needs her to sign in.
  - "Talk to them yourself" on Larkmere opens a Direct Conversation that
    answers her.
  - The dashboard shows both sessions tagged `poppy`, `poppy-sid:` and
    `poppy-agent:`.
- [ ] **Step 2: Check the facade refused nothing.**
  `kubectl --context aks-omnia-demo -n larkmere-mutual logs deploy/<company agent> -c facade | grep -i 'refus\|invalid_session'`
  should print nothing.
- [ ] **Step 3: Commit**
  (`docs: the Poppy storm demo, milestone 1, and its stand-ins`).

### Task 13: Independent check with the PAP validator

- [ ] **Step 1: Make a test identity.** In a scratch directory, run
  `node kit/company/validate/index.ts keygen --base-url https://assistant.azure.altairalabs.ai/poppy-test`
  from a checkout of `personal-agent-protocol/personal-agent-protocol`.
- [ ] **Step 2: Publish it.** Put `agent.json` and `jwks.json` in a
  ConfigMap mounted at `--test-identity-dir`, and set
  `personalAgent.testIdentity: true`. Add the validator's `client_id` to
  Larkmere's `poppy.allow` for this run only.
- [ ] **Step 3: Run it.**
  `node kit/company/validate/index.ts check https://larkmere.azure.altairalabs.ai --milestone interfaces`.
  Expected: exit 0. Record any SHOULD warnings.
- [ ] **Step 4: Classify each MUST failure** under AGENTS.md: our
  configuration, a stand-in, or an Omnia bug. Report only Omnia bugs
  upstream, quoting the spec.
- [ ] **Step 5: Undo the test entries.** Remove the validator from
  `poppy.allow`, set `testIdentity: false`, and redeploy. Commit nothing
  that keeps them.

---

## Milestone 2 (outline: its own design first, per the spec's Risks)

- **Issuer:** `demos/demoidp` becomes an OAuth issuer per Company, on
  `auth.larkmere.` and `auth.wrenmoor.azure.altairalabs.ai`:
  - RFC 8414 metadata with the Company domain in `protected_resources`;
  - authorization code with PKCE S256 and a consent page;
  - `private_key_jwt` client authentication, fetching the assistant's
    public keys;
  - RS256 `at+jwt` tokens.
- **Workspaces:** `auth.issuer`, `scopes` and `scopeHints`. Watch the
  `PoppyIssuer` condition and the identifier switch.
- **Personal agent:** Direct Sign-In (`/signin/callback`), refresh, and a
  `sign_in` tool that asks Ellen on the page.
- **Company tools that act as Ellen** move to `oauthOBO` with
  `tokenExchange`:
  - Larkmere `claim_open` and `claim_add_item`;
  - Wrenmoor outage duration and compensation.

  The stubs accept the exchanged token.
- **Demo:** story steps 3 and 4, the freezer handoff.

## Milestone 3 (outline: blocked on Omnia #4421 and #4422)

- Larkmere's settlement over £500 is referred by the existing ToolPolicy
  rule.
- Past the in-session window, the Company Agent's follow-up reply is
  "a claims handler will look at this and we'll get back to you".
- The decision arrives as a later Larkmere `message` event. The reader
  publishes it, and the page tells Ellen.
- No change to the personal agent is expected. Its reader already handles
  late messages.
