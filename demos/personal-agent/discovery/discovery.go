// Package discovery finds a company's Personal Agent Protocol (PAP 1.0, §3)
// agents and, if it has one, its token issuer.
package discovery

import (
	"context"
	"encoding/json"
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

// Company is what discovery learned about a domain.
type Company struct {
	Name, Domain string
	// Identifier is the issuer when the company has one, else https://<domain>.
	// It is the Session JWT aud.
	Identifier string
	Issuer     string
	Agents     []AgentEntry
}

// AgentEntry is one poppy protocol entry. Access is "session" or "signed_in".
type AgentEntry struct{ Endpoint, Description, Access string }

type poppyDoc struct {
	Organization struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
	} `json:"organization"`
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
	c := &Company{Name: doc.Organization.Name, Domain: domain}
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
	issuer, err := fetchIssuer(ctx, hc, domain)
	if err != nil {
		return nil, err
	}
	c.Issuer = issuer
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
			return fmt.Errorf("too many redirects fetching %s", poppyPath)
		}
		if req.URL.Scheme != "https" {
			return fmt.Errorf("redirect to non-https URL %q refused", req.URL)
		}
		if req.URL.Path != poppyPath || req.URL.RawQuery != "" {
			return fmt.Errorf("redirect to %q refused: final URL must be exactly %s with no query", req.URL, poppyPath)
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+domain+poppyPath, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", poppyPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: status %d", poppyPath, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodySize)).Decode(out); err != nil {
		return fmt.Errorf("decoding %s: %w", poppyPath, err)
	}
	return nil
}

// fetchIssuer returns "" for 404 (no issuer). Any other non-200 is an error,
// with no fallback, and redirects are not followed.
func fetchIssuer(ctx context.Context, hc *http.Client, domain string) (string, error) {
	client := *hc
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+domain+prmPath, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", prmPath, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return "", nil
	case http.StatusOK:
	default:
		return "", fmt.Errorf("fetching %s: status %d", prmPath, resp.StatusCode)
	}
	var prm struct {
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodySize)).Decode(&prm); err != nil {
		return "", fmt.Errorf("decoding %s: %w", prmPath, err)
	}
	if len(prm.AuthorizationServers) == 0 || prm.AuthorizationServers[0] == "" {
		return "", fmt.Errorf("%s has no authorization_servers", prmPath)
	}
	return prm.AuthorizationServers[0], nil
}
