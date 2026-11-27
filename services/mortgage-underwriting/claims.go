package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// Claims-triage tool implementations. See claimsdata.go for the fixtures and
// for why money is in pence.

// fetchClaim returns the claim as the case system holds it, including what is
// payable after the excess. The agent does not do the arithmetic.
func fetchClaim(req toolRequest) (any, int) {
	c, ok := claims[req.ClaimRef]
	if !ok {
		return errorBody("claim not found"), http.StatusNotFound
	}
	return c, http.StatusOK
}

// claimPerilFact answers the claims ToolPolicy's http producer: the peril of
// the claim a tool call names, so the policy can hold a peril rule (flood
// claims go to Flood Response, CP-7) that the call's own arguments can't
// express. The agent is never told the rule; the policy asks the system of
// record.
//
// The request is the policy broker's ({"producer", "tool", "args", "run"}), and
// the answer is {"value": <string>}. A call that names no claim, or one this
// system doesn't hold, answers "": the fact is always a string, so it never
// fails a call that has nothing to do with a claim's peril.
func claimPerilFact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Args struct {
			ClaimRef string `json:"claim_ref"`
		} `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	peril := claims[req.Args.ClaimRef].Peril
	log.Printf("policy lookup claim-peril claim_ref=%q peril=%q", req.Args.ClaimRef, peril)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"value": peril})
}

// settleClaim authorises payment.
//
// THIS SERVICE DOES NOT ENFORCE THE LIMIT, and that is deliberate rather than
// an omission. The control under demonstration is the ToolPolicy, evaluated by
// the policy broker before the call is dispatched — so an over-limit request
// never reaches this function. Enforcing it here as well would make the demo
// impossible to read: you could not tell whether the refusal came from the
// policy or from the back end quietly refusing anyway, which is precisely the
// ambiguity a viewer is entitled to be suspicious of.
//
// The second-guess check that DOES belong here is the one the policy cannot
// make: whether this claim was already settled. That is state, and a stateless
// CEL producer has no way to see it.
func settleClaim(req toolRequest) (any, int) {
	c, ok := claims[req.ClaimRef]
	if !ok {
		return errorBody("claim not found"), http.StatusNotFound
	}
	if c.Status == statusSettled {
		return errorBody("claim is already settled"), http.StatusConflict
	}
	if req.AmountPence <= 0 {
		return errorBody("amount_pence must be a positive number of pence"), http.StatusBadRequest
	}
	if req.AmountPence > c.PayablePence {
		return errorBody(fmt.Sprintf(
			"amount exceeds what is payable on this claim (%d pence)", c.PayablePence,
		)), http.StatusBadRequest
	}

	c.Status = statusSettled
	claims[req.ClaimRef] = c

	return settlementResponse{
		ClaimRef:     c.ClaimRef,
		SettledPence: req.AmountPence,
		PaymentRef:   "PAY-" + c.ClaimRef,
		PaidTo:       "Everyday checking ending 4471",
		Status:       statusSettled,
	}, http.StatusOK
}

// referClaim routes a claim to a human approver.
//
// This is what makes the refusal a control rather than a dead end: the agent
// that cannot settle can still do the useful thing, and the claim ends the
// conversation in a defined state rather than abandoned.
func referClaim(req toolRequest) (any, int) {
	c, ok := claims[req.ClaimRef]
	if !ok {
		return errorBody("claim not found"), http.StatusNotFound
	}

	c.Status = statusReferred
	claims[req.ClaimRef] = c

	// Routed by peril, as a claims system routes work: the agent gives the
	// reason, never the destination.
	to, queue := "Ruth Alderman", "Household claims approvals"
	if c.Peril == "Flood" {
		to, queue = "Dev Okafor", "Flood Response"
	}
	return referralResponse{
		ClaimRef:   c.ClaimRef,
		ReferredTo: to,
		Queue:      queue,
		Status:     statusReferred,
	}, http.StatusOK
}
