package signin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testDomain = "larkmere.example"

// goodMetadata is issuer metadata that passes every check for issuer and
// testDomain.
func goodMetadata(issuer string) map[string]any {
	return map[string]any{
		"issuer":                           issuer,
		"authorization_endpoint":           issuer + "/authorize",
		"token_endpoint":                   issuer + "/token",
		"revocation_endpoint":              issuer + "/revoke",
		"grant_types_supported":            []string{"authorization_code", "refresh_token"},
		"scopes_supported":                 []string{"policies:read"},
		"code_challenge_methods_supported": []string{"S256"},
		"protected_resources":              []string{"https://" + testDomain, "https://mcp.larkmere.example/mcp"},
	}
}

// fakeIssuer is a TLS issuer at srv.URL + "/tenant" whose metadata is
// served at the RFC 8414 §3.1 path. edit adjusts the good document; serve,
// when set, replaces the handler for the metadata path.
type fakeIssuer struct {
	srv    *httptest.Server
	issuer string
	paths  []string
}

func newFakeIssuer(t *testing.T, edit func(doc map[string]any), serve http.HandlerFunc) *fakeIssuer {
	t.Helper()
	f := &fakeIssuer{}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.paths = append(f.paths, r.URL.Path)
		if r.URL.Path != "/.well-known/oauth-authorization-server/tenant" {
			http.NotFound(w, r)
			return
		}
		if serve != nil {
			serve(w, r)
			return
		}
		doc := goodMetadata(f.issuer)
		if edit != nil {
			edit(doc)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(f.srv.Close)
	f.issuer = f.srv.URL + "/tenant"
	return f
}

func TestFetchMetadataPathIssuer(t *testing.T) {
	f := newFakeIssuer(t, nil, nil)
	md, err := FetchMetadata(context.Background(), f.srv.Client(), f.issuer, testDomain)
	if err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
	if len(f.paths) != 1 || f.paths[0] != "/.well-known/oauth-authorization-server/tenant" {
		t.Errorf("fetched %v, want only /.well-known/oauth-authorization-server/tenant", f.paths)
	}
	if md.Issuer != f.issuer {
		t.Errorf("Issuer = %q, want %q", md.Issuer, f.issuer)
	}
	if md.TokenEndpoint != f.issuer+"/token" || md.AuthorizationEndpoint != f.issuer+"/authorize" ||
		md.RevocationEndpoint != f.issuer+"/revoke" {
		t.Errorf("endpoints = %+v", md)
	}
	if strings.Join(md.ScopesSupported, " ") != "policies:read" {
		t.Errorf("ScopesSupported = %v", md.ScopesSupported)
	}
	if strings.Join(md.CodeChallengeMethods, " ") != "S256" {
		t.Errorf("CodeChallengeMethods = %v", md.CodeChallengeMethods)
	}
	if strings.Join(md.GrantTypes, " ") != "authorization_code refresh_token" {
		t.Errorf("GrantTypes = %v", md.GrantTypes)
	}
	if md.RequirePAR {
		t.Error("RequirePAR = true")
	}
}

func TestFetchMetadataRejects(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(doc map[string]any)
		serve http.HandlerFunc
		field string
	}{
		{
			name:  "issuer with a trailing slash",
			edit:  func(doc map[string]any) { doc["issuer"] = doc["issuer"].(string) + "/" },
			field: "issuer",
		},
		{
			name:  "protected_resources missing the domain",
			edit:  func(doc map[string]any) { doc["protected_resources"] = []string{"https://wrenmoor.example"} },
			field: "protected_resources",
		},
		{
			name:  "protected_resources with the domain and a path",
			edit:  func(doc map[string]any) { doc["protected_resources"] = []string{"https://" + testDomain + "/api"} },
			field: "protected_resources",
		},
		{
			name:  "no authorization_code grant",
			edit:  func(doc map[string]any) { doc["grant_types_supported"] = []string{"refresh_token"} },
			field: "grant_types_supported",
		},
		{
			name:  "no refresh_token grant",
			edit:  func(doc map[string]any) { doc["grant_types_supported"] = []string{"authorization_code"} },
			field: "grant_types_supported",
		},
		{
			name:  "plain PKCE only",
			edit:  func(doc map[string]any) { doc["code_challenge_methods_supported"] = []string{"plain"} },
			field: "code_challenge_methods_supported",
		},
		{
			name:  "PAR required",
			edit:  func(doc map[string]any) { doc["require_pushed_authorization_requests"] = true },
			field: "require_pushed_authorization_requests",
		},
		{
			name:  "http token endpoint",
			edit:  func(doc map[string]any) { doc["token_endpoint"] = "http://auth.larkmere.example/token" },
			field: "token_endpoint",
		},
		{
			name:  "missing authorization endpoint",
			edit:  func(doc map[string]any) { delete(doc, "authorization_endpoint") },
			field: "authorization_endpoint",
		},
		{
			name: "302 to another path",
			serve: func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/.well-known/elsewhere", http.StatusFound)
			},
			field: "redirect",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeIssuer(t, tc.edit, tc.serve)
			md, err := FetchMetadata(context.Background(), f.srv.Client(), f.issuer, testDomain)
			if err == nil {
				t.Fatalf("FetchMetadata accepted it: %+v", md)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Errorf("error %q does not name %s", err, tc.field)
			}
			if len(f.paths) != 1 {
				t.Errorf("fetched %v, want exactly one request (no redirect followed)", f.paths)
			}
		})
	}
}

func TestFetchMetadataRejectsHTTPIssuer(t *testing.T) {
	if _, err := FetchMetadata(context.Background(), http.DefaultClient, "http://auth.larkmere.example", testDomain); err == nil {
		t.Error("FetchMetadata accepted an http issuer")
	}
}

func TestFetchMetadataHostCaseInsensitive(t *testing.T) {
	f := newFakeIssuer(t, func(doc map[string]any) {
		doc["protected_resources"] = []string{"https://LARKMERE.example"}
	}, nil)
	if _, err := FetchMetadata(context.Background(), f.srv.Client(), f.issuer, "larkmere.example"); err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
}

func TestFetchMetadataWithoutPKCEMethodsIsAccepted(t *testing.T) {
	f := newFakeIssuer(t, func(doc map[string]any) { delete(doc, "code_challenge_methods_supported") }, nil)
	if _, err := FetchMetadata(context.Background(), f.srv.Client(), f.issuer, testDomain); err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
}
