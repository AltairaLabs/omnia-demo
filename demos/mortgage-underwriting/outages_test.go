package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// outagePost posts to the public outage route, with no member header.
func outagePost(t *testing.T, path string, body any) (map[string]any, int) {
	t.Helper()
	srv := newTestWren(t)
	raw, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out, resp.StatusCode
}

func TestOutageStatusHX7(t *testing.T) {
	out, status := outagePost(t, "/wren/outage-status", map[string]any{"postcode": "HX7 8AD"})
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, out)
	}
	if out["status"] != "off" || out["district"] != "HX7" {
		t.Fatalf("outage %v", out)
	}
	started, err1 := time.Parse(time.RFC3339, out["started"].(string))
	eta, err2 := time.Parse(time.RFC3339, out["estimated_restoration"].(string))
	if err1 != nil || err2 != nil || !eta.After(started) {
		t.Fatalf("started %v, estimated_restoration %v", out["started"], out["estimated_restoration"])
	}
}

func TestOutageStatusUnknownDistrict(t *testing.T) {
	out, status := outagePost(t, "/wren/outage-status", map[string]any{"postcode": "LS1 4AP"})
	if status != http.StatusNotFound {
		t.Fatalf("status %d: %v", status, out)
	}
	if out["error"] != "no outage data for LS1" {
		t.Fatalf("error %v", out["error"])
	}
}

// Outage status is public: no X-Omnia-User-Email, no signed-in member.
func TestOutageStatusNeedsNoMember(t *testing.T) {
	if _, status := outagePost(t, "/wren/outage-status", map[string]any{"postcode": "hx7 8ad"}); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
}

// Ellen is a Wrenmoor customer in the storm's district, so the outage
// covers her; Amira's account is untouched.
func TestEllenIsAWrenmoorCustomerInHX7(t *testing.T) {
	if wrenEllen.Postcode[:3] != "HX7" || wrenEllen.AccountID == wrenAccountID {
		t.Fatalf("ellen %+v", wrenEllen)
	}
}
