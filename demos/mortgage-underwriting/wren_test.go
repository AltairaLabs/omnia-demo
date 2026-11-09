package main

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestWren(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	newWren(func() time.Time { return time.Date(2026, 11, 9, 14, 18, 0, 0, time.UTC) }).register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func wrenPost(t *testing.T, srv *httptest.Server, path, user string, body any) (map[string]any, int) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(raw))
	if user != "" {
		req.Header.Set("X-Omnia-User-Id", user)
	}
	req.Header.Set("X-Omnia-Session-Id", "ses-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("%s: decode: %v", path, err)
	}
	return out, resp.StatusCode
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// The portal's numbers, from the account system.
func TestBillMatchesThePortal(t *testing.T) {
	srv := newTestWren(t)
	out, status := wrenPost(t, srv, "/wren/bill", wrenAccountID, map[string]any{"period": "2026-10"})
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	b := out["bill"].(map[string]any)
	if b["total_gbp"].(float64) != 84.10 || b["kwh"].(float64) != 312.4 {
		t.Fatalf("bill %v", b)
	}
	var sum float64
	for _, l := range b["lines"].([]any) {
		sum += l.(map[string]any)["amount_gbp"].(float64)
	}
	if !near(sum, 84.10, 0.001) {
		t.Fatalf("lines sum to %v", sum)
	}
	prev := out["previous"].(map[string]any)
	if prev["change_gbp"].(float64) != 14.20 || prev["change_kwh"].(float64) != 38 {
		t.Fatalf("change on September %v", prev)
	}
}

func TestOctoberUsageSumsToTheBill(t *testing.T) {
	first, _ := time.Parse(time.DateOnly, wrenUsageStart)
	var oct, cold float64
	for i, v := range wrenDaily {
		d := first.AddDate(0, 0, i)
		if d.Format("2006-01") == "2026-10" {
			oct += v
			if d.Day() >= 20 && d.Day() <= 24 {
				cold += v - 9.2
			}
		}
	}
	if !near(oct, 312.4, 0.001) {
		t.Fatalf("October usage %v, bill says 312.4", oct)
	}
	if cold < 38/2 {
		t.Fatalf("the cold spell should be most of the 38 kWh rise, got %v", cold)
	}
	if last := first.AddDate(0, 0, wrenUsageDays-1).Format(time.DateOnly); last != wrenToday {
		t.Fatalf("usage ends %s, want %s", last, wrenToday)
	}
}

func TestUsageIsHalfHourly(t *testing.T) {
	srv := newTestWren(t)
	out, _ := wrenPost(t, srv, "/wren/usage", "", map[string]any{"days": 60})
	if days := out["days"].([]any); len(days) != 60 {
		t.Fatalf("days %d", len(days))
	}
	one, _ := wrenPost(t, srv, "/wren/usage", "", map[string]any{"days": 1})
	today := one["days"].([]any)[0].(map[string]any)
	if len(today["half_hourly_kwh"].([]any)) != wrenTodaySlots || today["partial"] != true {
		t.Fatalf("today %v", today)
	}
}

func TestWeatherShowsTheColdNights(t *testing.T) {
	srv := newTestWren(t)
	out, status := wrenPost(t, srv, "/wren/weather", "", map[string]any{"postcode": "HX7", "from": "2026-10-20", "to": "2026-10-22"})
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if d := out["days"].([]any)[1].(map[string]any); d["overnight_min_c"].(float64) != 3 {
		t.Fatalf("21 Oct %v", d)
	}
	if _, status := wrenPost(t, srv, "/wren/weather", "", map[string]any{"postcode": "LS1"}); status != http.StatusNotFound {
		t.Fatalf("outside the area: status %d", status)
	}
}

func TestFlexQuoteIsAboutSeventyFive(t *testing.T) {
	srv := newTestWren(t)
	out, _ := wrenPost(t, srv, "/wren/quote", "", map[string]any{"tariff": "moor-flex"})
	est := out["october_estimate"].(map[string]any)["total_gbp"].(float64)
	if est < 74 || est > 76 {
		t.Fatalf("Moor Flex October estimate %v, the portal says about £75", est)
	}
	if s := out["saving_vs_actual_gbp"].(float64); s < 8.5 || s > 10 {
		t.Fatalf("saving %v, the portal says about £9", s)
	}
	steady, _ := wrenPost(t, srv, "/wren/quote", "", map[string]any{"tariff": "moor-steady"})
	if got := steady["october_estimate"].(map[string]any)["total_gbp"].(float64); !near(got, 84.10, 0.02) {
		t.Fatalf("Moor Steady re-prices October at %v, want 84.10", got)
	}
}

// The gate: no consent, someone else's consent, a consent for a different
// change, and a used consent are all refused; the customer's own consent
// switches once.
func TestSwitchNeedsTheCustomersConsent(t *testing.T) {
	srv := newTestWren(t)
	sw := map[string]any{"tariff": "moor-flex", "effective_date": "2026-12-01"}

	if _, status := wrenPost(t, srv, "/wren/switch", wrenAccountID, sw); status != http.StatusForbidden {
		t.Fatalf("no consent: status %d", status)
	}
	sw["consent_id"] = "CNS-MADEUP"
	if _, status := wrenPost(t, srv, "/wren/switch", wrenAccountID, sw); status != http.StatusForbidden {
		t.Fatalf("invented consent: status %d", status)
	}

	cons, status := wrenPost(t, srv, "/wren/consent", wrenAccountID, map[string]any{"tariff": "moor-flex", "effective_date": "2026-12-01", "session_id": "ses-1"})
	if status != http.StatusOK {
		t.Fatalf("consent: status %d", status)
	}
	sw["consent_id"] = cons["consent_id"]

	if _, status := wrenPost(t, srv, "/wren/switch", "99999999", sw); status != http.StatusNotFound {
		t.Fatalf("another customer: status %d", status)
	}
	other := map[string]any{"tariff": "moor-flex", "effective_date": "2027-01-01", "consent_id": cons["consent_id"]}
	if _, status := wrenPost(t, srv, "/wren/switch", wrenAccountID, other); status != http.StatusForbidden {
		t.Fatalf("different change: status %d", status)
	}

	out, status := wrenPost(t, srv, "/wren/switch", wrenAccountID, sw)
	if status != http.StatusOK || out["status"] != "scheduled" {
		t.Fatalf("switch: status %d, %v", status, out)
	}
	if _, status := wrenPost(t, srv, "/wren/switch", wrenAccountID, sw); status != http.StatusConflict {
		t.Fatalf("reused consent: status %d", status)
	}
	bill, _ := wrenPost(t, srv, "/wren/bill", wrenAccountID, map[string]any{})
	if bill["customer"].(map[string]any)["tariff"] != "moor-flex" {
		t.Fatal("the switch should show on the account")
	}
}

func TestReceiptsRecordEveryCall(t *testing.T) {
	srv := newTestWren(t)
	wrenPost(t, srv, "/wren/bill", wrenAccountID, map[string]any{"period": "2026-10"})
	wrenPost(t, srv, "/wren/switch", wrenAccountID, map[string]any{"tariff": "moor-flex"})

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/wren/receipts?session=ses-1", nil)
	req.Header.Set("X-Omnia-User-Id", wrenAccountID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Receipts []wrenReceipt `json:"receipts"`
		LastSeq  int           `json:"last_seq"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Receipts) != 2 || out.LastSeq != 2 {
		t.Fatalf("receipts %+v", out)
	}
	if r := out.Receipts[0]; r.Tool != "read_bill" || r.Detail != "2026-10" || r.Outcome != "read · allowed" {
		t.Fatalf("first receipt %+v", r)
	}
	if r := out.Receipts[1]; r.Tool != "switch_tariff" || r.Outcome != "write · refused" {
		t.Fatalf("refused switch receipt %+v", r)
	}

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/wren/receipts?after=2", nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp2.Body.Close() }()
	out.Receipts = nil
	_ = json.NewDecoder(resp2.Body).Decode(&out)
	if len(out.Receipts) != 0 {
		t.Fatalf("after=2 should be empty, got %+v", out.Receipts)
	}
}
