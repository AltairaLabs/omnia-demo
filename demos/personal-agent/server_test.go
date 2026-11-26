package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/conversation"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/identity"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/signin"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/signin/signintest"
)

// testIdentity generates the two identity keys in-process and loads them.
func testIdentity(t *testing.T) *identity.Identity {
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

// company is a fake Poppy Company over TLS: discovery documents, a poppy
// endpoint that starts conversations, and an events stream that sends the
// events in stream, then holds the stream open.
type company struct {
	srv    *httptest.Server
	domain string

	mu     sync.Mutex
	n      int
	stream map[string][]map[string]any // events to send, by conversation
	posts  []map[string]any            // POST bodies, by arrival
	paths  []string                    // every request, "METHOD path"
	// closedAlready makes /close answer 409 conversation_closed, as for a
	// conversation the Company closed first.
	closedAlready bool
	// issuer, when set, is named in the protected resource metadata, so
	// the Company offers sign-in there.
	issuer string
	// requireToken makes /events answer 401 with no Bearer challenge
	// error, as for a conversation that needs the User's account, to a
	// request without an access token.
	requireToken bool
	// authz is the Authorization header of every request, "METHOD path
	// header".
	authz []string
	// challenge, when set, is the WWW-Authenticate of requireToken's 401.
	challenge string
	// gate, when set, holds requireToken's 401 until it is closed.
	gate chan struct{}
	// authScopes is poppy.json auth.scopes.
	authScopes map[string]string
}

func newCompany(t *testing.T, name string) *company {
	t.Helper()
	c := &company{stream: map[string][]map[string]any{}}
	c.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.paths = append(c.paths, r.Method+" "+r.URL.Path)
		c.authz = append(c.authz, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		issuer, requireToken, challenge, gate, authScopes := c.issuer, c.requireToken, c.challenge, c.gate, c.authScopes
		c.mu.Unlock()
		switch {
		case r.URL.Path == "/.well-known/poppy.json":
			writeJSONTest(w, 200, map[string]any{
				"organization": map[string]any{"name": name, "domain": c.domain},
				"auth":         map[string]any{"scopes": authScopes},
				"agent": map[string]any{"protocols": []any{map[string]any{
					"type": "poppy", "endpoint": c.srv.URL + "/poppy", "description": "Power cuts", "access": "session",
				}}},
			})
		case r.URL.Path == "/.well-known/oauth-protected-resource":
			if issuer == "" {
				http.NotFound(w, r)
				return
			}
			writeJSONTest(w, 200, map[string]any{
				"resource": "https://" + c.domain, "authorization_servers": []string{issuer},
			})
		case r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			c.mu.Lock()
			c.posts = append(c.posts, body)
			c.n++
			id := fmt.Sprintf("cnv_%d", c.n)
			closedAlready := c.closedAlready
			c.mu.Unlock()
			if closedAlready && strings.HasSuffix(r.URL.Path, "/close") {
				writeJSONTest(w, http.StatusConflict, map[string]string{"error": "conversation_closed"})
				return
			}
			if strings.HasSuffix(r.URL.Path, "/messages") || strings.HasSuffix(r.URL.Path, "/close") {
				id = strings.Split(strings.TrimPrefix(r.URL.Path, "/poppy/"), "/")[0]
				writeJSONTest(w, http.StatusAccepted, map[string]string{"conversation_id": id})
				return
			}
			writeJSONTest(w, http.StatusCreated, map[string]string{"conversation_id": id})
		case strings.HasSuffix(r.URL.Path, "/events"):
			if requireToken && r.Header.Get("Authorization") == "" {
				if gate != nil {
					<-gate
				}
				if challenge != "" {
					w.Header().Set("WWW-Authenticate", challenge)
				}
				writeJSONTest(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			id := strings.Split(strings.TrimPrefix(r.URL.Path, "/poppy/"), "/")[0]
			c.mu.Lock()
			evs := c.stream[id]
			c.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			for _, e := range evs {
				b, _ := json.Marshal(e)
				fmt.Fprintf(w, "id: %s\ndata: %s\n\n", e["id"], b)
			}
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(c.srv.Close)
	c.domain = strings.TrimPrefix(c.srv.URL, "https://")
	return c
}

func (c *company) setStream(conv string, evs ...map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stream[conv] = evs
}

func (c *company) authorizations() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.authz...)
}

func (c *company) requests() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.paths...)
}

func (c *company) postBodies() []map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]map[string]any(nil), c.posts...)
}

func msgEvent(id, text string) map[string]any {
	return map[string]any{
		"id": id, "type": "message", "created_at": "2026-11-23T10:00:00Z",
		"message": map[string]any{"id": "m_" + id, "role": "company", "author": map[string]any{"type": "agent"}, "text": text},
	}
}

func writeJSONTest(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// harness is a running server with its store, stopped at test end.
type harness struct {
	srv   *server
	http  *httptest.Server
	store *conversation.MemoryStore
	id    *identity.Identity
	// flow and iss are set by newSignInHarness.
	flow *signin.Flow
	iss  *signintest.Issuer
}

func newHarness(t *testing.T, hc *http.Client, known ...string) *harness {
	t.Helper()
	id := testIdentity(t)
	st := conversation.NewMemoryStore(id, time.Now)
	return newHarnessWith(t, hc, id, st, known...)
}

func newHarnessWith(t *testing.T, hc *http.Client, id *identity.Identity, st *conversation.MemoryStore, known ...string) *harness {
	t.Helper()
	return newHarnessConfig(t, config{Identity: id, Store: st, HTTP: hc, KnownCompanies: known})
}

// newSignInHarness is a harness whose agent can sign in at co, a known
// Company whose issuer is a signintest.Issuer offering "claims" and
// "claims:read".
func newSignInHarness(t *testing.T, co *company) *harness {
	t.Helper()
	id := testIdentity(t)
	iss := signintest.NewIssuer(t, id, co.domain)
	iss.SetScopesSupported("claims", "claims:read")
	co.mu.Lock()
	co.issuer = iss.URL
	co.mu.Unlock()
	// Every httptest TLS server has the same certificate, so the Company's
	// client trusts the issuer too.
	flow := signin.NewFlow(co.srv.Client(), id, signin.NewMemoryTokenStore())
	h := newHarnessConfig(t, config{
		Identity: id, Store: conversation.NewMemoryStore(id, time.Now), HTTP: co.srv.Client(),
		KnownCompanies: []string{co.domain}, SignIn: flow,
	})
	h.flow, h.iss = flow, iss
	return h
}

func newHarnessConfig(t *testing.T, cfg config) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cfg.ReaderBackoff = 10 * time.Millisecond
	st := cfg.Store.(*conversation.MemoryStore)
	id := cfg.Identity
	s := newServer(ctx, cfg)
	ts := httptest.NewServer(s.handler())
	t.Cleanup(func() {
		cancel()
		ts.CloseClientConnections()
		ts.Close()
		s.wait()
	})
	return &harness{srv: s, http: ts, store: st, id: id}
}

// call posts body to path and returns the status and decoded JSON body.
func (h *harness) call(t *testing.T, path string, body any) (int, map[string]any, string) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(h.http.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s answered %d with non-JSON %q", path, resp.StatusCode, raw)
	}
	return resp.StatusCode, out, string(raw)
}

// updates subscribes to /api/updates and returns a channel of its updates.
func (h *harness) updates(t *testing.T) <-chan map[string]any {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, h.http.URL+"/api/updates", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("updates Content-Type = %q", ct)
	}
	out := make(chan map[string]any, 64)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			data, ok := strings.CutPrefix(sc.Text(), "data: ")
			if !ok {
				continue
			}
			var u map[string]any
			if json.Unmarshal([]byte(data), &u) == nil {
				out <- u
			}
		}
	}()
	return out
}

func waitFor(t *testing.T, ch <-chan map[string]any, match func(map[string]any) bool) map[string]any {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case u := <-ch:
			if match(u) {
				return u
			}
		case <-timeout:
			t.Fatal("no matching update within 5s")
			return nil
		}
	}
}

func TestFindCompanyNotFoundIsAnswerNotError(t *testing.T) {
	nowhere := httptest.NewTLSServer(http.NotFoundHandler())
	defer nowhere.Close()
	domain := strings.TrimPrefix(nowhere.URL, "https://")
	h := newHarness(t, nowhere.Client())

	status, body, _ := h.call(t, "/tools/find_company", map[string]string{"domain": domain})
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200: not finding a company is an answer", status)
	}
	if body["found"] != false || body["reason"] != "no Poppy agent at "+domain {
		t.Errorf("body = %v, want found false and reason %q", body, "no Poppy agent at "+domain)
	}
}

func TestFindCompanyAnswersItsAgents(t *testing.T) {
	co := newCompany(t, "Wrenmoor Energy")
	h := newHarness(t, co.srv.Client())
	status, body, _ := h.call(t, "/tools/find_company", map[string]string{"domain": "https://" + co.domain + "/"})
	agents, _ := body["agents"].([]any)
	if status != 200 || body["name"] != "Wrenmoor Energy" || body["domain"] != co.domain || len(agents) != 1 {
		t.Fatalf("find_company = %d %v", status, body)
	}
	a, _ := agents[0].(map[string]any)
	if a["description"] != "Power cuts" || a["access"] != "session" {
		t.Errorf("agent = %v", a)
	}
	if _, ok := a["endpoint"]; ok {
		t.Errorf("agent entry exposes its endpoint: %v", a)
	}
}

func TestStartConversationStartsReaderAndPublishes(t *testing.T) {
	co := newCompany(t, "Wrenmoor Energy")
	co.setStream("cnv_1", msgEvent("e1", "There is an outage at HX7."))
	h := newHarness(t, co.srv.Client(), co.domain)
	ups := h.updates(t)

	status, body, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "Is there a power cut at HX7?"})
	if status != 200 || body["conversation_id"] != "cnv_1" || body["company"] != "Wrenmoor Energy" {
		t.Fatalf("start_conversation = %d %s", status, raw)
	}
	u := waitFor(t, ups, func(u map[string]any) bool {
		ev, _ := u["event"].(map[string]any)
		return ev["id"] == "e1"
	})
	if u["conversation_id"] != "cnv_1" || u["company"] != "Wrenmoor Energy" {
		t.Errorf("update = %v", u)
	}

	// read_conversation reads what the Reader stored.
	status, body, raw = h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_1"})
	msgs, _ := body["messages"].([]any)
	if status != 200 || len(msgs) != 1 || body["company"] != "Wrenmoor Energy" || body["closed"] != false || body["handoff"] != "none" {
		t.Fatalf("read_conversation = %d %s", status, raw)
	}
	m, _ := msgs[0].(map[string]any)
	if m["role"] != "company" || m["author"] != "agent" || m["text"] != "There is an outage at HX7." {
		t.Errorf("message = %v", m)
	}
}

func TestStartConversationRefusesUnknownDomain(t *testing.T) {
	co := newCompany(t, "Wrenmoor Energy")
	h := newHarness(t, co.srv.Client()) // not known, and not found yet
	status, body, _ := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "hi"})
	if status == 200 || body["error"] == nil {
		t.Errorf("start_conversation on an unknown domain = %d %v, want an error", status, body)
	}
	for _, r := range co.requests() {
		if strings.HasPrefix(r, "POST") {
			t.Errorf("the Company was contacted: %s", r)
		}
	}
	// Once found, it may be started.
	h.call(t, "/tools/find_company", map[string]string{"domain": co.domain})
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "hi"}); status != 200 {
		t.Errorf("start_conversation after find_company = %d %s", status, raw)
	}
}

func TestReadConversationReportsState(t *testing.T) {
	co := newCompany(t, "Larkmere Mutual")
	h := newHarness(t, co.srv.Client(), co.domain)
	conv := conversation.Conversation{ID: "cnv_9", Company: co.domain, Endpoint: co.srv.URL + "/poppy"}
	if err := h.store.SaveConversation(conv); err != nil {
		t.Fatal(err)
	}
	// A handoff value this version doesn't know reads as pending (§8.7).
	if err := h.store.SetState("cnv_9", true, "transferring", false); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SetStopped("cnv_9", "unauthorized (HTTP 401): sign in"); err != nil {
		t.Fatal(err)
	}
	status, body, raw := h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_9"})
	if status != 200 || body["handoff"] != "pending" || body["working"] != true || body["closed"] != false {
		t.Errorf("read_conversation = %d %s", status, raw)
	}
	if s, _ := body["stopped"].(string); !strings.Contains(s, "401") {
		t.Errorf("stopped = %v, want the reason the Reader stopped", body["stopped"])
	}
	if status, body, _ := h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_nope"}); status == 200 || body["error"] == nil {
		t.Errorf("read_conversation of an unknown id = %d %v, want an error", status, body)
	}
}

func TestIdentityDocsServedWithoutRedirect(t *testing.T) {
	h := newHarness(t, nil)
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, path := range []string{"/agent.json", "/jwks.json"} {
		resp, err := noRedirect.Get(h.http.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s = %d, want 200 with no redirect", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s Content-Type = %q", path, ct)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Errorf("%s is not JSON: %s", path, raw)
		}
		switch path {
		case "/agent.json":
			if doc["client_id"] != h.id.ClientID || doc["jwks_uri"] != h.id.JWKSURI {
				t.Errorf("agent.json = %s", raw)
			}
		case "/jwks.json":
			if !bytes.Equal(bytes.TrimSpace(raw), h.id.JWKS()) {
				t.Errorf("jwks.json = %s, want the identity's JWKS", raw)
			}
		}
	}
}

func TestToolsNeverExposeUserIDOrKeys(t *testing.T) {
	co := newCompany(t, "Wrenmoor Energy")
	co.setStream("cnv_1", msgEvent("e1", "There is an outage at HX7."), authEvent("e2", "claims"))
	h := newSignInHarness(t, co)
	ups := h.updates(t)

	var bodies []string
	add := func(_ int, _ map[string]any, raw string) { bodies = append(bodies, raw) }
	add(h.call(t, "/tools/find_company", map[string]string{"domain": co.domain}))
	add(h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "outage?"}))
	waitFor(t, ups, func(u map[string]any) bool { ev, _ := u["event"].(map[string]any); return ev["id"] == "e2" })
	status, _, raw := h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claims"})
	if status != 200 {
		t.Fatalf("sign_in = %d %s", status, raw)
	}
	bodies = append(bodies, raw)
	_, _, page := h.callback(t, h.iss.Approve(pendingURL(t, ups)))
	bodies = append(bodies, page)
	waitFor(t, ups, func(u map[string]any) bool { ev, _ := u["event"].(map[string]any); return ev["status"] == "signed_in" })
	add(h.call(t, "/tools/send_message", map[string]string{"conversation_id": "cnv_1", "text": "thanks"}))
	add(h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_1"}))
	add(h.call(t, "/api/direct", map[string]string{"conversation_id": "cnv_1", "text": "It's Ellen."}))
	add(h.call(t, "/tools/close_conversation", map[string]string{"conversation_id": "cnv_1"}))
	for _, path := range []string{"/agent.json", "/jwks.json", "/api/conversations"} {
		_, _, raw := h.get(t, path)
		bodies = append(bodies, raw)
	}
	bodies = append(bodies, drain(ups)...)

	userID, err := h.store.UserID(h.iss.URL)
	if err != nil {
		t.Fatal(err)
	}
	tok := h.tokens(t)
	if tok == nil {
		t.Fatal("no tokens stored after the sign-in")
	}
	if !contains(co.authorizations(), "POST /poppy/cnv_1/messages Bearer "+tok.Access) {
		t.Errorf("send_message after the sign-in carried no access token: %v", co.authorizations())
	}
	secrets := append([]string{userID, tok.Access, tok.Refresh}, h.exchangeSecrets()...)
	for _, body := range bodies {
		for _, sec := range secrets {
			if strings.Contains(body, sec) {
				t.Errorf("a response or update exposes a User ID, token, code or verifier %q: %s", sec, body)
			}
		}
		if jwtShape.MatchString(body) {
			t.Errorf("a response holds what looks like a JWT: %s", body)
		}
		var doc any
		_ = json.Unmarshal([]byte(body), &doc)
		for _, k := range privateMembers(doc) {
			t.Errorf("a response holds private JWK member %q: %s", k, body)
		}
	}
}

// jwtShape matches a compact JWS: a base64url JSON header ("eyJ"), a dot, a
// payload and a dot. A bare "eyJ" can turn up in random base64url, such as
// an authorization URL's state.
var jwtShape = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.`)

// privateMembers walks a JSON document for private JWK members (RFC 7518
// §6.2.2, §6.3.2) in any object that has a kty.
func privateMembers(v any) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		if _, ok := x["kty"]; ok {
			for _, k := range []string{"d", "p", "q", "dp", "dq", "qi", "oth", "k"} {
				if _, ok := x[k]; ok {
					out = append(out, k)
				}
			}
		}
		for _, e := range x {
			out = append(out, privateMembers(e)...)
		}
	case []any:
		for _, e := range x {
			out = append(out, privateMembers(e)...)
		}
	}
	return out
}

func TestDirectConversationIsEllensOnly(t *testing.T) {
	co := newCompany(t, "Larkmere Mutual")
	h := newHarness(t, co.srv.Client(), co.domain)
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "flood cover?"}); status != 200 {
		t.Fatalf("start = %d %s", status, raw)
	}
	status, body, raw := h.call(t, "/api/direct", map[string]string{"conversation_id": "cnv_1", "text": "Hi, it's Ellen."})
	if status != 200 || body["conversation_id"] != "cnv_2" {
		t.Fatalf("direct = %d %s", status, raw)
	}
	// A second line from Ellen goes to the same Direct Conversation.
	if status, body, raw := h.call(t, "/api/direct", map[string]string{"conversation_id": "cnv_1", "text": "Is the freezer covered?"}); status != 200 || body["conversation_id"] != "cnv_2" {
		t.Fatalf("second direct = %d %s", status, raw)
	}
	posts := co.postBodies()
	if len(posts) != 3 || posts[1]["parent_conversation_id"] != "cnv_1" {
		t.Fatalf("posts = %v", posts)
	}
	for _, p := range posts[1:] {
		msg, _ := p["message"].(map[string]any)
		author, _ := msg["author"].(map[string]any)
		if author["type"] != "human" {
			t.Errorf("Ellen's message sent as %v", author["type"])
		}
	}
	if paths := co.requests(); !contains(paths, "POST /poppy/cnv_2/messages") {
		t.Errorf("second line not sent on cnv_2: %v", paths)
	}
	// The assistant may not speak, or close, in Ellen's own conversation.
	if status, body, _ := h.call(t, "/tools/send_message", map[string]string{"conversation_id": "cnv_2", "text": "as Ellen"}); status == 200 || body["error"] == nil {
		t.Errorf("send_message on a Direct Conversation = %d %v, want refused", status, body)
	}
	if status, body, _ := h.call(t, "/tools/close_conversation", map[string]string{"conversation_id": "cnv_2"}); status == 200 || body["error"] == nil {
		t.Errorf("close_conversation on a Direct Conversation = %d %v, want refused", status, body)
	}
	if n := len(co.postBodies()); n != 3 {
		t.Errorf("%d POSTs, want 3: refused calls reached the Company", n)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestResumeRestartsOpenReaders(t *testing.T) {
	co := newCompany(t, "Wrenmoor Energy")
	co.setStream("cnv_open", msgEvent("e7", "Power is back."))
	id := testIdentity(t)
	st := conversation.NewMemoryStore(id, time.Now)
	ep := co.srv.URL + "/poppy"
	for _, c := range []conversation.Conversation{
		{ID: "cnv_open", Company: co.domain, Endpoint: ep},
		{ID: "cnv_closed", Company: co.domain, Endpoint: ep, Closed: true},
		{ID: "cnv_stopped", Company: co.domain, Endpoint: ep},
	} {
		if err := st.SaveConversation(c); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SetStopped("cnv_stopped", "unauthorized (HTTP 401)"); err != nil {
		t.Fatal(err)
	}
	h := newHarnessWith(t, co.srv.Client(), id, st)
	ups := h.updates(t)
	h.srv.resume()
	u := waitFor(t, ups, func(u map[string]any) bool { ev, _ := u["event"].(map[string]any); return ev["id"] == "e7" })
	if u["conversation_id"] != "cnv_open" || u["company"] != "Wrenmoor Energy" {
		t.Errorf("update = %v", u)
	}
	for _, r := range co.requests() {
		if strings.Contains(r, "cnv_closed") || strings.Contains(r, "cnv_stopped") {
			t.Errorf("a Reader was restarted for a finished conversation: %s", r)
		}
	}
}

// The page loads the conversations it missed before it follows updates: each
// conversation's state and messages, from the Store alone.
func TestConversationsSnapshot(t *testing.T) {
	h := newHarness(t, nil)
	for _, c := range []conversation.Conversation{
		{ID: "cnv_1", Company: "wrenmoor.example"},
		{ID: "cnv_2", Company: "wrenmoor.example", ParentID: "cnv_1"},
		{ID: "cnv_3", Company: "larkmere.example", Closed: true},
	} {
		if err := h.store.SaveConversation(c); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.store.AppendEvent("cnv_1", conversation.Event{ID: "e1", Type: "message", Role: "user", AuthorType: "agent", Text: "Is there a power cut at HX7?"}); err != nil {
		t.Fatal(err)
	}
	if err := h.store.AppendEvent("cnv_1", conversation.Event{ID: "e2", Type: "state", Raw: json.RawMessage(`{"type":"state","handoff":"pending"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SetWorking("cnv_1", true); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SetStopped("cnv_2", "unauthorized (HTTP 401)"); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(h.http.URL + "/api/conversations")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("conversations = %d %q %s", resp.StatusCode, resp.Header.Get("Content-Type"), raw)
	}
	var got []struct {
		ConversationID       string       `json:"conversation_id"`
		Company              string       `json:"company"`
		ParentConversationID string       `json:"parent_conversation_id"`
		Working              bool         `json:"working"`
		Handoff              string       `json:"handoff"`
		Closed               bool         `json:"closed"`
		Stopped              string       `json:"stopped"`
		Messages             []messageOut `json:"messages"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("conversations is not a JSON list: %s", raw)
	}
	if len(got) != 3 || got[0].ConversationID != "cnv_1" || got[1].ConversationID != "cnv_2" || got[2].ConversationID != "cnv_3" {
		t.Fatalf("conversations = %s", raw)
	}
	c1, c2, c3 := got[0], got[1], got[2]
	if c1.Company != "wrenmoor.example" || !c1.Working || c1.Handoff != "pending" || c1.Closed || c1.Stopped != "" ||
		len(c1.Messages) != 1 || c1.Messages[0] != (messageOut{Role: "user", Author: "agent", Text: "Is there a power cut at HX7?"}) {
		t.Errorf("cnv_1 = %+v", c1)
	}
	if c2.ParentConversationID != "cnv_1" || c2.Stopped == "" || c2.Messages == nil {
		t.Errorf("cnv_2 = %+v (messages must be a list, never null)", c2)
	}
	if !c3.Closed || c3.Working {
		t.Errorf("cnv_3 = %+v", c3)
	}
	if strings.Contains(string(raw), "eyJ") {
		t.Errorf("the snapshot holds what looks like a JWT: %s", raw)
	}
}

func TestSlowSubscriberDoesNotBlockPublish(t *testing.T) {
	var hb hub
	_, stop := hb.subscribe() // never read
	defer stop()
	fast, stopFast := hb.subscribe()
	defer stopFast()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 10 * subscriberBuffer {
			hb.publish([]byte(fmt.Sprint(i)))
			<-fast
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("publish blocked on a subscriber that never reads")
	}
}

// callResult is one tool call made off the test goroutine.
type callResult struct {
	status  int
	body    map[string]any
	raw     string
	elapsed time.Duration
	err     error
}

// callAsync posts body to path in the background; t.Fatal can't be called
// off the test goroutine, so failures come back in the result.
func (h *harness) callAsync(path string, body any) <-chan callResult {
	out := make(chan callResult, 1)
	go func() {
		start := time.Now()
		b, _ := json.Marshal(body)
		resp, err := http.Post(h.http.URL+path, "application/json", bytes.NewReader(b))
		if err != nil {
			out <- callResult{err: err}
			return
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		r := callResult{status: resp.StatusCode, raw: string(raw), elapsed: time.Since(start)}
		r.err = json.Unmarshal(raw, &r.body)
		out <- r
	}()
	return out
}

// subscribed waits until the hub has n subscribers: a waiting
// read_conversation subscribes before it reads, so after this nothing it
// should see can be missed.
func subscribed(t *testing.T, hb *hub, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		hb.mu.Lock()
		got := len(hb.subs)
		hb.mu.Unlock()
		if got >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d subscribers after 5s, want %d", got, n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// arrive stores an event as the conversation's Reader would, and publishes it.
func (h *harness) arrive(t *testing.T, conv conversation.Conversation, ev map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(ev)
	e := conversation.Event{Raw: raw}
	e.ID, _ = ev["id"].(string)
	e.Type, _ = ev["type"].(string)
	if m, ok := ev["message"].(map[string]any); ok {
		e.Role, _ = m["role"].(string)
		e.Text, _ = m["text"].(string)
		a, _ := m["author"].(map[string]any)
		e.AuthorType, _ = a["type"].(string)
	}
	if err := h.store.AppendEvent(conv.ID, e); err != nil {
		t.Fatal(err)
	}
	h.srv.publish(conv, e)
}

func TestReadConversationWaitReturnsReplyThatArrives(t *testing.T) {
	h := newHarness(t, nil)
	conv := conversation.Conversation{ID: "cnv_w", Company: "wrenmoor.example", Name: "Wrenmoor Energy"}
	other := conversation.Conversation{ID: "cnv_x", Company: "larkmere.example", Name: "Larkmere Mutual"}
	for _, c := range []conversation.Conversation{conv, other} {
		if err := h.store.SaveConversation(c); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.store.SetWorking("cnv_w", true); err != nil {
		t.Fatal(err)
	}
	res := h.callAsync("/tools/read_conversation", map[string]any{"conversation_id": "cnv_w", "wait_seconds": 15})
	subscribed(t, &h.srv.hub, 1)

	// Another conversation's reply, and our own side's message, don't end it.
	h.arrive(t, other, msgEvent("x1", "Your cover includes flood."))
	user := msgEvent("w0", "Is there a power cut at HX7 6QA?")
	user["message"].(map[string]any)["role"] = "user"
	h.arrive(t, conv, user)
	select {
	case r := <-res:
		t.Fatalf("read_conversation returned before the company replied: %d %s", r.status, r.raw)
	case <-time.After(200 * time.Millisecond):
	}

	h.arrive(t, conv, msgEvent("w1", "Yes, the power is off in HX7."))
	select {
	case r := <-res:
		if r.err != nil || r.status != 200 {
			t.Fatalf("read_conversation = %d %s (%v)", r.status, r.raw, r.err)
		}
		msgs, _ := r.body["messages"].([]any)
		if len(msgs) != 2 || r.body["company"] != "Wrenmoor Energy" || r.body["handoff"] != "none" || r.body["closed"] != false {
			t.Fatalf("read_conversation = %s", r.raw)
		}
		if m, _ := msgs[1].(map[string]any); m["role"] != "company" || m["text"] != "Yes, the power is off in HX7." {
			t.Errorf("reply = %v", msgs[1])
		}
		if r.elapsed > 5*time.Second {
			t.Errorf("returned after %v, want as soon as the reply arrived", r.elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read_conversation did not return when the reply arrived")
	}
}

// After send_message, the Store can still end with the company's previous
// reply until the stream echoes the new message. A waiting read must not
// take that old reply as the answer: it waits for a company message newer
// than the send. (Wrenmoor's compensation reply was missed this way: the read
// returned at once with "she needs to sign in", and the turn ended.)
func TestReadConversationWaitIgnoresTheReplyBeforeASend(t *testing.T) {
	h := newHarness(t, nil)
	conv := conversation.Conversation{ID: "cnv_w", Company: "wrenmoor.example", Name: "Wrenmoor Energy"}
	if err := h.store.SaveConversation(conv); err != nil {
		t.Fatal(err)
	}
	h.arrive(t, conv, msgEvent("w1", "She needs to sign in for compensation."))
	h.srv.markSent(conv)

	res := h.callAsync("/tools/read_conversation", map[string]any{"conversation_id": "cnv_w", "wait_seconds": 15})
	subscribed(t, &h.srv.hub, 1)
	select {
	case r := <-res:
		t.Fatalf("read_conversation answered with the reply from before the send: %d %s", r.status, r.raw)
	case <-time.After(200 * time.Millisecond):
	}
	user := msgEvent("w2", "She has signed in. Please check her compensation.")
	user["message"].(map[string]any)["role"] = "user"
	h.arrive(t, conv, user)
	h.arrive(t, conv, msgEvent("w3", "Ellen is owed £95."))
	select {
	case r := <-res:
		msgs, _ := r.body["messages"].([]any)
		if m, _ := msgs[len(msgs)-1].(map[string]any); m["text"] != "Ellen is owed £95." {
			t.Errorf("read_conversation = %s, want the new reply", r.raw)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read_conversation did not return when the new reply arrived")
	}
	// Once that reply is in, a read answers at once again.
	start := time.Now()
	if status, _, raw := h.call(t, "/tools/read_conversation", map[string]any{"conversation_id": "cnv_w", "wait_seconds": 15}); status != 200 || time.Since(start) > 2*time.Second {
		t.Errorf("read after the reply = %d %s in %v, want at once", status, raw, time.Since(start))
	}
}

func TestReadConversationWaitReturnsWhenWorkingEnds(t *testing.T) {
	h := newHarness(t, nil)
	conv := conversation.Conversation{ID: "cnv_w", Company: "wrenmoor.example", Name: "Wrenmoor Energy"}
	if err := h.store.SaveConversation(conv); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SetWorking("cnv_w", true); err != nil {
		t.Fatal(err)
	}
	res := h.callAsync("/tools/read_conversation", map[string]any{"conversation_id": "cnv_w", "wait_seconds": 15})
	subscribed(t, &h.srv.hub, 1)
	if err := h.store.SetWorking("cnv_w", false); err != nil {
		t.Fatal(err)
	}
	f := false
	h.srv.publish(conv, conversation.Event{Type: "working", Working: &f})
	select {
	case r := <-res:
		if r.status != 200 || r.body["working"] != false {
			t.Fatalf("read_conversation = %d %s", r.status, r.raw)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read_conversation did not return when working ended")
	}
}

func TestReadConversationWaitReturnsAtTimeout(t *testing.T) {
	h := newHarness(t, nil)
	conv := conversation.Conversation{ID: "cnv_w", Company: "wrenmoor.example", Name: "Wrenmoor Energy"}
	if err := h.store.SaveConversation(conv); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SetWorking("cnv_w", true); err != nil {
		t.Fatal(err)
	}
	r := <-h.callAsync("/tools/read_conversation", map[string]any{"conversation_id": "cnv_w", "wait_seconds": 1})
	if r.err != nil || r.status != 200 {
		t.Fatalf("read_conversation = %d %s (%v)", r.status, r.raw, r.err)
	}
	if r.elapsed < time.Second || r.elapsed > 4*time.Second {
		t.Errorf("returned after %v, want at the 1s timeout", r.elapsed)
	}
	msgs, ok := r.body["messages"].([]any)
	if !ok || len(msgs) != 0 || r.body["working"] != true || r.body["company"] != "Wrenmoor Energy" {
		t.Errorf("read_conversation = %s, want the same shape, still working", r.raw)
	}

	// No wait, or none asked for, answers at once.
	start := time.Now()
	if status, _, raw := h.call(t, "/tools/read_conversation", map[string]any{"conversation_id": "cnv_w"}); status != 200 || time.Since(start) > 500*time.Millisecond {
		t.Errorf("read_conversation with no wait = %d %s after %v", status, raw, time.Since(start))
	}
}

func TestFindCompanyTransientFailureSaysTryAgain(t *testing.T) {
	down := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	}))
	defer down.Close()
	gone := httptest.NewTLSServer(http.NotFoundHandler())
	goneDomain := strings.TrimPrefix(gone.URL, "https://")
	goneClient := gone.Client()
	gone.Close() // nothing listens there now: a network error

	for _, tc := range []struct {
		name   string
		hc     *http.Client
		domain string
	}{
		{"5xx", down.Client(), strings.TrimPrefix(down.URL, "https://")},
		{"network error", goneClient, goneDomain},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, tc.hc)
			status, body, raw := h.call(t, "/tools/find_company", map[string]string{"domain": tc.domain})
			want := "couldn't reach " + tc.domain + " right now; try again shortly"
			if status != http.StatusOK || body["found"] != false || body["reason"] != want {
				t.Errorf("find_company = %d %s, want found false and reason %q", status, raw, want)
			}
		})
	}
}

func TestFindCompanyDomainMismatchIsNotFound(t *testing.T) {
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSONTest(w, 200, map[string]any{
			"organization": map[string]any{"name": "Elsewhere", "domain": "elsewhere.example"},
			"agent":        map[string]any{"protocols": []any{map[string]any{"type": "poppy", "endpoint": "https://elsewhere.example/poppy"}}},
		})
	}))
	defer other.Close()
	h := newHarness(t, other.Client())
	addr := strings.TrimPrefix(other.URL, "https://")
	status, body, raw := h.call(t, "/tools/find_company", map[string]string{"domain": addr})
	if status != http.StatusOK || body["found"] != false || body["reason"] != "no Poppy agent at "+addr {
		t.Errorf("find_company = %d %s, want no Poppy agent", status, raw)
	}
}

// A conversation keeps its Company's name, so after a restart (when nothing
// has been discovered yet) a closed one is still named, not shown by domain.
func TestConversationKeepsCompanyName(t *testing.T) {
	co := newCompany(t, "Wrenmoor Energy")
	h := newHarness(t, co.srv.Client(), co.domain)
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "outage?"}); status != 200 {
		t.Fatalf("start = %d %s", status, raw)
	}
	convs, _ := h.store.Conversations()
	if len(convs) != 1 || convs[0].Name != "Wrenmoor Energy" {
		t.Fatalf("stored = %+v, want the Company's name", convs)
	}

	st := conversation.NewMemoryStore(h.id, time.Now)
	if err := st.SaveConversation(conversation.Conversation{ID: "cnv_old", Company: "larkmere.example", Name: "Larkmere Mutual", Closed: true}); err != nil {
		t.Fatal(err)
	}
	restarted := newHarnessWith(t, nil, h.id, st)
	status, body, raw := restarted.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_old"})
	if status != 200 || body["company"] != "Larkmere Mutual" {
		t.Errorf("read_conversation after a restart = %d %s, want company Larkmere Mutual", status, raw)
	}
}

// A Company that closed the conversation first answers close with
// conversation_closed; the conversation is then closed here too.
func TestCloseConversationAlreadyClosedIsRecorded(t *testing.T) {
	co := newCompany(t, "Wrenmoor Energy")
	h := newHarness(t, co.srv.Client(), co.domain)
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "outage?"}); status != 200 {
		t.Fatalf("start = %d %s", status, raw)
	}
	co.mu.Lock()
	co.closedAlready = true
	co.mu.Unlock()
	status, body, raw := h.call(t, "/tools/close_conversation", map[string]string{"conversation_id": "cnv_1"})
	if status != 200 || body["closed"] != true {
		t.Fatalf("close_conversation = %d %s", status, raw)
	}
	convs, _ := h.store.Conversations()
	if len(convs) != 1 || !convs[0].Closed {
		t.Errorf("stored = %+v, want Closed", convs)
	}
	if _, _, closed, _ := h.store.State("cnv_1"); !closed {
		t.Error("state not closed")
	}
	if status, _, raw := h.call(t, "/tools/send_message", map[string]string{"conversation_id": "cnv_1", "text": "hello?"}); status != http.StatusConflict {
		t.Errorf("send_message after close = %d %s, want 409", status, raw)
	}
}

// A reply already there when the wait starts is the answer: waiting for a
// further change would hold the stage for up to 20s.
func TestReadConversationWaitAnswersAtOnceWhenAlreadyReplied(t *testing.T) {
	h := newHarness(t, nil)
	conv := conversation.Conversation{ID: "cnv_w", Company: "wrenmoor.example", Name: "Wrenmoor Energy"}
	if err := h.store.SaveConversation(conv); err != nil {
		t.Fatal(err)
	}
	h.arrive(t, conv, msgEvent("w1", "Yes, the power is off in HX7."))
	r := <-h.callAsync("/tools/read_conversation", map[string]any{"conversation_id": "cnv_w", "wait_seconds": 20})
	if r.err != nil || r.status != 200 {
		t.Fatalf("read_conversation = %d %s (%v)", r.status, r.raw, r.err)
	}
	if r.elapsed > time.Second {
		t.Errorf("returned after %v, want at once: the company has already replied", r.elapsed)
	}
	if msgs, _ := r.body["messages"].([]any); len(msgs) != 1 {
		t.Errorf("read_conversation = %s, want the reply", r.raw)
	}
}

func authEvent(id, scope string) map[string]any {
	return map[string]any{
		"id": id, "type": "authorization", "created_at": "2026-11-23T10:01:00Z",
		"error": "insufficient_scope", "scope": scope,
	}
}

// get fetches path and returns the status, headers and body.
func (h *harness) get(t *testing.T, path string) (int, http.Header, string) {
	t.Helper()
	resp, err := http.Get(h.http.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(raw)
}

// callback opens the sign-in callback with the redirect's query q, as
// Ellen's browser does after the Company's sign-in page.
func (h *harness) callback(t *testing.T, q url.Values) (int, http.Header, string) {
	t.Helper()
	return h.get(t, "/signin/callback?"+q.Encode())
}

// tokens is what the agent holds for Ellen at the harness's issuer.
func (h *harness) tokens(t *testing.T) *signin.Tokens {
	t.Helper()
	userID, err := h.store.UserID(h.iss.URL)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := h.flow.Tokens.Get(h.iss.URL, userID)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// exchangeSecrets are the codes, verifiers and refresh tokens the agent sent
// the issuer's token endpoint.
func (h *harness) exchangeSecrets() []string {
	var out []string
	for _, r := range h.iss.Requests() {
		for _, k := range []string{"code", "code_verifier", "refresh_token"} {
			if v := r.Form.Get(k); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// drain returns the updates that arrive within 200ms, as JSON.
func drain(ch <-chan map[string]any) []string {
	var out []string
	for {
		select {
		case u := <-ch:
			b, _ := json.Marshal(u)
			out = append(out, string(b))
		case <-time.After(200 * time.Millisecond):
			return out
		}
	}
}

func isSignIn(status string) func(map[string]any) bool {
	return func(u map[string]any) bool {
		ev, _ := u["event"].(map[string]any)
		return ev["type"] == "signin" && ev["status"] == status
	}
}

func signInOf(t *testing.T, body map[string]any, raw string) map[string]any {
	t.Helper()
	si, ok := body["sign_in"].(map[string]any)
	if !ok {
		t.Fatalf("read_conversation has no sign_in: %s", raw)
	}
	return si
}

func TestAuthorizationEventNeedsSignIn(t *testing.T) {
	co := newCompany(t, "Larkmere Mutual")
	co.setStream("cnv_1", msgEvent("e1", "I can look at your claim once you sign in."), authEvent("e2", "claims"))
	h := newSignInHarness(t, co)
	ups := h.updates(t)
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "my claim?"}); status != 200 {
		t.Fatalf("start = %d %s", status, raw)
	}
	waitFor(t, ups, func(u map[string]any) bool { ev, _ := u["event"].(map[string]any); return ev["id"] == "e2" })
	status, body, raw := h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_1"})
	if status != 200 {
		t.Fatalf("read_conversation = %d %s", status, raw)
	}
	si := signInOf(t, body, raw)
	if si["needed"] != true || si["scopes"] != "claims" || si["status"] != "needed" {
		t.Errorf("sign_in = %v, want needed, scopes claims, status needed", si)
	}

	// A conversation with no authorization event, not stopped, needs none.
	plain := conversation.Conversation{ID: "cnv_p", Company: co.domain, Name: "Larkmere Mutual"}
	if err := h.store.SaveConversation(plain); err != nil {
		t.Fatal(err)
	}
	if _, body, raw := h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_p"}); body["sign_in"] != nil {
		t.Errorf("read_conversation = %s, want no sign_in", raw)
	}

	// A waiting read wakes when one arrives.
	if err := h.store.SetWorking("cnv_p", true); err != nil {
		t.Fatal(err)
	}
	res := h.callAsync("/tools/read_conversation", map[string]any{"conversation_id": "cnv_p", "wait_seconds": 15})
	subscribed(t, &h.srv.hub, 2)
	h.arrive(t, plain, authEvent("p1", "claims:read"))
	select {
	case r := <-res:
		si, _ := r.body["sign_in"].(map[string]any)
		if r.status != 200 || si["needed"] != true || si["scopes"] != "claims:read" {
			t.Errorf("read_conversation = %d %s, want sign_in needed for claims:read", r.status, r.raw)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read_conversation did not return when sign-in became needed")
	}
}

func TestSignInToolReturnsURLNeverToken(t *testing.T) {
	co := newCompany(t, "Larkmere Mutual")
	co.setStream("cnv_1", authEvent("e1", "claims"))
	h := newSignInHarness(t, co)
	ups := h.updates(t)
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "my claim?"}); status != 200 {
		t.Fatalf("start = %d %s", status, raw)
	}
	// A first sign-in, so there are tokens, a code and a verifier the next
	// answer must not hold.
	_, _, raw := h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claims"})
	if status, _, page := h.callback(t, h.iss.Approve(pendingURL(t, ups))); status != 200 {
		t.Fatalf("first sign-in: callback = %d %s (sign_in %s)", status, page, raw)
	}
	tok := h.tokens(t)
	if tok == nil {
		t.Fatal("no tokens after the first sign-in")
	}

	status, body, raw := h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claims:read"})
	if status != 200 {
		t.Fatalf("sign_in = %d %s", status, raw)
	}
	note, _ := body["note"].(string)
	if body["status"] != "pending" || body["company"] != "Larkmere Mutual" || body["scopes"] != "claims claims:read" ||
		!strings.Contains(note, "Sign in to Larkmere Mutual") {
		t.Errorf("sign_in = %s, want pending at Larkmere Mutual for claims and claims:read, with the note", raw)
	}
	for _, sec := range append([]string{tok.Access, tok.Refresh}, h.exchangeSecrets()...) {
		if strings.Contains(raw, sec) {
			t.Errorf("sign_in's answer holds a token, code or verifier %q: %s", sec, raw)
		}
	}
	// The page gets the URL, as the button's link.
	u := waitFor(t, ups, isSignIn("pending"))
	ev := u["event"].(map[string]any)
	if authURL, _ := ev["authorization_url"].(string); !strings.HasPrefix(authURL, h.iss.URL+"/authorize?") {
		t.Errorf("authorization_url = %q, want the issuer's authorization endpoint", authURL)
	}
	if u["conversation_id"] != "cnv_1" || ev["scope"] != "claims claims:read" {
		t.Errorf("update = %v", u)
	}
	_, body, raw = h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_1"})
	if si := signInOf(t, body, raw); si["status"] != "pending" {
		t.Errorf("sign_in = %v, want pending", si)
	}

	// An assistant with no sign-in set up says so.
	plain := newHarness(t, co.srv.Client(), co.domain)
	status, body, _ = plain.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claims"})
	if status != http.StatusBadRequest || body["error"] != "sign-in isn't set up on this assistant" {
		t.Errorf("sign_in without a Flow = %d %v", status, body)
	}
}

// pendingURL waits for the pending sign-in update the page gets and returns
// its authorization URL: the only place the URL, and the state in it, goes.
func pendingURL(t *testing.T, ups <-chan map[string]any) string {
	t.Helper()
	u := waitFor(t, ups, isSignIn("pending"))
	ev, _ := u["event"].(map[string]any)
	authURL, _ := ev["authorization_url"].(string)
	if authURL == "" {
		t.Fatalf("pending sign-in update has no authorization_url: %v", u)
	}
	return authURL
}

// sign_in's answer goes to the model, so into the assistant's session
// transcript. It must not hold the authorization URL or its live state: whoever
// reads the transcript could complete the sign-in as someone else's account.
// The page gets the URL over /api/updates instead.
func TestSignInToolAnswerHoldsNoURLOrState(t *testing.T) {
	co := newCompany(t, "Larkmere Mutual")
	co.setStream("cnv_1", authEvent("e1", "claims"))
	h := newSignInHarness(t, co)

	// With no conversation open, nothing can show the button: the note says
	// to start one first.
	status, body, raw := h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claims"})
	if status != 200 {
		t.Fatalf("sign_in = %d %s", status, raw)
	}
	if note, _ := body["note"].(string); !strings.Contains(note, "Start a conversation with Larkmere Mutual first") {
		t.Errorf("sign_in with no conversation = %s, want a note to start a conversation first", raw)
	}
	if strings.Contains(raw, "authorization_url") || strings.Contains(raw, "/authorize") {
		t.Errorf("sign_in with no conversation holds the authorization URL: %s", raw)
	}

	ups := h.updates(t)
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "my claim?"}); status != 200 {
		t.Fatalf("start = %d %s", status, raw)
	}
	status, body, raw = h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claims"})
	if status != 200 {
		t.Fatalf("sign_in = %d %s", status, raw)
	}
	authURL := pendingURL(t, ups)
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	state := u.Query().Get("state")
	if state == "" {
		t.Fatalf("authorization URL has no state: %s", authURL)
	}
	if strings.Contains(raw, state) || strings.Contains(raw, "authorization_url") || strings.Contains(raw, "/authorize") {
		t.Errorf("sign_in's answer holds the authorization URL or its state: %s", raw)
	}
	want := []string{"company", "note", "scopes", "status"}
	if got := slices.Sorted(maps.Keys(body)); !slices.Equal(got, want) {
		t.Errorf("sign_in answer keys = %v, want %v", got, want)
	}
}

// A Reader stopped by a 401 asking for sign-in runs again, signed in, once
// the sign-in completes. The 401's Bearer challenge may name the scopes
// needed (PAP §7), or none: then sign_in falls back to the scopes the
// Company describes in poppy.json.
func TestCallbackSignsInAndResumesStoppedReader(t *testing.T) {
	for _, tc := range []struct {
		name, challenge, scopes, asked string
	}{
		{"challenge names a scope", `Bearer scope="claims:read"`, "claims:read", "claims:read"},
		{"challenge names none", "", "", "claims"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			co := newCompany(t, "Wrenmoor Energy")
			co.requireToken, co.challenge = true, tc.challenge
			co.authScopes = map[string]string{"claims": "See your claims"}
			h := newSignInHarness(t, co)
			ups := h.updates(t)
			if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "my outage?"}); status != 200 {
				t.Fatalf("start = %d %s", status, raw)
			}
			u := waitFor(t, ups, func(u map[string]any) bool { ev, _ := u["event"].(map[string]any); return ev["type"] == "stopped" })
			if ev := u["event"].(map[string]any); !strings.HasPrefix(ev["reason"].(string), conversation.CodeSignInRequired) {
				t.Fatalf("stopped = %v, want sign_in_required", ev)
			}
			_, body, raw := h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_1"})
			si := signInOf(t, body, raw)
			if si["needed"] != true || si["status"] != "needed" || si["scopes"] != tc.scopes {
				t.Errorf("sign_in = %v, want needed for %q", si, tc.scopes)
			}

			// The assistant passes on sign_in.scopes as it reads them.
			_, body, raw = h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": si["scopes"].(string)})
			if body["scopes"] != tc.asked {
				t.Fatalf("sign_in = %s, want a sign-in for %q", raw, tc.asked)
			}
			authURL := pendingURL(t, ups)
			q := h.iss.Approve(authURL)
			status, hdr, page := h.callback(t, q)
			if status != 200 || !strings.Contains(page, "Signed in to Wrenmoor Energy. You can close this tab and go back to your assistant.") {
				t.Fatalf("callback = %d %s", status, page)
			}
			checkPageHeaders(t, hdr)
			tok := h.tokens(t)
			if tok == nil {
				t.Fatal("no tokens after the callback")
			}
			for _, sec := range append([]string{tok.Access, tok.Refresh, q.Get("state")}, h.exchangeSecrets()...) {
				if strings.Contains(page, sec) {
					t.Errorf("the callback page holds a token, code, state or verifier %q: %s", sec, page)
				}
			}
			waitFor(t, ups, isSignIn("signed_in"))
			h.waitSignedInRead(t, co, tok, "cnv_1")
		})
	}
}

// waitSignedInRead waits for the conversation's Reader to read again with
// the access token, and checks the conversation no longer reads as stopped.
func (h *harness) waitSignedInRead(t *testing.T, co *company, tok *signin.Tokens, convID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !contains(co.authorizations(), "GET /poppy/"+convID+"/events Bearer "+tok.Access) {
		if time.Now().After(deadline) {
			t.Fatalf("the Reader did not read again with the access token: %v", co.authorizations())
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, body, raw := h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": convID})
	if body["stopped"] != nil {
		t.Errorf("read_conversation = %s, want no longer stopped", raw)
	}
	if si := signInOf(t, body, raw); si["needed"] != false || si["status"] != "signed_in" {
		t.Errorf("sign_in = %v, want signed in and not needed", si)
	}
}

// A Reader whose signed-out request is answered 401 only after a sign-in
// completed (so the callback found nothing stopped) runs again, signed in.
func TestSignInRacingA401ResumesReader(t *testing.T) {
	co := newCompany(t, "Wrenmoor Energy")
	co.requireToken, co.gate = true, make(chan struct{})
	co.authScopes = map[string]string{"claims": "See your claims"}
	h := newSignInHarness(t, co)
	ups := h.updates(t)
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "my outage?"}); status != 200 {
		t.Fatalf("start = %d %s", status, raw)
	}
	// The Reader's first read is out, signed out, and held at the Company.
	deadline := time.Now().Add(5 * time.Second)
	for !contains(co.authorizations(), "GET /poppy/cnv_1/events ") {
		if time.Now().After(deadline) {
			t.Fatalf("the Reader never read: %v", co.requests())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if status, _, raw := h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claims"}); status != 200 {
		t.Fatalf("sign_in = %d %s", status, raw)
	}
	if status, _, page := h.callback(t, h.iss.Approve(pendingURL(t, ups))); status != 200 {
		t.Fatalf("callback = %d %s", status, page)
	}
	waitFor(t, ups, isSignIn("signed_in"))
	close(co.gate) // now the held read is refused, after the sign-in
	tok := h.tokens(t)
	if tok == nil {
		t.Fatal("no tokens after the callback")
	}
	h.waitSignedInRead(t, co, tok, "cnv_1")
}

// The scopes needed are the union of the authorization events of the latest
// turn: those since the latest role "user" message.
func TestAuthorizationEventsOfTheTurnAreUnited(t *testing.T) {
	co := newCompany(t, "Larkmere Mutual")
	h := newSignInHarness(t, co)
	conv := conversation.Conversation{ID: "cnv_t", Company: co.domain, Name: "Larkmere Mutual"}
	if err := h.store.SaveConversation(conv); err != nil {
		t.Fatal(err)
	}
	user := func(id, text string) map[string]any {
		m := msgEvent(id, text)
		m["message"].(map[string]any)["role"] = "user"
		return m
	}
	h.arrive(t, conv, user("u1", "my policy?"))
	h.arrive(t, conv, authEvent("a1", "policies:read"))
	h.arrive(t, conv, user("u2", "and my claim?"))
	h.arrive(t, conv, authEvent("a2", "claims"))
	h.arrive(t, conv, authEvent("a3", "claims:read claims"))
	h.arrive(t, conv, msgEvent("c1", "Please sign in so I can see your claim."))
	_, body, raw := h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_t"})
	if si := signInOf(t, body, raw); si["needed"] != true || si["scopes"] != "claims claims:read" {
		t.Errorf("sign_in = %v, want needed for claims and claims:read only", si)
	}
	// A new turn with no authorization event needs none.
	h.arrive(t, conv, user("u3", "thanks"))
	if _, body, raw := h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_t"}); body["sign_in"] != nil {
		t.Errorf("read_conversation = %s, want no sign_in after a turn that asked for none", raw)
	}
}

// sign_in given no scopes asks for those Ellen holds at the Company, else
// those she last asked for there, else those its poppy.json describes; with
// none known it says so.
func TestSignInFallbackScopes(t *testing.T) {
	co := newCompany(t, "Larkmere Mutual")
	h := newSignInHarness(t, co)
	status, body, raw := h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain})
	if status != http.StatusBadRequest || !strings.Contains(raw, "none is known") {
		t.Errorf("sign_in with no scopes known = %d %s, want 400", status, raw)
	}
	// With no conversation open, the note says no button is showing.
	_, body, raw = h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claims:read"})
	if note, _ := body["note"].(string); !strings.Contains(note, "No conversation with Larkmere Mutual is open") {
		t.Errorf("sign_in with no conversation = %s, want a note that no button is showing", raw)
	}
	// With one open, the page gets the button.
	ups := h.updates(t)
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "my claim?"}); status != 200 {
		t.Fatalf("start = %d %s", status, raw)
	}
	_, body, raw = h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain})
	if body["scopes"] != "claims:read" {
		t.Errorf("sign_in = %s, want the scopes last asked for", raw)
	}
	if status, _, page := h.callback(t, h.iss.Approve(pendingURL(t, ups))); status != 200 {
		t.Fatalf("callback = %d %s", status, page)
	}
	co.mu.Lock()
	co.authScopes = map[string]string{"claims": "See your claims"}
	co.mu.Unlock()
	_, body, raw = h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain})
	if body["scopes"] != "claims:read" {
		t.Errorf("sign_in = %s, want the scopes Ellen holds", raw)
	}
}

// The model guesses scope names ("claim.report" for a Company that offers
// "claims"). sign_in asks only for scopes the Company offers: an unoffered one
// is dropped, and when none is left it falls back as if none was given. The
// Company's consent page shows Ellen what is asked for either way.
func TestSignInDropsScopesTheCompanyDoesNotOffer(t *testing.T) {
	co := newCompany(t, "Larkmere Mutual")
	co.mu.Lock()
	co.authScopes = map[string]string{"claims": "See and report your claims"}
	co.mu.Unlock()
	h := newSignInHarness(t, co)
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "my claim?"}); status != 200 {
		t.Fatalf("start = %d %s", status, raw)
	}
	status, body, raw := h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claim.report"})
	if status != 200 || body["scopes"] != "claims" {
		t.Errorf("sign_in with only an unoffered scope = %d %s, want 200 asking for claims", status, raw)
	}
	status, body, raw = h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claims power_cut.details"})
	if status != 200 || body["scopes"] != "claims" {
		t.Errorf("sign_in with one offered, one not = %d %s, want 200 asking for claims", status, raw)
	}
}

func checkPageHeaders(t *testing.T, hdr http.Header) {
	t.Helper()
	if ct := hdr.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if cc := hdr.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if csp := hdr.Get("Content-Security-Policy"); csp != "default-src 'none'; style-src 'unsafe-inline'" {
		t.Errorf("Content-Security-Policy = %q", csp)
	}
}

func TestCallbackDeclineKeepsConversationOpen(t *testing.T) {
	co := newCompany(t, "Larkmere Mutual")
	co.setStream("cnv_1", authEvent("e1", "claims"))
	h := newSignInHarness(t, co)
	ups := h.updates(t)
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "my claim?"}); status != 200 {
		t.Fatalf("start = %d %s", status, raw)
	}
	waitFor(t, ups, func(u map[string]any) bool { ev, _ := u["event"].(map[string]any); return ev["id"] == "e1" })
	if status, _, raw := h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claims"}); status != 200 {
		t.Fatalf("sign_in = %d %s", status, raw)
	}
	status, hdr, page := h.callback(t, h.iss.Decline(pendingURL(t, ups)))
	if status != 200 || !strings.Contains(page, "You didn&#39;t sign in to Larkmere Mutual.") {
		t.Fatalf("callback = %d %s", status, page)
	}
	checkPageHeaders(t, hdr)
	waitFor(t, ups, isSignIn("declined"))
	_, body, raw := h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_1"})
	if body["closed"] != false || body["stopped"] != nil {
		t.Errorf("read_conversation = %s, want open", raw)
	}
	if si := signInOf(t, body, raw); si["needed"] != true || si["status"] != "declined" || si["scopes"] != "claims" {
		t.Errorf("sign_in = %v, want still needed, declined", si)
	}
	if h.tokens(t) != nil {
		t.Error("tokens stored after a decline")
	}
	for _, r := range co.requests() {
		if strings.HasSuffix(r, "/close") {
			t.Errorf("the conversation was closed: %s", r)
		}
	}
}

func TestCallbackUnknownStateIsPlainRefusal(t *testing.T) {
	co := newCompany(t, "Larkmere Mutual")
	co.setStream("cnv_1", authEvent("e1", "claims"))
	h := newSignInHarness(t, co)
	ups := h.updates(t)
	if status, _, raw := h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "my claim?"}); status != 200 {
		t.Fatalf("start = %d %s", status, raw)
	}
	waitFor(t, ups, func(u map[string]any) bool { ev, _ := u["event"].(map[string]any); return ev["id"] == "e1" })

	q := url.Values{"state": {"state-nobody-started"}, "code": {"code-from-nowhere"}, "iss": {h.iss.URL}}
	status, hdr, page := h.callback(t, q)
	if status != http.StatusBadRequest || !strings.Contains(page, "expired or has already been used") {
		t.Errorf("callback = %d %s, want 400 and the reason", status, page)
	}
	checkPageHeaders(t, hdr)
	for _, v := range []string{"state-nobody-started", "code-from-nowhere"} {
		if strings.Contains(page, v) {
			t.Errorf("the page echoes %q: %s", v, page)
		}
	}
	for _, u := range drain(ups) {
		if strings.Contains(u, `"signin"`) {
			t.Errorf("an update was published: %s", u)
		}
	}
	if n := len(h.iss.Requests()); n != 0 {
		t.Errorf("the issuer saw %d requests, want none", n)
	}

	// A redirect from another issuer is refused too, and the sign-in failed.
	if status, _, raw := h.call(t, "/tools/sign_in", map[string]string{"domain": co.domain, "scopes": "claims"}); status != 200 {
		t.Fatalf("sign_in = %d %s", status, raw)
	}
	q = h.iss.Approve(pendingURL(t, ups))
	q.Set("iss", "https://elsewhere.example")
	if status, _, page := h.callback(t, q); status != http.StatusBadRequest || !strings.Contains(page, "refused") {
		t.Errorf("callback from another issuer = %d %s", status, page)
	}
	waitFor(t, ups, isSignIn("failed"))
	if h.tokens(t) != nil {
		t.Error("tokens stored after a refused redirect")
	}
}

// With Redis, sign-in tokens are sealed, so the key is required; without, they
// stay in memory.
func TestTokenStoreNeedsKeyWithRedis(t *testing.T) {
	if _, err := tokenStore("redis://127.0.0.1:1", ""); err == nil || !strings.Contains(err.Error(), "--token-key-file") {
		t.Errorf("tokenStore with Redis and no key = %v, want an error naming --token-key-file", err)
	}
	key := filepath.Join(t.TempDir(), "token.key")
	if err := os.WriteFile(key, []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ts, err := tokenStore("redis://127.0.0.1:1", key); err != nil {
		t.Errorf("tokenStore with Redis and a key = %v", err)
	} else if _, ok := ts.(*signin.RedisTokenStore); !ok {
		t.Errorf("tokenStore = %T, want a RedisTokenStore", ts)
	}
	if ts, err := tokenStore("", ""); err != nil {
		t.Errorf("tokenStore without Redis = %v", err)
	} else if _, ok := ts.(*signin.MemoryTokenStore); !ok {
		t.Errorf("tokenStore = %T, want a MemoryTokenStore", ts)
	}
}
