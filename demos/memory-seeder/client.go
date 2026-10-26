package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AltairaLabs/omnia-demo/demos/memory-seeder/identity"
	"golang.org/x/time/rate"
)

// Wire names from Omnia's memory-api OpenAPI document
// (internal/memory/api/openapi.yaml). The per-user key is `virtual_user_id`
// everywhere on the wire; `user_id` is refused on query routes.
const (
	fieldWorkspaceID   = "workspace_id"
	fieldVirtualUserID = "virtual_user_id"
	fieldAgentID       = "agent_id"
	fieldType          = "type"
	fieldContent       = "content"
	fieldConfidence    = "confidence"
	fieldMetadata      = "metadata"
	fieldProvenance    = "provenance"

	pathSaveMemory    = "/api/v1/memories"
	pathAgentMemories = "/api/v1/agent-memories"
	pathInstitutional = "/api/v1/institutional/memories"
	pathIngestBatch   = "/api/v1/institutional/ingest/batch"
	pathReembed       = "/admin/reembed"
	pathBuildLeaves   = "/admin/brain/build-leaves"
	pathHealthz       = "/healthz"

	// ingestSource names this populator on every institutional row it writes.
	ingestSource = "omnia-demo-memory-seeder"
	// maxBatchDocuments is memory-api's per-call document limit.
	maxBatchDocuments = 100

	// TokenPathEnv names the projected ServiceAccount token file, the same
	// variable Omnia's own callers read. A missing file sends no token.
	TokenPathEnv = "SESSION_API_TOKEN_PATH"

	consentDecisionHeader = "X-Consent-Decision"

	// The `error` texts of memory-api's two 403s that refuse a TIER to this
	// caller rather than the caller itself (Omnia
	// internal/memory/api/brain_trust.go errSharedForbidden and
	// service.go ErrSubjectUnverified). With internalServiceAuth on, a caller
	// that is not on memory-api's exact subject allowlist gets them for the
	// shared tiers and for any write that names a user.
	msgSharedForbidden    = "shared memory may be changed only by a system service, never on a user's behalf"
	msgSubjectUnverified  = "scope names a user, and the request does not prove it is that user"
	errEnterpriseRequired = "enterprise_required"
)

// seedRPS/seedBurst keep the seeder polite to the memory API.
const (
	seedRPS   = 60
	seedBurst = 30
)

// maxAttempts bounds how often a transient failure (transport error, 429,
// 5xx, load-shedding 503) is retried. A 4xx is the request's fault and is not.
const maxAttempts = 5

// maxRetryAfter caps how long a server's Retry-After can hold one attempt.
const maxRetryAfter = 30 * time.Second

// maxPrivacyUnavailable is how many writes may be suppressed because
// privacy-api was unreachable before the seed stops: past it, every user-tier
// write is being dropped and the run would only report success for nothing.
const maxPrivacyUnavailable = 5

// TierRefusal is a 403 memory-api gives this caller for one tier, not for the
// caller as such: with internalServiceAuth on, only exact-listed platform
// services may write the shared tiers or name a user, and a Core install
// answers enterprise_required on the institutional routes. The seed skips the
// tier.
type TierRefusal struct {
	Path string
	Body string
}

func (e *TierRefusal) Error() string {
	return fmt.Sprintf("%s: 403 %s", e.Path, strings.TrimSpace(e.Body))
}

// FatalError is a refusal that every later write would get too: the caller is
// not authenticated or not allowed (401/403), or it pseudonymises under a
// different key than the one memory-api's database is bound to (409
// pseudonym_key_mismatch). The seed stops on it instead of logging thousands
// of identical failures.
type FatalError struct {
	Status int
	Path   string
	Body   string
}

// StatusError is any other unexpected answer.
type StatusError struct {
	Status int
	Path   string
	Body   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s: status %d: %s", e.Path, e.Status, strings.TrimSpace(e.Body))
}

func (e *FatalError) Error() string {
	hint := ""
	switch e.Status {
	case http.StatusUnauthorized:
		hint = "memory-api requires a ServiceAccount token (internalServiceAuth): mount a projected token " +
			"with the memory-api audience and set " + TokenPathEnv
	case http.StatusForbidden:
		hint = "memory-api refused this caller: with internalServiceAuth on, user-scoped and shared-tier " +
			"writes need a caller on memory-api's exact subject allowlist, and any caller must run in the " +
			"workspace namespace; or the workspace UID is not this memory-api's"
	case http.StatusConflict:
		hint = "the seeder's pseudonym key is not the workspace's: run in the workspace namespace with " +
			identity.KeyEnv + " from the omnia-workspace-pseudonym-key Secret"
	case http.StatusNoContent:
		hint = "memory-api suppressed user-tier writes because it could not reach the workspace's privacy-api " +
			"(204 without " + consentDecisionHeader + "): fix privacy-api, then re-run"
	}
	return fmt.Sprintf("%s: status %d: %s (%s)", e.Path, e.Status, strings.TrimSpace(e.Body), hint)
}

// Client posts seed data to one workspace's memory-api.
type Client struct {
	base         string
	workspaceUID string
	tokenPath    string
	pseudo       identity.Pseudonymizer
	http         *http.Client
	limiter      *rate.Limiter
	backoff      time.Duration
	// sleep waits between attempts; a variable for tests.
	sleep func(ctx context.Context, d time.Duration) error

	// Suppressed counts writes the subject's consent refused (204 with
	// X-Consent-Decision). The seed writes some sensitive categories on
	// purpose, so these are expected.
	Suppressed int
	// PrivacyUnavailable counts writes memory-api dropped because privacy-api
	// was unreachable (204 without X-Consent-Decision): a fault, not consent.
	PrivacyUnavailable int
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// NewClient returns a client for the memory-api at base serving workspaceUID.
// tokenPath may be empty or name a file that does not exist (no token sent).
func NewClient(base, workspaceUID, tokenPath string, pseudo identity.Pseudonymizer) *Client {
	return &Client{
		base:         strings.TrimRight(base, "/"),
		workspaceUID: workspaceUID,
		tokenPath:    tokenPath,
		pseudo:       pseudo,
		// DisableKeepAlives: a fresh connection per request. Reused keep-alive
		// connections to the per-workspace memory-api were a source of
		// intermittent, body-dropping failures during bulk seeding.
		http:    &http.Client{Timeout: 2 * time.Minute, Transport: &http.Transport{DisableKeepAlives: true}},
		limiter: rate.NewLimiter(seedRPS, seedBurst),
		backoff: 200 * time.Millisecond,
		sleep:   sleepCtx,
	}
}

// CheckKeyBinding compares the fingerprint memory-api's database is bound to
// (stated on every answer, /healthz included) with the seeder's own, so a
// wrong or missing workspace key fails before anything is written. A server
// that states none, or cannot be reached here, is left to the per-request
// check.
func (c *Client) CheckKeyBinding(ctx context.Context) (bound string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+pathHealthz, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", nil
	}
	_ = resp.Body.Close()
	bound = resp.Header.Get(identity.BoundFingerprintHeader)
	if bound != "" && bound != c.pseudo.Fingerprint() {
		return bound, &FatalError{Status: http.StatusConflict, Path: pathHealthz,
			Body: fmt.Sprintf("memory-api is bound to key fingerprint %q and the seeder's key gives %q",
				bound, c.pseudo.Fingerprint())}
	}
	return bound, nil
}

// postJSON POSTs body to path and returns the response body when the status is
// wantStatus, or nil with a nil error for a 204 (a suppressed write).
func (c *Client) postJSON(ctx context.Context, path string, body any, wantStatus int) ([]byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var lastErr error
	wait := time.Duration(0)
	for attempt := range maxAttempts {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		if attempt > 0 {
			if err := c.sleep(ctx, max(wait, time.Duration(attempt)*c.backoff)); err != nil {
				return nil, err
			}
		}
		out, resp, err := c.doOnce(ctx, path, buf)
		if err != nil {
			lastErr, wait = err, 0
			continue
		}
		status := resp.StatusCode
		switch {
		case status == wantStatus:
			return out, nil
		case status == http.StatusNoContent:
			return nil, c.suppressed(path, resp)
		case status == http.StatusTooManyRequests || status >= 500:
			// 429 is the per-pod limiter; 503 with Retry-After is memory-api's
			// load shedder. Both say when to come back.
			lastErr, wait = &StatusError{Status: status, Path: path, Body: string(out)}, retryAfter(resp)
		default:
			return nil, classify(status, path, out)
		}
	}
	return nil, lastErr
}

// suppressed handles a 204: the privacy middleware dropped the write. With
// X-Consent-Decision it is the subject's consent, which the seed provokes on
// purpose; without it, privacy-api was unreachable and memory-api failed
// closed, which stops the seed once it keeps happening.
func (c *Client) suppressed(path string, resp *http.Response) error {
	if resp.Header.Get(consentDecisionHeader) != "" {
		c.Suppressed++
		return nil
	}
	c.PrivacyUnavailable++
	if c.PrivacyUnavailable >= maxPrivacyUnavailable {
		return &FatalError{Status: http.StatusNoContent, Path: path,
			Body: fmt.Sprintf("%d writes dropped with privacy-api unreachable", c.PrivacyUnavailable)}
	}
	return fmt.Errorf("%s: write dropped: privacy-api unreachable (204 without %s)", path, consentDecisionHeader)
}

// retryAfter reads a Retry-After given in seconds, capped; 0 when absent.
func retryAfter(resp *http.Response) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After")))
	if err != nil || secs <= 0 {
		return 0
	}
	return min(time.Duration(secs)*time.Second, maxRetryAfter)
}

// classify turns a non-retryable answer into its error. A refusal every later
// request would also get (401; a 403 that is not about one tier, such as a
// foreign workspace or a ServiceAccount outside the workspace namespace; 409
// pseudonym_key_mismatch) is a FatalError. A 403 that refuses one tier — the
// shared-tier and named-user refusals, and enterprise_required on a Core
// install — is a TierRefusal. Anything else is a StatusError: that write
// failed, the rest go on.
func classify(status int, path string, body []byte) error {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	switch {
	case status == http.StatusUnauthorized:
		return &FatalError{Status: status, Path: path, Body: string(body)}
	case status == http.StatusForbidden && (e.Error == msgSharedForbidden || e.Error == msgSubjectUnverified ||
		e.Error == errEnterpriseRequired):
		return &TierRefusal{Path: path, Body: string(body)}
	case status == http.StatusForbidden:
		return &FatalError{Status: status, Path: path, Body: string(body)}
	case status == http.StatusConflict && e.Error == identity.KeyMismatchError:
		return &FatalError{Status: status, Path: path, Body: string(body)}
	}
	return &StatusError{Status: status, Path: path, Body: string(body)}
}

// doOnce performs one POST and returns the body and the response.
func (c *Client) doOnce(ctx context.Context, path string, buf []byte) ([]byte, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(buf))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(identity.FingerprintHeader, c.pseudo.Fingerprint())
	if tok := c.token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	out := new(bytes.Buffer)
	_, _ = out.ReadFrom(resp.Body)
	return out.Bytes(), resp, nil
}

// token re-reads the projected token on each request: the kubelet rotates it.
func (c *Client) token() string {
	if c.tokenPath == "" {
		return ""
	}
	b, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// ingestBatchResponse is IngestBatchResponse in the spec.
type ingestBatchResponse struct {
	Accepted int `json:"accepted"`
	Skipped  int `json:"skipped"`
	Rejected []struct {
		Key   string `json:"key"`
		Error string `json:"error"`
	} `json:"rejected"`
	EmbeddingBacklog int `json:"embedding_backlog"`
}

// IngestDocs sends the knowledge-base documents through the batch ingest
// contract (202), at most 100 per call. memory-api chunks each one and keys
// it by url, so a re-seed rewrites rather than duplicates. It returns how many
// were accepted; per-document rejections are returned as one error.
func (c *Client) IngestDocs(ctx context.Context, docs []Doc) (int, error) {
	accepted := 0
	var rejected []string
	for start := 0; start < len(docs); start += maxBatchDocuments {
		end := min(start+maxBatchDocuments, len(docs))
		batch := make([]map[string]any, 0, end-start)
		for _, d := range docs[start:end] {
			batch = append(batch, map[string]any{"title": d.Title, "url": d.URL, "site": d.Site, "text": d.Text})
		}
		body := map[string]any{fieldWorkspaceID: c.workspaceUID, "source": ingestSource, "documents": batch}
		raw, err := c.postJSON(ctx, pathIngestBatch, body, http.StatusAccepted)
		if err != nil {
			return accepted, err
		}
		var r ingestBatchResponse
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &r); err != nil {
				return accepted, fmt.Errorf("%s: decode response: %w", pathIngestBatch, err)
			}
		}
		accepted += r.Accepted + r.Skipped
		for _, rj := range r.Rejected {
			rejected = append(rejected, rj.Key+": "+rj.Error)
		}
	}
	if len(rejected) > 0 {
		return accepted, fmt.Errorf("%d documents rejected: %s", len(rejected), strings.Join(rejected, "; "))
	}
	return accepted, nil
}

// SaveInstitutional saves one workspace-scoped curated memory (201). The
// workspace is the body's workspace_id.
func (c *Client) SaveInstitutional(ctx context.Context, typ, content string, confidence float64) (string, error) {
	body := map[string]any{
		fieldWorkspaceID: c.workspaceUID, fieldType: typ, fieldContent: content, fieldConfidence: confidence,
	}
	return c.saveID(ctx, pathInstitutional, body)
}

// SaveAgentMemory saves an agent-tier curated memory (201), scoped by the
// body's workspace_id and agent_id (an AgentRuntime UID).
func (c *Client) SaveAgentMemory(ctx context.Context, m AgentMemory) (string, error) {
	body := map[string]any{
		fieldWorkspaceID: c.workspaceUID, fieldAgentID: m.AgentID,
		fieldType: m.Type, fieldContent: m.Content, fieldConfidence: m.Confidence,
	}
	return c.saveID(ctx, pathAgentMemories, body)
}

// SaveUserMemory saves a user-tier memory for the workspace pseudonym of
// m.RawUserID — the same value Omnia's facade derives for that identity in
// this workspace. The raw identifier never leaves the process.
func (c *Client) SaveUserMemory(ctx context.Context, m UserMemory) (string, error) {
	return c.saveUser(ctx, m.RawUserID, m.Type, m.Content, m.Category, m.Confidence, "user_requested")
}

// SaveObservation saves a system-generated observation for the synthetic
// operations user, so each topic also has a dense run of related items.
func (c *Client) SaveObservation(ctx context.Context, o Observation) (string, error) {
	return c.saveUser(ctx, observationOwnerUser, typeObservation, o.Content, o.Category, 0.5, "system_generated")
}

func (c *Client) saveUser(ctx context.Context, rawUser, typ, content, category string, confidence float64,
	provenance string) (string, error) {
	body := map[string]any{
		fieldType: typ, fieldContent: content, fieldConfidence: confidence,
		"category": category,
		"scope": map[string]string{
			fieldWorkspaceID:   c.workspaceUID,
			fieldVirtualUserID: c.pseudo.ID(rawUser),
		},
		fieldMetadata: map[string]any{fieldProvenance: provenance},
	}
	return c.saveID(ctx, pathSaveMemory, body)
}

// saveResp covers MemoryResponse and SaveInstitutionalResponse.
type saveResp struct {
	Memory struct {
		ID string `json:"id"`
	} `json:"memory"`
}

func (c *Client) saveID(ctx context.Context, path string, body any) (string, error) {
	raw, err := c.postJSON(ctx, path, body, http.StatusCreated)
	if err != nil {
		return "", err
	}
	// A suppressed write (204) has no body and no id.
	if len(raw) == 0 {
		return "", nil
	}
	var sr saveResp
	if err := json.Unmarshal(raw, &sr); err != nil {
		return "", err
	}
	return sr.Memory.ID, nil
}

// reembedResp is ReembedResponse in the spec.
type reembedResp struct {
	Embedded  int    `json:"embedded"`
	Batches   int    `json:"batches"`
	Remaining *int   `json:"remaining"`
	Error     string `json:"error"`
}

// reembedBatchesPerCall keeps one /admin/reembed call (50 rows a batch) well
// inside memory-api's 60 s write timeout even with a slow provider.
const reembedBatchesPerCall = 50

// maxReembedCalls bounds the drain: 50 calls of up to 2,500 rows.
const maxReembedCalls = 50

// DrainEmbeddings embeds everything the seed wrote without a vector (ingested
// chunks are never embedded inline), repeating the call until it embeds
// nothing, as the spec prescribes. It returns how many it embedded. A brain
// memory-api does not start without an embedding provider, so a 503 here is
// load shedding, retried like any other.
func (c *Client) DrainEmbeddings(ctx context.Context) (int, error) {
	total := 0
	for range maxReembedCalls {
		raw, err := c.postJSON(ctx, pathReembed, map[string]any{"max_batches": reembedBatchesPerCall}, http.StatusOK)
		if err != nil {
			return total, err
		}
		var r reembedResp
		if err := json.Unmarshal(raw, &r); err != nil {
			return total, fmt.Errorf("%s: decode response: %w", pathReembed, err)
		}
		total += r.Embedded
		if r.Embedded == 0 {
			return total, nil
		}
	}
	return total, fmt.Errorf("%s: backlog not drained after %d calls", pathReembed, maxReembedCalls)
}

// Leaf-fit bounds from memory-api's spec for /admin/brain/build-leaves.
const (
	maxLeaves     = 50000
	defaultSample = 20000
)

// BuildLeaves fits the workspace's leaf clusters, which memory-api never does
// on a timer; the cluster worker then builds the tree the dashboard's Memory
// Galaxy draws. The fit is seeded, so a given --seed gives the same layout
// for the same corpus.
func (c *Client) BuildLeaves(ctx context.Context, k int, seed int64) (int, error) {
	// The sample must be at least k; memory-api's default is 20,000.
	body := map[string]any{"k": k, "sample": max(defaultSample, k), "seed": seed}
	raw, err := c.postJSON(ctx, pathBuildLeaves, body, http.StatusOK)
	if err != nil {
		return 0, err
	}
	var r struct {
		Leaves int `json:"leaves"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return 0, fmt.Errorf("%s: decode response: %w", pathBuildLeaves, err)
	}
	return r.Leaves, nil
}
