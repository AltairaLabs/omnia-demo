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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/conversation"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/identity"
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
}

func newCompany(t *testing.T, name string) *company {
	t.Helper()
	c := &company{stream: map[string][]map[string]any{}}
	c.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.paths = append(c.paths, r.Method+" "+r.URL.Path)
		c.mu.Unlock()
		switch {
		case r.URL.Path == "/.well-known/poppy.json":
			writeJSONTest(w, 200, map[string]any{
				"organization": map[string]any{"name": name, "domain": c.domain},
				"agent": map[string]any{"protocols": []any{map[string]any{
					"type": "poppy", "endpoint": c.srv.URL + "/poppy", "description": "Power cuts", "access": "session",
				}}},
			})
		case r.URL.Path == "/.well-known/oauth-protected-resource":
			http.NotFound(w, r)
		case r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			c.mu.Lock()
			c.posts = append(c.posts, body)
			c.n++
			id := fmt.Sprintf("cnv_%d", c.n)
			c.mu.Unlock()
			if strings.HasSuffix(r.URL.Path, "/messages") || strings.HasSuffix(r.URL.Path, "/close") {
				id = strings.Split(strings.TrimPrefix(r.URL.Path, "/poppy/"), "/")[0]
				writeJSONTest(w, http.StatusAccepted, map[string]string{"conversation_id": id})
				return
			}
			writeJSONTest(w, http.StatusCreated, map[string]string{"conversation_id": id})
		case strings.HasSuffix(r.URL.Path, "/events"):
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
}

func newHarness(t *testing.T, hc *http.Client, known ...string) *harness {
	t.Helper()
	id := testIdentity(t)
	st := conversation.NewMemoryStore(id, time.Now)
	return newHarnessWith(t, hc, id, st, known...)
}

func newHarnessWith(t *testing.T, hc *http.Client, id *identity.Identity, st *conversation.MemoryStore, known ...string) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := newServer(ctx, config{
		Identity: id, Store: st, HTTP: hc, KnownCompanies: known,
		ReaderBackoff: 10 * time.Millisecond,
	})
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
	co.setStream("cnv_1", msgEvent("e1", "There is an outage at HX7."))
	h := newHarness(t, co.srv.Client(), co.domain)
	ups := h.updates(t)

	var bodies []string
	add := func(_ int, _ map[string]any, raw string) { bodies = append(bodies, raw) }
	add(h.call(t, "/tools/find_company", map[string]string{"domain": co.domain}))
	add(h.call(t, "/tools/start_conversation", map[string]string{"domain": co.domain, "message": "outage?"}))
	u := waitFor(t, ups, func(u map[string]any) bool { ev, _ := u["event"].(map[string]any); return ev["id"] == "e1" })
	b, _ := json.Marshal(u)
	bodies = append(bodies, string(b))
	add(h.call(t, "/tools/send_message", map[string]string{"conversation_id": "cnv_1", "text": "thanks"}))
	add(h.call(t, "/tools/read_conversation", map[string]string{"conversation_id": "cnv_1"}))
	add(h.call(t, "/api/direct", map[string]string{"conversation_id": "cnv_1", "text": "It's Ellen."}))
	add(h.call(t, "/tools/close_conversation", map[string]string{"conversation_id": "cnv_1"}))
	for _, path := range []string{"/agent.json", "/jwks.json"} {
		resp, err := http.Get(h.http.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		bodies = append(bodies, string(raw))
	}

	userID, err := h.store.UserID("https://" + co.domain)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range bodies {
		if strings.Contains(body, userID) {
			t.Errorf("a response exposes the User ID: %s", body)
		}
		if strings.Contains(body, "eyJ") {
			t.Errorf("a response holds what looks like a JWT: %s", body)
		}
		var doc any
		_ = json.Unmarshal([]byte(body), &doc)
		for _, k := range privateMembers(doc) {
			t.Errorf("a response holds private JWK member %q: %s", k, body)
		}
	}
}

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
