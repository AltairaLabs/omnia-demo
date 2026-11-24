package signin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/discovery"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/identity"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/signin/signintest"
)

const ellen = "user-ellen"

type flowFixture struct {
	id    *identity.Identity
	iss   *signintest.Issuer
	co    *discovery.Company
	store *MemoryTokenStore
	flow  *Flow
}

func newFlowFixture(t *testing.T) *flowFixture {
	t.Helper()
	id := signintest.NewIdentity(t)
	iss := signintest.NewIssuer(t, id, testDomain)
	store := NewMemoryTokenStore()
	return &flowFixture{
		id:    id,
		iss:   iss,
		store: store,
		flow:  NewFlow(iss.Client(), id, store),
		co: &discovery.Company{
			Name:            "Larkmere Mutual",
			Domain:          testDomain,
			Identifier:      iss.URL,
			Issuer:          iss.URL,
			ScopesSupported: []string{"policies:read"},
			Scopes:          map[string]string{"claims:read": "See your claims"},
		},
	}
}

func (fx *flowFixture) begin(t *testing.T, scopes ...string) string {
	t.Helper()
	authURL, err := fx.flow.Begin(context.Background(), fx.co, ellen, scopes)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	return authURL
}

func query(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func tokenBody(access, refresh string, extra map[string]any) map[string]any {
	b := map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": 3600}
	for k, v := range extra {
		if v == nil {
			delete(b, k)
			continue
		}
		b[k] = v
	}
	return b
}

func TestBeginBuildsAuthorizationURL(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read", "claims:read")
	if !strings.HasPrefix(authURL, fx.iss.URL+"/authorize?") {
		t.Fatalf("authorization URL %q is not at the metadata's authorization_endpoint", authURL)
	}
	q := query(t, authURL)
	for k, want := range map[string]string{
		"response_type":         "code",
		"client_id":             fx.id.ClientID,
		"redirect_uri":          fx.id.RedirectURI,
		"prompt":                "consent",
		"code_challenge_method": "S256",
		"scope":                 "claims:read policies:read",
	} {
		if got := q.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if q.Has("resource") {
		t.Error("the authorization request names a resource")
	}
	if len(q.Get("state")) < 22 {
		t.Errorf("state %d characters, want at least 22", len(q.Get("state")))
	}
	if len(q.Get("code_challenge")) != 43 {
		t.Errorf("code_challenge is %d characters, want 43 (base64url SHA-256)", len(q.Get("code_challenge")))
	}
}

func TestBeginRefusesNoIssuerAndDisabled(t *testing.T) {
	fx := newFlowFixture(t)
	noIssuer := *fx.co
	noIssuer.Issuer = ""
	disabled := *fx.co
	disabled.SignInDisabled = true
	for name, co := range map[string]*discovery.Company{"no issuer": &noIssuer, "disabled": &disabled} {
		t.Run(name, func(t *testing.T) {
			_, err := fx.flow.Begin(context.Background(), co, ellen, []string{"policies:read"})
			if err == nil || !strings.Contains(err.Error(), "Larkmere Mutual offers no sign-in") {
				t.Errorf("Begin error = %v, want Larkmere Mutual offers no sign-in", err)
			}
		})
	}
}

func TestBeginRefusesUnofferedScope(t *testing.T) {
	fx := newFlowFixture(t)
	fx.iss.SetScopesSupported("policies:read", "payments:write")
	_, err := fx.flow.Begin(context.Background(), fx.co, ellen, []string{"policies:read", "account:delete"})
	if err == nil {
		t.Fatal("Begin accepted a scope nothing offers")
	}
	for _, s := range []string{"account:delete", "claims:read", "payments:write", "policies:read"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error %q does not name %s", err, s)
		}
	}
}

func TestBeginRequestsPreviouslyGrantedScopesToo(t *testing.T) {
	fx := newFlowFixture(t)
	if err := fx.store.Put(fx.co.Identifier, ellen, Tokens{Access: "a", Refresh: "r", Scopes: []string{"policies:read"}}); err != nil {
		t.Fatal(err)
	}
	q := query(t, fx.begin(t, "claims:read"))
	if got := q.Get("scope"); got != "claims:read policies:read" {
		t.Errorf("scope = %q, want the new scope and the one already granted", got)
	}
}

func TestBeginRefusesNoScopes(t *testing.T) {
	fx := newFlowFixture(t)
	if _, err := fx.flow.Begin(context.Background(), fx.co, ellen, nil); err == nil {
		t.Error("Begin accepted a sign-in with no scope")
	}
}

func TestCompleteExchangesCode(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read")
	challenge := query(t, authURL).Get("code_challenge")
	fx.iss.Next(http.StatusOK, tokenBody("access-1", "refresh-1", map[string]any{"scope": "policies:read"}))

	res, err := fx.flow.Complete(context.Background(), fx.iss.Approve(authURL))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Declined || res.Domain != testDomain || res.Name != "Larkmere Mutual" || !slices.Equal(res.Granted, []string{"policies:read"}) {
		t.Errorf("Result = %+v", res)
	}

	reqs := fx.iss.Requests()
	if len(reqs) != 1 {
		t.Fatalf("token endpoint saw %d requests, want 1", len(reqs))
	}
	r := reqs[0]
	sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
		t.Error("code_verifier is not the one the code_challenge was made from")
	}
	if len(r.Form.Get("code_verifier")) != 43 {
		t.Errorf("code_verifier is %d characters, want 43 (32 bytes base64url)", len(r.Form.Get("code_verifier")))
	}
	if r.Claims == nil {
		t.Fatal("the client assertion failed verification")
	}
	if aud, ok := r.Claims["aud"].(string); !ok || aud != fx.iss.URL {
		t.Errorf("assertion aud = %#v, want the issuer %q as a string", r.Claims["aud"], fx.iss.URL)
	}
	if r.Form.Get("redirect_uri") != fx.id.RedirectURI || r.Form.Get("grant_type") != "authorization_code" ||
		r.Form.Get("client_id") != fx.id.ClientID {
		t.Errorf("token request form fields: redirect_uri %q, grant_type %q, client_id %q",
			r.Form.Get("redirect_uri"), r.Form.Get("grant_type"), r.Form.Get("client_id"))
	}

	tok, err := fx.store.Get(fx.co.Identifier, ellen)
	if err != nil || tok == nil {
		t.Fatalf("Tokens.Get = %v, %v", tok, err)
	}
	if tok.Access != "access-1" || tok.Refresh != "refresh-1" || !slices.Equal(tok.Scopes, []string{"policies:read"}) {
		t.Errorf("stored %v", tok)
	}
	if d := time.Until(tok.Expiry); d < 59*time.Minute || d > 61*time.Minute {
		t.Errorf("expiry %v from now, want about an hour", d)
	}
}

func TestCompleteMissingExpiresInIsFiveMinutes(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read")
	fx.iss.Next(http.StatusOK, tokenBody("a", "r", map[string]any{"expires_in": nil}))
	if _, err := fx.flow.Complete(context.Background(), fx.iss.Approve(authURL)); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	tok, _ := fx.store.Get(fx.co.Identifier, ellen)
	if d := time.Until(tok.Expiry); d < 4*time.Minute || d > 5*time.Minute {
		t.Errorf("expiry %v from now, want 5 minutes", d)
	}
}

func TestCompleteRefusesUnknownState(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read")
	q := fx.iss.Approve(authURL)
	q.Set("state", "not-the-state-we-made-at-all")
	if _, err := fx.flow.Complete(context.Background(), q); !errors.Is(err, ErrUnknownState) {
		t.Errorf("Complete error = %v, want ErrUnknownState", err)
	}
	if n := len(fx.iss.Requests()); n != 0 {
		t.Errorf("token endpoint saw %d requests, want none", n)
	}
}

func TestCompleteRefusesReusedState(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read")
	q := fx.iss.Approve(authURL)
	if _, err := fx.flow.Complete(context.Background(), q); err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	if _, err := fx.flow.Complete(context.Background(), q); !errors.Is(err, ErrUnknownState) {
		t.Errorf("second Complete error = %v, want ErrUnknownState", err)
	}
	if n := len(fx.iss.Requests()); n != 1 {
		t.Errorf("token endpoint saw %d requests, want 1", n)
	}
}

func TestCompleteRefusesExpiredState(t *testing.T) {
	fx := newFlowFixture(t)
	now := time.Now()
	fx.flow.Now = func() time.Time { return now }
	authURL := fx.begin(t, "policies:read")
	now = now.Add(10*time.Minute + time.Second)
	if _, err := fx.flow.Complete(context.Background(), fx.iss.Approve(authURL)); !errors.Is(err, ErrUnknownState) {
		t.Errorf("Complete error = %v, want ErrUnknownState (expired)", err)
	}
	if n := len(fx.iss.Requests()); n != 0 {
		t.Errorf("token endpoint saw %d requests, want none", n)
	}
}

func TestCompleteRefusesIssuerMismatchEvenOnDecline(t *testing.T) {
	cases := map[string]func(q url.Values){
		"approve, other iss":   func(q url.Values) { q.Set("iss", "https://auth.wrenmoor.example") },
		"approve, trailing /":  func(q url.Values) { q.Set("iss", q.Get("iss")+"/") },
		"approve, missing iss": func(q url.Values) { q.Del("iss") },
		"decline, other iss":   func(q url.Values) { q.Set("iss", "https://auth.wrenmoor.example") },
		"decline, missing iss": func(q url.Values) { q.Del("iss") },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newFlowFixture(t)
			authURL := fx.begin(t, "policies:read")
			var q url.Values
			if strings.HasPrefix(name, "decline") {
				q = fx.iss.Decline(authURL)
			} else {
				q = fx.iss.Approve(authURL)
			}
			edit(q)
			res, err := fx.flow.Complete(context.Background(), q)
			if !errors.Is(err, ErrIssuerMismatch) {
				t.Errorf("Complete = %+v, %v, want ErrIssuerMismatch", res, err)
			}
			if n := len(fx.iss.Requests()); n != 0 {
				t.Errorf("token endpoint saw %d requests, want none", n)
			}
			if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok != nil {
				t.Error("tokens were stored")
			}
		})
	}
}

func TestCompleteDeclineStoresNothing(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read")
	res, err := fx.flow.Complete(context.Background(), fx.iss.Decline(authURL))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !res.Declined || res.Name != "Larkmere Mutual" || res.Domain != testDomain || len(res.Granted) != 0 {
		t.Errorf("Result = %+v, want a decline from Larkmere Mutual", res)
	}
	if n := len(fx.iss.Requests()); n != 0 {
		t.Errorf("token endpoint saw %d requests, want none", n)
	}
	if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok != nil {
		t.Error("tokens were stored")
	}
}

func TestCompleteEmptyGrantIsADecline(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read")
	fx.iss.Next(http.StatusOK, tokenBody("a", "r", map[string]any{"scope": ""}))
	res, err := fx.flow.Complete(context.Background(), fx.iss.Approve(authURL))
	if err != nil || !res.Declined {
		t.Errorf("Complete = %+v, %v, want a decline", res, err)
	}
	if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok != nil {
		t.Error("tokens were stored")
	}
}

func TestCompleteRecordsSubsetOfScopes(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read", "claims:read")
	fx.iss.Next(http.StatusOK, tokenBody("a", "r", map[string]any{"scope": "policies:read"}))
	res, err := fx.flow.Complete(context.Background(), fx.iss.Approve(authURL))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !slices.Equal(res.Granted, []string{"policies:read"}) {
		t.Errorf("Granted = %v", res.Granted)
	}
	tok, _ := fx.store.Get(fx.co.Identifier, ellen)
	if tok == nil || !slices.Equal(tok.Scopes, []string{"policies:read"}) {
		t.Errorf("stored %v", tok)
	}
}

func TestCompleteNoScopeGrantsExactlyRequested(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read", "claims:read")
	fx.iss.Next(http.StatusOK, tokenBody("a", "r", nil))
	res, err := fx.flow.Complete(context.Background(), fx.iss.Approve(authURL))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !slices.Equal(res.Granted, []string{"claims:read", "policies:read"}) {
		t.Errorf("Granted = %v, want both requested scopes", res.Granted)
	}
}

func TestCompleteRefusesExtraScopes(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read")
	fx.iss.Next(http.StatusOK, tokenBody("a", "r", map[string]any{"scope": "policies:read claims:read"}))
	if _, err := fx.flow.Complete(context.Background(), fx.iss.Approve(authURL)); err == nil || !strings.Contains(err.Error(), "claims:read") {
		t.Errorf("Complete error = %v, want a refusal naming claims:read", err)
	}
	if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok != nil {
		t.Error("tokens were stored")
	}
}

func TestCompleteRefusesDPoPAndMissingRefreshToken(t *testing.T) {
	cases := map[string]map[string]any{
		"DPoP":             {"token_type": "DPoP"},
		"no token_type":    {"token_type": nil},
		"no refresh token": {"refresh_token": nil},
		"no access token":  {"access_token": nil},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newFlowFixture(t)
			authURL := fx.begin(t, "policies:read")
			fx.iss.Next(http.StatusOK, tokenBody("a", "r", edit))
			if _, err := fx.flow.Complete(context.Background(), fx.iss.Approve(authURL)); err == nil {
				t.Error("Complete accepted it")
			}
			if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok != nil {
				t.Error("tokens were stored")
			}
		})
	}
}

func TestCompleteTokenError(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read")
	fx.iss.Next(http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
	if _, err := fx.flow.Complete(context.Background(), fx.iss.Approve(authURL)); err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("Complete error = %v, want one naming invalid_grant", err)
	}
	if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok != nil {
		t.Error("tokens were stored")
	}
}

func TestFlowNeverLogsSecrets(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	fx := newFlowFixture(t)
	var secrets []string
	var errs []string

	// A sign-in that succeeds.
	authURL := fx.begin(t, "policies:read")
	q := fx.iss.Approve(authURL)
	secrets = append(secrets, q.Get("code"), q.Get("state"), query(t, authURL).Get("code_challenge"))
	fx.iss.Next(http.StatusOK, tokenBody("access-secret-123", "refresh-secret-456", nil))
	secrets = append(secrets, "access-secret-123", "refresh-secret-456")
	if _, err := fx.flow.Complete(context.Background(), q); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, err := fx.flow.Complete(context.Background(), q); err != nil {
		errs = append(errs, err.Error())
	}

	// Sign-ins that fail at each step after the redirect.
	for _, body := range []map[string]any{
		tokenBody("access-secret-789", "refresh-secret-abc", map[string]any{"scope": "claims:read"}),
		tokenBody("access-secret-def", "refresh-secret-ghi", map[string]any{"token_type": "DPoP"}),
		{"error": "invalid_grant", "error_description": "access-secret-jkl"},
	} {
		authURL := fx.begin(t, "policies:read")
		q := fx.iss.Approve(authURL)
		secrets = append(secrets, q.Get("code"), q.Get("state"))
		status := http.StatusOK
		if body["error"] != nil {
			status = http.StatusBadRequest
		}
		fx.iss.Next(status, body)
		if _, err := fx.flow.Complete(context.Background(), q); err != nil {
			errs = append(errs, err.Error())
		}
	}
	secrets = append(secrets, "access-secret-789", "refresh-secret-abc", "access-secret-def", "refresh-secret-ghi", "access-secret-jkl")
	for _, r := range fx.iss.Requests() {
		secrets = append(secrets, r.Form.Get("code_verifier"))
	}

	out := buf.String() + "\n" + strings.Join(errs, "\n")
	for _, s := range secrets {
		if s != "" && strings.Contains(out, s) {
			t.Errorf("a secret reached the log or an error: %q", s)
		}
	}
}

// putTokens stores tokens for Ellen at the fixture's Company, expiring in d.
func (fx *flowFixture) putTokens(t *testing.T, access, refresh string, d time.Duration) {
	t.Helper()
	if err := fx.store.Put(fx.co.Identifier, ellen, Tokens{Access: access, Refresh: refresh, Scopes: []string{"policies:read"}, Expiry: time.Now().Add(d)}); err != nil {
		t.Fatal(err)
	}
}

func (fx *flowFixture) requestsTo(path string) []signintest.Request {
	var out []signintest.Request
	for _, r := range fx.iss.Requests() {
		if r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func TestAccessTokenUnexpiredNeedsNoRefresh(t *testing.T) {
	fx := newFlowFixture(t)
	fx.putTokens(t, "a0", "r0", 5*time.Minute)
	got, err := fx.flow.AccessToken(context.Background(), fx.co, ellen)
	if err != nil || got != "a0" {
		t.Fatalf("AccessToken = %q, %v; want a0", got, err)
	}
	if n := len(fx.iss.Requests()); n != 0 {
		t.Errorf("issuer saw %d requests, want none", n)
	}
	if got, err := fx.flow.AccessToken(context.Background(), fx.co, "user-nobody"); got != "" || err != nil {
		t.Errorf("AccessToken for a User with no tokens = %q, %v; want empty", got, err)
	}
}

func TestRefreshBeforeExpiry(t *testing.T) {
	fx := newFlowFixture(t)
	fx.putTokens(t, "a0", "r0", 30*time.Second)
	fx.iss.Next(http.StatusOK, tokenBody("a1", "r1", nil))

	got, err := fx.flow.AccessToken(context.Background(), fx.co, ellen)
	if err != nil || got != "a1" {
		t.Fatalf("AccessToken = %q, %v; want the refreshed a1", got, err)
	}
	reqs := fx.requestsTo("/token")
	if len(reqs) != 1 {
		t.Fatalf("token endpoint saw %d requests, want 1", len(reqs))
	}
	r := reqs[0]
	if r.Claims == nil {
		t.Error("the refresh's client assertion failed verification")
	}
	if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "r0" || r.Form.Has("scope") {
		t.Errorf("refresh form: grant_type %q, refresh_token %q, scope present %v",
			r.Form.Get("grant_type"), r.Form.Get("refresh_token"), r.Form.Has("scope"))
	}
	tok, _ := fx.store.Get(fx.co.Identifier, ellen)
	if tok == nil || tok.Access != "a1" || tok.Refresh != "r1" || !slices.Equal(tok.Scopes, []string{"policies:read"}) {
		t.Fatalf("stored %v, want a1, the rotated r1 and the same scopes", tok)
	}
	if d := time.Until(tok.Expiry); d < 59*time.Minute {
		t.Errorf("expiry %v from now, want about an hour", d)
	}
	// A rotated refresh token is not revoked: the issuer already retired it
	// (RFC 6749 §6), and presenting it again can look like reuse.
	if n := len(fx.requestsTo("/revoke")); n != 0 {
		t.Errorf("revocation endpoint saw %d requests, want none", n)
	}
}

func TestRefreshKeepsRefreshTokenNotRotated(t *testing.T) {
	fx := newFlowFixture(t)
	fx.putTokens(t, "a0", "r0", 30*time.Second)
	fx.iss.Next(http.StatusOK, tokenBody("a1", "", map[string]any{"refresh_token": nil}))
	if got, err := fx.flow.AccessToken(context.Background(), fx.co, ellen); err != nil || got != "a1" {
		t.Fatalf("AccessToken = %q, %v", got, err)
	}
	if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok == nil || tok.Refresh != "r0" {
		t.Errorf("stored %v, want the old refresh token kept", tok)
	}
}

func TestRefreshInvalidGrantDeletesTokens(t *testing.T) {
	fx := newFlowFixture(t)
	fx.putTokens(t, "a0", "r0", 5*time.Minute)
	fx.iss.Next(http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
	got, err := fx.flow.Refresh(context.Background(), fx.co, ellen)
	if got != "" || err != nil {
		t.Fatalf("Refresh = %q, %v; want empty and no error (signed out)", got, err)
	}
	if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok != nil {
		t.Errorf("tokens still stored: %v", tok)
	}
}

func TestRefreshFailureKeepsUnexpiredToken(t *testing.T) {
	fx := newFlowFixture(t)
	fx.putTokens(t, "a0", "r0", 30*time.Second)
	fx.iss.Next(http.StatusServiceUnavailable, map[string]any{"error": "temporarily_unavailable"})
	if got, err := fx.flow.AccessToken(context.Background(), fx.co, ellen); got != "a0" || err != nil {
		t.Errorf("AccessToken = %q, %v; want the unexpired a0", got, err)
	}
	if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok == nil || tok.Access != "a0" || tok.Refresh != "r0" {
		t.Errorf("stored %v, want it unchanged", tok)
	}

	// Expired, the same failure is an error, and the tokens are kept for
	// the next try.
	fx.putTokens(t, "a0", "r0", -time.Second)
	fx.iss.Next(http.StatusServiceUnavailable, map[string]any{"error": "temporarily_unavailable"})
	if got, err := fx.flow.AccessToken(context.Background(), fx.co, ellen); got != "" || err == nil {
		t.Errorf("AccessToken = %q, %v; want an error", got, err)
	}
	if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok == nil {
		t.Error("a failed refresh deleted the tokens")
	}

	// A network error with an unexpired token answers it too.
	fx.putTokens(t, "a0", "r0", 30*time.Second)
	broken := NewFlow(&http.Client{Transport: failingTransport{}}, fx.id, fx.store)
	if got, err := broken.AccessToken(context.Background(), fx.co, ellen); got != "a0" || err != nil {
		t.Errorf("AccessToken over a broken network = %q, %v; want a0", got, err)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network down")
}

func TestRefreshIsOncePerUserAtATime(t *testing.T) {
	fx := newFlowFixture(t)
	fx.putTokens(t, "a0", "r0", 30*time.Second)
	var wg sync.WaitGroup
	got := make([]string, 4)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i], _ = fx.flow.AccessToken(context.Background(), fx.co, ellen)
		}()
	}
	wg.Wait()
	if n := len(fx.requestsTo("/token")); n != 1 {
		t.Errorf("token endpoint saw %d refreshes, want 1", n)
	}
	for _, g := range got[1:] {
		if g != got[0] || g == "a0" || g == "" {
			t.Errorf("AccessToken answers %q, want one refreshed token for all", got)
			break
		}
	}
}

func TestRefreshRefusesExtraScopeAndRevokesItsRefreshToken(t *testing.T) {
	fx := newFlowFixture(t)
	fx.putTokens(t, "a0", "r0", -time.Second)
	fx.iss.Next(http.StatusOK, tokenBody("a1", "r1", map[string]any{"scope": "policies:read claims:read"}))
	if got, err := fx.flow.AccessToken(context.Background(), fx.co, ellen); got != "" || err == nil {
		t.Errorf("AccessToken = %q, %v; want a refusal", got, err)
	}
	revs := fx.requestsTo("/revoke")
	if len(revs) != 1 || revs[0].Form.Get("token") != "r1" || revs[0].Form.Get("token_type_hint") != "refresh_token" || revs[0].Claims == nil {
		t.Errorf("revocations = %+v, want r1 revoked with an assertion", revs)
	}
	if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok == nil || tok.Access != "a0" {
		t.Errorf("stored %v, want it unchanged", tok)
	}
}

func TestCompleteRevokesReplacedRefreshToken(t *testing.T) {
	fx := newFlowFixture(t)
	fx.putTokens(t, "a-old", "r-old", time.Hour)
	authURL := fx.begin(t, "claims:read")
	fx.iss.Next(http.StatusOK, tokenBody("a-new", "r-new", nil))
	if _, err := fx.flow.Complete(context.Background(), fx.iss.Approve(authURL)); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	revs := fx.requestsTo("/revoke")
	if len(revs) != 1 || revs[0].Form.Get("token") != "r-old" || revs[0].Form.Get("token_type_hint") != "refresh_token" {
		t.Errorf("revocations = %+v, want r-old revoked", revs)
	}
	if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok == nil || tok.Refresh != "r-new" {
		t.Errorf("stored %v, want r-new", tok)
	}
}

func TestCompleteRevokesDiscardedRefreshToken(t *testing.T) {
	cases := map[string]map[string]any{
		"extra scope": {"scope": "policies:read claims:read"},
		"empty grant": {"scope": ""},
		"DPoP":        {"token_type": "DPoP"},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newFlowFixture(t)
			authURL := fx.begin(t, "policies:read")
			fx.iss.Next(http.StatusOK, tokenBody("a", "r-discarded", edit))
			_, _ = fx.flow.Complete(context.Background(), fx.iss.Approve(authURL))
			revs := fx.requestsTo("/revoke")
			if len(revs) != 1 || revs[0].Form.Get("token") != "r-discarded" {
				t.Errorf("revocations = %+v, want r-discarded revoked", revs)
			}
			if tok, _ := fx.store.Get(fx.co.Identifier, ellen); tok != nil {
				t.Error("tokens were stored")
			}
		})
	}
}

func TestCompleteClampsExpiresIn(t *testing.T) {
	fx := newFlowFixture(t)
	authURL := fx.begin(t, "policies:read")
	fx.iss.Next(http.StatusOK, tokenBody("a", "r", map[string]any{"expires_in": int64(1) << 62}))
	if _, err := fx.flow.Complete(context.Background(), fx.iss.Approve(authURL)); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	tok, _ := fx.store.Get(fx.co.Identifier, ellen)
	if d := time.Until(tok.Expiry); d <= 23*time.Hour || d > 24*time.Hour {
		t.Errorf("expiry %v from now, want it clamped to 24h", d)
	}
}

func TestBeginCapsPendingSignInsPerUser(t *testing.T) {
	fx := newFlowFixture(t)
	other, err := fx.flow.Begin(context.Background(), fx.co, "user-other", []string{"policies:read"})
	if err != nil {
		t.Fatal(err)
	}
	var urls []string
	for range maxPendingPerUser + 1 {
		urls = append(urls, fx.begin(t, "policies:read"))
	}
	if _, err := fx.flow.Complete(context.Background(), fx.iss.Approve(urls[0])); !errors.Is(err, ErrUnknownState) {
		t.Errorf("the oldest sign-in past the cap: Complete error = %v, want ErrUnknownState", err)
	}
	if _, err := fx.flow.Complete(context.Background(), fx.iss.Approve(urls[len(urls)-1])); err != nil {
		t.Errorf("the newest sign-in: %v", err)
	}
	if _, err := fx.flow.Complete(context.Background(), fx.iss.Approve(other)); err != nil {
		t.Errorf("another User's sign-in: %v", err)
	}
	fx.flow.mu.Lock()
	n := len(fx.flow.pending)
	fx.flow.mu.Unlock()
	if n > maxPendingPerUser {
		t.Errorf("%d sign-ins pending, want at most %d", n, maxPendingPerUser)
	}
}

func TestRefreshNeverLogsSecrets(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	fx := newFlowFixture(t)
	var errs []string
	for _, c := range []struct {
		status int
		body   map[string]any
	}{
		{http.StatusOK, tokenBody("access-secret-1", "refresh-secret-2", nil)},
		{http.StatusOK, tokenBody("access-secret-3", "refresh-secret-4", map[string]any{"token_type": "DPoP"})},
		{http.StatusInternalServerError, map[string]any{"error": "server_error", "error_description": "access-secret-5"}},
		{http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "refresh-secret-6"}},
	} {
		fx.putTokens(t, "access-secret-0", "refresh-secret-0", -time.Second)
		fx.iss.Next(c.status, c.body)
		if _, err := fx.flow.AccessToken(context.Background(), fx.co, ellen); err != nil {
			errs = append(errs, err.Error())
		}
	}
	out := buf.String() + "\n" + strings.Join(errs, "\n")
	for _, s := range []string{"access-secret", "refresh-secret"} {
		if strings.Contains(out, s) {
			t.Errorf("a secret reached the log or an error:\n%s", out)
		}
	}
}
