// Package discovery finds a company's Personal Agent Protocol (PAP 1.0, §3)
// agents and, if it has one, its token issuer.
package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const (
	poppyPath   = "/.well-known/poppy.json"
	prmPath     = "/.well-known/oauth-protected-resource"
	maxBodySize = 1 << 20 // 1 MiB
)

// ErrUnreachable marks a discovery that failed in a way that may pass: a
// network error (no answer, a timeout, a TLS failure) or a 5xx. Every other
// failure (a 404 or other 4xx, a document that isn't one, a domain that
// doesn't match, a refused redirect) says the domain has no Poppy agent.
var ErrUnreachable = errors.New("unreachable")

// errRedirect is a redirect fetchPoppy refused. It reaches the caller as a
// network error from the client, but is the domain's answer, not a failure.
var errRedirect = errors.New("redirect refused")

// unreachable wraps err from fetching path as ErrUnreachable, unless it is a
// refused redirect.
func unreachable(path string, err error) error {
	if errors.Is(err, errRedirect) {
		return fmt.Errorf("fetching %s: %w", path, err)
	}
	return fmt.Errorf("fetching %s: %w: %w", path, ErrUnreachable, err)
}

// statusError is a non-200 answer for path; a 5xx is ErrUnreachable.
func statusError(path string, status int) error {
	if status >= 500 {
		return fmt.Errorf("fetching %s: %w: status %d", path, ErrUnreachable, status)
	}
	return fmt.Errorf("fetching %s: status %d", path, status)
}

// Company is what discovery learned about a domain.
type Company struct {
	Name, Domain string
	// Identifier is the issuer when the company has one, else https://<domain>.
	// It is the Session JWT aud.
	Identifier string
	Issuer     string
	Agents     []AgentEntry
	// SignInDisabled is poppy.json auth.disabled (PAP §3.2): the company
	// offers no sign-in, whatever its agents' access says.
	SignInDisabled bool
	// Scopes is poppy.json auth.scopes: scope name to description.
	Scopes map[string]string
	// ScopesSupported is the protected resource metadata's scopes_supported.
	ScopesSupported []string
}

// AgentEntry is one poppy protocol entry. Access is "session" or "signed_in".
//
// Resource is the RFC 9728 resource of the endpoint's own metadata, or "" when
// it has none or names a different resource (RFC 9728 §3.3). A token must not
// be sent to an endpoint with a resource of its own.
type AgentEntry struct{ Endpoint, Description, Access, Resource string }

type poppyDoc struct {
	Organization struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
	} `json:"organization"`
	Auth struct {
		Disabled bool              `json:"disabled"`
		Scopes   map[string]string `json:"scopes"`
	} `json:"auth"`
	Agent struct {
		Protocols []struct {
			Type        string `json:"type"`
			Endpoint    string `json:"endpoint"`
			Description string `json:"description"`
			Access      string `json:"access"`
		} `json:"protocols"`
	} `json:"agent"`
}

// Discover fetches https://<domain>/.well-known/poppy.json and resolves the
// issuer from protected resource metadata. domain is host[:port]. The client
// passed in is not modified.
func Discover(ctx context.Context, hc *http.Client, domain string) (*Company, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	var doc poppyDoc
	if err := fetchPoppy(ctx, hc, domain, &doc); err != nil {
		return nil, err
	}
	if doc.Organization.Domain != domain {
		return nil, fmt.Errorf("organization.domain %q does not match %q", doc.Organization.Domain, domain)
	}
	c := &Company{Name: doc.Organization.Name, Domain: domain, SignInDisabled: doc.Auth.Disabled, Scopes: doc.Auth.Scopes}
	for _, p := range doc.Agent.Protocols {
		if p.Type != "poppy" {
			continue
		}
		u, err := url.Parse(p.Endpoint)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			continue
		}
		access := p.Access
		if access != "session" && access != "signed_in" {
			access = "session"
		}
		c.Agents = append(c.Agents, AgentEntry{Endpoint: p.Endpoint, Description: p.Description, Access: access})
	}
	issuer, scopes, err := fetchIssuer(ctx, hc, domain)
	if err != nil {
		return nil, err
	}
	c.Issuer = issuer
	c.ScopesSupported = scopes
	if issuer != "" {
		for i := range c.Agents {
			if c.Agents[i].Resource, err = EndpointResource(ctx, hc, c.Agents[i].Endpoint); err != nil {
				return nil, err
			}
		}
	}
	c.Identifier = issuer
	if issuer == "" {
		c.Identifier = "https://" + domain
	}
	return c, nil
}

func fetchPoppy(ctx context.Context, hc *http.Client, domain string, out *poppyDoc) error {
	client := *hc
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("%w: too many redirects fetching %s", errRedirect, poppyPath)
		}
		if req.URL.Scheme != "https" {
			return fmt.Errorf("%w: to non-https URL %q", errRedirect, req.URL)
		}
		if req.URL.Path != poppyPath || req.URL.RawQuery != "" {
			return fmt.Errorf("%w: to %q: final URL must be exactly %s with no query", errRedirect, req.URL, poppyPath)
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+domain+poppyPath, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return unreachable(poppyPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError(poppyPath, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodySize)).Decode(out); err != nil {
		return fmt.Errorf("decoding %s: %w", poppyPath, err)
	}
	return nil
}

// noRedirect returns a copy of hc that does not follow redirects.
func noRedirect(hc *http.Client) *http.Client {
	client := *hc
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}

type prmDoc struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

// getPRM fetches protected resource metadata at path on host. found is false
// for 404. Redirects are not followed.
func getPRM(ctx context.Context, hc *http.Client, host, path string) (doc prmDoc, found bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+path, nil)
	if err != nil {
		return doc, false, err
	}
	resp, err := noRedirect(hc).Do(req)
	if err != nil {
		return doc, false, unreachable(path, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return doc, false, nil
	case http.StatusOK:
	default:
		return doc, false, statusError(path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodySize)).Decode(&doc); err != nil {
		return doc, false, fmt.Errorf("decoding %s: %w", path, err)
	}
	return doc, true, nil
}

// fetchIssuer returns "" for 404 (no issuer). Any other non-200 is an error,
// with no fallback, and redirects are not followed. The metadata's resource
// must be https://<domain>, else it describes some other resource.
func fetchIssuer(ctx context.Context, hc *http.Client, domain string) (string, []string, error) {
	prm, found, err := getPRM(ctx, hc, domain, prmPath)
	if err != nil || !found {
		return "", nil, err
	}
	if prm.Resource != "https://"+domain {
		return "", nil, fmt.Errorf("%s resource %q is not https://%s", prmPath, prm.Resource, domain)
	}
	if len(prm.AuthorizationServers) == 0 || prm.AuthorizationServers[0] == "" {
		return "", nil, fmt.Errorf("%s has no authorization_servers", prmPath)
	}
	return prm.AuthorizationServers[0], prm.ScopesSupported, nil
}

// EndpointResource returns the RFC 9728 resource the endpoint's own metadata
// (https://<host>/.well-known/oauth-protected-resource<path>) declares, or ""
// when it answers 404 or its resource is not the endpoint URL (§3.3). Any
// other non-200 is an error. Redirects are not followed.
func EndpointResource(ctx context.Context, hc *http.Client, endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("endpoint %q is not a URL", endpoint)
	}
	prm, found, err := getPRM(ctx, hc, u.Host, prmPath+u.Path)
	if err != nil || !found || prm.Resource != endpoint {
		return "", err
	}
	return prm.Resource, nil
}
