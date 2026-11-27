package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// STAND-IN: Wrenmoor's network outage feed.
//
// Fakes: the distribution network operator's live outage data, here one
// storm outage on the 11kV line at Mytholm, covering postcode district HX7,
// that is always in progress. Its times are relative to the clock (w.now):
// restoration is estimated for the top of the hour about three hours from
// now, and the cut started 18h20m before that.
// Real system: the operator's outage API or the supplier's outage service,
// which changes as crews restore supply.
// Downstream: the status stays "off" and the estimate never arrives: it is
// re-anchored about three hours ahead once it is less than an hour away (see
// outageETA). So the times are right on the day the demo is given, and hold
// still for a take, but the outage never ends. Anything that wants to show
// an outage ending needs this stand-in to move on.
//
// Outage status is public, as it is on a real outage page: the route needs no
// signed-in member and records no receipt, so it bypasses w.tool.

type outage struct {
	District  string  `json:"district"`
	Status    string  `json:"status"` // "off" or "restored"
	Started   string  `json:"started"`
	Estimated string  `json:"estimated_restoration"`
	Restored  *string `json:"restored"`
	Customers int     `json:"customers_affected"`
	Cause     string  `json:"cause"`
}

const (
	// outageAhead is how far ahead restoration is estimated, before the
	// estimate is truncated to the hour.
	outageAhead = 3 * time.Hour
	// outageLength is how long before the estimate the cut started.
	outageLength = 18*time.Hour + 20*time.Minute
	// outageReanchor is how close the estimate may come before it moves on.
	outageReanchor = time.Hour
)

// outageETA is the HX7 outage's estimated restoration: now + 3h truncated to
// the hour, kept until it is less than an hour away so it holds still for a
// take, then taken afresh from the clock.
func (w *wren) outageETA() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now().UTC()
	if w.eta.IsZero() || !now.Before(w.eta.Add(-outageReanchor)) {
		w.eta = now.Add(outageAhead).Truncate(time.Hour)
	}
	return w.eta
}

// outage is the district's outage, or false when there is none.
func (w *wren) outage(district string) (outage, bool) {
	if district != "HX7" {
		return outage{}, false
	}
	eta := w.outageETA()
	return outage{
		District: "HX7", Status: "off",
		Started:   eta.Add(-outageLength).Format(time.RFC3339),
		Estimated: eta.Format(time.RFC3339),
		Customers: 1840, Cause: "storm damage to the 11kV line at Mytholm",
	}, true
}

// postcodeDistrict reduces "HX7 8AD" (or "hx78ad") to "HX7".
func postcodeDistrict(postcode string) string {
	pc := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(postcode), " ", ""))
	if len(pc) > 3 && pc[len(pc)-3] >= '0' && pc[len(pc)-3] <= '9' {
		pc = pc[:len(pc)-3] // a full postcode: drop the inward code
	}
	return pc
}

// outageStatus answers POST /wren/outage-status for anyone.
func (w *wren) outageStatus(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req wrenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(rw, "invalid request body", http.StatusBadRequest)
		return
	}
	district := postcodeDistrict(req.Postcode)
	var body any
	status := http.StatusOK
	if o, ok := w.outage(district); ok {
		body = o
	} else {
		body, status = errorBody("no outage data for "+district), http.StatusNotFound
	}
	log.Printf("outage lookup: district=%s status=%d", district, status)
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	if err := json.NewEncoder(rw).Encode(body); err != nil {
		log.Printf("encode failed: outage-status err=%v", err)
	}
}
