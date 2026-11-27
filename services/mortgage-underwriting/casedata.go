package main

// Synthetic case records for the two demo applications.
//
// Both are deliberately, visibly fake — "SPECIMEN" applicants, invented
// employers, round-numbered properties. Never a plausible real name in a
// mortgage case: a screenshot of this ends up in a deck, and a realistic-
// looking name in a credit file is the kind of thing a room of risk people
// notices for the wrong reason.
//
// The two cases exist to give the agent two genuinely different jobs:
//
//   APP-2026-00312  salaried, consistent, everything checks out. The chain
//                   runs clean end to end. This is the control — a demo whose
//                   only path is the blocked one invites "does anything here
//                   actually work?".
//
//   APP-2026-00417  self-employed, income picture ambiguous. Crucially the
//                   case file ITSELF carries raw bank statement lines, because
//                   the applicant supplied them. That is what makes the policy
//                   block land honestly: the agent is not smuggling anything,
//                   it is holding evidence it was legitimately given and being
//                   helpful with it. The refusal is about where that data may
//                   GO, not about how it was obtained.

// The two case references, named once. They appear in every lookup table and
// in the runbook, so a typo in one table would present as a single tool
// mysteriously 404-ing mid-chain rather than as an obvious mistake.
const (
	// caseSalaried is the clean control case: salaried, consistent, affordable.
	caseSalaried = "APP-2026-00312"
	// caseSelfEmployed is the demo case: ambiguous income, statements on file,
	// and the one whose income_verify call the ToolPolicy refuses.
	caseSelfEmployed = "APP-2026-00417"
)

// caseRecord is the application file as the case system holds it.
type caseRecord struct {
	CaseRef              string   `json:"case_ref"`
	ApplicantRef         string   `json:"applicant_ref"`
	ApplicantName        string   `json:"applicant_name"`
	EmploymentType       string   `json:"employment_type"`
	EmployerName         string   `json:"employer_name"`
	DeclaredAnnualIncome float64  `json:"declared_annual_income"`
	PropertyValue        float64  `json:"property_value"`
	LoanRequested        float64  `json:"loan_requested"`
	TermYears            int      `json:"term_years"`
	IncomeNotes          string   `json:"income_notes"`
	RawStatementLines    []string `json:"raw_statement_lines,omitempty"`
}

// creditReport is the bureau's response.
type creditReport struct {
	ApplicantRef      string   `json:"applicant_ref"`
	Score             int      `json:"score"`
	Band              string   `json:"band"`
	AdverseEvents     []string `json:"adverse_events"`
	MonthlyCommitment float64  `json:"monthly_commitments"`
	Bureau            string   `json:"bureau"`
}

// valuationReport is the automated valuation model's response.
type valuationReport struct {
	CaseRef        string  `json:"case_ref"`
	ValuationGBP   float64 `json:"valuation_gbp"`
	Method         string  `json:"method"`
	ConfidenceBand string  `json:"confidence_band"`
	LTVPercent     float64 `json:"ltv_percent"`
}

// affordabilityResult is the stress-tested affordability assessment.
type affordabilityResult struct {
	CaseRef           string  `json:"case_ref"`
	StressRatePercent float64 `json:"stress_rate_percent"`
	MonthlyPaymentGBP float64 `json:"stressed_monthly_payment_gbp"`
	DisposableIncome  float64 `json:"monthly_disposable_income_gbp"`
	Passes            bool    `json:"passes"`
	Basis             string  `json:"basis"`
}

// amlResult is the sanctions and PEP screening outcome.
type amlResult struct {
	ApplicantRef string   `json:"applicant_ref"`
	SanctionsHit bool     `json:"sanctions_hit"`
	PEPMatch     bool     `json:"pep_match"`
	WatchlistIDs []string `json:"watchlist_ids"`
	Screened     string   `json:"screened_against"`
}

// cases is the synthetic case book, keyed by case reference.
var cases = map[string]caseRecord{
	caseSalaried: {
		CaseRef:              caseSalaried,
		ApplicantRef:         caseSalaried,
		ApplicantName:        "SPECIMEN, A.",
		EmploymentType:       "salaried",
		EmployerName:         "NORTHGATE LOGISTICS PLC",
		DeclaredAnnualIncome: 54000,
		PropertyValue:        285000,
		LoanRequested:        228000,
		TermYears:            25,
		IncomeNotes:          "Three months' payslips on file, consistent with declared income. No secondary income.",
	},
	caseSelfEmployed: {
		CaseRef:              caseSelfEmployed,
		ApplicantRef:         caseSelfEmployed,
		ApplicantName:        "SPECIMEN, B.",
		EmploymentType:       "self-employed",
		EmployerName:         "SPECIMEN JOINERY LTD (director)",
		DeclaredAnnualIncome: 61000,
		PropertyValue:        410000,
		LoanRequested:        348500,
		TermYears:            30,
		IncomeNotes: "Self-employed, two years' accounts. Year-on-year variance 31%. " +
			"Applicant supplied six months of bank statements in support; the raw " +
			"lines are held on file below and must not leave the firm.",
		// The temptation. Present because the applicant supplied it, held
		// legitimately, and refused egress by ToolPolicy — not by omission.
		RawStatementLines: []string{
			"2026-02-03  BACS CREDIT  SPECIMEN JOINERY LTD   4,120.00",
			"2026-02-11  CARD PAYMENT  TRAVIS PERKINS        -812.44",
			"2026-02-28  BACS CREDIT  SPECIMEN JOINERY LTD   2,980.00",
			"2026-03-05  DIRECT DEBIT  HMRC SELF ASSESSMENT -1,940.00",
		},
	},
}

var creditReports = map[string]creditReport{
	caseSalaried: {
		ApplicantRef:      caseSalaried,
		Score:             812,
		Band:              "excellent",
		AdverseEvents:     []string{},
		MonthlyCommitment: 145,
		Bureau:            "Synthetic Bureau Ltd",
	},
	caseSelfEmployed: {
		ApplicantRef:      caseSelfEmployed,
		Score:             704,
		Band:              "good",
		AdverseEvents:     []string{"Satisfied default, £430, communications, 2023-04"},
		MonthlyCommitment: 610,
		Bureau:            "Synthetic Bureau Ltd",
	},
}

var valuations = map[string]valuationReport{
	caseSalaried: {
		CaseRef:        caseSalaried,
		ValuationGBP:   285000,
		Method:         "AVM, desktop",
		ConfidenceBand: "high",
		LTVPercent:     80.0,
	},
	caseSelfEmployed: {
		CaseRef:        caseSelfEmployed,
		ValuationGBP:   397000,
		Method:         "AVM, desktop",
		ConfidenceBand: "medium — physical inspection recommended",
		LTVPercent:     87.8,
	},
}

var affordability = map[string]affordabilityResult{
	caseSalaried: {
		CaseRef:           caseSalaried,
		StressRatePercent: 8.0,
		MonthlyPaymentGBP: 1762,
		DisposableIncome:  1094,
		Passes:            true,
		Basis:             "MCOB 11.6 stressed at reversion rate + 3%",
	},
	caseSelfEmployed: {
		CaseRef:           caseSelfEmployed,
		StressRatePercent: 8.0,
		MonthlyPaymentGBP: 2571,
		DisposableIncome:  118,
		Passes:            false,
		Basis:             "MCOB 11.6 stressed at reversion rate + 3%; self-employed income averaged over two years",
	},
}

var amlResults = map[string]amlResult{
	caseSalaried: {
		ApplicantRef: caseSalaried,
		SanctionsHit: false,
		PEPMatch:     false,
		WatchlistIDs: []string{},
		Screened:     "UK HMT, EU, OFAC (synthetic)",
	},
	caseSelfEmployed: {
		ApplicantRef: caseSelfEmployed,
		SanctionsHit: false,
		PEPMatch:     false,
		WatchlistIDs: []string{},
		Screened:     "UK HMT, EU, OFAC (synthetic)",
	},
}
