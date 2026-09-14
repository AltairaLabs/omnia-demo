// Package main implements deterministic stub services for the public examples.
//
// mortgageUnderwriting: the case system, the credit bureau, the AVM, the
// affordability engine, the AML screen, the case-note store, and the
// third-party income-verification supplier.
//
// piiChat: the identity check behind the card-support agent's verify_identity
// tool. It lives here, in a directory named for the other demo, because both
// need exactly one thing — a real HTTP endpoint for a server-executed tool to
// call — and a second image to build, publish and keep alive would buy nothing
// but a tidier path. The directory name is the cost; see PII_CHAT.md.
//
// Why any of this is a real HTTP service rather than canned data inside the
// runtime: a ToolRegistry handler is one of http/openapi/grpc/mcp/client.
// There is no "mock" handler type, so a server-executed tool needs something
// real to call — and `client` is not an option, because client tools are
// browser-executed and never reach the runtime dispatch chokepoint the policy
// broker guards.
//
// The income supplier matters most. It is a separate endpoint so the example
// OUTSIDE the firm, and the demo's whole claim is that raw statement data
// never reached it. That claim is only observable because there is a real
// process on the other side of a real network hop that can be asked whether it
// was called. Fold the supplier into the runtime and the boundary — and the
// proof — disappear with it.
package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

// toolRequest is the union of the fields the tools send. One struct rather
// than seven: every endpoint keys off a case or applicant reference, and the
// handlers stay uniform enough to share a decode path.
type toolRequest struct {
	CaseRef      string `json:"case_ref"`
	ApplicantRef string `json:"applicant_ref"`

	// EmployerName and DeclaredAnnualIncome are what income_verify is
	// permitted to send: the derived summary.
	EmployerName         string  `json:"employer_name"`
	DeclaredAnnualIncome float64 `json:"declared_annual_income"`

	// Evidence is the field the ToolPolicy inspects. Raw statement items are
	// refused before dispatch, so a request carrying them never arrives here —
	// which is the point.
	Evidence []evidenceItem `json:"evidence"`

	// Note is the underwriter case note, for case_note_write.
	Note           string `json:"note"`
	Recommendation string `json:"recommendation"`

	// SSN and DOB belong to verify_identity, in the card-support demo. They
	// are the only PII this service accepts, and they are never logged — see
	// verifyIdentity.
	SSN string `json:"ssn"`
	DOB string `json:"dob"`

	// ClaimRef, AmountPence and Reason belong to claims triage.
	//
	// AmountPence is an int, and the ToolPolicy reads the same field off the
	// wire before this service ever sees it. That is the whole control: a
	// settlement above the delegated limit is refused at dispatch, so a
	// request carrying one never arrives here.
	ClaimRef    string `json:"claim_ref"`
	AmountPence int    `json:"amount_pence"`
	Reason      string `json:"reason"`
}

type evidenceItem struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// ref returns the case reference the request is about, accepting either
// spelling so a tool that thinks in applicants and one that thinks in cases
// can both be answered.
func (r toolRequest) ref() string {
	if r.CaseRef != "" {
		return r.CaseRef
	}
	return r.ApplicantRef
}

// verifyResponse is the income supplier's verdict.
type verifyResponse struct {
	Verified   bool    `json:"verified"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
}

// caseNoteResponse confirms the note was filed.
type caseNoteResponse struct {
	CaseRef  string `json:"case_ref"`
	NoteID   string `json:"note_id"`
	FiledFor string `json:"filed_for_review_by"`
}

// newServer returns the handler for every stub back-office service.
func newServer() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/case", jsonTool("case_fetch", fetchCase))
	mux.HandleFunc("/credit-report", jsonTool("credit_report", fetchCredit))
	mux.HandleFunc("/verify", jsonTool("income_verify", verifyIncome))
	mux.HandleFunc("/affordability", jsonTool("affordability_assess", assessAffordability))
	mux.HandleFunc("/valuation", jsonTool("property_valuation", valueProperty))
	mux.HandleFunc("/aml-screen", jsonTool("aml_screen", screenAML))
	mux.HandleFunc("/case-note", jsonTool("case_note_write", writeCaseNote))
	mux.HandleFunc("/verify-identity", jsonTool("verify_identity", verifyIdentity))
	// Claims triage. claim_settle is the one the ToolPolicy guards; the other
	// three are the reads and the escape hatch that make the refusal
	// actionable rather than a dead end.
	mux.HandleFunc("/claim", jsonTool("claim_fetch", fetchClaim))
	mux.HandleFunc("/claim-policy", jsonTool("claim_policy_fetch", fetchSettlementPolicy))
	mux.HandleFunc("/claim-settle", jsonTool("claim_settle", settleClaim))
	mux.HandleFunc("/claim-refer", jsonTool("claim_refer", referClaim))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

// jsonTool wraps a tool implementation with POST-only checking, JSON decode,
// JSON encode, and — importantly — a log line per request.
//
// The logging is not decoration. It exists so "the payload never reached the
// supplier" is a claim you CHECK rather than one you infer from an empty log.
// Without it, a log with no request lines looks identical whether the policy
// blocked the call or the service simply never says anything, and that
// difference is the entire demo. Count the lines; the count either moves or
// it does not.
func jsonTool(name string, fn func(toolRequest) (any, int)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// An EMPTY body is valid, and treating it as a decode failure cost a
		// live run of the claims demo.
		//
		// claim_policy_fetch takes no arguments. The scripted provider sends
		// `{}` so this decoded fine for months; a real model calling the same
		// tool sends no body at all, Decode returns io.EOF, and the request
		// 400s — BEFORE the log line below, so the service that is supposed to
		// make "did the call arrive?" checkable said nothing either. The agent
		// reported "unable to retrieve the delegated authority schedule due to
		// a system error" and referred the claim without ever learning its own
		// limit, which reads as the demo working.
		//
		// io.EOF here means "no body", which for a no-argument tool is exactly
		// right. Malformed JSON still fails.
		var req toolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		log.Printf("tool request received: tool=%s ref=%s remote=%s", name, req.ref(), r.RemoteAddr)

		body, status := fn(req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			log.Printf("encode failed: tool=%s err=%v", name, err)
		}
	}
}

// errorBody is what an unknown case reference gets back. A tool that returns a
// structured "not found" teaches the agent something; one that returns a bare
// 500 just makes it retry.
func errorBody(msg string) map[string]string {
	return map[string]string{"error": msg}
}

func fetchCase(req toolRequest) (any, int) {
	rec, ok := cases[req.ref()]
	if !ok {
		return errorBody("no such case reference"), http.StatusNotFound
	}
	return rec, http.StatusOK
}

func fetchCredit(req toolRequest) (any, int) {
	rep, ok := creditReports[req.ref()]
	if !ok {
		return errorBody("no bureau record for applicant"), http.StatusNotFound
	}
	return rep, http.StatusOK
}

// verifyIncome is the third-party supplier. The verdict is constant because
// the demo turns on whether the call is ALLOWED to happen, not on what the
// supplier would have said.
func verifyIncome(req toolRequest) (any, int) {
	if _, ok := cases[req.ref()]; !ok {
		return errorBody("no such applicant"), http.StatusNotFound
	}
	return verifyResponse{
		Verified:   true,
		Confidence: 0.94,
		Source:     "Employer payroll record (synthetic)",
	}, http.StatusOK
}

func assessAffordability(req toolRequest) (any, int) {
	res, ok := affordability[req.ref()]
	if !ok {
		return errorBody("no such case reference"), http.StatusNotFound
	}
	return res, http.StatusOK
}

func valueProperty(req toolRequest) (any, int) {
	val, ok := valuations[req.ref()]
	if !ok {
		return errorBody("no such case reference"), http.StatusNotFound
	}
	return val, http.StatusOK
}

func screenAML(req toolRequest) (any, int) {
	res, ok := amlResults[req.ref()]
	if !ok {
		return errorBody("no such applicant"), http.StatusNotFound
	}
	return res, http.StatusOK
}

// writeCaseNote files the underwriter note. It records rather than decides:
// the note is filed FOR a named human, which is the same boundary the pack's
// governance metadata asserts.
func writeCaseNote(req toolRequest) (any, int) {
	if _, ok := cases[req.ref()]; !ok {
		return errorBody("no such case reference"), http.StatusNotFound
	}
	return caseNoteResponse{
		CaseRef:  req.ref(),
		NoteID:   "NOTE-" + req.ref(),
		FiledFor: "R. Nkemdirim, Head of Credit Risk",
	}, http.StatusOK
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
func verifyIdentity(req toolRequest) (any, int) {
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
	return identityResponse{
		Verified:    true,
		CustomerRef: h.CustomerRef,
		FullName:    h.FullName,
		CardLast4:   h.CardLast4,
		CardStatus:  h.CardStatus,
	}, http.StatusOK
}
