package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const gwenEmail = "gwen.ashby@example.com"

// homeClaimEnv is a Larkmere account server and a way to call it as a member.
type homeClaimEnv struct {
	t   *testing.T
	f   *fakeIssuer
	clk *testClock
	srv http.Handler
}

func newHomeClaimEnv(t *testing.T) *homeClaimEnv {
	t.Helper()
	f, clk := newFakeIssuer(t), newTestClock()
	srv := newServerWith(accountConfig{api: accountAPILarkmere, verifier: newTestVerifier(f, clk)})
	return &homeClaimEnv{t: t, f: f, clk: clk, srv: srv}
}

// as posts body to path with a token for email, and decodes the answer.
func (e *homeClaimEnv) as(email, path, body string) (int, map[string]any) {
	e.t.Helper()
	c := e.f.claims(e.clk.now())
	c["email"] = email
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+e.f.mint(c))
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		e.t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	return rec.Code, out
}

const openBody = `{"peril":"power cut","description":"Freezer and kitchen lost power","occurred_on":"2026-11-20"}`

func (e *homeClaimEnv) open(email string) map[string]any {
	e.t.Helper()
	code, out := e.as(email, "/api/claims/open", openBody)
	if code != http.StatusCreated {
		e.t.Fatalf("open: status %d, body %v", code, out)
	}
	return out
}

func TestClaimOpenForSignedInMember(t *testing.T) {
	e := newHomeClaimEnv(t)
	out := e.open(ellenEmail)
	want := map[string]any{
		"claim_ref":   "LM-41001",
		"policy_ref":  "LM-HOME-4471",
		"member_name": "Ellen Whitcomb",
		"peril":       "power cut",
		"occurred_on": "2026-11-20",
		"status":      "reported",
	}
	for k, v := range want {
		if out[k] != v {
			t.Errorf("%s = %v, want %v", k, out[k], v)
		}
	}
	if items, ok := out["items"].([]any); !ok || len(items) != 0 {
		t.Errorf("items = %v, want []", out["items"])
	}
	if out["claimed_pence"] != float64(0) {
		t.Errorf("claimed_pence = %v, want 0", out["claimed_pence"])
	}
	if s, _ := out["next_steps"].(string); s == "" {
		t.Error("next_steps is empty")
	}
	if s, _ := out["reported_on"].(string); s != "2026-11-24" {
		t.Errorf("reported_on = %q, want the clock's date", s)
	}
	if again := e.open(ellenEmail); again["claim_ref"] != "LM-41002" {
		t.Errorf("second claim_ref = %v, want LM-41002", again["claim_ref"])
	}
}

func TestClaimOpenWithoutHomePolicyIsNotFound(t *testing.T) {
	e := newHomeClaimEnv(t)
	// Tomasz is a Larkmere member with no home policy.
	code, out := e.as("tomasz.brennan@example.com", "/api/claims/open", openBody)
	if code != http.StatusNotFound || out["error"] != "no home policy for this member" {
		t.Errorf("got %d %v", code, out)
	}
}

func TestClaimAddItemSumsPence(t *testing.T) {
	e := newHomeClaimEnv(t)
	ref := e.open(ellenEmail)["claim_ref"].(string)
	code, out := e.as(ellenEmail, "/api/claims/add-item",
		`{"claim_ref":"`+ref+`","description":"kitchen","value_pence":120000}`)
	if code != http.StatusOK {
		t.Fatalf("status %d, body %v", code, out)
	}
	code, out = e.as(ellenEmail, "/api/claims/add-item",
		`{"claim_ref":"`+ref+`","description":"freezer contents","value_pence":35050,"cause":"power off 19 hours (Wrenmoor Energy)"}`)
	if code != http.StatusOK {
		t.Fatalf("status %d, body %v", code, out)
	}
	if out["claimed_pence"] != float64(155050) {
		t.Errorf("claimed_pence = %v, want 155050", out["claimed_pence"])
	}
	items, _ := out["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %v, want 2", out["items"])
	}
	if cause := items[1].(map[string]any)["cause"]; cause != "power off 19 hours (Wrenmoor Energy)" {
		t.Errorf("cause = %v", cause)
	}
}

func TestClaimAddItemRejectsBadItems(t *testing.T) {
	e := newHomeClaimEnv(t)
	ref := e.open(ellenEmail)["claim_ref"].(string)
	for _, body := range []string{
		`{"claim_ref":"` + ref + `","description":"tv","value_pence":0}`,
		`{"claim_ref":"` + ref + `","description":"tv","value_pence":-5}`,
		`{"claim_ref":"` + ref + `","description":"","value_pence":500}`,
	} {
		if code, out := e.as(ellenEmail, "/api/claims/add-item", body); code != http.StatusBadRequest {
			t.Errorf("%s: status %d %v, want 400", body, code, out)
		}
	}
}

func TestClaimAddItemOtherMembersClaimIsNotFound(t *testing.T) {
	e := newHomeClaimEnv(t)
	ref := e.open(ellenEmail)["claim_ref"].(string)
	item := func(r string) string { return `{"claim_ref":"` + r + `","description":"tv","value_pence":500}` }
	c1, o1 := e.as(gwenEmail, "/api/claims/add-item", item(ref))
	c2, o2 := e.as(gwenEmail, "/api/claims/add-item", item("LM-49999"))
	if c1 != http.StatusNotFound || c2 != http.StatusNotFound {
		t.Fatalf("statuses %d, %d, want 404", c1, c2)
	}
	if o1["error"] != "claim not found" || o1["error"] != o2["error"] {
		t.Errorf("answers differ: %v vs %v", o1, o2)
	}
}

func TestClaimAddItemNotReportedIsConflict(t *testing.T) {
	f, clk := newFakeIssuer(t), newTestClock()
	hc := newHomeClaims(clk.now)
	mux := http.NewServeMux()
	hc.register(mux, newTestVerifier(f, clk))
	e := &homeClaimEnv{t: t, f: f, clk: clk, srv: mux}
	ref := e.open(ellenEmail)["claim_ref"].(string)
	// Only reported claims take items; move this one on directly.
	hc.setStatus(ref, "settled")
	code, _ := e.as(ellenEmail, "/api/claims/add-item",
		`{"claim_ref":"`+ref+`","description":"tv","value_pence":500}`)
	if code != http.StatusConflict {
		t.Errorf("status %d, want 409", code)
	}
}

func TestClaimStatusListsOnlyOwnClaims(t *testing.T) {
	e := newHomeClaimEnv(t)
	ellenRef := e.open(ellenEmail)["claim_ref"].(string)
	e.open(ellenEmail)
	gwenRef := e.open(gwenEmail)["claim_ref"].(string)

	code, out := e.as(ellenEmail, "/api/claims/status", `{}`)
	if code != http.StatusOK {
		t.Fatalf("status %d %v", code, out)
	}
	if list, _ := out["claims"].([]any); len(list) != 2 {
		t.Errorf("claims = %v, want Ellen's two", out["claims"])
	}
	code, out = e.as(ellenEmail, "/api/claims/status", `{"claim_ref":"`+ellenRef+`"}`)
	if code != http.StatusOK || out["claim_ref"] != ellenRef {
		t.Errorf("single: %d %v", code, out)
	}
	if code, _ = e.as(ellenEmail, "/api/claims/status", `{"claim_ref":"`+gwenRef+`"}`); code != http.StatusNotFound {
		t.Errorf("Gwen's claim as Ellen: status %d, want 404", code)
	}
	if code, out = e.as(gwenEmail, "/api/claims/status", `{}`); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if list, _ := out["claims"].([]any); len(list) != 1 {
		t.Errorf("Gwen's claims = %v, want one", out["claims"])
	}
}

func TestClaimRoutesRefuseUnauthenticated(t *testing.T) {
	e := newHomeClaimEnv(t)
	for _, p := range []string{"/api/claims/open", "/api/claims/add-item", "/api/claims/status"} {
		req := httptest.NewRequest(http.MethodPost, p, strings.NewReader(openBody))
		req.Header.Set("X-Omnia-User-Email", ellenEmail)
		rec := httptest.NewRecorder()
		e.srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", p, rec.Code)
		}
	}
}

func TestClaimRoutesAbsentWithoutAccountAPI(t *testing.T) {
	for _, p := range []string{"/api/claims/open", "/api/claims/add-item", "/api/claims/status"} {
		req := httptest.NewRequest(http.MethodPost, p, strings.NewReader(openBody))
		rec := httptest.NewRecorder()
		newServer().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", p, rec.Code)
		}
	}
}

func TestHomeClaimsConcurrentOpens(t *testing.T) {
	hc := newHomeClaims(newTestClock().now)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hc.open(ellenEmail, "power cut", "x", "2026-11-20")
		}()
	}
	wg.Wait()
	if n := len(hc.list(ellenEmail)); n != 20 {
		t.Errorf("claims = %d, want 20", n)
	}
}
