package signin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/discovery"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/identity"
)

const (
	// pendingLifetime is how long a state is valid between Begin and the
	// redirect back.
	pendingLifetime = 10 * time.Minute
	// defaultExpiresIn is taken when a token response has no expires_in.
	defaultExpiresIn = 5 * time.Minute
	// maxExpiresIn caps expires_in, so a huge value can't overflow into an
	// expiry in the past.
	maxExpiresIn = 24 * time.Hour
	// refreshAhead is how long before its expiry an access token is
	// refreshed, so it doesn't expire in flight.
	refreshAhead = time.Minute
	// maxPendingPerUser caps one User's sign-ins between Begin and the
	// redirect back; a new one past it drops that User's oldest.
	maxPendingPerUser = 8
	// revokeTimeout bounds a best-effort revocation.
	revokeTimeout    = 5 * time.Second
	maxTokenResponse = 1 << 20 // 1 MiB

	assertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
)

var (
	// ErrUnknownState is a redirect whose state this Flow did not start, has
	// already completed, or let expire.
	ErrUnknownState = errors.New("signin: unknown, used or expired sign-in")
	// ErrIssuerMismatch is a redirect whose iss is missing or is not the
	// issuer the sign-in started with (RFC 9207, PAP §4.2).
	ErrIssuerMismatch = errors.New("signin: the redirect's iss is not the issuer the sign-in started with")
)

// pending is a sign-in between Begin and Complete. verifier is a secret.
type pending struct {
	domain, name, identifier, issuer  string
	tokenEndpoint, revocationEndpoint string
	userID, verifier                  string
	scopes                            []string
	expiry                            time.Time
}

// Flow runs PAP Direct Sign-In (§4.2): the authorization code grant with
// S256 PKCE, prompt=consent and a private_key_jwt client assertion (§4.1).
//
// Pending sign-ins are held in this Flow's memory only. The agent runs one
// replica, and a restart between Begin and the redirect back loses them, so
// the User signs in again (the redirect answers ErrUnknownState).
//
// Nothing here logs or returns a code, verifier, state or token, except the
// state inside the authorization URL Begin returns, and the access token
// AccessToken and Refresh return for the Company it was granted by.
//
// A Flow is also the conversation Client's TokenSource: AccessToken and
// Refresh keep a User's access token fresh (§4.4).
type Flow struct {
	HTTP     *http.Client
	Identity *identity.Identity
	Tokens   TokenStore
	Now      func() time.Time

	mu      sync.Mutex
	pending map[string]pending
	locks   map[[2]string]*sync.Mutex // per Company identifier and User ID
}

// Result is how a sign-in ended. Granted is sorted; Declined means the User
// declined or approved no scope, and nothing was stored.
type Result struct {
	Domain, Name string
	Granted      []string
	Declined     bool
}

// NewFlow returns a Flow that talks to issuers with hc, authenticates as id
// and stores what sign-ins grant in ts.
func NewFlow(hc *http.Client, id *identity.Identity, ts TokenStore) *Flow {
	return &Flow{HTTP: hc, Identity: id, Tokens: ts, Now: time.Now}
}

func (f *Flow) now() time.Time {
	if f.Now == nil {
		return time.Now()
	}
	return f.Now()
}

// Begin starts a sign-in for userID at co and returns the authorization URL
// to open in the User's browser. It checks the issuer's metadata (§3.2)
// first, and asks for the scopes given plus those already granted to this
// User at this Company (§4.7), sorted.
func (f *Flow) Begin(ctx context.Context, co *discovery.Company, userID string, scopes []string) (string, error) {
	if co == nil {
		return "", errors.New("signin: no company")
	}
	if co.Issuer == "" || co.SignInDisabled {
		return "", fmt.Errorf("signin: %s offers no sign-in", co.Name)
	}
	md, err := FetchMetadata(ctx, f.HTTP, co.Issuer, co.Domain)
	if err != nil {
		return "", err
	}

	// §4.7: the scopes in either scopes_supported list, or described in
	// poppy.json auth.scopes.
	offered := slices.Concat(co.ScopesSupported, md.ScopesSupported, slices.Collect(maps.Keys(co.Scopes)))
	slices.Sort(offered)
	offered = slices.Compact(offered)
	for _, s := range scopes {
		if !slices.Contains(offered, s) {
			return "", fmt.Errorf("signin: %s does not offer the scope %q; it offers %s", co.Name, s, strings.Join(offered, ", "))
		}
	}
	requested := slices.Clone(scopes)
	prev, err := f.Tokens.Get(co.Identifier, userID)
	if err != nil {
		return "", err
	}
	if prev != nil {
		requested = append(requested, prev.Scopes...)
	}
	slices.Sort(requested)
	requested = slices.Compact(requested)
	if len(requested) == 0 || requested[0] == "" {
		return "", errors.New("signin: a sign-in must request at least one scope")
	}

	authURL, err := url.Parse(md.AuthorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("signin: authorization_endpoint: %w", err)
	}
	state := randomB64(16)
	verifier := randomB64(32)
	challenge := sha256.Sum256([]byte(verifier))

	q := authURL.Query()
	q.Set("response_type", "code")
	q.Set("client_id", f.Identity.ClientID)
	q.Set("redirect_uri", f.Identity.RedirectURI)
	q.Set("scope", strings.Join(requested, " "))
	q.Set("state", state)
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	q.Set("code_challenge_method", "S256")
	q.Set("prompt", "consent")
	authURL.RawQuery = q.Encode()

	now := f.now()
	f.mu.Lock()
	if f.pending == nil {
		f.pending = map[string]pending{}
	}
	var oldest string
	mine := 0
	for k, p := range f.pending {
		switch {
		case now.After(p.expiry):
			delete(f.pending, k)
		case p.userID == userID:
			mine++
			if oldest == "" || p.expiry.Before(f.pending[oldest].expiry) {
				oldest = k
			}
		}
	}
	if mine >= maxPendingPerUser {
		delete(f.pending, oldest)
	}
	f.pending[state] = pending{
		domain:             co.Domain,
		name:               co.Name,
		identifier:         co.Identifier,
		issuer:             co.Issuer,
		tokenEndpoint:      md.TokenEndpoint,
		revocationEndpoint: md.RevocationEndpoint,
		userID:             userID,
		verifier:           verifier,
		scopes:             requested,
		expiry:             now.Add(pendingLifetime),
	}
	f.mu.Unlock()

	log.Printf("signin: began a sign-in at %s for scopes %v", co.Domain, requested)
	return authURL.String(), nil
}

// tokenResponse is an RFC 6749 §5.1 token response. Scope is nil when the
// response leaves scope out, which grants exactly the scopes requested.
type tokenResponse struct {
	AccessToken  string  `json:"access_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    *int64  `json:"expires_in"`
	RefreshToken string  `json:"refresh_token"`
	Scope        *string `json:"scope"`
	Error        string  `json:"error"`
}

// Complete finishes the sign-in the redirect query q answers. The state is
// used up first, whatever follows, so it works once. A redirect whose iss
// isn't the issuer the sign-in started with is refused, even a decline. A
// decline, or a grant of no scope, answers Declined and stores nothing.
// Otherwise the code is exchanged, the response checked, and the tokens
// stored for the Company and User. A refresh token the response issued but
// that isn't kept, and one the new sign-in replaces, are revoked (§4.7).
func (f *Flow) Complete(ctx context.Context, q url.Values) (Result, error) {
	state := q.Get("state")
	f.mu.Lock()
	p, ok := f.pending[state]
	delete(f.pending, state)
	f.mu.Unlock()
	if !ok || state == "" {
		return Result{}, ErrUnknownState
	}
	if f.now().After(p.expiry) {
		return Result{}, fmt.Errorf("%w: the sign-in at %s expired", ErrUnknownState, p.domain)
	}
	res := Result{Domain: p.domain, Name: p.name}
	if len(q["iss"]) != 1 || q.Get("iss") != p.issuer {
		return res, ErrIssuerMismatch
	}

	if e := q.Get("error"); e != "" {
		if e == "access_denied" {
			res.Declined = true
			log.Printf("signin: the sign-in at %s was declined", p.domain)
			return res, nil
		}
		return res, fmt.Errorf("signin: %s answered the sign-in with error %q", p.name, e)
	}
	code := q.Get("code")
	if code == "" {
		return res, fmt.Errorf("signin: %s's redirect has no code", p.name)
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {f.Identity.RedirectURI},
		"code_verifier": {p.verifier},
	}
	tr, err := f.tokenRequest(ctx, p.issuer, p.tokenEndpoint, p.name, form)
	if err != nil {
		return res, err
	}
	granted, err := checkTokenResponse(tr, p.scopes, true)
	if err == nil && len(granted) == 0 {
		res.Declined = true
		log.Printf("signin: the sign-in at %s granted no scope", p.domain)
	}
	if err != nil || res.Declined {
		f.revoke(ctx, p.issuer, p.revocationEndpoint, p.domain, tr.RefreshToken)
		if err != nil {
			return res, fmt.Errorf("signin: %s's token response: %w", p.name, err)
		}
		return res, nil
	}

	lock := f.lock(p.identifier, p.userID)
	lock.Lock()
	defer lock.Unlock()
	prev, err := f.Tokens.Get(p.identifier, p.userID)
	if err != nil {
		prev = nil // unreadable: nothing to revoke, and Put replaces it
	}
	if err := f.Tokens.Put(p.identifier, p.userID, Tokens{
		Access:  tr.AccessToken,
		Refresh: tr.RefreshToken,
		Scopes:  granted,
		Expiry:  f.now().Add(expiresIn(tr)),
	}); err != nil {
		f.revoke(ctx, p.issuer, p.revocationEndpoint, p.domain, tr.RefreshToken)
		return res, err
	}
	if prev != nil && prev.Refresh != tr.RefreshToken {
		// §4.7: the new refresh token replaces the old one, which SHOULD be
		// revoked. This is not sign-out (§5.5).
		f.revoke(ctx, p.issuer, p.revocationEndpoint, p.domain, prev.Refresh)
	}
	res.Granted = granted
	log.Printf("signin: signed in at %s with scopes %v", p.domain, granted)
	return res, nil
}

// tokenRequest POSTs form to tokenEndpoint with the client assertion
// addressed to the issuer (§4.1), and decodes a 200 token response. name is
// the Company's, for errors.
func (f *Flow) tokenRequest(ctx context.Context, issuer, tokenEndpoint, name string, form url.Values) (*tokenResponse, error) {
	resp, err := f.postForm(ctx, issuer, tokenEndpoint, form)
	if err != nil {
		// A *url.Error names only the method and token endpoint; the form,
		// with the code, verifier or refresh token, is in the body.
		return nil, fmt.Errorf("signin: token request to %s: %w", name, err)
	}
	defer resp.Body.Close()

	var tr tokenResponse
	// A decoding error is not wrapped: it can quote the body, which holds
	// the tokens.
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, maxTokenResponse)).Decode(&tr)
	if resp.StatusCode != http.StatusOK {
		// Only the error code is shown, quoted: error_description is the
		// issuer's free text.
		if decodeErr == nil && tr.Error != "" {
			return nil, &tokenError{name: name, code: tr.Error, status: resp.StatusCode}
		}
		return nil, &tokenError{name: name, status: resp.StatusCode}
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("signin: %s's token response is not a JSON token response", name)
	}
	return &tr, nil
}

// tokenError is a token endpoint's refusal. code is the OAuth error code,
// "" when the body had none.
type tokenError struct {
	name, code string
	status     int
}

func (e *tokenError) Error() string {
	if e.code != "" {
		return fmt.Sprintf("signin: %s refused the token request: %q (status %d)", e.name, e.code, e.status)
	}
	return fmt.Sprintf("signin: %s refused the token request (status %d)", e.name, e.status)
}

// postForm POSTs form, with client_id and a fresh client assertion for
// issuer added, to endpoint, following no redirect. The caller closes the
// body.
func (f *Flow) postForm(ctx context.Context, issuer, endpoint string, form url.Values) (*http.Response, error) {
	assertion, err := f.Identity.ClientAssertion(issuer, f.now())
	if err != nil {
		return nil, err
	}
	form.Set("client_id", f.Identity.ClientID)
	form.Set("client_assertion_type", assertionType)
	form.Set("client_assertion", assertion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	hc := f.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	client := *hc
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client.Do(req)
}

// revoke revokes refresh at revocationEndpoint (RFC 7009, §4.6), when the
// issuer has one and there is a token. It is best effort: a failure is
// logged with the Company's domain only, never the token.
func (f *Flow) revoke(ctx context.Context, issuer, revocationEndpoint, domain, refresh string) {
	if revocationEndpoint == "" || refresh == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), revokeTimeout)
	defer cancel()
	resp, err := f.postForm(ctx, issuer, revocationEndpoint, url.Values{
		"token":           {refresh},
		"token_type_hint": {"refresh_token"},
	})
	if err != nil {
		log.Printf("signin: revoking a refresh token at %s failed", domain)
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxTokenResponse))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("signin: revoking a refresh token at %s answered status %d", domain, resp.StatusCode)
	}
}

// expiresIn is a token response's lifetime: expires_in capped at
// maxExpiresIn, or defaultExpiresIn without one.
func expiresIn(tr *tokenResponse) time.Duration {
	if tr.ExpiresIn == nil {
		return defaultExpiresIn
	}
	if *tr.ExpiresIn > int64(maxExpiresIn/time.Second) {
		return maxExpiresIn
	}
	return time.Duration(*tr.ExpiresIn) * time.Second
}

// lock is the mutex for one Company identifier and User ID, so one refresh
// runs at a time for them.
func (f *Flow) lock(identifier, userID string) *sync.Mutex {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.locks == nil {
		f.locks = map[[2]string]*sync.Mutex{}
	}
	k := [2]string{identifier, userID}
	if f.locks[k] == nil {
		f.locks[k] = &sync.Mutex{}
	}
	return f.locks[k]
}

// AccessToken is the access token to send to co for userID: the stored one,
// refreshed first when it expires within a minute (§4.4), or "" when there
// is none. If the refresh fails for any reason but invalid_grant, an
// unexpired token is still answered; an expired one is an error. On
// invalid_grant the tokens are deleted and it answers "" (§5.5).
func (f *Flow) AccessToken(ctx context.Context, co *discovery.Company, userID string) (string, error) {
	return f.access(ctx, co, userID, false)
}

// Refresh refreshes userID's tokens at co after the Company refused the
// access token (invalid_token), and answers the new access token, or ""
// when there are no tokens or the refresh answered invalid_grant, which
// deletes them (§5.5). Any other failure is an error.
func (f *Flow) Refresh(ctx context.Context, co *discovery.Company, userID string) (string, error) {
	return f.access(ctx, co, userID, true)
}

func (f *Flow) access(ctx context.Context, co *discovery.Company, userID string, force bool) (string, error) {
	if co == nil || co.Identifier == "" {
		return "", nil
	}
	lock := f.lock(co.Identifier, userID)
	lock.Lock()
	defer lock.Unlock()
	t, err := f.Tokens.Get(co.Identifier, userID)
	if err != nil || t == nil {
		return "", err
	}
	now := f.now()
	if !force && now.Before(t.Expiry.Add(-refreshAhead)) {
		return t.Access, nil
	}
	access, err := f.refresh(ctx, co, userID, t)
	if err != nil && !force && now.Before(t.Expiry) {
		log.Printf("signin: refreshing at %s failed; the current token has not expired, so it is used", co.Domain)
		return t.Access, nil
	}
	return access, err
}

// refresh sends the refresh token with grant_type=refresh_token and no scope
// (§4.4), and stores what it returns: the new access token, and the new
// refresh token when there is one (RFC 6749 §6). The scopes stay those of
// the sign-in. A rotated refresh token is not revoked: RFC 6749 §6 has the
// issuer retire it, and presenting it again can read as reuse.
func (f *Flow) refresh(ctx context.Context, co *discovery.Company, userID string, t *Tokens) (string, error) {
	if co.Issuer == "" {
		return "", fmt.Errorf("signin: %s has no issuer to refresh at", co.Name)
	}
	md, err := FetchMetadata(ctx, f.HTTP, co.Issuer, co.Domain)
	if err != nil {
		return "", err
	}
	tr, err := f.tokenRequest(ctx, co.Issuer, md.TokenEndpoint, co.Name, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {t.Refresh},
	})
	var te *tokenError
	if errors.As(err, &te) && te.code == "invalid_grant" {
		// §5.5: the refresh token is gone (revoked, expired, or the User
		// disconnected the agent). Continue signed out.
		if err := f.Tokens.Delete(co.Identifier, userID); err != nil {
			return "", err
		}
		log.Printf("signin: %s refused the refresh with invalid_grant; signed out there", co.Domain)
		return "", nil
	}
	if err != nil {
		return "", err
	}
	granted, err := checkTokenResponse(tr, t.Scopes, false)
	if err == nil && len(granted) == 0 {
		err = errors.New("granted no scope")
	}
	if err != nil {
		if tr.RefreshToken != t.Refresh {
			f.revoke(ctx, co.Issuer, md.RevocationEndpoint, co.Domain, tr.RefreshToken)
		}
		return "", fmt.Errorf("signin: %s's refresh response: %w", co.Name, err)
	}
	next := Tokens{Access: tr.AccessToken, Refresh: t.Refresh, Scopes: t.Scopes, Expiry: f.now().Add(expiresIn(tr))}
	if tr.RefreshToken != "" {
		next.Refresh = tr.RefreshToken
	}
	if err := f.Tokens.Put(co.Identifier, userID, next); err != nil {
		return "", err
	}
	log.Printf("signin: refreshed the access token at %s", co.Domain)
	return next.Access, nil
}

// checkTokenResponse checks a token response (§4.4, §4.5, §4.7) and returns
// the scopes it granted, sorted: those requested when it leaves scope out,
// and otherwise its scope, which must be within the request. A sign-in's
// response must have a refresh token (needRefresh); a refresh's may not.
func checkTokenResponse(tr *tokenResponse, requested []string, needRefresh bool) ([]string, error) {
	switch {
	case tr.AccessToken == "":
		return nil, errors.New("no access_token")
	case !strings.EqualFold(tr.TokenType, "Bearer"):
		// Omnia's facade accepts Bearer tokens only, and this agent sends no
		// DPoP proofs.
		return nil, fmt.Errorf("token_type %q is not Bearer", tr.TokenType)
	case needRefresh && tr.RefreshToken == "":
		return nil, errors.New("no refresh_token; a sign-in must return one (§4.4)")
	case tr.ExpiresIn != nil && *tr.ExpiresIn <= 0:
		return nil, fmt.Errorf("expires_in %d is not positive", *tr.ExpiresIn)
	}
	if tr.Scope == nil {
		return slices.Clone(requested), nil
	}
	granted := strings.Fields(*tr.Scope)
	slices.Sort(granted)
	granted = slices.Compact(granted)
	var extra []string
	for _, s := range granted {
		if !slices.Contains(requested, s) {
			extra = append(extra, s)
		}
	}
	if len(extra) > 0 {
		return nil, fmt.Errorf("granted scopes %v that were not requested (§4.7)", extra)
	}
	return granted, nil
}

// randomB64 returns n bytes from crypto/rand, base64url without padding.
func randomB64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error.
	return base64.RawURLEncoding.EncodeToString(b)
}
