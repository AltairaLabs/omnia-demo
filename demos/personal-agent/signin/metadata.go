// Package signin is the Personal Agent's side of PAP Direct Sign-In (PAP
// 1.0 §4.2, https://personalagentprotocol.org/docs/1.0/spec), written from
// the spec: checking a Company's issuer metadata (§3.2) before anything is
// sent to it, and keeping the tokens a sign-in grants sealed per Company and
// User (tokens.go).
package signin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const (
	metadataWellKnown = "/.well-known/oauth-authorization-server"
	maxMetadataSize   = 1 << 20 // 1 MiB
)

// Metadata is the part of a Company's OAuth Authorization Server Metadata
// (RFC 8414) that Direct Sign-In uses, after FetchMetadata has checked it.
type Metadata struct {
	Issuer, AuthorizationEndpoint, TokenEndpoint, RevocationEndpoint string
	GrantTypes, ScopesSupported, CodeChallengeMethods                []string
	RequirePAR                                                       bool
}

type metadataDoc struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RevocationEndpoint    string   `json:"revocation_endpoint"`
	GrantTypes            []string `json:"grant_types_supported"`
	ScopesSupported       []string `json:"scopes_supported"`
	CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
	RequirePAR            bool     `json:"require_pushed_authorization_requests"`
	ProtectedResources    []string `json:"protected_resources"`
}

// FetchMetadata fetches the metadata of issuer, the issuer discovery found
// for domain, and checks it before the Personal Agent trusts it (PAP §3.2):
//
//   - it is fetched over HTTPS at the RFC 8414 §3.1 URL, the well-known
//     path inserted between the issuer's host and path, with no redirect
//     followed and at most 1 MiB read;
//   - its issuer equals the argument character for character;
//   - its protected_resources lists https://<domain>, compared with the
//     host lowercased (RFC 9728 §3.3, RFC 3986 §6.2.2.1). Without this,
//     any domain could name another Company's issuer and receive its
//     tokens;
//   - Direct Sign-In is offered: grant_types_supported holds
//     authorization_code and refresh_token (§4.4), and the authorization
//     and token endpoints are HTTPS;
//   - S256 PKCE is supported when code_challenge_methods_supported is
//     listed, and Pushed Authorization Requests are not required, since
//     this agent does not send them.
//
// Each error names the field that failed.
func FetchMetadata(ctx context.Context, hc *http.Client, issuer, domain string) (*Metadata, error) {
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, fmt.Errorf("signin: issuer %q must be an https URL with no query or fragment", issuer)
	}
	if domain == "" {
		return nil, errors.New("signin: no domain to check protected_resources against")
	}
	// RFC 8414 §3.1: drop a terminating "/" from the issuer's path, then
	// insert the well-known path between the host and the path.
	wellKnown := "https://" + u.Host + metadataWellKnown + strings.TrimSuffix(u.EscapedPath(), "/")

	doc, err := getMetadata(ctx, hc, wellKnown)
	if err != nil {
		return nil, err
	}
	if err := checkMetadata(doc, issuer, domain); err != nil {
		return nil, &invalidMetadataError{err: err}
	}
	return &Metadata{
		Issuer:                doc.Issuer,
		AuthorizationEndpoint: doc.AuthorizationEndpoint,
		TokenEndpoint:         doc.TokenEndpoint,
		RevocationEndpoint:    doc.RevocationEndpoint,
		GrantTypes:            doc.GrantTypes,
		ScopesSupported:       doc.ScopesSupported,
		CodeChallengeMethods:  doc.CodeChallengeMethods,
		RequirePAR:            doc.RequirePAR,
	}, nil
}

func getMetadata(ctx context.Context, hc *http.Client, wellKnown string) (*metadataDoc, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	client := *hc
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return nil, fmt.Errorf("signin: issuer metadata: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("signin: fetching %s: %w", wellKnown, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, fmt.Errorf("signin: fetching %s: refused a redirect (status %d) to %q", wellKnown, resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("signin: fetching %s: status %d", wellKnown, resp.StatusCode)
	}
	var doc metadataDoc
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxMetadataSize)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("signin: decoding %s: %w", wellKnown, err)
	}
	return &doc, nil
}

// invalidMetadataError is metadata the issuer served that fails checkMetadata:
// the issuer's answer, not a failure that may pass, unlike a network error, a
// 5xx or a 404. Its message is checkMetadata's.
type invalidMetadataError struct{ err error }

func (e *invalidMetadataError) Error() string { return e.err.Error() }
func (e *invalidMetadataError) Unwrap() error { return e.err }

func checkMetadata(doc *metadataDoc, issuer, domain string) error {
	switch {
	case doc.Issuer != issuer:
		return fmt.Errorf("signin: metadata issuer %q is not %q", doc.Issuer, issuer)
	case !listsDomain(doc.ProtectedResources, domain):
		return fmt.Errorf("signin: metadata protected_resources %q does not list https://%s", doc.ProtectedResources, domain)
	case !slices.Contains(doc.GrantTypes, "authorization_code"):
		return fmt.Errorf("signin: metadata grant_types_supported %q lacks authorization_code (Direct Sign-In)", doc.GrantTypes)
	case !slices.Contains(doc.GrantTypes, "refresh_token"):
		return fmt.Errorf("signin: metadata grant_types_supported %q lacks refresh_token", doc.GrantTypes)
	case !isHTTPS(doc.AuthorizationEndpoint):
		return fmt.Errorf("signin: metadata authorization_endpoint %q is not an https URL", doc.AuthorizationEndpoint)
	case !isHTTPS(doc.TokenEndpoint):
		return fmt.Errorf("signin: metadata token_endpoint %q is not an https URL", doc.TokenEndpoint)
	case doc.RevocationEndpoint != "" && !isHTTPS(doc.RevocationEndpoint):
		return fmt.Errorf("signin: metadata revocation_endpoint %q is not an https URL", doc.RevocationEndpoint)
	case doc.CodeChallengeMethods != nil && !slices.Contains(doc.CodeChallengeMethods, "S256"):
		return fmt.Errorf("signin: metadata code_challenge_methods_supported %q lacks S256", doc.CodeChallengeMethods)
	case doc.RequirePAR:
		return errors.New("signin: metadata require_pushed_authorization_requests is true, and Pushed Authorization Requests are not supported")
	}
	return nil
}

// listsDomain reports whether resources holds https://<domain> with no
// path, query or fragment, comparing hosts lowercased.
func listsDomain(resources []string, domain string) bool {
	want := strings.ToLower(domain)
	for _, r := range resources {
		u, err := url.Parse(r)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" {
			continue
		}
		if u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			continue
		}
		if strings.ToLower(u.Host) == want {
			return true
		}
	}
	return false
}

func isHTTPS(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != ""
}
