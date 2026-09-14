package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// The seeder is the worked example of a memory populator: it speaks
// memory-api's batch ingest contract (POST /api/v1/institutional/ingest/batch)
// and nothing else. It holds no store credentials and never chunks — it sends
// documents with the structure it can see (sections), a content hash so an
// unchanged document is skipped, the source's own modified time and author,
// and a manifest of what the site still lists so deletions propagate. See
// docs/reference/memory-api and the "write a populator" guide.
const (
	// seedSource stamps every row this populator writes, so the whole corpus
	// can be listed (GET /institutional/documents?source=) or forgotten
	// (DELETE /institutional/sources/sharepoint) at once.
	seedSource = "sharepoint"
	// seedBatchSize is how many documents go in one request. The contract
	// allows 100; twenty keeps a request well under the body limit for
	// long documents and bounds what one failed request loses.
	seedBatchSize = 20
	// drainBatchesPerCall is the max_batches asked of POST /admin/reembed
	// per call, and maxDrainCalls bounds the calls, so a provider that keeps
	// reporting progress cannot spin the job forever.
	drainBatchesPerCall = 50
	maxDrainCalls       = 20
)

// Seeder fetches extracted documents and ingests them via the batch contract.
type Seeder struct {
	src         DocSource
	memoryURL   string
	workspaceID string
	http        *http.Client
	log         *slog.Logger
	// tokenPath is the projected ServiceAccount token memory-api requires.
	// Empty sends no Authorization header, which only works against a
	// memory-api with auth disabled.
	tokenPath string
}

// bearerToken reads the projected ServiceAccount token.
//
// READ PER REQUEST, not cached at startup. The kubelet refreshes a projected
// token in place well before expiry, so a value read once at process start is
// a value that expires mid-run on any job long enough to matter.
func (s *Seeder) bearerToken() (string, error) {
	if s.tokenPath == "" {
		return "", nil
	}
	b, err := os.ReadFile(s.tokenPath)
	if err != nil {
		return "", fmt.Errorf("read service account token %q: %w", s.tokenPath, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// The wire shapes mirror the public institutional-ingest HTTP contract;
// field names are the contract.

type batchDocument struct {
	Title       string     `json:"title"`
	URL         string     `json:"url"`
	Site        string     `json:"site,omitempty"`
	Text        string     `json:"text,omitempty"`
	Sections    []Section  `json:"sections,omitempty"`
	Author      string     `json:"author,omitempty"`
	ModifiedAt  *time.Time `json:"modified_at,omitempty"`
	ContentHash string     `json:"content_hash"`
}

type batchManifest struct {
	Kind string   `json:"kind"`
	Keys []string `json:"keys"`
}

type batchRequest struct {
	WorkspaceID string          `json:"workspace_id,omitempty"`
	Source      string          `json:"source"`
	Documents   []batchDocument `json:"documents"`
	Present     *batchManifest  `json:"present,omitempty"`
}

type batchResponse struct {
	Accepted  int   `json:"accepted"`
	Skipped   int   `json:"skipped"`
	Forgotten int64 `json:"forgotten"`
	Rejected  []struct {
		Key   string `json:"key"`
		Error string `json:"error"`
	} `json:"rejected"`
	EmbeddingBacklog int `json:"embedding_backlog"`
}

// Run lists documents, fetches each, and posts them in batches; the last
// batch carries the manifest of everything the site lists, so a document
// removed from SharePoint is forgotten from memory. Per-document Fetch
// failures are logged and skipped; a batch that memory-api refuses loses its
// documents for this run; only a List failure returns an error.
//
// Returns how many documents landed (accepted or skipped as unchanged) and
// how many were found, so the caller can tell "nothing to do" from
// "everything failed" — the same `seeded` number and very different events.
func (s *Seeder) Run(ctx context.Context) (seeded, found int, err error) {
	docs, err := s.src.List(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("list documents: %w", err)
	}
	if len(docs) == 0 {
		// No manifest on an empty listing: an empty site is legitimate, but
		// forgetting the whole corpus on one is not something a sync should do
		// by itself.
		return 0, 0, nil
	}
	present := &batchManifest{Kind: "document", Keys: make([]string, 0, len(docs))}
	var prepared []batchDocument
	for _, d := range docs {
		present.Keys = append(present.Keys, d.URL)
		bd, err := s.prepare(ctx, d)
		if err != nil {
			s.log.Warn("skipping document", "url", d.URL, "error", err)
			continue
		}
		prepared = append(prepared, bd)
	}
	for start := 0; start < len(prepared) || start == 0; start += seedBatchSize {
		end := min(start+seedBatchSize, len(prepared))
		batch := prepared[start:end]
		var manifest *batchManifest
		if end == len(prepared) {
			manifest = present
		}
		n, err := s.postBatch(ctx, batch, manifest)
		if err != nil {
			s.log.Warn("batch failed; its documents are not updated this run",
				"documents", len(batch), "error", err)
			continue
		}
		seeded += n
	}
	return seeded, len(docs), nil
}

// prepare fetches one document and shapes it for the contract.
func (s *Seeder) prepare(ctx context.Context, d Doc) (batchDocument, error) {
	content, err := s.src.Fetch(ctx, d.URL)
	if err != nil {
		return batchDocument{}, fmt.Errorf("fetch %q: %w", d.URL, err)
	}
	bd := batchDocument{
		Title: d.Title, URL: d.URL, Site: d.Site, Sections: content.Sections,
		Author: content.Author, ModifiedAt: content.ModifiedAt,
	}
	if bd.Author == "" {
		bd.Author = d.Author
	}
	if bd.ModifiedAt == nil {
		bd.ModifiedAt = d.ModifiedAt
	}
	if len(bd.Sections) == 0 {
		bd.Text = content.Text
	}
	bd.ContentHash = contentHash(bd)
	return bd, nil
}

// contentHash is sha256 over what memory-api will split (text or sections),
// so a document whose bytes changed but whose extracted content did not is
// still skipped, and a change in structure alone is still a change.
func contentHash(bd batchDocument) string {
	h := sha256.New()
	if len(bd.Sections) > 0 {
		enc, _ := json.Marshal(bd.Sections)
		h.Write(enc)
	} else {
		h.Write([]byte(bd.Text))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// postBatch sends one batch and returns how many of its documents landed
// (accepted + skipped). Rejections are logged per document.
func (s *Seeder) postBatch(ctx context.Context, docs []batchDocument, present *batchManifest) (int, error) {
	body := batchRequest{WorkspaceID: s.workspaceID, Source: seedSource, Documents: docs, Present: present}
	if body.Documents == nil {
		body.Documents = []batchDocument{}
	}
	data, err := s.postJSON(ctx, "/api/v1/institutional/ingest/batch", body, http.StatusAccepted)
	if err != nil {
		return 0, err
	}
	var resp batchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, fmt.Errorf("decode batch response: %w", err)
	}
	for _, r := range resp.Rejected {
		s.log.Warn("document rejected", "key", r.Key, "error", r.Error)
	}
	s.log.Info("batch ingested", "accepted", resp.Accepted, "skipped", resp.Skipped,
		"forgotten", resp.Forgotten, "rejected", len(resp.Rejected), "embeddingBacklog", resp.EmbeddingBacklog)
	return resp.Accepted + resp.Skipped, nil
}

// DrainEmbeddings embeds what was just ingested, instead of leaving it to the
// re-embed worker's next tick.
//
// Ingest returns 202 with the chunks stored and no vectors. Until they are
// embedded the corpus is present and unsearchable, so an agent asking a
// question straight after a seed gets nothing back and cannot tell that from an
// empty corpus. The worker's interval is an hour by default; this closes that
// window.
//
// Best effort: a failure here leaves the documents stored and the worker will
// still embed them on its own schedule, so it is logged and not returned as a
// seeding failure.
func (s *Seeder) DrainEmbeddings(ctx context.Context) {
	total := 0
	for range maxDrainCalls {
		r, err := s.reembed(ctx)
		if err != nil {
			s.log.Warn("embedding drain failed; the re-embed worker will catch up on its own schedule",
				"embedded", total, "error", err)
			return
		}
		total += r.Embedded
		if r.Embedded == 0 || (r.Remaining != nil && *r.Remaining == 0) {
			s.log.Info("embedding drain complete", "embedded", total)
			return
		}
	}
	s.log.Warn("embedding drain hit its call cap; the re-embed worker will finish the rest",
		"embedded", total, "calls", maxDrainCalls)
}

// reembedResponse mirrors memory-api's ReembedResponse.
type reembedResponse struct {
	Embedded  int    `json:"embedded"`
	Batches   int    `json:"batches"`
	Remaining *int   `json:"remaining,omitempty"`
	Error     string `json:"error,omitempty"`
}

// reembed asks memory-api to embed up to drainBatchesPerCall batches.
func (s *Seeder) reembed(ctx context.Context) (reembedResponse, error) {
	data, err := s.postJSON(ctx, "/admin/reembed", map[string]int{"max_batches": drainBatchesPerCall}, http.StatusOK)
	if err != nil {
		return reembedResponse{}, err
	}
	var out reembedResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return reembedResponse{}, fmt.Errorf("decode reembed response: %w", err)
	}
	return out, nil
}

// postJSON posts body to memory-api with the bearer token and returns the
// response body when the status is wantStatus.
func (s *Seeder) postJSON(ctx context.Context, path string, body any, wantStatus int) ([]byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(s.memoryURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	tok, err := s.bearerToken()
	if err != nil {
		return nil, err
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		return nil, fmt.Errorf("memory-api status=%d body=%s", resp.StatusCode, truncate(string(data), maxLoggedBodyBytes))
	}
	return data, nil
}
