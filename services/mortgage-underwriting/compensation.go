package main

// Wrenmoor's outage and compensation, for the signed-in customer.
//
// STAND-IN: Wrenmoor's guaranteed standards of service. The compensation rule
// is Wrenmoor's own fictional standard, not a regulator's: £95 once the supply
// has been off 12 hours, then £45 for each further full 12 hours, paid
// automatically within 28 days. A real supplier would calculate this from its
// network operator's confirmed interruption record and the customer's
// premises, and would pay it through billing. Here the hours come from the
// same fictional HX7 outage the public outage feed reports (outages.go), so
// they follow the clock and the outage never ends. Downstream: "owed" is a
// statement, not a payment: nothing is paid or credited to the account, and
// a customer outside HX7 has no outage and nothing owed. The customer is the
// verified email of the exchanged token (obo.go) and nothing else; there is no
// input to bound, as the amount is computed from the clock, never taken from
// the caller. Money is in pence.

import (
	"net/http"
	"strings"
	"time"
)

const (
	compensationFirstHours = 12
	compensationFirstPence = 9500
	compensationStepHours  = 12
	compensationStepPence  = 4500
	compensationRule       = "Wrenmoor guaranteed standard: £95 once the supply has been off 12 hours, then £45 for each further full 12 hours, paid automatically within 28 days."
)

// compensationPence is what a supply off for hoursOff whole hours is owed.
func compensationPence(hoursOff int) int {
	if hoursOff < compensationFirstHours {
		return 0
	}
	return compensationFirstPence + (hoursOff-compensationFirstHours)/compensationStepHours*compensationStepPence
}

type compensation struct {
	AccountID   string `json:"account_id"`
	HoursOff    int    `json:"hours_off"`
	Eligible    bool   `json:"eligible"`
	AmountPence int    `json:"amount_pence"`
	Rule        string `json:"rule"`
	Status      string `json:"status"`
}

// outageFor is the customer's outage, from their postcode's district, or
// {"status":"on"} and false when there is none.
func (w *wren) outageFor(c wrenCustomer) (map[string]any, bool) {
	district := postcodeDistrict(c.Postcode)
	o, ok := w.outage(district)
	if !ok {
		return map[string]any{"status": "on"}, false
	}
	started, err := time.Parse(time.RFC3339, o.Started)
	hours := 0
	if err == nil {
		hours = int(w.now().Sub(started) / time.Hour)
	}
	if hours < 0 {
		hours = 0
	}
	return map[string]any{
		"account_id": c.AccountID, "postcode": c.Postcode, "district": district,
		"status": o.Status, "started": o.Started, "hours_off": hours,
		"estimated_restoration": o.Estimated,
	}, true
}

func (w *wren) compensationFor(c wrenCustomer) compensation {
	out := compensation{AccountID: c.AccountID, Rule: compensationRule, Status: "not_yet"}
	if o, ok := w.outageFor(c); ok {
		out.HoursOff = o["hours_off"].(int)
	}
	out.AmountPence = compensationPence(out.HoursOff)
	if out.AmountPence > 0 {
		out.Eligible, out.Status = true, "owed"
	}
	return out
}

// registerWrenAccount adds the routes, each guarded by v with the outages
// scope.
func registerWrenAccount(mux *http.ServeMux, v *oboVerifier, w *wren) {
	mux.HandleFunc("/api/account/outage", v.guard("my_outage", "outages", func(member string, rw http.ResponseWriter, _ *http.Request) {
		c, ok := wrenAccounts[strings.ToLower(member)]
		if !ok {
			writeAccountError(rw, http.StatusNotFound, errNoAccount)
			return
		}
		out, _ := w.outageFor(c)
		writeAccountJSON(rw, http.StatusOK, out)
	}))
	mux.HandleFunc("/api/account/compensation", v.guard("compensation_status", "outages", func(member string, rw http.ResponseWriter, _ *http.Request) {
		c, ok := wrenAccounts[strings.ToLower(member)]
		if !ok {
			writeAccountError(rw, http.StatusNotFound, errNoAccount)
			return
		}
		writeAccountJSON(rw, http.StatusOK, w.compensationFor(c))
	}))
}
