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
	domain, name, identifier, issuer, tokenEndpoint string
	userID, verifier                                string
	scopes                                          []string
	expiry                                          time.Time
}

// Flow runs PAP Direct Sign-In (§4.2): the authorization code grant with
// S256 PKCE, prompt=consent and a private_key_jwt client assertion (§4.1).
//
// Pending sign-ins are held in this Flow's memory only. The agent runs one
// replica, and a restart between Begin and the redirect back loses them, so
// the User signs in again (the redirect answers ErrUnknownState).
//
// Nothing here logs or returns a code, verifier, state or token, except the
// state inside the authorization URL Begin returns.
type Flow struct {
	HTTP     *http.Client
	Identity *identity.Identity
	Tokens   TokenStore
	Now      func() time.Time

	mu      sync.Mutex
	pending map[string]pending
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
	for k, p := range f.pending {
		if now.After(p.expiry) {
			delete(f.pending, k)
		}
	}
	f.pending[state] = pending{
		domain:        co.Domain,
		name:          co.Name,
		identifier:    co.Identifier,
		issuer:        co.Issuer,
		tokenEndpoint: md.TokenEndpoint,
		userID:        userID,
		verifier:      verifier,
		scopes:        requested,
		expiry:        now.Add(pendingLifetime),
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
// stored for the Company and User.
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

	tr, err := f.exchange(ctx, p, code)
	if err != nil {
		return res, err
	}
	granted, err := checkTokenResponse(tr, p.scopes)
	if err != nil {
		return res, fmt.Errorf("signin: %s's token response: %w", p.name, err)
	}
	if len(granted) == 0 {
		res.Declined = true
		log.Printf("signin: the sign-in at %s granted no scope", p.domain)
		return res, nil
	}
	expiresIn := defaultExpiresIn
	if tr.ExpiresIn != nil {
		expiresIn = time.Duration(*tr.ExpiresIn) * time.Second
	}
	if err := f.Tokens.Put(p.identifier, p.userID, Tokens{
		Access:  tr.AccessToken,
		Refresh: tr.RefreshToken,
		Scopes:  granted,
		Expiry:  f.now().Add(expiresIn),
	}); err != nil {
		return res, err
	}
	res.Granted = granted
	log.Printf("signin: signed in at %s with scopes %v", p.domain, granted)
	return res, nil
}

// exchange POSTs the authorization code to the token endpoint with the
// PKCE verifier and a client assertion addressed to the issuer (§4.1).
func (f *Flow) exchange(ctx context.Context, p pending, code string) (*tokenResponse, error) {
	assertion, err := f.Identity.ClientAssertion(p.issuer, f.now())
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"grant_type":            {"authorization_code"},
		"code":                  {code},
		"redirect_uri":          {f.Identity.RedirectURI},
		"code_verifier":         {p.verifier},
		"client_id":             {f.Identity.ClientID},
		"client_assertion_type": {assertionType},
		"client_assertion":      {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("signin: token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	hc := f.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	client := *hc
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		// A *url.Error names only the method and token endpoint; the form,
		// with the code and verifier, is in the body.
		return nil, fmt.Errorf("signin: token request to %s: %w", p.name, err)
	}
	defer resp.Body.Close()

	var tr tokenResponse
	// A decoding error is not wrapped: it can quote the body, which holds
	// the tokens.
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, maxTokenResponse)).Decode(&tr)
	if resp.StatusCode != http.StatusOK {
		// Only the error code is shown: error_description is the issuer's
		// free text.
		if decodeErr == nil && tr.Error != "" {
			return nil, fmt.Errorf("signin: %s refused the token request: %s (status %d)", p.name, tr.Error, resp.StatusCode)
		}
		return nil, fmt.Errorf("signin: %s refused the token request (status %d)", p.name, resp.StatusCode)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("signin: %s's token response is not a JSON token response", p.name)
	}
	return &tr, nil
}

// checkTokenResponse checks a sign-in's token response (§4.4, §4.5, §4.7)
// and returns the scopes it granted, sorted: those requested when it leaves
// scope out, and otherwise its scope, which must be within the request.
func checkTokenResponse(tr *tokenResponse, requested []string) ([]string, error) {
	switch {
	case tr.AccessToken == "":
		return nil, errors.New("no access_token")
	case !strings.EqualFold(tr.TokenType, "Bearer"):
		// Omnia's facade accepts Bearer tokens only, and this agent sends no
		// DPoP proofs.
		return nil, fmt.Errorf("token_type %q is not Bearer", tr.TokenType)
	case tr.RefreshToken == "":
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
