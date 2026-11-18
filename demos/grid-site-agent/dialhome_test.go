package main

import (
	"context"
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
// gRPC through it, as the facade does. Nothing ever connects to the site.
type relay struct {
	srv *httptest.Server
	mu  sync.Mutex
	ses *yamux.Session
}

func newRelay(t *testing.T) *relay {
	t.Helper()
	r := &relay{}
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
// sent from the cluster side runs the work-order tool on the site. The
// facade's stream carries the reply and no tool call: the site's tools never
// cross the channel.
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

	tctx, tcancel := context.WithTimeout(ctx, 15*time.Second)
	defer tcancel()
	stream, err := runtimev1.NewRuntimeServiceClient(r.conn(t)).Converse(tctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&runtimev1.ClientMessage{SessionId: "s1",
		Content: "Raise an urgent work order to replace the blade B pitch motor on turbine 7, estimated £6,400"}); err != nil {
		t.Fatal(err)
	}
	var final string
	for final == "" {
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
			final = d.GetFinalContent()
		}
	}
	if !strings.Contains(final, "Raised WO-WE-0412 on WE-T07") {
		t.Fatalf("final = %q", final)
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
	if code := run(nil, func(string) string { return "" }); code != exitConfig {
		t.Fatalf("run without a dial-home URL = %d, want %d", code, exitConfig)
	}
}
