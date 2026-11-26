package main

// Synthetic home policies for Larkmere Mutual's home claims, keyed by the
// verified email the signed-in member is known by (see the STAND-IN note in
// obo.go). Every value here is invented. Members absent from this map have no
// home policy.
type homePolicy struct {
	PolicyRef  string
	Address    string
	Cover      string
	MemberName string
}

var homePolicies = map[string]homePolicy{
	"ellen.whitcomb@example.com": {
		PolicyRef:  "LM-HOME-4471",
		MemberName: "Ellen Whitcomb",
		Address:    "4 Mytholm Steeps, Hebden Bridge, HX7 6QA",
		Cover:      "buildings and contents",
	},
	"gwen.ashby@example.com": {
		PolicyRef:  "LM-HOME-5133",
		MemberName: "Gwen Ashby",
		Address:    "12 Calder Row, Hebden Bridge, HX7 8AA",
		Cover:      "buildings and contents",
	},
}
