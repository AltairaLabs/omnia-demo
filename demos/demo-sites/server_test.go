package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, name, agentURL, backendURL string) *httptest.Server {
	t.Helper()
	b, err := lookupBrand(name, member{})
	if err != nil {
		t.Fatal(err)
	}
	h, err := newServer(b, agentURL, backendURL)
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
	var gotPath, gotUser, gotMethod string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUser, gotMethod = r.URL.Path, r.Header.Get("X-Omnia-User-Id"), r.Method
		_, _ = io.WriteString(w, "{}")
	}))
	defer backend.Close()
	srv := serve(t, "wrenmoor-energy", "http://127.0.0.1:1", backend.URL)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+consentPath, strings.NewReader(`{"tariff":"moor-flex"}`))
	req.Header.Set("X-Omnia-User-Id", "someone-else")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if gotPath != "/wren/consent" || gotUser != "40021187" || gotMethod != http.MethodPost {
		t.Fatalf("consent forwarded as %s %s user %q", gotMethod, gotPath, gotUser)
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

func TestAccountRoutesOffWithoutBackend(t *testing.T) {
	srv := serve(t, "larkmere-mutual", "http://127.0.0.1:1", "")
	if resp, _ := get(t, srv.URL+receiptsPath); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("receipts without a backend: status %d", resp.StatusCode)
	}
}
