package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AltairaLabs/omnia-demo/demos/memory-seeder/identity"
	"golang.org/x/time/rate"
)

// The request shapes below are hand-copied from the schemas in Omnia's
// memory-api OpenAPI document (internal/memory/api/openapi.yaml) at
// AltairaLabs/Omnia 6d8aea78566562f51fc77ad8d6522a7207ab9ac5:
// SaveMemoryRequest, SaveInstitutionalRequest, SaveAgentScopedRequest,
// IngestBatchRequest/IngestBatchDocument and the /admin/reembed and
// /admin/brain/build-leaves bodies. Refresh them when the spec changes. The fake server decodes with
// DisallowUnknownFields, so a field the spec does not declare — `user_id`, an
// undocumented `about` — fails the test instead of being silently ignored.

type specSaveMemoryRequest struct {
	Type         string            `json:"type"`
	Content      string            `json:"content"`
	Metadata     map[string]any    `json:"metadata"`
	Confidence   float64           `json:"confidence"`
	Scope        map[string]string `json:"scope"`
	SessionID    string            `json:"session_id"`
	TurnRange    []int             `json:"turn_range"`
	Category     string            `json:"category"`
	Tier         string            `json:"tier"`
	ShareAgentID string            `json:"share_agent_id"`
	Facet        string            `json:"facet"`
}

type specSaveInstitutionalRequest struct {
	WorkspaceID string         `json:"workspace_id"`
	Type        string         `json:"type"`
	Content     string         `json:"content"`
	Metadata    map[string]any `json:"metadata"`
	Confidence  float64        `json:"confidence"`
	ExpiresAt   string         `json:"expires_at"`
}

type specSaveAgentScopedRequest struct {
	WorkspaceID string         `json:"workspace_id"`
	AgentID     string         `json:"agent_id"`
	Type        string         `json:"type"`
	Content     string         `json:"content"`
	Metadata    map[string]any `json:"metadata"`
	Confidence  float64        `json:"confidence"`
	ExpiresAt   string         `json:"expires_at"`
}

type specIngestBatchDocument struct {
	Title       string         `json:"title"`
	URL         string         `json:"url"`
	Site        string         `json:"site"`
	Text        string         `json:"text"`
	Sections    []any          `json:"sections"`
	Author      string         `json:"author"`
	ModifiedAt  string         `json:"modified_at"`
	Version     string         `json:"version"`
	ContentHash string         `json:"content_hash"`
	Metadata    map[string]any `json:"metadata"`
}

type specIngestBatchRequest struct {
	WorkspaceID string                    `json:"workspace_id"`
	Source      string                    `json:"source"`
	Documents   []specIngestBatchDocument `json:"documents"`
	Items       []any                     `json:"items"`
	Edges       []any                     `json:"edges"`
	Present     map[string]any            `json:"present"`
}

type specReembedRequest struct {
	MaxBatches int `json:"max_batches"`
}

type specBuildLeavesRequest struct {
	K      int   `json:"k"`
	Sample int   `json:"sample"`
	Seed   int64 `json:"seed"`
}

const (
	testWorkspace = "0b6b1c1e-7a52-4c2e-9d4b-5f0e2a1c3d4e"
	testAgent     = "7f3a9c2d-1e4b-4a8f-b6c5-2d9e0f1a3b7c"
	// A public test fixture from Omnia's pseudonym vectors, not a real key.
	testKey   = "5528ce38f4b53ab2d14398cc64fd9d1cf3cf4a649078bf6537f17b8f1ad3b883"
	testToken = "projected-sa-token"
)

// fakeMemoryAPI records what the seeder sends and answers like memory-api.
type fakeMemoryAPI struct {
	t     *testing.T
	bound string // fingerprint memory_meta is bound to

	mu            sync.Mutex
	calls         map[string]int
	userMemories  []specSaveMemoryRequest
	agentMemories []specSaveAgentScopedRequest
	institutional []specSaveInstitutionalRequest
	ingests       []specIngestBatchRequest
	leaves        []specBuildLeavesRequest
	reembeds      []specReembedRequest
	reembedLeft   int

	// override, when set, answers a path instead of the default handler.
	override func(w http.ResponseWriter, r *http.Request) bool
}

func newFake(t *testing.T, bound string) (*fakeMemoryAPI, *httptest.Server) {
	f := &fakeMemoryAPI{t: t, bound: bound, calls: map[string]int{}, reembedLeft: 3}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

func decodeStrict(t *testing.T, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Errorf("%s %s: body does not match the spec: %v", r.Method, r.URL.Path, err)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeMemoryAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(identity.BoundFingerprintHeader, f.bound)
	f.mu.Lock()
	f.calls[r.Method+" "+r.URL.Path]++
	f.mu.Unlock()
	if f.override != nil && f.override(w, r) {
		return
	}
	if r.URL.Path == pathHealthz {
		w.WriteHeader(http.StatusOK)
		return
	}
	f.checkHeaders(r)
	f.route(w, r)
}

func (f *fakeMemoryAPI) checkHeaders(r *http.Request) {
	if r.Method != http.MethodPost {
		f.t.Errorf("%s %s: seeder only POSTs", r.Method, r.URL.Path)
	}
	if r.URL.RawQuery != "" {
		f.t.Errorf("POST %s: unexpected query %q; the spec's POST routes take the body only", r.URL.Path, r.URL.RawQuery)
	}
	if got := r.Header.Get(identity.FingerprintHeader); got != f.bound {
		f.t.Errorf("POST %s: fingerprint header %q, want %q", r.URL.Path, got, f.bound)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
		f.t.Errorf("POST %s: Authorization %q", r.URL.Path, got)
	}
}

func (f *fakeMemoryAPI) route(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case pathSaveMemory:
		var req specSaveMemoryRequest
		if decodeStrict(f.t, r, &req) {
			f.userMemories = append(f.userMemories, req)
		}
		writeJSON(w, http.StatusCreated, map[string]any{"memory": map[string]any{"id": "u"}})
	case pathAgentMemories:
		var req specSaveAgentScopedRequest
		if decodeStrict(f.t, r, &req) {
			f.agentMemories = append(f.agentMemories, req)
		}
		writeJSON(w, http.StatusCreated, map[string]any{"memory": map[string]any{"id": "a"}})
	case pathInstitutional:
		var req specSaveInstitutionalRequest
		if decodeStrict(f.t, r, &req) {
			f.institutional = append(f.institutional, req)
		}
		writeJSON(w, http.StatusCreated, map[string]any{"memory": map[string]any{"id": "i"}, "embedding_backlog": 1})
	case pathIngestBatch:
		var req specIngestBatchRequest
		if decodeStrict(f.t, r, &req) {
			f.ingests = append(f.ingests, req)
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"accepted": len(req.Documents), "skipped": 0,
			"forgotten": 0, "rejected": []any{}, "embedding_backlog": 10})
	case pathReembed:
		var req specReembedRequest
		if decodeStrict(f.t, r, &req) {
			f.reembeds = append(f.reembeds, req)
		}
		embedded := 0
		if f.reembedLeft > 0 {
			f.reembedLeft--
			embedded = 50
		}
		writeJSON(w, http.StatusOK, map[string]any{"embedded": embedded, "batches": 1, "remaining": f.reembedLeft * 50})
	case pathBuildLeaves:
		var req specBuildLeavesRequest
		if decodeStrict(f.t, r, &req) {
			f.leaves = append(f.leaves, req)
		}
		writeJSON(w, http.StatusOK, map[string]any{"leaves": req.K})
	default:
		f.t.Errorf("POST %s: not a route in memory-api's spec", r.URL.Path)
		http.NotFound(w, r)
	}
}

func (f *fakeMemoryAPI) callCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[key]
}

// step is the result of the named step, zero when it did not run.
func (s summary) step(name string) stepResult {
	for _, st := range s.Steps {
		if st.Name == name {
			return st
		}
	}
	return stepResult{}
}

func smallScenario() Scenario {
	s := DefaultScenario(testWorkspace)
	s.AgentUID = testAgent
	s.InstitutionalDocs = 7
	s.AgentMemories = 5
	s.Users = 4
	s.MemoriesPerUser = 6
	s.ObservationThemes = 3
	s.ObsPerTheme = 2
	return s
}

func testClient(t *testing.T, base, key string) *Client {
	tok := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tok, []byte(testToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := NewClient(base, testWorkspace, tok, identity.New(key))
	c.limiter = rate.NewLimiter(rate.Inf, 1)
	c.backoff = time.Millisecond
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

// recordSleeps makes c record, not wait out, the pauses between attempts.
func recordSleeps(c *Client) *[]time.Duration {
	var got []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error {
		got = append(got, d)
		return nil
	}
	return &got
}

func TestGenerateIsDeterministicPerSeed(t *testing.T) {
	s := DefaultScenario(testWorkspace)
	s.AgentUID = testAgent
	a := Generate(s, rand.New(rand.NewSource(7)))
	b := Generate(s, rand.New(rand.NewSource(7)))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different data")
	}
	c := Generate(s, rand.New(rand.NewSource(8)))
	if reflect.DeepEqual(a, c) {
		t.Fatal("different seeds produced identical data")
	}
	if len(a.Docs) != s.InstitutionalDocs || len(a.AgentMemories) != s.AgentMemories ||
		len(a.UserMemories) != s.Users*s.MemoriesPerUser || len(a.Observations) != s.ObservationThemes*s.ObsPerTheme {
		t.Fatalf("unexpected counts: %d docs, %d agent, %d user, %d obs",
			len(a.Docs), len(a.AgentMemories), len(a.UserMemories), len(a.Observations))
	}
}

func TestGenerateUsesTypesTheBuiltinSchemaAdmits(t *testing.T) {
	s := DefaultScenario(testWorkspace)
	s.AgentUID = testAgent
	g := Generate(s, rand.New(rand.NewSource(1)))
	seen := map[string]bool{}
	for _, m := range g.UserMemories {
		if !slices.Contains(UserTierTypes, m.Type) {
			t.Fatalf("user memory type %q is rejected by memory-api's built-in schema", m.Type)
		}
		seen[m.Category] = true
	}
	for _, m := range g.AgentMemories {
		if !slices.Contains(AgentTierTypes, m.Type) {
			t.Fatalf("agent memory type %q is rejected by memory-api's built-in schema", m.Type)
		}
		if m.AgentID != testAgent {
			t.Fatalf("agent memory scoped to %q", m.AgentID)
		}
	}
	for _, c := range Categories {
		if !seen[c] {
			t.Errorf("category %s not covered", c)
		}
	}
}

func TestRunWritesTheSpecContract(t *testing.T) {
	key := testKey
	pseudo := identity.New(key)
	fake, srv := newFake(t, pseudo.Fingerprint())
	c := testClient(t, srv.URL, key)
	g := Generate(smallScenario(), rand.New(rand.NewSource(3)))

	if _, err := c.CheckKeyBinding(context.Background()); err != nil {
		t.Fatalf("key binding: %v", err)
	}
	sum, err := run(context.Background(), c, g, options{buildGalaxy: true, leaves: 16, seed: 3})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if sum.failures() != 0 {
		t.Fatalf("%d tolerated failures", sum.failures())
	}
	for name, want := range map[string]int{stepDocuments: 7, stepInstitutional: institutionalFacts,
		stepAgent: 5, stepUser: 24, stepObservations: 6, stepGalaxy: 1} {
		if st := sum.step(name); st.Stored != want || st.Items != want || st.Skipped != "" {
			t.Fatalf("%s: %+v, want %d stored", name, st, want)
		}
	}

	// Documents go through one batch call, scoped to the workspace UID.
	if len(fake.ingests) != 1 || len(fake.ingests[0].Documents) != 7 ||
		fake.ingests[0].Source == "" || fake.ingests[0].WorkspaceID != testWorkspace {
		t.Fatalf("ingest batches: %+v", fake.ingests)
	}
	for _, m := range fake.agentMemories {
		if m.WorkspaceID != testWorkspace || m.AgentID != testAgent {
			t.Fatalf("agent memory scope: %+v", m)
		}
	}
	for _, m := range fake.institutional {
		if m.WorkspaceID != testWorkspace {
			t.Fatalf("institutional scope: %+v", m)
		}
	}

	// Every user-tier write names the workspace pseudonym, never a raw id.
	want := map[string]bool{pseudo.ID(observationOwnerUser): true}
	for u := range 4 {
		want[pseudo.ID(rawUser(u))] = true
	}
	for _, m := range fake.userMemories {
		vu := m.Scope[fieldVirtualUserID]
		if !want[vu] {
			t.Fatalf("virtual_user_id %q is not a workspace pseudonym of a seeded user", vu)
		}
		if m.Scope[fieldWorkspaceID] != testWorkspace {
			t.Fatalf("scope %v", m.Scope)
		}
		if _, ok := m.Scope["user_id"]; ok {
			t.Fatal("scope carries the retired user_id key")
		}
		if !slices.Contains(Categories, m.Category) {
			t.Fatalf("category %q", m.Category)
		}
	}
	// The keyed pseudonym is what Omnia derives for this key, not plain SHA-256.
	if pseudo.ID("customer-000") == identity.New("").ID("customer-000") {
		t.Fatal("keyed and unkeyed pseudonyms coincide")
	}

	// Galaxy: drained until embedded == 0, then one seeded leaf fit.
	if got := fake.callCount("POST " + pathReembed); got != 4 {
		t.Fatalf("reembed calls = %d, want 4", got)
	}
	for _, rq := range fake.reembeds {
		if rq.MaxBatches != reembedBatchesPerCall {
			t.Fatalf("reembed max_batches = %d", rq.MaxBatches)
		}
	}
	if len(fake.leaves) != 1 || fake.leaves[0].K != 16 || fake.leaves[0].Seed != 3 ||
		fake.leaves[0].Sample != defaultSample {
		t.Fatalf("build-leaves: %+v", fake.leaves)
	}
	if got := fake.callCount("POST /api/v1/relations"); got != 0 {
		t.Fatal("relations route was removed with the legacy store")
	}
}

func rawUser(u int) string {
	g := Generate(Scenario{Users: u + 1, MemoriesPerUser: 1}, rand.New(rand.NewSource(1)))
	return g.UserMemories[u].RawUserID
}

func TestRunStopsOnAuthRefusal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"unauthenticated", http.StatusUnauthorized, `{"error":"unauthorized"}`},
		{"foreign workspace", http.StatusForbidden,
			`{"error":"workspace \"x\" is not this memory-api's workspace (\"default\")"}`},
		{"key mismatch", http.StatusConflict, `{"error":"pseudonym_key_mismatch","message":"m"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake, srv := newFake(t, identity.UnkeyedFingerprint)
			fake.override = func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == pathAgentMemories {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
					return true
				}
				return false
			}
			c := testClient(t, srv.URL, "")
			_, err := run(context.Background(), c, Generate(smallScenario(), rand.New(rand.NewSource(1))),
				options{buildGalaxy: true, leaves: 8})
			var fe *FatalError
			if !errors.As(err, &fe) || fe.Status != tc.status {
				t.Fatalf("err = %v, want FatalError %d", err, tc.status)
			}
			if n := fake.callCount("POST " + pathAgentMemories); n != 1 {
				t.Fatalf("agent route called %d times; a fatal refusal must not be retried", n)
			}
			if n := fake.callCount("POST " + pathSaveMemory); n != 0 {
				t.Fatalf("run continued after a fatal refusal (%d user writes)", n)
			}
		})
	}
}

func TestEnterpriseRequiredSkipsTheInstitutionalTiers(t *testing.T) {
	fake, srv := newFake(t, identity.UnkeyedFingerprint)
	fake.override = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasPrefix(r.URL.Path, "/api/v1/institutional/") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "enterprise_required"})
			return true
		}
		return false
	}
	c := testClient(t, srv.URL, "")
	sum, err := run(context.Background(), c, Generate(smallScenario(), rand.New(rand.NewSource(1))), options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, name := range []string{stepDocuments, stepInstitutional} {
		if st := sum.step(name); !strings.Contains(st.Skipped, "Enterprise") {
			t.Fatalf("%s: %+v, want skipped for Enterprise", name, st)
		}
	}
	if n := fake.callCount("POST " + pathInstitutional); n != 1 {
		t.Fatalf("institutional route called %d times after the tier was refused", n)
	}
	if st := sum.step(stepUser); st.Stored != 24 {
		t.Fatalf("user step %+v", st)
	}
}

// With internalServiceAuth on, a workspace ServiceAccount may ingest documents
// but not write the shared tiers or name a user. The seed lands the documents
// and skips every other tier, each on its first refusal, and succeeds.
func TestServiceAuthOnSeedsDocumentsAndSkipsTheRest(t *testing.T) {
	fake, srv := newFake(t, identity.UnkeyedFingerprint)
	fake.override = func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case pathInstitutional, pathAgentMemories, pathBuildLeaves:
			writeJSON(w, http.StatusForbidden, map[string]string{"error": msgSharedForbidden})
			return true
		case pathSaveMemory:
			writeJSON(w, http.StatusForbidden, map[string]string{"error": msgSubjectUnverified})
			return true
		}
		return false
	}
	c := testClient(t, srv.URL, "")
	sum, err := run(context.Background(), c, Generate(smallScenario(), rand.New(rand.NewSource(1))),
		options{buildGalaxy: true, leaves: 8})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if st := sum.step(stepDocuments); st.Stored != 7 || st.Skipped != "" {
		t.Fatalf("documents %+v", st)
	}
	for _, name := range []string{stepInstitutional, stepAgent, stepUser, stepObservations, stepGalaxy} {
		if st := sum.step(name); st.Skipped == "" || st.Failed != 0 {
			t.Fatalf("%s: %+v, want skipped", name, st)
		}
	}
	for path, want := range map[string]int{pathInstitutional: 1, pathAgentMemories: 1, pathSaveMemory: 1} {
		if n := fake.callCount("POST " + path); n != want {
			t.Fatalf("%s called %d times, want %d: a refused tier is not retried", path, n, want)
		}
	}
	if sum.failures() != 0 {
		t.Fatalf("tier refusals counted as failures: %d", sum.failures())
	}
}

func TestAStepThatStoresNothingFailsTheRun(t *testing.T) {
	fake, srv := newFake(t, identity.UnkeyedFingerprint)
	fake.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == pathSaveMemory {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory schema rejects unknown type"})
			return true
		}
		return false
	}
	c := testClient(t, srv.URL, "")
	sum, err := run(context.Background(), c, Generate(smallScenario(), rand.New(rand.NewSource(1))), options{})
	if err == nil || !strings.Contains(err.Error(), stepUser) || !strings.Contains(err.Error(), stepObservations) {
		t.Fatalf("err = %v, want the empty user and observation steps named", err)
	}
	if st := sum.step(stepUser); st.Failed != 24 {
		t.Fatalf("user step %+v", st)
	}
}

func TestSummaryErr(t *testing.T) {
	for _, tc := range []struct {
		name  string
		steps []stepResult
		ok    bool
	}{
		{"all stored", []stepResult{{Name: "a", Items: 2, Stored: 2}}, true},
		{"some failed", []stepResult{{Name: "a", Items: 2, Stored: 1, Failed: 1}}, true},
		{"skipped and stored", []stepResult{{Name: "a", Items: 2, Skipped: "x"}, {Name: "b", Items: 1, Stored: 1}}, true},
		{"nothing to write", []stepResult{{Name: "a"}}, true},
		{"empty step", []stepResult{{Name: "a", Items: 2, Failed: 2}, {Name: "b", Items: 1, Stored: 1}}, false},
		{"all consent-refused", []stepResult{{Name: "a", Items: 2}}, false},
		{"everything skipped", []stepResult{{Name: "a", Items: 2, Skipped: "x"}}, false},
	} {
		if err := (summary{Steps: tc.steps}).Err(); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}
}

func TestCheckKeyBinding(t *testing.T) {
	for _, tc := range []struct {
		name, bound, key string
		fatal            bool
	}{
		{"keyed match", identity.New(testKey).Fingerprint(), testKey, false},
		{"unkeyed match", identity.UnkeyedFingerprint, "", false},
		{"missing key on a keyed install", identity.New(testKey).Fingerprint(), "", true},
		{"server states none", "", testKey, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := newFake(t, tc.bound)
			_, err := testClient(t, srv.URL, tc.key).CheckKeyBinding(context.Background())
			var fe *FatalError
			if got := errors.As(err, &fe); got != tc.fatal {
				t.Fatalf("err = %v, fatal = %t, want %t", err, got, tc.fatal)
			}
		})
	}
}

func TestSuppressedWrites(t *testing.T) {
	fake, srv := newFake(t, identity.UnkeyedFingerprint)
	consent := true
	fake.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != pathSaveMemory {
			return false
		}
		if consent {
			w.Header().Set(consentDecisionHeader, "deny; category=memory:health")
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	c := testClient(t, srv.URL, "")
	m := UserMemory{RawUserID: "customer-001", Category: catHealth, Type: typePreference, Content: "x", Confidence: 0.7}
	id, err := c.SaveUserMemory(context.Background(), m)
	if err != nil || id != "" {
		t.Fatalf("consent-denied write: id %q err %v", id, err)
	}
	consent = false
	for i := 1; i < maxPrivacyUnavailable; i++ {
		_, err := c.SaveUserMemory(context.Background(), m)
		var fe *FatalError
		if err == nil || errors.As(err, &fe) {
			t.Fatalf("occurrence %d: err = %v, want a tolerated error", i, err)
		}
	}
	_, err = c.SaveUserMemory(context.Background(), m)
	var fe *FatalError
	if !errors.As(err, &fe) || !strings.Contains(err.Error(), "privacy-api") {
		t.Fatalf("err = %v, want FatalError after %d privacy-api outages", err, maxPrivacyUnavailable)
	}
	if c.Suppressed != 1 || c.PrivacyUnavailable != maxPrivacyUnavailable {
		t.Fatalf("suppressed = %d, privacy unavailable = %d", c.Suppressed, c.PrivacyUnavailable)
	}
}

func TestRetriesTransientButNotClientErrors(t *testing.T) {
	fake, srv := newFake(t, identity.UnkeyedFingerprint)
	shed := 2
	fake.override = func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case pathAgentMemories:
			if shed > 0 {
				shed--
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusServiceUnavailable)
				return true
			}
		case pathInstitutional:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expires_at in the past"})
			return true
		}
		return false
	}
	c := testClient(t, srv.URL, "")
	sleeps := recordSleeps(c)
	if _, err := c.SaveAgentMemory(context.Background(), AgentMemory{AgentID: testAgent, Type: typeFact,
		Content: "x", Confidence: 0.8}); err != nil {
		t.Fatalf("shed write not retried to success: %v", err)
	}
	if n := fake.callCount("POST " + pathAgentMemories); n != 3 {
		t.Fatalf("agent calls = %d, want 3", n)
	}
	if len(*sleeps) != 2 || (*sleeps)[0] < time.Second || (*sleeps)[1] < time.Second {
		t.Fatalf("pauses %v: Retry-After: 1 not honoured", *sleeps)
	}
	_, err := c.SaveInstitutional(context.Background(), typePolicy, "x", 0.9)
	var se *StatusError
	if !errors.As(err, &se) || se.Status != http.StatusBadRequest {
		t.Fatalf("err = %v", err)
	}
	if n := fake.callCount("POST " + pathInstitutional); n != 1 {
		t.Fatalf("400 retried: %d calls", n)
	}
}

// A 503 from /admin/reembed is load shedding, not a missing embedder (a brain
// memory-api does not start without one): it is retried after Retry-After and
// the leaf fit still runs.
func TestReembedLoadShedIsRetried(t *testing.T) {
	fake, srv := newFake(t, identity.UnkeyedFingerprint)
	shed := 2
	fake.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == pathReembed && shed > 0 {
			shed--
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	c := testClient(t, srv.URL, "")
	sleeps := recordSleeps(c)
	if err := buildGalaxy(context.Background(), c, options{leaves: 8}); err != nil {
		t.Fatalf("buildGalaxy: %v", err)
	}
	if n := fake.callCount("POST " + pathBuildLeaves); n != 1 {
		t.Fatalf("build-leaves called %d times", n)
	}
	if len(*sleeps) < 2 || (*sleeps)[0] < time.Second {
		t.Fatalf("pauses %v: Retry-After not honoured", *sleeps)
	}
}

func TestPersistentReembedFailureFailsTheGalaxyStep(t *testing.T) {
	fake, srv := newFake(t, identity.UnkeyedFingerprint)
	fake.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == pathReembed {
			w.WriteHeader(http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	c := testClient(t, srv.URL, "")
	sum, err := run(context.Background(), c, Generate(smallScenario(), rand.New(rand.NewSource(1))),
		options{buildGalaxy: true, leaves: 8})
	if err == nil || !strings.Contains(err.Error(), stepGalaxy) {
		t.Fatalf("err = %v, want the galaxy step named", err)
	}
	if n := fake.callCount("POST " + pathReembed); n != maxAttempts {
		t.Fatalf("reembed called %d times, want %d", n, maxAttempts)
	}
	if st := sum.step(stepGalaxy); st.Failed != 1 || st.Stored != 0 {
		t.Fatalf("galaxy %+v", st)
	}
}

func TestBuildLeavesSampleCoversK(t *testing.T) {
	fake, srv := newFake(t, identity.UnkeyedFingerprint)
	c := testClient(t, srv.URL, "")
	if _, err := c.BuildLeaves(context.Background(), maxLeaves, 1); err != nil {
		t.Fatal(err)
	}
	if got := fake.leaves[0]; got.Sample < got.K {
		t.Fatalf("sample %d < k %d", got.Sample, got.K)
	}
}

func TestIngestRejectionsAreReported(t *testing.T) {
	fake, srv := newFake(t, identity.UnkeyedFingerprint)
	fake.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != pathIngestBatch {
			return false
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"accepted": 1, "skipped": 0, "forgotten": 0,
			"rejected": []map[string]string{{"key": "kb://x", "error": "boom"}}, "embedding_backlog": 0})
		return true
	}
	c := testClient(t, srv.URL, "")
	n, err := c.IngestDocs(context.Background(), []Doc{{URL: "kb://y"}, {URL: "kb://x"}})
	if n != 1 || err == nil || !strings.Contains(err.Error(), "kb://x") {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestIngestSplitsIntoBatchesOf100(t *testing.T) {
	fake, srv := newFake(t, identity.UnkeyedFingerprint)
	c := testClient(t, srv.URL, "")
	docs := make([]Doc, 230)
	for i := range docs {
		docs[i] = Doc{URL: "kb://" + strings.Repeat("d", i%5+1), Title: "t", Text: "x"}
	}
	n, err := c.IngestDocs(context.Background(), docs)
	if err != nil || n != 230 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(fake.ingests) != 3 || len(fake.ingests[2].Documents) != 30 {
		t.Fatalf("batches: %d", len(fake.ingests))
	}
}

func TestNoTokenFileSendsNoAuthorization(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Authorization"))
		writeJSON(w, http.StatusCreated, map[string]any{"memory": map[string]any{"id": "x"}})
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, testWorkspace, filepath.Join(t.TempDir(), "absent"), identity.New(""))
	c.limiter = rate.NewLimiter(rate.Inf, 1)
	if _, err := c.SaveInstitutional(context.Background(), typePolicy, "x", 1); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestLoadPseudonymizer(t *testing.T) {
	t.Setenv(identity.KeyEnv, "")
	t.Setenv(identity.KeyRequiredEnv, "true")
	if _, err := loadPseudonymizer(); err == nil {
		t.Fatal("required key accepted while empty")
	}
	t.Setenv(identity.KeyRequiredEnv, "maybe")
	if _, err := loadPseudonymizer(); err == nil {
		t.Fatal("non-boolean required flag accepted")
	}
	t.Setenv(identity.KeyRequiredEnv, "true")
	t.Setenv(identity.KeyEnv, testKey)
	p, err := loadPseudonymizer()
	if err != nil || !p.Keyed() || p.Fingerprint() != identity.New(testKey).Fingerprint() {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	t.Setenv(identity.KeyRequiredEnv, "")
	t.Setenv(identity.KeyEnv, "")
	if p, err := loadPseudonymizer(); err != nil || p.Keyed() {
		t.Fatalf("unkeyed: p=%+v err=%v", p, err)
	}
}

func TestFatalErrorNamesTheRemedy(t *testing.T) {
	for status, want := range map[int]string{
		http.StatusUnauthorized: TokenPathEnv,
		http.StatusForbidden:    "allowlist",
		http.StatusConflict:     "omnia-workspace-pseudonym-key",
	} {
		msg := (&FatalError{Status: status, Path: "/p", Body: "b"}).Error()
		if !strings.Contains(msg, want) {
			t.Errorf("%d: %q lacks %q", status, msg, want)
		}
	}
	if bytes.Contains([]byte((&FatalError{Status: 409}).Error()), []byte(testKey)) {
		t.Error("error leaks key material")
	}
}
