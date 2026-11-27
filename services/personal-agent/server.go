package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AltairaLabs/omnia-demo/services/personal-agent/conversation"
	"github.com/AltairaLabs/omnia-demo/services/personal-agent/discovery"
	"github.com/AltairaLabs/omnia-demo/services/personal-agent/identity"
	"github.com/AltairaLabs/omnia-demo/services/personal-agent/signin"
)

const (
	// subscriberBuffer is how many updates a page may fall behind by before
	// it misses some. A page that stops reading never holds up a Reader.
	subscriberBuffer = 64
	maxRequestBody   = 1 << 20
	companyTimeout   = 30 * time.Second
	updatesPing      = 25 * time.Second
	// signInPendingFor is how long a begun sign-in reads as pending: as long
	// as the Flow keeps its state (10 minutes). After that its redirect is
	// refused anyway.
	signInPendingFor = 10 * time.Minute
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
	// SignIn runs Direct Sign-In (PAP §4.2) and keeps the tokens it grants
	// fresh. nil means this agent never signs in: every request is signed
	// out and sign_in says sign-in isn't set up.
	SignIn *signin.Flow
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
	signIn  *signin.Flow // nil when sign-in isn't set up

	mu      sync.Mutex
	allowed map[string]bool   // domains start_conversation may use
	names   map[string]string // Company name by domain
	ids     map[string]string // Company identifier (§3.2) by domain
	readers map[string]bool   // conversations with a running Reader
	// again marks a conversation whose stopped Reader is to run once more
	// as soon as it has left, because a sign-in completed meanwhile.
	again   map[string]bool
	signins map[string]signInState // the latest sign-in begun, by domain
	// asked is the scopes the latest sign-in asked for, by domain.
	asked map[string][]string
	// lastRead is, by conversation, what read_conversation last showed the
	// assistant. A conversation that has moved on from it is unread in
	// /api/conversations.
	lastRead map[string]readOut
	// signedIn counts completed sign-ins by domain. A Reader that stops
	// asking for sign-in after the count moved since it began raced a
	// sign-in, and runs again.
	signedIn map[string]int
	wg       sync.WaitGroup

	directMu sync.Mutex // one Direct Conversation per parent
}

func newServer(ctx context.Context, cfg config) *server {
	s := &server{
		ctx:      ctx,
		id:       cfg.Identity,
		store:    cfg.Store,
		client:   conversation.NewClient(cfg.HTTP, cfg.Store),
		testDir:  cfg.TestIdentityDir,
		backoff:  cfg.ReaderBackoff,
		signIn:   cfg.SignIn,
		allowed:  map[string]bool{},
		names:    map[string]string{},
		ids:      map[string]string{},
		readers:  map[string]bool{},
		again:    map[string]bool{},
		signins:  map[string]signInState{},
		lastRead: map[string]readOut{},
		asked:    map[string][]string{},

		signedIn: map[string]int{},
	}
	if cfg.SignIn != nil {
		// Only when set: a nil *Flow in the interface would not be nil.
		s.client.Tokens = cfg.SignIn
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
			s.ids[co.Domain] = co.Identifier
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
	mux.HandleFunc("POST /tools/sign_in", s.tool("sign_in", s.signInTool))
	// Public: where the Company's sign-in page sends Ellen's browser back
	// (PAP §4.2). It answers a page that names the Company and nothing else.
	mux.HandleFunc("GET /signin/callback", s.signInCallback)
	// The page. Not tools: only Ellen takes a conversation over.
	mux.HandleFunc("POST /api/direct", s.tool("direct", s.direct))
	mux.HandleFunc("GET /api/updates", s.updates)
	mux.HandleFunc("GET /api/conversations", s.conversations)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

// wait returns once every Reader has stopped, after the server's context
// has ended. follow checks the context and adds to wg under s.mu, so once
// wait has taken s.mu no new Reader can start, and wg.Add never races
// wg.Wait.
func (s *server) wait() {
	s.mu.Lock() // an empty critical section: a barrier for follow
	s.mu.Unlock()
	s.wg.Wait()
}

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
	s.start(conv, false)
}

// refollow follows conv again after its Reader stopped: at once, or, if the
// stopped Reader has not quite left yet, as soon as it has.
func (s *server) refollow(conv conversation.Conversation) {
	s.start(conv, true)
}

func (s *server) start(conv conversation.Conversation, again bool) {
	s.mu.Lock()
	if s.ctx.Err() != nil {
		// The server is shutting down: a Reader started now would outlive
		// wait.
		s.mu.Unlock()
		return
	}
	if s.readers[conv.ID] {
		if again {
			s.again[conv.ID] = true
		}
		s.mu.Unlock()
		return
	}
	s.readers[conv.ID] = true
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		for {
			s.mu.Lock()
			gen := s.signedIn[conv.Company]
			s.mu.Unlock()
			r := &conversation.Reader{Client: s.client, Backoff: s.backoff}
			r.Run(s.ctx, conv, s.publish)
			reason, _ := s.store.Stopped(conv.ID)
			// A Reader whose request went out signed out before a sign-in
			// completed, and that stopped asking for sign-in after the
			// callback looked for stopped conversations, runs again.
			raced := strings.HasPrefix(reason, conversation.CodeSignInRequired)
			// Leave before saying why, so whoever acts on it (a sign-in that
			// resumes the conversation) can start a new Reader.
			s.mu.Lock()
			raced = raced && s.signedIn[conv.Company] != gen
			rerun := (s.again[conv.ID] || raced) && s.ctx.Err() == nil
			delete(s.again, conv.ID)
			if !rerun {
				delete(s.readers, conv.ID)
			}
			s.mu.Unlock()
			if !rerun {
				break
			}
			if raced {
				log.Printf("signin: resuming conversation=%s company=%s, stopped as a sign-in completed", conv.ID, conv.Company)
				if err := s.store.SetStopped(conv.ID, ""); err != nil {
					log.Printf("signin: resuming conversation=%s: %v", conv.ID, err)
				}
			}
		}
		if reason, _ := s.store.Stopped(conv.ID); reason != "" {
			log.Printf("reader stopped: conversation=%s reason=%s", conv.ID, reason)
			b, _ := json.Marshal(map[string]string{"type": "stopped", "reason": reason})
			s.publishRaw(conv, b)
			return
		}
		// A Reader that left because the conversation closed with no state
		// event (conversation_closed, or an expired conversation) has
		// published nothing saying so. Say it, so the page and a waiting
		// read_conversation see it.
		if s.ctx.Err() == nil && s.isClosed(conv) {
			b, _ := json.Marshal(map[string]any{"type": "state", "working": false, "handoff": "none", "closed": true})
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
		Company: s.name(conv), Event: raw,
	})
	if err != nil {
		return
	}
	s.hub.publish(b)
}

// name is a conversation's Company name: as last discovered, else as stored
// when the conversation started, else its domain.
func (s *server) name(c conversation.Conversation) string {
	s.mu.Lock()
	n := s.names[c.Company]
	s.mu.Unlock()
	switch {
	case n != "":
		return n
	case c.Name != "":
		return c.Name
	}
	return c.Company
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
	// Scopes are the scopes sign_in asks for, space-separated.
	Scopes string `json:"scopes"`
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
// answer, with HTTP 200, not an error the model would retry. A failure that
// may pass (a network error or a 5xx) says so, and may be retried once.
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
		if errors.Is(err, discovery.ErrUnreachable) {
			return map[string]any{"found": false, "reason": unreachableReason(domain)}, nil
		}
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
		if errors.Is(err, discovery.ErrUnreachable) {
			return nil, fail(http.StatusBadGateway, "%s", unreachableReason(domain))
		}
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

func unreachableReason(domain string) string {
	return "couldn't reach " + domain + " right now; try again shortly"
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
	// SignIn is set while the Company wants Ellen signed in, or a sign-in
	// with it has begun or ended.
	SignIn *signInOut `json:"sign_in,omitempty"`
}

// signInOut is a conversation's sign-in state as the model sees it. Needed
// is whether the Company wants Ellen signed in for Scopes (space-separated,
// "" when it didn't say). Status is needed, pending (a sign-in begun and not
// completed), signed_in or declined.
type signInOut struct {
	Needed bool   `json:"needed"`
	Scopes string `json:"scopes"`
	Status string `json:"status"`
}

// readConversation answers from the Store alone: what the conversation's
// Reader has stored. It never reads from the Company itself, which would
// race the Reader for the same events. It never waits either: the Company
// answers in its own time (PAP §8.5, §8.6), the Reader takes the answer off
// the stream, and the page wakes the assistant for it (unread in
// /api/conversations says whether there is anything it has not read).
func (s *server) readConversation(_ context.Context, req toolRequest) (any, *toolError) {
	c, terr := s.lookup(req.ConversationID)
	if terr != nil {
		return nil, terr
	}
	out, terr := s.read(c)
	if terr != nil {
		return nil, terr
	}
	s.mu.Lock()
	s.lastRead[c.ID] = out
	s.mu.Unlock()
	return out, nil
}

// unread reports whether c has moved on since the assistant last read it:
// the company said something new, a sign-in changed, a person took over,
// or the conversation stopped. A close is not news: the assistant closes
// conversations itself. Never read, it is unread once the company has said
// or asked anything.
func (s *server) unread(id string, now readOut) bool {
	s.mu.Lock()
	last, ok := s.lastRead[id]
	s.mu.Unlock()
	if !ok {
		last = readOut{Handoff: "none"}
	}
	return companyMessages(now) > companyMessages(last) ||
		now.Handoff != last.Handoff ||
		now.Stopped != last.Stopped ||
		!sameSignIn(last.SignIn, now.SignIn)
}

func sameSignIn(a, b *signInOut) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func companyMessages(r readOut) int {
	n := 0
	for _, m := range r.Messages {
		if m.Role == "company" {
			n++
		}
	}
	return n
}

// read is a conversation as the Store holds it: its state and its messages.
func (s *server) read(c conversation.Conversation) (readOut, *toolError) {
	working, handoff, closed, err := s.store.State(c.ID)
	if err != nil {
		return readOut{}, fail(http.StatusInternalServerError, "reading state: %v", err)
	}
	stopped, err := s.store.Stopped(c.ID)
	if err != nil {
		return readOut{}, fail(http.StatusInternalServerError, "reading state: %v", err)
	}
	events, err := s.store.Events(c.ID)
	if err != nil {
		return readOut{}, fail(http.StatusInternalServerError, "reading events: %v", err)
	}
	out := readOut{
		Company: s.name(c), Working: working, Handoff: handoffOf(handoff),
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
	out.SignIn = s.signInOf(c, events, stopped)
	return out, nil
}

// snapshotOut is one conversation in /api/conversations.
type snapshotOut struct {
	ConversationID       string `json:"conversation_id"`
	ParentConversationID string `json:"parent_conversation_id,omitempty"`
	// Unread is whether the conversation has moved on since the assistant
	// last read it, so the page knows whether to wake it.
	Unread bool `json:"unread"`
	readOut
}

// conversations is every stored conversation, oldest first, with its state
// and messages: what a page that has just loaded missed before it follows
// /api/updates. Like read_conversation it answers from the Store alone.
func (s *server) conversations(w http.ResponseWriter, _ *http.Request) {
	convs, err := s.store.Conversations()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reading conversations"})
		log.Printf("conversations: %v", err)
		return
	}
	out := make([]snapshotOut, 0, len(convs))
	for _, c := range convs {
		r, terr := s.read(c)
		if terr != nil {
			writeJSON(w, terr.status, map[string]string{"error": terr.msg})
			return
		}
		out = append(out, snapshotOut{ConversationID: c.ID, ParentConversationID: c.ParentID, Unread: s.unread(c.ID, r), readOut: r})
	}
	writeJSON(w, http.StatusOK, out)
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
		err := s.client.Close(ctx, c)
		switch {
		case conversation.IsCode(err, "conversation_closed"):
			// The Company closed it first: record it closed here too, so it
			// is never sent on again (Client.Close records only its own).
			c.Closed = true
			if err := s.store.SaveConversation(c); err != nil {
				return nil, fail(http.StatusInternalServerError, "recording the close: %v", err)
			}
			if err := s.store.SetState(c.ID, false, "none", true); err != nil {
				return nil, fail(http.StatusInternalServerError, "recording the close: %v", err)
			}
		case err != nil:
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
		"company": s.name(target),
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

// signInState is the latest sign-in begun with a Company: pending from
// sign_in until its redirect comes back, then signed_in or declined. A
// sign-in that failed leaves none. It is kept in memory only, like the
// Flow's pending sign-ins; after a restart the token store alone says
// whether Ellen is signed in.
type signInState struct {
	status string
	at     time.Time
}

// signInStatus is the domain's latest sign-in status, "" for none or for a
// pending one whose redirect can no longer come back.
func (s *server) signInStatus(domain string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.signins[domain]
	if st.status == "pending" && time.Since(st.at) > signInPendingFor {
		return ""
	}
	return st.status
}

func (s *server) setSignInStatus(domain, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if status == "" {
		delete(s.signins, domain)
		return
	}
	s.signins[domain] = signInState{status: status, at: time.Now()}
}

// signInOf is a conversation's sign-in state. Sign-in is needed when the
// authorization events (§8.11) of the latest turn, those since the latest
// role "user" message, name scopes Ellen's tokens for the Company don't all
// hold, or when its Reader stopped on a 401 asking for sign-in (§5.4).
// Scopes is the union of those events' scopes and the 401 challenge's. An
// event whose scope is "" names none, so for it holds is vacuously true once
// Ellen holds any tokens at the Company: signed in, it needs no more. Signed
// out, it needs sign-in, with Scopes "" (sign_in then falls back).
// nil when sign-in isn't needed and none has begun.
func (s *server) signInOf(c conversation.Conversation, events []conversation.Event, stopped string) *signInOut {
	var scopes []string
	asked := false
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Type == "message" && e.Role == "user" {
			break
		}
		if e.Type != "authorization" {
			continue
		}
		var a struct {
			Scope string `json:"scope"`
		}
		_ = json.Unmarshal(e.Raw, &a)
		scopes, asked = append(scopes, strings.Fields(a.Scope)...), true
	}
	stoppedForSignIn := strings.HasPrefix(stopped, conversation.CodeSignInRequired)
	if stoppedForSignIn {
		scopes = append(scopes, conversation.StopScopes(stopped)...)
	}
	slices.Sort(scopes)
	scopes = slices.Compact(scopes)
	needed := stoppedForSignIn || (asked && !s.holds(c.Company, scopes))
	st := s.signInStatus(c.Company)
	if !needed && st == "" {
		return nil
	}
	status := st
	switch {
	case st == "pending":
	case needed && st != "declined":
		// Not yet signed in, or signed in without what is needed now.
		status = "needed"
	}
	return &signInOut{Needed: needed, Scopes: strings.Join(scopes, " "), Status: status}
}

// holds reports whether Ellen's stored tokens for the Company at domain hold
// every scope in scopes. With no Flow, or a Company not discovered since
// this process started, there are none.
func (s *server) holds(domain string, scopes []string) bool {
	if s.signIn == nil || s.signIn.Tokens == nil {
		return false
	}
	s.mu.Lock()
	identifier := s.ids[domain]
	s.mu.Unlock()
	if identifier == "" {
		return false
	}
	userID, err := s.store.UserID(identifier)
	if err != nil {
		return false
	}
	t, err := s.signIn.Tokens.Get(identifier, userID)
	if err != nil || t == nil {
		return false
	}
	for _, sc := range scopes {
		if !slices.Contains(t.Scopes, sc) {
			return false
		}
	}
	return true
}

// publishDomain sends raw to the page on every open conversation with the
// Company at domain, and answers how many there were.
func (s *server) publishDomain(domain string, raw json.RawMessage) int {
	convs, err := s.store.Conversations()
	if err != nil {
		log.Printf("publish: %v", err)
		return 0
	}
	n := 0
	for _, c := range convs {
		if c.Company == domain && !s.isClosed(c) {
			s.publishRaw(c, raw)
			n++
		}
	}
	return n
}

// fallbackScopes are the scopes sign_in asks for when it was given none:
// those Ellen holds at the Company, else those she last asked for there,
// else those the Company describes in poppy.json auth.scopes. nil when
// none is known.
func (s *server) fallbackScopes(co *discovery.Company, userID string) []string {
	if t, err := s.signIn.Tokens.Get(co.Identifier, userID); err == nil && t != nil && len(t.Scopes) > 0 {
		return slices.Clone(t.Scopes)
	}
	s.mu.Lock()
	asked := slices.Clone(s.asked[co.Domain])
	s.mu.Unlock()
	if len(asked) > 0 {
		return asked
	}
	return slices.Sorted(maps.Keys(co.Scopes))
}

// signInTool begins a Direct Sign-In (PAP §4.2) at a Company for scopes.
// It publishes the authorization URL to the page (/api/updates), which shows
// Ellen a button; she signs in on the Company's own page, and the redirect
// comes back to /signin/callback. The tool's answer, which the model sees,
// holds no URL or state. Nothing it answers or publishes holds a token, code
// or verifier: the URL carries only the state and the code challenge.
func (s *server) signInTool(ctx context.Context, req toolRequest) (any, *toolError) {
	if s.signIn == nil {
		return nil, fail(http.StatusBadRequest, "sign-in isn't set up on this assistant")
	}
	domain, ok := normalizeDomain(req.Domain)
	if !ok {
		return nil, fail(http.StatusBadRequest, "%q is not a domain name", req.Domain)
	}
	scopes := strings.Fields(req.Scopes)
	s.mu.Lock()
	allowed := s.allowed[domain]
	s.mu.Unlock()
	if !allowed {
		return nil, fail(http.StatusBadRequest, "%s is not a company Ellen has an account at, nor one found with find_company", domain)
	}
	ctx, cancel := context.WithTimeout(ctx, companyTimeout)
	defer cancel()
	co, err := s.client.Discover(ctx, domain)
	if err != nil {
		log.Printf("sign_in: domain=%s: %v", domain, err)
		if errors.Is(err, discovery.ErrUnreachable) {
			return nil, fail(http.StatusBadGateway, "%s", unreachableReason(domain))
		}
		return nil, fail(http.StatusBadGateway, "no Poppy agent at %s", domain)
	}
	s.client.Remember(co)
	if co.Issuer == "" || co.SignInDisabled {
		return nil, fail(http.StatusBadRequest, "%s offers no sign-in", co.Name)
	}
	userID, err := s.store.UserID(co.Identifier)
	if err != nil {
		return nil, fail(http.StatusInternalServerError, "reading the User ID: %v", err)
	}
	// Ask only for scopes the Company offers (§4.7). The model guesses names
	// ("claim.report" where the Company offers "claims"); refusing the whole
	// sign-in for a guess leaves Ellen with no button. An unoffered scope is
	// dropped, and with none left this falls back as if none was given. The
	// Company's consent page shows Ellen what is asked for either way.
	if len(scopes) > 0 {
		offered, err := s.signIn.Offered(ctx, co)
		if err != nil {
			log.Printf("sign_in: domain=%s: %v", domain, err)
			return nil, fail(http.StatusBadGateway, "couldn't start a sign-in at %s: %s", co.Name, strings.TrimPrefix(err.Error(), "signin: "))
		}
		kept := scopes[:0]
		for _, sc := range scopes {
			if slices.Contains(offered, sc) {
				kept = append(kept, sc)
			} else {
				log.Printf("sign_in: domain=%s: dropping scope %q, which %s does not offer (offers %s)", domain, sc, co.Name, strings.Join(offered, ", "))
			}
		}
		scopes = kept
	}
	if len(scopes) == 0 {
		// A 401 or an authorization event that named no scope, or only
		// scopes the Company does not offer.
		if scopes = s.fallbackScopes(co, userID); len(scopes) == 0 {
			return nil, fail(http.StatusBadRequest, "%s named no scopes to sign in for, and none is known; pass scopes, space-separated", co.Name)
		}
	}
	authURL, err := s.signIn.Begin(ctx, co, userID, scopes)
	if err != nil {
		log.Printf("sign_in: domain=%s: %v", domain, err)
		return nil, fail(http.StatusBadGateway, "couldn't start a sign-in at %s: %s", co.Name, strings.TrimPrefix(err.Error(), "signin: "))
	}
	// The scopes asked for: those given, plus any Ellen already granted.
	asked := ""
	if u, err := url.Parse(authURL); err == nil {
		asked = u.Query().Get("scope")
	}
	s.setSignInStatus(domain, "pending")
	s.mu.Lock()
	s.asked[domain] = strings.Fields(asked)
	s.mu.Unlock()
	b, _ := json.Marshal(map[string]string{
		"type": "signin", "status": "pending", "scope": asked, "authorization_url": authURL,
	})
	note := fmt.Sprintf("A 'Sign in to %s' button is on Ellen's screen; she signs in on %s's own page.", co.Name, co.Name)
	if s.publishDomain(domain, b) == 0 {
		note = fmt.Sprintf("No conversation with %s is open on Ellen's screen, so there is nowhere to show the sign-in button. Start a conversation with %s first, then call sign_in again.", co.Name, co.Name)
	}
	// The URL goes only to the page, in the update above. This answer goes to
	// the model and so into the assistant's session transcript, and the URL's
	// state completes the sign-in for whoever began it: anyone who read it
	// there could finish the sign-in with their own account at the Company.
	return map[string]string{
		"company": co.Name,
		"scopes":  asked,
		"status":  "pending",
		"note":    note,
	}, nil
}

// signInCallback is where the Company's sign-in page sends Ellen's browser
// back (PAP §4.2). It completes the sign-in the state names and answers a
// small page saying how it went. The page holds no token, code or state.
//
// It is public, and cannot tell who opened it: a sign-in link opened on
// another device, or forwarded, completes for whoever began it (§4.2,
// §10.8). The Company's consent page (prompt=consent) is what guards that.
func (s *server) signInCallback(w http.ResponseWriter, r *http.Request) {
	if s.signIn == nil {
		writeSignInPage(w, http.StatusNotFound, "Sign-in isn't set up", "This assistant doesn't sign in to companies.")
		return
	}
	// The exchange outlives a browser that gives up waiting: the state is
	// used up either way.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), companyTimeout)
	defer cancel()
	res, err := s.signIn.Complete(ctx, r.URL.Query())
	name := res.Name
	if name == "" {
		name = res.Domain
	}
	switch {
	case errors.Is(err, signin.ErrUnknownState):
		log.Printf("signin callback: %v", err)
		writeSignInPage(w, http.StatusBadRequest, "Sign-in not completed",
			"This sign-in link has expired or has already been used. Ask your assistant to start the sign-in again.")
	case errors.Is(err, signin.ErrIssuerMismatch):
		log.Printf("signin callback: domain=%s: %v", res.Domain, err)
		s.finishSignIn(res, "failed")
		writeSignInPage(w, http.StatusBadRequest, "Sign-in not completed",
			fmt.Sprintf("This sign-in didn't come back from %s's own sign-in page, so your assistant refused it. Ask your assistant to start the sign-in again.", name))
	case err != nil:
		log.Printf("signin callback: domain=%s: %v", res.Domain, err)
		s.finishSignIn(res, "failed")
		writeSignInPage(w, http.StatusBadGateway, "Sign-in not completed",
			fmt.Sprintf("%s didn't complete the sign-in. Ask your assistant to try again.", name))
	case res.Declined:
		s.finishSignIn(res, "declined")
		writeSignInPage(w, http.StatusOK, "Not signed in",
			fmt.Sprintf("You didn't sign in to %s. You can close this tab and go back to your assistant.", name))
	default:
		s.finishSignIn(res, "signed_in")
		writeSignInPage(w, http.StatusOK, "Signed in",
			fmt.Sprintf("Signed in to %s. You can close this tab and go back to your assistant.", name))
	}
}

// finishSignIn records how a sign-in at res.Domain ended and tells the page,
// on each open conversation with that Company. Signed in, it first resumes
// each conversation whose Reader stopped asking for sign-in, so a read woken
// by the update finds it followed again.
func (s *server) finishSignIn(res signin.Result, status string) {
	if res.Domain == "" {
		return
	}
	if status == "failed" {
		s.setSignInStatus(res.Domain, "")
	} else {
		s.setSignInStatus(res.Domain, status)
	}
	if status == "signed_in" {
		s.mu.Lock()
		s.signedIn[res.Domain]++
		s.mu.Unlock()
		convs, err := s.store.Conversations()
		if err != nil {
			log.Printf("signin: resuming conversations: %v", err)
		}
		for _, c := range convs {
			if c.Company != res.Domain || s.isClosed(c) {
				continue
			}
			reason, err := s.store.Stopped(c.ID)
			if err != nil || !strings.HasPrefix(reason, conversation.CodeSignInRequired) {
				continue
			}
			if err := s.store.SetStopped(c.ID, ""); err != nil {
				log.Printf("signin: resuming conversation=%s: %v", c.ID, err)
				continue
			}
			log.Printf("signin: resuming conversation=%s company=%s", c.ID, c.Company)
			s.refollow(c)
		}
	}
	b, _ := json.Marshal(map[string]string{"type": "signin", "status": status, "scope": strings.Join(res.Granted, " ")})
	s.publishDomain(res.Domain, b)
}

var signInPage = template.Must(template.New("signin").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>body{font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;line-height:1.5}</style>
</head>
<body>
<h1>{{.Title}}</h1>
<p>{{.Text}}</p>
</body>
</html>
`))

// writeSignInPage answers the callback's page: no caching, no scripts, no
// referrer (the URL held a code), and its text escaped by html/template.
func writeSignInPage(w http.ResponseWriter, status int, title, text string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := signInPage.Execute(w, struct{ Title, Text string }{title, text}); err != nil {
		log.Printf("signin page: %v", err)
	}
}
