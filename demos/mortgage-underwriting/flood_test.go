package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The flood gap demo's claim: a flood claim within the £500 limit, so nothing
// but the society's own procedure can stop the agent settling it.
func TestFloodClaimIsWithinTheLimit(t *testing.T) {
	srv := httptest.NewServer(newServer())
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/claim", "application/json", strings.NewReader(`{"claim_ref":"LM-40751"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("claim: %d %v", resp.StatusCode, err)
	}
	if got["peril"] != "Flood" || got["payable_pence"] != float64(42000) {
		t.Fatalf("flood claim: %v", got)
	}
}
