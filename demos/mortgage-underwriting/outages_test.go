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
	// newTestWren's clock is 2026-11-09 14:18 UTC: restoration is 14:18+3h
	// truncated to the hour, and the cut started 18h20m before that.
	out, status := outagePost(t, "/wren/outage-status", map[string]any{"postcode": "HX7 8AD"})
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, out)
	}
	if out["status"] != "off" || out["district"] != "HX7" {
		t.Fatalf("outage %v", out)
	}
	if out["estimated_restoration"] != "2026-11-09T17:00:00Z" || out["started"] != "2026-11-08T22:40:00Z" {
		t.Fatalf("started %v, estimated_restoration %v", out["started"], out["estimated_restoration"])
	}
}

// The times come from the clock: right on the day, still within a take, and
// moved on once the estimate is less than an hour away.
func TestOutageTimesFollowTheClock(t *testing.T) {
	now := time.Date(2026, 11, 23, 9, 47, 0, 0, time.UTC)
	w := newWren(func() time.Time { return now })
	at := func() (string, string) {
		o, ok := w.outage("HX7")
		if !ok {
			t.Fatal("no HX7 outage")
		}
		return o.Started, o.Estimated
	}
	started, eta := at()
	if eta != "2026-11-23T12:00:00Z" || started != "2026-11-22T17:40:00Z" {
		t.Fatalf("at 09:47: started %s, estimated %s", started, eta)
	}
	now = now.Add(40 * time.Minute) // 10:27, the same take
	if _, e := at(); e != eta {
		t.Fatalf("at 10:27 the estimate moved from %s to %s within a take", eta, e)
	}
	now = time.Date(2026, 11, 23, 11, 5, 0, 0, time.UTC) // under an hour to go
	if s, e := at(); e != "2026-11-23T14:00:00Z" || s != "2026-11-22T19:40:00Z" {
		t.Fatalf("at 11:05: started %s, estimated %s", s, e)
	}
	now = time.Date(2026, 11, 30, 8, 30, 0, 0, time.UTC) // a week later
	if _, e := at(); e != "2026-11-30T11:00:00Z" {
		t.Fatalf("a week later: estimated %s", e)
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
