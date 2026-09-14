package main

// Synthetic cardholder records for the card-support demo. Keyed by SSN because
// that is what the agent is given in
// the conversation and what it therefore sends to verify_identity — which is
// the whole point of the demo: the identifier travels through a chat message
// AND a tool call, and the SessionPrivacyPolicy redacts it in both records.
//
// Every value here is invented. 123-45-6789 is the SSN the repo's e2e suite
// used by the deterministic example.
type cardholder struct {
	CustomerRef string `json:"customer_ref"`
	FullName    string `json:"full_name"`
	DOB         string `json:"dob"`
	CardLast4   string `json:"card_last4"`
	CardStatus  string `json:"card_status"`
}

// The member here is the one the Larkmere portal is signed in as — same name,
// same member number, same card. The portal's rail says "Ellen Whitcomb, No.
// 4471-889" and its account list ends 4471, so a conversation about anyone else
// reads as a seam the moment both are on screen together.
var cardholders = map[string]cardholder{
	"123-45-6789": {
		CustomerRef: "MEM-4471-889",
		FullName:    "Ellen Whitcomb",
		DOB:         "1988-03-14",
		CardLast4:   "4471",
		CardStatus:  "active",
	},
}

// identityResponse is what verify_identity returns. It deliberately carries
// only the last four of the card: the supplier of an identity check has no
// business handing back a full PAN, and a demo that returned one would be
// teaching the wrong thing while claiming to teach data minimisation.
type identityResponse struct {
	Verified    bool   `json:"verified"`
	CustomerRef string `json:"customer_ref"`
	FullName    string `json:"full_name"`
	CardLast4   string `json:"card_last4"`
	CardStatus  string `json:"card_status"`
}
