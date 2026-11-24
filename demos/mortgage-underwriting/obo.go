package main

// The account routes' gate: a Company API that serves a customer only when
// the request carries an on-behalf-of access token from that Company's
// issuer.
//
// Ellen's Personal Agent signs in to the Company (PAP Direct Sign-In) and
// holds the Company's token. On a tool call, Omnia's token exchange swaps it
// for one scoped to this API (RFC 8693), and the stub checks that token:
// signed RS256 with a key from the issuer's JWKS, typed as an access token
// (RFC 9068), from the configured issuer, for this API's audience,
// unexpired, and carrying the route's scope. Anything else is refused with
// 401 or 403 (RFC 6750), and there is no fallback: these routes never read
// X-Omnia-User-Email, which the older tools in this binary trust through the
// edge. A request with only that header gets 401.
//
// STAND-IN: account linking. This binary fakes each Company's account
// systems, and accountHolders fakes the link from a sign-in identity to an
// account. The member is the token's `email` claim, which the issuer gateway
// (Omnia #4443, as proposed) copies from the upstream identity provider only
// when the provider marks it email_verified; the stub matches it against the
// member data it already holds (identitydata.go for Larkmere, wrendata.go for
// Wrenmoor). A real account system would hold its own link from the identity
// provider's user to the account, made when the customer first registered,
// and would not depend on an email matching. Downstream: a token without an
// email claim is refused (401), so a gateway that does not map `email` locks
// every customer out; and whoever controls an email at the upstream provider
// is that member here.

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accountAPILarkmere = "larkmere"
	accountAPIWrenmoor = "wrenmoor"

	// jwksRefetchInterval bounds how often an unknown kid sends the stub back
	// to the issuer, so a stream of junk tokens can't make it hammer the
	// issuer.
	jwksRefetchInterval = 30 * time.Second
	maxIssuerDocSize    = 1 << 20 // 1 MiB

	errInvalidToken      = "invalid_token"
	errInsufficientScope = "insufficient_scope"
	errNoAccount         = "no account for this customer"
	errNotConfigured     = "account access is not configured"
)

// accountConfig says which Company's account routes this stub serves, and
// the verifier guarding them. The zero value serves none.
type accountConfig struct {
	api      string // "", accountAPILarkmere or accountAPIWrenmoor
	verifier *oboVerifier
}

// register adds the account routes of the Company acct.api names. Every one
// is wrapped in acct.verifier.guard.
func (acct accountConfig) register(mux *http.ServeMux) {
	switch acct.api {
	case accountAPILarkmere, accountAPIWrenmoor:
		// No account routes yet for either Company.
	}
}

// accountHolders answers whether a verified email belongs to one of the
// Company's customers (see the STAND-IN note above).
func accountHolders(api string) (func(email string) bool, error) {
	switch api {
	case accountAPILarkmere:
		return func(email string) bool {
			_, ok := cardholders[email]
			return ok
		}, nil
	case accountAPIWrenmoor:
		return func(email string) bool { return strings.EqualFold(email, wrenEmail) }, nil
	default:
		return nil, fmt.Errorf("account api %q: want %q or %q", api, accountAPILarkmere, accountAPIWrenmoor)
	}
}

// oboVerifier checks exchanged access tokens from one issuer for one
// audience.
type oboVerifier struct {
	issuer, audience string
	known            func(email string) bool
	hc               *http.Client
	now              func() time.Time

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey // by kid
	lastFetch time.Time                 // zero until the first fetch
}

func newOBOVerifier(issuer, audience string, known func(email string) bool, hc *http.Client, now func() time.Time) *oboVerifier {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	if now == nil {
		now = time.Now
	}
	return &oboVerifier{issuer: issuer, audience: audience, known: known, hc: hc, now: now}
}

// oboClaims are the claims an exchanged token is checked for beyond the
// registered ones.
type oboClaims struct {
	jwt.RegisteredClaims
	Scope         string `json:"scope"`
	Email         any    `json:"email"`
	EmailVerified any    `json:"email_verified"`
}

// member answers the member the request's bearer token was issued for, or
// the HTTP status and error to refuse it with. It reads nothing but the
// Authorization header.
func (v *oboVerifier) member(r *http.Request, scope string) (member string, status int, code string) {
	m, status, code, _ := v.check(r, scope)
	return m, status, code
}

// check is member with the reason for a refusal, which goes to the log only.
// The order is the brief's: the token itself, then its scope, then the
// account it names.
func (v *oboVerifier) check(r *http.Request, scope string) (member string, status int, code, reason string) {
	scheme, raw, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	raw = strings.TrimSpace(raw)
	if !ok || !strings.EqualFold(scheme, "Bearer") || raw == "" {
		return "", http.StatusUnauthorized, errInvalidToken, "no bearer token"
	}

	var claims oboClaims
	_, err := jwt.ParseWithClaims(raw, &claims,
		func(t *jwt.Token) (any, error) { return v.key(r.Context(), t) },
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		return "", http.StatusUnauthorized, errInvalidToken, err.Error()
	}

	email, ok := claims.Email.(string)
	email = strings.ToLower(strings.TrimSpace(email))
	if !ok || email == "" {
		return "", http.StatusUnauthorized, errInvalidToken, "no email claim"
	}
	if verified, present := claims.EmailVerified.(bool); claims.EmailVerified != nil && (!present || !verified) {
		return "", http.StatusUnauthorized, errInvalidToken, "email not verified"
	}
	if !hasScope(claims.Scope, scope) {
		return "", http.StatusForbidden, errInsufficientScope, "scope " + scope + " not granted"
	}
	if v.known == nil || !v.known(email) {
		return email, http.StatusNotFound, errNoAccount, "no account"
	}
	return email, http.StatusOK, "", ""
}

func hasScope(granted, want string) bool {
	for _, s := range strings.Fields(granted) {
		if s == want {
			return true
		}
	}
	return false
}

// key is the keyfunc: the token must be typed as an access token and name a
// kid the issuer publishes. An unknown kid refetches the issuer's keys, at
// most once every jwksRefetchInterval.
func (v *oboVerifier) key(ctx context.Context, t *jwt.Token) (any, error) {
	typ, _ := t.Header["typ"].(string)
	if !strings.EqualFold(typ, "at+jwt") && !strings.EqualFold(typ, "application/at+jwt") {
		return nil, fmt.Errorf("typ %q is not at+jwt", typ)
	}
	kid, _ := t.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("no kid")
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	if !v.lastFetch.IsZero() && v.now().Sub(v.lastFetch) < jwksRefetchInterval {
		return nil, fmt.Errorf("unknown kid %q", kid)
	}
	v.lastFetch = v.now()
	keys, err := v.fetchKeys(ctx)
	if err != nil {
		return nil, err
	}
	v.keys = keys
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("unknown kid %q", kid)
}

// fetchKeys reads the issuer's RFC 8414 metadata, checks it names this
// issuer, and loads the RSA signing keys from its jwks_uri.
func (v *oboVerifier) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	u, err := url.Parse(v.issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("issuer %q is not an https URL", v.issuer)
	}
	// RFC 8414 §3.1: the well-known path goes between the host and the path.
	wellKnown := "https://" + u.Host + "/.well-known/oauth-authorization-server" + strings.TrimSuffix(u.EscapedPath(), "/")
	var meta struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := v.getJSON(ctx, wellKnown, &meta); err != nil {
		return nil, err
	}
	if meta.Issuer != v.issuer {
		return nil, fmt.Errorf("metadata names issuer %q, want %q", meta.Issuer, v.issuer)
	}
	if ju, err := url.Parse(meta.JWKSURI); err != nil || ju.Scheme != "https" || ju.Host == "" {
		return nil, fmt.Errorf("jwks_uri %q is not an https URL", meta.JWKSURI)
	}

	var set struct {
		Keys []struct {
			Kty, Use, Alg, Kid, N, E string
		} `json:"keys"`
	}
	if err := v.getJSON(ctx, meta.JWKSURI, &set); err != nil {
		return nil, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" || (k.Use != "" && k.Use != "sig") || (k.Alg != "" && k.Alg != "RS256") {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if errN != nil || errE != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	return keys, nil
}

func (v *oboVerifier) getJSON(ctx context.Context, target string, into any) error {
	client := *v.hc
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: status %d", target, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxIssuerDocSize)).Decode(into); err != nil {
		return fmt.Errorf("decoding %s: %w", target, err)
	}
	return nil
}

// guard wraps an account route: POST only, and fn runs only for a verified
// member. A refusal answers {"error"} and, for 401 and 403, a
// WWW-Authenticate challenge (RFC 6750 §3). Each request is logged with the
// tool, status and member; never the token or the Authorization header.
func (v *oboVerifier) guard(tool, scope string, fn func(member string, w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if v == nil {
			log.Printf("account request: tool=%s status=%d reason=%q", tool, http.StatusServiceUnavailable, errNotConfigured)
			writeAccountError(w, http.StatusServiceUnavailable, errNotConfigured)
			return
		}
		member, status, code, reason := v.check(r, scope)
		if status != http.StatusOK {
			log.Printf("account request: tool=%s status=%d member=%q reason=%q remote=%s", tool, status, member, reason, r.RemoteAddr)
			switch status {
			case http.StatusUnauthorized:
				w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer error=%q", code))
			case http.StatusForbidden:
				w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer error=%q, scope=%q", code, scope))
			}
			writeAccountError(w, status, code)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		fn(member, rec, r)
		log.Printf("account request: tool=%s status=%d member=%q remote=%s", tool, rec.status, member, r.RemoteAddr)
	}
}

func writeAccountError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody(msg))
}

// statusRecorder keeps the status an account route answered, for the log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
