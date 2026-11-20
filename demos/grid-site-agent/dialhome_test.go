package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	dialhome "github.com/AltairaLabs/omnia-dialhome"
	"github.com/AltairaLabs/omnia-dialhome/runtimev1"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const relayKey = "odh_site-test-key"

// relay is a stand-in for the relay Omnia runs beside the agent's facade: it
// accepts the site's outbound connection, and reaches the site's runtime over
// gRPC through it, as the facade does. Nothing ever connects to the site. It
// answers the site's policy questions as grid-site-policy does, and keeps the
// tool calls the site records.
type relay struct {
	srv *httptest.Server
	mu  sync.Mutex
	ses *yamux.Session

	asked    []dialhome.PolicyRequest
	recorded []dialhome.ToolCallRecord
	streams  map[dialhome.StreamKind]*streamListener
}

// governance serves the site's policy and tool-call streams.
func (r *relay) governance() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+dialhome.PolicyDecisionPath, func(w http.ResponseWriter, req *http.Request) {
		var q dialhome.PolicyRequest
		_ = json.NewDecoder(req.Body).Decode(&q)
		r.mu.Lock()
		r.asked = append(r.asked, q)
		r.mu.Unlock()
		_ = json.NewEncoder(w).Encode(decide(q))
	})
	mux.HandleFunc("POST "+dialhome.ToolCallsPath, func(w http.ResponseWriter, req *http.Request) {
		var rec dialhome.ToolCallRecord
		_ = json.NewDecoder(req.Body).Decode(&rec)
		r.mu.Lock()
		r.recorded = append(r.recorded, rec)
		r.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})
	return mux
}

// accept hands the streams the site opens to their kind's server; any other
// kind is closed, as Omnia's relay does.
func (r *relay) accept(s *yamux.Session) {
	for {
		st, err := s.AcceptStream()
		if err != nil {
			return
		}
		kind, err := dialhome.ReadStreamHeader(st)
		if l, ok := r.streams[kind]; err == nil && ok {
			select {
			case l.conns <- st:
			case <-l.done:
				_ = st.Close()
			}
			continue
		}
		_ = st.Close()
	}
}

func (r *relay) records() []dialhome.ToolCallRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]dialhome.ToolCallRecord(nil), r.recorded...)
}

// streamListener is a net.Listener of the streams of one kind.
type streamListener struct {
	conns chan net.Conn
	done  chan struct{}
}

func (l *streamListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *streamListener) Close() error   { return nil }
func (l *streamListener) Addr() net.Addr { return dialhome.Addr("relay") }

func newRelay(t *testing.T) *relay {
	t.Helper()
	r := &relay{streams: map[dialhome.StreamKind]*streamListener{}}
	done := make(chan struct{})
	gov := &http.Server{Handler: r.governance(), ReadHeaderTimeout: time.Second}
	for _, kind := range []dialhome.StreamKind{dialhome.KindPolicy, dialhome.KindToolCalls} {
		l := &streamListener{conns: make(chan net.Conn), done: done}
		r.streams[kind] = l
		go func() { _ = gov.Serve(l) }()
	}
	t.Cleanup(func() {
		close(done)
		_ = gov.Close()
	})
	up := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc(dialhome.ChannelPath, func(w http.ResponseWriter, req *http.Request) {
		if k, ok := dialhome.BearerKey(req.Header.Get("Authorization")); !ok || k != relayKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ws, err := up.Upgrade(w, req, nil)
		if err != nil {
			return
		}
		s, err := yamux.Server(dialhome.NewWSConn(ws), dialhome.YamuxConfig(time.Second))
		if err != nil {
			return
		}
		r.mu.Lock()
		r.ses = s
		r.mu.Unlock()
		go r.accept(s)
		<-s.CloseChan()
	})
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)
	return r
}

func (r *relay) url() string {
	return "ws" + strings.TrimPrefix(r.srv.URL, "http") + dialhome.ChannelPath
}

func (r *relay) session() *yamux.Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ses
}

func (r *relay) conn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	cc, err := grpc.NewClient("passthrough:///site",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			s := r.session()
			if s == nil {
				return nil, dialhome.ErrNotConnected
			}
			st, err := s.OpenStream()
			if err != nil {
				return nil, err
			}
			return st, dialhome.WriteStreamHeader(st, dialhome.KindRuntime)
		}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	return cc
}

// The whole site agent, as the image runs it: it dials the relay, and a turn
// sent from the cluster side runs the work-order tool on the site once Omnia
// has allowed it, and records it on the session. The facade's stream carries
// the reply and no tool call: the site's tools never cross the channel. The
// £48,000 order is refused by the policy before the tool runs.
func TestTheSiteDialsHomeAndServesATurn(t *testing.T) {
	r := newRelay(t)
	env := map[string]string{
		"OMNIA_DIAL_HOME_URL": r.url(),
		"OMNIA_DIAL_HOME_KEY": relayKey,
		"SITE_AGENT_PACK":     testPack,
	}
	ctx, cancel := context.WithCancel(context.Background())
	exited := make(chan int, 1)
	go func() { exited <- serve(ctx, nil, func(k string) string { return env[k] }) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			t.Error("the site agent did not stop")
		}
	})

	deadline := time.Now().Add(10 * time.Second)
	for r.session() == nil {
		if time.Now().After(deadline) {
			t.Fatal("the site never dialled home")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cc := r.conn(t)
	final := turn(ctx, t, cc, "s1", "Raise an urgent work order to replace the blade B pitch motor on turbine 7, estimated £6,400")
	if !strings.Contains(final, "Raised WO-WE-0412 on WE-T07") {
		t.Fatalf("final = %q", final)
	}
	deadline = time.Now().Add(5 * time.Second)
	for len(r.records()) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("tool calls recorded: %+v", r.records())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if recs := r.records(); recs[0].Phase != dialhome.ToolCallStarted || recs[1].Phase != dialhome.ToolCallCompleted ||
		recs[1].Tool != "raise_work_order" || recs[1].SessionID != "s1" {
		t.Fatalf("tool calls recorded: %+v", recs)
	}

	final = turn(ctx, t, cc, "s2",
		"Raise an urgent work order to regenerate the oil and replace two cooling fans on the transformer, estimated £48,000")
	if !strings.HasPrefix(final, "Omnia refused that. This work order is above the £25000") {
		t.Fatalf("final = %q", final)
	}
	if strings.Contains(final, "WO-WE-0413") {
		t.Fatal("the order above the limit was raised")
	}
	// Omnia was asked, over the channel, under the site's registry name.
	r.mu.Lock()
	asked := append([]dialhome.PolicyRequest(nil), r.asked...)
	r.mu.Unlock()
	last := asked[len(asked)-1]
	if last.SessionID != "s2" || last.Tool != "raise_work_order" || last.Registry != defaultRegistry ||
		!strings.Contains(string(last.Arguments), `"estimated_cost_gbp":48000`) {
		t.Fatalf("Omnia was asked %+v", last)
	}
	// The refused call never ran, so the site recorded nothing for it: the
	// relay records a refusal itself.
	for _, rec := range r.records() {
		if rec.SessionID == "s2" {
			t.Fatalf("the site recorded a refused call: %+v", rec)
		}
	}
}

// turn sends one message on session and returns the reply.
func turn(ctx context.Context, t *testing.T, cc *grpc.ClientConn, session, msg string) string {
	t.Helper()
	tctx, tcancel := context.WithTimeout(ctx, 15*time.Second)
	defer tcancel()
	stream, err := runtimev1.NewRuntimeServiceClient(cc).Converse(tctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&runtimev1.ClientMessage{SessionId: session, Content: msg}); err != nil {
		t.Fatal(err)
	}
	for {
		m, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if e := m.GetError(); e != nil {
			t.Fatalf("runtime error: %v", e)
		}
		if m.GetToolCall() != nil {
			t.Fatal("a site tool crossed the channel as a tool call")
		}
		if d := m.GetDone(); d != nil {
			return d.GetFinalContent()
		}
	}
}

func TestParse(t *testing.T) {
	o, err := parse(nil, func(string) string { return "" })
	if err != nil || o.pack != defaultPack || o.prompt != defaultPrompt || o.model != modelScript {
		t.Fatalf("defaults = %+v, %v", o, err)
	}
	if _, err := parse([]string{"-model", "gpt"}, func(string) string { return "" }); err == nil {
		t.Fatal("an unknown model was accepted")
	}
	o, err = parse(nil, func(k string) string {
		return map[string]string{"SITE_AGENT_POLICY_REGISTRY": "other-tools", "SITE_AGENT_MEMORY": "true"}[k]
	})
	if err != nil || o.registry != "other-tools" || !o.memory {
		t.Fatalf("governance settings = %+v, %v", o, err)
	}
	if o, _ := parse(nil, func(string) string { return "" }); o.registry != defaultRegistry || o.memory {
		t.Fatalf("governance defaults = %+v", o)
	}
	if code := run(nil, func(string) string { return "" }); code != exitConfig {
		t.Fatalf("run without a dial-home URL = %d, want %d", code, exitConfig)
	}
}
