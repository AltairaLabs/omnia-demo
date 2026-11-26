// Package signintest provides a fake Company issuer for testing PAP Direct
// Sign-In, in the way net/http/httptest provides test servers.
//
// It is a stand-in for a Company's OAuth authorization server (in the demo,
// the Omnia issuer gateway in front of Auth0, Omnia #4443). It fakes the
// User: Approve and Decline stand for Ellen signing in and approving, or
// declining, on the Company's consent page, with no browser and no account.
// A real issuer would authenticate her and show its own consent page, so
// nothing here says who approved a sign-in. It is for tests only and is never
// built into the agent.
package signintest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/identity"
)

const assertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"

// NewIdentity generates the two identity keys in a temporary directory and
// loads them as the agent at https://assistant.example.test.
func NewIdentity(t testing.TB) *identity.Identity {
	t.Helper()
	dir := t.TempDir()
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]any{"es256.pem": ec, "ps384.pem": rs} {
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		b := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	id, err := identity.Load(dir, "https://assistant.example.test")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Request is one request the issuer's token or revocation endpoint received.
type Request struct {
	Path string
	Form url.Values
	// Claims are the client assertion's claims once verified, nil when it
	// failed verification.
	Claims jwt.MapClaims
}

type canned struct {
	status int
	body   map[string]any
}

// grant is what Approve issued a code for.
type grant struct {
	clientID, redirectURI, challenge string
}

// Issuer is a fake Company issuer over TLS: RFC 8414 metadata at
// /.well-known/oauth-authorization-server, an authorization endpoint that
// Approve and Decline stand in for, a token endpoint and a revocation
// endpoint.
type Issuer struct {
	// URL is the issuer, the server's https origin with no path.
	URL string

	t      testing.TB
	srv    *httptest.Server
	id     *identity.Identity
	domain string

	mu     sync.Mutex
	scopes []string
	next   []canned
	reqs   []Request
	codes  map[string]grant
	jtis   map[string]bool // client assertion jti values already used
}

// NewIssuer starts an issuer for the Company at domain, whose metadata lists
// https://<domain> in protected_resources, and which accepts client
// assertions signed by id. It is closed when the test ends.
func NewIssuer(t testing.TB, id *identity.Identity, domain string) *Issuer {
	t.Helper()
	iss := &Issuer{
		t:      t,
		id:     id,
		domain: domain,
		scopes: []string{"claims:read", "policies:read"},
		codes:  map[string]grant{},
		jtis:   map[string]bool{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", iss.metadata)
	mux.HandleFunc("POST /token", iss.token)
	mux.HandleFunc("POST /revoke", iss.revoke)
	iss.srv = httptest.NewTLSServer(mux)
	t.Cleanup(iss.srv.Close)
	iss.URL = iss.srv.URL
	return iss
}

// Client is an HTTP client that trusts the issuer's certificate.
func (i *Issuer) Client() *http.Client { return i.srv.Client() }

// SetScopesSupported replaces the metadata's scopes_supported.
func (i *Issuer) SetScopesSupported(scopes ...string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.scopes = slices.Clone(scopes)
}

// Next queues the answer to the next token request whose client assertion
// (and, for a code, PKCE verifier and redirect_uri) checks out. With nothing
// queued the token endpoint answers 200 with fresh random Bearer tokens,
// expires_in 3600 and no scope (exactly the scopes requested).
func (i *Issuer) Next(status int, body map[string]any) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.next = append(i.next, canned{status: status, body: body})
}

// Requests returns the token and revocation requests received so far.
func (i *Issuer) Requests() []Request {
	i.mu.Lock()
	defer i.mu.Unlock()
	return slices.Clone(i.reqs)
}

// Approve stands in for the User approving the sign-in at authURL. It checks
// the authorization request (including prompt=consent and no resource, PAP
// §4.2, §4.5), issues a one-time code bound to its
// code_challenge and redirect_uri, and returns the query of the redirect
// back to the agent: code, state and iss.
func (i *Issuer) Approve(authURL string) url.Values {
	i.t.Helper()
	q := i.checkAuthorization(authURL)
	code := random(16)
	i.mu.Lock()
	i.codes[code] = grant{clientID: q.Get("client_id"), redirectURI: q.Get("redirect_uri"), challenge: q.Get("code_challenge")}
	i.mu.Unlock()
	return url.Values{"code": {code}, "state": {q.Get("state")}, "iss": {i.URL}}
}

// Decline stands in for the User declining the sign-in at authURL. It
// returns the query of the redirect back: error=access_denied, state and
// iss.
func (i *Issuer) Decline(authURL string) url.Values {
	i.t.Helper()
	q := i.checkAuthorization(authURL)
	return url.Values{"error": {"access_denied"}, "state": {q.Get("state")}, "iss": {i.URL}}
}

func (i *Issuer) checkAuthorization(authURL string) url.Values {
	i.t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		i.t.Fatalf("signintest: authorization URL: %v", err)
	}
	if u.Scheme+"://"+u.Host+u.Path != i.URL+"/authorize" {
		i.t.Fatalf("signintest: authorization URL %s is not at %s/authorize", u.Redacted(), i.URL)
	}
	q := u.Query()
	for k, want := range map[string]string{
		"response_type":         "code",
		"client_id":             i.id.ClientID,
		"redirect_uri":          i.id.RedirectURI,
		"code_challenge_method": "S256",
		"prompt":                "consent",
	} {
		if got := q.Get(k); got != want {
			i.t.Fatalf("signintest: authorization request %s = %q, want %q", k, got, want)
		}
	}
	for _, k := range []string{"state", "code_challenge", "scope"} {
		if q.Get(k) == "" {
			i.t.Fatalf("signintest: authorization request has no %s", k)
		}
	}
	if q.Has("resource") {
		i.t.Fatalf("signintest: authorization request names a resource %q; a sign-in leaves it out (§4.5)", q.Get("resource"))
	}
	return q
}

func (i *Issuer) metadata(w http.ResponseWriter, _ *http.Request) {
	i.mu.Lock()
	scopes := slices.Clone(i.scopes)
	i.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                i.URL,
		"authorization_endpoint":                i.URL + "/authorize",
		"token_endpoint":                        i.URL + "/token",
		"revocation_endpoint":                   i.URL + "/revoke",
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"scopes_supported":                      scopes,
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"private_key_jwt"},
		"protected_resources":                   []string{"https://" + i.domain},
	})
}

func (i *Issuer) token(w http.ResponseWriter, r *http.Request) {
	form, ok := i.authenticate(w, r)
	if !ok {
		return
	}
	switch form.Get("grant_type") {
	case "authorization_code":
		i.mu.Lock()
		g, found := i.codes[form.Get("code")]
		delete(i.codes, form.Get("code")) // a code is single use
		i.mu.Unlock()
		sum := sha256.Sum256([]byte(form.Get("code_verifier")))
		switch {
		case !found:
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "unknown code"})
			return
		case g.clientID != form.Get("client_id") || g.redirectURI != form.Get("redirect_uri"):
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "client_id or redirect_uri differs"})
			return
		case base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge:
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "code_verifier does not match"})
			return
		}
	case "refresh_token":
		if form.Get("refresh_token") == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request", "error_description": "no refresh_token"})
			return
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported_grant_type"})
		return
	}
	i.mu.Lock()
	var c canned
	if len(i.next) > 0 {
		c, i.next = i.next[0], i.next[1:]
	} else {
		c = canned{status: http.StatusOK, body: map[string]any{
			"access_token":  random(24),
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": random(24),
		}}
	}
	i.mu.Unlock()
	writeJSON(w, c.status, c.body)
}

func (i *Issuer) revoke(w http.ResponseWriter, r *http.Request) {
	if _, ok := i.authenticate(w, r); ok {
		w.WriteHeader(http.StatusOK)
	}
}

// authenticate parses the form, records the request and verifies its client
// assertion, answering 400 or 401 itself when it fails.
func (i *Issuer) authenticate(w http.ResponseWriter, r *http.Request) (url.Values, bool) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return nil, false
	}
	form := r.PostForm
	claims, err := i.verifyAssertion(form)
	i.mu.Lock()
	if err == nil {
		// RFC 7523 §3: an assertion's jti is used once.
		jti, _ := claims["jti"].(string)
		if i.jtis[jti] {
			err = fmt.Errorf("client assertion jti %q was already used", jti)
			claims = nil
		}
		i.jtis[jti] = true
	}
	i.reqs = append(i.reqs, Request{Path: r.URL.Path, Form: form, Claims: claims})
	i.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_client", "error_description": err.Error()})
		return nil, false
	}
	return form, true
}

// verifyAssertion checks the private_key_jwt client assertion (PAP §4.1, RFC
// 7523): ES256 under a key in the agent's jwks.json, iss = sub = client_id,
// aud the issuer as a single string, unexpired, living at most 5 minutes,
// with a jti. authenticate refuses a jti used before.
func (i *Issuer) verifyAssertion(form url.Values) (jwt.MapClaims, error) {
	if form.Get("client_assertion_type") != assertionType {
		return nil, fmt.Errorf("client_assertion_type %q", form.Get("client_assertion_type"))
	}
	if form.Get("client_id") != i.id.ClientID {
		return nil, fmt.Errorf("client_id %q is not %q", form.Get("client_id"), i.id.ClientID)
	}
	keys, err := es256Keys(i.id.JWKS())
	if err != nil {
		return nil, err
	}
	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(form.Get("client_assertion"), claims, func(tok *jwt.Token) (any, error) {
		kid, _ := tok.Header["kid"].(string)
		if k, ok := keys[kid]; ok {
			return k, nil
		}
		return nil, fmt.Errorf("no ES256 key %q in jwks.json", kid)
	}, jwt.WithValidMethods([]string{"ES256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil {
		return nil, fmt.Errorf("client assertion: %w", err)
	}
	if aud, ok := claims["aud"].(string); !ok || aud != i.URL {
		return nil, fmt.Errorf("client assertion aud %v is not the issuer %q as a string", claims["aud"], i.URL)
	}
	if claims["iss"] != i.id.ClientID || claims["sub"] != i.id.ClientID {
		return nil, errors.New("client assertion iss and sub must be the client_id")
	}
	iat, _ := claims.GetIssuedAt()
	exp, _ := claims.GetExpirationTime()
	if iat == nil || exp.Sub(iat.Time) > 5*time.Minute {
		return nil, errors.New("client assertion lives more than 5 minutes, or has no iat")
	}
	if jti, _ := claims["jti"].(string); jti == "" {
		return nil, errors.New("client assertion has no jti")
	}
	return claims, nil
}

// es256Keys reads the ES256 keys of a jwks.json document by kid.
func es256Keys(jwks []byte) (map[string]*ecdsa.PublicKey, error) {
	var doc struct {
		Keys []struct{ Kty, Crv, X, Y, Kid, Alg string } `json:"keys"`
	}
	if err := json.Unmarshal(jwks, &doc); err != nil {
		return nil, fmt.Errorf("jwks.json: %w", err)
	}
	keys := map[string]*ecdsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" || k.Alg != "ES256" {
			continue
		}
		x, errX := base64.RawURLEncoding.DecodeString(k.X)
		y, errY := base64.RawURLEncoding.DecodeString(k.Y)
		if errX != nil || errY != nil {
			return nil, errors.New("jwks.json: bad EC coordinates")
		}
		keys[k.Kid] = &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
	}
	return keys, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func random(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
