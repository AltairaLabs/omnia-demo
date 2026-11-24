// Package conversation holds the Personal Agent's side of Personal Agent
// Protocol conversations (PAP 1.0, §8): a Client that starts, continues,
// reads and closes them with a freshly signed Session header on every
// request (§5.2), a Reader that streams their events (§8.6), and the Store
// that keeps User IDs, conversations, events and cursors. It is written
// from the spec.
package conversation

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/discovery"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/identity"
)

const (
	protocolVersion = "1.0"
	maxBody         = 1 << 20 // 1 MiB
	attempts        = 3
	retryAfterCap   = 10 * time.Second
)

// CodeSignInRequired is the Error.Code of a 401 that a refresh can't fix:
// the request needs the User to sign in at the Company (§5.4, section 7).
const CodeSignInRequired = "sign_in_required"

// TokenSource gives the access token to send to a Company for a User ID.
// Tokens are kept per Company identifier and User ID, so one is only ever
// found for the Company that granted it.
type TokenSource interface {
	// AccessToken is the current token, refreshed when it expires within a minute; "" for none.
	AccessToken(ctx context.Context, co *discovery.Company, userID string) (string, error)
	// Refresh refreshes after the Company refused the token; "" when the tokens are gone.
	Refresh(ctx context.Context, co *discovery.Company, userID string) (string, error)
}

var (
	_ Store = (*MemoryStore)(nil)
	_ Store = (*RedisStore)(nil)
)

// Error is a Company's error response (§5.3, §8.13). Code is empty when the
// body had none; the caller then goes by Status.
type Error struct {
	Status         int
	Code           string
	Description    string
	ConversationID string
}

func (e *Error) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("conversation: HTTP %d", e.Status)
	}
	if e.Description != "" {
		return fmt.Sprintf("conversation: %s (HTTP %d): %s", e.Code, e.Status, e.Description)
	}
	return fmt.Sprintf("conversation: %s (HTTP %d)", e.Code, e.Status)
}

// IsCode reports whether err is a Company error with the given code.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// Client talks to Companies' poppy conversation endpoints for the Users
// whose IDs and keys are in Store.
type Client struct {
	HTTP  *http.Client
	Store Store
	// Now is the clock Session JWTs are signed with.
	Now func() time.Time
	// Backoff is the wait before the first retry; it doubles each time.
	Backoff time.Duration
	// Discover finds a Company not seen since this process started, such as
	// one a stored conversation names after a restart.
	Discover func(ctx context.Context, domain string) (*discovery.Company, error)
	// Tokens gives the access tokens of signed-in Users. nil means every
	// request is signed out.
	Tokens TokenSource

	mu        sync.Mutex
	companies map[string]*discovery.Company // by domain
	sids      map[string]string             // Session ID by User ID
}

// NewClient returns a Client that sends with hc and keeps state in st.
func NewClient(hc *http.Client, st Store) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{
		HTTP: hc, Store: st, Now: time.Now, Backoff: 250 * time.Millisecond,
		Discover: func(ctx context.Context, domain string) (*discovery.Company, error) {
			return discovery.Discover(ctx, hc, domain)
		},
		companies: map[string]*discovery.Company{},
		sids:      map[string]string{},
	}
}

// Remember records a discovered Company, so conversations with it can be
// continued without discovering it again.
func (c *Client) Remember(co *discovery.Company) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.companies[co.Domain] = co
}

func (c *Client) company(ctx context.Context, domain string) (*discovery.Company, error) {
	c.mu.Lock()
	co := c.companies[domain]
	c.mu.Unlock()
	if co != nil {
		return co, nil
	}
	co, err := c.Discover(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("conversation: rediscovering %s: %w", domain, err)
	}
	c.Remember(co)
	return co, nil
}

// sid returns the Session ID for a User ID: one random ID per User ID for
// the life of this process (§5.2). rotate replaces it, after session_ended.
func (c *Client) sid(userID string, rotate bool) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.sids[userID]; ok && !rotate {
		return s
	}
	s := randomID()
	c.sids[userID] = s
	return s
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error.
	return base64.RawURLEncoding.EncodeToString(b)
}

// author is a message's author (§8.4, §8.8).
type author struct {
	Type string `json:"type"`
}

type message struct {
	Author author `json:"author"`
	Text   string `json:"text"`
}

// postBody is the body of every conversation POST (§8.3, §8.10, §8.12).
type postBody struct {
	ID                   string   `json:"id"`
	ParentConversationID string   `json:"parent_conversation_id,omitempty"`
	Message              *message `json:"message,omitempty"`
}

type postResponse struct {
	ConversationID string `json:"conversation_id"`
}

// Start begins a conversation with agent at co with its first message, in
// the Personal Agent's own words (§8.3), and stores it.
func (c *Client) Start(ctx context.Context, co *discovery.Company, agent discovery.AgentEntry, text string) (Conversation, error) {
	c.Remember(co)
	body := postBody{ID: randomID(), Message: &message{Author: author{Type: "agent"}, Text: text}}
	id, err := c.post(ctx, co, agent.Endpoint, body)
	if err != nil {
		return Conversation{}, err
	}
	conv := Conversation{ID: id, Company: co.Domain, Name: co.Name, Endpoint: agent.Endpoint}
	return conv, c.Store.SaveConversation(conv)
}

// OpenDirect starts a Direct Conversation linked to parent (§8.10). Its
// first message is the User's own words, so its author is human (§8.8).
func (c *Client) OpenDirect(ctx context.Context, parent Conversation, text string) (Conversation, error) {
	co, err := c.company(ctx, parent.Company)
	if err != nil {
		return Conversation{}, err
	}
	body := postBody{
		ID:                   randomID(),
		ParentConversationID: parent.ID,
		Message:              &message{Author: author{Type: "human"}, Text: text},
	}
	id, err := c.post(ctx, co, parent.Endpoint, body)
	if err != nil {
		return Conversation{}, err
	}
	name := parent.Name
	if name == "" {
		name = co.Name
	}
	conv := Conversation{ID: id, Company: parent.Company, Name: name, Endpoint: parent.Endpoint, ParentID: parent.ID}
	return conv, c.Store.SaveConversation(conv)
}

// Send sends another message (§8.3): in the Personal Agent's own words, or
// in a Direct Conversation the User's.
func (c *Client) Send(ctx context.Context, conv Conversation, text string) error {
	co, err := c.company(ctx, conv.Company)
	if err != nil {
		return err
	}
	// Every message in a Direct Conversation is the User's own (§8.8, §8.10).
	who := "agent"
	if conv.ParentID != "" {
		who = "human"
	}
	body := postBody{ID: randomID(), Message: &message{Author: author{Type: who}, Text: text}}
	_, err = c.post(ctx, co, convURL(conv, "messages"), body)
	return err
}

// Close asks the Company to close the conversation (§8.12) and marks it
// closed in the store: the Personal Agent sends nothing more on it.
func (c *Client) Close(ctx context.Context, conv Conversation) error {
	co, err := c.company(ctx, conv.Company)
	if err != nil {
		return err
	}
	if _, err := c.post(ctx, co, convURL(conv, "close"), postBody{ID: randomID()}); err != nil {
		return err
	}
	conv.Closed = true
	return c.Store.SaveConversation(conv)
}

func convURL(conv Conversation, parts ...string) string {
	u := strings.TrimSuffix(conv.Endpoint, "/") + "/" + url.PathEscape(conv.ID)
	for _, p := range parts {
		u += "/" + p
	}
	return u
}

func (c *Client) post(ctx context.Context, co *discovery.Company, u string, body postBody) (string, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	resp, err := c.do(ctx, co, http.MethodPost, u, b, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var pr postResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&pr); err != nil {
		return "", fmt.Errorf("conversation: decoding response from %s: %w", u, err)
	}
	if pr.ConversationID == "" {
		return "", fmt.Errorf("conversation: response from %s has no conversation_id", u)
	}
	return pr.ConversationID, nil
}

// readResponse is a read of a conversation's events (§8.5).
type readResponse struct {
	Events  []json.RawMessage `json:"events"`
	Cursor  string            `json:"cursor"`
	HasMore bool              `json:"has_more"`
	Working bool              `json:"working"`
	Handoff string            `json:"handoff"`
	Closed  bool              `json:"closed"`
}

// saveState records a read's state fields, which hold once has_more is
// false (§8.5).
func (c *Client) saveState(convID string, rr readResponse) error {
	if rr.HasMore {
		return nil
	}
	handoff := rr.Handoff
	if handoff == "" {
		handoff = "none"
	}
	return c.Store.SetState(convID, rr.Working, handoff, rr.Closed)
}

// ReadOnce reads the conversation's events after its stored cursor, holding
// the first read up to wait, and keeps reading while has_more is true
// (§8.5). It stores each event it hasn't seen and the cursor after them,
// and returns those new events. A cursor_expired read starts again from the
// beginning, skipping events already stored.
func (c *Client) ReadOnce(ctx context.Context, conv Conversation, wait time.Duration) ([]Event, error) {
	evs, _, err := c.read(ctx, conv, wait)
	return evs, err
}

// read is ReadOnce, also reporting the conversation's closed state.
func (c *Client) read(ctx context.Context, conv Conversation, wait time.Duration) ([]Event, bool, error) {
	co, err := c.company(ctx, conv.Company)
	if err != nil {
		return nil, false, err
	}
	seen, err := c.seen(conv.ID)
	if err != nil {
		return nil, false, err
	}
	cursor, err := c.Store.Cursor(conv.ID)
	if err != nil {
		return nil, false, err
	}
	var out []Event
	for {
		q := url.Values{}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if secs := int(wait / time.Second); secs > 0 {
			q.Set("wait", strconv.Itoa(secs))
			wait = 0 // only the first read waits; has_more means read right away
		}
		u := convURL(conv, "events")
		if len(q) > 0 {
			u += "?" + q.Encode()
		}
		resp, err := c.do(ctx, co, http.MethodGet, u, nil, nil)
		if IsCode(err, "cursor_expired") && cursor != "" {
			cursor = ""
			if err := c.Store.SetCursor(conv.ID, ""); err != nil {
				return out, false, err
			}
			continue
		}
		if err != nil {
			return out, false, err
		}
		var rr readResponse
		err = json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&rr)
		resp.Body.Close()
		if err != nil {
			return out, false, fmt.Errorf("conversation: decoding events: %w", err)
		}
		for _, raw := range rr.Events {
			e, err := parseEvent(raw)
			if err != nil || e.ID == "" || seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			if err := c.Store.AppendEvent(conv.ID, e); err != nil {
				return out, false, err
			}
			out = append(out, e)
		}
		if rr.Cursor != "" {
			cursor = rr.Cursor
			if err := c.Store.SetCursor(conv.ID, cursor); err != nil {
				return out, false, err
			}
		}
		if !rr.HasMore {
			return out, rr.Closed, c.saveState(conv.ID, rr)
		}
	}
}

// seen is the set of event IDs already stored for a conversation.
func (c *Client) seen(convID string) (map[string]bool, error) {
	stored, err := c.Store.Events(convID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(stored))
	for _, e := range stored {
		seen[e.ID] = true
	}
	return seen, nil
}

// do sends one request with a Session JWT signed for it alone. It retries
// network errors, 5xx and 429 up to attempts times, re-signing each time.
// It waits at least Retry-After (§5.6), and gives up instead when that is
// longer than retryAfterCap. A POST retry sends
// the same body, and so the same request ID (§8.3). session_ended starts a
// new Session and sends the request once more (§5.5). A 401 invalid_token
// for a token sent refreshes it and sends the request once more, freshly
// signed (§5.5); any other 401 is CodeSignInRequired. Neither resend counts
// as an attempt. Any other 4xx is returned as an *Error without a retry. On
// success the caller closes the response body.
func (c *Client) do(ctx context.Context, co *discovery.Company, method, u string, body []byte, hdr http.Header) (*http.Response, error) {
	userID, err := c.Store.UserID(co.Identifier)
	if err != nil {
		return nil, err
	}
	backoff := c.Backoff
	rotated, refreshed := false, false
	var wait time.Duration // before the next send
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := sleep(ctx, wait); err != nil {
			return nil, err
		}
		wait = backoff
		backoff *= 2
		resp, sent, err := c.send(ctx, co, userID, method, u, body, hdr)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = fmt.Errorf("conversation: %s %s: %w", method, u, err)
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		bearer := bearerError(resp.Header)
		cerr := readError(resp)
		switch {
		case resp.StatusCode == http.StatusUnauthorized && sent && bearer == "invalid_token" && !refreshed:
			refreshed = true
			if _, err := c.Tokens.Refresh(ctx, co, userID); err != nil {
				// Not an *Error: the issuer failing is no reason to stop
				// following a conversation, so a Reader tries again later.
				return nil, fmt.Errorf("conversation: refreshing the token for %s: %w", co.Name, err)
			}
			attempt-- // a fresh token is not a retry of the same request
			wait = 0
			continue
		case resp.StatusCode == http.StatusUnauthorized:
			// The Company's description is dropped: the reason a Reader
			// records is the same for every sign-in it needs.
			return nil, &Error{Status: http.StatusUnauthorized, Code: CodeSignInRequired}
		case cerr.Code == "session_ended" && !rotated:
			rotated = true
			c.sid(userID, true)
			attempt-- // a new Session is not a retry of the same request
			wait = 0
			continue
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			ra := retryAfter(resp)
			if ra > retryAfterCap {
				return nil, cerr // §5.6: never sooner than Retry-After, so give up
			}
			wait = max(wait, ra)
			lastErr = cerr
			continue
		}
		return nil, cerr
	}
	return nil, lastErr
}

// send sends one request, signed now. It asks Tokens for the User's
// current access token at every send, so a retry carries a refreshed one,
// and reports whether it sent one.
func (c *Client) send(ctx context.Context, co *discovery.Company, userID, method, u string, body []byte, hdr http.Header) (*http.Response, bool, error) {
	var token string
	if c.Tokens != nil && mayCarryToken(co, u) {
		t, err := c.Tokens.AccessToken(ctx, co, userID)
		if err != nil {
			return nil, false, err
		}
		token = t
	}
	d, err := c.Store.Delegation(userID)
	if err != nil {
		return nil, false, err
	}
	jwt, err := d.SignSession(identity.SessionClaims{
		Aud: co.Identifier, Sub: userID, Sid: c.sid(userID, false), Method: method, URL: u,
		AccessToken: token,
	}, c.Now())
	if err != nil {
		return nil, false, err
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, false, err
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Poppy-Session", jwt)
	req.Header.Set("Poppy-Version", protocolVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	return resp, token != "", err
}

// mayCarryToken reports whether a request to u may carry co's default
// access token (§4.5): u is one of co's discovered conversation endpoints,
// or under one, and that endpoint's metadata names no resource of its own.
// Such an endpoint takes only tokens issued for it, which this agent does
// not request, so it is used signed out.
func mayCarryToken(co *discovery.Company, u string) bool {
	for _, a := range co.Agents {
		ep := strings.TrimSuffix(a.Endpoint, "/")
		if u != ep && !strings.HasPrefix(u, ep+"/") && !strings.HasPrefix(u, ep+"?") {
			continue
		}
		return a.Resource == "" || a.Resource == "https://"+co.Domain
	}
	return false
}

// bearerError is the error parameter of a Bearer challenge in
// WWW-Authenticate (RFC 6750 §3), or "" when there is none.
func bearerError(h http.Header) string {
	for _, v := range h.Values("WWW-Authenticate") {
		scheme, params, _ := strings.Cut(strings.TrimSpace(v), " ")
		if !strings.EqualFold(scheme, "Bearer") {
			continue
		}
		if m := bearerErrorParam.FindStringSubmatch(params); m != nil {
			return m[1] + m[2]
		}
	}
	return ""
}

// bearerErrorParam matches error="..." or error=token among a challenge's
// parameters.
var bearerErrorParam = regexp.MustCompile(`(?:^|[\s,])error\s*=\s*(?:"([^"]*)"|([^\s,"]+))`)

func readError(resp *http.Response) *Error {
	defer resp.Body.Close()
	var b struct {
		Error          string `json:"error"`
		Description    string `json:"error_description"`
		ConversationID string `json:"conversation_id"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&b)
	return &Error{Status: resp.StatusCode, Code: b.Error, Description: b.Description, ConversationID: b.ConversationID}
}

// retryAfter reads Retry-After as seconds or an HTTP date.
func retryAfter(resp *http.Response) time.Duration {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	var d time.Duration
	if secs, err := strconv.Atoi(v); err == nil {
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(v); err == nil {
		d = time.Until(t)
	}
	return max(d, 0)
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
