package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type fixture struct {
	poppy       func(host string) string
	poppyHandle http.HandlerFunc // overrides poppy when set
	prm         http.HandlerFunc // nil => 404
}

func serve(t *testing.T, f fixture) (*httptest.Server, string) {
	t.Helper()
	var host string
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/poppy.json", func(w http.ResponseWriter, r *http.Request) {
		if f.poppyHandle != nil {
			f.poppyHandle(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(f.poppy(host)))
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		if f.prm == nil {
			http.NotFound(w, r)
			return
		}
		f.prm(w, r)
	})
	mux.HandleFunc("/elsewhere", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(f.poppy(host)))
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	host = u.Host
	return srv, host
}

func doc(domain string, protocols ...map[string]string) string {
	b, _ := json.Marshal(map[string]any{
		"organization": map[string]string{"name": "Wrenmoor Energy", "domain": domain},
		"agent":        map[string]any{"protocols": protocols},
	})
	return string(b)
}

func poppyProto() map[string]string {
	return map[string]string{"type": "poppy", "endpoint": "https://agents.example/poppy", "description": "Outages", "access": "signed_in"}
}

func TestDiscoverNoIssuer(t *testing.T) {
	srv, host := serve(t, fixture{poppy: func(h string) string { return doc(h, poppyProto()) }})
	c, err := Discover(context.Background(), srv.Client(), host)
	if err != nil {
		t.Fatal(err)
	}
	if c.Identifier != "https://"+host || c.Issuer != "" || c.Name != "Wrenmoor Energy" || c.Domain != host {
		t.Fatalf("unexpected company %+v", c)
	}
	if len(c.Agents) != 1 || c.Agents[0].Access != "signed_in" || c.Agents[0].Endpoint != "https://agents.example/poppy" {
		t.Fatalf("unexpected agents %+v", c.Agents)
	}
}

func TestDiscoverWithIssuer(t *testing.T) {
	srv, host := serve(t, fixture{
		poppy: func(h string) string { return doc(h, poppyProto()) },
		prm: func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"authorization_servers":["https://idp.example/tenant","https://other"]}`))
		},
	})
	c, err := Discover(context.Background(), srv.Client(), host)
	if err != nil {
		t.Fatal(err)
	}
	if c.Issuer != "https://idp.example/tenant" || c.Identifier != c.Issuer {
		t.Fatalf("unexpected company %+v", c)
	}
}

func TestDiscoverDomainMismatch(t *testing.T) {
	srv, host := serve(t, fixture{poppy: func(string) string { return doc("evil.example", poppyProto()) }})
	_, err := Discover(context.Background(), srv.Client(), host)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("want domain mismatch error, got %v", err)
	}
	if errors.Is(err, ErrUnreachable) {
		t.Errorf("a domain mismatch is the domain's answer, not ErrUnreachable: %v", err)
	}
}

func TestDiscoverRedirectToOtherPath(t *testing.T) {
	srv, host := serve(t, fixture{
		poppy: func(h string) string { return doc(h, poppyProto()) },
		poppyHandle: func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		},
	})
	_, err := Discover(context.Background(), srv.Client(), host)
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("want redirect error, got %v", err)
	}
	if errors.Is(err, ErrUnreachable) {
		t.Errorf("a refused redirect is the domain's answer, not ErrUnreachable: %v", err)
	}
}

func TestDiscoverProtectedResource500(t *testing.T) {
	srv, host := serve(t, fixture{
		poppy: func(h string) string { return doc(h, poppyProto()) },
		prm:   func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) },
	})
	_, err := Discover(context.Background(), srv.Client(), host)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("want 500 error, got %v", err)
	}
	if !errors.Is(err, ErrUnreachable) {
		t.Errorf("a 5xx may pass, so want ErrUnreachable: %v", err)
	}
}

func TestDiscoverIgnoresNonPoppyProtocols(t *testing.T) {
	srv, host := serve(t, fixture{poppy: func(h string) string {
		return doc(h,
			map[string]string{"type": "mcp", "endpoint": "https://a.example/mcp"},
			map[string]string{"type": "poppy", "endpoint": "http://insecure.example/p"},
			map[string]string{"type": "poppy", "endpoint": "https://ok.example/p", "access": "bogus"},
		)
	}})
	c, err := Discover(context.Background(), srv.Client(), host)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Agents) != 1 || c.Agents[0].Endpoint != "https://ok.example/p" || c.Agents[0].Access != "session" {
		t.Fatalf("unexpected agents %+v", c.Agents)
	}
}
