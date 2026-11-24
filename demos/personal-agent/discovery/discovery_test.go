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
	endpointPRM http.HandlerFunc // the endpoint's own metadata; nil => 404
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
	mux.HandleFunc("/.well-known/oauth-protected-resource/", func(w http.ResponseWriter, r *http.Request) {
		if f.endpointPRM == nil {
			http.NotFound(w, r)
			return
		}
		f.endpointPRM(w, r)
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

// hostProto is a poppy agent served by the fixture itself, so its endpoint
// metadata can be fetched.
func hostProto(h string) map[string]string {
	return map[string]string{"type": "poppy", "endpoint": "https://" + h + "/poppy", "access": "signed_in"}
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
		poppy: func(h string) string { return doc(h, hostProto(h)) },
		prm:   prmFor("", `"authorization_servers":["https://idp.example/tenant","https://other"]`),
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

// prmFor answers protected resource metadata whose resource is https://<host>
// (read from the request) unless resource is non-empty, with extra JSON members.
func prmFor(resource, members string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res := resource
		if res == "" {
			res = "https://" + r.Host
		}
		_, _ = w.Write([]byte(`{"resource":"` + res + `",` + members + `}`))
	}
}

func authDoc(host string, auth map[string]any) string {
	b, _ := json.Marshal(map[string]any{
		"organization": map[string]string{"name": "Wrenmoor Energy", "domain": host},
		"agent":        map[string]any{"protocols": []map[string]string{poppyProto()}},
		"auth":         auth,
	})
	return string(b)
}

func TestDiscoverReadsAuthDisabledAndScopes(t *testing.T) {
	srv, host := serve(t, fixture{poppy: func(h string) string {
		return authDoc(h, map[string]any{"disabled": true, "scopes": map[string]string{"claims": "Read your claims"}})
	}})
	c, err := Discover(context.Background(), srv.Client(), host)
	if err != nil {
		t.Fatal(err)
	}
	if !c.SignInDisabled || c.Scopes["claims"] != "Read your claims" || len(c.Scopes) != 1 {
		t.Fatalf("unexpected company %+v", c)
	}
}

func TestDiscoverReadsScopesSupported(t *testing.T) {
	srv, host := serve(t, fixture{
		poppy: func(h string) string { return doc(h, hostProto(h)) },
		prm:   prmFor("", `"authorization_servers":["https://idp.example/tenant"],"scopes_supported":["claims"]`),
	})
	c, err := Discover(context.Background(), srv.Client(), host)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.ScopesSupported) != 1 || c.ScopesSupported[0] != "claims" {
		t.Fatalf("unexpected scopes %+v", c.ScopesSupported)
	}
}

func TestDiscoverRejectsProtectedResourceForOtherResource(t *testing.T) {
	srv, host := serve(t, fixture{
		poppy: func(h string) string { return doc(h, hostProto(h)) },
		prm:   prmFor("https://elsewhere.example", `"authorization_servers":["https://idp.example/tenant"]`),
	})
	_, err := Discover(context.Background(), srv.Client(), host)
	if err == nil || !strings.Contains(err.Error(), "resource") {
		t.Fatalf("want a resource error, got %v", err)
	}
}

func TestDiscoverSetsAgentResourceOnlyWithIssuer(t *testing.T) {
	endpoint := func(h string) string {
		return doc(h, map[string]string{"type": "poppy", "endpoint": "https://" + h + "/poppy", "access": "signed_in"})
	}
	own := func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"resource":"https://` + r.Host + `/poppy"}`))
	}
	srv, host := serve(t, fixture{
		poppy: endpoint,
		prm:   prmFor("", `"authorization_servers":["https://idp.example/tenant"]`), endpointPRM: own,
	})
	c, err := Discover(context.Background(), srv.Client(), host)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Agents[0].Resource; got != "https://"+host+"/poppy" {
		t.Fatalf("want the endpoint's own resource, got %q", got)
	}
	// No issuer: the endpoint metadata is not consulted.
	srv, host = serve(t, fixture{poppy: endpoint, endpointPRM: own})
	c, err = Discover(context.Background(), srv.Client(), host)
	if err != nil {
		t.Fatal(err)
	}
	if c.Agents[0].Resource != "" {
		t.Fatalf("want no resource without an issuer, got %q", c.Agents[0].Resource)
	}
}

func TestEndpointResource404IsDefault(t *testing.T) {
	srv, host := serve(t, fixture{poppy: func(h string) string { return doc(h) }})
	got, err := EndpointResource(context.Background(), srv.Client(), "https://"+host+"/poppy")
	if err != nil || got != "" {
		t.Fatalf("want \"\" and nil, got %q, %v", got, err)
	}
}

func TestEndpointResourceMismatchIsAbsent(t *testing.T) {
	srv, host := serve(t, fixture{
		poppy: func(h string) string { return doc(h) },
		endpointPRM: func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"resource":"https://other.example/poppy"}`))
		},
	})
	got, err := EndpointResource(context.Background(), srv.Client(), "https://"+host+"/poppy")
	if err != nil || got != "" {
		t.Fatalf("want \"\" and nil, got %q, %v", got, err)
	}
}

func TestEndpointResourceNon200IsError(t *testing.T) {
	srv, host := serve(t, fixture{
		poppy:       func(h string) string { return doc(h) },
		endpointPRM: func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) },
	})
	if _, err := EndpointResource(context.Background(), srv.Client(), "https://"+host+"/poppy"); err == nil {
		t.Fatal("want an error")
	}
}
