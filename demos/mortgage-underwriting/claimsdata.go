package main

// Synthetic claims for the claims-triage demo.
//
// This lives in the shared stub binary for the same reason verify_identity
// does: it is one more back-office endpoint on a service that already exists,
// and a second image bought nothing but a tidier name. All three demos belong
// to Larkmere Mutual, so the case systems being one service is if anything
// closer to the truth than three would be.
//
// MONEY IS IN PENCE, everywhere, and never a float. A delegated-authority limit
// is the one number in this demo a viewer may check against the policy on
// screen, and "£990.00" arriving as 989.9999999 because it went through a
// float64 would discredit the whole control. The ToolPolicy compares
// amount_pence against 50000 for the same reason.

// The states a claim can be in. Constants because these strings are both
// written to the claim and returned on the wire, so a typo in one place would
// produce a claim that reads as settled to the agent and unsettled to the
// already-settled check that stops a split payment.
const (
	statusAssessed = "assessed"
	statusSettled  = "settled"
	statusReferred = "referred"
)

// claim is a claim as the case system holds it.
type claim struct {
	ClaimRef      string `json:"claim_ref"`
	PolicyRef     string `json:"policy_ref"`
	MemberName    string `json:"member_name"`
	Peril         string `json:"peril"`
	Cover         string `json:"cover"`
	ReportedOn    string `json:"reported_on"`
	AssessedPence int    `json:"assessed_pence"`
	ExcessPence   int    `json:"excess_pence"`
	// PayablePence is assessed less excess, computed by the case system rather
	// than left for the agent to derive. An agent that does its own arithmetic
	// on money is an agent whose arithmetic you have to audit.
	PayablePence int    `json:"payable_pence"`
	PriorClaims  string `json:"prior_claims"`
	Status       string `json:"status"`
}

// The two claims the demo turns on, and they differ in exactly one thing that
// matters: the amount. Same peril class, same member, same cover, same excess
// treatment — so when one settles and one is refused, there is nothing else a
// viewer can attribute the difference to.
//
// LM-40718 is the claim the spec's scripted conversation names
// (the example policy), down to the
// £1,240 assessment, the £250 excess and the £990 payable.
var claims = map[string]claim{
	"LM-40718": {
		ClaimRef:      "LM-40718",
		PolicyRef:     "LM-HOME-4471",
		MemberName:    "Ellen Whitcomb",
		Peril:         "Escape of water",
		Cover:         "Buildings",
		ReportedOn:    "2026-09-02",
		AssessedPence: 124000, // £1,240.00
		ExcessPence:   25000,  // £250.00
		PayablePence:  99000,  // £990.00 — nearly twice the limit
		PriorClaims:   "One prior claim on this policy, November 2023. Nothing flagged.",
		Status:        statusAssessed,
	},
	// The contrast. Under the limit, so the same agent calling the same tool
	// settles it without asking anyone — which is the half of the demo that
	// answers "so the work just doesn't get done?".
	"LM-40733": {
		ClaimRef:      "LM-40733",
		PolicyRef:     "LM-HOME-4471",
		MemberName:    "Ellen Whitcomb",
		Peril:         "Accidental damage",
		Cover:         "Contents",
		ReportedOn:    "2026-09-05",
		AssessedPence: 43000, // £430.00
		ExcessPence:   25000, // £250.00
		PayablePence:  18000, // £180.00 — inside the limit
		PriorClaims:   "One prior claim on this policy, November 2023. Nothing flagged.",
		Status:        statusAssessed,
	},
}

// settlementPolicy is what claim_policy_fetch returns: the rule the agent is
// working under, in the agent's own words rather than the auditor's.
//
// It is returned BY A TOOL rather than baked into the system prompt on purpose.
// A limit the agent has to look up is a limit an operator can change without
// redeploying a pack — and on camera it makes the point that the agent is
// reading policy, not remembering it.
type settlementPolicy struct {
	PolicyRef           string `json:"policy_ref"`
	DelegatedLimitPence int    `json:"delegated_authority_limit_pence"`
	Reference           string `json:"reference"`
	Rule                string `json:"rule"`
	NoSplitRule         string `json:"no_split_rule"`
}

var delegatedAuthority = settlementPolicy{
	PolicyRef:           "DA-2",
	DelegatedLimitPence: 50000, // £500.00
	Reference:           "Larkmere Mutual delegated authority schedule DA-2",
	Rule: "An automated assessor may settle a claim up to £500.00. Above that a " +
		"human approver must authorise the payment.",
	NoSplitRule: "A settlement may not be split into parts to bring each part under " +
		"the limit. Refer the whole claim instead.",
}

// settlementResponse confirms a payment was authorised.
type settlementResponse struct {
	ClaimRef     string `json:"claim_ref"`
	SettledPence int    `json:"settled_pence"`
	PaymentRef   string `json:"payment_ref"`
	PaidTo       string `json:"paid_to"`
	Status       string `json:"status"`
}

// referralResponse confirms a claim was routed to a human.
type referralResponse struct {
	ClaimRef   string `json:"claim_ref"`
	ReferredTo string `json:"referred_to"`
	Queue      string `json:"queue"`
	Status     string `json:"status"`
}
