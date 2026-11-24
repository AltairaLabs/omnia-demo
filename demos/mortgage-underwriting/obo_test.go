package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testAudience = "https://larkmere.example/api"
	ellenEmail   = "ellen.whitcomb@example.com"
)

// fakeIssuer stands in for a Company's issuer (in the demo, the Omnia
// gateway, #4443) for the stub's tests: RFC 8414 metadata naming a jwks_uri,
// an RSA signing key, and mint, which signs an exchanged access token. It is
// test-only and never built into the stub.
type fakeIssuer struct {
	t   *testing.T
	srv *httptest.Server

	mu         sync.Mutex
	keys       map[string]*rsa.PrivateKey
	kid        string
	jwksFetch  int
	metaIssuer string // the issuer the metadata claims; "" means the real one
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	t.Helper()
	f := &fakeIssuer{t: t, keys: map[string]*rsa.PrivateKey{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		iss := f.metaIssuer
		f.mu.Unlock()
		if iss == "" {
			iss = f.issuer()
		}
		writeTestJSON(w, map[string]any{
			"issuer":         iss,
			"token_endpoint": f.issuer() + "/oauth/token",
			"jwks_uri":       f.issuer() + "/jwks.json",
		})
	})
	mux.HandleFunc("GET /jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.jwksFetch++
		var keys []map[string]string
		for kid, k := range f.keys {
			keys = append(keys, map[string]string{
				"kty": "RSA", "use": "sig", "alg": "RS256", "kid": kid,
				"n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes()),
			})
		}
		writeTestJSON(w, map[string]any{"keys": keys})
	})
	f.srv = httptest.NewTLSServer(mux)
	t.Cleanup(f.srv.Close)
	f.rotate("key-1")
	return f
}

func writeTestJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeIssuer) issuer() string { return f.srv.URL }

// rotate adds a new signing key under kid and signs with it from now on.
func (f *fakeIssuer) rotate(kid string) {
	f.t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys[kid] = k
	f.kid = kid
}

func (f *fakeIssuer) fetches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jwksFetch
}

// claims are an exchanged token's claims for Ellen, valid at now, for the
// stub's audience, with the claims scope.
func (f *fakeIssuer) claims(now time.Time) jwt.MapClaims {
	return jwt.MapClaims{
		"iss":            f.issuer(),
		"sub":            "gw|ellen-7f3a",
		"aud":            testAudience,
		"iat":            now.Unix(),
		"exp":            now.Add(5 * time.Minute).Unix(),
		"scope":          "openid claims",
		"client_id":      "privacy-api",
		"email":          ellenEmail,
		"email_verified": true,
	}
}

// mint signs claims as an RFC 9068 access token with the current key.
func (f *fakeIssuer) mint(claims jwt.MapClaims) string {
	f.t.Helper()
	f.mu.Lock()
	kid, key := f.kid, f.keys[f.kid]
	f.mu.Unlock()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	tok.Header["typ"] = "at+jwt"
	s, err := tok.SignedString(key)
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

// testClock is a clock the tests move by hand.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestClock() *testClock {
	return &testClock{t: time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)}
}

func larkmereMembers(email string) bool {
	_, ok := cardholders[email]
	return ok
}

func newTestVerifier(f *fakeIssuer, clk *testClock) *oboVerifier {
	return newOBOVerifier(f.issuer(), testAudience, larkmereMembers, f.srv.Client(), clk.now)
}

// echoMember is an account route that answers the member it was given.
func echoMember(member string, w http.ResponseWriter, _ *http.Request) {
	writeTestJSON(w, map[string]string{"member": member})
}

// call sends one POST through guard with the given bearer token ("" for
// none) and headers.
func oboCall(v *oboVerifier, token string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/claims/status", strings.NewReader(`{}`))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, val := range hdr {
		req.Header.Set(k, val)
	}
	rec := httptest.NewRecorder()
	v.guard("claim_status", "claims", echoMember)(rec, req)
	return rec
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	return body["error"]
}

func wantRefused(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body.String())
	}
	if got := errorOf(t, rec); got != code {
		t.Errorf("error = %q, want %q", got, code)
	}
	if strings.Contains(rec.Body.String(), "member") {
		t.Errorf("refusal reached the handler: %s", rec.Body.String())
	}
	www := rec.Header().Get("WWW-Authenticate")
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		want := fmt.Sprintf(`Bearer error=%q`, code)
		if !strings.HasPrefix(www, want) {
			t.Errorf("WWW-Authenticate = %q, want prefix %q", www, want)
		}
	default:
		if www != "" {
			t.Errorf("WWW-Authenticate = %q on a %d", www, status)
		}
	}
}

func TestOBOAcceptsValidToken(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	rec := oboCall(newTestVerifier(f, clk), f.mint(f.claims(clk.now())), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"member":"`+ellenEmail+`"`) {
		t.Errorf("body = %s, want member %s", rec.Body.String(), ellenEmail)
	}
}

func TestOBOMatchesEmailCaseInsensitively(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	c := f.claims(clk.now())
	c["email"] = "Ellen.Whitcomb@Example.com"
	rec := oboCall(newTestVerifier(f, clk), f.mint(c), nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), ellenEmail) {
		t.Fatalf("status = %d; body %s", rec.Code, rec.Body.String())
	}
}

func TestOBORefusesMissingToken(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	rec := oboCall(newTestVerifier(f, clk), "", nil)
	wantRefused(t, rec, http.StatusUnauthorized, "invalid_token")
}

func TestOBORefusesNonBearerScheme(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	req := httptest.NewRequest(http.MethodPost, "/api/claims/status", nil)
	req.Header.Set("Authorization", "Basic "+f.mint(f.claims(clk.now())))
	rec := httptest.NewRecorder()
	newTestVerifier(f, clk).guard("claim_status", "claims", echoMember)(rec, req)
	wantRefused(t, rec, http.StatusUnauthorized, "invalid_token")
}

// The facade's own access token (aud https://larkmere.example) replayed at
// the API must not be accepted there.
func TestOBORefusesWrongAudience(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	c := f.claims(clk.now())
	c["aud"] = "https://larkmere.example"
	rec := oboCall(newTestVerifier(f, clk), f.mint(c), nil)
	wantRefused(t, rec, http.StatusUnauthorized, "invalid_token")
}

func TestOBORefusesOtherIssuer(t *testing.T) {
	f, other, clk := newFakeIssuer(t), newFakeIssuer(t), newTestClock()
	v := newTestVerifier(f, clk)

	// The other Company's token, as it is.
	wantRefused(t, oboCall(v, other.mint(other.claims(clk.now())), nil), http.StatusUnauthorized, "invalid_token")

	// The other issuer's key, claiming to be ours.
	c := other.claims(clk.now())
	c["iss"] = f.issuer()
	wantRefused(t, oboCall(v, other.mint(c), nil), http.StatusUnauthorized, "invalid_token")

	// Our key, but a different iss.
	c = f.claims(clk.now())
	c["iss"] = other.issuer()
	wantRefused(t, oboCall(v, f.mint(c), nil), http.StatusUnauthorized, "invalid_token")
}

func TestOBORefusesIssuerWhoseMetadataNamesAnother(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	f.metaIssuer = "https://elsewhere.example"
	rec := oboCall(newTestVerifier(f, clk), f.mint(f.claims(clk.now())), nil)
	wantRefused(t, rec, http.StatusUnauthorized, "invalid_token")
}

func TestOBORefusesExpired(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	v := newTestVerifier(f, clk)
	tok := f.mint(f.claims(clk.now()))
	clk.advance(6 * time.Minute)
	wantRefused(t, oboCall(v, tok, nil), http.StatusUnauthorized, "invalid_token")

	c := f.claims(clk.now())
	delete(c, "exp")
	wantRefused(t, oboCall(v, f.mint(c), nil), http.StatusUnauthorized, "invalid_token")
}

func TestOBORefusesHS256AndNone(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	v := newTestVerifier(f, clk)

	// HS256 keyed with the issuer's public modulus: the classic RS/HS
	// confusion.
	f.mu.Lock()
	pub := f.keys[f.kid].N.Bytes()
	f.mu.Unlock()
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, f.claims(clk.now()))
	hs.Header["kid"] = "key-1"
	hs.Header["typ"] = "at+jwt"
	hsTok, err := hs.SignedString(pub)
	if err != nil {
		t.Fatal(err)
	}
	wantRefused(t, oboCall(v, hsTok, nil), http.StatusUnauthorized, "invalid_token")

	none := jwt.NewWithClaims(jwt.SigningMethodNone, f.claims(clk.now()))
	none.Header["kid"] = "key-1"
	none.Header["typ"] = "at+jwt"
	noneTok, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	wantRefused(t, oboCall(v, noneTok, nil), http.StatusUnauthorized, "invalid_token")
}

// An ID token or any other JWT from the issuer is not an access token
// (RFC 9068 §4).
func TestOBORefusesWrongTyp(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, f.claims(clk.now()))
	tok.Header["kid"] = "key-1"
	tok.Header["typ"] = "JWT"
	f.mu.Lock()
	key := f.keys["key-1"]
	f.mu.Unlock()
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	wantRefused(t, oboCall(newTestVerifier(f, clk), s, nil), http.StatusUnauthorized, "invalid_token")
}

func TestOBORefusesMissingScope(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	c := f.claims(clk.now())
	c["scope"] = "openid outages claimsx"
	rec := oboCall(newTestVerifier(f, clk), f.mint(c), nil)
	wantRefused(t, rec, http.StatusForbidden, "insufficient_scope")
	if www := rec.Header().Get("WWW-Authenticate"); !strings.Contains(www, `scope="claims"`) {
		t.Errorf("WWW-Authenticate = %q, want scope=\"claims\"", www)
	}
}

// Ledger ruling: the member is the token's verified email. A token without
// one names nobody.
func TestOBORefusesTokenWithoutEmail(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	v := newTestVerifier(f, clk)

	c := f.claims(clk.now())
	delete(c, "email")
	wantRefused(t, oboCall(v, f.mint(c), nil), http.StatusUnauthorized, "invalid_token")

	c = f.claims(clk.now())
	c["email"] = "  "
	wantRefused(t, oboCall(v, f.mint(c), nil), http.StatusUnauthorized, "invalid_token")

	c = f.claims(clk.now())
	c["email"] = 42
	wantRefused(t, oboCall(v, f.mint(c), nil), http.StatusUnauthorized, "invalid_token")

	// The gateway maps email only when the upstream verified it; a token
	// that says otherwise is refused outright.
	c = f.claims(clk.now())
	c["email_verified"] = false
	wantRefused(t, oboCall(v, f.mint(c), nil), http.StatusUnauthorized, "invalid_token")

	// Absent, or not the boolean true, is not verified either.
	c = f.claims(clk.now())
	delete(c, "email_verified")
	wantRefused(t, oboCall(v, f.mint(c), nil), http.StatusUnauthorized, "invalid_token")

	c = f.claims(clk.now())
	c["email_verified"] = "true"
	wantRefused(t, oboCall(v, f.mint(c), nil), http.StatusUnauthorized, "invalid_token")
}

// 30 seconds of clock skew are allowed on nbf (and exp); more is refused.
func TestOBONotBeforeLeeway(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	v := newTestVerifier(f, clk)

	c := f.claims(clk.now())
	c["nbf"] = clk.now().Add(10 * time.Second).Unix()
	if rec := oboCall(v, f.mint(c), nil); rec.Code != http.StatusOK {
		t.Fatalf("nbf 10s ahead: status = %d; body %s", rec.Code, rec.Body.String())
	}

	c = f.claims(clk.now())
	c["nbf"] = clk.now().Add(5 * time.Minute).Unix()
	wantRefused(t, oboCall(v, f.mint(c), nil), http.StatusUnauthorized, "invalid_token")
}

func TestOBOUnknownEmailIsNoAccount(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	c := f.claims(clk.now())
	c["email"] = "someone.else@example.com"
	rec := oboCall(newTestVerifier(f, clk), f.mint(c), nil)
	wantRefused(t, rec, http.StatusNotFound, "no account for this customer")
}

// The account routes never fall back to the email the edge asserts.
func TestAccountRouteIgnoresEmailHeader(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	v := newTestVerifier(f, clk)
	hdr := map[string]string{"X-Omnia-User-Email": ellenEmail}
	wantRefused(t, oboCall(v, "", hdr), http.StatusUnauthorized, "invalid_token")

	// And a valid token for someone else is not overridden by the header.
	c := f.claims(clk.now())
	c["email"] = "gwen.ashby@example.com"
	rec := oboCall(v, f.mint(c), hdr)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "gwen.ashby@example.com") {
		t.Fatalf("status = %d; body %s, want Gwen from the token", rec.Code, rec.Body.String())
	}
}

func TestOBOUnknownKidRefetchesOnce(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	v := newTestVerifier(f, clk)
	if rec := oboCall(v, f.mint(f.claims(clk.now())), nil); rec.Code != http.StatusOK {
		t.Fatalf("first call status = %d", rec.Code)
	}
	if n := f.fetches(); n != 1 {
		t.Fatalf("jwks fetched %d times, want 1", n)
	}

	f.rotate("key-2")
	clk.advance(10 * time.Second)
	// Within 30s of the last fetch: the unknown kid is refused, not refetched.
	wantRefused(t, oboCall(v, f.mint(f.claims(clk.now())), nil), http.StatusUnauthorized, "invalid_token")
	wantRefused(t, oboCall(v, f.mint(f.claims(clk.now())), nil), http.StatusUnauthorized, "invalid_token")
	if n := f.fetches(); n != 1 {
		t.Fatalf("jwks fetched %d times within 30s, want 1", n)
	}

	clk.advance(21 * time.Second)
	if rec := oboCall(v, f.mint(f.claims(clk.now())), nil); rec.Code != http.StatusOK {
		t.Fatalf("after rotation status = %d; body %s", rec.Code, rec.Body.String())
	}
	if n := f.fetches(); n != 2 {
		t.Fatalf("jwks fetched %d times, want 2", n)
	}
	// The old key is still published, and a known kid never refetches.
	if rec := oboCall(v, f.mint(f.claims(clk.now())), nil); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if n := f.fetches(); n != 2 {
		t.Fatalf("jwks fetched %d times, want 2", n)
	}
}

func TestOBONeverLogsToken(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	f, clk := newFakeIssuer(t), newTestClock()
	v := newTestVerifier(f, clk)
	good := f.mint(f.claims(clk.now()))
	c := f.claims(clk.now())
	c["aud"] = "https://larkmere.example"
	bad := f.mint(c)
	oboCall(v, good, nil)
	oboCall(v, bad, nil)

	out := buf.String()
	if !strings.Contains(out, "tool=claim_status") {
		t.Fatalf("no request logged: %q", out)
	}
	for _, tok := range []string{good, bad} {
		for _, part := range strings.Split(tok, ".") {
			if strings.Contains(out, part) {
				t.Fatalf("log holds a token part: %q", out)
			}
		}
	}
	if strings.Contains(out, "Bearer") {
		t.Errorf("log holds the Authorization header: %q", out)
	}
}

func TestOBONotConfiguredIs503(t *testing.T) {
	var v *oboVerifier
	rec := oboCall(v, "anything", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := errorOf(t, rec); got != "account access is not configured" {
		t.Errorf("error = %q", got)
	}
}

func TestOBOGuardIsPostOnly(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	req := httptest.NewRequest(http.MethodGet, "/api/claims/status", nil)
	req.Header.Set("Authorization", "Bearer "+f.mint(f.claims(clk.now())))
	rec := httptest.NewRecorder()
	newTestVerifier(f, clk).guard("claim_status", "claims", echoMember)(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestAccountHolders(t *testing.T) {
	for _, tc := range []struct {
		api, email string
		want       bool
	}{
		{"larkmere", ellenEmail, true},
		{"larkmere", "amira.saleem@example.com", false},
		{"wrenmoor", "amira.saleem@example.com", true},
		{"wrenmoor", "nobody@example.com", false},
	} {
		known, err := accountHolders(tc.api)
		if err != nil {
			t.Fatal(err)
		}
		if got := known(tc.email); got != tc.want {
			t.Errorf("accountHolders(%q)(%q) = %v, want %v", tc.api, tc.email, got, tc.want)
		}
	}
	if _, err := accountHolders("halvern"); err == nil {
		t.Error("accountHolders(halvern) gave no error")
	}
}

func TestAccountConfigFromFlags(t *testing.T) {
	if acct, err := accountConfigFrom("", "", ""); err != nil || acct.api != "" || acct.verifier != nil {
		t.Errorf("no account api: %+v, %v", acct, err)
	}
	if _, err := accountConfigFrom("halvern", "https://auth.example", testAudience); err == nil {
		t.Error("an unknown Company was accepted")
	}
	if _, err := accountConfigFrom("larkmere", "http://auth.example", testAudience); err == nil {
		t.Error("an http issuer was accepted")
	}
	// Unconfigured: the routes exist and refuse with 503, never serve.
	if acct, err := accountConfigFrom("wrenmoor", "", testAudience); err != nil || acct.api != "wrenmoor" || acct.verifier != nil {
		t.Errorf("unconfigured: %+v, %v", acct, err)
	}
	acct, err := accountConfigFrom("larkmere", "https://auth.example", testAudience)
	if err != nil || acct.verifier == nil || acct.verifier.issuer != "https://auth.example" || acct.verifier.audience != testAudience {
		t.Errorf("configured: %+v, %v", acct, err)
	}
}
