package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AltairaLabs/omnia-demo/demos/demoidp"
	"github.com/golang-jwt/jwt/v5"
)

func serve(t *testing.T, name, agentURL, backendURL string) *httptest.Server {
	t.Helper()
	b, err := lookupBrand(name, member{})
	if err != nil {
		t.Fatal(err)
	}
	h, err := newServer(b, agentURL, backendURL, nil, endUserPlane{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func TestEveryBrandServesItsPagesAndTheSharedScripts(t *testing.T) {
	pages := map[string][]string{
		"larkmere-mutual":  {"/ui_kits/marketing/", "/ui_kits/portal/"},
		"wrenmoor-energy":  {"/ui_kits/marketing/", "/ui_kits/portal/", "/ui_kits/intranet/"},
		"halvern-partners": {"/ui_kits/marketing/", "/ui_kits/practice-desk/", "/ui_kits/design-system/", "/tokens/fonts.css", "/assets/larkmere-logo-lockup.svg"},
	}
	shared := []string{"/react-shim.js", "/vendor/react.production.min.js", "/vendor/react-dom.production.min.js"}
	for name, paths := range pages {
		srv := serve(t, name, "http://127.0.0.1:1", "")
		for _, p := range append(paths, shared...) {
			if resp, _ := get(t, srv.URL+p); resp.StatusCode != http.StatusOK {
				t.Errorf("%s %s: status %d", name, p, resp.StatusCode)
			}
		}
	}
}

func TestBrandsDoNotLeakIntoEachOther(t *testing.T) {
	srv := serve(t, "wrenmoor-energy", "http://127.0.0.1:1", "")
	if resp, _ := get(t, srv.URL+"/assets/logo-mark.svg"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Larkmere's logo served under Wrenmoor: status %d", resp.StatusCode)
	}
}

func TestRootRedirectIsRelative(t *testing.T) {
	srv := serve(t, "wrenmoor-energy", "http://127.0.0.1:1", "")
	resp, _ := get(t, srv.URL+"/")
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "ui_kits/marketing/" {
		t.Fatalf("root: %d -> %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestJSXIsJavaScript(t *testing.T) {
	srv := serve(t, "wrenmoor-energy", "http://127.0.0.1:1", "")
	resp, _ := get(t, srv.URL+"/ui_kits/portal/App.jsx")
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Fatalf("App.jsx content type %q", ct)
	}
}

func TestTextIsGzipped(t *testing.T) {
	srv := serve(t, "wrenmoor-energy", "http://127.0.0.1:1", "")
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/vendor/react-dom.production.min.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	// A bare Transport, so the client does not ask for and undo gzip itself.
	resp, err := (&http.Transport{DisableCompression: true}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("not gzip (encoding %q): %v", resp.Header.Get("Content-Encoding"), err)
	}
	body, _ := io.ReadAll(zr)
	if resp.Header.Get("Content-Encoding") != "gzip" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/javascript") || len(body) < 100000 {
		t.Fatalf("encoding %q type %q, %d bytes", resp.Header.Get("Content-Encoding"), resp.Header.Get("Content-Type"), len(body))
	}
	if resp.Header.Get("Cache-Control") == "" {
		t.Fatal("vendored script served without Cache-Control")
	}
	// A photo is not recompressed.
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/assets/photos/hero-moor.jpg", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp2, err := (&http.Transport{DisableCompression: true}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.Header.Get("Content-Encoding") != "" {
		t.Fatal("a jpeg was gzipped")
	}
}

func TestUnknownBrand(t *testing.T) {
	if _, err := lookupBrand("acme", member{}); err == nil {
		t.Fatal("an unknown site should be an error")
	}
}

func TestMemberOverride(t *testing.T) {
	b, err := lookupBrand("wrenmoor-energy", member{email: "someone@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if b.who.id != "40021187" || b.who.email != "someone@example.com" {
		t.Fatalf("override: %+v", b.who)
	}
}

// The proxy asserts the site's identity and drops whatever the browser said.
func TestAssistantProxyAssertsIdentity(t *testing.T) {
	var got http.Header
	var path string
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, path = r.Header.Clone(), r.URL.Path
	}))
	defer agent.Close()
	srv := serve(t, "wrenmoor-energy", agent.URL, "")

	req, _ := http.NewRequest(http.MethodGet, srv.URL+assistantPath, nil)
	req.Header.Set("x-user-id", "someone-else")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if path != "/ws" || got.Get("x-user-id") != "40021187" || got.Get("x-user-roles") != "customer" {
		t.Fatalf("path %q, headers %v", path, got)
	}
}

func TestAssistantRefusesCrossOrigin(t *testing.T) {
	srv := serve(t, "wrenmoor-energy", "http://127.0.0.1:1", "")
	req, _ := http.NewRequest(http.MethodGet, srv.URL+assistantPath, nil)
	req.Header.Set("Origin", "https://elsewhere.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin: status %d", resp.StatusCode)
	}
}

func TestAccountProxy(t *testing.T) {
	var gotPath, gotUser, gotMethod, gotEmail, gotCookie string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUser, gotMethod = r.URL.Path, r.Header.Get("X-Omnia-User-Id"), r.Method
		gotEmail, gotCookie = r.Header.Get("X-Omnia-User-Email"), r.Header.Get("Cookie")
		_, _ = io.WriteString(w, "{}")
	}))
	defer backend.Close()
	srv := serve(t, "wrenmoor-energy", "http://127.0.0.1:1", backend.URL)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+consentPath, strings.NewReader(`{"tariff":"moor-flex"}`))
	req.Header.Set("X-Omnia-User-Id", "someone-else")
	req.Header.Set("X-Omnia-User-Email", "someone@else.example")
	req.Header.Set("Cookie", "session=operator")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if gotPath != "/wren/consent" || gotUser != "40021187" || gotMethod != http.MethodPost {
		t.Fatalf("consent forwarded as %s %s user %q", gotMethod, gotPath, gotUser)
	}
	if gotEmail != "" || gotCookie != "" {
		t.Fatalf("browser identity passed through: email %q cookie %q", gotEmail, gotCookie)
	}

	if resp, _ := get(t, srv.URL+receiptsPath+"?session=s1"); resp.StatusCode != http.StatusOK || gotPath != "/wren/receipts" {
		t.Fatalf("receipts: status %d path %q", resp.StatusCode, gotPath)
	}
	if resp, _ := get(t, srv.URL+consentPath); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET consent: status %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPost, srv.URL+consentPath, strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://elsewhere.example")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin consent: status %d", resp.StatusCode)
	}
}

func TestAssistantDropsCookie(t *testing.T) {
	var gotCookie, gotUser string
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie, gotUser = r.Header.Get("Cookie"), r.Header.Get("x-user-id")
	}))
	defer agent.Close()
	srv := serve(t, "wrenmoor-energy", agent.URL, "")
	req, _ := http.NewRequest(http.MethodGet, srv.URL+assistantPath, nil)
	req.Header.Set("Cookie", "session=operator")
	req.Header.Set("x-user-id", "someone-else")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if gotCookie != "" || gotUser == "someone-else" || gotUser == "" {
		t.Fatalf("assistant forwarded cookie %q user %q", gotCookie, gotUser)
	}
}

// With an issuer, every assistant connection carries a token for the member
// the site asserts (the browser's own Authorization dropped), and the site
// serves the issuer's JWKS for the install to trust.
func TestAssistantForwardsMemberToken(t *testing.T) {
	var gotAuth string
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
	}))
	defer agent.Close()
	iss, err := demoidp.New("http://larkmere-portal.test:8080")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := lookupBrand("larkmere-mutual", member{})
	h, err := newServer(b, agent.URL, "", iss, endUserPlane{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+assistantPath, nil)
	req.Header.Set("Authorization", "Bearer from-the-browser")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	tok, ok := strings.CutPrefix(gotAuth, "Bearer ")
	if !ok || tok == "from-the-browser" || strings.Count(tok, ".") != 2 {
		t.Fatalf("Authorization forwarded: %q", gotAuth)
	}
	if resp, body := get(t, srv.URL+demoidp.JWKSPath); resp.StatusCode != http.StatusOK || !strings.Contains(body, `"RS256"`) {
		t.Fatalf("jwks: %d %s", resp.StatusCode, body)
	}
}

// myDataSite serves Larkmere with My data proxied to mgmt, returning the site
// and what mgmt last received.
func myDataSite(t *testing.T) (*httptest.Server, *http.Request) {
	t.Helper()
	got := &http.Request{}
	mgmt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = *r.Clone(r.Context())
		_, _ = io.WriteString(w, "{}")
	}))
	t.Cleanup(mgmt.Close)
	iss, err := demoidp.New("http://larkmere-portal.test:8080")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := lookupBrand("larkmere-mutual", member{})
	h, err := newServer(b, "http://127.0.0.1:1", "", iss,
		endUserPlane{url: mgmt.URL, workspace: "larkmere-mutual", audience: "larkmere-members"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, got
}

func do(t *testing.T, method, url string, hdr map[string]string) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(`{}`))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// My data reaches the member's own end-user routes with a token for the
// member and the plane's audience; nothing of the browser's passes.
func TestMyDataForwardsAsTheMember(t *testing.T) {
	srv, got := myDataSite(t)
	code := do(t, http.MethodPut, srv.URL+myDataPrefix+"privacy/consent", map[string]string{
		"Authorization": "Bearer from-the-browser", "Cookie": "session=operator", "X-Omnia-User-Id": "someone-else",
	})
	if code != http.StatusOK || got.Method != http.MethodPut ||
		got.URL.Path != "/api/v1/me/workspaces/larkmere-mutual/privacy/consent" {
		t.Fatalf("status %d, forwarded %s %s", code, got.Method, got.URL.Path)
	}
	if got.Header.Get("Cookie") != "" || got.Header.Get("X-Omnia-User-Id") != "" {
		t.Fatalf("browser identity passed: %v", got.Header)
	}
	tok, ok := strings.CutPrefix(got.Header.Get("Authorization"), "Bearer ")
	if !ok || tok == "from-the-browser" {
		t.Fatalf("Authorization %q", got.Header.Get("Authorization"))
	}
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(tok, claims); err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "4471-889" || claims["aud"] != "larkmere-members" {
		t.Fatalf("token claims %v", claims)
	}
}

// Memories pass only their paging and type filters; nothing names a subject.
func TestMyDataMemoryQuery(t *testing.T) {
	srv, got := myDataSite(t)
	if code := do(t, http.MethodGet, srv.URL+myDataPrefix+"memories?limit=5&virtual_user_id=someone-else", nil); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if got.URL.RawQuery != "limit=5" {
		t.Fatalf("query %q", got.URL.RawQuery)
	}
	if code := do(t, http.MethodDelete, srv.URL+myDataPrefix+"memories/abc-123", nil); code != http.StatusOK ||
		got.URL.Path != "/api/v1/me/workspaces/larkmere-mutual/memories/abc-123" {
		t.Fatalf("forget: status %d path %q", code, got.URL.Path)
	}
}

// Only My data's routes pass: not the plane's MCP endpoint or opt-outs, not a
// method a route does not take, not a cross-origin write.
func TestMyDataRefusals(t *testing.T) {
	srv, _ := myDataSite(t)
	for _, c := range []struct {
		method, path string
		hdr          map[string]string
		want         int
	}{
		{http.MethodPost, "mcp", nil, http.StatusNotFound},
		{http.MethodPost, "privacy/opt-out", nil, http.StatusNotFound},
		{http.MethodDelete, "privacy/consent", nil, http.StatusNotFound},
		{http.MethodGet, "memories/abc", nil, http.StatusNotFound},
		{http.MethodDelete, "memories/a%2Fb", nil, http.StatusNotFound},
		{http.MethodPost, "privacy/erasure", map[string]string{"Origin": "https://elsewhere.example"}, http.StatusForbidden},
	} {
		if code := do(t, c.method, srv.URL+myDataPrefix+c.path, c.hdr); code != c.want {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, code, c.want)
		}
	}
}

// The issuer's discovery document is served beside its JWKS.
func TestDiscoveryServed(t *testing.T) {
	srv, _ := myDataSite(t)
	if resp, body := get(t, srv.URL+demoidp.DiscoveryPath); resp.StatusCode != http.StatusOK ||
		!strings.Contains(body, `"jwks_uri":"http://larkmere-portal.test:8080/.well-known/jwks.json"`) {
		t.Fatalf("discovery: %d %s", resp.StatusCode, body)
	}
}

func TestMyDataNeedsIssuer(t *testing.T) {
	b, _ := lookupBrand("larkmere-mutual", member{})
	if _, err := newServer(b, "http://127.0.0.1:1", "", nil,
		endUserPlane{url: "http://mgmt", workspace: "w", audience: "a"}); err == nil {
		t.Fatal("My data without an issuer would forward no credential")
	}
}

func TestAccountRoutesOffWithoutBackend(t *testing.T) {
	srv := serve(t, "larkmere-mutual", "http://127.0.0.1:1", "")
	if resp, _ := get(t, srv.URL+receiptsPath); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("receipts without a backend: status %d", resp.StatusCode)
	}
}
