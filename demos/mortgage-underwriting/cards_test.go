package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func post(t *testing.T, srv *httptest.Server, path, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// A card frozen in one conversation is frozen in the next: verify_identity
// reports the desk's status, and freezing twice says nothing changed.
func TestFreezeIsWhatVerifyReports(t *testing.T) {
	srv := httptest.NewServer(newServer())
	defer srv.Close()
	ellen := `{"ssn":"123-45-6789","dob":"1988-03-14"}`

	if _, got := post(t, srv, "/verify-identity", ellen); got["card_status"] != "active" {
		t.Fatalf("before: %v", got)
	}
	code, got := post(t, srv, "/card-freeze", `{"customer_ref":"MEM-4471-889"}`)
	if code != http.StatusOK || got["card_status"] != "frozen" || got["changed"] != true {
		t.Fatalf("freeze: %d %v", code, got)
	}
	if _, got := post(t, srv, "/verify-identity", ellen); got["card_status"] != "frozen" {
		t.Fatalf("after freeze: %v", got)
	}
	if _, got := post(t, srv, "/card-freeze", `{"customer_ref":"MEM-4471-889"}`); got["changed"] != false {
		t.Fatalf("second freeze: %v", got)
	}
	if _, got := post(t, srv, "/card-unfreeze", `{"customer_ref":"MEM-4471-889"}`); got["card_status"] != "active" || got["changed"] != true {
		t.Fatalf("unfreeze: %v", got)
	}
}

// The freeze tools take a customer reference only, and refuse one they do not
// know.
func TestFreezeNeedsAKnownReference(t *testing.T) {
	srv := httptest.NewServer(newServer())
	defer srv.Close()
	for _, body := range []string{`{}`, `{"customer_ref":"MEM-0000-000"}`, `{"ssn":"123-45-6789"}`} {
		if code, _ := post(t, srv, "/card-freeze", body); code != http.StatusNotFound {
			t.Errorf("%s: status %d", body, code)
		}
	}
}
