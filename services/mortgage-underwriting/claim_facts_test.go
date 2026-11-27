package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The ToolPolicy's http producer asks the claims system for a claim's peril,
// in the broker's request shape, and reads {"value": ...} back.
func claimPeril(t *testing.T, srv *httptest.Server, args string) (int, map[string]any) {
	t.Helper()
	body := `{"producer":"claimPeril","tool":{"registry":"claims-triage-tools","name":"claim_settle"},"args":` + args + `,"run":{}}`
	resp, err := http.Post(srv.URL+"/policy/claim-peril", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, got
}

func TestClaimPerilAnswersThePolicyWithTheClaimsPeril(t *testing.T) {
	srv := httptest.NewServer(newServer())
	defer srv.Close()
	code, got := claimPeril(t, srv, `{"claim_ref":"LM-40751","amount_pence":42000}`)
	if code != http.StatusOK || got["value"] != "Flood" {
		t.Fatalf("flood claim: %d %v", code, got)
	}
}

// A call that names no claim, or one the system doesn't hold, has no peril.
// The answer is still a string, so the policy compiles and the producer never
// fails a call that has nothing to do with perils.
func TestClaimPerilIsEmptyWithoutAKnownClaim(t *testing.T) {
	srv := httptest.NewServer(newServer())
	defer srv.Close()
	for _, args := range []string{`{}`, `{"claim_ref":"LM-00000"}`} {
		code, got := claimPeril(t, srv, args)
		if code != http.StatusOK || got["value"] != "" {
			t.Fatalf("%s: %d %v", args, code, got)
		}
	}
}
