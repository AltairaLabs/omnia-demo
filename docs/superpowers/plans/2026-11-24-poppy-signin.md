# Poppy personal agent, milestone 2 (signed in): implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Story steps 3 and 4 of the storm demo. Ellen signs in to Larkmere
Mutual and Wrenmoor Energy through her assistant (PAP Direct Sign-In). Each
Company Agent then acts on her account through `oauthOBO` tools, and the
assistant carries Wrenmoor's outage length to Larkmere's claim.

**Architecture:**
- **The issuer is not built here.** It is Omnia's Poppy issuer gateway
  (Omnia #4443), in front of an Auth0 tenant. This repo builds the two sides
  that meet it.
- **Personal agent (`demos/personal-agent`):**
  - a new package `signin` holds the issuer metadata check, Direct Sign-In,
    the code exchange with a `private_key_jwt` assertion, refresh, and an
    encrypted token store;
  - `conversation.Client` sends `Authorization: Bearer` plus `ath` on a
    signed-in Company's requests;
  - the server gains a `sign_in` tool, a public `GET /signin/callback`, and a
    sign-in state on `read_conversation` and `/api/updates`.
- **Company side:**
  - the stub binary (`demos/mortgage-underwriting`) gains account routes that
    accept only a verified exchanged JWT;
  - the chart gains, per Company: the Workspace `auth` block, a
    `tokenExchange` Connection, an account ToolRegistry of `oauthOBO`
    handlers with its ToolPolicy, and an HTTPRoute that publishes those
    routes on the Company's host;
  - each Company Agent's pack gains the tools and the sign-in rule.

**Tech Stack:**
- Go 1.26: standard library, `github.com/golang-jwt/jwt/v5` and
  `github.com/redis/go-redis/v9`, both already in `go.mod`. No new module.
  AES-256-GCM comes from `crypto/aes` and `crypto/cipher`.
- Helm (`charts/omnia-demo`), PromptKit packs (`hack/build-packs.sh`,
  `promptarena validate`), and the demo-sites React kits (esbuild).

**Spec:** `docs/superpowers/specs/2026-11-23-poppy-personal-agent-design.md`
(milestone 2 as revised 2026-11-24). Protocol: PAP 1.0 §4.1, §4.2, §4.4,
§4.5, §4.7, §5.2–5.5 and §8.11. Omnia, at origin/main:
- `how-to/agents/let-personal-agents-sign-in.md`;
- `reference/platform/poppy-facade.md` ("Access tokens", "Account binding",
  "Sign-in during a turn");
- `reference/core/connection.md` ("On-behalf-of credentials", "Which token
  goes where");
- `reference/core/toolregistry.md` (`oauthOBO`);
- `reference/core/workspace.md` ("Company sign-in").

The gateway's surface is as written in Omnia #4443.

## Global Constraints

- **No loosened gates:**
  - never `personalAgents.any`;
  - never `facade.poppy.insecureTestHosts`;
  - never trust `X-Omnia-User-Email` (or any header but a verified bearer
    token) on an account tool. A missing or invalid token is refused with
    401, and there is no fallback.
- **Licensing:** nothing copied from Omnia's commercial code (`omnia/test/poppyagent`
  or the gateway). Write from the PAP spec.
- **Client assertion (PAP §4.1, RFC 7523):**
  - signed with the identity ES256 key, `kid` that key's id in `jwks.json`;
  - `iss` = `sub` = the `client_id`;
  - `aud` is the Company's issuer as a single string, never an array and
    never the token endpoint;
  - `exp` is at most 5 minutes after `iat` (use 60 seconds);
  - a unique, unguessable `jti` (128 random bits, base64url).
- **Authorization request (PAP §4.2, §4.7):**
  - `response_type=code`, `client_id`, `redirect_uri` exactly
    `https://assistant.azure.altairalabs.ai/signin/callback`;
  - `scope` always present;
  - `state` 128 random bits, single use, valid 10 minutes;
  - `code_challenge` S256 of a 32-byte random verifier,
    `code_challenge_method=S256`;
  - `prompt=consent`, and no `resource`.
- **Redirect (RFC 9207):** `iss` must equal the issuer the sign-in started
  with, character for character, on success and on `error=access_denied`.
  A redirect without `iss` is refused.
- **Tokens:**
  - stored per Company identifier and User ID, sealed with AES-256-GCM under
    a key from a Secret;
  - never logged, and never in a tool answer, an update, a page or a URL;
  - `token_type` must be `Bearer` (Omnia's facade refuses DPoP);
  - a response granting a scope that was not requested is refused (§4.7);
  - a refresh token is required from sign-in, and a rotated one replaces it
    (§4.4).
- **Refresh:** within 60 seconds of `exp`, before the request. Once more on
  a `401` `invalid_token`. On `invalid_grant`, delete the tokens and continue
  signed out (§5.5).
- **Session JWT:** `ath` = base64url SHA-256 of the access token whenever
  one is sent, and absent otherwise (existing `identity.SessionClaims.AccessToken`).
- **Public routes:** the assistant's HTTPRoute gains exactly one Exact path,
  `/signin/callback`. `/tools/*` and `/api/*` stay private. Each Company host
  gains only the Exact paths of its own account routes.
- **No `@refer` on an account tool.** An OBO call can't be held (Omnia #4257,
  closed as intended). The account tools' ToolPolicy uses `permit` and
  `forbid` only.
- **People and companies:** only Larkmere Mutual, Wrenmoor Energy and Halvern
  Partners exist. The person is Ellen Whitcomb, `ellen.whitcomb@example.com`,
  HX7 6QA.
- **Stand-ins:** every fake gets a `docs/stand-ins.md` entry and a comment
  where it is built: what it fakes, the real system, what changes downstream.
- **kubectl:** every command names `--context aks-omnia-demo`.
- **CI:** `go test ./...`, `go vet ./...`, `go mod tidy` with no diff,
  `helm lint charts/omnia-demo`, and
  `helm template omnia-demo charts/omnia-demo`. Commits are conventional and
  signed off (`git commit -s`).
- **Gated verification:** a step marked **[needs #4443 + Auth0]** needs the
  gateway deployed and an Auth0 tenant with Ellen's user. Everything else
  passes without them: every code path is tested against a fake issuer
  (`httptest.NewTLSServer`) in the tests.

## Review Focus

- **A callback that isn't the one we started.** This covers an unknown,
  expired or reused `state`, an `iss` that differs or is missing, and an
  `error=access_denied`. Each must store nothing, exchange nothing, and tell
  Ellen plainly. A decline must leave the conversation open and the
  assistant able to say she declined. (Task 4:
  `TestCompleteRefusesUnknownState`, `TestCompleteRefusesReusedState`,
  `TestCompleteRefusesIssuerMismatchEvenOnDecline`,
  `TestCompleteDeclineStoresNothing`.)
- **Fewer scopes, or more, than asked.** A response without `scope` grants
  exactly what was asked. With `scope`, it may grant a subset, which is
  recorded, and the row stays "needs sign-in" when the needed scope wasn't
  granted. A grant beyond the request is refused outright. (Task 4:
  `TestCompleteRecordsSubsetOfScopes`, `TestCompleteRefusesExtraScopes`.)
- **The token expiring mid-conversation.** It is refreshed ahead of `exp`.
  A `401` `invalid_token` refreshes once and retries with a fresh Session JWT
  and `ath`. `invalid_grant` drops the tokens, and the Reader then stops with
  "needs sign-in" instead of looping. (Task 5:
  `TestRefreshBeforeExpiry`, `TestInvalidTokenRefreshesOnceAndRetries`,
  `TestInvalidGrantDropsTokensAndStopsSignedOut`.)
- **Tokens reaching the wrong place.** A token must never go to another
  Company's identifier or User ID, and never to an endpoint whose RFC 9728
  metadata names its own resource. A sealed blob copied to another key must
  not open. (Task 3: `TestSealedTokenWontOpenUnderAnotherKey`. Task 5:
  `TestTokenNeverSentToAnotherCompany`,
  `TestTokenWithheldFromEndpointWithOwnResource`.)
- **The stub fed the wrong token.** Each of these must be refused with no
  fallback to the email header:
  - the Personal Agent's own access token replayed at the API (wrong `aud`);
  - a token from the other Company's issuer;
  - an expired token, or one with `alg` other than RS256;
  - a token without the scope;
  - a linked `sub` absent from the account links.
  
  (Task 9 tests, and `TestAccountRouteIgnoresEmailHeader`.)

---

## The personal agent

### Task 1: Discovery reads the Company's sign-in facts

**Files:**
- Modify: `demos/personal-agent/discovery/discovery.go`
- Test: `demos/personal-agent/discovery/discovery_test.go`

**Interfaces:**
- Produces, in package `discovery`:
  - `Company` gains:
    - `SignInDisabled bool`, from `poppy.json` `auth.disabled` (PAP §3.2);
    - `Scopes map[string]string`, from `auth.scopes`;
    - `ScopesSupported []string`, from the protected resource metadata's
      `scopes_supported`.
  - `AgentEntry` gains `Resource string`. It is the RFC 9728 `resource` of
    the endpoint's own metadata, or "" when that answers 404 or its
    `resource` isn't the endpoint URL (RFC 9728 §3.3).
  - `func EndpointResource(ctx context.Context, hc *http.Client, endpoint string) (string, error)`.
    It fetches `https://<host>/.well-known/oauth-protected-resource<path>`
    with no redirects. A 404 answers "". Any other non-200 is an error.
  - `fetchIssuer` also checks that the metadata's `resource` is exactly
    `https://<domain>`, and errors otherwise.
  - `Discover` calls `EndpointResource` for each poppy agent only when the
    Company has an issuer.

- [ ] **Step 1: Write the failing tests.**
  - `TestDiscoverReadsAuthDisabledAndScopes`: `auth.disabled: true` and
    `auth.scopes {"claims": "..."}` reach `Company`.
  - `TestDiscoverReadsScopesSupported`: from protected resource metadata
    `{"resource":"https://<host>","authorization_servers":[iss],"scopes_supported":["claims"]}`.
  - `TestDiscoverRejectsProtectedResourceForOtherResource`: `resource`
    `https://elsewhere.example` gives an error that names `resource`.
  - `TestEndpointResource404IsDefault`: answers "" with a nil error.
  - `TestEndpointResourceMismatchIsAbsent`: a `resource` that isn't the
    endpoint URL answers "".
- [ ] **Step 2: Run them and see them fail.**
  `go test ./demos/personal-agent/discovery` fails to compile on the missing
  fields.
- [ ] **Step 3: Implement** the fields and `EndpointResource`. Reuse
  `fetchIssuer`'s no-redirect client copy.
- [ ] **Step 4: Run them and see them pass.**
  `go test ./demos/personal-agent/...` passes, with existing tests
  unchanged.
- [ ] **Step 5: Commit**
  (`feat(personal-agent): read a company's sign-in facts at discovery`).

### Task 2: Issuer metadata and the client assertion

**Files:**
- Create: `demos/personal-agent/identity/assertion.go`
- Create: `demos/personal-agent/signin/metadata.go` (package doc says: PAP
  Direct Sign-In, written from the spec)
- Test: `demos/personal-agent/identity/identity_test.go`,
  `demos/personal-agent/signin/signin_test.go`

**Interfaces:**
- Produces:
  - `func (i *Identity) ClientAssertion(issuer string, now time.Time) (string, error)`.
    ES256 with the identity key, header `kid` = `es256Kid`. Claims are
    `iss` and `sub` (the `ClientID`), `aud` (the issuer, a string), `iat`,
    `exp = iat+60s`, and `jti`. An empty issuer is an error.
  - `type Metadata struct { Issuer, AuthorizationEndpoint, TokenEndpoint, RevocationEndpoint string; GrantTypes, ScopesSupported, CodeChallengeMethods []string; RequirePAR bool }`.
  - `func FetchMetadata(ctx context.Context, hc *http.Client, issuer, domain string) (*Metadata, error)`:
    - fetches RFC 8414 §3.1: issuer host + `/.well-known/oauth-authorization-server`
      + the issuer's path. HTTPS only, no redirects, at most 1 MiB;
    - `issuer` must equal the argument exactly;
    - `protected_resources` must include `https://<domain>`, compared with
      the host lowercased;
    - `token_endpoint` and `authorization_endpoint` must be https;
    - `grant_types_supported` must hold `authorization_code` and
      `refresh_token`;
    - `code_challenge_methods_supported`, when present, must hold `S256`;
    - `require_pushed_authorization_requests: true` is an error saying PAR
      is not supported.
    
    Each failure's error names the field.

- [ ] **Step 1: Write the failing tests.**
  - `TestClientAssertionClaims`:
    - it parses with the identity public key;
    - `aud` is a JSON string equal to the issuer, not an array;
    - `iss == sub == ClientID`;
    - `exp-iat <= 300` and `> 0`;
    - two calls give two `jti` values;
    - header `alg` is `ES256` and `kid` is the ES256 key's thumbprint.
  - `TestFetchMetadataPathIssuer`: issuer `https://<host>/tenant` is
    fetched at `/.well-known/oauth-authorization-server/tenant`.
  - `TestFetchMetadataRejects`, table-driven. Each case expects an error
    naming the field:
    - issuer with a trailing slash;
    - `protected_resources` missing the domain;
    - no `authorization_code` grant;
    - `code_challenge_methods_supported: ["plain"]`;
    - `require_pushed_authorization_requests: true`;
    - a 302 to another path.
  - `TestFetchMetadataHostCaseInsensitive`:
    `protected_resources: ["https://LARKMERE.example"]` matches domain
    `larkmere.example`.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them and see them pass.**
  `go test ./demos/personal-agent/...`.
- [ ] **Step 5: Commit**
  (`feat(personal-agent): check an issuer's metadata and sign client assertions`).

### Task 3: A sealed token store

**Files:**
- Create: `demos/personal-agent/signin/tokens.go`
- Test: `demos/personal-agent/signin/tokens_test.go`

**Interfaces:**
- Produces:
  - `type Tokens struct { Access, Refresh string; Scopes []string; Expiry time.Time }`.
  - `type TokenStore interface { Get(identifier, userID string) (*Tokens, error); Put(identifier, userID string, t Tokens) error; Delete(identifier, userID string) error }`.
    `Get` answers `nil, nil` when there are none.
  - `func NewMemoryTokenStore() *MemoryTokenStore`, for tests.
  - `func NewRedisTokenStore(url string, key []byte) (*RedisTokenStore, error)`.
    The Redis key is `pa:tokens:<identifier>|<userID>`. The value is
    `nonce || AES-256-GCM(json(Tokens))` with additional data
    `identifier + "\n" + userID`. A blob moved to another key fails to open,
    and is reported as an error, never as "no tokens".
  - `func LoadKey(path string) ([]byte, error)`: reads one line of standard
    base64 that must decode to exactly 32 bytes.

- [ ] **Step 1: Write the failing tests.**
  - `TestMemoryTokenStoreRoundTrip`.
  - `TestLoadKeyRejectsWrongLength`: 16 bytes is an error.
  - `TestRedisTokenStoreSealed` (skips without `PA_TEST_REDIS_URL`, as
    `TestRedisStoreEventsAndState` does). After `Put`, the raw Redis value
    contains neither the access nor the refresh token. `Get` gives them back.
  - `TestSealedTokenWontOpenUnderAnotherKey`: copy the raw value to
    another identifier's key, and `Get` there errors. Test the seal and open
    helpers directly, so this runs without Redis.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.** The seal and open helpers are unexported
  functions taking the key and additional data, so the test above can call
  them.
- [ ] **Step 4: Run them and see them pass.**
- [ ] **Step 5: Commit**
  (`feat(personal-agent): keep sign-in tokens sealed per company and user`).

### Task 4: Direct Sign-In: begin and complete

**Files:**
- Create: `demos/personal-agent/signin/flow.go`
- Create: `demos/personal-agent/signin/signintest/issuer.go`, the fake
  issuer, a non-test package like `net/http/httptest` so that Tasks 5 and 6
  can reuse it:
  - `func NewIssuer(t testing.TB, id *identity.Identity, domain string) *Issuer`
    wraps an `httptest.NewTLSServer` serving RFC 8414 metadata and a token
    endpoint;
  - the token endpoint verifies the client assertion against `id.JWKS()`
    and checks `code_verifier`. It answers `authorization_code` and
    `refresh_token` grants from canned responses the test sets
    (`Issuer.Next(status int, body map[string]any)`), and records each
    request (`Issuer.Requests()`);
  - `Issuer.URL` is the issuer, and `Issuer.Client()` is the HTTP client
    that trusts it.
- Test: `demos/personal-agent/signin/flow_test.go`.

**Interfaces:**
- Consumes:
  - `identity.(*Identity).ClientAssertion` and `RedirectURI`;
  - `FetchMetadata` and `TokenStore`;
  - `discovery.Company`.
- Produces:
  - `type Flow struct { HTTP *http.Client; Identity *identity.Identity; Tokens TokenStore; Now func() time.Time }`,
    plus unexported pending sign-ins in memory (state → company domain,
    identifier, issuer, User ID, verifier, scopes, expiry).
  - `func NewFlow(hc *http.Client, id *identity.Identity, ts TokenStore) *Flow`.
  - `func (f *Flow) Begin(ctx context.Context, co *discovery.Company, userID string, scopes []string) (string, error)`.
    It returns the authorization URL. It errors when:
    - `co.Issuer == ""` or `co.SignInDisabled` ("<name> offers no sign-in");
    - a scope is not in `co.ScopesSupported`, the metadata's
      `scopes_supported` or `co.Scopes` (the error lists the offered ones).
    
    The requested scopes are the union of `scopes` and the scopes already
    granted for this identifier and User ID (§4.7), sorted.
  - `type Result struct { Domain, Name string; Granted []string; Declined bool }`.
  - `func (f *Flow) Complete(ctx context.Context, q url.Values) (Result, error)`:
    - looks up and deletes the pending sign-in by `state` before anything
      else, so a state is single use;
    - checks that it hasn't expired;
    - checks `iss`;
    - on `error=access_denied`, answers `Declined: true` and stores nothing;
    - otherwise POSTs the token request (form fields: `grant_type`,
      `code`, `redirect_uri`, `code_verifier`, `client_id`,
      `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer`,
      `client_assertion`) and checks the response as Global Constraints
      say;
    - a missing `expires_in` is taken as 5 minutes;
    - an empty granted set is a decline;
    - stores the `Tokens`.
  - Exported errors: `ErrUnknownState`, `ErrIssuerMismatch`.

- [ ] **Step 1: Write the failing tests.**
  - `TestBeginBuildsAuthorizationURL`:
    - `response_type=code`, `client_id`, the exact `redirect_uri`,
      `prompt=consent`, `code_challenge_method=S256`;
    - `scope` is space-separated;
    - no `resource`;
    - `state` is at least 22 characters.
  - `TestBeginRefusesNoIssuerAndDisabled`.
  - `TestBeginRefusesUnofferedScope`: the error lists the offered scopes.
  - `TestBeginRequestsPreviouslyGrantedScopesToo`.
  - `TestCompleteExchangesCode`. The fake token endpoint sees:
    - the S256 of the verifier;
    - an assertion whose `aud` is the issuer string;
    - the same `redirect_uri`.
    
    Afterwards `Tokens.Get` holds the access and refresh tokens.
  - `TestCompleteRefusesUnknownState`.
  - `TestCompleteRefusesReusedState`: a second `Complete` with the same
    query gives `ErrUnknownState`.
  - `TestCompleteRefusesIssuerMismatchEvenOnDecline`: a missing `iss` is
    refused too.
  - `TestCompleteDeclineStoresNothing`.
  - `TestCompleteRecordsSubsetOfScopes` and `TestCompleteRefusesExtraScopes`.
  - `TestCompleteRefusesDPoPAndMissingRefreshToken`.
  - `TestFlowNeverLogsSecrets`: capture `log` output over begin and
    complete. It contains no code, verifier, state, access token or refresh
    token.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.** Pending sign-ins live in memory only: one
  replica, and a restart between begin and callback means Ellen signs in
  again. Say so in a comment. The verifier is 32 random bytes, base64url,
  and the challenge is base64url(SHA-256(verifier)).
- [ ] **Step 4: Run them and see them pass.**
- [ ] **Step 5: Commit**
  (`feat(personal-agent): PAP Direct Sign-In with PKCE and a private_key_jwt exchange`).

### Task 5: Signed-in requests: refresh, Bearer and `ath`

**Files:**
- Modify: `demos/personal-agent/signin/flow.go`
- Modify: `demos/personal-agent/conversation/client.go`
- Modify: `demos/personal-agent/conversation/reader.go`
- Test: `demos/personal-agent/signin/flow_test.go`,
  `demos/personal-agent/conversation/conversation_test.go`

**Interfaces:**
- Consumes: Task 4's `Flow` and `TokenStore`.
- Produces:
  - In `conversation`:
    ```go
    // TokenSource gives the access token to send to a Company for a User ID.
    type TokenSource interface {
        // AccessToken is the current token, refreshed when it expires within a minute; "" for none.
        AccessToken(ctx context.Context, co *discovery.Company, userID string) (string, error)
        // Refresh refreshes after the Company refused the token; "" when the tokens are gone.
        Refresh(ctx context.Context, co *discovery.Company, userID string) (string, error)
    }
    ```
    - `Client` gains `Tokens TokenSource`; nil means always signed out.
    - `const CodeSignInRequired = "sign_in_required"`.
  - `(*signin.Flow)` implements `TokenSource`:
    - a refresh posts `grant_type=refresh_token`, `refresh_token` and the
      client assertion, with no `scope`;
    - a new refresh token replaces the stored one;
    - `invalid_grant` deletes the tokens and answers "";
    - a network error or 5xx with an unexpired token answers that token;
    - one mutex per identifier and User ID, so two Readers don't refresh at
      once.
  - `Client.send`:
    - asks `Tokens.AccessToken` for `co` and the User ID;
    - withholds the token when the endpoint's agent has a `Resource` other
      than `https://<domain>` (Task 1);
    - sets `Authorization: Bearer <t>` and passes `AccessToken: t` to
      `SignSession`.
  - `Client.do`: a 401 is parsed from `WWW-Authenticate: Bearer error="..."`
    into `Error.Code`.
    - If a token was sent and the code is `invalid_token`, it calls
      `Refresh` once and resends, re-signed. This doesn't count as an
      attempt, like `session_ended`.
    - Any other 401 is returned as `*Error{Status: 401, Code: CodeSignInRequired}`.
  - `Reader.Run`: unchanged, except that a 401 stops with reason
    `sign_in_required (HTTP 401)`, through the existing 4xx path.

- [ ] **Step 1: Write the failing tests.**
  - `TestRefreshBeforeExpiry`: a token with 30s left is refreshed. A
    rotated refresh token replaces the old one.
  - `TestInvalidGrantDropsTokensAndStopsSignedOut`. Refresh answers
    `invalid_grant`, so the store is empty. The fake Company then answers
    `401` with `WWW-Authenticate: Bearer`, and the Reader records stopped
    `sign_in_required (HTTP 401)`.
  - `TestSignedInRequestCarriesBearerAndAth`: the fake Company sees
    `Authorization: Bearer t1`, and the Session JWT's `ath` is the hash of
    `t1`.
  - `TestSignedOutRequestHasNoAth`, with `Tokens` nil.
  - `TestInvalidTokenRefreshesOnceAndRetries`. The first answer is
    `401 WWW-Authenticate: Bearer error="invalid_token"` and the second is
    201. There are two requests with distinct `jti` values, the second
    carries the refreshed token and its `ath`, and the message is not
    re-sent a third time.
  - `TestTokenNeverSentToAnotherCompany`: tokens for identifier A, and a
    request to Company B carries no `Authorization`.
  - `TestTokenWithheldFromEndpointWithOwnResource`.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.** Keep the token lookup inside `send`, so every
  attempt reads the current token and signs at send time.
- [ ] **Step 4: Run them and see them pass.**
  `go test ./demos/personal-agent/...`.
- [ ] **Step 5: Commit**
  (`feat(personal-agent): send Bearer and ath when signed in, and refresh before expiry`).

### Task 6: The sign_in tool, the callback and sign-in state

**Files:**
- Modify: `demos/personal-agent/server.go`, `demos/personal-agent/main.go`
- Test: `demos/personal-agent/server_test.go`

**Interfaces:**
- Consumes:
  - `signin.Flow` (`Begin`, `Complete`, `TokenSource`) and
    `signin.TokenStore`;
  - `conversation.CodeSignInRequired`.
- Produces:
  - `config` gains `SignIn *signin.Flow`. When it is nil, `sign_in` answers
    400 "sign-in isn't set up on this assistant". `newServer` sets
    `client.Tokens = cfg.SignIn` when it is non-nil.
  - `POST /tools/sign_in`:
    - takes `{"domain","scopes"}`, where scopes is a space-separated string;
    - answers `{"company","scopes","authorization_url","status":"pending","note":"A 'Sign in to <name>' button is on Ellen's screen; she signs in on <name>'s own page."}`;
    - publishes `{"type":"signin","status":"pending","scope":...,"authorization_url":...}`
      on every open conversation with that domain.
    
    `toolRequest` gains `Scopes string`.
  - `GET /signin/callback` (public):
    - calls `Complete`;
    - on success, for each conversation of that domain stopped with
      `sign_in_required`, calls `SetStopped(id, "")` then `follow`;
    - publishes `{"type":"signin","status":"signed_in"|"declined"|"failed","scope":...}`
      on that domain's conversations;
    - answers a small HTML page: "Signed in to <name>. You can close this
      tab and go back to your assistant." Or, on refusal, the reason in
      plain words.
    
    The page sends `Cache-Control: no-store`, a
    `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'`,
    and `html/template` escaping. It carries no token, code or state.
  - `readOut` gains
    `SignIn *signInOut \`json:"sign_in,omitempty"\``, with
    `type signInOut struct { Needed bool; Scopes, Status string }` (JSON
    `needed`, `scopes`, `status`). It is computed in `read`:
    - `needed` when the latest `authorization` event's scopes aren't all
      held in the token store for that Company's identifier and User ID, or
      when stopped starts with `sign_in_required`;
    - `status` is one of `needed`, `pending` (a sign-in begun and not
      completed), `signed_in` or `declined`;
    - nil when neither applies.
    
    `changed` also wakes a waiting read when `SignIn` changes.
  - Flags: `--token-key-file`. It is required with `--redis-url`, when the
    service uses `signin.NewRedisTokenStore`. Without Redis it uses
    `signin.NewMemoryTokenStore`.
  - `main.go`'s stand-in comment adds: the sign-in redirect lands on a
    public callback that cannot tell who completed it (PAP §4.2, §10.8).

The Reader needs no change for the `authorization` event: it already stores
every event that has an `id`, whatever its type, so `read` finds it in
`Store.Events`.

- [ ] **Step 1: Write the failing tests** with a fake Company and
  `signintest.NewIssuer` (Task 4).
  - `TestAuthorizationEventNeedsSignIn`: after the Reader stores an
    `authorization` event with `scope: "claims"`, `read_conversation`
    answers `sign_in: {needed: true, scopes: "claims", status: "needed"}`.
  - `TestSignInToolReturnsURLNeverToken`: the answer has
    `authorization_url` and no field whose value contains the token, code
    or verifier. An update with `type: signin` and `status: pending` is
    published.
  - `TestCallbackSignsInAndResumesStoppedReader`. A conversation is stopped
    with `sign_in_required`. After the callback, its Reader runs again and
    its next request to the fake Company carries `Authorization`. The
    callback page's body contains no token.
  - `TestCallbackDeclineKeepsConversationOpen`: `status: declined`, and the
    conversation is not closed.
  - `TestCallbackUnknownStateIsPlainRefusal`: 400 and an HTML reason, with
    no update published.
  - `TestToolsNeverExposeUserIDOrKeys`: extend it to cover `sign_in` and
    `read_conversation` after sign-in, so no body contains an access or
    refresh token.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them and see them pass.**
  `go test ./demos/personal-agent/...` and `go vet ./...`.
- [ ] **Step 5: Commit**
  (`feat(personal-agent): a sign_in tool, the sign-in callback, and sign-in state for the assistant`).

### Task 7: The assistant's side: chart, keys and prompt

**Files:**
- Modify: `charts/omnia-demo/templates/personal-agent.yaml`:
  - the HTTPRoute gains `- path: { type: Exact, value: /signin/callback }`.
    Its comment says this is the one route added, and why it is public;
  - the Deployment gains the `--token-key-file /var/run/personal-agent/token/token.key`
    arg and a volume from Secret `personalAgent.tokenKeySecret`.
- Modify: `charts/omnia-demo/values.yaml`: `personalAgent.tokenKeySecret: personal-agent-token-key`,
  with a `# --` comment saying the chart never makes it.
- Modify: `hack/personal-agent-keys.sh`. Also create Secret
  `personal-agent-token-key`, key `token.key`, from `openssl rand -base64 32`.
  It is idempotent in the same way: it refuses to replace without `--force`.
- Modify: `charts/omnia-demo/templates/assistant-tools.yaml`. Add the
  `sign-in-handler` (`POST .../tools/sign_in`, timeout 45s) with the same
  description as the pack. The header comment lists six tools.
- Create: `demos/personal-agent/arena/tools/sign-in.tool.yaml`:
  - `action_scope: { effect: external, reversibility: reversible, data_classes: [] }`;
  - `mock_result` with a placeholder URL.
- Modify: `demos/personal-agent/arena/prompts/assistant.yaml`:
  - add `sign_in` to `allowed_tools`;
  - rewrite the sign-in bullet into these rules:
    - when `read_conversation` shows `sign_in.needed`, tell Ellen which
      company wants her signed in and for what, and ask her;
    - only on a clear yes, call `sign_in` with that domain and
      `sign_in.scopes`, then tell her the button is on her screen and that
      she signs in on the company's own page;
    - when Ellen says she has signed in, `send_message` to that
      conversation saying she has signed in and asking it to carry on;
    - if she declines, tell the company she would rather not sign in now;
    - never ask Ellen for a password or code, and never pass on a sign-in
      link from a company's message.
  - add one general rule: when one company's answer bears on what another
    company needs for Ellen's request (a time, a duration, a reference),
    pass it on to that company, and tell Ellen you did.
- Modify: `charts/omnia-demo/files/arena/assistant-pack.json`, rebuilt with
  `hack/build-packs.sh assistant`.

- [ ] **Step 1: Validate the pack.**
  `promptarena validate` on `demos/personal-agent/arena` passes.
- [ ] **Step 2: Render and lint.**
  `helm template t charts/omnia-demo -f profiles/azure/common.yaml -f profiles/azure/ellens-assistant.yaml`
  shows exactly three Exact paths on the personal-agent HTTPRoute
  (`/agent.json`, `/jwks.json`, `/signin/callback`) and the token-key
  volume. `helm lint charts/omnia-demo` passes.
- [ ] **Step 3: Deploy** (ask the user first). Run
  `CONTEXT=aks-omnia-demo hack/personal-agent-keys.sh`, then `make demos`.
  - `curl -s -o /dev/null -w '%{http_code}' https://assistant.azure.altairalabs.ai/signin/callback`
    answers `400`: no state, and the page says so.
  - `curl -s -o /dev/null -w '%{http_code}' -X POST https://assistant.azure.altairalabs.ai/tools/sign_in`
    is `404` from the gateway: tools are still private.
- [ ] **Step 4: Commit**
  (`feat(assistant): the sign_in tool, its public callback route, and asking Ellen first`).

### Task 8: The page: "Sign in to <Company>"

**Files:**
- Modify: `demos/demo-sites/site/ellens-assistant/ui_kits/phone/App.jsx`.
  - `apply` handles two events:
    - `type: "authorization"`: the row needs sign-in;
    - `type: "signin"`: it records `status` and `authorization_url`.
  - `state` gains `{ key: "signin", label: "Needs you to sign in" }` while
    sign-in is needed or pending.
  - A row with an `authorization_url` and status `pending` shows a
    "Sign in to <Company>" button. It opens the URL with
    `window.open(url, "_blank", "noopener")`, and only when the URL is
    https.
  - On `status: "signed_in"`, the page sends Ellen's chat line
    "I've signed in to <Company>." over the assistant socket, once per
    sign-in, and the row shows "Signed in".
  - On `declined`, it sends "I didn't sign in to <Company>."
- Modify: `demos/demo-sites/server_test.go`:
  `TestAssistantSiteNeverProxiesSignIn`. `/signin/callback` and
  `/tools/sign_in` through the page's site are 404: the callback is the
  personal agent's public route, never the page's.

- [ ] **Step 1: Write the failing test** and run
  `go test ./demos/demo-sites -run TestAssistantSiteNeverProxiesSignIn`. It
  may pass already; if it does, keep it as the guard and note that in the
  commit.
- [ ] **Step 2: Implement the page** and run `demos/demo-sites/hack/build-kits.sh`,
  which must build.
- [ ] **Step 3: `go test ./demos/demo-sites` passes.**
- [ ] **Step 4: Commit**
  (`feat(demo-sites): a "Sign in to <company>" action on Ellen's conversation strip`).

---

## The companies

### Task 9: The stub verifies the exchanged token

**Files:**
- Create: `demos/mortgage-underwriting/obo.go` (STAND-IN comment: the
  account links fake the Company's own account-to-identity link)
- Modify: `demos/mortgage-underwriting/main.go`, `server.go`
- Test: `demos/mortgage-underwriting/obo_test.go`, with a fake issuer
  `httptest.NewTLSServer`: RFC 8414 metadata with `jwks_uri`, an RSA key, and
  a helper that mints tokens.

**Interfaces:**
- Produces:
  - `type oboVerifier struct` (issuer, audience, account links, HTTP
    client, clock, cached RSA keys by `kid`).
  - `func newOBOVerifier(issuer, audience string, links map[string]string, hc *http.Client, now func() time.Time) *oboVerifier`.
  - `func (v *oboVerifier) member(r *http.Request, scope string) (string, int, string)`
    answers the member email, or an HTTP status and an `error` code. It
    checks, in order:
    - `Authorization: Bearer` present, else 401 `invalid_token`;
    - RS256 only;
    - `kid` in the issuer's JWKS, found through the metadata's `jwks_uri`
      and refetched on an unknown `kid` at most once every 30s;
    - `iss` equals `issuer`;
    - `aud` contains `audience`;
    - `exp` in the future;
    - `scope` (space-separated) contains `scope`, else 403
      `insufficient_scope`;
    - `sub` in links, else 404 "no account for this customer".
    
    It never reads `X-Omnia-User-Email`.
  - `func (v *oboVerifier) guard(tool, scope string, fn func(member string, w http.ResponseWriter, r *http.Request)) http.HandlerFunc`.
    POST only. A refusal sets `WWW-Authenticate: Bearer error="<code>"`
    (plus `scope="<scope>"` for 403) and a JSON `{"error"}` body. It logs
    `tool`, status and member, and never the header. A nil verifier answers
    503 "account access is not configured".
  - `func loadAccountLinks(path string) (map[string]string, error)`: a JSON
    object of token `sub` to member email.
  - Flags:
    - `--account-api` (`""`, `larkmere` or `wrenmoor`) registers that
      Company's account routes only (Tasks 10 and 11);
    - `--obo-issuer`, `--obo-audience` and `--account-links`.
    
    `newServer()` stays as it is for the existing tests and calls
    `newServerWith(accountConfig{})`.

- [ ] **Step 1: Write the failing tests.** Each runs through `guard`
  around a handler that echoes the member.
  - `TestOBOAcceptsValidToken`: Ellen's linked `sub` gives
    `ellen.whitcomb@example.com`.
  - `TestOBORefusesMissingToken`: 401 with `WWW-Authenticate: Bearer`.
  - `TestOBORefusesWrongAudience`: a token for `https://larkmere.example`,
    the facade's audience, replayed at `https://larkmere.example/api`.
  - `TestOBORefusesOtherIssuer`, `TestOBORefusesExpired` and
    `TestOBORefusesHS256AndNone`.
  - `TestOBORefusesMissingScope`: 403 `insufficient_scope`.
  - `TestOBOUnlinkedSubjectIsNoAccount`: 404.
  - `TestAccountRouteIgnoresEmailHeader`:
    `X-Omnia-User-Email: ellen.whitcomb@example.com` without a token gets
    401.
  - `TestOBOUnknownKidRefetchesOnce`: a key rotated at the issuer is
    picked up.
  - `TestOBONeverLogsToken`: captured log output has no token.
  - `TestOBONotConfiguredIs503`.
- [ ] **Step 2: Run them and see them fail.**
  `go test ./demos/mortgage-underwriting -run TestOBO` fails to compile.
- [ ] **Step 3: Implement** with golang-jwt's `jwt.WithValidMethods([]string{"RS256"})`,
  `WithIssuer`, `WithAudience` and `WithExpirationRequired`.
- [ ] **Step 4: Run them and see them pass.**
  `go test ./demos/mortgage-underwriting` passes, with existing tests
  unchanged.
- [ ] **Step 5: Commit**
  (`feat(stubs): verify the exchanged on-behalf-of token; never the email header`).

### Task 10: Larkmere: claims as the member

**Files:**
- Create: `demos/mortgage-underwriting/homeclaims.go`
- Create: `demos/mortgage-underwriting/homeclaimsdata.go`:
  - home policies by email: Ellen `LM-HOME-4471`, 4 Mytholm Steeps,
    Hebden Bridge, HX7 6QA, buildings and contents;
  - Gwen Ashby `LM-HOME-5133`.
- Test: `demos/mortgage-underwriting/homeclaims_test.go`

**Interfaces:**
- Consumes: `oboVerifier.guard` (Task 9), with scope `claims`.
- Produces, registered when `--account-api larkmere`:
  - `POST /api/claims/open`:
    - takes `{"peril","description","occurred_on"}`;
    - answers 201 `{"claim_ref","policy_ref","member_name","peril","description","occurred_on","reported_on","status":"reported","items":[],"claimed_pence":0,"next_steps"}`;
    - `claim_ref` runs from `LM-41001`;
    - no home policy is 404 "no home policy for this member".
  - `POST /api/claims/add-item`:
    - takes `{"claim_ref","description","value_pence","cause"}`;
    - answers the claim with the item appended and `claimed_pence` summed;
    - another member's claim, or an unknown one, is 404 "claim not found",
      the same answer for both;
    - `value_pence <= 0` or an empty description is 400;
    - a claim not `reported` is 409.
  - `POST /api/claims/status`: takes `{"claim_ref"}`, which is optional.
    It answers that claim, or `{"claims":[...]}` for every claim this
    member opened.
  - `type homeClaims struct` with a mutex, because these routes are written
    concurrently, unlike the claims-triage maps.
  - Money in pence, as `claimsdata.go` says. The stub enforces no £500
    limit, as `claimsdata.go` says: milestone 3 adds the policy.

- [ ] **Step 1: Write the failing tests** with tokens from Task 9's fake
  issuer.
  - `TestClaimOpenForSignedInMember`.
  - `TestClaimAddItemSumsPence`: a kitchen item, then "freezer contents"
    with cause "power off 19 hours (Wrenmoor Energy)", and `claimed_pence`
    is the sum.
  - `TestClaimAddItemOtherMembersClaimIsNotFound`: Gwen's token on Ellen's
    claim gets 404.
  - `TestClaimStatusListsOnlyOwnClaims`.
  - `TestClaimRoutesAbsentWithoutAccountAPI`: with `newServer()`,
    `/api/claims/open` is 404.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them and see them pass.**
- [ ] **Step 5: Commit**
  (`feat(larkmere): report a home claim and add to it, as the signed-in member`).

### Task 11: Wrenmoor: her outage and compensation

**Files:**
- Modify: `demos/mortgage-underwriting/wrendata.go`. Add
  `wrenAccounts = map[string]wrenCustomer` keyed by email: Amira, and
  `wrenEllen`. Update `wrenEllen`'s STAND-IN comment: the account tools now
  serve her, and the older bill tools still don't.
- Create: `demos/mortgage-underwriting/compensation.go`. Its STAND-IN
  comment says the compensation rule is Wrenmoor's own fictional guaranteed
  standard: £95 once the supply has been off 12 hours, then £45 for each
  further full 12 hours, paid automatically within 28 days.
- Test: `demos/mortgage-underwriting/compensation_test.go`

**Interfaces:**
- Consumes:
  - `oboVerifier.guard` with scope `outages`;
  - `(*wren).outage(district)` and `postcodeDistrict` (`outages.go`).
- Produces, registered when `--account-api wrenmoor`:
  - `POST /api/account/outage` answers
    `{"account_id","postcode","district","status","started","hours_off","estimated_restoration"}`
    for the token's customer, from their postcode's district.
    - `hours_off` is whole hours from `started` to `w.now()`, rounded down.
    - With no outage in the district it answers `{"status":"on"}` and 200.
  - `POST /api/account/compensation` answers
    `{"account_id","hours_off","eligible","amount_pence","rule","status"}`.
    `status` is `owed` when eligible, else `not_yet`.
  - `func compensationPence(hoursOff int) int`: 0 below 12, 9500 at 12–23,
    14000 at 24–35, and so on.

- [ ] **Step 1: Write the failing tests.**
  - `TestCompensationPence`, a table: 11→0, 12→9500, 23→9500, 24→14000,
    36→18500.
  - `TestMyOutageForEllen`: with a fixed clock, `district=="HX7"` and
    `hours_off` is the clock's distance from `started`.
  - `TestCompensationOwedToEllen`: `status=="owed"` and `amount_pence`
    matches `compensationPence(hours_off)`.
  - `TestMyOutageServesTokenCustomerOnly`: Amira's token (her HX7 8DJ is
    HX7 too) is served her own `account_id`, never Ellen's.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run them and see them pass.**
- [ ] **Step 5: Commit**
  (`feat(wrenmoor): the signed-in customer's outage and compensation`).

### Task 12: Chart: Company sign-in and the account tools

**Files:**
- Modify: `charts/omnia-demo/templates/workspace.yaml`. Under the Company,
  when `.Values.poppy.auth.enabled`, render `auth:`:
  - `scopes`, `scopeHints`, and `accessTokens.audiences`, `clientIDClaim`
    and `scopeClaim`, from values;
  - either `issuer: <poppy.auth.issuer>`, or, when
    `poppy.auth.gateway.enabled`, the gateway block exactly as Omnia #4443
    proposes:
    `gateway: { upstream: { issuer, clientID, clientSecretRef: {name} }, host }`.
  
  A comment says the `gateway` field does not exist until #4443 ships, so
  it stays off.
- Create: `charts/omnia-demo/templates/poppy-account-tools.yaml`, rendered
  when `poppy.enabled`, `poppy.agent.enabled` and `poppy.auth.enabled`. A
  dict keyed by `poppy.agent.company` picks the tools:
  - larkmere: `claim_open`, `claim_add_item` and `claim_status` on
    `/api/claims/{open,add-item,status}`, backend Service `claims-systems`;
  - wrenmoor: `my_outage` and `compensation_status` on
    `/api/account/{outage,compensation}`, backend Service `wren-account`.
  
  It renders:
  - a **Connection** `<agent>-account`: `hosts: [<poppy.domain>]`, and
    `credential.kind: tokenExchange` with `tokenURL`, `clientID`,
    `clientSecretRef`, `scopes`, `audience`, `subjectIssuer` and
    `encryption`, all from `poppy.account.*`;
  - a **ToolRegistry** `<agent>-account-tools`: one `http` handler per
    tool, `endpoint: https://<poppy.domain><path>`, `method: POST`, and
    `auth: { type: oauthOBO, oauthOBO: { connection: <agent>-account, scopes: [<the Company's one scope>] } }`;
  - a **ToolPolicy** `<agent>-account-policy` on that registry, in
    `mode: enforce` and `onFailure: deny`:
    - `permit` the registry;
    - `forbid` undeclared tools;
    - `forbid` `tool.effect == "external"`.
    
    Its comment says every handler in this registry acts as the signed-in
    customer, which is why writes are permitted here and nowhere else, and
    that there is no `@refer` (Omnia #4257).
  - an **HTTPRoute** `<agent>-account-api` in the tenant namespace on
    `poppy.domain`. It has only the tools' Exact paths, to the stub Service
    on 8080, and parentRef `omnia-external`. Its comment says each path
    refuses any request without a verified token.
- Modify: `charts/omnia-demo/templates/poppy-company-agent.yaml`:
  - add `<agent>-account-tools` to `toolRegistryRefs` when
    `poppy.auth.enabled`, for both companies;
  - fail the render for larkmere without `claimsTriage.enabled`, as
    wrenmoor already does for `wren.enabled`.
- Modify: the stub Deployments `wren-agent.yaml` (`wren-account`) and
  `claims-triage-agent.yaml` (`claims-systems`). When `poppy.auth.enabled`
  and this tenant's company matches, add these args:
  - `--account-api <company>`;
  - `--obo-issuer <poppy.account.subjectIssuer>` and
    `--obo-audience <poppy.account.audience>`;
  - `--account-links /etc/account-links/links.json`, from a ConfigMap
    rendered from `poppy.account.links`.
- Modify: `charts/omnia-demo/values.yaml`. Each key has a `# --` comment:
  ```yaml
  poppy:
    auth:
      enabled: false
      issuer: ""
      gateway: { enabled: false, host: "", upstream: { issuer: "", clientID: "", clientSecretRef: "" } }
      scopes: {}
      scopeHints: {}
      accessTokens: { audiences: [], clientIDClaim: client_id, scopeClaim: scope }
    account:
      tokenURL: ""         # the gateway's token_endpoint, from its RFC 8414 metadata
      clientID: ""
      clientSecretRef: ""  # Secret labelled omnia.altairalabs.ai/token-exchange-client-secret: "true"
      audience: ""         # the stub's audience, https://<domain>/api
      subjectIssuer: ""    # the gateway issuer
      encryption: {}       # {kmsProvider, keyID, secretRef}; required when enabled
      links: {}            # token sub -> member email (stand-in, docs/stand-ins.md)
  ```
- Modify: `profiles/azure/larkmere-mutual.yaml` and `wrenmoor-energy.yaml`.
  Write the whole block with `enabled: false` and a comment: "turn on once
  Omnia #4443 is deployed on omnia-azure".
  - Larkmere:
    - issuer `https://auth.larkmere.azure.altairalabs.ai`;
    - scope `claims`: "Report a home insurance claim, add items to it, and
      see its status";
    - hints `{changes: irreversible, personalData: true}`;
    - audiences `["https://larkmere.azure.altairalabs.ai"]`;
    - account audience `https://larkmere.azure.altairalabs.ai/api`.
  - Wrenmoor:
    - issuer `https://auth.wrenmoor.azure.altairalabs.ai`;
    - scope `outages`: "See how long your supply has been off and the
      compensation you're owed";
    - hints `{changes: none, personalData: true}`;
    - audiences `["https://wrenmoor.azure.altairalabs.ai"]`;
    - account audience `https://wrenmoor.azure.altairalabs.ai/api`.
  - Never `personalAgents.any`.

- [ ] **Step 1: Render with sign-in off.**
  `helm template t charts/omnia-demo -f profiles/azure/common.yaml -f profiles/azure/larkmere-mutual.yaml`
  is identical to `main` apart from comments: no `auth`, no Connection, no
  new route.
- [ ] **Step 2: Render with sign-in on.** Add `--set poppy.auth.enabled=true`
  and fill the required `poppy.account.*` values on the command line. The
  output shows:
  - the Workspace `auth` with `issuer`;
  - one Connection with `subjectIssuer`;
  - three `oauthOBO` handlers on `https://larkmere.azure.altairalabs.ai/api/claims/...`;
  - a ToolPolicy with no `@refer`;
  - an HTTPRoute with three Exact paths;
  - `--account-api larkmere` on `claims-systems`.
  
  Repeat for Wrenmoor. Then render once more with
  `--set poppy.auth.gateway.enabled=true`: `auth.gateway` replaces
  `auth.issuer`.
- [ ] **Step 3: Lint.** `helm lint charts/omnia-demo` passes, and
  `helm template omnia-demo charts/omnia-demo` passes.
- [ ] **Step 4: Commit**
  (`feat(chart): Company sign-in, a token-exchange Connection and on-behalf-of account tools`).

### Task 13: The Company Agents' packs

**Files:**
- Create, for Larkmere: `demos/claims-triage/poppy/tools/claim-open.tool.yaml`,
  `claim-add-item.tool.yaml` and `claim-status.tool.yaml`.
  - `claim_open` and `claim_add_item`:
    `action_scope: { effect: write, reversibility: compensable, data_classes: [pii] }`.
  - `claim_status`:
    `action_scope: { effect: read, reversibility: reversible, data_classes: [pii] }`.
  - Schemas match Task 10. The descriptions match the ToolRegistry's.
- Create, for Wrenmoor: `demos/wren/poppy/tools/my-outage.tool.yaml` and
  `compensation-status.tool.yaml`, both
  `action_scope: { effect: read, reversibility: reversible, data_classes: [pii] }`.
- Modify: `demos/claims-triage/poppy/prompts/larkmere-claims-desk.yaml`
  (adding the three to `allowed_tools`) and
  `demos/wren/poppy/prompts/wrenmoor-outages.yaml` (adding the two). Add
  `pii` to each pack's `governance-declarations.yaml` if it is absent.
- Prompt rules. These are written for any customer's representative and
  never mention this assistant:
  - Account actions (reporting a claim, adding to it, its status; the
    customer's own outage and compensation) act on the customer's account.
    Call the tool. If the customer isn't signed in, the call is refused and
    they are asked to sign in: say they need to sign in to Larkmere Mutual
    (or Wrenmoor Energy) to do that, and stop there.
  - Never identify a customer from a name, address, policy number or email
    given in the conversation. Only sign-in does that.
  - When told the customer has signed in, carry on with what was asked.
  - Larkmere: report the claim with `claim_open` once you have the peril,
    what happened and when. Add each damaged item with `claim_add_item`,
    with its value and cause as given. Read back the claim reference and
    what it holds. Say what the policy covers for emergency repairs and
    alternative accommodation from the member guidance, and say a claims
    handler will contact them.
  - Wrenmoor: give the outage on the customer's own supply with
    `my_outage`, and compensation only as `compensation_status` returns it.
    Replace the old "Never quote an amount" line with "Never quote an amount
    a tool did not return".
- Modify: `charts/omnia-demo/files/arena/larkmere-claims-desk-pack.json` and
  `wrenmoor-outages-pack.json`, rebuilt with `hack/build-packs.sh`.

- [ ] **Step 1: Validate.** `promptarena validate` on
  `demos/claims-triage/poppy` and `demos/wren/poppy` passes.
- [ ] **Step 2: Rebuild the packs.** Run
  `hack/build-packs.sh larkmere-claims-desk` and `hack/build-packs.sh wrenmoor-outages`.
  `git diff --stat charts/omnia-demo/files/arena` shows both packs changed.
- [ ] **Step 3: Render and lint** the chart (Task 12 Steps 1–3 still hold).
- [ ] **Step 4: Commit**
  (`feat(company-agents): account tools as the signed-in customer, and asking for sign-in`).

---

## Docs and the live run

### Task 14: Stand-ins, runbook, catalogue and the live run

**Files:**
- Modify: `docs/stand-ins.md`.
  - **Personal Agent:**
    - its tokens are sealed in the shared Redis under a key from a Secret
      (`personal-agent-token-key`);
    - the public `/signin/callback` cannot tell who completed a sign-in,
      because a real Personal Agent receives the redirect in its own app
      signed in as the User (PAP §4.2, §10.8). Here the page is on another
      host, behind the catalogue's sign-in. The defence left is Larkmere's
      consent page (`prompt=consent`);
    - pending sign-ins are in memory: a restart between the button and the
      callback means signing in again.
  - **Line-of-business stubs:**
    - the account routes take only a verified exchanged token, never the
      email header;
    - the account links (`poppy.account.links`) fake the Company's own link
      from its identity provider's user to an account. A real account
      system holds that link. Here the gateway's `sub` must be copied in by
      hand once Ellen has signed in the first time (RUNBOOK);
    - the compensation rule is fictional;
    - `homeClaims` is in memory.
  - **New: "Ellen at the upstream identity provider"**:
    - fakes: Ellen's login at Larkmere and at Wrenmoor;
    - real system: each Company's customer identity provider, with its own
      accounts. Here one Auth0 tenant per Company, or one shared tenant
      (see the open question), holds a synthetic Ellen with a password
      kept by the presenter;
    - downstream: anyone with that password signs in as Ellen at both
      Companies.
- Modify: `demos/personal-agent/RUNBOOK.md`:
  - **Setup** gains the gateway prerequisites **[needs #4443 + Auth0]**:
    - the Auth0 tenant and Ellen's user;
    - the gateway's `auth.gateway` values, and turning on `poppy.auth`;
    - the exchange client's Secret, with its label;
    - the account links. After Ellen's first sign-in, the stub logs
      `no account for subject <sub>`; copy that into
      `poppy.account.links` and run `make demos`.
  - **The beats** gains steps 3 and 4:
    - Larkmere asks for sign-in, the strip shows "Sign in to Larkmere
      Mutual", Ellen approves on Larkmere's page, and the claim is
      reported;
    - Wrenmoor gives her hours off and the compensation owed;
    - the assistant adds the freezer contents to the Larkmere claim.
  - **Dashboard** gains:
    - the Larkmere session's `poppy-account:` tag;
    - the account tools' calls, allowed by `<agent>-account-policy`.
  - **Checks** gains the three commands from Step 1 below. Mark every
    gateway-dependent line.
- Modify: `charts/omnia-demo/files/catalogue/index.html`. In the Ellen's
  assistant entry, add the sign-in beat and the capabilities "Direct
  Sign-In" and "On-behalf-of tools". Status stays Live for milestone 1,
  with the sign-in beat marked "needs the Poppy issuer gateway (Omnia
  #4443)" until Step 2 passes.

- [ ] **Step 1: Pre-gateway checks.** These run now, with sign-in off:
  - `go test ./...`, `go vet ./...`, and `go mod tidy` with
    `git diff --exit-code go.mod go.sum`;
  - `helm lint charts/omnia-demo` and
    `helm template omnia-demo charts/omnia-demo >/dev/null`;
  - Milestone 1's demo still runs, and no conversation shows a sign-in row.
- [ ] **Step 2: [needs #4443 + Auth0] Turn sign-in on.** Set
  `poppy.auth.enabled: true` in both profiles and run `make demos` (ask
  first).
  - `kubectl --context aks-omnia-demo get workspace larkmere-mutual -o jsonpath='{.status.conditions[?(@.type=="PoppyIssuer")]}'`
    is `True` with `PoppyIssuerVerified`.
  - `kubectl --context aks-omnia-demo get workspace larkmere-mutual -o jsonpath='{.status.poppy.companies[*].switch.phase}'`
    reaches `Published`.
  - `curl -s https://larkmere.azure.altairalabs.ai/.well-known/oauth-protected-resource`
    names `https://auth.larkmere.azure.altairalabs.ai`.
  - `kubectl --context aks-omnia-demo -n larkmere-mutual get agentruntime larkmere-claims-desk -o jsonpath='{.status.conditions[?(@.type=="PoppyFacade")]}'`
    is not `PoppyOnBehalfNeedsEnterprise`.
  - The same four for Wrenmoor.
- [ ] **Step 3: [needs #4443 + Auth0] Run story steps 3 and 4 live.**
  Ellen types the storm line. Expected:
  - Larkmere's row shows "Needs you to sign in". The assistant asks her, she
    says yes, and the button opens Larkmere's consent page on
    `auth.larkmere.azure.altairalabs.ai`.
  - After consent, the callback says "Signed in to Larkmere Mutual", the
    chat shows "I've signed in to Larkmere Mutual.", and Larkmere reports
    the claim with an `LM-41…` reference.
  - Wrenmoor's sign-in, the same way. It reports her hours off and the
    compensation owed.
  - The assistant adds the freezer contents to the Larkmere claim. Larkmere
    confirms the item and the new total.
  - `kubectl --context aks-omnia-demo -n larkmere-mutual logs deploy/claims-systems | grep 'tool=claim_add_item'`
    shows `status=200` and Ellen's email.
  - `kubectl --context aks-omnia-demo -n ellens-assistant logs deploy/personal-agent | grep -i -E 'bearer|eyJ'`
    prints nothing: no token in the logs.
- [ ] **Step 4: [needs #4443 + Auth0] Classify every refusal** under
  AGENTS.md (our configuration, a stand-in, or an Omnia bug) before
  changing anything. Report only Omnia bugs upstream, quoting the
  contradiction.
- [ ] **Step 5: Commit**
  (`docs: the signed-in storm demo, its stand-ins and runbook`).
  After Step 3 passes, a second commit drops the "needs the Poppy issuer
  gateway" marker from the catalogue.

---

## Open questions (for a maintainer, before Task 12 is turned on)

1. **Account linking.** The gateway's `sub` is derived from the upstream
   `iss` and `sub` (#4443), so the stub can't know Ellen's until she signs in
   once. The plan copies it into `poppy.account.links` by hand. The
   alternative is to have the gateway put `email` in exchanged tokens (#4443
   allows configured claims) and key the stub on that verified claim. Which
   one should we use?
2. **The Connection's KMS key.** `tokenExchange.encryption` needs a KMS key,
   and omnia-azure configures none today. Which Key Vault key, and is it
   added in omnia-azure?
3. **The exchange client at the gateway.** How is `poppy.account.clientID`
   registered, and does the gateway honour RFC 8693 `audience`? The stub's
   `aud` check depends on it.
4. **Auth0:** one tenant shared by both Companies' gateways, or one each?
   The issuers are distinct either way (`PoppyIssuerConflict`).

## Milestone 3 (unchanged; blocked on Omnia #4421 and #4422)

As outlined in `2026-11-23-poppy-personal-agent.md`. One thing changes: a
settlement call that acts as the member can't be held (#4257). So the
referral in milestone 3 must come from a settlement tool that is **not**
`oauthOBO`, such as the claims assessor's own `claim_settle` on a claim the
member reported here.
