package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/conversation"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/discovery"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/identity"
)

const (
	// subscriberBuffer is how many updates a page may fall behind by before
	// it misses some. A page that stops reading never holds up a Reader.
	subscriberBuffer = 64
	maxRequestBody   = 1 << 20
	companyTimeout   = 30 * time.Second
	updatesPing      = 25 * time.Second
)

// config is what the server is built from; main fills it from flags.
type config struct {
	Identity *identity.Identity
	Store    conversation.Store
	// HTTP reaches the Companies: discovery and conversations.
	HTTP *http.Client
	// KnownCompanies are the domains Ellen has accounts at. Conversations
	// may be started with these, and with any domain find_company found.
	KnownCompanies []string
	// TestIdentityDir, when set, is served at /poppy-test/ (the PAP
	// validator's test identity, Task 13 only).
	TestIdentityDir string
	// ReaderBackoff is each Reader's base reconnect wait; 0 is the Reader's
	// own default.
	ReaderBackoff time.Duration
}

// server is the Personal Agent's HTTP surface: its public identity
// documents, the tools Ellen's assistant calls, and the page's updates
// stream and take-over route.
type server struct {
	ctx     context.Context // the server's lifetime; Readers run until it ends
	id      *identity.Identity
	store   conversation.Store
	client  *conversation.Client
	testDir string
	backoff time.Duration
	hub     hub

	mu      sync.Mutex
	allowed map[string]bool   // domains start_conversation may use
	names   map[string]string // Company name by domain
	readers map[string]bool   // conversations with a running Reader
	wg      sync.WaitGroup

	directMu sync.Mutex // one Direct Conversation per parent
}

func newServer(ctx context.Context, cfg config) *server {
	s := &server{
		ctx:     ctx,
		id:      cfg.Identity,
		store:   cfg.Store,
		client:  conversation.NewClient(cfg.HTTP, cfg.Store),
		testDir: cfg.TestIdentityDir,
		backoff: cfg.ReaderBackoff,
		allowed: map[string]bool{},
		names:   map[string]string{},
		readers: map[string]bool{},
	}
	for _, d := range cfg.KnownCompanies {
		if d, ok := normalizeDomain(d); ok {
			s.allowed[d] = true
		}
	}
	// Learn each Company's name whenever it is discovered, including the
	// rediscovery a Reader does after a restart, so updates can name it.
	discover := s.client.Discover
	s.client.Discover = func(ctx context.Context, domain string) (*discovery.Company, error) {
		co, err := discover(ctx, domain)
		if err == nil {
			s.mu.Lock()
			s.names[co.Domain] = co.Name
			s.mu.Unlock()
		}
		return co, err
	}
	return s
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	// Public (PAP §4.1): exact paths, answered directly, never redirected.
	mux.HandleFunc("GET /agent.json", s.agentJSON)
	mux.HandleFunc("GET /jwks.json", s.jwksJSON)
	if s.testDir != "" {
		mux.HandleFunc("GET /poppy-test/{file}", s.testIdentity)
	}
	// Ellen's assistant's tools. The model sees only what these answer:
	// never a User ID, a key or a Session JWT.
	mux.HandleFunc("POST /tools/find_company", s.tool("find_company", s.findCompany))
	mux.HandleFunc("POST /tools/start_conversation", s.tool("start_conversation", s.startConversation))
	mux.HandleFunc("POST /tools/send_message", s.tool("send_message", s.sendMessage))
	mux.HandleFunc("POST /tools/read_conversation", s.tool("read_conversation", s.readConversation))
	mux.HandleFunc("POST /tools/close_conversation", s.tool("close_conversation", s.closeConversation))
	// The page. Not tools: only Ellen takes a conversation over.
	mux.HandleFunc("POST /api/direct", s.tool("direct", s.direct))
	mux.HandleFunc("GET /api/updates", s.updates)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

// wait returns once every Reader has stopped, after the server's context
// has ended.
func (s *server) wait() { s.wg.Wait() }

// resume restarts a Reader for each stored conversation that is neither
// closed nor stopped, as after a restart. Each Reader rediscovers its
// Company by domain.
func (s *server) resume() {
	convs, err := s.store.Conversations()
	if err != nil {
		log.Printf("resume: %v", err)
		return
	}
	for _, c := range convs {
		if c.Closed {
			continue
		}
		if _, _, closed, err := s.store.State(c.ID); err != nil || closed {
			continue
		}
		if reason, err := s.store.Stopped(c.ID); err != nil || reason != "" {
			continue
		}
		log.Printf("resume: following conversation=%s company=%s", c.ID, c.Company)
		s.follow(c)
	}
}

// follow runs a Reader for conv until the server stops or the conversation
// ends, unless one already runs. Its lifetime is the server's, not the
// request's that started it.
func (s *server) follow(conv conversation.Conversation) {
	s.mu.Lock()
	if s.readers[conv.ID] {
		s.mu.Unlock()
		return
	}
	s.readers[conv.ID] = true
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.readers, conv.ID)
			s.mu.Unlock()
		}()
		r := &conversation.Reader{Client: s.client, Backoff: s.backoff}
		r.Run(s.ctx, conv, s.publish)
		if reason, _ := s.store.Stopped(conv.ID); reason != "" {
			log.Printf("reader stopped: conversation=%s reason=%s", conv.ID, reason)
			b, _ := json.Marshal(map[string]string{"type": "stopped", "reason": reason})
			s.publishRaw(conv, b)
		}
	}()
}

// update is one line of /api/updates.
type update struct {
	ConversationID string `json:"conversation_id"`
	// ParentConversationID is set on a Direct Conversation's updates.
	ParentConversationID string          `json:"parent_conversation_id,omitempty"`
	Company              string          `json:"company"`
	Event                json.RawMessage `json:"event"`
}

// publish sends a Reader's event to every connected page: the event's JSON
// as the Company sent it (§8.5), or {"type":"working","working":b} for a
// stream's working notice (§8.6), which has none of its own.
func (s *server) publish(conv conversation.Conversation, e conversation.Event) {
	raw := e.Raw
	if e.ID == "" && e.Type == "working" && e.Working != nil {
		raw, _ = json.Marshal(map[string]any{"type": "working", "working": *e.Working})
	}
	s.publishRaw(conv, raw)
}

func (s *server) publishRaw(conv conversation.Conversation, raw json.RawMessage) {
	if len(raw) == 0 || !json.Valid(raw) {
		return
	}
	// json.Marshal compacts the RawMessage, so the update is one line.
	b, err := json.Marshal(update{
		ConversationID: conv.ID, ParentConversationID: conv.ParentID,
		Company: s.name(conv.Company), Event: raw,
	})
	if err != nil {
		return
	}
	s.hub.publish(b)
}

func (s *server) name(domain string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := s.names[domain]; n != "" {
		return n
	}
	return domain
}

func (s *server) agentJSON(w http.ResponseWriter, _ *http.Request) {
	b, err := json.Marshal(s.id.Metadata())
	if err != nil {
		http.Error(w, "metadata", http.StatusInternalServerError)
		return
	}
	writeDoc(w, b)
}

func (s *server) jwksJSON(w http.ResponseWriter, _ *http.Request) {
	writeDoc(w, s.id.JWKS())
}

func writeDoc(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(b)
}

// testIdentity serves a file from --test-identity-dir. {file} is one path
// segment and fs.ValidPath refuses "..", so nothing outside it is served.
func (s *server) testIdentity(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !fs.ValidPath(name) || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	b, err := fs.ReadFile(os.DirFS(s.testDir), name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(name, ".json") {
		w.Header().Set("Content-Type", "application/json")
	}
	_, _ = w.Write(b)
}

// toolRequest is the union of the tools' fields.
type toolRequest struct {
	Domain         string `json:"domain"`
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id"`
	Text           string `json:"text"`
}

// toolError is a refusal or failure the model can read and act on.
type toolError struct {
	status int
	msg    string
}

func (e *toolError) Error() string { return e.msg }

func fail(status int, format string, a ...any) *toolError {
	return &toolError{status: status, msg: fmt.Sprintf(format, a...)}
}

// tool wraps a tool with JSON decoding and encoding and a log line. A
// failure answers {"error": "..."} with its status.
func (s *server) tool(name string, fn func(context.Context, toolRequest) (any, *toolError)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req toolRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the request body is not JSON"})
			return
		}
		log.Printf("tool request: tool=%s domain=%s conversation=%s", name, req.Domain, req.ConversationID)
		out, terr := fn(r.Context(), req)
		if terr != nil {
			log.Printf("tool response: tool=%s status=%d error=%s", name, terr.status, terr.msg)
			writeJSON(w, terr.status, map[string]string{"error": terr.msg})
			return
		}
		log.Printf("tool response: tool=%s status=200", name)
		writeJSON(w, http.StatusOK, out)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode: %v", err)
	}
}

// companyError turns a failed call to a Company into a tool error that says
// what the Company said.
func companyError(domain string, err error) *toolError {
	var cerr *conversation.Error
	if errors.As(err, &cerr) {
		return fail(http.StatusBadGateway, "%s refused: %s", domain, strings.TrimPrefix(cerr.Error(), "conversation: "))
	}
	return fail(http.StatusBadGateway, "could not reach %s: %v", domain, err)
}

// normalizeDomain accepts host[:port], or an https URL with no path, and
// returns the host[:port].
func normalizeDomain(in string) (string, bool) {
	d := strings.ToLower(strings.TrimSpace(in))
	if d == "" {
		return "", false
	}
	if !strings.Contains(d, "://") {
		d = "https://" + d
	}
	u, err := url.Parse(d)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	return u.Host, true
}

type agentOut struct {
	Description string `json:"description"`
	Access      string `json:"access"`
}

type companyOut struct {
	Name   string     `json:"name"`
	Domain string     `json:"domain"`
	Agents []agentOut `json:"agents"`
}

// findCompany discovers a domain's Poppy agents (§3). Not finding one is an
// answer, with HTTP 200, not an error the model would retry.
func (s *server) findCompany(ctx context.Context, req toolRequest) (any, *toolError) {
	domain, ok := normalizeDomain(req.Domain)
	if !ok {
		return map[string]any{"found": false, "reason": fmt.Sprintf("%q is not a domain name", req.Domain)}, nil
	}
	notFound := map[string]any{"found": false, "reason": "no Poppy agent at " + domain}
	ctx, cancel := context.WithTimeout(ctx, companyTimeout)
	defer cancel()
	co, err := s.client.Discover(ctx, domain)
	if err != nil {
		log.Printf("find_company: domain=%s: %v", domain, err)
		return notFound, nil
	}
	if len(co.Agents) == 0 {
		return notFound, nil
	}
	s.client.Remember(co)
	s.mu.Lock()
	s.allowed[domain] = true
	s.mu.Unlock()
	out := companyOut{Name: co.Name, Domain: co.Domain}
	for _, a := range co.Agents {
		out.Agents = append(out.Agents, agentOut{Description: a.Description, Access: a.Access})
	}
	return out, nil
}

// startConversation opens a conversation with a known or found Company's
// first agent (§8.3) and follows it with a Reader.
func (s *server) startConversation(ctx context.Context, req toolRequest) (any, *toolError) {
	domain, ok := normalizeDomain(req.Domain)
	if !ok {
		return nil, fail(http.StatusBadRequest, "%q is not a domain name", req.Domain)
	}
	if strings.TrimSpace(req.Message) == "" {
		return nil, fail(http.StatusBadRequest, "message is empty")
	}
	s.mu.Lock()
	allowed := s.allowed[domain]
	s.mu.Unlock()
	if !allowed {
		return nil, fail(http.StatusBadRequest, "%s is not a company Ellen has an account at, nor one found with find_company; call find_company first", domain)
	}
	ctx, cancel := context.WithTimeout(ctx, companyTimeout)
	defer cancel()
	co, err := s.client.Discover(ctx, domain)
	if err != nil {
		log.Printf("start_conversation: domain=%s: %v", domain, err)
		return nil, fail(http.StatusBadGateway, "no Poppy agent at %s", domain)
	}
	if len(co.Agents) == 0 {
		return nil, fail(http.StatusBadGateway, "no Poppy agent at %s", domain)
	}
	s.client.Remember(co)
	conv, err := s.client.Start(ctx, co, co.Agents[0], req.Message)
	if err != nil {
		return nil, companyError(domain, err)
	}
	s.follow(conv)
	return map[string]string{"conversation_id": conv.ID, "company": co.Name}, nil
}

// lookup finds a stored conversation by ID.
func (s *server) lookup(id string) (conversation.Conversation, *toolError) {
	if id == "" {
		return conversation.Conversation{}, fail(http.StatusBadRequest, "conversation_id is empty")
	}
	convs, err := s.store.Conversations()
	if err != nil {
		return conversation.Conversation{}, fail(http.StatusInternalServerError, "reading conversations: %v", err)
	}
	for _, c := range convs {
		if c.ID == id {
			return c, nil
		}
	}
	return conversation.Conversation{}, fail(http.StatusNotFound, "no conversation %s", id)
}

func (s *server) isClosed(c conversation.Conversation) bool {
	if c.Closed {
		return true
	}
	_, _, closed, err := s.store.State(c.ID)
	return err == nil && closed
}

// assistants refuses a Direct Conversation: it is Ellen's own (§8.10), and
// the assistant neither speaks nor closes in it.
func assistants(c conversation.Conversation) *toolError {
	if c.ParentID != "" {
		return fail(http.StatusForbidden, "conversation %s is Ellen's own Direct Conversation; only she writes in it", c.ID)
	}
	return nil
}

func (s *server) sendMessage(ctx context.Context, req toolRequest) (any, *toolError) {
	c, terr := s.lookup(req.ConversationID)
	if terr != nil {
		return nil, terr
	}
	if terr := assistants(c); terr != nil {
		return nil, terr
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, fail(http.StatusBadRequest, "text is empty")
	}
	if s.isClosed(c) {
		return nil, fail(http.StatusConflict, "conversation %s is closed", c.ID)
	}
	ctx, cancel := context.WithTimeout(ctx, companyTimeout)
	defer cancel()
	if err := s.client.Send(ctx, c, req.Text); err != nil {
		return nil, companyError(c.Company, err)
	}
	return map[string]any{"conversation_id": c.ID, "sent": true}, nil
}

type messageOut struct {
	Role   string `json:"role"`
	Author string `json:"author"`
	Text   string `json:"text"`
}

type readOut struct {
	Company  string       `json:"company"`
	Working  bool         `json:"working"`
	Handoff  string       `json:"handoff"`
	Messages []messageOut `json:"messages"`
	Closed   bool         `json:"closed"`
	// Stopped is why the conversation is no longer followed, such as a
	// Company asking for sign-in.
	Stopped string `json:"stopped,omitempty"`
}

// readConversation answers from the Store alone: what the conversation's
// Reader has stored. It never reads from the Company itself, which would
// race the Reader for the same events.
func (s *server) readConversation(_ context.Context, req toolRequest) (any, *toolError) {
	c, terr := s.lookup(req.ConversationID)
	if terr != nil {
		return nil, terr
	}
	working, handoff, closed, err := s.store.State(c.ID)
	if err != nil {
		return nil, fail(http.StatusInternalServerError, "reading state: %v", err)
	}
	stopped, err := s.store.Stopped(c.ID)
	if err != nil {
		return nil, fail(http.StatusInternalServerError, "reading state: %v", err)
	}
	events, err := s.store.Events(c.ID)
	if err != nil {
		return nil, fail(http.StatusInternalServerError, "reading events: %v", err)
	}
	out := readOut{
		Company: s.name(c.Company), Working: working, Handoff: handoffOf(handoff),
		Messages: []messageOut{}, Closed: closed || c.Closed, Stopped: stopped,
	}
	if out.Closed {
		out.Working = false
	}
	for _, e := range events {
		if e.Type == "message" {
			out.Messages = append(out.Messages, messageOut{Role: e.Role, Author: e.AuthorType, Text: e.Text})
		}
	}
	return out, nil
}

// handoffOf is a stored handoff as the model sees it. §8.7 defines none and
// pending, and says to treat any other value as pending.
func handoffOf(h string) string {
	if h == "" || h == "none" {
		return "none"
	}
	return "pending"
}

func (s *server) closeConversation(ctx context.Context, req toolRequest) (any, *toolError) {
	c, terr := s.lookup(req.ConversationID)
	if terr != nil {
		return nil, terr
	}
	if terr := assistants(c); terr != nil {
		return nil, terr
	}
	if !c.Closed {
		ctx, cancel := context.WithTimeout(ctx, companyTimeout)
		defer cancel()
		if err := s.client.Close(ctx, c); err != nil && !conversation.IsCode(err, "conversation_closed") {
			return nil, companyError(c.Company, err)
		}
	}
	return map[string]any{"conversation_id": c.ID, "closed": true}, nil
}

// direct is Ellen taking a conversation over (§8.10): her first line opens
// a Direct Conversation linked to it, and later lines go to that same one.
// conversation_id may name the parent or the Direct Conversation itself.
func (s *server) direct(ctx context.Context, req toolRequest) (any, *toolError) {
	c, terr := s.lookup(req.ConversationID)
	if terr != nil {
		return nil, terr
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, fail(http.StatusBadRequest, "text is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, companyTimeout)
	defer cancel()
	s.directMu.Lock()
	defer s.directMu.Unlock()

	target := c
	if c.ParentID == "" {
		target = conversation.Conversation{}
		convs, err := s.store.Conversations()
		if err != nil {
			return nil, fail(http.StatusInternalServerError, "reading conversations: %v", err)
		}
		for _, d := range convs {
			if d.ParentID == c.ID && !s.isClosed(d) {
				target = d
				break
			}
		}
	}
	if target.ID == "" {
		d, err := s.client.OpenDirect(ctx, c, req.Text)
		if err != nil {
			return nil, companyError(c.Company, err)
		}
		s.follow(d)
		target = d
	} else {
		if s.isClosed(target) {
			return nil, fail(http.StatusConflict, "conversation %s is closed", target.ID)
		}
		if err := s.client.Send(ctx, target, req.Text); err != nil {
			return nil, companyError(target.Company, err)
		}
	}
	return map[string]string{
		"conversation_id": target.ID, "parent_conversation_id": target.ParentID,
		"company": s.name(target.Company),
	}, nil
}

// updates streams every conversation's new events to the page, one
// {"conversation_id","company","event"} JSON line per event (SSE).
func (s *server) updates(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch, stop := s.hub.subscribe()
	defer stop()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	ping := time.NewTicker(updatesPing)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		case b := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// hub fans updates out to every connected page. Each page has its own
// bounded buffer; when it is full the update is dropped for that page
// alone, so a slow page never blocks a Reader or another page.
type hub struct {
	mu      sync.Mutex
	subs    map[chan []byte]struct{}
	dropped int
}

func (h *hub) subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, subscriberBuffer)
	h.mu.Lock()
	if h.subs == nil {
		h.subs = map[chan []byte]struct{}{}
	}
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

func (h *hub) publish(b []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
			h.dropped++
			if h.dropped == 1 || h.dropped%100 == 0 {
				log.Printf("updates: a page is not keeping up; %d updates dropped so far", h.dropped)
			}
		}
	}
}
