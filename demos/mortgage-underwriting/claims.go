package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
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

// fetchSettlementPolicy returns the delegated authority schedule.
//
// A tool rather than a line in the system prompt: an operator can change the
// limit without redeploying a pack, and on camera it shows the agent reading
// policy rather than remembering it.
func fetchSettlementPolicy(_ toolRequest) (any, int) {
	return reportedSchedule(os.Getenv), http.StatusOK
}

// reportedSchedule is the schedule as the claims system reports it: DA-2, or,
// with CLAIMS_SCHEDULE_LIMIT_PENCE set to a positive whole number of pence,
// DA-2 with that limit.
//
// The override exists for the demo's drift beat. A real claims system's
// schedule can be set wrong, and when it is, the agent believes it: it attempts
// a settlement it thinks is within authority. The ToolPolicy holds the real
// limit independently, so the broker refuses the payment anyway. A value that
// isn't a positive whole number is ignored, so a typo can never make the
// reported limit zero.
func reportedSchedule(getenv func(string) string) settlementPolicy {
	s := delegatedAuthority
	v, err := strconv.Atoi(strings.TrimSpace(getenv("CLAIMS_SCHEDULE_LIMIT_PENCE")))
	if err != nil || v <= 0 {
		return s
	}
	s.DelegatedLimitPence = v
	s.Rule = fmt.Sprintf("An automated assessor may settle a claim up to £%d.%02d. Above that a "+
		"human approver must authorise the payment.", v/100, v%100)
	return s
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

	return referralResponse{
		ClaimRef:   c.ClaimRef,
		ReferredTo: "Ruth Alderman",
		Queue:      "Household claims — approvals above delegated authority",
		Status:     statusReferred,
	}, http.StatusOK
}
