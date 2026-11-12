package main

// Synthetic cardholders for the member assistant, keyed by the email the
// signed-in member is known by. The portal (and the scripted callers) verify
// who the member is before the conversation starts, and Omnia's runtime
// forwards that member's email on every tool call (X-Omnia-User-Email), so
// the card system looks the member up by it. Nobody proves who they are in
// the chat.
//
// Every value here is invented.
type cardholder struct {
	FullName   string
	CardLast4  string
	CardStatus string
}

// The first member is the one the Larkmere portal is signed in as — same name,
// same member number (4471-889), same card. The portal's rail says "Ellen
// Whitcomb" and its account list ends 4471, so a conversation about anyone
// else reads as a seam the moment both are on screen together. The other four
// are the canary's scripted callers (demos/card-callers).
var cardholders = map[string]cardholder{
	"ellen.whitcomb@example.com":  {FullName: "Ellen Whitcomb", CardLast4: "4471", CardStatus: "active"},
	"tomasz.brennan@example.com":  {FullName: "Tomasz Brennan", CardLast4: "2041", CardStatus: "active"},
	"priya.hallworth@example.com": {FullName: "Priya Hallworth", CardLast4: "3318", CardStatus: "active"},
	"gwen.ashby@example.com":      {FullName: "Gwen Ashby", CardLast4: "5562", CardStatus: "active"},
	"callum.reyes@example.com":    {FullName: "Callum Reyes", CardLast4: "7709", CardStatus: "active"},
}
