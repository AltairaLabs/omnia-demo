package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
)

// cardDesk is the card system behind Larkmere's member assistant: the signed-in
// member's card, and whether it is frozen.
//
// The member is the one Omnia's runtime names on the tool call: it forwards the
// email the edge asserted for the verified user (X-Omnia-User-Email; see
// wren.go for why the email, not X-Omnia-User-Id, which is a pseudonym). No
// argument the model writes selects the member, so the agent can act only on
// the card of the person it is talking to. A call with no member acts on no
// card at all.
//
// The routes take no arguments for the same reason. Before they existed, card
// support said "your card has been frozen" with nothing to freeze it with: give
// an agent the tools for what it is asked to do, or it will pretend
// (docs/stand-ins.md).
//
// State is in memory, per server, like the other stubs: a restart unfreezes
// every card.
type cardDesk struct {
	mu     sync.Mutex
	status map[string]string // by member email; absent means the default
}

func newCardDesk() *cardDesk {
	return &cardDesk{status: map[string]string{}}
}

func (d *cardDesk) register(mux *http.ServeMux) {
	mux.HandleFunc("/card-status", d.handle("get_card_status", ""))
	mux.HandleFunc("/card-freeze", d.handle("freeze_card", "frozen"))
	mux.HandleFunc("/card-unfreeze", d.handle("unfreeze_card", "active"))
}

// cardStatusResponse is what every card route returns.
type cardStatusResponse struct {
	FullName   string `json:"full_name"`
	CardLast4  string `json:"card_last4"`
	CardStatus string `json:"card_status"`
	// Changed is false when the card was already in the requested state (and
	// always for get_card_status), so the agent can say so rather than claim
	// it did something.
	Changed bool `json:"changed"`
}

// handle serves one card route. set is the status it sets; empty only reads.
func (d *cardDesk) handle(tool, set string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		email := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Omnia-User-Email")))
		// Logged by card, never by email: the log is on camera.
		status, body := d.serve(email, set)
		log.Printf("tool response: tool=%s status=%d", tool, status)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
}

func (d *cardDesk) serve(email, set string) (int, any) {
	if email == "" {
		return http.StatusUnauthorized, errorBody("no verified member on this call")
	}
	h, ok := cardholders[email]
	if !ok {
		return http.StatusNotFound, errorBody("no card on file for this member")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	current, ok := d.status[email]
	if !ok {
		current = h.CardStatus
	}
	changed := set != "" && set != current
	if set != "" {
		d.status[email] = set
		current = set
	}
	log.Printf("tool request served: card=%s", h.CardLast4)
	return http.StatusOK, cardStatusResponse{FullName: h.FullName, CardLast4: h.CardLast4, CardStatus: current, Changed: changed}
}
