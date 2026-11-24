package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// wrenAccountEnv is a Wrenmoor account server on a fixed clock.
type wrenAccountEnv struct {
	t   *testing.T
	f   *fakeIssuer
	clk *testClock
	srv http.Handler
}

func newWrenAccountEnv(t *testing.T) *wrenAccountEnv {
	t.Helper()
	f, clk := newFakeIssuer(t), newTestClock()
	known, err := accountHolders(accountAPIWrenmoor)
	if err != nil {
		t.Fatal(err)
	}
	v := newOBOVerifier(f.issuer(), testAudience, known, f.srv.Client(), clk.now)
	srv := newServerWith(accountConfig{api: accountAPIWrenmoor, verifier: v, wren: newWren(clk.now)})
	return &wrenAccountEnv{t: t, f: f, clk: clk, srv: srv}
}

func (e *wrenAccountEnv) as(email, path string) (int, map[string]any) {
	e.t.Helper()
	c := e.f.claims(e.clk.now())
	c["email"] = email
	c["scope"] = "outages"
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+e.f.mint(c))
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		e.t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	return rec.Code, out
}

func TestCompensationPence(t *testing.T) {
	for _, tc := range []struct{ hours, want int }{
		{0, 0}, {11, 0}, {12, 9500}, {23, 9500}, {24, 14000}, {35, 14000}, {36, 18500},
	} {
		if got := compensationPence(tc.hours); got != tc.want {
			t.Errorf("compensationPence(%d) = %d, want %d", tc.hours, got, tc.want)
		}
	}
}

func TestMyOutageForEllen(t *testing.T) {
	e := newWrenAccountEnv(t)
	code, out := e.as(ellenEmail, "/api/account/outage")
	if code != http.StatusOK {
		t.Fatalf("status %d, body %v", code, out)
	}
	if out["district"] != "HX7" || out["status"] != "off" || out["account_id"] != wrenEllen.AccountID || out["postcode"] != wrenEllen.Postcode {
		t.Errorf("answer %v", out)
	}
	started, err := time.Parse(time.RFC3339, out["started"].(string))
	if err != nil {
		t.Fatal(err)
	}
	want := int(e.clk.now().Sub(started) / time.Hour)
	if out["hours_off"] != float64(want) {
		t.Errorf("hours_off = %v, want %d", out["hours_off"], want)
	}
	// The same outage outage_status reports.
	o, _ := newWren(e.clk.now).outage("HX7")
	if out["started"] != o.Started || out["estimated_restoration"] != o.Estimated {
		t.Errorf("times %v / %v differ from the outage feed %s / %s", out["started"], out["estimated_restoration"], o.Started, o.Estimated)
	}
}

func TestCompensationOwedToEllen(t *testing.T) {
	e := newWrenAccountEnv(t)
	_, outage := e.as(ellenEmail, "/api/account/outage")
	code, out := e.as(ellenEmail, "/api/account/compensation")
	if code != http.StatusOK {
		t.Fatalf("status %d, body %v", code, out)
	}
	hours := int(outage["hours_off"].(float64))
	if out["hours_off"] != outage["hours_off"] || out["status"] != "owed" || out["eligible"] != true {
		t.Errorf("answer %v", out)
	}
	if out["amount_pence"] != float64(compensationPence(hours)) || hours < 12 {
		t.Errorf("amount_pence = %v for %d hours", out["amount_pence"], hours)
	}
	if s, _ := out["rule"].(string); s == "" {
		t.Error("rule is empty")
	}
}

func TestCompensationNotYetWithNoOutage(t *testing.T) {
	// A customer outside the outage district has nothing to be compensated.
	cust := wrenCustomer{AccountID: "x", Postcode: "LS1 4AP"}
	res := newWren(newTestClock().now).compensationFor(cust)
	if res.Status != "not_yet" || res.Eligible || res.AmountPence != 0 {
		t.Errorf("no outage: %+v", res)
	}
}

func TestMyOutageServesTokenCustomerOnly(t *testing.T) {
	e := newWrenAccountEnv(t)
	_, out := e.as("amira.saleem@example.com", "/api/account/outage")
	if out["account_id"] != wrenAccountID || out["account_id"] == wrenEllen.AccountID {
		t.Errorf("Amira was served %v", out["account_id"])
	}
	_, out = e.as(ellenEmail, "/api/account/outage")
	if out["account_id"] != wrenEllen.AccountID {
		t.Errorf("Ellen was served %v", out["account_id"])
	}
}

func TestAccountOutageNoOutageInDistrict(t *testing.T) {
	w := newWren(newTestClock().now)
	res, ok := w.outageFor(wrenCustomer{AccountID: "1", Postcode: "LS1 4AP"})
	if ok || res["status"] != "on" {
		t.Errorf("got %v, %v", res, ok)
	}
}

func TestAccountRoutesRefuseUnknownMemberAndNoToken(t *testing.T) {
	e := newWrenAccountEnv(t)
	if code, _ := e.as("nobody@example.com", "/api/account/outage"); code != http.StatusNotFound {
		t.Errorf("unknown member: %d", code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/account/compensation", strings.NewReader(`{}`))
	req.Header.Set("X-Omnia-User-Email", ellenEmail)
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("header only: %d", rec.Code)
	}
}
