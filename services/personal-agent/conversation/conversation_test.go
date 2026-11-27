package conversation

import (
	"context"
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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AltairaLabs/omnia-demo/services/personal-agent/discovery"
	"github.com/AltairaLabs/omnia-demo/services/personal-agent/identity"
	"github.com/AltairaLabs/omnia-demo/services/personal-agent/signin"
	"github.com/AltairaLabs/omnia-demo/services/personal-agent/signin/signintest"
)

// testIdentity generates the two identity keys in-process and loads them as
// the agent's identity.
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

// decodeJWT splits a compact JWT into header and claims without verifying
// it, for assertions on the wire format.
func decodeJWT(compact string) (map[string]any, map[string]any, error) {
	parts := strings.Split(compact, ".")
	if len(parts) != 3 {
		return nil, nil, fmt.Errorf("JWT has %d parts", len(parts))
	}
	var out [2]map[string]any
	for i := range 2 {
		raw, err := base64.RawURLEncoding.DecodeString(parts[i])
		if err != nil {
			return nil, nil, err
		}
		if err := json.Unmarshal(raw, &out[i]); err != nil {
			return nil, nil, err
		}
	}
	return out[0], out[1], nil
}

// recorded is one request the fake Company saw.
type recorded struct {
	Method, Path, LastEventID, Accept string
	Authorization                     string
	Query                             url.Values
	Claims                            map[string]any
	Body                              map[string]any
}

// fakeCompany is an httptest TLS Company that checks every request's
// Session header shape and records it before handing it to handle.
type fakeCompany struct {
	t      *testing.T
	srv    *httptest.Server
	co     *discovery.Company
	handle func(w http.ResponseWriter, r *http.Request, n int)

	mu   sync.Mutex
	reqs []recorded
	auds map[string]bool // accepted aud values: the Company identifiers in use
}

func newFakeCompany(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, n int)) *fakeCompany {
	t.Helper()
	f := &fakeCompany{t: t, handle: handle}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	host := strings.TrimPrefix(f.srv.URL, "https://")
	f.co = &discovery.Company{
		Name:       "Wrenmoor Energy",
		Domain:     host,
		Identifier: "https://auth.wrenmoor.example.test",
		Issuer:     "https://auth.wrenmoor.example.test",
		Agents:     []discovery.AgentEntry{{Endpoint: f.srv.URL + "/poppy", Description: "support", Access: "session"}},
	}
	f.auds = map[string]bool{f.co.Identifier: true}
	return f
}

func (f *fakeCompany) serve(w http.ResponseWriter, r *http.Request) {
	rec := recorded{
		Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(),
		LastEventID: r.Header.Get("Last-Event-ID"), Accept: r.Header.Get("Accept"),
		Authorization: r.Header.Get("Authorization"),
	}
	if v := r.Header.Get("Poppy-Version"); v != "1.0" {
		f.t.Errorf("Poppy-Version = %q, want 1.0", v)
	}
	hdr, claims, err := decodeJWT(r.Header.Get("Poppy-Session"))
	if err != nil {
		f.t.Errorf("Poppy-Session: %v", err)
	} else {
		if hdr["typ"] != "poppy-session+jwt" || hdr["alg"] != "ES256" || hdr["poppy_dlg"] == nil {
			f.t.Errorf("Session JWT header = %v", hdr)
		}
		f.mu.Lock()
		okAud := f.auds[fmt.Sprint(claims["aud"])]
		f.mu.Unlock()
		if !okAud {
			f.t.Errorf("aud = %v, want the Company identifier", claims["aud"])
		}
		if claims["htm"] != r.Method {
			f.t.Errorf("htm = %v, want %s", claims["htm"], r.Method)
		}
		if want := "https://" + r.Host + r.URL.Path; claims["htu"] != want {
			f.t.Errorf("htu = %v, want %s", claims["htu"], want)
		}
		for _, k := range []string{"sub", "sid", "jti", "iss"} {
			if s, _ := claims[k].(string); s == "" {
				f.t.Errorf("claim %s missing", k)
			}
		}
		rec.Claims = claims
	}
	if r.Method == http.MethodPost {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &rec.Body); err != nil {
			f.t.Errorf("POST body is not JSON: %s", raw)
		}
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, rec)
	n := len(f.reqs)
	f.mu.Unlock()
	f.handle(w, r, n)
}

func (f *fakeCompany) requests() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.reqs...)
}

func newTestClient(t *testing.T, f *fakeCompany) (*Client, Store) {
	t.Helper()
	st := NewMemoryStore(testIdentity(t), time.Now)
	c := NewClient(f.srv.Client(), st)
	c.Backoff = time.Millisecond
	return c, st
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func messageAuthor(t *testing.T, body map[string]any) string {
	t.Helper()
	msg, _ := body["message"].(map[string]any)
	author, _ := msg["author"].(map[string]any)
	s, _ := author["type"].(string)
	return s
}

func TestStartSendsSignedSession(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		writeJSON(w, http.StatusCreated, map[string]string{"conversation_id": fmt.Sprintf("cnv_%d", n)})
	})
	c, st := newTestClient(t, f)
	ctx := context.Background()
	agent := f.co.Agents[0]

	c1, err := c.Start(ctx, f.co, agent, "Is there an outage at HX7?")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := c.Start(ctx, f.co, agent, "Another question")
	if err != nil {
		t.Fatal(err)
	}
	if c1.ID != "cnv_1" || c2.ID != "cnv_2" || c1.Company != f.co.Domain || c1.Endpoint != agent.Endpoint {
		t.Fatalf("conversations = %+v, %+v", c1, c2)
	}
	reqs := f.requests()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want 2", len(reqs))
	}
	for _, r := range reqs {
		if r.Method != http.MethodPost || r.Path != "/poppy" {
			t.Errorf("request %s %s, want POST /poppy", r.Method, r.Path)
		}
		if messageAuthor(t, r.Body) != "agent" {
			t.Errorf("author.type = %q, want agent", messageAuthor(t, r.Body))
		}
		if id, _ := r.Body["id"].(string); id == "" {
			t.Errorf("body has no request id: %v", r.Body)
		}
	}
	if reqs[0].Claims["sub"] != reqs[1].Claims["sub"] {
		t.Errorf("sub changed between starts: %v, %v", reqs[0].Claims["sub"], reqs[1].Claims["sub"])
	}
	if reqs[0].Body["id"] == reqs[1].Body["id"] {
		t.Errorf("two starts reused request id %v", reqs[0].Body["id"])
	}
	uid, _ := st.UserID(f.co.Identifier)
	if reqs[0].Claims["sub"] != uid {
		t.Errorf("sub = %v, want the stored User ID %s", reqs[0].Claims["sub"], uid)
	}
	convs, _ := st.Conversations()
	if len(convs) != 2 {
		t.Errorf("stored %d conversations, want 2", len(convs))
	}
}

func TestRetryResignsSession(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if n == 1 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily_unavailable"})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"conversation_id": "cnv_1"})
	})
	c, _ := newTestClient(t, f)
	if _, err := c.Start(context.Background(), f.co, f.co.Agents[0], "hello"); err != nil {
		t.Fatal(err)
	}
	reqs := f.requests()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want 2", len(reqs))
	}
	if reqs[0].Claims["jti"] == reqs[1].Claims["jti"] {
		t.Errorf("retry reused jti %v", reqs[0].Claims["jti"])
	}
	if reqs[0].Body["id"] != reqs[1].Body["id"] {
		t.Errorf("retry changed the request id: %v, %v", reqs[0].Body["id"], reqs[1].Body["id"])
	}
}

func TestSessionEndedStartsNewSession(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if n == 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_ended"})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"conversation_id": "cnv_1"})
	})
	c, _ := newTestClient(t, f)
	if _, err := c.Start(context.Background(), f.co, f.co.Agents[0], "hello"); err != nil {
		t.Fatal(err)
	}
	reqs := f.requests()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want 2", len(reqs))
	}
	if reqs[0].Claims["sid"] == reqs[1].Claims["sid"] {
		t.Error("session_ended did not start a new Session")
	}
	if reqs[0].Claims["sub"] != reqs[1].Claims["sub"] {
		t.Error("a new Session changed the User ID")
	}
}

func TestRetryNeverOn4xx(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "conversation_closed"})
	})
	c, _ := newTestClient(t, f)
	c.Remember(f.co)
	err := c.Send(context.Background(), Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}, "hi")
	if err == nil {
		t.Fatal("Send succeeded, want conversation_closed")
	}
	if !IsCode(err, "conversation_closed") {
		t.Errorf("error = %v, want conversation_closed", err)
	}
	if n := len(f.requests()); n != 1 {
		t.Errorf("%d requests, want 1 (no retry on 4xx)", n)
	}
}

func TestDirectConversationBody(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		writeJSON(w, http.StatusCreated, map[string]string{"conversation_id": fmt.Sprintf("cnv_%d", n)})
	})
	c, _ := newTestClient(t, f)
	ctx := context.Background()
	parent, err := c.Start(ctx, f.co, f.co.Agents[0], "outage?")
	if err != nil {
		t.Fatal(err)
	}
	direct, err := c.OpenDirect(ctx, parent, "Hi, it's Ellen. When will power be back?")
	if err != nil {
		t.Fatal(err)
	}
	if direct.ParentID != parent.ID || direct.ID == parent.ID || direct.Endpoint != parent.Endpoint {
		t.Errorf("direct = %+v, parent = %+v", direct, parent)
	}
	r := f.requests()[1]
	if r.Path != "/poppy" || r.Body["parent_conversation_id"] != parent.ID {
		t.Errorf("direct start %s body %v", r.Path, r.Body)
	}
	if got := messageAuthor(t, r.Body); got != "human" {
		t.Errorf("author.type = %q, want human", got)
	}
}

func TestSendAndClose(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		writeJSON(w, http.StatusAccepted, map[string]string{"conversation_id": "cnv_1"})
	})
	c, _ := newTestClient(t, f)
	ctx := context.Background()
	conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
	c.Remember(f.co)
	if err := c.Send(ctx, conv, "more"); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(ctx, conv); err != nil {
		t.Fatal(err)
	}
	reqs := f.requests()
	if reqs[0].Path != "/poppy/cnv_1/messages" || messageAuthor(t, reqs[0].Body) != "agent" {
		t.Errorf("send = %s %v", reqs[0].Path, reqs[0].Body)
	}
	if reqs[1].Path != "/poppy/cnv_1/close" || reqs[1].Body["id"] == nil {
		t.Errorf("close = %s %v", reqs[1].Path, reqs[1].Body)
	}
}

func TestReadOnceFollowsHasMoreAndSavesCursor(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		cur := r.URL.Query().Get("cursor")
		switch cur {
		case "":
			writeJSON(w, 200, map[string]any{"conversation_id": "cnv_1", "events": []any{msgEvent("e1", "hi")}, "cursor": "e1", "has_more": true, "working": true, "handoff": "none", "responder": "agent", "closed": false})
		case "e1":
			writeJSON(w, 200, map[string]any{"conversation_id": "cnv_1", "events": []any{msgEvent("e2", "there")}, "cursor": "e2", "has_more": false, "working": true, "handoff": "pending", "responder": "agent", "closed": false})
		default:
			t.Errorf("unexpected cursor %q", cur)
		}
	})
	c, st := newTestClient(t, f)
	c.Remember(f.co)
	conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
	evs, err := c.ReadOnce(context.Background(), conv, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[0].ID != "e1" || evs[1].Text != "there" || evs[1].Role != "company" || evs[1].AuthorType != "agent" {
		t.Fatalf("events = %+v", evs)
	}
	if f.requests()[0].Query.Get("wait") != "5" {
		t.Errorf("wait = %q, want 5", f.requests()[0].Query.Get("wait"))
	}
	if cur, _ := st.Cursor("cnv_1"); cur != "e2" {
		t.Errorf("cursor = %q, want e2", cur)
	}
	// The last read's state fields are the state once has_more is false (§8.5).
	if working, handoff, closed, err := st.State("cnv_1"); err != nil || !working || handoff != "pending" || closed {
		t.Errorf("State = %v %q %v %v, want true pending false", working, handoff, closed, err)
	}
}

// msgEvent is a company message event as §8.5 shows it.
func msgEvent(id, text string) map[string]any {
	return map[string]any{
		"id": id, "type": "message", "created_at": "2026-11-23T10:00:00Z",
		"message": map[string]any{"id": "m_" + id, "role": "company", "author": map[string]any{"type": "agent"}, "text": text},
	}
}

func stateEvent(id string, closed bool) map[string]any {
	return map[string]any{"id": id, "type": "state", "created_at": "2026-11-23T10:00:00Z", "handoff": "none", "responder": "agent", "closed": closed}
}

// sse writes conversation events as server-sent events (§8.6).
func sse(w http.ResponseWriter, events ...map[string]any) {
	for _, e := range events {
		b, _ := json.Marshal(e)
		fmt.Fprintf(w, "id: %s\ndata: %s\n\n", e["id"], b)
	}
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}
}

func startStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": ping\n\nevent: working\ndata: {\"working\":true}\n\n")
}

// runReader runs a Reader until it returns or the test times out, and
// collects what it publishes.
func runReader(t *testing.T, c *Client, conv Conversation) []Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	var got []Event
	r := &Reader{Client: c, Backoff: time.Millisecond}
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Run(ctx, conv, func(_ Conversation, e Event) {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, e)
		})
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Reader.Run did not return")
	}
	mu.Lock()
	defer mu.Unlock()
	var out []Event
	for _, e := range got {
		if e.ID != "" { // conversation events only, not working notices
			out = append(out, e)
		}
	}
	return out
}

func ids(evs []Event) string {
	var s []string
	for _, e := range evs {
		s = append(s, e.ID)
	}
	return strings.Join(s, ",")
}

func TestReaderResumesAfterDrop(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		startStream(w)
		switch n {
		case 1:
			sse(w, msgEvent("e1", "one"), msgEvent("e2", "two"))
		default:
			if got := r.Header.Get("Last-Event-ID"); got != "e2" {
				t.Errorf("reconnect Last-Event-ID = %q, want e2", got)
			}
			sse(w, msgEvent("e3", "three"), stateEvent("e4", true))
		}
	})
	c, st := newTestClient(t, f)
	c.Remember(f.co)
	conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
	got := runReader(t, c, conv)
	if ids(got) != "e1,e2,e3,e4" {
		t.Errorf("published %s, want e1,e2,e3,e4", ids(got))
	}
	if n := len(f.requests()); n != 2 {
		t.Errorf("%d connections, want 2", n)
	}
	stored, _ := st.Events("cnv_1")
	if ids(stored) != "e1,e2,e3,e4" {
		t.Errorf("stored %s", ids(stored))
	}
	if len(stored) < 3 || !strings.Contains(string(stored[2].Raw), `"three"`) {
		t.Errorf("stored[2].Raw is not the third event's JSON: %+v", stored)
	}
}

func TestReaderCursorExpired(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if r.Header.Get("Last-Event-ID") == "e2" || r.URL.Query().Get("cursor") == "e2" {
			writeJSON(w, http.StatusGone, map[string]string{"error": "cursor_expired"})
			return
		}
		startStream(w)
		sse(w, msgEvent("e1", "one"), msgEvent("e2", "two"), msgEvent("e3", "three"), stateEvent("e4", true))
	})
	c, st := newTestClient(t, f)
	c.Remember(f.co)
	conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
	for _, e := range []map[string]any{msgEvent("e1", "one"), msgEvent("e2", "two")} {
		raw, _ := json.Marshal(e)
		ev, _ := parseEvent(raw)
		if err := st.AppendEvent("cnv_1", ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SetCursor("cnv_1", "e2"); err != nil {
		t.Fatal(err)
	}
	got := runReader(t, c, conv)
	if ids(got) != "e3,e4" {
		t.Errorf("published %s, want e3,e4", ids(got))
	}
	stored, _ := st.Events("cnv_1")
	if ids(stored) != "e1,e2,e3,e4" {
		t.Errorf("stored %s, want e1,e2,e3,e4 with no duplicates", ids(stored))
	}
}

func TestReaderStopsWhenClosed(t *testing.T) {
	t.Run("state event", func(t *testing.T) {
		f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
			startStream(w)
			sse(w, msgEvent("e1", "bye"), stateEvent("e2", true))
			<-r.Context().Done() // hold the stream open: the Reader must leave by itself
		})
		c, st := newTestClient(t, f)
		c.Remember(f.co)
		conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
		if err := st.SaveConversation(conv); err != nil {
			t.Fatal(err)
		}
		if got := runReader(t, c, conv); ids(got) != "e1,e2" {
			t.Errorf("published %s", ids(got))
		}
		convs, _ := st.Conversations()
		if len(convs) != 1 || !convs[0].Closed {
			t.Errorf("stored conversation = %+v, want Closed", convs)
		}
	})
	t.Run("conversation_closed", func(t *testing.T) {
		f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "conversation_closed"})
		})
		c, _ := newTestClient(t, f)
		c.Remember(f.co)
		conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
		runReader(t, c, conv)
		if n := len(f.requests()); n != 1 {
			t.Errorf("%d requests, want 1", n)
		}
	})
}

func TestUserIDsArePerCompanyAndRandom(t *testing.T) {
	st := NewMemoryStore(nil, time.Now)
	a, err := st.UserID("https://auth.larkmere.example.test")
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.UserID("https://auth.wrenmoor.example.test")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := st.UserID("https://auth.larkmere.example.test")
	if a == b || a != again {
		t.Errorf("ids %q %q %q: want one stable ID per company", a, b, again)
	}
	for _, id := range []string{a, b} {
		if strings.Contains(strings.ToLower(id), "ellen") {
			t.Errorf("User ID %q contains a name", id)
		}
		raw, err := base64.RawURLEncoding.DecodeString(id)
		if err != nil || len(raw) != 16 {
			t.Errorf("User ID %q is not 128 random bits, base64url", id)
		}
	}
}

func TestDelegationRenewsWithinAnHourOfExpiry(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	st := NewMemoryStore(testIdentity(t), clock)
	d1, err := st.Delegation("u1")
	if err != nil {
		t.Fatal(err)
	}
	if d2, _ := st.Delegation("u1"); d2 != d1 {
		t.Error("a fresh delegation was not reused")
	}
	if d3, _ := st.Delegation("u2"); d3 == d1 || d3.KID == d1.KID {
		t.Error("two User IDs share a delegated key")
	}
	now = d1.Expiry.Add(-59 * time.Minute)
	d4, err := st.Delegation("u1")
	if err != nil {
		t.Fatal(err)
	}
	if d4 == d1 || !d4.Expiry.After(d1.Expiry) {
		t.Error("delegation within 1h of expiry was not renewed")
	}
}

func TestSendInDirectConversationIsHuman(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		writeJSON(w, http.StatusAccepted, map[string]string{"conversation_id": "cnv_2"})
	})
	c, _ := newTestClient(t, f)
	c.Remember(f.co)
	ctx := context.Background()
	agentConv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
	direct := Conversation{ID: "cnv_2", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint, ParentID: "cnv_1"}
	if err := c.Send(ctx, agentConv, "from the assistant"); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(ctx, direct, "from Ellen"); err != nil {
		t.Fatal(err)
	}
	reqs := f.requests()
	if got := messageAuthor(t, reqs[0].Body); got != "agent" {
		t.Errorf("author.type in a conversation = %q, want agent", got)
	}
	if got := messageAuthor(t, reqs[1].Body); got != "human" || reqs[1].Path != "/poppy/cnv_2/messages" {
		t.Errorf("author.type in a Direct Conversation = %q (%s), want human", got, reqs[1].Path)
	}
}

func TestUserIDsAreScopedByIdentifier(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		writeJSON(w, http.StatusCreated, map[string]string{"conversation_id": fmt.Sprintf("cnv_%d", n)})
	})
	c, _ := newTestClient(t, f)
	ctx := context.Background()
	// The same domain, first with no issuer, then naming one (§3.2, §5.1).
	before := *f.co
	before.Identifier = "https://" + f.co.Domain
	f.mu.Lock()
	f.auds[before.Identifier] = true
	f.mu.Unlock()
	if _, err := c.Start(ctx, &before, before.Agents[0], "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Start(ctx, f.co, f.co.Agents[0], "two"); err != nil {
		t.Fatal(err)
	}
	reqs := f.requests()
	if reqs[0].Claims["aud"] != before.Identifier || reqs[1].Claims["aud"] != f.co.Identifier {
		t.Fatalf("aud = %v, %v", reqs[0].Claims["aud"], reqs[1].Claims["aud"])
	}
	if reqs[0].Claims["sub"] == reqs[1].Claims["sub"] {
		t.Error("two identifiers for one domain share a User ID")
	}
}

func TestStateDefaults(t *testing.T) {
	st := NewMemoryStore(nil, time.Now)
	working, handoff, closed, err := st.State("cnv_unknown")
	if err != nil || working || handoff != "none" || closed {
		t.Errorf("State = %v %q %v %v, want false none false (§8.7)", working, handoff, closed, err)
	}
}

func TestAppendEventSkipsKnownID(t *testing.T) {
	st := NewMemoryStore(nil, time.Now)
	for _, e := range []map[string]any{msgEvent("e1", "one"), msgEvent("e2", "two"), msgEvent("e1", "one again")} {
		raw, _ := json.Marshal(e)
		ev, _ := parseEvent(raw)
		if err := st.AppendEvent("cnv_1", ev); err != nil {
			t.Fatal(err)
		}
	}
	stored, _ := st.Events("cnv_1")
	if ids(stored) != "e1,e2" || stored[0].Text != "one" {
		t.Errorf("stored %s (%+v), want e1,e2 with the first e1 kept", ids(stored), stored)
	}
	// The same ID in another conversation is a different event.
	raw, _ := json.Marshal(msgEvent("e1", "elsewhere"))
	ev, _ := parseEvent(raw)
	if err := st.AppendEvent("cnv_2", ev); err != nil {
		t.Fatal(err)
	}
	if other, _ := st.Events("cnv_2"); ids(other) != "e1" {
		t.Errorf("cnv_2 stored %s, want e1", ids(other))
	}
}

// TestRedisStoreEventsAndState runs only with PA_TEST_REDIS_URL set, such
// as redis://localhost:6379/15, and flushes that database.
func TestRedisStoreEventsAndState(t *testing.T) {
	u := os.Getenv("PA_TEST_REDIS_URL")
	if u == "" {
		t.Skip("PA_TEST_REDIS_URL not set")
	}
	st, err := NewRedisStore(u, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.rdb.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	pending := stateEvent("e2", false)
	pending["handoff"] = "pending"
	for _, e := range []map[string]any{msgEvent("e1", "one"), pending, msgEvent("e1", "one again")} {
		raw, _ := json.Marshal(e)
		ev, _ := parseEvent(raw)
		if err := st.AppendEvent("cnv_1", ev); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := st.Events("cnv_1")
	if err != nil || ids(stored) != "e1,e2" || stored[0].Text != "one" {
		t.Errorf("stored %s, %v; want e1,e2 with the first e1 kept", ids(stored), err)
	}
	if w, h, c, err := st.State("cnv_1"); err != nil || w || h != "pending" || c {
		t.Errorf("State = %v %q %v %v, want false pending false", w, h, c, err)
	}
	if r, err := st.Stopped("cnv_1"); r != "" || err != nil {
		t.Errorf("Stopped = %q, %v; want empty", r, err)
	}
	if err := st.SetStopped("cnv_1", "unauthorized (HTTP 401)"); err != nil {
		t.Fatal(err)
	}
	if r, _ := st.Stopped("cnv_1"); r != "unauthorized (HTTP 401)" {
		t.Errorf("Stopped = %q", r)
	}
	if _, h, _, _ := st.State("cnv_1"); h != "pending" {
		t.Errorf("SetStopped changed handoff to %q", h)
	}
}

// The poppy facade closes every stream after five minutes. A stream that
// ends cleanly is not a failure, so the Reader must not back off further
// each time, or replies arrive later and later.
func TestReaderReconnectsAtBaseBackoffAfterCleanEnd(t *testing.T) {
	var mu sync.Mutex
	streams := 0
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if r.Header.Get("Accept") != "text/event-stream" {
			// The check for an expired conversation after an empty stream.
			writeJSON(w, 200, map[string]any{"conversation_id": "cnv_1", "events": []any{}, "has_more": false, "working": false, "handoff": "none", "closed": false})
			return
		}
		mu.Lock()
		streams++
		s := streams
		mu.Unlock()
		startStream(w)
		if s == 4 {
			sse(w, stateEvent("e1", true))
		}
		// Otherwise return: the stream ends cleanly with no event.
	})
	c, _ := newTestClient(t, f)
	c.Remember(f.co)
	conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
	var waits []time.Duration
	r := &Reader{Client: c, Backoff: 10 * time.Millisecond}
	r.sleep = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		return ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r.Run(ctx, conv, func(Conversation, Event) {})
	if len(waits) != 3 {
		t.Fatalf("waited %v, want three waits", waits)
	}
	for _, d := range waits {
		if d != 10*time.Millisecond {
			t.Errorf("waits = %v, want each the base 10ms after a clean end", waits)
			break
		}
	}
}

// cursor_expired on a read that sent no cursor can't be cured by dropping
// the cursor, so the Reader backs off instead of asking again at once.
func TestReaderCursorExpiredWithoutCursorBacksOff(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		writeJSON(w, http.StatusGone, map[string]string{"error": "cursor_expired"})
	})
	c, _ := newTestClient(t, f)
	c.Remember(f.co)
	conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var waits []time.Duration
	r := &Reader{Client: c, Backoff: 10 * time.Millisecond}
	r.sleep = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		if len(waits) == 3 {
			cancel()
		}
		return ctx.Err()
	}
	r.Run(ctx, conv, func(Conversation, Event) {})
	if len(waits) != 3 {
		t.Fatalf("waited %v, want the Reader to back off between reads", waits)
	}
	if n := len(f.requests()); n != 3 {
		t.Errorf("%d requests, want 3: one per wait", n)
	}
}

// A Reader that stops because the Company refused it for good records why,
// so read_conversation can tell the model.
func TestReaderRecordsTerminalStop(t *testing.T) {
	for _, tc := range []struct {
		name, code, want string
		status           int
	}{
		// Any 401 but invalid_token on a token sent asks for sign-in.
		{"sign-in needed", "unauthorized", "sign_in_required (HTTP 401)", http.StatusUnauthorized},
		{"version unsupported", "version_unsupported", "version_unsupported", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
				writeJSON(w, tc.status, map[string]string{"error": tc.code, "error_description": "refused for the test"})
			})
			c, st := newTestClient(t, f)
			c.Remember(f.co)
			conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
			if err := st.SaveConversation(conv); err != nil {
				t.Fatal(err)
			}
			runReader(t, c, conv)
			reason, err := st.Stopped("cnv_1")
			if err != nil || !strings.Contains(reason, tc.want) {
				t.Errorf("Stopped = %q, %v; want a reason naming %s", reason, err, tc.want)
			}
			if _, _, closed, _ := st.State("cnv_1"); closed {
				t.Error("a stopped conversation was marked closed")
			}
		})
	}
	st := NewMemoryStore(nil, time.Now)
	if reason, err := st.Stopped("cnv_unknown"); reason != "" || err != nil {
		t.Errorf("Stopped of an unknown conversation = %q, %v; want empty", reason, err)
	}
}

func TestStateFollowsStream(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		startStream(w) // working: true
		pending := stateEvent("e2", false)
		pending["handoff"] = "pending"
		sse(w, msgEvent("e1", "looking for someone"), pending)
		fmt.Fprint(w, "event: working\ndata: {\"working\":false}\n\n")
		sse(w, stateEvent("e3", true))
	})
	c, st := newTestClient(t, f)
	c.Remember(f.co)
	conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}

	type snap struct {
		working bool
		handoff string
		closed  bool
	}
	var mu sync.Mutex
	var at []snap // the stored state as each event is published
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	(&Reader{Client: c, Backoff: time.Millisecond}).Run(ctx, conv, func(_ Conversation, e Event) {
		w, h, cl, _ := st.State(conv.ID)
		mu.Lock()
		at = append(at, snap{w, h, cl})
		mu.Unlock()
	})
	// Published: working(true), e1, e2 (pending), working(false), e3 (closed).
	want := []snap{{true, "none", false}, {true, "none", false}, {true, "pending", false}, {false, "pending", false}, {false, "none", true}}
	if fmt.Sprint(at) != fmt.Sprint(want) {
		t.Errorf("state as published = %v, want %v", at, want)
	}
}

// setIdentifier makes issuer f's Company's issuer and identifier.
func (f *fakeCompany) setIdentifier(issuer string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.co.Issuer, f.co.Identifier = issuer, issuer
	f.auds[issuer] = true
}

// signedIn is a Client whose TokenSource is a real signin.Flow against a
// fake issuer for f's Company.
type signedIn struct {
	c      *Client
	st     Store
	iss    *signintest.Issuer
	tokens *signin.MemoryTokenStore
}

func newSignedInClient(t *testing.T, f *fakeCompany) *signedIn {
	t.Helper()
	id := signintest.NewIdentity(t)
	st := NewMemoryStore(id, time.Now)
	c := NewClient(f.srv.Client(), st)
	c.Backoff = time.Millisecond
	iss := signintest.NewIssuer(t, id, f.co.Domain)
	f.setIdentifier(iss.URL)
	ts := signin.NewMemoryTokenStore()
	c.Tokens = signin.NewFlow(iss.Client(), id, ts)
	return &signedIn{c: c, st: st, iss: iss, tokens: ts}
}

// put stores tokens for the User at co, expiring in d.
func (s *signedIn) put(t *testing.T, co *discovery.Company, access, refresh string, d time.Duration) {
	t.Helper()
	uid, err := s.st.UserID(co.Identifier)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.tokens.Put(co.Identifier, uid, signin.Tokens{Access: access, Refresh: refresh, Scopes: []string{"policies:read"}, Expiry: time.Now().Add(d)}); err != nil {
		t.Fatal(err)
	}
}

func athOf(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func created(w http.ResponseWriter, _ *http.Request, n int) {
	writeJSON(w, http.StatusCreated, map[string]string{"conversation_id": fmt.Sprintf("cnv_%d", n)})
}

func TestSignedInRequestCarriesBearerAndAth(t *testing.T) {
	f := newFakeCompany(t, created)
	s := newSignedInClient(t, f)
	s.put(t, f.co, "t1", "r1", time.Hour)
	if _, err := s.c.Start(context.Background(), f.co, f.co.Agents[0], "Has my claim been paid?"); err != nil {
		t.Fatal(err)
	}
	r := f.requests()[0]
	if r.Authorization != "Bearer t1" {
		t.Errorf("Authorization = %q, want Bearer t1", r.Authorization)
	}
	if r.Claims["ath"] != athOf("t1") {
		t.Errorf("ath = %v, want the hash of t1", r.Claims["ath"])
	}
}

func TestSignedOutRequestHasNoAth(t *testing.T) {
	f := newFakeCompany(t, created)
	c, _ := newTestClient(t, f)
	if _, err := c.Start(context.Background(), f.co, f.co.Agents[0], "Is there an outage?"); err != nil {
		t.Fatal(err)
	}
	r := f.requests()[0]
	if r.Authorization != "" {
		t.Errorf("Authorization = %q, want none", r.Authorization)
	}
	if _, ok := r.Claims["ath"]; ok {
		t.Errorf("ath = %v, want none", r.Claims["ath"])
	}
}

func TestInvalidTokenRefreshesOnceAndRetries(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if n == 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="larkmere", error="invalid_token", error_description="expired"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		created(w, r, n)
	})
	s := newSignedInClient(t, f)
	s.put(t, f.co, "t1", "r1", time.Hour)
	s.iss.Next(http.StatusOK, map[string]any{"access_token": "t2", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "r2"})
	if _, err := s.c.Start(context.Background(), f.co, f.co.Agents[0], "hello"); err != nil {
		t.Fatal(err)
	}
	reqs := f.requests()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want 2", len(reqs))
	}
	if reqs[0].Claims["jti"] == reqs[1].Claims["jti"] {
		t.Error("the retry reused the Session JWT's jti")
	}
	if reqs[0].Body["id"] != reqs[1].Body["id"] {
		t.Error("the retry changed the request id")
	}
	if reqs[1].Authorization != "Bearer t2" || reqs[1].Claims["ath"] != athOf("t2") {
		t.Errorf("retry Authorization %q, ath %v; want t2 and its hash", reqs[1].Authorization, reqs[1].Claims["ath"])
	}

	// Refused again after the refresh, it gives up: no third send.
	g := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		w.WriteHeader(http.StatusUnauthorized)
	})
	s = newSignedInClient(t, g)
	s.put(t, g.co, "t1", "r1", time.Hour)
	_, err := s.c.Start(context.Background(), g.co, g.co.Agents[0], "hello")
	var cerr *Error
	if !errors.As(err, &cerr) || cerr.Status != http.StatusUnauthorized || cerr.Code != CodeSignInRequired {
		t.Errorf("Start error = %v, want sign_in_required (HTTP 401)", err)
	}
	if n := len(g.requests()); n != 2 {
		t.Errorf("%d requests, want 2: one refresh, one resend", n)
	}
}

func TestInvalidGrantDropsTokensAndStopsSignedOut(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("request %d carried a token after invalid_grant", n)
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="larkmere"`)
		w.WriteHeader(http.StatusUnauthorized)
	})
	s := newSignedInClient(t, f)
	s.c.Remember(f.co)
	s.put(t, f.co, "t1", "r1", 30*time.Second)
	s.iss.Next(http.StatusBadRequest, map[string]any{"error": "invalid_grant"})
	conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
	if err := s.st.SaveConversation(conv); err != nil {
		t.Fatal(err)
	}
	runReader(t, s.c, conv)
	uid, _ := s.st.UserID(f.co.Identifier)
	if tok, _ := s.tokens.Get(f.co.Identifier, uid); tok != nil {
		t.Errorf("tokens still stored after invalid_grant: %v", tok)
	}
	if reason, _ := s.st.Stopped("cnv_1"); reason != "sign_in_required (HTTP 401)" {
		t.Errorf("Stopped = %q, want sign_in_required (HTTP 401)", reason)
	}
	if n := len(f.requests()); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestTokenNeverSentToAnotherCompany(t *testing.T) {
	a := newFakeCompany(t, created)
	b := newFakeCompany(t, created)
	s := newSignedInClient(t, a)
	s.put(t, a.co, "token-for-a", "r1", time.Hour)
	ctx := context.Background()
	if _, err := s.c.Start(ctx, b.co, b.co.Agents[0], "hello b"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.c.Start(ctx, a.co, a.co.Agents[0], "hello a"); err != nil {
		t.Fatal(err)
	}
	if r := b.requests()[0]; r.Authorization != "" || r.Claims["ath"] != nil {
		t.Errorf("Company B saw Authorization %q, ath %v; want neither", r.Authorization, r.Claims["ath"])
	}
	if r := a.requests()[0]; r.Authorization != "Bearer token-for-a" {
		t.Errorf("Company A saw Authorization %q, want its own token", r.Authorization)
	}
}

func TestTokenWithheldFromEndpointWithOwnResource(t *testing.T) {
	f := newFakeCompany(t, created)
	s := newSignedInClient(t, f)
	s.put(t, f.co, "t1", "r1", time.Hour)
	f.co.Agents[0].Resource = f.co.Agents[0].Endpoint
	ctx := context.Background()
	conv, err := s.c.Start(ctx, f.co, f.co.Agents[0], "hello")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.c.Send(ctx, conv, "more"); err != nil {
		t.Fatal(err)
	}
	for _, r := range f.requests() {
		if r.Authorization != "" || r.Claims["ath"] != nil {
			t.Errorf("%s %s carried Authorization %q, ath %v; want neither", r.Method, r.Path, r.Authorization, r.Claims["ath"])
		}
	}
}

func TestRefreshRefusedStopsSignedOut(t *testing.T) {
	for _, c := range []struct {
		name      string
		expiresIn time.Duration
		challenge string
	}{
		{"expired token", -time.Second, `Bearer realm="larkmere"`},
		{"after invalid_token", time.Hour, `Bearer error="invalid_token"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
				w.Header().Set("WWW-Authenticate", c.challenge)
				w.WriteHeader(http.StatusUnauthorized)
			})
			s := newSignedInClient(t, f)
			s.c.Remember(f.co)
			s.put(t, f.co, "t1", "r1", c.expiresIn)
			s.iss.Next(http.StatusBadRequest, map[string]any{"error": "unauthorized_client"})
			conv := Conversation{ID: "cnv_1", Company: f.co.Domain, Endpoint: f.co.Agents[0].Endpoint}
			if err := s.st.SaveConversation(conv); err != nil {
				t.Fatal(err)
			}
			runReader(t, s.c, conv)
			if reason, _ := s.st.Stopped("cnv_1"); reason != "sign_in_required (HTTP 401)" {
				t.Errorf("Stopped = %q, want sign_in_required (HTTP 401)", reason)
			}
			if n := len(s.iss.Requests()); n != 1 {
				t.Errorf("issuer saw %d requests, want 1", n)
			}
			if n := len(f.requests()); n > 2 {
				t.Errorf("Company saw %d requests, want at most 2", n)
			}
			if last := f.requests()[len(f.requests())-1]; last.Authorization != "" {
				t.Errorf("the last request carried %q, want it signed out", last.Authorization)
			}
		})
	}
}

// rotatingTokens answers t1 to the first AccessToken and t2 after, as if
// another request refreshed in between, and counts Refresh calls.
type rotatingTokens struct {
	mu        sync.Mutex
	calls     int
	refreshes int
}

func (r *rotatingTokens) AccessToken(context.Context, *discovery.Company, string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.calls == 1 {
		return "t1", nil
	}
	return "t2", nil
}

func (r *rotatingTokens) Refresh(context.Context, *discovery.Company, string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refreshes++
	return "t3", nil
}

func TestInvalidTokenAfterAnotherRefreshResendsWithoutRefreshing(t *testing.T) {
	f := newFakeCompany(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if n == 1 {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		created(w, r, n)
	})
	c, _ := newTestClient(t, f)
	ts := &rotatingTokens{}
	c.Tokens = ts
	if _, err := c.Start(context.Background(), f.co, f.co.Agents[0], "hello"); err != nil {
		t.Fatal(err)
	}
	reqs := f.requests()
	if len(reqs) != 2 || reqs[1].Authorization != "Bearer t2" {
		t.Errorf("%d requests, the second with %q; want 2, the second with t2", len(reqs), reqs[len(reqs)-1].Authorization)
	}
	if ts.refreshes != 0 {
		t.Errorf("Refresh called %d times, want 0: another request already refreshed", ts.refreshes)
	}
}

func TestMayCarryTokenWithheldWhenAnyMatchHasOwnResource(t *testing.T) {
	co := &discovery.Company{Domain: "larkmere.example", Agents: []discovery.AgentEntry{
		{Endpoint: "https://larkmere.example/poppy"},
		{Endpoint: "https://larkmere.example/poppy/claims", Resource: "https://larkmere.example/poppy/claims"},
		{Endpoint: "https://larkmere.example/home", Resource: "https://larkmere.example"},
	}}
	for u, want := range map[string]bool{
		"https://larkmere.example/poppy":                    true,
		"https://larkmere.example/poppy/cnv_1/messages":     true,
		"https://larkmere.example/poppy/claims":             false,
		"https://larkmere.example/poppy/claims/cnv_1/close": false,
		"https://larkmere.example/home/cnv_1":               true,
		"https://larkmere.example/other":                    false,
		"https://evil.example/poppy":                        false,
	} {
		if got := mayCarryToken(co, u); got != want {
			t.Errorf("mayCarryToken(%s) = %v, want %v", u, got, want)
		}
	}
}

// A sign_in_required stop reason carries the scopes the 401's Bearer
// challenge named, and StopScopes reads them back.
func TestStopReasonKeepsChallengeScope(t *testing.T) {
	h := http.Header{"Www-Authenticate": {`Bearer realm="x", scope="claims  claims:read"`}}
	e := &Error{Status: http.StatusUnauthorized, Code: CodeSignInRequired, Scope: strings.Join(strings.Fields(bearerParam(h, bearerScopeParam)), " ")}
	reason := strings.TrimPrefix(e.Error(), "conversation: ")
	if got := strings.Join(StopScopes(reason), " "); got != "claims claims:read" {
		t.Errorf("StopScopes(%q) = %q", reason, got)
	}
	for _, r := range []string{"sign_in_required (HTTP 401)", `conversation_not_found (HTTP 404) scope="x"`, ""} {
		if got := StopScopes(r); got != nil {
			t.Errorf("StopScopes(%q) = %v, want none", r, got)
		}
	}
	if got := bearerParam(http.Header{"Www-Authenticate": {"Bearer scope=claims"}}, bearerScopeParam); got != "claims" {
		t.Errorf("unquoted scope = %q", got)
	}
}
