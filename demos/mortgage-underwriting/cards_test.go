package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cardCall POSTs to a card route as member email ("" sends no member).
func cardCall(t *testing.T, srv *httptest.Server, path, email string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(`{}`))
	if email != "" {
		req.Header.Set("X-Omnia-User-Email", email)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

const ellen = "ellen.whitcomb@example.com"

// The card is the signed-in member's: the runtime forwards their email on
// the tool call, and a freeze shows in the next status check.
func TestFreezeIsThePortalMembers(t *testing.T) {
	srv := httptest.NewServer(newServer())
	defer srv.Close()

	if _, got := cardCall(t, srv, "/card-status", ellen); got["card_status"] != "active" || got["card_last4"] != "4471" {
		t.Fatalf("before: %v", got)
	}
	code, got := cardCall(t, srv, "/card-freeze", ellen)
	if code != http.StatusOK || got["card_status"] != "frozen" || got["changed"] != true {
		t.Fatalf("freeze: %d %v", code, got)
	}
	if _, got := cardCall(t, srv, "/card-status", ellen); got["card_status"] != "frozen" {
		t.Fatalf("after freeze: %v", got)
	}
	if _, got := cardCall(t, srv, "/card-freeze", ellen); got["changed"] != false {
		t.Fatalf("second freeze: %v", got)
	}
	if _, got := cardCall(t, srv, "/card-unfreeze", ellen); got["card_status"] != "active" || got["changed"] != true {
		t.Fatalf("unfreeze: %v", got)
	}
}

// One member's freeze never touches another's card.
func TestMembersAreSeparate(t *testing.T) {
	srv := httptest.NewServer(newServer())
	defer srv.Close()
	if code, _ := cardCall(t, srv, "/card-freeze", ellen); code != http.StatusOK {
		t.Fatalf("freeze: %d", code)
	}
	if _, got := cardCall(t, srv, "/card-status", "tomasz.brennan@example.com"); got["card_status"] != "active" || got["card_last4"] != "2041" {
		t.Fatalf("Tomasz: %v", got)
	}
}

// A call with no verified member, or an unknown one, acts on no card.
func TestNoMemberNoCard(t *testing.T) {
	srv := httptest.NewServer(newServer())
	defer srv.Close()
	for _, path := range []string{"/card-status", "/card-freeze", "/card-unfreeze"} {
		code, got := cardCall(t, srv, path, "")
		if code != http.StatusUnauthorized || got["error"] != "no verified member on this call" {
			t.Errorf("%s with no member: %d %v", path, code, got)
		}
		if code, _ := cardCall(t, srv, path, "someone@else.example"); code != http.StatusNotFound {
			t.Errorf("%s for an unknown member: %d", path, code)
		}
	}
}
