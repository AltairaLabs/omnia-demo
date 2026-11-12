package main

import (
	"net/http"
	"sync"
)

// cardDesk is the card system behind card support: who holds which card, and
// whether it is frozen. verify_identity reads the status from here, so a card
// frozen in one conversation is reported frozen in the next.
//
// Before it existed, card support had only verify_identity, and a live model
// asked to freeze a card said "your card has been frozen" with nothing to
// freeze it with; the next conversation found it active. A stand-in that
// cannot do what the agent is asked to do teaches the agent to pretend
// (docs/stand-ins.md).
//
// State is in memory, per server, like the other stubs: a restart unfreezes
// every card.
type cardDesk struct {
	mu sync.Mutex
	// status by customer reference; a reference not here has its default
	// status from cardholders.
	status map[string]string
}

func newCardDesk() *cardDesk {
	return &cardDesk{status: map[string]string{}}
}

func (d *cardDesk) register(mux *http.ServeMux) {
	mux.HandleFunc("/verify-identity", jsonTool("verify_identity", d.verifyIdentity))
	mux.HandleFunc("/card-freeze", jsonTool("freeze_card", d.freeze))
	mux.HandleFunc("/card-unfreeze", jsonTool("unfreeze_card", d.unfreeze))
}

// cardStatusResponse is what freeze_card and unfreeze_card return.
type cardStatusResponse struct {
	CustomerRef string `json:"customer_ref"`
	CardLast4   string `json:"card_last4"`
	CardStatus  string `json:"card_status"`
	// Changed is false when the card was already in the requested state, so
	// the agent can say so rather than claim it did something.
	Changed bool `json:"changed"`
}

// holderByRef finds a cardholder by customer reference: the freeze tools take
// the reference verify_identity returned, never the SSN, so the SSN travels
// through one tool call only.
func holderByRef(ref string) (cardholder, bool) {
	for _, h := range cardholders {
		if h.CustomerRef == ref {
			return h, true
		}
	}
	return cardholder{}, false
}

func (d *cardDesk) statusOf(h cardholder) string {
	if s, ok := d.status[h.CustomerRef]; ok {
		return s
	}
	return h.CardStatus
}

// verifyIdentity is the card-support demo's identity check. It requires BOTH
// the SSN and the date of birth, so the agent genuinely has to put the SSN in
// the tool call rather than merely mention it in conversation — the recorded
// tool-call arguments are half of what the demo shows.
//
// Note what is missing: a log line naming the subject. jsonTool logs
// req.ref(), which is empty here, and that is deliberate. The only identifier
// this endpoint receives is an SSN, and a demo about not persisting SSNs must
// not print one into a container log that someone may well open on camera.
func (d *cardDesk) verifyIdentity(req toolRequest) (any, int) {
	h, ok := cardholders[req.SSN]
	if !ok {
		// Same answer for an unknown SSN and a mismatched DOB. Distinguishing
		// them would turn this into an oracle that confirms whether a given
		// SSN is on file.
		return errorBody("identity could not be verified"), http.StatusNotFound
	}
	if req.DOB != h.DOB {
		return errorBody("identity could not be verified"), http.StatusNotFound
	}
	d.mu.Lock()
	status := d.statusOf(h)
	d.mu.Unlock()
	return identityResponse{
		Verified:    true,
		CustomerRef: h.CustomerRef,
		FullName:    h.FullName,
		CardLast4:   h.CardLast4,
		CardStatus:  status,
	}, http.StatusOK
}

func (d *cardDesk) freeze(req toolRequest) (any, int)   { return d.set(req, "frozen") }
func (d *cardDesk) unfreeze(req toolRequest) (any, int) { return d.set(req, "active") }

func (d *cardDesk) set(req toolRequest, status string) (any, int) {
	h, ok := holderByRef(req.CustomerRef)
	if !ok {
		return errorBody("no card for that customer reference; verify the caller first"), http.StatusNotFound
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	changed := d.statusOf(h) != status
	d.status[h.CustomerRef] = status
	return cardStatusResponse{CustomerRef: h.CustomerRef, CardLast4: h.CardLast4, CardStatus: status, Changed: changed}, http.StatusOK
}
