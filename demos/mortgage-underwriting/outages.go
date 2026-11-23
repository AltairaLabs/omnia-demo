package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

// STAND-IN: Wrenmoor's network outage feed.
//
// Fakes: the distribution network operator's live outage data, here one fixed
// storm outage on the 11kV line at Mytholm, covering postcode district HX7.
// Real system: the operator's outage API or the supplier's outage service,
// which changes as crews restore supply.
// Downstream: the answer never changes, so the status stays "off" and the
// restoration estimate never slips or arrives. Anything that wants to show an
// outage ending needs this stand-in to move on.
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

var outages = map[string]outage{
	"HX7": {
		District: "HX7", Status: "off",
		Started: "2026-11-22T21:40:00Z", Estimated: "2026-11-23T16:00:00Z",
		Customers: 1840, Cause: "storm damage to the 11kV line at Mytholm",
	},
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
	if o, ok := outages[district]; ok {
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
