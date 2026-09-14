package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Doc is a SharePoint document reference (returned by /list, used for seeding).
type Doc struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Site    string `json:"site"`
	Summary string `json:"summary"`
	// Author and ModifiedAt come from Graph's lastModifiedBy /
	// lastModifiedDateTime and go onto the document node as citation
	// metadata; modified_at is what recency decay tracks.
	Author     string     `json:"author,omitempty"`
	ModifiedAt *time.Time `json:"modified_at,omitempty"`
}

// DocContent is a fetched document (returned by /fetch). Text is the flat
// view the agent's fetch tool reads; Sections is the structure the seeder
// hands to memory-api.
type DocContent struct {
	Title      string     `json:"title"`
	URL        string     `json:"url"`
	Text       string     `json:"text"`
	Sections   []Section  `json:"sections,omitempty"`
	Author     string     `json:"author,omitempty"`
	ModifiedAt *time.Time `json:"modified_at,omitempty"`
}

// TokenSource returns a Graph bearer token.
type TokenSource func(ctx context.Context) (string, error)

// GraphError carries the upstream Graph HTTP status so the server can pass it
// through (the governance beat relies on a restricted-site 403 surfacing).
//
// URL is carried so a failure can be diagnosed from the adapter's own logs.
// Without it the only copy of Graph's explanation went to the agent, which
// paraphrased it into prose, and the log kept a bare status code.
type GraphError struct {
	StatusCode int
	URL        string
	Body       string
}

func (e *GraphError) Error() string {
	return fmt.Sprintf("graph request failed: status=%d url=%s body=%s", e.StatusCode, e.URL, e.Body)
}

// GraphClient talks to Microsoft Graph for a single SharePoint site.
type GraphClient struct {
	baseURL string
	siteID  string
	http    *http.Client
	token   TokenSource
}

// NewGraphClient builds a client. baseURL/httpClient default when empty/nil.
func NewGraphClient(baseURL, siteID string, token TokenSource, httpClient *http.Client) *GraphClient {
	if baseURL == "" {
		baseURL = defaultGraphBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &GraphClient{baseURL: strings.TrimRight(baseURL, "/"), siteID: siteID, token: token, http: httpClient}
}

func (g *GraphClient) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	tok, err := g.token(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (g *GraphClient) doJSON(ctx context.Context, url string, out any) error {
	data, err := g.doRawBytes(ctx, url)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (g *GraphClient) doRawBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, &GraphError{StatusCode: resp.StatusCode, URL: url, Body: string(data)}
	}
	return data, nil
}

// driveChild is one entry of the site drive's root listing. `id` is the
// driveItem id, which is what every app-only content read is addressed by.
type driveChild struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	WebURL string `json:"webUrl"`
	File   *struct {
		MimeType string `json:"mimeType"`
	} `json:"file"`
	LastModifiedDateTime string `json:"lastModifiedDateTime"`
	LastModifiedBy       *struct {
		User struct {
			DisplayName string `json:"displayName"`
		} `json:"user"`
	} `json:"lastModifiedBy"`
}

// author is the last modifier's display name, empty when Graph sent none.
func (c *driveChild) author() string {
	if c.LastModifiedBy == nil {
		return ""
	}
	return c.LastModifiedBy.User.DisplayName
}

// modifiedAt parses lastModifiedDateTime; nil when absent or unparseable.
func (c *driveChild) modifiedAt() *time.Time {
	if c.LastModifiedDateTime == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, c.LastModifiedDateTime)
	if err != nil {
		return nil
	}
	return &t
}

// listChildren enumerates the site's default drive root, folders included.
//
// This endpoint is the reason both List and Fetch go through it: it is one of
// the driveItem routes that accepts an APPLICATION token. Sharing-link
// resolution (/shares/{encoded-url}) does not, and answers app-only callers
// with 403 accessDenied — "The sharing link no longer exists, or you do not
// have permission to access it" — whatever Sites.Read.All grants say.
func (g *GraphClient) listChildren(ctx context.Context) ([]driveChild, error) {
	u := fmt.Sprintf("%s/sites/%s/drive/root/children", g.baseURL, g.siteID)
	var resp struct {
		Value []driveChild `json:"value"`
	}
	if err := g.doJSON(ctx, u, &resp); err != nil {
		return nil, err
	}
	return resp.Value, nil
}

// List enumerates documents in the site's default drive root (folders skipped).
func (g *GraphClient) List(ctx context.Context) ([]Doc, error) {
	children, err := g.listChildren(ctx)
	if err != nil {
		return nil, err
	}
	docs := make([]Doc, 0, len(children))
	for _, item := range children {
		if item.File == nil { // skip folders
			continue
		}
		docs = append(docs, Doc{
			Title:      item.Name,
			URL:        item.WebURL,
			Site:       g.siteID,
			Summary:    item.Name, // demo: summary == title
			Author:     item.author(),
			ModifiedAt: item.modifiedAt(),
		})
	}
	return docs, nil
}

// fileNameFromURL pulls a document's file name out of a SharePoint URL.
//
// SharePoint hands out two shapes and only one carries the name in the path:
// a direct path URL (.../Shared Documents/policy.docx) and a Doc.aspx viewer
// URL, whose path is identical for every document in the site and whose name
// lives in the `file` query parameter. Comparing viewer URLs on path alone
// would match every document against every other.
func fileNameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if f := u.Query().Get("file"); f != "" {
		return f
	}
	name := path.Base(u.Path)
	if name == "." || name == "/" {
		return ""
	}
	if decoded, err := url.PathUnescape(name); err == nil {
		return decoded
	}
	return name
}

// resolveItem finds the drive item a webURL refers to.
//
// Exact webUrl first, because that is what List handed the caller and what the
// seeded corpus stores. File name is the fallback, for a URL that has been
// round-tripped through a model or an index and lost its query ordering.
func (g *GraphClient) resolveItem(ctx context.Context, webURL string) (*driveChild, error) {
	children, err := g.listChildren(ctx)
	if err != nil {
		return nil, err
	}
	for i := range children {
		if children[i].File != nil && children[i].WebURL == webURL {
			return &children[i], nil
		}
	}
	if name := fileNameFromURL(webURL); name != "" {
		for i := range children {
			if children[i].File != nil && strings.EqualFold(children[i].Name, name) {
				return &children[i], nil
			}
		}
	}
	// 404 rather than a bare error so the agent is told the document is not in
	// this site, which is a different fact from being refused access to it.
	return nil, &GraphError{
		StatusCode: http.StatusNotFound,
		URL:        webURL,
		Body:       fmt.Sprintf("no document in this site matches %q", webURL),
	}
}

// Fetch resolves a document URL to its driveItem and returns the content as
// extracted text plus sections. OOXML files (.docx, .pptx, .xlsx) are
// converted via extractDocument; all other types pass through as-is.
func (g *GraphClient) Fetch(ctx context.Context, webURL string) (*DocContent, error) {
	item, err := g.resolveItem(ctx, webURL)
	if err != nil {
		return nil, err
	}
	raw, err := g.doRawBytes(ctx, fmt.Sprintf("%s/sites/%s/drive/items/%s/content", g.baseURL, g.siteID, item.ID))
	if err != nil {
		return nil, err
	}
	ex, err := extractDocument(item.Name, raw)
	if err != nil {
		return nil, fmt.Errorf("extract %q: %w", item.Name, err)
	}
	return &DocContent{
		Title: item.Name, URL: item.WebURL, Text: ex.Text, Sections: ex.Sections,
		Author: item.author(), ModifiedAt: item.modifiedAt(),
	}, nil
}
